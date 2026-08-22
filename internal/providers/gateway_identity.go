package providers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log/slog"
	"net/http"
	"regexp"
	"strings"
)

// GatewayRunIdentity carries agent-run identity to the LiteLLM gateway so that
// the many LLM calls of one agent run can be reassembled downstream.
//
// Why headers and not the request body: LiteLLM reads three request headers and
// persists them into LiteLLM_SpendLogs — `x-litellm-session-id` becomes
// SpendLogs.session_id (indexed), `x-litellm-agent-id` becomes SpendLogs.agent_id,
// and `x-litellm-spend-logs-metadata` (JSON) lands in metadata.spend_logs_metadata.
// Without them every call gets a per-request fallback id: measured 2026-08-22 on
// prod HNI, goclaw-service produced 2110-2616 calls/day with distinct session_id
// == call count, i.e. zero run correlation.
//
// Why injected via context instead of read from store.RunContext: `store` imports
// `providers`, so `providers` cannot import `store` without an import cycle. The
// agent loop owns RunContext and pushes the subset needed here, same pattern as
// WithReasoningDecision / WithRetryHook in this package.
type GatewayRunIdentity struct {
	RunID      string // run identifier — becomes SpendLogs.session_id
	AgentKey   string // agent key — becomes SpendLogs.agent_id
	AgentType  string // agent type, carried in spend-logs metadata
	SessionKey string // conversation/session key, carried in spend-logs metadata

	// Cohort dimensions. Comparing two branches requires holding the other two
	// fixed, so a missing dimension turns every comparison into noise.
	//
	// AgentVersion is a run-level constant; the agent row cannot change mid-run.
	// SkillVersion and PromptVersion are per-call: `SkillFilter` is resolved per
	// request and the prompt mode per turn, so a single run can switch skills.
	// They are filled by WithGatewayCallVersions right before each call.
	AgentVersion  string
	SkillVersion  string
	PromptVersion string
}

// ShortConfigHash returns a 12-hex-char digest of parts, stable across
// processes and restarts. Twelve chars keep ClickHouse LowCardinality columns
// cheap while leaving collision odds irrelevant at the scale of one row per
// agent run.
func ShortConfigHash(parts ...string) string {
	h := sha256.New()
	for _, p := range parts {
		_, _ = h.Write([]byte(p))
		// Separator so ("ab","c") and ("a","bc") cannot collide.
		_, _ = h.Write([]byte{0})
	}
	return hex.EncodeToString(h.Sum(nil))[:12]
}

// The pinned/available skills block renders above CacheBoundaryMarker
// (systemprompt.go section 4), so it lands inside the stable prefix and would
// otherwise make prompt and skill versions move together.
var availableSkillsBlock = regexp.MustCompile(`(?s)<available_skills>.*?</available_skills>`)

// DeriveSystemPromptVersions splits a rendered system prompt into its two
// independent cohort dimensions.
//
// Only the part above CacheBoundaryMarker is hashed: everything below it is
// per-turn content (current date, chat context, reply target), so hashing the
// whole prompt would produce a new "version" on every single request and
// measure nothing.
func DeriveSystemPromptVersions(system string) (promptVersion, skillVersion string) {
	if strings.TrimSpace(system) == "" {
		return "", ""
	}
	stable := system
	if idx := strings.Index(system, CacheBoundaryMarker); idx >= 0 {
		stable = system[:idx]
	}
	if blocks := availableSkillsBlock.FindAllString(stable, -1); len(blocks) > 0 {
		skillVersion = ShortConfigHash(blocks...)
		stable = availableSkillsBlock.ReplaceAllString(stable, "")
	}
	return ShortConfigHash(stable), skillVersion
}

// WithGatewayCallVersions fills the per-call cohort dimensions from the system
// messages of one outgoing request. No-op when ctx carries no identity, so
// third-party providers and non-agent traffic stay untouched.
func WithGatewayCallVersions(ctx context.Context, messages []Message) context.Context {
	id, ok := GatewayRunIdentityFromContext(ctx)
	if !ok {
		return ctx
	}
	var system strings.Builder
	for _, m := range messages {
		if m.Role == "system" {
			system.WriteString(m.Content)
		}
	}
	id.PromptVersion, id.SkillVersion = DeriveSystemPromptVersions(system.String())
	return WithGatewayRunIdentity(ctx, id)
}

type gatewayRunIdentityKey struct{}

// WithGatewayRunIdentity attaches run identity for gateway attribution. Callers
// should only attach it for the internal LiteLLM gateway provider, so run ids and
// agent keys never leak to third-party endpoints.
func WithGatewayRunIdentity(ctx context.Context, id GatewayRunIdentity) context.Context {
	return context.WithValue(ctx, gatewayRunIdentityKey{}, id)
}

// GatewayRunIdentityFromContext returns the identity attached to ctx, if any.
func GatewayRunIdentityFromContext(ctx context.Context) (GatewayRunIdentity, bool) {
	id, ok := ctx.Value(gatewayRunIdentityKey{}).(GatewayRunIdentity)
	return id, ok
}

// applyGatewayIdentityHeaders sets the LiteLLM attribution headers when ctx
// carries a GatewayRunIdentity. No-op otherwise, so non-agent traffic
// (embeddings, health probes) and third-party providers stay untouched.
func applyGatewayIdentityHeaders(ctx context.Context, req *http.Request) {
	id, ok := GatewayRunIdentityFromContext(ctx)
	if !ok {
		return
	}
	if id.RunID != "" {
		req.Header.Set("x-litellm-session-id", id.RunID)
	}
	if id.AgentKey != "" {
		req.Header.Set("x-litellm-agent-id", id.AgentKey)
	}

	meta := map[string]string{}
	if id.AgentType != "" {
		meta["agent_type"] = id.AgentType
	}
	if id.SessionKey != "" {
		meta["session_key"] = id.SessionKey
	}
	if id.AgentVersion != "" {
		meta["agent_version"] = id.AgentVersion
	}
	if id.SkillVersion != "" {
		meta["skill_version"] = id.SkillVersion
	}
	if id.PromptVersion != "" {
		meta["prompt_version"] = id.PromptVersion
	}
	if len(meta) == 0 {
		return
	}
	encoded, err := json.Marshal(meta)
	if err != nil {
		// Attribution is best-effort: a metadata encode failure must never fail
		// the LLM call itself.
		slog.Warn("gateway identity: marshal spend-logs metadata failed", "error", err)
		return
	}
	req.Header.Set("x-litellm-spend-logs-metadata", string(encoded))
}
