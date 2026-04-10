package ardenn

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type mockEventStore struct {
	events []Event
}

func (m *mockEventStore) Emit(_ context.Context, e Event) (int64, error) {
	seq := int64(len(m.events) + 1)
	e.Sequence = seq
	m.events = append(m.events, e)
	return seq, nil
}

func (m *mockEventStore) GetEvents(_ context.Context, q EventQuery) ([]Event, error) {
	var result []Event
	for _, e := range m.events {
		if e.RunID != q.RunID {
			continue
		}
		if e.Sequence <= q.FromSequence {
			continue
		}
		result = append(result, e)
	}
	return result, nil
}

func (m *mockEventStore) GetLastSequence(_ context.Context, runID uuid.UUID) (int64, error) {
	var max int64
	for _, e := range m.events {
		if e.RunID == runID && e.Sequence > max {
			max = e.Sequence
		}
	}
	return max, nil
}

func TestProjectorRebuild(t *testing.T) {
	ctx := context.Background()
	store := &mockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	wfID := uuid.New()
	stepID := uuid.New()

	// Emit 4 events: run.created, run.started, step.dispatched, step.completed
	store.Emit(ctx, Event{
		RunID:     runID,
		TenantID:  tenantID,
		Type:      EventRunCreated,
		ActorType: ActorSystem,
		Payload: map[string]any{
			"workflow_id": wfID.String(),
			"tier":        "standard",
		},
	})
	store.Emit(ctx, Event{
		RunID:     runID,
		TenantID:  tenantID,
		Type:      EventRunStarted,
		ActorType: ActorEngine,
		Payload:   map[string]any{},
	})
	store.Emit(ctx, Event{
		RunID:     runID,
		TenantID:  tenantID,
		Type:      EventStepDispatched,
		StepID:    &stepID,
		ActorType: ActorEngine,
		Payload: map[string]any{
			"hand_type":      "agent",
			"dispatch_count": float64(1),
		},
	})
	store.Emit(ctx, Event{
		RunID:     runID,
		TenantID:  tenantID,
		Type:      EventStepCompleted,
		StepID:    &stepID,
		ActorType: ActorEngine,
		Payload:   map[string]any{},
	})

	proj := NewProjector(store)
	state, err := proj.Rebuild(ctx, runID)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}

	if state.ID != runID {
		t.Fatalf("expected RunID %s, got %s", runID, state.ID)
	}
	if state.Status != "running" {
		t.Fatalf("expected status running, got %s", state.Status)
	}
	if state.Tier != TierStandard {
		t.Fatalf("expected tier standard, got %s", state.Tier)
	}
	if state.WorkflowID != wfID {
		t.Fatalf("expected WorkflowID %s, got %s", wfID, state.WorkflowID)
	}
	if state.StartedAt == nil {
		t.Fatal("expected StartedAt to be set")
	}
	if state.LastSequence != 4 {
		t.Fatalf("expected LastSequence 4, got %d", state.LastSequence)
	}

	sr, ok := state.StepRuns[stepID]
	if !ok {
		t.Fatal("step run not found")
	}
	if sr.Status != "completed" {
		t.Fatalf("expected step status completed, got %s", sr.Status)
	}
	if sr.HandType != "agent" {
		t.Fatalf("expected hand_type agent, got %s", sr.HandType)
	}
}

func TestProjectorUpdate(t *testing.T) {
	ctx := context.Background()
	store := &mockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	wfID := uuid.New()

	// Initial event
	store.Emit(ctx, Event{
		RunID:     runID,
		TenantID:  tenantID,
		Type:      EventRunCreated,
		ActorType: ActorSystem,
		Payload: map[string]any{
			"workflow_id": wfID.String(),
			"tier":        "light",
		},
	})

	proj := NewProjector(store)
	state, err := proj.Rebuild(ctx, runID)
	if err != nil {
		t.Fatalf("Rebuild failed: %v", err)
	}
	if state.Status != "pending" {
		t.Fatalf("expected pending, got %s", state.Status)
	}

	// Add more events after rebuild
	store.Emit(ctx, Event{
		RunID:     runID,
		TenantID:  tenantID,
		Type:      EventRunStarted,
		ActorType: ActorEngine,
		Payload:   map[string]any{},
	})

	// Incremental update
	if err := proj.Update(ctx, state); err != nil {
		t.Fatalf("Update failed: %v", err)
	}
	if state.Status != "running" {
		t.Fatalf("expected running after update, got %s", state.Status)
	}
	if state.LastSequence != 2 {
		t.Fatalf("expected LastSequence 2, got %d", state.LastSequence)
	}
}
