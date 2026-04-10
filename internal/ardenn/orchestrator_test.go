package ardenn

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

func TestOrchestrator_TwoStepWorkflow(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "step done"})

	executor := &StepExecutor{
		events: events,
		hands:  hands,
		gates:  NewGateKeeper(events),
	}
	projector := NewProjector(events)
	orch := NewOrchestrator(events, projector, executor)

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()
	stepB := uuid.New()

	// Setup events: create run, start it, mark both steps ready.
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunCreated,
		Payload: map[string]any{"tier": "light"},
	})
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunStarted,
		Payload: map[string]any{},
	})
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, StepID: &stepA, Type: EventStepReady,
		Payload: map[string]any{},
	})
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, StepID: &stepB, Type: EventStepReady,
		Payload: map[string]any{},
	})

	// Register step definitions.
	orch.RegisterStepDefs(runID, map[uuid.UUID]*StepDef{
		stepA: {ID: stepA, Slug: "step-a", Name: "Step A", AgentKey: "agent-a", TaskTemplate: "Do A", DispatchTo: "agent"},
		stepB: {ID: stepB, Slug: "step-b", Name: "Step B", AgentKey: "agent-b", TaskTemplate: "Do B", DispatchTo: "agent"},
	})

	// Set dependency: B depends on A.
	state, err := projector.Rebuild(context.Background(), runID)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}
	state.StepRuns[stepB].DependsOn = []uuid.UUID{stepA}

	// Process the run.
	err = orch.ProcessRunWithState(context.Background(), runID, state)
	if err != nil {
		t.Fatalf("ProcessRun failed: %v", err)
	}

	// Rebuild final state and verify.
	final, err := projector.Rebuild(context.Background(), runID)
	if err != nil {
		t.Fatalf("final Rebuild failed: %v", err)
	}

	if final.StepRuns[stepA].Status != "completed" {
		t.Errorf("step A status = %q, want completed", final.StepRuns[stepA].Status)
	}
	if final.StepRuns[stepB].Status != "completed" {
		t.Errorf("step B status = %q, want completed", final.StepRuns[stepB].Status)
	}

	var hasRunCompleted bool
	for _, e := range events.events {
		if e.Type == EventRunCompleted {
			hasRunCompleted = true
		}
	}
	if !hasRunCompleted {
		t.Error("expected run.completed event")
	}
}

func TestOrchestrator_NoStepDefs(t *testing.T) {
	events := &mockEventStore{}
	projector := NewProjector(events)
	executor := &StepExecutor{
		events: events,
		hands:  NewHandRegistry(),
		gates:  NewGateKeeper(events),
	}
	orch := NewOrchestrator(events, projector, executor)

	runID := uuid.New()
	tenantID := uuid.New()

	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunCreated,
		Payload: map[string]any{"tier": "light"},
	})
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunStarted,
		Payload: map[string]any{},
	})

	err := orch.ProcessRun(context.Background(), runID)
	if err == nil {
		t.Fatal("expected error for missing step defs")
	}
}

func TestOrchestrator_Wake(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "woke"})

	executor := &StepExecutor{
		events: events,
		hands:  hands,
		gates:  NewGateKeeper(events),
	}
	projector := NewProjector(events)
	orch := NewOrchestrator(events, projector, executor)

	runID := uuid.New()
	tenantID := uuid.New()
	stepID := uuid.New()

	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunCreated,
		Payload: map[string]any{"tier": "light"},
	})
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunStarted,
		Payload: map[string]any{},
	})
	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, StepID: &stepID, Type: EventStepReady,
		Payload: map[string]any{},
	})

	orch.RegisterStepDefs(runID, map[uuid.UUID]*StepDef{
		stepID: {ID: stepID, Slug: "only-step", Name: "Only Step", AgentKey: "agent", TaskTemplate: "Do it", DispatchTo: "agent"},
	})

	// Wake should rebuild from events and complete the run.
	err := orch.Wake(context.Background(), runID)
	if err != nil {
		t.Fatalf("Wake failed: %v", err)
	}

	final, _ := projector.Rebuild(context.Background(), runID)
	if final.StepRuns[stepID].Status != "completed" {
		t.Errorf("step status = %q, want completed", final.StepRuns[stepID].Status)
	}
}
