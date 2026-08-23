package agent

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/providers"
)

// Tool-level events for the observability store on the gateway side. These are
// separate from the local span collector on purpose: spans serve GoClaw's own
// operational UI, while these land next to the spend logs of the same run so
// cohort reports read one store instead of joining two databases (ADR-008).
//
// Every helper is a no-op unless the run is served by the internal LiteLLM
// gateway, so run ids and tool names never reach a third-party endpoint.

func (l *Loop) workflowEmitter() providers.WorkflowEventEmitter {
	if l.provider == nil {
		return nil
	}
	return providers.WorkflowEmitterFor(l.provider)
}

func (l *Loop) emitWorkflowRunStart(ctx context.Context, runID string) {
	e := l.workflowEmitter()
	if e == nil {
		return
	}
	meta := map[string]string{"agent_key": l.id, "agent_type": l.agentType}
	if l.agentVersion != "" {
		meta["agent_version"] = l.agentVersion
	}
	e.EnsureRun(ctx, runID, "goclaw", meta)
}

func (l *Loop) emitWorkflowToolCalled(ctx context.Context, runID, toolName, argsJSON string) {
	if e := l.workflowEmitter(); e != nil {
		e.ToolCalled(ctx, runID, toolName, argsJSON)
	}
}

func (l *Loop) emitWorkflowToolReturned(ctx context.Context, runID, toolName string, durationMS int, isError bool) {
	e := l.workflowEmitter()
	if e == nil {
		return
	}
	status := "ok"
	if isError {
		status = "error"
	}
	e.ToolReturned(ctx, runID, toolName, durationMS, status)
}

func (l *Loop) emitWorkflowRunFinished(ctx context.Context, runID string, failed bool) {
	e := l.workflowEmitter()
	if e == nil {
		return
	}
	status := "completed"
	if failed {
		status = "failed"
	}
	e.RunFinished(ctx, runID, status)
}
