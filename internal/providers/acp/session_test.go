package acp

import (
	"context"
	"encoding/json"
	"io"
	"strings"
	"testing"
	"time"
)

// buildACPProcess creates an ACPProcess wired to an in-process io.Pipe conn.
// Returns the process, a writer the test can inject responses into (serverW),
// and a reader the test can read outbound requests from (serverR).
//
// The returned serverR is a *bufferedPipeReader that always drains — so the
// conn's writeMessage never blocks even if the test ignores outbound bytes.
//
//	clientW → serverR  (test reads what the process sends)
//	serverW → clientR  (test injects responses to the process)
func buildACPProcess(handler RequestHandler, extraNotify NotifyHandler) (
	proc *ACPProcess,
	serverW io.WriteCloser,
	serverR io.ReadCloser,
) {
	clientR, sW := io.Pipe() // test writes here → conn reads
	sR, clientW := io.Pipe() // conn writes here → test reads

	// Create proc first so the notifyHandler closure can reference it.
	proc = &ACPProcess{
		exited: make(chan struct{}),
		ctx:    context.Background(),
		cancel: func() {},
	}

	// Mirror production spawn: always route session/update to proc.dispatchUpdate,
	// then optionally forward to the test's extra notify hook.
	internalNotify := func(method string, params json.RawMessage) {
		if method == "session/update" {
			var update SessionUpdate
			if json.Unmarshal(params, &update) == nil {
				proc.dispatchUpdate(update)
			}
		}
		if extraNotify != nil {
			extraNotify(method, params)
		}
	}

	conn := NewConn(clientW, clientR, handler, internalNotify)
	conn.Start()
	proc.conn = conn

	return proc, sW, sR
}

// drainPipe reads sR in background so writes to clientW never block.
// Returns a channel that receives each complete line written by the conn.
func drainPipeLines(sR io.ReadCloser) chan []byte {
	ch := make(chan []byte, 64)
	go func() {
		defer close(ch)
		buf := make([]byte, 32*1024)
		for {
			n, err := sR.Read(buf)
			if n > 0 {
				line := make([]byte, n)
				copy(line, buf[:n])
				select {
				case ch <- line:
				default:
				}
			}
			if err != nil {
				return
			}
		}
	}()
	return ch
}

// replyTo reads one JSON-RPC request from r, then writes a response with the
// given result JSON back to w. Runs in its own goroutine and signals done.
func replyTo(r io.Reader, w io.Writer, resultJSON string) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 32*1024)
		n, err := r.Read(buf)
		if err != nil || n == 0 {
			return
		}
		var req jsonrpcMessage
		if err := json.Unmarshal(buf[:n], &req); err != nil {
			return
		}
		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(resultJSON),
		}
		data, _ := json.Marshal(resp)
		w.Write(append(data, '\n'))
	}()
	return done
}

// replyError reads one request and sends back a JSON-RPC error response.
func replyError(r io.Reader, w io.Writer, code int, message string) chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		buf := make([]byte, 32*1024)
		n, _ := r.Read(buf)
		if n == 0 {
			return
		}
		var req jsonrpcMessage
		json.Unmarshal(buf[:n], &req)
		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Error:   &jsonrpcError{Code: code, Message: message},
		}
		data, _ := json.Marshal(resp)
		w.Write(append(data, '\n'))
	}()
	return done
}

// --- Initialize tests ---

func TestACPProcess_Initialize_Success(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	respJSON := `{"agentInfo":{"name":"claude","version":"1.0"},"agentCapabilities":{"loadSession":true}}`
	done := replyTo(serverR, serverW, respJSON)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	if err := proc.Initialize(ctx); err != nil {
		t.Fatalf("Initialize error: %v", err)
	}
	if !proc.agentCaps.LoadSession {
		t.Error("expected LoadSession=true after Initialize")
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for replyTo goroutine")
	}
}

func TestACPProcess_Initialize_ErrorResponse(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	done := replyError(serverR, serverW, -32600, "not supported")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	err := proc.Initialize(ctx)
	if err == nil {
		t.Fatal("expected error from Initialize")
	}
	if !strings.Contains(err.Error(), "acp initialize") {
		t.Errorf("expected 'acp initialize' prefix, got %q", err.Error())
	}

	<-done
}

func TestACPProcess_Initialize_Timeout(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()

	// Drain requests so writeMessage doesn't block, but never send a response.
	go func() {
		buf := make([]byte, 4096)
		for {
			_, err := serverR.Read(buf)
			if err != nil {
				return
			}
		}
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := proc.Initialize(ctx)
	if err == nil {
		t.Fatal("expected timeout error")
	}
	serverR.Close()
}

// --- NewSession tests ---

func TestACPProcess_NewSession_Success(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	done := replyTo(serverR, serverW, `{"sessionId":"sess-xyz"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid, err := proc.NewSession(ctx)
	if err != nil {
		t.Fatalf("NewSession error: %v", err)
	}
	if sid != "sess-xyz" {
		t.Errorf("expected sessionID='sess-xyz', got %q", sid)
	}
	<-done
}

func TestACPProcess_NewSession_Error(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	done := replyError(serverR, serverW, -32000, "session failed")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := proc.NewSession(ctx)
	if err == nil {
		t.Fatal("expected error from NewSession")
	}
	if !strings.Contains(err.Error(), "acp session/new") {
		t.Errorf("expected 'acp session/new' prefix, got %q", err.Error())
	}
	<-done
}

// TestACPProcess_NewSessionImpl_NilShimMatchesLegacy guards the Phase 4
// Task 6 refactor: newSessionImpl(ctx, nil, nil) must behave identically
// to the pre-refactor NewSession — same request shape (empty McpServers,
// resolved cwd), same response handling. We assert by inspecting the
// outbound request bytes.
func TestACPProcess_NewSessionImpl_NilShimMatchesLegacy(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	captured := make(chan jsonrpcMessage, 1)
	go func() {
		buf := make([]byte, 32*1024)
		n, err := serverR.Read(buf)
		if err != nil || n == 0 {
			return
		}
		var req jsonrpcMessage
		if err := json.Unmarshal(buf[:n], &req); err != nil {
			return
		}
		captured <- req
		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"sessionId":"sess-impl"}`),
		}
		data, _ := json.Marshal(resp)
		serverW.Write(append(data, '\n'))
	}()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid, err := proc.newSessionImpl(ctx, nil, nil)
	if err != nil {
		t.Fatalf("newSessionImpl error: %v", err)
	}
	if sid != "sess-impl" {
		t.Errorf("expected sessionID='sess-impl', got %q", sid)
	}

	select {
	case req := <-captured:
		if req.Method != "session/new" {
			t.Errorf("expected method=session/new, got %q", req.Method)
		}
		var body NewSessionRequest
		if err := json.Unmarshal(req.Params, &body); err != nil {
			t.Fatalf("unmarshal params: %v", err)
		}
		// Phase 3 legacy shape: McpServers populated as empty slice
		// (NOT nil) so the wire JSON is `"mcpServers":[]` — matches the
		// pre-refactor NewSession exactly.
		if body.McpServers == nil {
			t.Error("expected McpServers to be non-nil empty slice (legacy parity)")
		}
		if len(body.McpServers) != 0 {
			t.Errorf("expected empty McpServers, got %v", body.McpServers)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for captured request")
	}
}

// --- Phase 4 Task 7: McpServers populate + capability gate ---

// fakeShim is a minimal ShimHandle stand-in. SessionURL returns a
// recognizable shape so tests can assert that the URL actually got
// appended to NewSessionRequest.McpServers, and RegisterSession records
// each call (we only care that the proposed sid reaches the registrar
// — full SessionEntry shape lives in the mcp_shim adapter).
type fakeShim struct {
	registered []string
	url        string
}

func (f *fakeShim) URL() string                  { return "http://shim.local/mcp" }
func (f *fakeShim) SessionURL(sid string) string { return "http://shim.local/mcp?session=" + sid }
func (f *fakeShim) RegisterSession(entry any)    { f.registered = append(f.registered, "any") }
func (f *fakeShim) UnregisterSession(sid string) {}
func (f *fakeShim) SetMCPSessionBuilder(fn any)  {}

// mcpEntryURL extracts the `url` field from a marshaled McpServer entry.
// After JSON round-trip into []any the element is a map[string]any, so
// tests can't index it as a struct — this helper hides the cast.
func mcpEntryURL(entry any) string {
	m, ok := entry.(map[string]any)
	if !ok {
		return ""
	}
	url, _ := m["url"].(string)
	return url
}

// TestNewSessionImpl_PopulatesMcpServers_WhenHTTPAdvertised verifies the
// happy path: HTTP MCP capability advertised AND shim non-nil → reserve a
// SID, call register, advertise the shim URL in McpServers.
func TestNewSessionImpl_PopulatesMcpServers_WhenHTTPAdvertised(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()
	proc.agentCaps = AgentCaps{MCPCapabilities: &MCPCaps{HTTP: true}}

	captured := make(chan jsonrpcMessage, 1)
	go func() {
		buf := make([]byte, 32*1024)
		n, err := serverR.Read(buf)
		if err != nil || n == 0 {
			return
		}
		var req jsonrpcMessage
		if err := json.Unmarshal(buf[:n], &req); err != nil {
			return
		}
		captured <- req
		// Echo the proposed sid back so the test can also assert no remap log.
		var body NewSessionRequest
		_ = json.Unmarshal(req.Params, &body)
		var actualSID string
		if len(body.McpServers) > 0 {
			// Pull the proposed sid out of the URL for the response.
			url := mcpEntryURL(body.McpServers[0])
			if i := strings.Index(url, "session="); i >= 0 {
				actualSID = url[i+len("session="):]
			}
		}
		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"sessionId":"` + actualSID + `"}`),
		}
		data, _ := json.Marshal(resp)
		serverW.Write(append(data, '\n'))
	}()

	shim := &fakeShim{}
	var registered []string
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid, err := proc.newSessionImpl(ctx, shim, func(s string) {
		registered = append(registered, s)
	})
	if err != nil {
		t.Fatalf("newSessionImpl: %v", err)
	}
	if sid == "" {
		t.Fatal("expected non-empty session id")
	}
	if len(registered) != 1 {
		t.Fatalf("expected exactly 1 register() invocation, got %d", len(registered))
	}
	if registered[0] != sid {
		t.Errorf("register() got sid %q, session/new returned %q (parity expected)",
			registered[0], sid)
	}

	select {
	case req := <-captured:
		var body NewSessionRequest
		if err := json.Unmarshal(req.Params, &body); err != nil {
			t.Fatalf("unmarshal params: %v", err)
		}
		if len(body.McpServers) != 1 {
			t.Fatalf("expected exactly 1 mcpServers entry, got %d (%v)",
				len(body.McpServers), body.McpServers)
		}
		want := shim.SessionURL(sid)
		if got := mcpEntryURL(body.McpServers[0]); got != want {
			t.Errorf("McpServers[0].url = %q, want %q", got, want)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for captured request")
	}
}

// TestNewSessionImpl_GracefullyDegrades_WhenHTTPNotAdvertised verifies the
// capability gate: shim non-nil but agent did NOT advertise MCPCapabilities.HTTP
// → register is not called, McpServers stays empty (Phase 3 parity).
func TestNewSessionImpl_GracefullyDegrades_WhenHTTPNotAdvertised(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()
	// Capability flag NOT set — agentCaps.MCPCapabilities == nil.
	proc.agentCaps = AgentCaps{LoadSession: true}

	done := replyTo(serverR, serverW, `{"sessionId":"sess-no-shim"}`)

	shim := &fakeShim{}
	var registerCalls int
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	sid, err := proc.newSessionImpl(ctx, shim, func(s string) {
		registerCalls++
	})
	if err != nil {
		t.Fatalf("newSessionImpl: %v", err)
	}
	if sid != "sess-no-shim" {
		t.Errorf("expected echo of agent-minted sid, got %q", sid)
	}
	if registerCalls != 0 {
		t.Errorf("expected register() NOT to fire on cap-gated path, fired %d times", registerCalls)
	}
	<-done
}

// TestLoadSessionImpl_PopulatesMcpServers_WhenHTTPAdvertised mirrors
// NewSession populate for the session/load path (Revision 1 supplement).
// The shim URL uses the prior sid so the per-session allowlist survives
// process respawn.
func TestLoadSessionImpl_PopulatesMcpServers_WhenHTTPAdvertised(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()
	proc.agentCaps = AgentCaps{LoadSession: true, MCPCapabilities: &MCPCaps{HTTP: true}}

	captured := make(chan jsonrpcMessage, 1)
	go func() {
		buf := make([]byte, 32*1024)
		n, err := serverR.Read(buf)
		if err != nil || n == 0 {
			return
		}
		var req jsonrpcMessage
		if err := json.Unmarshal(buf[:n], &req); err != nil {
			return
		}
		captured <- req
		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"sessionId":"sess-restored"}`),
		}
		data, _ := json.Marshal(resp)
		serverW.Write(append(data, '\n'))
	}()

	shim := &fakeShim{}
	var registered []string
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	priorSID := "sess-restored"
	sid, err := proc.loadSessionImpl(ctx, priorSID, shim, func(s string) {
		registered = append(registered, s)
	})
	if err != nil {
		t.Fatalf("loadSessionImpl: %v", err)
	}
	if sid != priorSID {
		t.Errorf("expected sid=%q, got %q", priorSID, sid)
	}
	if len(registered) != 1 || registered[0] != priorSID {
		t.Errorf("expected register(%q), got %v", priorSID, registered)
	}

	select {
	case req := <-captured:
		var body LoadSessionRequest
		if err := json.Unmarshal(req.Params, &body); err != nil {
			t.Fatalf("unmarshal: %v", err)
		}
		if len(body.McpServers) != 1 {
			t.Fatalf("expected 1 mcpServers entry, got %d", len(body.McpServers))
		}
		want := shim.SessionURL(priorSID)
		if got := mcpEntryURL(body.McpServers[0]); got != want {
			t.Errorf("McpServers[0].url = %q, want %q", got, want)
		}
		if body.SessionID != priorSID {
			t.Errorf("LoadSessionRequest.SessionID = %q, want %q", body.SessionID, priorSID)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for captured request")
	}
}

// --- Prompt tests ---

func TestACPProcess_Prompt_Success(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	done := replyTo(serverR, serverW, `{"stopReason":"endTurn"}`)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := proc.Prompt(ctx, "sess-1", []ContentBlock{{Type: "text", Text: "hi"}}, nil)
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}
	if resp.StopReason != "endTurn" {
		t.Errorf("expected stopReason='endTurn', got %q", resp.StopReason)
	}
	<-done
}

func TestACPProcess_Prompt_WithUpdateCallback(t *testing.T) {
	updates := make(chan SessionUpdate, 10)
	notifyHandler := func(method string, params json.RawMessage) {
		if method == "session/update" {
			var u SessionUpdate
			json.Unmarshal(params, &u)
			updates <- u
		}
	}

	proc, serverW, serverR := buildACPProcess(nil, notifyHandler)
	defer serverW.Close()
	defer serverR.Close()

	const sid = "sess-2"

	// Goroutine: send a notification, then a response
	go func() {
		buf := make([]byte, 32*1024)
		n, _ := serverR.Read(buf)
		var req jsonrpcMessage
		json.Unmarshal(buf[:n], &req)

		// Send session/update notification with matching session ID
		notif := jsonrpcMessage{
			JSONRPC: "2.0",
			Method:  "session/update",
			Params:  json.RawMessage(`{"sessionId":"sess-2","kind":"message","stopReason":""}`),
		}
		nd, _ := json.Marshal(notif)
		serverW.Write(append(nd, '\n'))

		// Send prompt response
		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"stopReason":"endTurn"}`),
		}
		rd, _ := json.Marshal(resp)
		serverW.Write(append(rd, '\n'))
	}()

	var received []SessionUpdate
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := proc.Prompt(ctx, sid, []ContentBlock{{Type: "text", Text: "hello"}}, func(u SessionUpdate) {
		received = append(received, u)
	})
	if err != nil {
		t.Fatalf("Prompt error: %v", err)
	}

	// Allow notification delivery
	time.Sleep(20 * time.Millisecond)
	// Verify no panic and at least one update was dispatched
	if len(received) == 0 {
		t.Error("expected at least one session/update to be dispatched")
	}
}

func TestACPProcess_Prompt_Error(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	done := replyError(serverR, serverW, -32000, "prompt failed")

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	_, err := proc.Prompt(ctx, "sess-3", []ContentBlock{{Type: "text", Text: "hi"}}, nil)
	if err == nil {
		t.Fatal("expected error from Prompt")
	}
	if !strings.Contains(err.Error(), "acp session/prompt") {
		t.Errorf("expected 'acp session/prompt' prefix, got %q", err.Error())
	}
	<-done
}

func TestACPProcess_Prompt_SetsInUse(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()
	defer serverR.Close()

	started := make(chan struct{})
	go func() {
		buf := make([]byte, 32*1024)
		n, _ := serverR.Read(buf)
		var req jsonrpcMessage
		json.Unmarshal(buf[:n], &req)

		close(started)
		time.Sleep(20 * time.Millisecond)

		resp := jsonrpcMessage{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result:  json.RawMessage(`{"stopReason":"endTurn"}`),
		}
		data, _ := json.Marshal(resp)
		serverW.Write(append(data, '\n'))
	}()

	done := make(chan struct{})
	go func() {
		defer close(done)
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		proc.Prompt(ctx, "sess-4", []ContentBlock{{Type: "text", Text: "x"}}, nil)
	}()

	<-started
	if proc.inUse.Load() != 1 {
		t.Errorf("expected inUse=1 during Prompt, got %d", proc.inUse.Load())
	}

	<-done
	if proc.inUse.Load() != 0 {
		t.Errorf("expected inUse=0 after Prompt, got %d", proc.inUse.Load())
	}
}

// --- Cancel tests ---

func TestACPProcess_Cancel_SendsNotification(t *testing.T) {
	proc, serverW, serverR := buildACPProcess(nil, nil)
	defer serverW.Close()

	received := make(chan string, 1)
	go func() {
		buf := make([]byte, 4096)
		n, _ := serverR.Read(buf)
		var msg jsonrpcMessage
		json.Unmarshal(buf[:n], &msg)
		received <- msg.Method
	}()

	err := proc.Cancel("sess-cancel")
	if err != nil {
		t.Fatalf("Cancel error: %v", err)
	}

	select {
	case method := <-received:
		if method != "session/cancel" {
			t.Errorf("expected 'session/cancel', got %q", method)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancel notification")
	}
}

// --- dispatchUpdate / registerUpdateFn ---

func TestACPProcess_DispatchUpdate_NilFn(t *testing.T) {
	proc := &ACPProcess{}
	// Should not panic with no registered fn for the session ID
	proc.dispatchUpdate(SessionUpdate{SessionID: "x", Kind: "message"})
}

func TestACPProcess_DispatchUpdate_CallsFn(t *testing.T) {
	proc := &ACPProcess{}
	called := false
	proc.registerUpdateFn("sid-1", func(u SessionUpdate) {
		called = true
	})
	proc.dispatchUpdate(SessionUpdate{SessionID: "sid-1", Kind: "message"})
	if !called {
		t.Error("expected updateFn to be called")
	}
}

func TestACPProcess_DispatchUpdate_RoutesCorrectSession(t *testing.T) {
	proc := &ACPProcess{}
	var calledA, calledB bool
	proc.registerUpdateFn("sid-A", func(u SessionUpdate) { calledA = true })
	proc.registerUpdateFn("sid-B", func(u SessionUpdate) { calledB = true })

	proc.dispatchUpdate(SessionUpdate{SessionID: "sid-A"})
	if !calledA {
		t.Error("expected fn for sid-A to be called")
	}
	if calledB {
		t.Error("expected fn for sid-B NOT to be called")
	}
}

func TestACPProcess_UnregisterUpdateFn(t *testing.T) {
	proc := &ACPProcess{}
	called := false
	proc.registerUpdateFn("sid-x", func(u SessionUpdate) { called = true })
	proc.unregisterUpdateFn("sid-x")
	proc.dispatchUpdate(SessionUpdate{SessionID: "sid-x"})
	if called {
		t.Error("expected fn to not be called after unregister")
	}
}
