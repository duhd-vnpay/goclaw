package ardenn

import (
	"context"
	"testing"
)

func TestGateKeeper_AutoGate(t *testing.T) {
	store := &mockEventStore{}
	gk := NewGateKeeper(store)
	result := gk.RequestApproval(context.Background(), &RunState{}, &StepDef{
		Gate: &GateConfig{Type: "auto"},
	}, "output")
	if result.Status != "approved" {
		t.Errorf("auto gate should auto-approve, got %q", result.Status)
	}
}

func TestGateKeeper_HumanGate(t *testing.T) {
	store := &mockEventStore{}
	gk := NewGateKeeper(store)
	runState := &RunState{ID: [16]byte{1}}
	stepDef := &StepDef{
		ID:   [16]byte{2},
		Gate: &GateConfig{Type: "human", ApproverRole: "can_approve_gate"},
	}
	result := gk.RequestApproval(context.Background(), runState, stepDef, "output")
	if result.Status != "pending" {
		t.Errorf("human gate should be pending, got %q", result.Status)
	}
	if len(store.events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(store.events))
	}
	if store.events[0].Type != EventGatePending {
		t.Errorf("expected gate.pending event, got %q", store.events[0].Type)
	}
}

func TestGateKeeper_NoGate(t *testing.T) {
	store := &mockEventStore{}
	gk := NewGateKeeper(store)
	result := gk.RequestApproval(context.Background(), &RunState{}, &StepDef{}, "output")
	if result.Status != "approved" {
		t.Errorf("no gate should auto-approve, got %q", result.Status)
	}
}
