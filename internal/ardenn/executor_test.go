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

func TestRunEvalLoop_PassFirstRound(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "good output"})

	exec := &StepExecutor{events: events, hands: hands, gates: NewGateKeeper(events)}

	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "check", kind: SensorComputational, pass: true, applies: true}},
		nil, 3, EscalationPolicy{},
	)

	run := &RunState{
		ID: uuid.New(), TenantID: uuid.New(), Tier: TierFull,
		Variables: map[string]any{},
		StepRuns:  map[uuid.UUID]*StepRunState{},
	}
	stepID := uuid.New()
	run.StepRuns[stepID] = &StepRunState{ID: uuid.New(), StepID: stepID, Metadata: map[string]any{}}

	step := &StepDef{ID: stepID, AgentKey: "test", TaskTemplate: "do thing"}

	hand, _ := hands.Get(HandAgent)
	output, passed := exec.runEvalLoop(context.Background(), run, step, pipeline, "good", hand, "test", "do thing")
	if !passed {
		t.Fatal("expected pass")
	}
	if output != "good" {
		t.Errorf("output = %q, want good", output)
	}
}

func TestRunEvalLoop_FailAllRoundsAbort(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "still bad"})

	exec := &StepExecutor{events: events, hands: hands, gates: NewGateKeeper(events)}

	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "check", kind: SensorComputational, pass: false, applies: true}},
		nil, 2, EscalationPolicy{AfterMaxRounds: "abort"},
	)

	run := &RunState{
		ID: uuid.New(), TenantID: uuid.New(), Tier: TierFull,
		Variables: map[string]any{},
		StepRuns:  map[uuid.UUID]*StepRunState{},
	}
	stepID := uuid.New()
	run.StepRuns[stepID] = &StepRunState{ID: uuid.New(), StepID: stepID, Metadata: map[string]any{}}

	step := &StepDef{ID: stepID, AgentKey: "test", EvalMaxRounds: 2}

	hand, _ := hands.Get(HandAgent)
	_, passed := exec.runEvalLoop(context.Background(), run, step, pipeline, "bad", hand, "test", "do thing")
	if passed {
		t.Fatal("expected failure")
	}

	hasEscalated := false
	for _, e := range events.events {
		if e.Type == EventEvalEscalated {
			hasEscalated = true
		}
	}
	if !hasEscalated {
		t.Error("expected eval.escalated event")
	}
}

func TestExecutor_FullTier_ConstraintBlocks(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})

	constraints := NewConstraintEngine(events, &alwaysBlockGuard{name: "deny-all"})

	exec := &StepExecutor{
		events: events, hands: hands, gates: NewGateKeeper(events),
		constraints: constraints,
	}

	stepID := uuid.New()
	run := &RunState{
		ID: uuid.New(), TenantID: uuid.New(), Tier: TierFull, Status: "running",
		Variables: map[string]any{},
		StepRuns: map[uuid.UUID]*StepRunState{
			stepID: {ID: uuid.New(), StepID: stepID, Status: "pending", Metadata: map[string]any{}},
		},
	}

	step := &StepDef{ID: stepID, AgentKey: "test", TaskTemplate: "task", DispatchTo: "agent"}

	exec.Execute(context.Background(), run, step)

	hasFailed := false
	for _, e := range events.events {
		if e.Type == EventStepFailed {
			hasFailed = true
		}
		if e.Type == EventStepDispatched {
			t.Error("should not dispatch when constraint blocks")
		}
	}
	if !hasFailed {
		t.Error("expected step.failed event")
	}
}

func TestExecutor_FullTier_WithEval(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "good output"})

	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "check", kind: SensorComputational, pass: true, applies: true}},
		nil, 3, EscalationPolicy{},
	)

	exec := &StepExecutor{
		events: events, hands: hands, gates: NewGateKeeper(events),
		evalPipeline: pipeline,
	}

	stepID := uuid.New()
	run := &RunState{
		ID: uuid.New(), TenantID: uuid.New(), Tier: TierFull, Status: "running",
		Variables: map[string]any{},
		StepRuns: map[uuid.UUID]*StepRunState{
			stepID: {ID: uuid.New(), StepID: stepID, Status: "pending", Metadata: map[string]any{}},
		},
	}

	step := &StepDef{
		ID: stepID, AgentKey: "test", TaskTemplate: "task", DispatchTo: "agent",
		Evaluation: &EvalConfig{Computational: []string{"check"}},
	}

	exec.Execute(context.Background(), run, step)

	hasCompleted, hasEvalStarted := false, false
	for _, e := range events.events {
		if e.Type == EventStepCompleted {
			hasCompleted = true
		}
		if e.Type == EventEvalStarted {
			hasEvalStarted = true
		}
	}
	if !hasEvalStarted {
		t.Error("expected eval.started")
	}
	if !hasCompleted {
		t.Error("expected step.completed")
	}
}

func TestExecutor_LightTier_SkipsConstraintsAndEval(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})

	constraints := NewConstraintEngine(events, &alwaysBlockGuard{name: "deny-all"})
	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "fail", kind: SensorComputational, pass: false, applies: true}},
		nil, 3, EscalationPolicy{},
	)

	exec := &StepExecutor{
		events: events, hands: hands, gates: NewGateKeeper(events),
		constraints: constraints, evalPipeline: pipeline,
	}

	stepID := uuid.New()
	run := &RunState{
		ID: uuid.New(), TenantID: uuid.New(), Tier: TierLight, Status: "running",
		Variables: map[string]any{},
		StepRuns: map[uuid.UUID]*StepRunState{
			stepID: {ID: uuid.New(), StepID: stepID, Status: "pending", Metadata: map[string]any{}},
		},
	}

	step := &StepDef{
		ID: stepID, AgentKey: "test", TaskTemplate: "task", DispatchTo: "agent",
		Evaluation: &EvalConfig{Computational: []string{"fail"}},
	}

	exec.Execute(context.Background(), run, step)

	hasCompleted := false
	for _, e := range events.events {
		if e.Type == EventStepCompleted {
			hasCompleted = true
		}
	}
	if !hasCompleted {
		t.Error("expected step.completed — light tier should skip constraints + eval")
	}
}
