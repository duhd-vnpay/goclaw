package ardenn

import (
	"context"
	"fmt"
)

// StepExecutor handles dispatch, hand invocation, and gate checks for a single step.
type StepExecutor struct {
	events       EventStore
	hands        *HandRegistry
	gates        *GateKeeper
	constraints  *ConstraintEngine
	evalPipeline *ArdennEvalPipeline
}

// Execute runs a single step within a workflow run, respecting tier-aware layers.
func (se *StepExecutor) Execute(ctx context.Context, run *RunState, step *StepDef) error {
	stepRun := run.StepRuns[step.ID]
	if stepRun == nil {
		return fmt.Errorf("step run not found for step %s", step.ID)
	}

	// L1: Constraint check (standard + full tiers)
	if run.Tier.Has(LayerConstraints) && se.constraints != nil {
		cc := ConstraintContext{
			TenantID:  run.TenantID,
			RunID:     run.ID,
			StepID:    step.ID,
			AgentKey:  step.AgentKey,
			Variables: run.Variables,
			Metadata:  stepRun.Metadata,
		}
		cr := se.constraints.Check(ctx, cc)
		if cr.Blocked {
			se.emitStepEvent(ctx, run, step, EventStepFailed, map[string]any{
				"reason": "constraint blocked",
			})
			return nil
		}
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

	output := result.Output
	se.emitStepEvent(ctx, run, step, EventStepResult, map[string]any{
		"output":      truncate(output, 10240),
		"duration_ms": result.Duration.Milliseconds(),
	})

	// L3: Evaluation loop (full tier only)
	if run.Tier.Has(LayerEvaluation) && se.evalPipeline != nil && step.Evaluation != nil {
		var evalPassed bool
		output, evalPassed = se.runEvalLoop(ctx, run, step, se.evalPipeline, output, hand, target, taskInput)
		if !evalPassed {
			se.emitStepEvent(ctx, run, step, EventStepFailed, map[string]any{
				"reason": "evaluation failed after max rounds",
			})
			return nil
		}
	}

	// Gate check (standard + full)
	if run.Tier.Has(LayerConstraints) && step.Gate != nil {
		gateResult := se.gates.RequestApproval(ctx, run, step, output)
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

func (se *StepExecutor) runEvalLoop(
	ctx context.Context,
	run *RunState,
	step *StepDef,
	pipeline *ArdennEvalPipeline,
	initialOutput string,
	hand Hand,
	target string,
	taskInput string,
) (string, bool) {
	maxRounds := step.EvalMaxRounds
	if maxRounds <= 0 {
		maxRounds = pipeline.MaxRounds()
	}

	output := initialOutput
	stepID := step.ID

	se.events.Emit(ctx, Event{
		TenantID: run.TenantID, RunID: run.ID, StepID: &stepID,
		Type: EventEvalStarted, ActorType: ActorEngine,
		Payload: map[string]any{"max_rounds": maxRounds},
	})

	for round := 1; round <= maxRounds; round++ {
		sc := SensorContext{
			RunID:     run.ID,
			StepID:    step.ID,
			AgentKey:  step.AgentKey,
			Task:      taskInput,
			Output:    output,
			Variables: run.Variables,
		}

		evalResult := pipeline.RunOnce(ctx, sc, round)

		if evalResult.Pass {
			se.events.Emit(ctx, Event{
				TenantID: run.TenantID, RunID: run.ID, StepID: &stepID,
				Type: EventEvalRoundPassed, ActorType: ActorEngine,
				Payload: map[string]any{"round": round, "track": evalResult.Track},
			})
			return output, true
		}

		se.events.Emit(ctx, Event{
			TenantID: run.TenantID, RunID: run.ID, StepID: &stepID,
			Type: EventEvalRoundFailed, ActorType: ActorEngine,
			Payload: map[string]any{"round": round, "track": evalResult.Track, "feedback": evalResult.Feedback},
		})

		if evalResult.Escalate {
			se.events.Emit(ctx, Event{
				TenantID: run.TenantID, RunID: run.ID, StepID: &stepID,
				Type: EventEvalEscalated, ActorType: ActorEngine,
				Payload: map[string]any{"round": round, "reason": "critical issue detected"},
			})
			return output, false
		}

		if round < maxRounds {
			retryInput := taskInput + "\n\n## Evaluation Feedback (Round " +
				fmt.Sprintf("%d/%d", round, maxRounds) + ")\n" + evalResult.Feedback +
				"\n\nPlease fix the issues above and try again."

			stepRun := run.StepRuns[step.ID]
			retryResult := hand.Execute(ctx, HandRequest{
				RunID:     run.ID,
				StepRunID: stepRun.ID,
				Name:      target,
				Input:     retryInput,
				Metadata:  stepRun.Metadata,
			})
			if retryResult.Error != nil {
				return output, false
			}
			output = retryResult.Output
		}
	}

	se.events.Emit(ctx, Event{
		TenantID: run.TenantID, RunID: run.ID, StepID: &stepID,
		Type: EventEvalEscalated, ActorType: ActorEngine,
		Payload: map[string]any{"reason": "max rounds exhausted", "policy": pipeline.Escalation().AfterMaxRounds},
	})

	switch pipeline.Escalation().AfterMaxRounds {
	case "force_pass":
		return output, true
	default:
		return output, false
	}
}
