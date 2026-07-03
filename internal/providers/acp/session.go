package acp

import (
	"context"
	crand "crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"log/slog"
	"path/filepath"
	"time"
)

// Initialize sends the ACP initialize request to establish capabilities.
func (p *ACPProcess) Initialize(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req := InitializeRequest{
		ProtocolVersion: 1,
		ClientInfo:      ClientInfo{Name: "GoClaw", Version: "1.0"},
		Capabilities:    ClientCaps{},
	}
	var resp InitializeResponse
	if err := p.conn.Call(ctx, "initialize", req, &resp); err != nil {
		return fmt.Errorf("acp initialize: %w", err)
	}
	p.agentCaps = resp.Capabilities
	slog.Info("acp: initialized", "agent", resp.AgentInfo.Name, "version", resp.AgentInfo.Version, "loadSession", resp.Capabilities.LoadSession)
	return nil
}

// NewSession creates a new ACP session and returns its session ID. This is
// the legacy entry point preserved for callers that do not need the in-
// process MCP shim wired into the session (Phase 3 behavior). New callers
// requiring shim wiring should use ProcessPool.NewSessionWithShim, which
// reaches newSessionImpl with a non-nil shim handle + per-session register.
func (p *ACPProcess) NewSession(ctx context.Context) (string, error) {
	return p.newSessionImpl(ctx, nil, nil)
}

// newSessionImpl is the real implementation behind NewSession and
// ProcessPool.NewSessionWithShim.
//
// When shim is non-nil AND the agent advertises MCPCapabilities.HTTP, the
// shim's per-session URL is appended to NewSessionRequest.McpServers and
// the register callback is invoked with the proposed session id BEFORE
// session/new returns — claude-agent-acp may connect the shim immediately
// on receipt of the response, so the SessionEntry must be in place first
// to avoid a 404 race.
//
// If the agent rejects the proposed sid and mints its own, the actual sid
// is logged via acp.shim.session_remap and Task 8 will follow up with an
// explicit remap path. For now the caller's pre-registration stays put;
// the actual session is still functional because the shim looks up by
// whichever sid the agent reports.
func (p *ACPProcess) newSessionImpl(ctx context.Context, shim ShimHandle, register func(sid string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cwd := p.workDir
	if cwd == "" {
		cwd, _ = filepath.Abs(".")
	}

	mcpServers := []any{}
	useShim := shim != nil && p.agentCaps.MCPCapabilities != nil && p.agentCaps.MCPCapabilities.HTTP
	if shim != nil && !useShim {
		slog.Warn("acp.shim.cap_unsupported",
			"reason", "agent does not advertise MCPCapabilities.HTTP",
			"loadSession", p.agentCaps.LoadSession)
	}

	// Reserve a SID up front so we can register BEFORE session/new returns
	// — see the function doc for the rationale.
	var reservedSID string
	if useShim {
		reservedSID = newSessionID()
		mcpServers = append(mcpServers, McpServerHTTP{
			Type:    "http",
			Name:    "goclaw-shim",
			URL:     shim.SessionURL(reservedSID),
			Headers: []HTTPHeader{},
		})
		if register != nil {
			register(reservedSID)
		}
	}

	req := NewSessionRequest{Cwd: cwd, McpServers: mcpServers}
	var resp NewSessionResponse
	if err := p.conn.Call(ctx, "session/new", req, &resp); err != nil {
		return "", fmt.Errorf("acp session/new: %w", err)
	}
	if useShim && resp.SessionID != reservedSID {
		slog.Info("acp.shim.session_remap",
			"proposed", reservedSID, "actual", resp.SessionID)
	}

	slog.Info("acp: session/new", "sid", resp.SessionID, "cwd", cwd, "mcp_servers", len(mcpServers))
	return resp.SessionID, nil
}

// LoadSession restores a previous ACP session by ID (used after process restart).
// Returns the session ID to use going forward (may equal the requested ID).
// Only call if AgentCaps().LoadSession is true.
//
// Legacy entry point — see ProcessPool.LoadSessionWithShim for the
// shim-aware variant.
func (p *ACPProcess) LoadSession(ctx context.Context, sessionID string) (string, error) {
	return p.loadSessionImpl(ctx, sessionID, nil, nil)
}

// loadSessionImpl mirrors newSessionImpl for the session/load path, so a
// process respawn re-attaches the same sid with shim wiring re-established.
// LoadSessionRequest.McpServers is populated symmetrically (Phase 4
// Revision 1 supplement) — the shim URL we advertise references the
// existing sessionID so the per-session allowlist remains valid after
// reconnect.
func (p *ACPProcess) loadSessionImpl(ctx context.Context, sessionID string, shim ShimHandle, register func(sid string)) (string, error) {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cwd := p.workDir
	if cwd == "" {
		cwd, _ = filepath.Abs(".")
	}

	mcpServers := []any{}
	useShim := shim != nil && p.agentCaps.MCPCapabilities != nil && p.agentCaps.MCPCapabilities.HTTP
	if shim != nil && !useShim {
		slog.Warn("acp.shim.cap_unsupported",
			"reason", "agent does not advertise MCPCapabilities.HTTP (load path)",
			"loadSession", p.agentCaps.LoadSession)
	}
	if useShim {
		// Re-use the existing sid for the URL so the shim's session map key
		// matches what claude-agent-acp will reconnect with.
		mcpServers = append(mcpServers, McpServerHTTP{
			Type:    "http",
			Name:    "goclaw-shim",
			URL:     shim.SessionURL(sessionID),
			Headers: []HTTPHeader{},
		})
		if register != nil {
			register(sessionID)
		}
	}

	req := LoadSessionRequest{SessionID: sessionID, Cwd: cwd, McpServers: mcpServers}
	var resp LoadSessionResponse
	if err := p.conn.Call(ctx, "session/load", req, &resp); err != nil {
		return "", fmt.Errorf("acp session/load: %w", err)
	}
	slog.Info("acp: session/load", "sid", resp.SessionID, "mcp_servers", len(mcpServers))
	return resp.SessionID, nil
}

// newSessionID returns a hex-encoded 128-bit random SID for the shim URL.
// crypto/rand is intentional — the SID is the session-lookup key inside the
// shim, so we want it unguessable from any other process on the host that
// might be peeking at the listen port.
func newSessionID() string {
	var b [16]byte
	_, _ = io.ReadFull(crand.Reader, b[:])
	return hex.EncodeToString(b[:])
}

// Prompt sends user content to sessionID and blocks until the agent completes,
// invoking onUpdate for each session/update notification received.
func (p *ACPProcess) Prompt(ctx context.Context, sessionID string, content []ContentBlock, onUpdate func(SessionUpdate)) (*PromptResponse, error) {
	p.inUse.Add(1)
	defer p.inUse.Add(-1)

	p.mu.Lock()
	p.lastActive = time.Now()
	p.mu.Unlock()

	p.registerUpdateFn(sessionID, onUpdate)
	defer p.unregisterUpdateFn(sessionID)

	goclawSession := goclawSessionFromCtx(ctx)
	slog.Info("acp: session/prompt", "session", goclawSession, "sid", sessionID)
	req := PromptRequest{
		SessionID: sessionID,
		Prompt:    content,
	}

	var resp PromptResponse
	if err := p.conn.Call(ctx, "session/prompt", req, &resp); err != nil {
		return nil, fmt.Errorf("acp session/prompt: %w", err)
	}

	p.mu.Lock()
	p.lastActive = time.Now()
	p.mu.Unlock()

	slog.Info("acp: session/prompt completed", "session", goclawSession, "sid", sessionID, "stopReason", resp.StopReason)
	return &resp, nil
}

// Cancel sends a session/cancel notification for the given session.
func (p *ACPProcess) Cancel(sessionID string) error {
	return p.conn.Notify("session/cancel", CancelNotification{
		SessionID: sessionID,
	})
}

// SetSessionConfigOption sets a per-session config option (e.g. model selection)
// via session/set_config_option. Callers should invoke this once right after
// obtaining a session id (session/new or session/load), not on every prompt.
func (p *ACPProcess) SetSessionConfigOption(ctx context.Context, sessionID, configID, value string) error {
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	req := SetSessionConfigOptionRequest{SessionID: sessionID, ConfigID: configID, Value: value}
	var resp SetSessionConfigOptionResponse
	if err := p.conn.Call(ctx, "session/set_config_option", req, &resp); err != nil {
		return fmt.Errorf("acp session/set_config_option: %w", err)
	}
	slog.Info("acp: session/set_config_option", "sid", sessionID, "configId", configID, "value", value)
	return nil
}
