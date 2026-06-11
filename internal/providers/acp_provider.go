package providers

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/providers/acp"
)

// ACPToolResolver computes the per-session allowlist (tool names) that the
// shim should expose to the ACP sub-session. The concrete implementation
// (mcp_shim.Resolver) lives in providers/acp/mcp_shim — defining the
// surface here as an interface keeps acp_provider.go cycle-free
// (mcp_shim → internal/mcp → internal/store → internal/providers).
//
// The returned slice carries only the names; the shim's SessionEntry
// stores them as-is and the session-aware tool handler enforces the
// allowlist at /mcp request time.
type ACPToolResolver interface {
	ResolveACPToolNames(ctx context.Context, agentID, tenantID string) ([]string, error)
}

// ACPRoutingContext is the snapshot of tool-routing values the shim needs
// to inject when builtin tools (write_file deliver=true, message, etc.) run
// inside an ACP sub-session. Provided by ACPContextReader.
type ACPRoutingContext struct {
	AgentKey   string
	ChannelID  string
	ChatID     string
	PeerKind   string
	SessionKey string
}

// ACPContextReader extracts the cron/routing context from the parent ctx so
// the shim's SessionEntry can carry it into per-tool dispatch. The cmd
// layer plugs in a concrete reader that calls tools.Tool*FromCtx; defining
// the surface as an interface keeps acp_provider.go from importing
// internal/tools (which would create providers → tools → store → providers).
type ACPContextReader interface {
	ReadRouting(ctx context.Context) ACPRoutingContext
}

// ACPConfigReader reads a single tenant-scoped system_configs value used by
// the shim feature-flag (Task 9 rollback kill switch). The cmd layer plugs
// in an adapter that wraps store.SystemConfigStore + binds the owning
// tenant on ctx — keeping providers/ free of the internal/store import
// (store → providers cycle).
//
// Returns the raw string value; empty string + nil error is treated as a
// missing row (fail OPEN: default = shim enabled). Any other error is
// likewise treated as "missing" by the caller after a slog.Warn so a
// transient DB hiccup never silently disables a feature meant to be on.
type ACPConfigReader interface {
	GetSystemConfig(ctx context.Context, key string) (string, error)
}

// acpSessionEntry tracks a live ACP session for one goclaw conversation.
type acpSessionEntry struct {
	id       string          // ACP session ID returned by session/new or session/load
	proc     *acp.ACPProcess // process that owns this session (for respawn detection)
	lastUsed time.Time
}

// ACPProvider implements Provider by orchestrating ACP-compatible agent subprocesses.
// One shared Gemini process is used; each goclaw conversation gets its own ACP session.
type ACPProvider struct {
	name         string
	pool         *acp.ProcessPool
	bridge       *acp.ToolBridge
	defaultModel string
	permMode     string
	poolKey      string // key for the shared process in the pool (binary + args)

	// Phase 4: optional shim wiring. When both shim and resolver are set the
	// provider populates per-session SessionEntries (allowlist + cron ctx)
	// before each session/new — without them ACP falls back to Phase 3
	// behavior (empty mcpServers, no shim-side state).
	shim       acp.ShimHandle
	resolver   ACPToolResolver
	ctxReader  ACPContextReader
	// cfgReader reads system_configs for the kill-switch feature flag
	// (acp.shim.enabled). Nil → shim stays on (default-OPEN). Per-session
	// DB read; no cache — config row mutates effective on next session
	// creation without a restart.
	cfgReader  ACPConfigReader
	// tenantID resolution for the resolver. Provided by the caller when
	// constructing the provider — at registration time we know the owning
	// tenant; agent id is resolved per-request from the session ctx.
	tenantID string

	acpSessions sync.Map // goclawSessionKey → *acpSessionEntry
	sessionMu   sync.Map // goclawSessionKey → *sync.Mutex (prevents concurrent session creation)

	done      chan struct{}
	closeOnce sync.Once
}

// ACPOption configures an ACPProvider.
type ACPOption func(*ACPProvider)

// WithACPName overrides the provider name (default: "acp").
func WithACPName(name string) ACPOption {
	return func(p *ACPProvider) {
		if name != "" {
			p.name = name
		}
	}
}

// WithACPModel sets the default model/agent name.
func WithACPModel(model string) ACPOption {
	return func(p *ACPProvider) {
		if model != "" {
			p.defaultModel = model
		}
	}
}

// WithACPPermMode sets the permission mode for the tool bridge.
func WithACPPermMode(mode string) ACPOption {
	return func(p *ACPProvider) {
		if mode != "" {
			p.permMode = mode
		}
	}
}

// WithACPShim wires the in-process MCP shim handle so each ACP session
// gets a per-session HTTP MCP server URL advertised via
// NewSessionRequest.McpServers. Pair with WithACPResolver — without a
// resolver the provider has no way to compute the per-session allowlist.
func WithACPShim(shim acp.ShimHandle) ACPOption {
	return func(p *ACPProvider) {
		p.shim = shim
	}
}

// WithACPResolver wires the tool slice resolver used to compute the
// per-session allowlist before SessionEntry registration.
func WithACPResolver(r ACPToolResolver) ACPOption {
	return func(p *ACPProvider) {
		p.resolver = r
	}
}

// WithACPTenantID provides the tenant id the resolver should scope reads
// to. Used for the agent_grants / acp_tools lookup.
func WithACPTenantID(tid string) ACPOption {
	return func(p *ACPProvider) {
		p.tenantID = tid
	}
}

// WithACPContextReader plugs in a routing-context extractor (cmd layer
// satisfies it by reading tools.Tool*FromCtx). Without this the shim
// SessionEntry is populated with empty cron context — write_file
// deliver=true and similar routing-dependent tools won't be able to
// reach the originating channel.
func WithACPContextReader(r ACPContextReader) ACPOption {
	return func(p *ACPProvider) {
		p.ctxReader = r
	}
}

// WithACPConfigReader plugs in the system_configs reader used to honor the
// acp.shim.enabled kill switch (Task 9). Without it the shim stays
// enabled (default-OPEN); a missing or malformed config row is also
// treated as enabled so a broken/empty seed never silently disables the
// shim.
func WithACPConfigReader(r ACPConfigReader) ACPOption {
	return func(p *ACPProvider) {
		p.cfgReader = r
	}
}

// NewACPProvider creates a provider that orchestrates ACP agents as subprocesses.
func NewACPProvider(binary string, args []string, workDir string, idleTTL time.Duration, denyPatterns []*regexp.Regexp, opts ...ACPOption) *ACPProvider {
	// Pool key identifies the shared process: binary + args combination
	poolKey := binary
	if len(args) > 0 {
		poolKey += "|" + strings.Join(args, " ")
	}

	p := &ACPProvider{
		name:         "acp",
		defaultModel: "claude",
		poolKey:      poolKey,
		done:         make(chan struct{}),
	}
	for _, opt := range opts {
		opt(p)
	}

	var bridgeOpts []acp.ToolBridgeOption
	if len(denyPatterns) > 0 {
		bridgeOpts = append(bridgeOpts, acp.WithDenyPatterns(denyPatterns))
	}
	if p.permMode != "" {
		bridgeOpts = append(bridgeOpts, acp.WithPermMode(p.permMode))
	}
	p.bridge = acp.NewToolBridge(workDir, bridgeOpts...)

	p.pool = acp.NewProcessPool(binary, args, workDir, idleTTL)
	p.pool.SetToolHandler(p.bridge.Handle)
	if p.shim != nil {
		p.pool.SetShim(p.shim)
	}

	go p.sessionReaper()
	return p
}

// sessionReaper removes ACP sessions idle for more than 30 minutes.
// Sends session/cancel to release resources on the agent side before purging locally.
// Also unregisters the shim SessionEntry so the per-session URL 404s on any
// late connect attempt from claude-agent-acp.
func (p *ACPProvider) sessionReaper() {
	const sessionIdleTTL = 30 * time.Minute
	ticker := time.NewTicker(5 * time.Minute)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			p.acpSessions.Range(func(key, value any) bool {
				entry := value.(*acpSessionEntry)
				if time.Since(entry.lastUsed) > sessionIdleTTL {
					slog.Info("acp: expiring idle session", "goclaw_session", key, "sid", entry.id)
					if entry.proc != nil {
						_ = entry.proc.Cancel(entry.id)
					}
					if p.shim != nil {
						p.shim.UnregisterSession(entry.id)
					}
					p.acpSessions.Delete(key)
				}
				return true
			})
		case <-p.done:
			return
		}
	}
}

// shimEnabledKey is the system_configs key for the Task 9 rollback flag.
// Set value='false' to fall back to the legacy ACP path (no MCP tools
// advertised) on the next session creation; no restart required.
const shimEnabledKey = "acp.shim.enabled"

// shimEnabled returns true when the shim should be wired into the next
// ACP session creation. Defaults to true (fail OPEN) when:
//   - no config reader is wired, or
//   - the system_configs row is missing, or
//   - the row value fails to parse as a bool (a warning is logged).
//
// Only an explicit "false" (case-insensitive) routes the session through
// the legacy proc.NewSession / proc.LoadSession path with no shim wiring.
// Read per-session-creation against system_configs — the operator flips
// the row via UPDATE and the next session picks up the change. No cache.
func (p *ACPProvider) shimEnabled(ctx context.Context) bool {
	if p.cfgReader == nil {
		return true
	}
	raw, err := p.cfgReader.GetSystemConfig(ctx, shimEnabledKey)
	if err != nil || raw == "" {
		// Missing row or transient read error → default ON. We deliberately
		// do NOT log on every miss — the seed row may simply not be
		// applied yet on a fresh DB; the shim is the intended default.
		return true
	}
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "true", "1", "yes", "on":
		return true
	case "false", "0", "no", "off":
		return false
	default:
		slog.Warn("acp.shim.flag_parse_failed",
			"key", shimEnabledKey, "value", raw, "default", "enabled")
		return true
	}
}

// resolveSession returns the ACP session ID for a goclaw session key.
// It creates a new session if none exists, or reloads it after a process respawn.
// A per-key mutex prevents concurrent creation races for the same session.
func (p *ACPProvider) resolveSession(ctx context.Context, proc *acp.ACPProcess, goclawKey string) (string, error) {
	actual, _ := p.sessionMu.LoadOrStore(goclawKey, &sync.Mutex{})
	mu := actual.(*sync.Mutex)
	mu.Lock()
	defer mu.Unlock()

	// Task 9: read the kill-switch once per session-creation path. When
	// shim is wired but the flag is OFF, route the session through the
	// legacy proc.NewSession / proc.LoadSession (no MCP tools advertised,
	// no SessionEntry registered). When the flag is ON (default) the
	// normal shim wiring runs.
	useShim := p.shim != nil && p.shimEnabled(ctx)

	if val, ok := p.acpSessions.Load(goclawKey); ok {
		entry := val.(*acpSessionEntry)
		if entry.proc == proc {
			// Same process instance: session is still live, just update last-used
			entry.lastUsed = time.Now()
			return entry.id, nil
		}
		// Process was respawned — try to restore the session
		slog.Info("acp: process respawned, attempting session restore",
			"goclaw_session", goclawKey, "old_sid", entry.id)
		if proc.AgentCaps().LoadSession {
			var sid string
			var err error
			if useShim {
				registerFn := p.makeShimRegisterFn(ctx, goclawKey)
				sid, err = p.pool.LoadSessionWithShim(ctx, proc, entry.id, registerFn)
			} else {
				if p.shim != nil {
					slog.Info("acp.shim.disabled_by_flag",
						"goclaw_session", goclawKey, "old_sid", entry.id, "path", "load")
				}
				sid, err = proc.LoadSession(ctx, entry.id)
			}
			if err == nil {
				p.acpSessions.Store(goclawKey, &acpSessionEntry{id: sid, proc: proc, lastUsed: time.Now()})
				return sid, nil
			}
			slog.Warn("acp: session/load failed, creating new session", "old_sid", entry.id, "error", err)
		}
		// session/load not supported or failed — fall through to create new
	}

	slog.Info("acp: creating new session", "goclaw_session", goclawKey, "pool_key", p.poolKey)
	var sid string
	var err error
	if useShim {
		registerFn := p.makeShimRegisterFn(ctx, goclawKey)
		sid, err = p.pool.NewSessionWithShim(ctx, proc, registerFn)
	} else {
		if p.shim != nil {
			slog.Info("acp.shim.disabled_by_flag",
				"goclaw_session", goclawKey, "path", "new")
		}
		sid, err = proc.NewSession(ctx)
	}
	if err != nil {
		return "", err
	}
	p.acpSessions.Store(goclawKey, &acpSessionEntry{id: sid, proc: proc, lastUsed: time.Now()})
	return sid, nil
}

// makeShimRegisterFn returns the register callback newSessionImpl /
// loadSessionImpl invoke with the reserved session id BEFORE session/new
// returns. When shim or resolver is not wired this returns nil — the
// session-level RPC then runs with empty McpServers (Phase 3 behavior).
//
// The closure resolves the allowlist via the resolver, captures the
// cron/routing context off ctx (channel, chatID, peerKind, sessionKey),
// and stores a SessionEntry in the shim's session registry. The shim
// looks up by sid at HTTP request time so the entry must exist before
// claude-agent-acp opens the streamable-http connection.
func (p *ACPProvider) makeShimRegisterFn(ctx context.Context, goclawKey string) func(sid string) {
	if p.shim == nil || p.resolver == nil {
		return nil
	}
	// Capture routing context off the parent ctx via the cmd-supplied
	// reader (it knows the tools.Tool*FromCtx keys; we can't import tools
	// here without creating providers → tools → store → providers cycle).
	var rc ACPRoutingContext
	if p.ctxReader != nil {
		rc = p.ctxReader.ReadRouting(ctx)
	}

	return func(sid string) {
		// Resolve the per-session allowlist. The resolver applies hard
		// blacklist + BridgeToolNames intersect on top of the grants store,
		// so the returned slice is safe to use as-is.
		allowlist, err := p.resolver.ResolveACPToolNames(ctx, rc.AgentKey, p.tenantID)
		if err != nil {
			slog.Warn("acp.shim.resolve_failed",
				"goclaw_session", goclawKey, "sid", sid, "agent_key", rc.AgentKey, "error", err)
			// Fall through with empty allowlist — claude-agent-acp will see
			// /mcp respond but tools/list returns nothing for the session.
			allowlist = nil
		}

		entry := acp.ShimSessionEntry{
			SID:           sid,
			Allowlist:     allowlist,
			AgentID:       rc.AgentKey,
			ChannelID:     rc.ChannelID,
			DeliverTarget: rc.ChatID,
			PeerKind:      rc.PeerKind,
			SessionKey:    rc.SessionKey,
		}
		p.shim.RegisterSession(entry)
	}
}

func (p *ACPProvider) Name() string         { return p.name }
func (p *ACPProvider) DefaultModel() string { return p.defaultModel }

// Capabilities implements CapabilitiesAware for pipeline code-path selection.
func (p *ACPProvider) Capabilities() ProviderCapabilities {
	return ProviderCapabilities{
		Streaming:        true,
		ToolCalling:      true,
		StreamWithTools:  true,
		Thinking:         true,
		Vision:           false,
		CacheControl:     false,
		MaxContextWindow: 200_000,
		TokenizerID:      "cl100k_base",
	}
}

// Chat sends a prompt and returns the complete response (non-streaming).
func (p *ACPProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	sessionKey := extractStringOpt(req.Options, OptSessionKey)
	if sessionKey == "" {
		sessionKey = fmt.Sprintf("temp-%d", time.Now().UnixNano())
	}

	proc, err := p.pool.GetOrSpawn(ctx, p.poolKey)
	if err != nil {
		return nil, fmt.Errorf("acp: spawn failed: %w", err)
	}

	acpSessionID, err := p.resolveSession(ctx, proc, sessionKey)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(sessionKey, "temp-") {
		defer p.purgeSession(sessionKey)
	}

	content := extractACPContent(req)
	if len(content) == 0 {
		return nil, fmt.Errorf("acp: no user message in request")
	}

	ctx = acp.WithGoclawSession(ctx, sessionKey)

	var buf strings.Builder
	var updateCount int
	promptResp, err := proc.Prompt(ctx, acpSessionID, content, func(update acp.SessionUpdate) {
		if update.Message != nil {
			for _, block := range update.Message.Content {
				if block.Type == "text" {
					buf.WriteString(block.Text)
					updateCount++
				}
			}
		}
	})
	if err != nil {
		slog.Error("acp: chat error", "session", sessionKey, "sid", acpSessionID, "error", err)
		return &ChatResponse{
			Content:      fmt.Sprintf("[ACP Error] %v", err),
			FinishReason: "error",
		}, err
	}

	slog.Info("acp: chat completed", "session", sessionKey, "sid", acpSessionID,
		"stopReason", mapStopReason(promptResp), "updates", updateCount, "contentLen", buf.Len())
	return &ChatResponse{
		Content:      buf.String(),
		FinishReason: mapStopReason(promptResp),
		Usage:        &Usage{},
	}, nil
}

// ChatStream sends a prompt and streams response chunks via onChunk callback.
func (p *ACPProvider) ChatStream(ctx context.Context, req ChatRequest, onChunk func(StreamChunk)) (*ChatResponse, error) {
	sessionKey := extractStringOpt(req.Options, OptSessionKey)
	if sessionKey == "" {
		sessionKey = fmt.Sprintf("temp-%d", time.Now().UnixNano())
	}

	proc, err := p.pool.GetOrSpawn(ctx, p.poolKey)
	if err != nil {
		return nil, fmt.Errorf("acp: spawn failed: %w", err)
	}

	acpSessionID, err := p.resolveSession(ctx, proc, sessionKey)
	if err != nil {
		return nil, err
	}
	if strings.HasPrefix(sessionKey, "temp-") {
		defer p.purgeSession(sessionKey)
	}

	content := extractACPContent(req)
	if len(content) == 0 {
		return nil, fmt.Errorf("acp: no user message in request")
	}

	ctx = acp.WithGoclawSession(ctx, sessionKey)

	// done channel ensures the cancel goroutine exits cleanly on normal completion,
	// preventing it from sending a spurious session/cancel after the prompt finishes.
	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			if errors.Is(ctx.Err(), context.Canceled) {
				_ = proc.Cancel(acpSessionID)
			}
		case <-done:
		}
	}()

	var buf strings.Builder
	var updateCount int
	promptResp, err := proc.Prompt(ctx, acpSessionID, content, func(update acp.SessionUpdate) {
		if update.Message != nil {
			for _, block := range update.Message.Content {
				if block.Type == "text" {
					onChunk(StreamChunk{Content: block.Text})
					buf.WriteString(block.Text)
					updateCount++
				}
			}
		}
		if update.ToolCall != nil && update.ToolCall.Status == "running" {
			slog.Debug("acp: tool call", "name", update.ToolCall.Name)
		}
	})
	if err != nil {
		slog.Error("acp: chat error", "session", sessionKey, "sid", acpSessionID, "error", err)
		return &ChatResponse{
			Content:      fmt.Sprintf("[ACP Error] %v", err),
			FinishReason: "error",
		}, err
	}

	onChunk(StreamChunk{Done: true})
	slog.Info("acp: chat stream completed", "session", sessionKey, "sid", acpSessionID,
		"stopReason", mapStopReason(promptResp), "updates", updateCount, "contentLen", buf.Len())

	return &ChatResponse{
		Content:      buf.String(),
		FinishReason: mapStopReason(promptResp),
		Usage:        &Usage{},
	}, nil
}

// purgeSession removes a session entry from both tracking maps.
// Sends session/cancel to release resources on the agent side before purging locally.
// Used to immediately discard one-shot (temp-) sessions after completion.
// Also unregisters the shim SessionEntry to free per-session state.
func (p *ACPProvider) purgeSession(key string) {
	if val, ok := p.acpSessions.Load(key); ok {
		entry := val.(*acpSessionEntry)
		if entry.proc != nil {
			_ = entry.proc.Cancel(entry.id)
		}
		if p.shim != nil {
			p.shim.UnregisterSession(entry.id)
		}
	}
	p.acpSessions.Delete(key)
	p.sessionMu.Delete(key)
	slog.Info("acp: purged temp session", "goclaw_session", key)
}

// Close shuts down all subprocesses and cleans up terminals.
func (p *ACPProvider) Close() error {
	p.closeOnce.Do(func() {
		close(p.done)
	})
	_ = p.bridge.Close()
	return p.pool.Close()
}

// extractACPContent extracts user message + images from ChatRequest into ACP ContentBlocks.
func extractACPContent(req ChatRequest) []acp.ContentBlock {
	systemPrompt, userMsg, images := extractFromMessages(req.Messages)
	if userMsg == "" {
		return nil
	}

	var blocks []acp.ContentBlock

	// Prepend system prompt to user message (ACP agents have no separate system prompt API)
	text := userMsg
	if systemPrompt != "" {
		text = systemPrompt + "\n\n" + userMsg
	}
	blocks = append(blocks, acp.ContentBlock{Type: "text", Text: text})

	for _, img := range images {
		blocks = append(blocks, acp.ContentBlock{
			Type:     "image",
			Data:     img.Data,
			MimeType: img.MimeType,
		})
	}

	return blocks
}

// mapStopReason converts ACP stopReason to GoClaw finish reason.
func mapStopReason(resp *acp.PromptResponse) string {
	if resp == nil {
		return "stop"
	}
	switch resp.StopReason {
	case "max_tokens", "maxContextLength":
		return "length"
	case "cancelled":
		return "stop"
	default:
		return "stop"
	}
}
