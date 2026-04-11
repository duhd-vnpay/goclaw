package ardenn

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

// --- Mocks needed only for this file ---

type warnGuard struct{ name string }

func (g *warnGuard) Name() string { return g.name }
func (g *warnGuard) Check(_ context.Context, _ ConstraintContext) GuardResult {
	return GuardResult{Name: g.name, Pass: false, Reason: "warning only", Severity: "warn"}
}

// mockSensorWithIssues returns a failure with critical issues (for escalation tests).
type mockSensorWithIssues struct {
	name string
	kind SensorKind
}

func (s *mockSensorWithIssues) Name() string                 { return s.name }
func (s *mockSensorWithIssues) Kind() SensorKind             { return s.kind }
func (s *mockSensorWithIssues) Applies(_ SensorContext) bool { return true }
func (s *mockSensorWithIssues) Evaluate(_ context.Context, _ SensorContext) SensorResult {
	return SensorResult{
		Pass:     false,
		Feedback: "critical issue found",
		Kind:     s.kind,
		Issues: []EvalIssue{
			{Severity: "critical", Location: "output", Problem: "security violation", Fix: "fix it"},
		},
	}
}

// ============================================================
// State machine edge cases
// ============================================================

func TestRunStateApply_RunPaused(t *testing.T) {
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventRunCreated, Payload: map[string]any{"tier": "light"}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventRunStarted, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 3, Type: EventRunPaused, Payload: map[string]any{}})

	if state.Status != "paused" {
		t.Fatalf("expected status paused, got %s", state.Status)
	}
	if state.IsTerminal() {
		t.Fatal("paused should not be terminal")
	}
}

func TestRunStateApply_RunResumed(t *testing.T) {
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventRunCreated, Payload: map[string]any{"tier": "light"}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventRunStarted, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 3, Type: EventRunPaused, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 4, Type: EventRunResumed, Payload: map[string]any{}})

	if state.Status != "running" {
		t.Fatalf("expected status running after resume, got %s", state.Status)
	}
}

func TestRunStateApply_RunFailed(t *testing.T) {
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventRunCreated, Payload: map[string]any{"tier": "light"}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventRunStarted, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 3, Type: EventRunFailed, Payload: map[string]any{}})

	if state.Status != "failed" {
		t.Fatalf("expected status failed, got %s", state.Status)
	}
	if state.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set on failure")
	}
	if !state.IsTerminal() {
		t.Fatal("failed should be terminal")
	}
}

func TestRunStateApply_RunCancelled(t *testing.T) {
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventRunCreated, Payload: map[string]any{"tier": "light"}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventRunStarted, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 3, Type: EventRunCancelled, Payload: map[string]any{}})

	if state.Status != "cancelled" {
		t.Fatalf("expected status cancelled, got %s", state.Status)
	}
	if state.CompletedAt == nil {
		t.Fatal("expected CompletedAt to be set on cancellation")
	}
	if !state.IsTerminal() {
		t.Fatal("cancelled should be terminal")
	}
}

func TestRunStateApply_StepSkipped(t *testing.T) {
	stepID := uuid.New()
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventStepReady, StepID: &stepID, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventStepSkipped, StepID: &stepID, Payload: map[string]any{}})

	sr := state.StepRuns[stepID]
	if sr.Status != "skipped" {
		t.Fatalf("expected step status skipped, got %s", sr.Status)
	}
}

func TestRunStateApply_StepCancelled(t *testing.T) {
	stepID := uuid.New()
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventStepReady, StepID: &stepID, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventStepCancelled, StepID: &stepID, Payload: map[string]any{}})

	sr := state.StepRuns[stepID]
	if sr.Status != "cancelled" {
		t.Fatalf("expected step status cancelled, got %s", sr.Status)
	}
}

func TestRunStateApply_GateRejected(t *testing.T) {
	stepID := uuid.New()
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventStepReady, StepID: &stepID, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventGatePending, StepID: &stepID, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 3, Type: EventGateRejected, StepID: &stepID, Payload: map[string]any{}})

	sr := state.StepRuns[stepID]
	if sr.GateStatus != "rejected" {
		t.Fatalf("expected gate status rejected, got %s", sr.GateStatus)
	}
	if sr.Status != "running" {
		t.Fatalf("expected step status running after gate rejection (retry), got %s", sr.Status)
	}
}

func TestRunStateApply_GateTimedOut(t *testing.T) {
	stepID := uuid.New()
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventStepReady, StepID: &stepID, Payload: map[string]any{}})
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: EventGatePending, StepID: &stepID, Payload: map[string]any{}})
	// gate.timed_out is defined as a constant but not handled in Apply — should not panic
	state.Apply(Event{RunID: uuid.New(), Sequence: 3, Type: EventGateTimedOut, StepID: &stepID, Payload: map[string]any{}})

	// Verify the step still exists and no panic occurred
	sr := state.StepRuns[stepID]
	if sr == nil {
		t.Fatal("step run should still exist after gate.timed_out")
	}
	if sr.LastSequence != 3 {
		t.Fatalf("expected LastSequence 3, got %d", sr.LastSequence)
	}
}

func TestRunStateApply_UnknownEventType(t *testing.T) {
	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	// Unknown run-level event type should not panic
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: "run.unknown_type", Payload: map[string]any{}})

	// State should still track the sequence
	if state.LastSequence != 1 {
		t.Fatalf("expected LastSequence 1, got %d", state.LastSequence)
	}

	// Unknown step-level event type should not panic
	stepID := uuid.New()
	state.Apply(Event{RunID: uuid.New(), Sequence: 2, Type: "step.unknown_type", StepID: &stepID, Payload: map[string]any{}})

	sr := state.StepRuns[stepID]
	if sr == nil {
		t.Fatal("step run should be created even for unknown event type")
	}
}

func TestRunStateApply_MultipleSteps(t *testing.T) {
	step1 := uuid.New()
	step2 := uuid.New()
	step3 := uuid.New()

	state := &RunState{StepRuns: map[uuid.UUID]*StepRunState{}}
	state.Apply(Event{RunID: uuid.New(), Sequence: 1, Type: EventRunCreated, Payload: map[string]any{"tier": "full"}})

	// Step 1: ready → dispatched → completed
	state.Apply(Event{Sequence: 2, Type: EventStepReady, StepID: &step1, Payload: map[string]any{}})
	state.Apply(Event{Sequence: 3, Type: EventStepDispatched, StepID: &step1, Payload: map[string]any{"hand_type": "agent", "dispatch_count": float64(1)}})
	state.Apply(Event{Sequence: 4, Type: EventStepCompleted, StepID: &step1, Payload: map[string]any{}})

	// Step 2: ready → dispatched → failed
	state.Apply(Event{Sequence: 5, Type: EventStepReady, StepID: &step2, Payload: map[string]any{}})
	state.Apply(Event{Sequence: 6, Type: EventStepDispatched, StepID: &step2, Payload: map[string]any{"hand_type": "user", "dispatch_count": float64(1)}})
	state.Apply(Event{Sequence: 7, Type: EventStepFailed, StepID: &step2, Payload: map[string]any{}})

	// Step 3: ready → skipped
	state.Apply(Event{Sequence: 8, Type: EventStepReady, StepID: &step3, Payload: map[string]any{}})
	state.Apply(Event{Sequence: 9, Type: EventStepSkipped, StepID: &step3, Payload: map[string]any{}})

	if len(state.StepRuns) != 3 {
		t.Fatalf("expected 3 step runs, got %d", len(state.StepRuns))
	}
	if state.StepRuns[step1].Status != "completed" {
		t.Errorf("step1 status = %q, want completed", state.StepRuns[step1].Status)
	}
	if state.StepRuns[step1].HandType != "agent" {
		t.Errorf("step1 hand_type = %q, want agent", state.StepRuns[step1].HandType)
	}
	if state.StepRuns[step2].Status != "failed" {
		t.Errorf("step2 status = %q, want failed", state.StepRuns[step2].Status)
	}
	if state.StepRuns[step2].HandType != "user" {
		t.Errorf("step2 hand_type = %q, want user", state.StepRuns[step2].HandType)
	}
	if state.StepRuns[step3].Status != "skipped" {
		t.Errorf("step3 status = %q, want skipped", state.StepRuns[step3].Status)
	}
	if !state.HasFailedSteps() {
		t.Error("expected HasFailedSteps=true")
	}
}

// ============================================================
// Engine tests
// ============================================================

func TestEngine_NewEngine_NilHands(t *testing.T) {
	events := &mockEventStore{}
	// NewEngine with nil HandRegistry should not panic — it just means no hands registered
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("NewEngine panicked with nil hands: %v", r)
		}
	}()
	_ = NewEngine(events, nil)
}

func TestEngine_StartRun_InvalidTier(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	engine := NewEngine(events, hands)

	_, err := engine.StartRun(context.Background(), StartRunRequest{
		TenantID:   uuid.New(),
		WorkflowID: uuid.New(),
		Tier:       "invalid_tier",
		Variables:  map[string]any{},
		StepDefs:   map[uuid.UUID]*StepDef{},
	})

	if err == nil {
		t.Fatal("expected error for invalid tier")
	}
}

func TestEngine_StartRun_EmptyStepDefs(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})
	engine := NewEngine(events, hands)

	runID, err := engine.StartRun(context.Background(), StartRunRequest{
		TenantID:   uuid.New(),
		WorkflowID: uuid.New(),
		Tier:       "light",
		Variables:  map[string]any{},
		StepDefs:   map[uuid.UUID]*StepDef{},
	})

	if err != nil {
		t.Fatalf("expected no error, got %v", err)
	}
	if runID == uuid.Nil {
		t.Fatal("expected non-nil runID")
	}

	// Should have emitted at least run.created and run.started
	hasCreated, hasStarted := false, false
	for _, e := range events.events {
		if e.Type == EventRunCreated {
			hasCreated = true
		}
		if e.Type == EventRunStarted {
			hasStarted = true
		}
	}
	if !hasCreated {
		t.Error("expected run.created event")
	}
	if !hasStarted {
		t.Error("expected run.started event")
	}
}

func TestEngine_GetRunState_NonExistent(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	engine := NewEngine(events, hands)

	state, err := engine.GetRunState(context.Background(), uuid.New())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Non-existent run should return empty state (no events to replay)
	if state.Status != "" {
		t.Errorf("expected empty status for non-existent run, got %q", state.Status)
	}
	if len(state.StepRuns) != 0 {
		t.Errorf("expected 0 step runs, got %d", len(state.StepRuns))
	}
}

func TestEngine_GateDecide_EmitsEvent(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})
	eng := NewEngine(events, hands)

	runID := uuid.New()
	stepID := uuid.New()
	actorID := uuid.New()

	// Pre-populate: create and start run, register step defs
	events.Emit(context.Background(), Event{
		RunID: runID, TenantID: uuid.New(), Type: EventRunCreated,
		Payload: map[string]any{"tier": "light"},
	})
	events.Emit(context.Background(), Event{
		RunID: runID, TenantID: uuid.New(), Type: EventRunStarted,
		Payload: map[string]any{},
	})
	events.Emit(context.Background(), Event{
		RunID: runID, StepID: &stepID, Type: EventStepReady,
		Payload: map[string]any{},
	})

	// Register step defs for the orchestrator
	eng.orchestrator.RegisterStepDefs(runID, map[uuid.UUID]*StepDef{
		stepID: {ID: stepID, Slug: "gated-step", AgentKey: "agent", TaskTemplate: "task", DispatchTo: "agent"},
	})

	err := eng.GateDecide(context.Background(), runID, stepID, true, &actorID, "looks good")
	if err != nil {
		t.Fatalf("GateDecide failed: %v", err)
	}

	// Verify gate.approved event was emitted
	hasApproved := false
	for _, e := range events.events {
		if e.Type == EventGateApproved && e.StepID != nil && *e.StepID == stepID {
			hasApproved = true
			if e.Payload["decided_by"] != actorID.String() {
				t.Errorf("expected decided_by=%s, got %v", actorID, e.Payload["decided_by"])
			}
			if e.Payload["feedback"] != "looks good" {
				t.Errorf("expected feedback='looks good', got %v", e.Payload["feedback"])
			}
		}
	}
	if !hasApproved {
		t.Error("expected gate.approved event")
	}
}

// ============================================================
// Orchestrator edge cases
// ============================================================

func TestOrchestrator_ParkOnWaitingGate(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "output"})

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
		Payload: map[string]any{"tier": "standard"},
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
		stepID: {
			ID: stepID, Slug: "gated", AgentKey: "agent", TaskTemplate: "task", DispatchTo: "agent",
			Gate: &GateConfig{Type: "human", ApproverRole: "reviewer"},
		},
	})

	err := orch.ProcessRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("ProcessRun failed: %v", err)
	}

	// The run should park (not complete) because the gate is pending
	hasRunCompleted := false
	hasGatePending := false
	for _, e := range events.events {
		if e.Type == EventRunCompleted {
			hasRunCompleted = true
		}
		if e.Type == EventGatePending {
			hasGatePending = true
		}
	}
	if !hasGatePending {
		t.Error("expected gate.pending event")
	}
	if hasRunCompleted {
		t.Error("run should NOT be completed while gate is pending")
	}
}

func TestOrchestrator_AllStepsFailed(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})

	// Use a constraint engine that blocks everything
	constraints := NewConstraintEngine(events, &alwaysBlockGuard{name: "deny-all"})

	executor := &StepExecutor{
		events:      events,
		hands:       hands,
		gates:       NewGateKeeper(events),
		constraints: constraints,
	}
	projector := NewProjector(events)
	orch := NewOrchestrator(events, projector, executor)

	runID := uuid.New()
	tenantID := uuid.New()
	stepA := uuid.New()
	stepB := uuid.New()

	events.Emit(context.Background(), Event{
		TenantID: tenantID, RunID: runID, Type: EventRunCreated,
		Payload: map[string]any{"tier": "standard"},
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

	orch.RegisterStepDefs(runID, map[uuid.UUID]*StepDef{
		stepA: {ID: stepA, Slug: "step-a", AgentKey: "agent-a", TaskTemplate: "A", DispatchTo: "agent"},
		stepB: {ID: stepB, Slug: "step-b", AgentKey: "agent-b", TaskTemplate: "B", DispatchTo: "agent"},
	})

	err := orch.ProcessRun(context.Background(), runID)
	if err != nil {
		t.Fatalf("ProcessRun failed: %v", err)
	}

	// Both steps should have failed, run should emit run.failed
	hasRunFailed := false
	for _, e := range events.events {
		if e.Type == EventRunFailed {
			hasRunFailed = true
		}
	}
	if !hasRunFailed {
		t.Error("expected run.failed event when all steps fail")
	}
}

func TestOrchestrator_ParallelReadySteps(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "done"})

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

	// No dependencies — both should be ready simultaneously
	orch.RegisterStepDefs(runID, map[uuid.UUID]*StepDef{
		stepA: {ID: stepA, Slug: "step-a", AgentKey: "agent-a", TaskTemplate: "A", DispatchTo: "agent"},
		stepB: {ID: stepB, Slug: "step-b", AgentKey: "agent-b", TaskTemplate: "B", DispatchTo: "agent"},
	})

	state, _ := projector.Rebuild(context.Background(), runID)
	ready := state.GetReadySteps()
	if len(ready) != 2 {
		t.Fatalf("expected 2 ready steps, got %d", len(ready))
	}

	err := orch.ProcessRunWithState(context.Background(), runID, state)
	if err != nil {
		t.Fatalf("ProcessRun failed: %v", err)
	}

	final, _ := projector.Rebuild(context.Background(), runID)
	for _, sid := range []uuid.UUID{stepA, stepB} {
		sr := final.StepRuns[sid]
		if sr == nil || sr.Status != "completed" {
			t.Errorf("step %s status = %v, want completed", sid, sr)
		}
	}
}

// ============================================================
// Eval loop
// ============================================================

func TestRunEvalLoop_ForcePassPolicy(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "still bad"})

	exec := &StepExecutor{events: events, hands: hands, gates: NewGateKeeper(events)}

	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "check", kind: SensorComputational, pass: false, applies: true}},
		nil, 2, EscalationPolicy{AfterMaxRounds: "force_pass"},
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

	if !passed {
		t.Fatal("expected pass with force_pass escalation policy")
	}
}

func TestRunEvalLoop_EscalateOnCriticalIssue(t *testing.T) {
	events := &mockEventStore{}
	hands := NewHandRegistry()
	hands.Register(&mockHand{handType: HandAgent, output: "output"})

	exec := &StepExecutor{events: events, hands: hands, gates: NewGateKeeper(events)}

	// Use inferential sensor with critical issues to trigger escalation
	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "regex", kind: SensorComputational, pass: true, applies: true}},
		[]Sensor{&mockSensorWithIssues{name: "judge", kind: SensorInferential}},
		3, EscalationPolicy{CriticalIssues: "always_block"},
	)

	run := &RunState{
		ID: uuid.New(), TenantID: uuid.New(), Tier: TierFull,
		Variables: map[string]any{},
		StepRuns:  map[uuid.UUID]*StepRunState{},
	}
	stepID := uuid.New()
	run.StepRuns[stepID] = &StepRunState{ID: uuid.New(), StepID: stepID, Metadata: map[string]any{}}
	step := &StepDef{ID: stepID, AgentKey: "test", EvalMaxRounds: 3}

	hand, _ := hands.Get(HandAgent)
	_, passed := exec.runEvalLoop(context.Background(), run, step, pipeline, "bad code", hand, "test", "do thing")

	if passed {
		t.Fatal("expected failure with critical issue escalation")
	}

	// Verify escalation event emitted
	hasEscalated := false
	for _, e := range events.events {
		if e.Type == EventEvalEscalated {
			hasEscalated = true
		}
	}
	if !hasEscalated {
		t.Error("expected eval.escalated event for critical issue")
	}
}

// ============================================================
// Constraint edge cases
// ============================================================

func TestConstraintEngine_WarningSeverity(t *testing.T) {
	events := &mockEventStore{}
	ce := NewConstraintEngine(events, &warnGuard{name: "warn-guard"})

	cc := ConstraintContext{
		TenantID: uuid.New(), RunID: uuid.New(), StepID: uuid.New(),
		AgentKey: "test", Variables: map[string]any{},
	}

	result := ce.Check(context.Background(), cc)
	// warn severity should NOT block
	if result.Blocked {
		t.Fatal("warn severity guard should not block")
	}
	if !result.Pass {
		t.Fatal("warn severity guard should pass (only blocks on severity=block)")
	}
}

func TestConstraintEngine_EmptyGuards(t *testing.T) {
	events := &mockEventStore{}
	ce := NewConstraintEngine(events) // no guards

	cc := ConstraintContext{
		TenantID: uuid.New(), RunID: uuid.New(), StepID: uuid.New(),
		AgentKey: "test", Variables: map[string]any{},
	}

	result := ce.Check(context.Background(), cc)
	if !result.Pass {
		t.Fatal("expected pass with no guards")
	}
	if result.Blocked {
		t.Fatal("expected not blocked with no guards")
	}
	if len(result.Results) != 0 {
		t.Fatalf("expected 0 results, got %d", len(result.Results))
	}
}

// ============================================================
// Variables
// ============================================================

func TestResolveTemplate_MissingKey(t *testing.T) {
	vars := map[string]any{"name": "test"}
	// {{.unknown}} with missingkey=zero renders as "<no value>" for map[string]any
	got := ResolveTemplate("Hello {{.unknown}} world", vars)
	expected := "Hello <no value> world"
	if got != expected {
		t.Errorf("ResolveTemplate with missing key = %q, want %q", got, expected)
	}
}

func TestMergeVariables_EmptyMaps(t *testing.T) {
	merged := MergeVariables(map[string]any{}, map[string]any{})
	if len(merged) != 0 {
		t.Errorf("expected empty merged map, got %d entries", len(merged))
	}
}

func TestMergeVariables_NilMap(t *testing.T) {
	// Merging with nil maps should not panic
	defer func() {
		if r := recover(); r != nil {
			t.Fatalf("MergeVariables panicked with nil: %v", r)
		}
	}()
	merged := MergeVariables(nil, map[string]any{"a": "1"}, nil)
	if merged["a"] != "1" {
		t.Errorf("expected a=1, got %v", merged["a"])
	}
}
