package ardenn

import (
	"context"

	"github.com/google/uuid"
)

// StepDef defines a single step within a workflow definition.
type StepDef struct {
	ID               uuid.UUID
	WorkflowID       uuid.UUID
	Slug             string
	Name             string
	Description      string
	Position         int
	AgentKey         string
	TaskTemplate     string
	DependsOn        []uuid.UUID
	Condition        string
	Timeout          string
	Constraints      map[string]any
	Continuity       map[string]any
	Evaluation       *EvalConfig
	Gate             *GateConfig
	DispatchTo       string
	DispatchTarget   string
	EvalMaxRounds    int
	EscalationPolicy string
}

// EvalConfig holds evaluation configuration for a step.
type EvalConfig struct {
	Computational []string
	Inferential   *InfConfig
}

// InfConfig holds inferential evaluation configuration.
type InfConfig struct {
	Evaluator string
	RubricRef string
}

// GateConfig defines the approval gate for a step.
type GateConfig struct {
	Type         string `json:"type"`
	ApproverRole string `json:"approver_role"`
	ApproverUser string `json:"approver_user"`
	AutoPassExpr string `json:"auto_pass_expr"`
	Timeout      string `json:"timeout"`
}

// GateResult is the outcome of a gate evaluation.
type GateResult struct {
	Status   string
	Feedback string
}

// GateKeeper evaluates approval gates for workflow steps.
type GateKeeper struct {
	events EventStore
}

// NewGateKeeper creates a GateKeeper backed by the given event store.
func NewGateKeeper(events EventStore) *GateKeeper {
	return &GateKeeper{events: events}
}

// RequestApproval evaluates the gate on a step and returns the result.
func (gk *GateKeeper) RequestApproval(ctx context.Context, run *RunState, step *StepDef, output string) GateResult {
	if step.Gate == nil {
		return GateResult{Status: "approved"}
	}

	switch step.Gate.Type {
	case "auto":
		return GateResult{Status: "approved"}

	case "human":
		stepID := step.ID
		gk.events.Emit(ctx, Event{
			TenantID:  run.TenantID,
			RunID:     run.ID,
			StepID:    &stepID,
			Type:      EventGatePending,
			ActorType: ActorEngine,
			Payload: map[string]any{
				"gate_type":     step.Gate.Type,
				"approver_role": step.Gate.ApproverRole,
				"approver_user": step.Gate.ApproverUser,
			},
		})
		return GateResult{Status: "pending"}

	case "conditional":
		return GateResult{Status: "approved"}

	default:
		return GateResult{Status: "approved"}
	}
}
