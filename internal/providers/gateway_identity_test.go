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

const testSkillsBlock = "<available_skills><skill><name>incident-analysis</name></skill></available_skills>"

func testPrompt(skills, stableExtra, dynamic string) string {
	return "## Tooling\n" + stableExtra + "\n" + skills + "\n" + CacheBoundaryMarker + "\nCurrent date: " + dynamic
}

// The part below the cache boundary changes every request (date, chat context).
// If it fed the hash, prompt_version would be a new value on every call and
// would measure nothing.
func TestDeriveSystemPromptVersionsIgnoresDynamicTail(t *testing.T) {
	a, aSkill := DeriveSystemPromptVersions(testPrompt(testSkillsBlock, "", "2026-08-22"))
	b, bSkill := DeriveSystemPromptVersions(testPrompt(testSkillsBlock, "", "2026-08-23"))

	if a != b {
		t.Errorf("prompt_version must ignore content below the boundary: %q vs %q", a, b)
	}
	if aSkill != bSkill {
		t.Errorf("skill_version must ignore content below the boundary: %q vs %q", aSkill, bSkill)
	}
	if a == "" || aSkill == "" {
		t.Fatalf("expected both dimensions populated, got prompt=%q skill=%q", a, aSkill)
	}
}

// The two dimensions must move independently, otherwise "same skill_version,
// different prompt_version" — the comparison the whole cohort design rests on —
// can never be expressed.
func TestDeriveSystemPromptVersionsSeparatesSkillsFromPrompt(t *testing.T) {
	basePrompt, baseSkill := DeriveSystemPromptVersions(testPrompt(testSkillsBlock, "", "d"))
	otherSkills := "<available_skills><skill><name>deploy-runbook</name></skill></available_skills>"

	skillChanged, skillChangedSkill := DeriveSystemPromptVersions(testPrompt(otherSkills, "", "d"))
	if skillChanged != basePrompt {
		t.Errorf("changing skills must not move prompt_version: %q vs %q", skillChanged, basePrompt)
	}
	if skillChangedSkill == baseSkill {
		t.Error("changing skills must move skill_version")
	}

	promptChanged, promptChangedSkill := DeriveSystemPromptVersions(testPrompt(testSkillsBlock, "## Safety", "d"))
	if promptChanged == basePrompt {
		t.Error("changing stable prompt content must move prompt_version")
	}
	if promptChangedSkill != baseSkill {
		t.Errorf("changing prompt must not move skill_version: %q vs %q", promptChangedSkill, baseSkill)
	}
}

func TestDeriveSystemPromptVersionsNoSkillsLeavesSkillVersionEmpty(t *testing.T) {
	_, skill := DeriveSystemPromptVersions("## Tooling\n" + CacheBoundaryMarker + "\nCurrent date: x")
	if skill != "" {
		t.Errorf("expected empty skill_version without a skills block, got %q", skill)
	}
}

func TestWithGatewayCallVersionsWithoutIdentityIsNoOp(t *testing.T) {
	ctx := WithGatewayCallVersions(context.Background(), []Message{{Role: "system", Content: "x"}})
	if _, ok := GatewayRunIdentityFromContext(ctx); ok {
		t.Error("must not fabricate an identity for non-agent traffic")
	}
}

func TestApplyGatewayIdentityHeadersCarriesVersionDimensions(t *testing.T) {
	req := newTestRequest(t)
	ctx := WithGatewayRunIdentity(context.Background(), GatewayRunIdentity{
		RunID:        "run-1",
		AgentKey:     "litellm-ops-agent",
		AgentVersion: "abc123abc123",
	})
	ctx = WithGatewayCallVersions(ctx, []Message{
		{Role: "system", Content: testPrompt(testSkillsBlock, "", "2026-08-22")},
		{Role: "user", Content: "ignored"},
	})

	applyGatewayIdentityHeaders(ctx, req)

	var meta map[string]string
	if err := json.Unmarshal([]byte(req.Header.Get("x-litellm-spend-logs-metadata")), &meta); err != nil {
		t.Fatalf("metadata header is not JSON: %v", err)
	}
	if meta["agent_version"] != "abc123abc123" {
		t.Errorf("agent_version = %q", meta["agent_version"])
	}
	for _, k := range []string{"skill_version", "prompt_version"} {
		if len(meta[k]) != 12 {
			t.Errorf("%s = %q, want a 12-char digest", k, meta[k])
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
