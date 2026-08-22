package providers

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
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
