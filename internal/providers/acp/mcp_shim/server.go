// Package mcp_shim wraps the GoClaw tool registry in a session-multiplexed
// HTTP MCP server suitable for advertising to ACP sub-sessions via
// NewSessionRequest.McpServers.
//
// Architecture (Phase 4, Revision 1):
//
//   - ONE process-wide mcpserver.NewMCPServer is constructed at startup with
//     every candidate tool from the registry registered. Per-session filtering
//     happens at HTTP request time by attaching an allowlist to the request
//     ctx; the session-aware ToolHandlerFunc consults this allowlist before
//     dispatching to tools.Registry.ExecuteWithContext.
//
//   - The HTTP layer is a thin /mcp endpoint that validates the ?session=<sid>
//     URL param against a sync.Map registry, injects per-session ctx values,
//     and forwards the request to the underlying mark3labs StreamableHTTPServer.
//
// The shim deliberately reuses mcp.NewBridgeServer's helpers
// (ConvertToMCPTool, ForwardMediaToOutbound) to keep media-forwarding logic in
// one place.
package mcp_shim

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"sync"
	"time"

	mcpserver "github.com/mark3labs/mcp-go/server"

	"github.com/nextlevelbuilder/goclaw/internal/bus"
	"github.com/nextlevelbuilder/goclaw/internal/mcp"
	"github.com/nextlevelbuilder/goclaw/internal/tools"
)

// ServerConfig configures the shim listener.
type ServerConfig struct {
	// ListenAddr is the bind address. Use "127.0.0.1:0" to allocate an
	// ephemeral port — the resolved port is exposed via Server.URL().
	ListenAddr string

	// Registry is the (already populated) GoClaw tool registry. Every tool
	// currently registered is wired into the underlying mcp-go server; the
	// per-session allowlist narrows what each ACP session can actually see.
	Registry *tools.Registry

	// MsgBus is the outbound message bus used to forward tool-produced media
	// (e.g. write_file deliver=true → channel attachment). May be nil during
	// tests.
	MsgBus *bus.MessageBus

	// Version is the goclaw version string surfaced via MCP serverInfo.
	Version string
}

// MCPSessionBuilder builds a per-session *tools.Registry containing every tool
// the agent is granted access to (builtins + DB-driven MCP bridge tools). It
// captures the shared mcpbridge.Pool + store.MCPServerStore from cmd wiring so
// the shim can construct a per-session catalog without importing those types
// itself (keeps mcp_shim → store cycle off the import graph). Returns nil
// registry when no per-agent catalog is needed (e.g. agent has no MCP grants);
// the shim then falls back to the process-wide global catalog. Errors are
// logged at RegisterSession but do not abort session creation — the session
// still works for builtin tools that survived the lazy-sync path.
//
// Fork.15f-acp (2026-06-11) — fixes Bug E2: previously the shim only saw the
// process-wide tools.Registry, but mcp_ops__* tools live in per-agent CLONES
// (internal/agent/resolver.go:312-326) that the shim never references. The
// per-session builder closes that gap by mirroring resolver.go's clone+Manager
// pattern for the duration of the ACP session.
type MCPSessionBuilder func(ctx context.Context, agentKey string) (*tools.Registry, error)

// Server is the in-process HTTP MCP shim. One per goclaw process; many ACP
// sessions multiplex over it via ?session=<sid>.
type Server struct {
	listener  net.Listener
	srv       *http.Server
	mcp       *mcpserver.StreamableHTTPServer
	sessions  sync.Map // string → *SessionEntry
	closeOnce sync.Once

	// inner + registry + msgBus retained so handleMCP can lazy-sync tools
	// that registered AFTER NewServer ran (e.g. DB-driven MCP bridge tools
	// connect on first agent grant resolution, well after shim startup).
	inner    *mcpserver.MCPServer
	registry *tools.Registry
	msgBus   *bus.MessageBus
	version  string

	regMu           sync.Mutex
	registeredTools map[string]bool

	// Per-session MCP catalogs (Bug E2 fix). When mcpSessionBuilder is wired,
	// RegisterSession builds a per-session mcp-go MCPServer containing the
	// agent's full granted catalog (builtins + DB-driven MCP bridge tools)
	// and stores it here keyed by SID. handleMCP routes incoming /mcp
	// requests to the per-session server when present, else falls back to
	// the global s.mcp. Cleared in UnregisterSession.
	mcpBuilderMu      sync.RWMutex
	mcpSessionBuilder MCPSessionBuilder
	sessionServers    sync.Map // string → *mcpserver.StreamableHTTPServer
	sessionRegs       sync.Map // string → *tools.Registry (lifetime anchor for sessionServers)
}

// allowlistCtxKey is the context key used to thread the per-session allowlist
// from handleMCP down into the session-aware ToolHandlerFunc. Kept unexported
// so callers cannot tamper with the value.
type allowlistCtxKey struct{}

// sessionCtxKey threads the *SessionEntry (containing the per-session
// rateBucket and SID) from handleMCP into the session-aware tool handler.
// Kept unexported for the same reason as allowlistCtxKey.
type sessionCtxKey struct{}

// Per-session rate-limit defaults: 100 tool calls per 5 minutes. Constants
// rather than ServerConfig knobs because the limit is a security defense
// rather than a tunable.
const (
	defaultRateCapacity    = 100
	defaultRateRefillEvery = 5 * time.Minute
)

// NewServer binds the listener and starts the HTTP server. The underlying
// mark3labs MCP server is constructed once with every tool the registry
// currently knows about; per-session filtering is done in the handler.
//
// Returns an error if the bind fails — caller must treat as fatal.
func NewServer(cfg ServerConfig) (*Server, error) {
	if cfg.Registry == nil {
		return nil, errors.New("mcp_shim: ServerConfig.Registry is required")
	}

	ln, err := net.Listen("tcp", cfg.ListenAddr)
	if err != nil {
		slog.Error("acp.shim.listen_failed", "addr", cfg.ListenAddr, "error", err)
		return nil, fmt.Errorf("mcp_shim listen: %w", err)
	}

	s := &Server{
		listener:        ln,
		registry:        cfg.Registry,
		msgBus:          cfg.MsgBus,
		version:         cfg.Version,
		registeredTools: make(map[string]bool),
	}

	// Build the underlying MCP server with every potentially-allowed tool.
	// Per-session filtering happens in the handler via ctx allowlist; the
	// registration here is intentionally permissive — the security gate is
	// the allowlist check inside makeSessionAwareHandler.
	s.inner = mcpserver.NewMCPServer("goclaw-acp-shim", cfg.Version,
		mcpserver.WithToolCapabilities(false),
	)

	registered := s.syncTools()
	slog.Info("acp.shim.tools_registered", "count", registered)

	s.mcp = mcpserver.NewStreamableHTTPServer(s.inner,
		mcpserver.WithStateLess(true),
	)

	mux := http.NewServeMux()
	mux.HandleFunc("/mcp", s.handleMCP)
	s.srv = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		if err := s.srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			slog.Error("acp.shim.serve_exited", "error", err)
		}
	}()
	slog.Info("acp.shim.listen_started", "addr", ln.Addr().String())

	return s, nil
}

// URL returns the base http URL clients should connect to. The session id is
// appended as a query parameter by SessionURL.
func (s *Server) URL() string {
	if s.listener == nil {
		return ""
	}
	return "http://" + s.listener.Addr().String() + "/mcp"
}

// SessionURL is the per-session URL passed to claude-agent-acp via
// NewSessionRequest.McpServers.
func (s *Server) SessionURL(sid string) string {
	return s.URL() + "?session=" + sid
}

// SetMCPSessionBuilder wires the per-session MCP catalog builder. Called once
// at startup by cmd wiring AFTER mcpbridge.Pool + store.MCPServerStore exist
// (those are created inside wireExtras, which runs after setupACPShim). Safe
// to call before any session is registered. Nil clears the builder.
func (s *Server) SetMCPSessionBuilder(fn MCPSessionBuilder) {
	s.mcpBuilderMu.Lock()
	s.mcpSessionBuilder = fn
	s.mcpBuilderMu.Unlock()
	slog.Info("acp.shim.session_builder_wired", "set", fn != nil)
}

// getSessionBuilder returns the current per-session builder under a read lock.
// Returns nil when no builder is wired — RegisterSession then degrades to the
// pre-15f behavior (global catalog only).
func (s *Server) getSessionBuilder() MCPSessionBuilder {
	s.mcpBuilderMu.RLock()
	defer s.mcpBuilderMu.RUnlock()
	return s.mcpSessionBuilder
}

// RegisterSession adds a session to the registry. Idempotent; later
// registrations overwrite earlier ones (callers must not reuse sid for
// different agents).
//
// When MCPSessionBuilder is wired and the session carries an AgentID, this
// builds a per-session mcp-go MCPServer containing the agent's full granted
// catalog (builtins + DB-driven mcp_<server>__<tool> bridge tools). handleMCP
// routes incoming /mcp requests to the per-session server when present.
func (s *Server) RegisterSession(e SessionEntry) {
	if e.CreatedAt.IsZero() {
		e.CreatedAt = time.Now()
	}
	if e.rateBucket == nil {
		e.rateBucket = newRateBucket(defaultRateCapacity, defaultRateRefillEvery)
	}
	s.sessions.Store(e.SID, &e)

	// Bug E2 fix: build per-session MCP server with full per-agent catalog.
	// Done after sessions.Store so handleMCP can find the SessionEntry even
	// if a request races the catalog build (it falls back to global s.mcp).
	perSessionTools := s.buildSessionServer(e)

	slog.Info("acp.shim.session_registered",
		"sid", e.SID,
		"agent", e.Cron.AgentID,
		"run", e.Cron.RunID,
		"channel", e.Cron.ChannelID,
		"allowlist", len(e.Allowlist),
		"per_session_tools", perSessionTools,
	)
}

// buildSessionServer constructs the per-session MCP server when the builder is
// wired and the session carries an AgentID. Returns the number of tools wired
// into the per-session server (0 if no per-session catalog was built). The
// catalog is stored in s.sessionServers + s.sessionRegs keyed by SID.
//
// Errors are logged but never abort session registration — handleMCP then
// transparently uses the global s.mcp catalog, which contains at least the
// process-wide builtin set.
func (s *Server) buildSessionServer(e SessionEntry) int {
	builder := s.getSessionBuilder()
	if builder == nil || e.Cron.AgentID == "" {
		return 0
	}
	if len(e.Allowlist) == 0 {
		// No granted tools — even the per-session server would be empty.
		// Skip the build to avoid burning a Pool connection + tools/list RTT
		// upstream when there's nothing to advertise.
		return 0
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	sessReg, err := builder(ctx, e.Cron.AgentID)
	if err != nil {
		slog.Warn("acp.shim.session_builder_failed",
			"sid", e.SID, "agent", e.Cron.AgentID, "error", err)
		if sessReg == nil {
			return 0
		}
		// Partial result: builder returned a registry but reported errors
		// (e.g. one MCP server failed to connect). We still use what we got.
	}
	if sessReg == nil {
		return 0
	}

	// Build a fresh mcp-go MCPServer wrapping exactly the allowed tools.
	// makeSessionAwareHandler binds to sessReg so MCP bridge tools dispatch
	// via the per-session manager's connections (and so the audit log /
	// rate-limit context still flows through ctx values attached upstream
	// in handleMCP).
	inner := mcpserver.NewMCPServer("goclaw-acp-shim", s.version,
		mcpserver.WithToolCapabilities(false),
	)
	allow := e.AllowlistSet()
	var registered int
	var missing []string
	for _, name := range e.Allowlist {
		if !allow[name] {
			continue
		}
		t, ok := sessReg.Get(name)
		if !ok {
			// Fall back to global registry — covers builtins that the
			// builder didn't clone (shouldn't happen with the cmd wiring
			// using Clone()+LoadForAgent, but defensive in case the
			// builder returns a fresh empty registry).
			t, ok = s.registry.Get(name)
			if !ok {
				missing = append(missing, name)
				continue
			}
		}
		mcpTool := mcp.ConvertToMCPTool(t)
		inner.AddTool(mcpTool, s.makeSessionAwareHandler(sessReg, s.msgBus, name))
		registered++
	}

	if len(missing) > 0 {
		slog.Warn("acp.shim.session_tools_missing",
			"sid", e.SID, "agent", e.Cron.AgentID,
			"missing", missing)
	}

	httpSrv := mcpserver.NewStreamableHTTPServer(inner,
		mcpserver.WithStateLess(true),
	)
	s.sessionServers.Store(e.SID, httpSrv)
	s.sessionRegs.Store(e.SID, sessReg)
	return registered
}

// UnregisterSession removes a session from the registry. Subsequent requests
// to the session URL will receive HTTP 404.
func (s *Server) UnregisterSession(sid string) {
	s.sessions.Delete(sid)
	s.sessionServers.Delete(sid)
	s.sessionRegs.Delete(sid)
	slog.Info("acp.shim.session_unregistered", "sid", sid)
}

// Close shuts the listener; in-flight requests get a 503.
func (s *Server) Close() error {
	var err error
	s.closeOnce.Do(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if s.srv != nil {
			err = s.srv.Shutdown(ctx)
		}
	})
	return err
}

// handleMCP is the /mcp HTTP entry point. It validates the session ID,
// attaches per-session ctx values, and forwards to the underlying mcp-go
// StreamableHTTPServer (which handles JSON-RPC envelope, method routing,
// and the tools/call wire format).
//
// For tools/list the response is intercepted and filtered against
// SessionEntry.Allowlist before being written to the wire — the mcp-go server
// owns ONE global tool catalogue, so per-session list filtering is enforced
// here rather than by reconstructing the catalogue per request.
func (s *Server) handleMCP(w http.ResponseWriter, r *http.Request) {
	sid := r.URL.Query().Get("session")
	v, ok := s.sessions.Load(sid)
	if !ok {
		slog.Warn("acp.shim.unknown_session", "sid", sid)
		http.Error(w, "unknown session", http.StatusNotFound)
		return
	}
	sess := v.(*SessionEntry)

	// Lazy-sync newly-registered tools (e.g. DB-driven MCP bridge tools that
	// connected AFTER NewServer ran). Cheap when nothing changed — just a
	// map lookup per registry entry under regMu.
	if added := s.syncTools(); added > 0 {
		slog.Info("acp.shim.tools_synced", "added", added)
	}

	// Attach per-session ctx so the session-aware tool handler can:
	//   1. Consult the allowlist before dispatch.
	//   2. Read CronContext to inject routing (channel, chatID, peerKind,
	//      sessionKey) for builtin tools like write_file deliver=true.
	ctx := r.Context()
	ctx = context.WithValue(ctx, allowlistCtxKey{}, sess.AllowlistSet())
	ctx = context.WithValue(ctx, sessionCtxKey{}, sess)
	if sess.Cron.ChannelID != "" {
		ctx = tools.WithToolChannel(ctx, sess.Cron.ChannelID)
	}
	if sess.Cron.DeliverTarget != "" {
		ctx = tools.WithToolChatID(ctx, sess.Cron.DeliverTarget)
	}
	if sess.Cron.PeerKind != "" {
		ctx = tools.WithToolPeerKind(ctx, sess.Cron.PeerKind)
	}
	if sess.Cron.SessionKey != "" {
		ctx = tools.WithToolSessionKey(ctx, sess.Cron.SessionKey)
	}

	// Detect tools/list so we can filter the response. We read the body once
	// then re-attach it to the request before forwarding.
	method, body, err := peekMethod(r)
	if err != nil {
		slog.Warn("acp.shim.peek_method_failed", "sid", sid, "error", err)
		http.Error(w, "invalid request body", http.StatusBadRequest)
		return
	}
	r.Body = io.NopCloser(bytes.NewReader(body))
	r.ContentLength = int64(len(body))

	r = r.WithContext(ctx)

	// fork.15e-acp: Bug E2 debug — log every request reaching the shim so we
	// can see whether Claude Code SDK is even calling tools/list. Log full body
	// truncated to 512 bytes for tools/call tool-name inspection. Remove after
	// root cause is confirmed.
	bodyPreview := string(body)
	if len(bodyPreview) > 512 {
		bodyPreview = bodyPreview[:512] + "...(truncated)"
	}
	slog.Info("acp.shim.request",
		"sid", sid,
		"http_method", r.Method,
		"jsonrpc_method", method,
		"content_type", r.Header.Get("Content-Type"),
		"accept", r.Header.Get("Accept"),
		"len", len(body),
		"body_preview", bodyPreview,
	)

	// Bug E2 fix: prefer per-session MCP server (built at RegisterSession with
	// the agent's full DB-driven catalog) over the process-wide s.mcp. The
	// per-session server is constructed only when SetMCPSessionBuilder is wired
	// and the session carries an AgentID — sessions registered before late-bind
	// (or without an agent) still hit the global path below.
	target := s.mcp
	usingPerSession := false
	if v, ok := s.sessionServers.Load(sid); ok {
		if perSess, ok := v.(*mcpserver.StreamableHTTPServer); ok && perSess != nil {
			target = perSess
			usingPerSession = true
		}
	}

	if method == "tools/list" {
		rec := &capturingWriter{header: http.Header{}}
		target.ServeHTTP(rec, r)
		// Per-session server is already built with only allowed tools, so the
		// allowlist filter is redundant — but cheap, and double-applying keeps
		// the security gate uniform whether we're hitting the per-session or
		// global catalog.
		s.writeFilteredToolsList(w, rec, sess.AllowlistSet())
		if usingPerSession {
			slog.Debug("acp.shim.tools_list_per_session", "sid", sid)
		}
		return
	}

	target.ServeHTTP(w, r)
}

// peekMethod parses just enough of the JSON-RPC body to extract the method
// name, returning the body bytes so the caller can re-attach them to the
// request. POST bodies for this transport are small (kilobytes), so reading
// in memory is acceptable.
//
// Limitation: assumes a single JSON-RPC request, not a batch (array of
// requests). claude-agent-acp uses single-call requests so this is fine
// today; revisit if MCP clients in this codebase start batching.
func peekMethod(r *http.Request) (string, []byte, error) {
	if r.Body == nil {
		return "", nil, nil
	}
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return "", nil, err
	}
	if len(body) == 0 {
		return "", body, nil
	}
	var probe struct {
		Method string `json:"method"`
	}
	if err := json.Unmarshal(body, &probe); err != nil {
		// Not fatal — let mcp-go produce the proper JSON-RPC parse error.
		return "", body, nil
	}
	return probe.Method, body, nil
}

// writeFilteredToolsList parses the mcp-go tools/list response and emits a
// new response containing only the tools allowed for the current session.
func (s *Server) writeFilteredToolsList(w http.ResponseWriter, rec *capturingWriter, allow map[string]bool) {
	// Copy headers (content-type, etc.) from the captured response.
	for k, vs := range rec.header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}

	var env struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
		Error   json.RawMessage `json:"error,omitempty"`
	}
	if err := json.Unmarshal(rec.body.Bytes(), &env); err != nil {
		// Couldn't parse — pass through unchanged so the client sees the
		// underlying mcp-go error message.
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}
	if len(env.Error) > 0 || len(env.Result) == 0 {
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}

	var list struct {
		Tools []json.RawMessage `json:"tools"`
		Rest  map[string]json.RawMessage
	}
	// Decode into a generic map so we preserve any extra result fields
	// (e.g. PaginatedResult.NextCursor) the mcp-go server may add.
	var raw map[string]json.RawMessage
	if err := json.Unmarshal(env.Result, &raw); err != nil {
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}
	toolsRaw, ok := raw["tools"]
	if !ok {
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}
	if err := json.Unmarshal(toolsRaw, &list.Tools); err != nil {
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}

	filtered := make([]json.RawMessage, 0, len(list.Tools))
	allowedNames := make([]string, 0, len(filtered))
	allTotalNames := make([]string, 0, len(list.Tools))
	for _, t := range list.Tools {
		var entry struct {
			Name string `json:"name"`
		}
		if err := json.Unmarshal(t, &entry); err != nil {
			continue
		}
		allTotalNames = append(allTotalNames, entry.Name)
		if allow[entry.Name] {
			filtered = append(filtered, t)
			allowedNames = append(allowedNames, entry.Name)
		}
	}
	allowKeys := make([]string, 0, len(allow))
	for k := range allow {
		allowKeys = append(allowKeys, k)
	}
	slog.Info("acp.shim.tools_list_filtered",
		"total_in_catalog", len(list.Tools),
		"allowlist_size", len(allow),
		"emitted", len(filtered),
		"allowlist", allowKeys,
		"emitted_names", allowedNames,
		"catalog_names", allTotalNames,
	)

	filteredRaw, err := json.Marshal(filtered)
	if err != nil {
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}
	raw["tools"] = filteredRaw
	resultRaw, err := json.Marshal(raw)
	if err != nil {
		w.WriteHeader(rec.statusCode())
		_, _ = w.Write(rec.body.Bytes())
		return
	}

	out := struct {
		JSONRPC string          `json:"jsonrpc"`
		ID      json.RawMessage `json:"id"`
		Result  json.RawMessage `json:"result"`
	}{
		JSONRPC: env.JSONRPC,
		ID:      env.ID,
		Result:  resultRaw,
	}

	w.WriteHeader(rec.statusCode())
	_ = json.NewEncoder(w).Encode(out)
}

// capturingWriter is a minimal http.ResponseWriter that buffers the response
// body and headers so we can rewrite the tools/list payload before flushing
// to the real client.
type capturingWriter struct {
	header http.Header
	body   bytes.Buffer
	status int
}

func (c *capturingWriter) Header() http.Header        { return c.header }
func (c *capturingWriter) Write(b []byte) (int, error) { return c.body.Write(b) }
func (c *capturingWriter) WriteHeader(status int)     { c.status = status }
func (c *capturingWriter) statusCode() int {
	if c.status == 0 {
		return http.StatusOK
	}
	return c.status
}

// allCandidateTools returns the list of registered tool names that should be
// wired into the underlying MCP server. The set is intentionally broad — the
// security gate is the per-session allowlist; this list only constrains which
// tools the mcp-go server is even able to dispatch.
//
// Implementation note: we currently expose every tool in the registry. The
// resolver (Task 2) handles the security-relevant blacklist before populating
// SessionEntry.Allowlist, so any tool that survives resolution is allowed to
// be invoked for at least one session. Registering everything keeps the
// candidate set in sync with the registry without a second list to maintain.
func allCandidateTools(reg *tools.Registry) []string {
	return reg.List()
}

// syncTools scans the current GoClaw tool registry and lazily registers any
// tool not already wired into the underlying mcp-go server. Returns the count
// of newly-registered tools (0 if everything was already up to date).
//
// Background — Bug E (Phase 4 fork.15c-acp): NewServer only saw the 15 builtin
// tools because DB-driven MCP bridge servers (e.g. ops-mcp's
// mcp_ops__litellm_psql_query, mcp_ops__shell_exec_read) connect lazily AFTER
// shim startup, via mcpbridge.Manager.connectServer → registry.RegisterToolGroup.
// Without lazy-sync, those tools never appeared in the shim's catalog and the
// claude-agent-acp wrapper reported them as unavailable even though the agent
// grant resolver had added them to the per-session allowlist.
//
// Called at the top of handleMCP so each incoming JSON-RPC request observes the
// current registry state. mcp-go's MCPServer.AddTool is safe for concurrent
// runtime use, but we still guard with regMu so we don't double-add the same
// tool across overlapping requests.
func (s *Server) syncTools() int {
	if s.inner == nil || s.registry == nil {
		return 0
	}
	s.regMu.Lock()
	defer s.regMu.Unlock()
	var added int
	for _, name := range s.registry.List() {
		if s.registeredTools[name] {
			continue
		}
		t, ok := s.registry.Get(name)
		if !ok {
			continue
		}
		mcpTool := mcp.ConvertToMCPTool(t)
		s.inner.AddTool(mcpTool, s.makeSessionAwareHandler(s.registry, s.msgBus, name))
		s.registeredTools[name] = true
		added++
	}
	return added
}

