package providers

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Must match litellmGatewayProviderName in the agent package. Duplicated rather
// than shared because `store` imports `providers`, so the dependency cannot go
// the other way; same reason CacheBoundaryMarker exists twice.
const internalGatewayProviderName = "api-llm"

// Tool-level truth for one agent run, sent to the LiteLLM gateway's workflow
// endpoints so it lands in the same store as the spend logs of that run.
//
// Why not read it from GoClaw's own span store: the observability platform keeps
// 13 months of rollup on the gateway side, while GoClaw spans are a local
// operational store; joining across two databases for every report was the cost
// ADR-008 rejected.
//
// What this adds over `LiteLLM_SpendLogToolIndex`, which the proxy already
// derives per request: `args_hash`. Repeating one tool with identical arguments
// is the actual loop signal. Without it, loop detection can only use the
// dominance proxy measured on 2026-08-22, which cannot tell a retry sweep from a
// stuck agent.
//
// `step_index` and `exit_code` from the ADR sketch are deliberately absent. The
// dispatch seam (`makeExecuteToolRaw`) has no iteration counter, and threading
// one through three layers to serve a metric that has not been calibrated yet
// buys nothing today: `sequence_number` is assigned server-side and already
// orders events within a run. Exit codes are not a cross-tool concept in GoClaw;
// `status` carries the outcome.
type WorkflowEventEmitter interface {
	// EnsureRun registers the run once. Safe to call on every step.
	EnsureRun(ctx context.Context, runID, workflowType string, meta map[string]string)
	ToolCalled(ctx context.Context, runID, toolName, argsJSON string)
	ToolReturned(ctx context.Context, runID, toolName string, durationMS int, status string)
	RunFinished(ctx context.Context, runID, status string)
}

// WorkflowEmitterFor returns an emitter when p is the internal LiteLLM gateway,
// unwrapping a fallback wrapper, and nil otherwise. Run ids and tool names must
// never travel to third-party endpoints.
func WorkflowEmitterFor(p Provider) WorkflowEventEmitter {
	if fb, ok := p.(*ModelFallbackProvider); ok {
		p = fb.PrimaryProvider()
	}
	op, ok := p.(*OpenAIProvider)
	if !ok || op == nil || op.name != internalGatewayProviderName {
		return nil
	}
	return op
}

// Bound on in-flight event posts. Emission is best-effort telemetry: dropping an
// event must never slow a tool call or fail a run.
const workflowEmitConcurrency = 8

var (
	workflowEmitSlots = make(chan struct{}, workflowEmitConcurrency)
	workflowRunsSeen  sync.Map // runID -> struct{}, so EnsureRun posts once per run
	workflowDropped   int64
	workflowDropMu    sync.Mutex
)

func (p *OpenAIProvider) workflowPost(ctx context.Context, path string, body any) {
	select {
	case workflowEmitSlots <- struct{}{}:
	default:
		workflowDropMu.Lock()
		workflowDropped++
		n := workflowDropped
		workflowDropMu.Unlock()
		if n%100 == 1 {
			slog.Warn("workflow events: emitter saturated, dropping", "dropped_total", n)
		}
		return
	}
	go func() {
		defer func() { <-workflowEmitSlots }()
		// Detached from the caller's context on purpose: a tool call finishing or
		// its context being cancelled must not lose the event describing it.
		reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()

		payload, err := json.Marshal(body)
		if err != nil {
			slog.Warn("workflow events: marshal failed", "path", path, "error", err)
			return
		}
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPost, p.workflowURL(path), bytes.NewReader(payload))
		if err != nil {
			slog.Warn("workflow events: build request failed", "path", path, "error", err)
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", p.workflowAuthPrefix()+p.apiKey)
		resp, err := p.client.Do(req)
		if err != nil {
			slog.Warn("workflow events: post failed", "path", path, "error", err)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 300 {
			// 403 here means the key's allowed_routes lack the /v1/workflows
			// prefix, which is silent otherwise and was measured on prod
			// 2026-08-23 before the key was widened.
			slog.Warn("workflow events: rejected", "path", path, "status", resp.StatusCode)
		}
	}()
}

func (p *OpenAIProvider) EnsureRun(ctx context.Context, runID, workflowType string, meta map[string]string) {
	if runID == "" {
		return
	}
	if _, loaded := workflowRunsSeen.LoadOrStore(runID, struct{}{}); loaded {
		return
	}
	body := map[string]any{
		"workflow_type": workflowType,
		// The proxy accepts a caller-supplied session_id and treats a repeat as
		// idempotent, so the run keeps the identity it already sends on
		// x-litellm-session-id instead of adopting a second one.
		"session_id": runID,
	}
	if len(meta) > 0 {
		body["metadata"] = meta
	}
	p.workflowPost(ctx, "/v1/workflows/runs", body)
}

func (p *OpenAIProvider) ToolCalled(ctx context.Context, runID, toolName, argsJSON string) {
	if runID == "" || toolName == "" {
		return
	}
	p.workflowPost(ctx, "/v1/workflows/runs/"+runID+"/events", map[string]any{
		"event_type": "tool.called",
		"step_name":  toolName,
		"data": map[string]any{
			"tool_name": toolName,
			"args_hash": ShortConfigHash(argsJSON),
			"args_len":  len(argsJSON),
		},
	})
}

func (p *OpenAIProvider) ToolReturned(ctx context.Context, runID, toolName string, durationMS int, status string) {
	if runID == "" || toolName == "" {
		return
	}
	p.workflowPost(ctx, "/v1/workflows/runs/"+runID+"/events", map[string]any{
		"event_type": "tool.returned",
		"step_name":  toolName,
		"data": map[string]any{
			"tool_name":   toolName,
			"duration_ms": durationMS,
			"status":      status,
		},
	})
}

func (p *OpenAIProvider) RunFinished(ctx context.Context, runID, status string) {
	if runID == "" {
		return
	}
	workflowRunsSeen.Delete(runID)
	// Terminal state is a run field, not an event: the proxy derives status from
	// events only for its own step.* vocabulary.
	p.workflowPatch(ctx, "/v1/workflows/runs/"+runID, map[string]any{"status": status})
}

func (p *OpenAIProvider) workflowPatch(ctx context.Context, path string, body any) {
	select {
	case workflowEmitSlots <- struct{}{}:
	default:
		return
	}
	go func() {
		defer func() { <-workflowEmitSlots }()
		reqCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
		defer cancel()
		payload, err := json.Marshal(body)
		if err != nil {
			return
		}
		req, err := http.NewRequestWithContext(reqCtx, http.MethodPatch, p.workflowURL(path), bytes.NewReader(payload))
		if err != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", p.workflowAuthPrefix()+p.apiKey)
		resp, err := p.client.Do(req)
		if err != nil {
			slog.Warn("workflow events: patch failed", "path", path, "error", err)
			return
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode >= 300 {
			slog.Warn("workflow events: patch rejected", "path", path, "status", resp.StatusCode)
		}
	}()
}

// workflowAuthPrefix mirrors the defaulting in doRequest: an empty prefix means
// the standard bearer scheme, and a bare key would be rejected as unauthorised.
func (p *OpenAIProvider) workflowAuthPrefix() string {
	if p.authPrefix == "" {
		return "Bearer "
	}
	return p.authPrefix
}

// workflowURL builds an absolute proxy route from the provider's api base.
// apiBase points at the OpenAI-compatible surface and ends in /v1 by convention
// (GITNEXUS_LLM_BASE_URL=http://litellm:4000/v1), so concatenating a route that
// also starts with /v1 yields /v1/v1/workflows and a 404 on every event —
// measured on prod within minutes of the first rollout, which is what the
// rejection warning above is for.
func (p *OpenAIProvider) workflowURL(path string) string {
	if u, err := url.Parse(p.apiBase); err == nil && u.Scheme != "" && u.Host != "" {
		return u.Scheme + "://" + u.Host + path
	}
	// Malformed base: strip a trailing /v1 so the common case still works.
	return strings.TrimSuffix(strings.TrimSuffix(p.apiBase, "/"), "/v1") + path
}
