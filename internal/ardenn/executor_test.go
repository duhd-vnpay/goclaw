package ardenn

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
)

type mockHand struct {
	handType HandType
	output   string
	err      error
}

func (m *mockHand) Type() HandType { return m.handType }
func (m *mockHand) Execute(_ context.Context, req HandRequest) HandResult {
	return HandResult{Output: m.output, Error: m.err, Duration: 10 * time.Millisecond}
}
func (m *mockHand) Cancel(_ context.Context, _ uuid.UUID) error { return nil }

func TestExecutor_LightTier(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})

	exec := &StepExecutor{
		events: events,
		hands:  hands,
		gates:  NewGateKeeper(events),
	}

	runID := uuid.New()
	stepID := uuid.New()
	stepRunID := uuid.New()
	run := &RunState{
		ID:       runID,
		TenantID: uuid.New(),
		Tier:     TierLight,
		Status:   "running",
		Variables: map[string]any{"name": "test"},
		StepRuns: map[uuid.UUID]*StepRunState{
			stepID: {ID: stepRunID, StepID: stepID, Status: "pending"},
		},
	}

	step := &StepDef{
		ID:           stepID,
		AgentKey:     "test-agent",
		TaskTemplate: "Do {{.name}}",
		DispatchTo:   "agent",
	}

	err := exec.Execute(context.Background(), run, step)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var types []string
	for _, e := range events.events {
		types = append(types, e.Type)
	}

	want := []string{EventStepDispatched, EventStepResult, EventStepCompleted}
	if len(types) != len(want) {
		t.Fatalf("got %d events %v, want %d %v", len(types), types, len(want), want)
	}
	for i, w := range want {
		if types[i] != w {
			t.Errorf("event[%d] = %q, want %q", i, types[i], w)
		}
	}
}

func TestExecutor_StandardTier_WithGate(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})

	exec := &StepExecutor{
		events: events,
		hands:  hands,
		gates:  NewGateKeeper(events),
	}

	runID := uuid.New()
	stepID := uuid.New()
	run := &RunState{
		ID:       runID,
		TenantID: uuid.New(),
		Tier:     TierStandard,
		Status:   "running",
		Variables: map[string]any{},
		StepRuns: map[uuid.UUID]*StepRunState{
			stepID: {ID: uuid.New(), StepID: stepID, Status: "pending"},
		},
	}

	step := &StepDef{
		ID:           stepID,
		AgentKey:     "test-agent",
		TaskTemplate: "Do task",
		DispatchTo:   "agent",
		Gate:         &GateConfig{Type: "human", ApproverRole: "can_approve_gate"},
	}

	err := exec.Execute(context.Background(), run, step)
	if err != nil {
		t.Fatalf("Execute failed: %v", err)
	}

	var hasGatePending bool
	var hasStepCompleted bool
	for _, e := range events.events {
		if e.Type == EventGatePending {
			hasGatePending = true
		}
		if e.Type == EventStepCompleted {
			hasStepCompleted = true
		}
	}
	if !hasGatePending {
		t.Error("expected gate.pending event")
	}
	if hasStepCompleted {
		t.Error("step should NOT be completed while gate is pending")
	}
}
