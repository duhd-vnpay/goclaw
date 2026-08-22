package providers

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func newTestRequest(t *testing.T) *http.Request {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, "http://litellm.litellm.svc.cluster.local:4000/v1/chat/completions", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	return req
}

func TestApplyGatewayIdentityHeadersNoIdentityIsNoOp(t *testing.T) {
	req := newTestRequest(t)

	applyGatewayIdentityHeaders(context.Background(), req)

	for _, h := range []string{"x-litellm-session-id", "x-litellm-agent-id", "x-litellm-spend-logs-metadata"} {
		if got := req.Header.Get(h); got != "" {
			t.Errorf("expected %s unset without identity, got %q", h, got)
		}
	}
}

func TestApplyGatewayIdentityHeadersSetsAttribution(t *testing.T) {
	req := newTestRequest(t)
	ctx := WithGatewayRunIdentity(context.Background(), GatewayRunIdentity{
		RunID:      "8f1c2d3e-4a5b-6c7d-8e9f-0a1b2c3d4e5f",
		AgentKey:   "litellm-ops-agent",
		AgentType:  "ops",
		SessionKey: "agent:litellm-ops-agent:cron:abc123",
	})

	applyGatewayIdentityHeaders(ctx, req)

	if got := req.Header.Get("x-litellm-session-id"); got != "8f1c2d3e-4a5b-6c7d-8e9f-0a1b2c3d4e5f" {
		t.Errorf("session id header = %q", got)
	}
	if got := req.Header.Get("x-litellm-agent-id"); got != "litellm-ops-agent" {
		t.Errorf("agent id header = %q", got)
	}

	raw := req.Header.Get("x-litellm-spend-logs-metadata")
	if raw == "" {
		t.Fatal("spend-logs metadata header missing")
	}
	var meta map[string]string
	if err := json.Unmarshal([]byte(raw), &meta); err != nil {
		t.Fatalf("metadata is not valid JSON (LiteLLM parses it with safe_json_loads): %v", err)
	}
	if meta["agent_type"] != "ops" || meta["session_key"] != "agent:litellm-ops-agent:cron:abc123" {
		t.Errorf("metadata payload = %v", meta)
	}
}

func TestApplyGatewayIdentityHeadersSkipsEmptyFields(t *testing.T) {
	req := newTestRequest(t)
	ctx := WithGatewayRunIdentity(context.Background(), GatewayRunIdentity{RunID: "run-12345678"})

	applyGatewayIdentityHeaders(ctx, req)

	if got := req.Header.Get("x-litellm-session-id"); got != "run-12345678" {
		t.Errorf("session id header = %q", got)
	}
	if got := req.Header.Get("x-litellm-agent-id"); got != "" {
		t.Errorf("expected no agent id header, got %q", got)
	}
	if got := req.Header.Get("x-litellm-spend-logs-metadata"); got != "" {
		t.Errorf("expected no metadata header when nothing to carry, got %q", got)
	}
}

// The provider must send the headers on the real request path, not only via the
// helper — this is what actually reaches the gateway.
func TestDoRequestSendsGatewayIdentityHeaders(t *testing.T) {
	var got http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		got = r.Header.Clone()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[]}`))
	}))
	defer srv.Close()

	p := NewOpenAIProvider("api-llm", "sk-test", srv.URL, "vnpay-medium")
	ctx := WithGatewayRunIdentity(context.Background(), GatewayRunIdentity{
		RunID:    "run-abcdef123456",
		AgentKey: "goclaw-ops-agent",
	})

	body, err := p.doRequest(ctx, map[string]any{"model": "vnpay-medium"})
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	defer func() { _ = body.Close() }()

	if v := got.Get("x-litellm-session-id"); v != "run-abcdef123456" {
		t.Errorf("session id header on wire = %q", v)
	}
	if v := got.Get("x-litellm-agent-id"); v != "goclaw-ops-agent" {
		t.Errorf("agent id header on wire = %q", v)
	}
}
