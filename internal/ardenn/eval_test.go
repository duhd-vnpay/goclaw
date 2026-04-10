package ardenn

import (
	"context"
	"testing"

	"github.com/google/uuid"
)

type mockSensor struct {
	name    string
	kind    SensorKind
	pass    bool
	applies bool
}

func (s *mockSensor) Name() string                                       { return s.name }
func (s *mockSensor) Kind() SensorKind                                   { return s.kind }
func (s *mockSensor) Applies(_ SensorContext) bool                       { return s.applies }
func (s *mockSensor) Evaluate(_ context.Context, _ SensorContext) SensorResult {
	return SensorResult{Pass: s.pass, Feedback: "mock feedback", Kind: s.kind}
}

func TestArdennEvalPipeline_AllPass(t *testing.T) {
	events := &mockEventStore{}
	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "regex", kind: SensorComputational, pass: true, applies: true}},
		[]Sensor{&mockSensor{name: "judge", kind: SensorInferential, pass: true, applies: true}},
		3, EscalationPolicy{},
	)

	sc := SensorContext{RunID: uuid.New(), StepID: uuid.New(), Output: "ok"}
	result := pipeline.RunOnce(context.Background(), sc, 1)
	if !result.Pass {
		t.Fatal("expected pass")
	}
	if result.Track != "all" {
		t.Errorf("track = %q, want all", result.Track)
	}
}

func TestArdennEvalPipeline_CompFailBlocksInf(t *testing.T) {
	events := &mockEventStore{}
	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "regex", kind: SensorComputational, pass: false, applies: true}},
		[]Sensor{&mockSensor{name: "judge", kind: SensorInferential, pass: true, applies: true}},
		3, EscalationPolicy{},
	)

	sc := SensorContext{RunID: uuid.New(), StepID: uuid.New(), Output: "bad"}
	result := pipeline.RunOnce(context.Background(), sc, 1)
	if result.Pass {
		t.Fatal("expected failure")
	}
	if result.Track != "computational" {
		t.Errorf("track = %q, want computational", result.Track)
	}
	// Inferential sensor should NOT have run
	for _, r := range result.Results {
		if r.Kind == SensorInferential {
			t.Error("inferential sensor should not run when computational fails")
		}
	}
}

func TestArdennEvalPipeline_InfFailEscalates(t *testing.T) {
	events := &mockEventStore{}
	infSensor := &mockSensor{name: "judge", kind: SensorInferential, pass: false, applies: true}
	// Override evaluate to include a critical issue
	pipeline := NewArdennEvalPipeline(events,
		[]Sensor{&mockSensor{name: "regex", kind: SensorComputational, pass: true, applies: true}},
		[]Sensor{infSensor},
		3, EscalationPolicy{CriticalIssues: "always_block"},
	)

	sc := SensorContext{RunID: uuid.New(), StepID: uuid.New(), Output: "bad"}
	result := pipeline.RunOnce(context.Background(), sc, 1)
	if result.Pass {
		t.Fatal("expected failure")
	}
	if result.Track != "inferential" {
		t.Errorf("track = %q, want inferential", result.Track)
	}
}

func TestArdennEvalPipeline_DefaultMaxRounds(t *testing.T) {
	events := &mockEventStore{}
	pipeline := NewArdennEvalPipeline(events, nil, nil, 0, EscalationPolicy{})
	if pipeline.MaxRounds() != 3 {
		t.Errorf("max rounds = %d, want 3", pipeline.MaxRounds())
	}
}
