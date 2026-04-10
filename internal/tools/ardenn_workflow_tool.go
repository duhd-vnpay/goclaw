// internal/tools/ardenn_workflow_tool.go
package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
	"github.com/nextlevelbuilder/goclaw/internal/store"
	pgardenn "github.com/nextlevelbuilder/goclaw/internal/store/pg/ardenn"
)

// ArdennWorkflowTool lets agents start, monitor, and manage Ardenn workflow runs.
type ArdennWorkflowTool struct {
	engine    *ardenn.Engine
	defStore  *pgardenn.PGDefinitionStore
	projStore *pgardenn.PGProjectionStore
}

// NewArdennWorkflowTool creates the ardenn_workflow tool.
// All dependencies are required — caller must nil-check before constructing.
func NewArdennWorkflowTool(
	engine *ardenn.Engine,
	defStore *pgardenn.PGDefinitionStore,
	projStore *pgardenn.PGProjectionStore,
) *ArdennWorkflowTool {
	return &ArdennWorkflowTool{
		engine:    engine,
		defStore:  defStore,
		projStore: projStore,
	}
}

func (t *ArdennWorkflowTool) Name() string { return "ardenn_workflow" }

func (t *ArdennWorkflowTool) Description() string {
	return `Start, monitor, and manage Ardenn workflow runs.
Always send a JSON object with an "action" field.

VALID ACTIONS AND EXACT PAYLOAD SHAPES:

1) start — Start a new workflow run
{
  "action": "start",
  "workflow_slug": "code-review",
  "variables": { "pr_url": "...", "repo": "..." },
  "tier": "standard"
}
workflow_slug: required. The slug of the published workflow to start.
variables: optional. Key-value pairs passed to step task templates.
tier: optional. "light" (L4 only), "standard" (L1+L4, default), or "full" (all layers).

2) status — Check the status of a running workflow
{
  "action": "status",
  "run_id": "uuid-string"
}

3) cancel — Cancel a running workflow
{
  "action": "cancel",
  "run_id": "uuid-string"
}

4) approve — Approve a step waiting at a gate
{
  "action": "approve",
  "run_id": "uuid-string",
  "step_id": "uuid-string",
  "feedback": "optional approval comment"
}

5) reject — Reject a step waiting at a gate
{
  "action": "reject",
  "run_id": "uuid-string",
  "step_id": "uuid-string",
  "feedback": "reason for rejection"
}`
}

func (t *ArdennWorkflowTool) Parameters() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"start", "status", "cancel", "approve", "reject"},
				"description": "The action to perform",
			},
			"workflow_slug": map[string]any{
				"type":        "string",
				"description": "Workflow slug (for start action)",
			},
			"run_id": map[string]any{
				"type":        "string",
				"description": "Workflow run ID (for status/cancel/approve/reject)",
			},
			"step_id": map[string]any{
				"type":        "string",
				"description": "Step ID (for approve/reject actions)",
			},
			"variables": map[string]any{
				"type":        "object",
				"description": "Variables to pass to the workflow (for start action)",
			},
			"tier": map[string]any{
				"type":        "string",
				"enum":        []string{"light", "standard", "full"},
				"description": "Execution tier (for start action, default: standard)",
			},
			"feedback": map[string]any{
				"type":        "string",
				"description": "Approval/rejection feedback (for approve/reject actions)",
			},
		},
		"required": []string{"action"},
	}
}

// Execute dispatches to the appropriate action handler.
func (t *ArdennWorkflowTool) Execute(ctx context.Context, args map[string]any) *Result {
	action, _ := args["action"].(string)

	switch action {
	case "start":
		return t.executeStart(ctx, args)
	case "status":
		return t.executeStatus(ctx, args)
	case "cancel":
		return t.executeCancel(ctx, args)
	case "approve":
		return t.executeApprove(ctx, args)
	case "reject":
		return t.executeReject(ctx, args)
	default:
		return ErrorResult(fmt.Sprintf("unknown action %q — valid actions: start, status, cancel, approve, reject", action))
	}
}

// executeStart looks up a workflow by slug, gets steps, builds StepDefs, and starts a run.
func (t *ArdennWorkflowTool) executeStart(ctx context.Context, args map[string]any) *Result {
	slug, _ := args["workflow_slug"].(string)
	if slug == "" {
		return ErrorResult("workflow_slug is required for start action")
	}

	tenantID := store.TenantIDFromContext(ctx)
	if tenantID == uuid.Nil {
		tenantID = store.MasterTenantID
	}

	// Look up published workflow by slug
	wf, err := t.defStore.GetPublishedWorkflow(ctx, tenantID, slug)
	if err != nil {
		slog.Warn("ardenn_workflow: workflow not found",
			"slug", slug, "tenant_id", tenantID, "error", err)
		return ErrorResult(fmt.Sprintf("workflow %q not found or not published", slug))
	}

	// Get step definitions
	steps, err := t.defStore.GetSteps(ctx, wf.ID)
	if err != nil {
		return ErrorResult(fmt.Sprintf("failed to load steps for workflow %q: %v", slug, err))
	}
	if len(steps) == 0 {
		return ErrorResult(fmt.Sprintf("workflow %q has no steps defined", slug))
	}

	stepDefs := pgardenn.ToStepDefs(steps)

	// Resolve tier: explicit arg > workflow default > "standard"
	tierStr, _ := args["tier"].(string)
	if tierStr == "" {
		tierStr = wf.Tier
	}
	if tierStr == "" {
		tierStr = "standard"
	}

	// Resolve variables: merge workflow defaults + caller variables
	var wfVars map[string]any
	if len(wf.Variables) > 2 {
		_ = json.Unmarshal(wf.Variables, &wfVars)
	}
	callerVars, _ := args["variables"].(map[string]any)
	mergedVars := ardenn.MergeVariables(wfVars, callerVars)

	// Resolve triggered_by from context
	var triggeredBy *uuid.UUID
	if agentID := store.AgentIDFromContext(ctx); agentID != uuid.Nil {
		triggeredBy = &agentID
	}

	// Start the run
	runID, err := t.engine.StartRun(ctx, ardenn.StartRunRequest{
		TenantID:    tenantID,
		WorkflowID:  wf.ID,
		TriggeredBy: triggeredBy,
		Tier:        tierStr,
		Variables:   mergedVars,
		StepDefs:    stepDefs,
	})
	if err != nil {
		slog.Error("ardenn_workflow: start failed",
			"workflow", slug, "error", err)
		return ErrorResult(fmt.Sprintf("failed to start workflow: %v", err))
	}

	// Format step summary
	var stepNames []string
	for _, s := range steps {
		stepNames = append(stepNames, s.Name)
	}

	slog.Info("ardenn_workflow: run started",
		"run_id", runID,
		"workflow", slug,
		"tier", tierStr,
		"steps", len(steps),
	)

	return NewResult(fmt.Sprintf(
		"Workflow **%s** started.\n\n"+
			"- **Run ID:** `%s`\n"+
			"- **Tier:** %s\n"+
			"- **Steps:** %s\n\n"+
			"Use `ardenn_workflow` with `action: \"status\"` and `run_id: \"%s\"` to check progress.",
		wf.Name, runID, tierStr, strings.Join(stepNames, " -> "), runID,
	))
}

// executeStatus is a stub — will be implemented in Task 3.
func (t *ArdennWorkflowTool) executeStatus(_ context.Context, _ map[string]any) *Result {
	return ErrorResult("not yet implemented")
}

// executeCancel is a stub — will be implemented in Task 3.
func (t *ArdennWorkflowTool) executeCancel(_ context.Context, _ map[string]any) *Result {
	return ErrorResult("not yet implemented")
}

// executeApprove is a stub — will be implemented in Task 4.
func (t *ArdennWorkflowTool) executeApprove(_ context.Context, _ map[string]any) *Result {
	return ErrorResult("not yet implemented")
}

// executeReject is a stub — will be implemented in Task 4.
func (t *ArdennWorkflowTool) executeReject(_ context.Context, _ map[string]any) *Result {
	return ErrorResult("not yet implemented")
}

// parseRunID extracts and validates a run_id from tool arguments.
func parseRunID(args map[string]any) (uuid.UUID, error) {
	runIDStr, _ := args["run_id"].(string)
	if runIDStr == "" {
		return uuid.Nil, fmt.Errorf("run_id is required")
	}
	runID, err := uuid.Parse(runIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid run_id %q: %v", runIDStr, err)
	}
	return runID, nil
}

// parseStepID extracts and validates a step_id from tool arguments.
func parseStepID(args map[string]any) (uuid.UUID, error) {
	stepIDStr, _ := args["step_id"].(string)
	if stepIDStr == "" {
		return uuid.Nil, fmt.Errorf("step_id is required")
	}
	stepID, err := uuid.Parse(stepIDStr)
	if err != nil {
		return uuid.Nil, fmt.Errorf("invalid step_id %q: %v", stepIDStr, err)
	}
	return stepID, nil
}
