package ardenn

import (
	"testing"

	"github.com/google/uuid"
)

func TestRunStateApply_RunCreated(t *testing.T) {
	runID := uuid.New()
	tenantID := uuid.New()
	wfID := uuid.New()

	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{
		RunID:    runID,
		TenantID: tenantID,
		Sequence: 1,
		Type:     EventRunCreated,
		Payload: map[string]any{
			"workflow_id": wfID.String(),
			"tier":        "full",
			"variables":   map[string]any{"env": "prod"},
		},
	})

	if state.ID != runID {
		t.Fatalf("expected RunID %s, got %s", runID, state.ID)
	}
	if state.TenantID != tenantID {
		t.Fatalf("expected TenantID %s, got %s", tenantID, state.TenantID)
	}
	if state.Status != "pending" {
		t.Fatalf("expected status pending, got %s", state.Status)
	}
	if state.WorkflowID != wfID {
		t.Fatalf("expected WorkflowID %s, got %s", wfID, state.WorkflowID)
	}
	if state.Tier != TierFull {
		t.Fatalf("expected tier full, got %s", state.Tier)
	}
	if state.Variables["env"] != "prod" {
		t.Fatalf("expected variable env=prod, got %v", state.Variables["env"])
	}
	if state.LastSequence != 1 {
		t.Fatalf("expected LastSequence 1, got %d", state.LastSequence)
	}
}

func TestRunStateApply_StepDispatched(t *testing.T) {
	stepID := uuid.New()

	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{
		RunID:    uuid.New(),
		Sequence: 1,
		Type:     EventStepDispatched,
		StepID:   &stepID,
		Payload: map[string]any{
			"hand_type":      "agent",
			"dispatch_count": float64(2),
		},
	})

	sr, ok := state.StepRuns[stepID]
	if !ok {
		t.Fatal("step run not found")
	}
	if sr.Status != "running" {
		t.Fatalf("expected status running, got %s", sr.Status)
	}
	if sr.HandType != "agent" {
		t.Fatalf("expected hand_type agent, got %s", sr.HandType)
	}
	if sr.DispatchCount != 2 {
		t.Fatalf("expected dispatch_count 2, got %d", sr.DispatchCount)
	}
}

func TestRunStateApply_StepCompleted(t *testing.T) {
	stepID := uuid.New()

	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{
		RunID:    uuid.New(),
		Sequence: 1,
		Type:     EventStepReady,
		StepID:   &stepID,
		Payload:  map[string]any{},
	})
	state.Apply(Event{
		RunID:    uuid.New(),
		Sequence: 2,
		Type:     EventStepCompleted,
		StepID:   &stepID,
		Payload:  map[string]any{},
	})

	sr := state.StepRuns[stepID]
	if sr.Status != "completed" {
		t.Fatalf("expected status completed, got %s", sr.Status)
	}
}

func TestRunStateGetReadySteps(t *testing.T) {
	stepA := uuid.New()
	stepB := uuid.New()
	stepC := uuid.New()

	state := &RunState{
		Status: "running",
		StepRuns: map[uuid.UUID]*StepRunState{
			stepA: {StepID: stepA, Status: "completed"},
			stepB: {StepID: stepB, Status: "pending", DependsOn: []uuid.UUID{stepA}},
			stepC: {StepID: stepC, Status: "pending", DependsOn: []uuid.UUID{stepB}},
		},
	}

	ready := state.GetReadySteps()
	if len(ready) != 1 {
		t.Fatalf("expected 1 ready step, got %d", len(ready))
	}
	if ready[0] != stepB {
		t.Fatalf("expected step B (%s) to be ready, got %s", stepB, ready[0])
	}
}

func TestRunStateIsTerminal(t *testing.T) {
	cases := []struct {
		status   string
		terminal bool
	}{
		{"completed", true},
		{"failed", true},
		{"cancelled", true},
		{"running", false},
		{"pending", false},
		{"paused", false},
	}
	for _, tc := range cases {
		s := &RunState{Status: tc.status}
		if got := s.IsTerminal(); got != tc.terminal {
			t.Errorf("IsTerminal() for status %q: expected %v, got %v", tc.status, tc.terminal, got)
		}
	}
}

func TestRunStateHasFailedSteps(t *testing.T) {
	s := &RunState{
		StepRuns: map[uuid.UUID]*StepRunState{
			uuid.New(): {Status: "completed"},
			uuid.New(): {Status: "running"},
		},
	}
	if s.HasFailedSteps() {
		t.Fatal("expected no failed steps")
	}

	s.StepRuns[uuid.New()] = &StepRunState{Status: "failed"}
	if !s.HasFailedSteps() {
		t.Fatal("expected failed steps")
	}
}
