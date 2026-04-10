package ardenn

import (
	"context"
	"fmt"
)

// StepExecutor handles dispatch, hand invocation, and gate checks for a single step.
type StepExecutor struct {
	events EventStore
	hands  *HandRegistry
	gates  *GateKeeper
}

// Execute runs a single step within a workflow run, respecting tier-aware layers.
func (se *StepExecutor) Execute(ctx context.Context, run *RunState, step *StepDef) error {
	stepRun := run.StepRuns[step.ID]
	if stepRun == nil {
		return fmt.Errorf("step run not found for step %s", step.ID)
	}

	// L4: Resolve hand + dispatch
	handType := ResolveHandType(step.DispatchTo)
	hand, err := se.hands.Get(handType)
	if err != nil {
		se.emitStepEvent(ctx, run, step, EventStepFailed, map[string]any{
			"reason": fmt.Sprintf("hand not found: %s", err),
		})
		return nil
	}

	target := step.AgentKey
	if step.DispatchTarget != "" {
		target = step.DispatchTarget
	}

	se.emitStepEvent(ctx, run, step, EventStepDispatched, map[string]any{
		"hand_type":      string(handType),
		"hand_target":    target,
		"dispatch_count": stepRun.DispatchCount + 1,
	})

	taskInput := ResolveTemplate(step.TaskTemplate, run.Variables)
	result := hand.Execute(ctx, HandRequest{
		RunID:     run.ID,
		StepRunID: stepRun.ID,
		Name:      target,
		Input:     taskInput,
		Metadata:  stepRun.Metadata,
	})

	if result.Error != nil {
		se.emitStepEvent(ctx, run, step, EventHandFailed, map[string]any{
			"hand_type": string(handType),
			"error":     result.Error.Error(),
		})
		se.emitStepEvent(ctx, run, step, EventStepFailed, map[string]any{
			"reason":         result.Error.Error(),
			"dispatch_count": stepRun.DispatchCount + 1,
		})
		return nil
	}

	se.emitStepEvent(ctx, run, step, EventStepResult, map[string]any{
		"output":      truncate(result.Output, 10240),
		"duration_ms": result.Duration.Milliseconds(),
	})

	// Gate check (standard + full)
	if run.Tier.Has(LayerConstraints) && step.Gate != nil {
		gateResult := se.gates.RequestApproval(ctx, run, step, result.Output)
		if gateResult.Status == "pending" {
			return nil
		}
		if gateResult.Status == "rejected" {
			return nil
		}
	}

	se.emitStepEvent(ctx, run, step, EventStepCompleted, nil)
	return nil
}

func (se *StepExecutor) emitStepEvent(ctx context.Context, run *RunState, step *StepDef, eventType string, payload map[string]any) {
	stepID := step.ID
	if payload == nil {
		payload = map[string]any{}
	}
	se.events.Emit(ctx, Event{
		TenantID:  run.TenantID,
		RunID:     run.ID,
		StepID:    &stepID,
		Type:      eventType,
		ActorType: ActorEngine,
		Payload:   payload,
	})
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}
