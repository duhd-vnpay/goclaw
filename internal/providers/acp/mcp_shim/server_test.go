package mcp_shim

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// fakeTool is a minimal tools.Tool used in shim tests. It records the args it
// receives and returns a configurable result so we can verify that the
// session-aware handler routes correctly to the registry.
type fakeTool struct {
	name        string
	description string
	params      map[string]any
	mu          sync.Mutex
	calls       int
	lastArgs    map[string]any
}

func (f *fakeTool) Name() string             { return f.name }
func (f *fakeTool) Description() string      { return f.description }
func (f *fakeTool) Parameters() map[string]any { return f.params }
func (f *fakeTool) Execute(ctx context.Context, args map[string]any) *tools.Result {
	f.mu.Lock()
	f.calls++
	f.lastArgs = args
	f.mu.Unlock()
	return tools.NewResult("ok:" + f.name)
}

func newFakeTool(name string) *fakeTool {
	return &fakeTool{
		name:        name,
		description: "fake tool " + name,
		params: map[string]any{
			"type":       "object",
			"properties": map[string]any{},
		},
	}
}

func newTestRegistry(t *testing.T, names ...string) *tools.Registry {
	t.Helper()
	reg := tools.NewRegistry()
	for _, n := range names {
		reg.Register(newFakeTool(n))
	}
	return reg
}

func newTestServer(t *testing.T, reg *tools.Registry) *Server {
	t.Helper()
	s, err := NewServer(ServerConfig{
		ListenAddr: "127.0.0.1:0",
		Registry:   reg,
		Version:    "test",
	})
	if err != nil {
		t.Fatalf("NewServer: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// rpcRequest builds a JSON-RPC request body.
func rpcRequest(t *testing.T, id int, method string, params any) []byte {
	t.Helper()
	body := map[string]any{
		"jsonrpc": "2.0",
		"id":      id,
		"method":  method,
	}
	if params != nil {
		body["params"] = params
	}
	b, err := json.Marshal(body)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

// postMCP issues a POST against the shim's /mcp endpoint and returns the
// recorder. The body must already be JSON-RPC.
func postMCP(t *testing.T, s *Server, sid string, body []byte) *httptest.ResponseRecorder {
	t.Helper()
	path := "/mcp"
	if sid != "" {
		path += "?session=" + sid
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	s.handleMCP(w, req)
	return w
}

// decodeResult extracts the `result` payload from a JSON-RPC response.
func decodeResult(t *testing.T, body []byte, target any) {
	t.Helper()
	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   *struct {
			Code    int    `json:"code"`
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("decode envelope: %v (body=%s)", err, string(body))
	}
	if env.Error != nil {
		t.Fatalf("unexpected JSON-RPC error: code=%d msg=%s", env.Error.Code, env.Error.Message)
	}
	if len(env.Result) == 0 {
		t.Fatalf("no result in body=%s", string(body))
	}
	if err := json.Unmarshal(env.Result, target); err != nil {
		t.Fatalf("decode result: %v (body=%s)", err, string(env.Result))
	}
}

type toolListEntry struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	InputSchema json.RawMessage `json:"inputSchema"`
}

type listToolsResult struct {
	Tools []toolListEntry `json:"tools"`
}

type callToolResult struct {
	Content []struct {
		Type string `json:"type"`
		Text string `json:"text"`
	} `json:"content"`
	IsError bool `json:"isError"`
}

func TestServer_AllowlistFiltersToolList(t *testing.T) {
	reg := newTestRegistry(t, "write_file", "exec", "read_file")
	s := newTestServer(t, reg)

	s.RegisterSession(SessionEntry{
		SID:       "sid-1",
		Allowlist: []string{"write_file"},
	})

	w := postMCP(t, s, "sid-1", rpcRequest(t, 1, "tools/list", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	var lr listToolsResult
	decodeResult(t, w.Body.Bytes(), &lr)

	if len(lr.Tools) != 1 || lr.Tools[0].Name != "write_file" {
		t.Errorf("expected only write_file, got %+v", lr.Tools)
	}
}

func TestServer_AllowlistRejectsUnknownTool(t *testing.T) {
	reg := newTestRegistry(t, "write_file", "exec")
	s := newTestServer(t, reg)

	s.RegisterSession(SessionEntry{
		SID:       "sid-2",
		Allowlist: []string{"write_file"},
	})

	body := rpcRequest(t, 2, "tools/call", map[string]any{
		"name":      "exec",
		"arguments": map[string]any{"cmd": "ls"},
	})
	w := postMCP(t, s, "sid-2", body)
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	var cr callToolResult
	decodeResult(t, w.Body.Bytes(), &cr)

	if !cr.IsError {
		t.Fatalf("expected IsError=true, got: %+v", cr)
	}
	if len(cr.Content) == 0 || !strings.Contains(cr.Content[0].Text, "not granted for this ACP session") {
		t.Errorf("expected reject message, got: %+v", cr.Content)
	}
}

func TestServer_UnknownSessionReturns404(t *testing.T) {
	reg := newTestRegistry(t, "write_file")
	s := newTestServer(t, reg)

	w := postMCP(t, s, "does-not-exist", rpcRequest(t, 1, "tools/list", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404 (body=%s)", w.Code, w.Body.String())
	}
}

func TestServer_TwoSessionsNoCrosstalk(t *testing.T) {
	reg := newTestRegistry(t, "write_file", "exec", "read_file")
	s := newTestServer(t, reg)

	s.RegisterSession(SessionEntry{SID: "a", Allowlist: []string{"write_file"}})
	s.RegisterSession(SessionEntry{SID: "b", Allowlist: []string{"exec"}})

	for _, c := range []struct {
		sid  string
		want string
	}{
		{"a", "write_file"},
		{"b", "exec"},
	} {
		w := postMCP(t, s, c.sid, rpcRequest(t, 1, "tools/list", nil))
		if w.Code != http.StatusOK {
			t.Fatalf("session %s: status=%d body=%s", c.sid, w.Code, w.Body.String())
		}
		var lr listToolsResult
		decodeResult(t, w.Body.Bytes(), &lr)
		if len(lr.Tools) != 1 || lr.Tools[0].Name != c.want {
			t.Errorf("session %s: got %+v, want only %s", c.sid, lr.Tools, c.want)
		}
	}
}

// capturingHandler is a slog.Handler that records emitted records so tests can
// assert on structured-log keys without spawning a goroutine.
type capturingHandler struct {
	mu      sync.Mutex
	records []slog.Record
}

func (h *capturingHandler) Enabled(_ context.Context, _ slog.Level) bool { return true }
func (h *capturingHandler) Handle(_ context.Context, r slog.Record) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.records = append(h.records, r)
	return nil
}
func (h *capturingHandler) WithAttrs(_ []slog.Attr) slog.Handler { return h }
func (h *capturingHandler) WithGroup(_ string) slog.Handler      { return h }

func (h *capturingHandler) hasMessage(msg string) bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	for _, r := range h.records {
		if r.Message == msg {
			return true
		}
	}
	return false
}

// TestServer_LazySyncToolsRegisteredAfterStartup verifies Bug E fix: tools
// registered into the GoClaw registry AFTER NewServer ran (e.g. DB-driven MCP
// bridge tools like mcp_ops__*) still appear in tools/list when the shim
// receives a request. Without lazy-sync the tool would be silently dropped
// from the catalogue even though the per-session allowlist marks it allowed.
func TestServer_LazySyncToolsRegisteredAfterStartup(t *testing.T) {
	reg := newTestRegistry(t, "write_file")
	s := newTestServer(t, reg)

	// Simulate a DB-MCP bridge tool registering AFTER shim startup.
	reg.Register(newFakeTool("mcp_ops__litellm_psql_query"))

	s.RegisterSession(SessionEntry{
		SID:       "sid-lazy",
		Allowlist: []string{"mcp_ops__litellm_psql_query"},
	})

	w := postMCP(t, s, "sid-lazy", rpcRequest(t, 1, "tools/list", nil))
	if w.Code != http.StatusOK {
		t.Fatalf("status: got %d, want 200 (body=%s)", w.Code, w.Body.String())
	}

	var lr listToolsResult
	decodeResult(t, w.Body.Bytes(), &lr)

	if len(lr.Tools) != 1 || lr.Tools[0].Name != "mcp_ops__litellm_psql_query" {
		t.Errorf("expected lazy-synced mcp_ops__litellm_psql_query in tools/list, got %+v", lr.Tools)
	}
}

func TestServer_RegisterUnregisterSession(t *testing.T) {
	// Swap the default slog logger for the duration of this test.
	cap := &capturingHandler{}
	prev := slog.Default()
	slog.SetDefault(slog.New(cap))
	t.Cleanup(func() { slog.SetDefault(prev) })

	reg := newTestRegistry(t, "write_file")
	s := newTestServer(t, reg)

	s.RegisterSession(SessionEntry{SID: "sid-reg", Allowlist: []string{"write_file"}})
	if !cap.hasMessage("acp.shim.session_registered") {
		t.Errorf("expected slog message acp.shim.session_registered, got %+v",
			func() []string {
				cap.mu.Lock()
				defer cap.mu.Unlock()
				out := make([]string, len(cap.records))
				for i, r := range cap.records {
					out[i] = r.Message
				}
				return out
			}())
	}

	s.UnregisterSession("sid-reg")
	if !cap.hasMessage("acp.shim.session_unregistered") {
		t.Errorf("expected slog message acp.shim.session_unregistered")
	}

	// Verify session is gone — second tools/list should now 404.
	w := postMCP(t, s, "sid-reg", rpcRequest(t, 1, "tools/list", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("after unregister: status=%d, want 404", w.Code)
	}
}
