package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

type capturedCall struct {
	method string
	path   string
	auth   string
	body   map[string]any
}

// The emitter posts from a goroutine, so tests wait on the recorder instead of
// sleeping a fixed amount.
type recorder struct {
	mu    sync.Mutex
	calls []capturedCall
	got   chan struct{}
}

func newRecorder() *recorder {
	return &recorder{got: make(chan struct{}, 16)}
}

func (r *recorder) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		raw, _ := io.ReadAll(req.Body)
		var body map[string]any
		_ = json.Unmarshal(raw, &body)
		r.mu.Lock()
		r.calls = append(r.calls, capturedCall{
			method: req.Method,
			path:   req.URL.Path,
			auth:   req.Header.Get("Authorization"),
			body:   body,
		})
		r.mu.Unlock()
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
		r.got <- struct{}{}
	}
}

func (r *recorder) wait(t *testing.T, n int) []capturedCall {
	t.Helper()
	for i := range n {
		select {
		case <-r.got:
		case <-time.After(5 * time.Second):
			t.Fatalf("timed out waiting for call %d of %d", i+1, n)
		}
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]capturedCall, len(r.calls))
	copy(out, r.calls)
	return out
}

func newGatewayProvider(base string) *OpenAIProvider {
	return NewOpenAIProvider(internalGatewayProviderName, "sk-test", base, "vnpay-medium")
}

func TestWorkflowEmitterForRejectsThirdPartyProviders(t *testing.T) {
	if e := WorkflowEmitterFor(NewOpenAIProvider("openrouter", "sk-x", "https://openrouter.ai", "m")); e != nil {
		t.Error("run ids must never be sent to a third-party endpoint")
	}
	if e := WorkflowEmitterFor(newGatewayProvider("http://127.0.0.1:4000")); e == nil {
		t.Error("internal gateway must get an emitter")
	}
}

func TestWorkflowEmitterUnwrapsFallbackWrapper(t *testing.T) {
	inner := newGatewayProvider("http://127.0.0.1:4000")
	fb := NewModelFallbackProvider(
		FallbackCandidate{Provider: inner, ProviderName: internalGatewayProviderName, Model: "m"},
		nil, 1, false,
	)
	if WorkflowEmitterFor(fb) == nil {
		t.Error("a fallback-wrapped internal gateway must still emit")
	}
}

func TestEnsureRunPostsSessionIdOncePerRun(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	p := newGatewayProvider(srv.URL)

	workflowRunsSeen.Delete("cron:abc")
	p.EnsureRun(context.Background(), "cron:abc", "goclaw", map[string]string{"agent_key": "ops"})
	p.EnsureRun(context.Background(), "cron:abc", "goclaw", nil)

	calls := rec.wait(t, 1)
	if len(calls) != 1 {
		t.Fatalf("expected one create for one run, got %d", len(calls))
	}
	c := calls[0]
	if c.path != "/v1/workflows/runs" {
		t.Errorf("path = %q", c.path)
	}
	if c.body["session_id"] != "cron:abc" {
		t.Errorf("session_id = %v, want the run's own id", c.body["session_id"])
	}
	if c.auth != "Bearer sk-test" {
		t.Errorf("auth header = %q; a bare key is rejected as unauthorised", c.auth)
	}
	workflowRunsSeen.Delete("cron:abc")
}

// Identical arguments must produce one hash: that equality is the whole loop
// signal, and the dominance proxy exists only because it was missing.
func TestToolCalledHashesArgumentsStably(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	p := newGatewayProvider(srv.URL)

	p.ToolCalled(context.Background(), "run-1", "bash", `{"cmd":"ls"}`)
	p.ToolCalled(context.Background(), "run-1", "bash", `{"cmd":"ls"}`)
	p.ToolCalled(context.Background(), "run-1", "bash", `{"cmd":"rm -rf /"}`)

	calls := rec.wait(t, 3)
	if len(calls) != 3 {
		t.Fatalf("expected 3 events, got %d", len(calls))
	}
	// Emission is concurrent by design, so arrival order says nothing; assert on
	// the multiset instead.
	counts := map[string]int{}
	var anyData map[string]any
	for _, c := range calls {
		data, _ := c.body["data"].(map[string]any)
		h, _ := data["args_hash"].(string)
		if len(h) != 12 {
			t.Fatalf("args_hash = %q, want a 12-char digest", h)
		}
		counts[h]++
		anyData = data
		if c.path != "/v1/workflows/runs/run-1/events" {
			t.Errorf("path = %q", c.path)
		}
		if c.body["event_type"] != "tool.called" {
			t.Errorf("event_type = %v", c.body["event_type"])
		}
	}
	if len(counts) != 2 {
		t.Fatalf("expected 2 distinct hashes for 2 distinct payloads, got %d: %v", len(counts), counts)
	}
	seenPair := false
	for _, n := range counts {
		if n == 2 {
			seenPair = true
		}
	}
	if !seenPair {
		t.Error("identical arguments must hash identically or loops stay invisible")
	}
	// Raw arguments must not travel: only the digest and the length.
	if _, leaked := anyData["args"]; leaked {
		t.Error("tool arguments must not be sent to the observability store")
	}
	if anyData["args_len"] == nil {
		t.Error("args_len missing")
	}
}

func TestToolReturnedCarriesDurationAndStatus(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	p := newGatewayProvider(srv.URL)

	p.ToolReturned(context.Background(), "run-1", "bash", 1234, "error")

	calls := rec.wait(t, 1)
	data, _ := calls[0].body["data"].(map[string]any)
	if calls[0].body["event_type"] != "tool.returned" {
		t.Errorf("event_type = %v", calls[0].body["event_type"])
	}
	if data["duration_ms"] != float64(1234) {
		t.Errorf("duration_ms = %v", data["duration_ms"])
	}
	if data["status"] != "error" {
		t.Errorf("status = %v", data["status"])
	}
}

func TestRunFinishedPatchesStatusAndReleasesRun(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	p := newGatewayProvider(srv.URL)

	workflowRunsSeen.Store("run-9", struct{}{})
	p.RunFinished(context.Background(), "run-9", "completed")

	calls := rec.wait(t, 1)
	if calls[0].method != http.MethodPatch {
		t.Errorf("method = %s, want PATCH", calls[0].method)
	}
	if calls[0].body["status"] != "completed" {
		t.Errorf("status = %v", calls[0].body["status"])
	}
	if _, still := workflowRunsSeen.Load("run-9"); still {
		t.Error("a finished run must be forgotten so a reused id can register again")
	}
}

// A cancelled tool call must still deliver the event describing it.
func TestEmissionSurvivesCallerCancellation(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	p := newGatewayProvider(srv.URL)

	ctx, cancel := context.WithCancel(context.Background())
	p.ToolReturned(ctx, "run-2", "bash", 5, "ok")
	cancel()

	calls := rec.wait(t, 1)
	if len(calls) != 1 {
		t.Fatalf("expected the event to survive cancellation, got %d calls", len(calls))
	}
}

func TestEmptyRunIdEmitsNothing(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	p := newGatewayProvider(srv.URL)

	p.EnsureRun(context.Background(), "", "goclaw", nil)
	p.ToolCalled(context.Background(), "", "bash", "{}")
	p.ToolReturned(context.Background(), "", "bash", 1, "ok")
	p.RunFinished(context.Background(), "", "completed")

	select {
	case <-rec.got:
		t.Error("no run id means no event")
	case <-time.After(300 * time.Millisecond):
	}
}

// The api base ends in /v1 by convention, so a route that also starts with /v1
// must not be concatenated onto it. Prod returned 404 on every event before this
// was fixed.
func TestWorkflowURLDoesNotDoubleTheVersionPrefix(t *testing.T) {
	cases := []struct {
		base string
		want string
	}{
		{"http://litellm.litellm.svc.cluster.local:4000/v1", "http://litellm.litellm.svc.cluster.local:4000/v1/workflows/runs"},
		{"http://litellm:4000/v1/", "http://litellm:4000/v1/workflows/runs"},
		{"http://litellm:4000", "http://litellm:4000/v1/workflows/runs"},
		{"https://gw.example.com/openai/v1", "https://gw.example.com/v1/workflows/runs"},
	}
	for _, c := range cases {
		got := NewOpenAIProvider(internalGatewayProviderName, "sk", c.base, "m").workflowURL("/v1/workflows/runs")
		if got != c.want {
			t.Errorf("base %q -> %q, want %q", c.base, got, c.want)
		}
	}
}

func TestEnsureRunPostsToTheResolvedUrl(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	// Mirrors production: the base carries the /v1 suffix.
	p := newGatewayProvider(srv.URL + "/v1")

	workflowRunsSeen.Delete("cron:url")
	p.EnsureRun(context.Background(), "cron:url", "goclaw", nil)

	calls := rec.wait(t, 1)
	if calls[0].path != "/v1/workflows/runs" {
		t.Errorf("path = %q, want /v1/workflows/runs", calls[0].path)
	}
	workflowRunsSeen.Delete("cron:url")
}
