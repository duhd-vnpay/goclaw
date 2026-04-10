package ardenn

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// richMockEventStore extends mockEventStore with StepID and Limit filtering.
type richMockEventStore struct {
	events []Event
}

func (m *richMockEventStore) Emit(_ context.Context, e Event) (int64, error) {
	seq := int64(len(m.events) + 1)
	e.Sequence = seq
	e.ID = uuid.New()
	m.events = append(m.events, e)
	return seq, nil
}

func (m *richMockEventStore) GetEvents(_ context.Context, q EventQuery) ([]Event, error) {
	var result []Event
	for _, e := range m.events {
		if e.RunID != q.RunID {
			continue
		}
		if q.FromSequence > 0 && e.Sequence <= q.FromSequence {
			continue
		}
		if q.StepID != nil {
			if e.StepID == nil || *e.StepID != *q.StepID {
				continue
			}
		}
		result = append(result, e)
		if q.Limit > 0 && len(result) >= q.Limit {
			break
		}
	}
	return result, nil
}

func (m *richMockEventStore) GetLastSequence(_ context.Context, runID uuid.UUID) (int64, error) {
	var max int64
	for _, e := range m.events {
		if e.RunID == runID && e.Sequence > max {
			max = e.Sequence
		}
	}
	return max, nil
}

func TestContextBuilder_Compaction(t *testing.T) {
	ctx := context.Background()
	store := &richMockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()
	stepB := uuid.New()
	stepC := uuid.New()

	// Global event (no StepID)
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, Type: EventRunStarted, ActorType: ActorEngine, Payload: map[string]any{}})

	// Step A result
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepA, Type: EventStepCompleted, ActorType: ActorEngine, Payload: map[string]any{"output": "resultA"}})

	// Step B result
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepB, Type: EventStepCompleted, ActorType: ActorEngine, Payload: map[string]any{"output": "resultB"}})

	run := &RunState{
		ID:       runID,
		TenantID: tenantID,
		StepRuns: map[uuid.UUID]*StepRunState{
			stepC: {StepID: stepC, DependsOn: []uuid.UUID{stepA, stepB}},
		},
	}
	step := &StepDef{ID: stepC, Continuity: map[string]any{"strategy": "compaction"}}

	cb := NewContextBuilder(store)
	events, err := cb.BuildStepContext(ctx, run, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Should have: stepA event + stepB event + global event = 3
	if len(events) != 3 {
		t.Fatalf("expected 3 events, got %d", len(events))
	}

	// Verify sorted by sequence
	for i := 1; i < len(events); i++ {
		if events[i].Sequence < events[i-1].Sequence {
			t.Errorf("events not sorted: seq %d < %d at index %d", events[i].Sequence, events[i-1].Sequence, i)
		}
	}
}

func TestContextBuilder_Reset(t *testing.T) {
	ctx := context.Background()
	store := &richMockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()

	// Global event (should NOT appear in reset)
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, Type: EventRunStarted, ActorType: ActorEngine, Payload: map[string]any{}})

	// Step A's own events
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepA, Type: EventStepDispatched, ActorType: ActorEngine, Payload: map[string]any{}})
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepA, Type: EventStepResult, ActorType: ActorEngine, Payload: map[string]any{"output": "attempt1"}})

	run := &RunState{
		ID:       runID,
		TenantID: tenantID,
		StepRuns: map[uuid.UUID]*StepRunState{
			stepA: {StepID: stepA},
		},
	}
	step := &StepDef{ID: stepA, Continuity: map[string]any{"strategy": "reset"}}

	cb := NewContextBuilder(store)
	events, err := cb.BuildStepContext(ctx, run, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only step A's own events (2), no global
	if len(events) != 2 {
		t.Fatalf("expected 2 events, got %d", len(events))
	}
	for _, e := range events {
		if e.StepID == nil || *e.StepID != stepA {
			t.Errorf("reset should only return step's own events, got step_id=%v", e.StepID)
		}
	}
}

func TestContextBuilder_Adaptive_SmallEventCount(t *testing.T) {
	ctx := context.Background()
	store := &richMockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()
	stepB := uuid.New()

	// Global event
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, Type: EventRunStarted, ActorType: ActorEngine, Payload: map[string]any{}})

	// Dependency step event
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepA, Type: EventStepCompleted, ActorType: ActorEngine, Payload: map[string]any{}})

	// Total events < 100 → should use compaction (includes deps + global)
	run := &RunState{
		ID:       runID,
		TenantID: tenantID,
		StepRuns: map[uuid.UUID]*StepRunState{
			stepB: {StepID: stepB, DependsOn: []uuid.UUID{stepA}},
		},
	}
	step := &StepDef{ID: stepB, Continuity: map[string]any{"strategy": "adaptive"}}

	cb := NewContextBuilder(store)
	events, err := cb.BuildStepContext(ctx, run, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Compaction: dep event + global event = 2
	if len(events) != 2 {
		t.Fatalf("expected 2 events (compaction), got %d", len(events))
	}
}

func TestContextBuilder_Adaptive_LargeEventCount(t *testing.T) {
	ctx := context.Background()
	store := &richMockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()
	stepB := uuid.New()

	// Emit 100+ events to trigger reset path
	for i := 0; i < 105; i++ {
		store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, Type: EventRunResumed, ActorType: ActorEngine, Payload: map[string]any{}})
	}

	// Add step A dep event and step B's own event
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepA, Type: EventStepCompleted, ActorType: ActorEngine, Payload: map[string]any{}})
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, StepID: &stepB, Type: EventStepDispatched, ActorType: ActorEngine, Payload: map[string]any{}})

	run := &RunState{
		ID:       runID,
		TenantID: tenantID,
		StepRuns: map[uuid.UUID]*StepRunState{
			stepB: {StepID: stepB, DependsOn: []uuid.UUID{stepA}},
		},
	}
	step := &StepDef{ID: stepB, Continuity: map[string]any{"strategy": "adaptive"}}

	cb := NewContextBuilder(store)
	events, err := cb.BuildStepContext(ctx, run, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Reset: only stepB's own events (1)
	if len(events) != 1 {
		t.Fatalf("expected 1 event (reset), got %d", len(events))
	}
	if events[0].StepID == nil || *events[0].StepID != stepB {
		t.Errorf("expected stepB event, got step_id=%v", events[0].StepID)
	}
}

func TestContextBuilder_NoDependencies(t *testing.T) {
	ctx := context.Background()
	store := &richMockEventStore{}

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()

	// Global events
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, Type: EventRunStarted, ActorType: ActorEngine, Payload: map[string]any{}})
	store.Emit(ctx, Event{RunID: runID, TenantID: tenantID, Type: EventRunResumed, ActorType: ActorEngine, Payload: map[string]any{}})

	run := &RunState{
		ID:       runID,
		TenantID: tenantID,
		StepRuns: map[uuid.UUID]*StepRunState{
			stepA: {StepID: stepA, DependsOn: nil},
		},
	}
	step := &StepDef{ID: stepA, Continuity: map[string]any{"strategy": "compaction"}}

	cb := NewContextBuilder(store)
	events, err := cb.BuildStepContext(ctx, run, step)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Only global events (2), no dependency slices
	if len(events) != 2 {
		t.Fatalf("expected 2 global events, got %d", len(events))
	}
	for _, e := range events {
		if e.StepID != nil {
			t.Errorf("expected only global events (nil StepID), got step_id=%v", e.StepID)
		}
	}
}
