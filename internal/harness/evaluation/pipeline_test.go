package evaluation

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

type mockSensor struct {
	name    string
	kind    SensorKind
	applies bool
	result  SensorResult
}

func (m *mockSensor) Name() string                       { return m.name }
func (m *mockSensor) Kind() SensorKind                   { return m.kind }
func (m *mockSensor) Applies(_ EvalContext) bool          { return m.applies }
func (m *mockSensor) Evaluate(_ EvalContext) SensorResult { return m.result }

func TestEvalPipeline_ComputationalFailStopsBeforeInferential(t *testing.T) {
	compFail := &mockSensor{
		name: "comp", kind: Computational, applies: true,
		result: SensorResult{Pass: false, Feedback: "JSON invalid", Kind: Computational, Duration: time.Millisecond},
	}
	infSensor := &mockSensor{
		name: "inf", kind: Inferential, applies: true,
		result: SensorResult{Pass: true, Kind: Inferential, Duration: time.Second},
	}

	p := NewEvalPipeline([]Sensor{compFail}, []Sensor{infSensor}, 3, EscalationPolicy{})
	result := p.RunOnce(EvalContext{Output: "bad json"})

	assert.False(t, result.Pass)
	assert.Equal(t, "computational", result.Track)
	assert.Contains(t, result.Feedback, "JSON invalid")
	assert.Len(t, result.Results, 1) // inferential never ran
}

func TestEvalPipeline_AllPassReturnsPass(t *testing.T) {
	comp := &mockSensor{
		name: "comp", kind: Computational, applies: true,
		result: SensorResult{Pass: true, Kind: Computational, Duration: time.Millisecond},
	}
	inf := &mockSensor{
		name: "inf", kind: Inferential, applies: true,
		result: SensorResult{Pass: true, Kind: Inferential, Duration: time.Second},
	}

	p := NewEvalPipeline([]Sensor{comp}, []Sensor{inf}, 3, EscalationPolicy{})
	result := p.RunOnce(EvalContext{Output: "good output"})

	assert.True(t, result.Pass)
	assert.Equal(t, "all", result.Track)
	assert.Len(t, result.Results, 2)
}

func TestEvalPipeline_InferentialFailReturnsFeedback(t *testing.T) {
	comp := &mockSensor{
		name: "comp", kind: Computational, applies: true,
		result: SensorResult{Pass: true, Kind: Computational, Duration: time.Millisecond},
	}
	inf := &mockSensor{
		name: "inf", kind: Inferential, applies: true,
		result: SensorResult{Pass: false, Feedback: "Missing test plan", Kind: Inferential, Duration: time.Second},
	}

	p := NewEvalPipeline([]Sensor{comp}, []Sensor{inf}, 3, EscalationPolicy{})
	result := p.RunOnce(EvalContext{})

	assert.False(t, result.Pass)
	assert.Equal(t, "inferential", result.Track)
	assert.Contains(t, result.Feedback, "Missing test plan")
}

func TestEvalPipeline_SkipsNonApplicableSensors(t *testing.T) {
	comp := &mockSensor{name: "skip", kind: Computational, applies: false, result: SensorResult{Pass: false}}
	inf := &mockSensor{name: "inf", kind: Inferential, applies: true, result: SensorResult{Pass: true, Kind: Inferential}}

	p := NewEvalPipeline([]Sensor{comp}, []Sensor{inf}, 3, EscalationPolicy{})
	result := p.RunOnce(EvalContext{})

	assert.True(t, result.Pass)
}

func TestEvalPipeline_EscalatesOnCritical(t *testing.T) {
	comp := &mockSensor{name: "comp", kind: Computational, applies: true, result: SensorResult{Pass: true, Kind: Computational}}
	inf := &mockSensor{
		name: "inf", kind: Inferential, applies: true,
		result: SensorResult{
			Pass: false, Feedback: "Critical issue",
			Issues: []Issue{{Severity: "critical", Problem: "security breach"}},
			Kind:   Inferential,
		},
	}

	p := NewEvalPipeline([]Sensor{comp}, []Sensor{inf}, 3, EscalationPolicy{CriticalIssues: "always_block"})
	result := p.RunOnce(EvalContext{})

	assert.False(t, result.Pass)
	assert.True(t, result.Escalate)
}
