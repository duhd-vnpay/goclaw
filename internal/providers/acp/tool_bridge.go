package acp

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
)

// ToolBridge handles agent→client requests (fs, terminal, permission).
// It enforces workspace sandboxing and shell deny patterns.
type ToolBridge struct {
	workspace      string
	terminals      sync.Map // string → *Terminal
	denyPatterns   []*regexp.Regexp
	permMode       string // "approve-all" (default), "approve-reads", "deny-all"
	nextTermID     atomic.Int64
	maxOutputBytes int
}

// ToolBridgeOption configures a ToolBridge.
type ToolBridgeOption func(*ToolBridge)

// WithDenyPatterns sets shell deny patterns.
func WithDenyPatterns(patterns []*regexp.Regexp) ToolBridgeOption {
	return func(tb *ToolBridge) { tb.denyPatterns = patterns }
}

// WithPermMode sets the permission handling mode.
func WithPermMode(mode string) ToolBridgeOption {
	return func(tb *ToolBridge) {
		if mode != "" {
			tb.permMode = mode
		}
	}
}

// NewToolBridge creates a tool bridge sandboxed to the given workspace.
func NewToolBridge(workspace string, opts ...ToolBridgeOption) *ToolBridge {
	tb := &ToolBridge{
		workspace:      workspace,
		permMode:       "approve-all",
		maxOutputBytes: 10 * 1024 * 1024, // 10MB
	}
	for _, opt := range opts {
		opt(tb)
	}
	return tb
}

// Handle dispatches agent→client requests by method name.
// Implements the RequestHandler signature for Conn.
func (tb *ToolBridge) Handle(ctx context.Context, method string, params json.RawMessage) (any, error) {
	switch method {
	case "fs/readTextFile":
		if tb.permMode == "deny-all" {
			return nil, fmt.Errorf("read denied by permission mode: %s", tb.permMode)
		}
		var req ReadTextFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.readFile(req)
	case "fs/writeTextFile":
		if tb.permMode == "deny-all" || tb.permMode == "approve-reads" {
			return nil, fmt.Errorf("write denied by permission mode: %s", tb.permMode)
		}
		var req WriteTextFileRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.writeFile(req)
	case "terminal/create":
		if tb.permMode == "deny-all" || tb.permMode == "approve-reads" {
			return nil, fmt.Errorf("terminal denied by permission mode: %s", tb.permMode)
		}
		var req CreateTerminalRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.createTerminal(req)
	case "terminal/output":
		var req TerminalOutputRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.terminalOutput(req)
	case "terminal/release":
		var req ReleaseTerminalRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.releaseTerminal(req)
	case "terminal/waitForExit":
		var req WaitForTerminalExitRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.waitForExit(ctx, req)
	case "terminal/kill":
		if tb.permMode == "deny-all" {
			return nil, fmt.Errorf("terminal kill denied by permission mode: %s", tb.permMode)
		}
		var req KillTerminalRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.killTerminal(req)
	case "session/request_permission":
		var req RequestPermissionRequest
		if err := json.Unmarshal(params, &req); err != nil {
			return nil, fmt.Errorf("invalid params: %w", err)
		}
		return tb.handlePermission(req)
	default:
		return nil, fmt.Errorf("unknown method: %s", method)
	}
}

// readFile reads a file validated against the workspace boundary.
func (tb *ToolBridge) readFile(req ReadTextFileRequest) (*ReadTextFileResponse, error) {
	resolved, err := tb.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(resolved)
	if err != nil {
		return nil, fmt.Errorf("read failed: %w", err)
	}
	return &ReadTextFileResponse{Content: string(data)}, nil
}

// writeFile writes a file validated against the workspace boundary.
func (tb *ToolBridge) writeFile(req WriteTextFileRequest) (*WriteTextFileResponse, error) {
	resolved, err := tb.resolvePath(req.Path)
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(resolved), 0755); err != nil {
		return nil, fmt.Errorf("mkdir failed: %w", err)
	}
	if err := os.WriteFile(resolved, []byte(req.Content), 0644); err != nil {
		return nil, fmt.Errorf("write failed: %w", err)
	}
	return &WriteTextFileResponse{}, nil
}

// handlePermission picks one option from req.Options based on the configured
// permission mode and returns it as a "selected" outcome. Per schema, outcome
// must be either "cancelled" (never used here — no user to cancel) or
// "selected" with a non-empty optionId chosen from the provided options[].
//
// Selection strategy:
//   - "deny-all": prefer reject_always, fall back to reject_once, then first option
//   - "approve-reads": if toolCall.kind ∈ {read, search, fetch}, behave like approve-all;
//     otherwise behave like deny-all
//   - "approve-all" (default): prefer allow_always, fall back to allow_once, then first option
//
// If req.Options is empty (should not happen per schema) we still return
// "selected" with empty optionId rather than error — the wrapper will reject
// it client-side, but that's clearer than crashing the dispatch.
func (tb *ToolBridge) handlePermission(req RequestPermissionRequest) (*RequestPermissionResponse, error) {
	approve := func() string {
		if id := pickOptionByKind(req.Options, "allow_always"); id != "" {
			return id
		}
		if id := pickOptionByKind(req.Options, "allow_once"); id != "" {
			return id
		}
		if len(req.Options) > 0 {
			return req.Options[0].OptionID
		}
		return ""
	}
	deny := func() string {
		if id := pickOptionByKind(req.Options, "reject_always"); id != "" {
			return id
		}
		if id := pickOptionByKind(req.Options, "reject_once"); id != "" {
			return id
		}
		if len(req.Options) > 0 {
			return req.Options[0].OptionID
		}
		return ""
	}

	switch tb.permMode {
	case "deny-all":
		return &RequestPermissionResponse{Outcome: "selected", OptionID: deny()}, nil
	case "approve-reads":
		kind := strings.ToLower(req.ToolCall.Kind)
		if kind == "read" || kind == "search" || kind == "fetch" {
			return &RequestPermissionResponse{Outcome: "selected", OptionID: approve()}, nil
		}
		return &RequestPermissionResponse{Outcome: "selected", OptionID: deny()}, nil
	default: // "approve-all" or unknown → approve
		return &RequestPermissionResponse{Outcome: "selected", OptionID: approve()}, nil
	}
}

func pickOptionByKind(opts []PermissionOption, kind string) string {
	for _, o := range opts {
		if o.Kind == kind {
			return o.OptionID
		}
	}
	return ""
}

// resolvePath validates that a path stays within the workspace boundary.
func (tb *ToolBridge) resolvePath(path string) (string, error) {
	cleaned := filepath.Clean(path)
	if !filepath.IsAbs(cleaned) {
		cleaned = filepath.Join(tb.workspace, cleaned)
	}
	// Resolve symlinks for the target (may not exist yet for writes)
	real, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		real = cleaned // file may not exist yet — validate parent
	}
	wsReal, _ := filepath.EvalSymlinks(tb.workspace)
	if wsReal == "" {
		wsReal = tb.workspace
	}
	if real != wsReal && !strings.HasPrefix(real, wsReal+string(filepath.Separator)) {
		slog.Warn("security.acp_path_escape", "path", path, "resolved", real, "workspace", wsReal)
		return "", fmt.Errorf("access denied: path outside workspace")
	}
	return real, nil
}

// Close kills all active terminals.
func (tb *ToolBridge) Close() error {
	tb.terminals.Range(func(key, value any) bool {
		t := value.(*Terminal)
		t.cancel()
		tb.terminals.Delete(key)
		return true
	})
	return nil
}
