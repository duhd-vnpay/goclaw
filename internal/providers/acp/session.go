package acp

import (
	"context"
	"fmt"
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
// ProcessPool.NewSessionWithShim. Both shim and register are accepted but
// not yet consumed — Task 7 in the Phase 4 ACP↔Tool Registry Bridge plan
// will populate NewSessionRequest.McpServers from shim.SessionURL and
// invoke register(sid) so the caller can attach the SessionEntry under
// the resolved session ID. For now the body is identical to the
// pre-refactor NewSession so all existing callers preserve their behavior.
func (p *ACPProcess) newSessionImpl(ctx context.Context, shim ShimHandle, register func(sid string)) (string, error) {
	_ = shim     // reserved for Task 7
	_ = register // reserved for Task 7

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cwd := p.workDir
	if cwd == "" {
		cwd, _ = filepath.Abs(".")
	}

	req := NewSessionRequest{
		Cwd:        cwd,
		McpServers: []string{},
	}
	var resp NewSessionResponse
	if err := p.conn.Call(ctx, "session/new", req, &resp); err != nil {
		return "", fmt.Errorf("acp session/new: %w", err)
	}
	slog.Info("acp: session/new", "sid", resp.SessionID, "cwd", cwd)
	return resp.SessionID, nil
}

// LoadSession restores a previous ACP session by ID (used after process restart).
// Returns the session ID to use going forward (may equal the requested ID).
// Only call if AgentCaps().LoadSession is true.
//
// Legacy entry point — see ProcessPool.LoadSessionWithShim for the
// shim-aware variant (Task 7 will populate LoadSessionRequest.McpServers
// to mirror NewSession parity per the Phase 4 Revision 1 supplement).
func (p *ACPProcess) LoadSession(ctx context.Context, sessionID string) (string, error) {
	return p.loadSessionImpl(ctx, sessionID, nil, nil)
}

// loadSessionImpl is the real implementation behind LoadSession and
// ProcessPool.LoadSessionWithShim. Symmetry with newSessionImpl keeps
// Task 7's populate step a single pattern applied in two places. shim and
// register are reserved for Task 7 and currently unused.
func (p *ACPProcess) loadSessionImpl(ctx context.Context, sessionID string, shim ShimHandle, register func(sid string)) (string, error) {
	_ = shim     // reserved for Task 7
	_ = register // reserved for Task 7

	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()

	cwd := p.workDir
	if cwd == "" {
		cwd, _ = filepath.Abs(".")
	}

	req := LoadSessionRequest{SessionID: sessionID, Cwd: cwd}
	var resp LoadSessionResponse
	if err := p.conn.Call(ctx, "session/load", req, &resp); err != nil {
		return "", fmt.Errorf("acp session/load: %w", err)
	}
	slog.Info("acp: session/load", "sid", resp.SessionID)
	return resp.SessionID, nil
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
