//go:build integration

// Task 10 — Phase 4 ACP↔MCP bridge: integration test with real claude-agent-acp.
//
// What this test proves end-to-end (that unit tests cannot):
//
//   - The mcp_shim.Server can be wired into a real acp.ProcessPool via
//     mcp_shim.NewHandle and pool.SetShim.
//   - A real claude-agent-acp subprocess can be spawned, completes the ACP
//     `initialize` handshake against our jsonrpc Conn, and advertises
//     MCPCapabilities.HTTP — which is the gate that decides whether the shim
//     URL is even threaded into NewSessionRequest.McpServers.
//   - session/new with the shim URL in McpServers is accepted by the agent
//     (i.e. the upstream binary parses our URL and returns a session id).
//   - The shim's per-session entry is registered before session/new returns
//     (acp.shim.session_registered emitted via slog).
//
// This deliberately stops short of issuing a real Anthropic API prompt. The
// task spec allows scoping down to "spawn + handshake + session/new accepted"
// when a full LLM loop would balloon the test surface — that's the path
// chosen here. A follow-up test gated on ACP_SHIM_SMOKE_FULL=1 can drive a
// real tools/call once we have a stable cron path for it.
//
// Skip preconditions (all three must be satisfied or the test exits cleanly):
//   - `claude-agent-acp` on PATH (exec.LookPath)
//   - ANTHROPIC_API_KEY or CLAUDE_CODE_OAUTH_TOKEN in env (upstream auth)
//   - Test not running with -short
//
// On a CI runner without the binary the expected outcome is `--- SKIP: ...`.
// The test must NEVER fail unless the integration is genuinely broken.

package integration

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"os/exec"
	"sync"
	"testing"
	"time"

	acpprov "github.com/nextlevelbuilder/goclaw/internal/providers/acp"
	"github.com/nextlevelbuilder/goclaw/internal/providers/acp/mcp_shim"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// smokeTool is the single allowlisted tool the shim advertises for this test.
// Its Execute is intentionally trivial — we never actually invoke it through
// the LLM path in this build; the tool exists so the registry has at least
// one entry that the shim can register, and so the per-session allowlist has
// a real referent.
type smokeTool struct{}

func (smokeTool) Name() string        { return "acp_shim_smoke_ping" }
func (smokeTool) Description() string { return "smoke-test ping tool" }
func (smokeTool) Parameters() map[string]any {
	return map[string]any{
		"type":       "object",
		"properties": map[string]any{},
	}
}
func (smokeTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	return tools.NewResult("pong")
}

// capturingHandler buffers slog records so the test can assert which
// shim/audit messages fired during the spawn → session/new sequence.
type capturingHandler struct {
	mu       sync.Mutex
	inner    slog.Handler
	messages []string
}

func (h *capturingHandler) Enabled(ctx context.Context, lvl slog.Level) bool {
	return h.inner.Enabled(ctx, lvl)
}
func (h *capturingHandler) Handle(ctx context.Context, r slog.Record) error {
	h.mu.Lock()
	h.messages = append(h.messages, r.Message)
	h.mu.Unlock()
	return h.inner.Handle(ctx, r)
}
func (h *capturingHandler) WithAttrs(attrs []slog.Attr) slog.Handler {
	return &capturingHandler{inner: h.inner.WithAttrs(attrs), messages: h.messages}
}
func (h *capturingHandler) WithGroup(name string) slog.Handler {
	return &capturingHandler{inner: h.inner.WithGroup(name), messages: h.messages}
}
func (h *capturingHandler) has(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, m := range h.messages {
		if m == msg {
			return true
		}
	}
	return false
}

// TestACPShimSmoke spawns a real claude-agent-acp process, wires the shim
// into the pool, and verifies session/new succeeds with the shim URL in
// McpServers.
//
// Runs ~5-15s on a workstation with claude-agent-acp installed + a valid
// ANTHROPIC_API_KEY. Skips cleanly on CI machines without those.
func TestACPShimSmoke(t *testing.T) {
	if testing.Short() {
		t.Skip("ACP shim integration test skipped under -short")
	}

	binary, err := exec.LookPath("claude-agent-acp")
	if err != nil {
		t.Skip("claude-agent-acp not on PATH; skipping ACP shim integration test")
	}

	// Either env satisfies upstream auth — production setups use one or
	// the other (API key vs Claude Max OAuth token).
	if os.Getenv("ANTHROPIC_API_KEY") == "" && os.Getenv("CLAUDE_CODE_OAUTH_TOKEN") == "" {
		t.Skip("ANTHROPIC_API_KEY/CLAUDE_CODE_OAUTH_TOKEN not set; skipping ACP shim integration test")
	}

	// Bounded deadline so a wedged spawn or stuck handshake never hangs CI.
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Capture slog so we can assert acp.shim.session_registered fired. Restore
	// the previous default on test exit so neighbouring tests (if any run in
	// the same package later) see the original handler.
	prev := slog.Default()
	cap := &capturingHandler{inner: prev.Handler()}
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })

	// Tool registry with one harmless tool. The shim registers every tool
	// at startup; the per-session allowlist below narrows it to just ours.
	reg := tools.NewRegistry()
	reg.Register(smokeTool{})

	shim, err := mcp_shim.NewServer(mcp_shim.ServerConfig{
		ListenAddr: "127.0.0.1:0",
		Registry:   reg,
		Version:    "integration-test",
	})
	if err != nil {
		t.Fatalf("mcp_shim.NewServer: %v", err)
	}
	t.Cleanup(func() { _ = shim.Close() })

	shimHandle := mcp_shim.NewHandle(shim)

	// Real ProcessPool against the on-PATH binary. workDir is a temp dir to
	// keep the spawned agent from touching anything in the repo.
	workDir := t.TempDir()
	pool := acpprov.NewProcessPool(binary, nil, workDir, 1*time.Minute)
	pool.SetShim(shimHandle)
	pool.SetToolHandler(func(ctx context.Context, method string, params json.RawMessage) (any, error) {
		// No-op handler — claude-agent-acp may issue fs/terminal requests
		// during initialize; returning nil lets it proceed with defaults.
		return nil, nil
	})
	t.Cleanup(func() { _ = pool.Close() })

	t.Logf("spawning %s in %s", binary, workDir)
	proc, err := pool.GetOrSpawn(ctx, "smoke")
	if err != nil {
		t.Fatalf("GetOrSpawn: %v", err)
	}

	caps := proc.AgentCaps()
	if caps.MCPCapabilities == nil || !caps.MCPCapabilities.HTTP {
		// Without HTTP MCP support the shim URL never gets advertised, so
		// the rest of the test would be a no-op. This is an upstream
		// regression and warrants a failure rather than a skip.
		t.Fatalf("claude-agent-acp did not advertise MCPCapabilities.HTTP "+
			"(caps=%+v); shim wiring would be silently disabled", caps)
	}
	t.Logf("agent advertised MCPCapabilities.HTTP=true")

	var registeredSID string
	sid, err := pool.NewSessionWithShim(ctx, proc, func(s string) {
		registeredSID = s
		shimHandle.RegisterSession(acpprov.ShimSessionEntry{
			SID:       s,
			Allowlist: []string{"acp_shim_smoke_ping"},
			AgentID:   "integration-test",
		})
	})
	if err != nil {
		t.Fatalf("NewSessionWithShim: %v", err)
	}
	if sid == "" {
		t.Fatal("NewSessionWithShim returned empty session id")
	}
	if registeredSID == "" {
		t.Fatal("register callback was never invoked — shim URL was not advertised")
	}
	t.Logf("session/new returned sid=%s (registered=%s)", sid, registeredSID)

	// session_registered is emitted by Server.RegisterSession; if the register
	// callback fired and routed through Handle, this must be present.
	if !cap.has("acp.shim.session_registered") {
		t.Errorf("expected slog message acp.shim.session_registered, not found")
	}

	// Cleanup explicit so the agent doesn't keep a stale session entry —
	// the pool.Close cleanup above will tear the process down regardless,
	// but unregistering here exercises the symmetric path.
	shimHandle.UnregisterSession(sid)
	if !cap.has("acp.shim.session_unregistered") {
		t.Errorf("expected slog message acp.shim.session_unregistered, not found")
	}
}
