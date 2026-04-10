// internal/ardenn/eval.go
package ardenn

import (
	"context"
	"strings"
	"time"

	"github.com/google/uuid"
)

// SensorKind distinguishes fast (computational) from LLM-based (inferential) sensors.
type SensorKind string

const (
	SensorComputational SensorKind = "computational"
	SensorInferential   SensorKind = "inferential"
)

// SensorContext provides data to sensors for evaluation.
type SensorContext struct {
	RunID     uuid.UUID
	StepID    uuid.UUID
	AgentKey  string
	Task      string
	Output    string
	Variables map[string]any
}

// SensorResult is the outcome of a single sensor evaluation.
type SensorResult struct {
	Name     string        `json:"name"`
	Pass     bool          `json:"pass"`
	Score    *float64      `json:"score,omitempty"`
	Feedback string        `json:"feedback,omitempty"`
	Issues   []EvalIssue   `json:"issues,omitempty"`
	Kind     SensorKind    `json:"kind"`
	Duration time.Duration `json:"duration"`
}

// EvalIssue describes a problem found by a sensor.
type EvalIssue struct {
	Severity string `json:"severity"` // "critical","high","medium","low"
	Location string `json:"location"`
	Problem  string `json:"problem"`
	Fix      string `json:"fix"`
}

// Sensor evaluates a step's output and returns a result.
type Sensor interface {
	Name() string
	Kind() SensorKind
	Applies(sc SensorContext) bool
	Evaluate(ctx context.Context, sc SensorContext) SensorResult
}

// EvalResult is the outcome of a full pipeline round.
type EvalResult struct {
	Pass     bool           `json:"pass"`
	Round    int            `json:"round"`
	Track    string         `json:"track"` // "computational","inferential","all"
	Feedback string         `json:"feedback"`
	Escalate bool           `json:"escalate"`
	Results  []SensorResult `json:"results"`
}

// EscalationPolicy configures what happens when evaluation fails after max rounds.
type EscalationPolicy struct {
	AfterMaxRounds string `json:"after_max_rounds"` // "human_decision","force_pass","abort"
	CriticalIssues string `json:"critical_issues"`  // "always_block","human_decision"
}

// ArdennEvalPipeline orchestrates two-track evaluation with event emission.
type ArdennEvalPipeline struct {
	computational []Sensor
	inferential   []Sensor
	maxRounds     int
	escalation    EscalationPolicy
	events        EventStore
}

// NewArdennEvalPipeline creates an Ardenn evaluation pipeline.
func NewArdennEvalPipeline(events EventStore, comp, inf []Sensor, maxRounds int, escalation EscalationPolicy) *ArdennEvalPipeline {
	if maxRounds <= 0 {
		maxRounds = 3
	}
	return &ArdennEvalPipeline{
		computational: comp,
		inferential:   inf,
		maxRounds:     maxRounds,
		escalation:    escalation,
		events:        events,
	}
}

// MaxRounds returns the configured maximum retry rounds.
func (p *ArdennEvalPipeline) MaxRounds() int {
	return p.maxRounds
}

// Escalation returns the configured escalation policy.
func (p *ArdennEvalPipeline) Escalation() EscalationPolicy {
	return p.escalation
}

// RunOnce executes one round of the evaluation pipeline.
// Track 1 (computational): run all applicable sensors, collect failures.
// Track 2 (inferential): run only if Track 1 passes, stop on first failure.
func (p *ArdennEvalPipeline) RunOnce(ctx context.Context, sc SensorContext, round int) EvalResult {
	var allResults []SensorResult

	// Emit per-sensor results as events
	emitSensorResult := func(sr SensorResult) {
		stepID := sc.StepID
		payload := map[string]any{
			"sensor":   sr.Name,
			"kind":     string(sr.Kind),
			"pass":     sr.Pass,
			"feedback": sr.Feedback,
			"round":    round,
		}
		if sr.Score != nil {
			payload["score"] = *sr.Score
		}
		p.events.Emit(ctx, Event{
			RunID:     sc.RunID,
			StepID:    &stepID,
			Type:      EventEvalSensorResult,
			ActorType: ActorEngine,
			Payload:   payload,
		})
	}

	// Track 1: Computational (all applicable, collect failures)
	var compFeedback []string
	for _, s := range p.computational {
		if !s.Applies(sc) {
			continue
		}
		result := s.Evaluate(ctx, sc)
		result.Name = s.Name()
		allResults = append(allResults, result)
		emitSensorResult(result)
		if !result.Pass {
			compFeedback = append(compFeedback, result.Feedback)
		}
	}

	if len(compFeedback) > 0 {
		return EvalResult{
			Pass:     false,
			Round:    round,
			Track:    "computational",
			Feedback: strings.Join(compFeedback, "\n"),
			Results:  allResults,
		}
	}

	// Track 2: Inferential (sequential, stop on first failure)
	for _, s := range p.inferential {
		if !s.Applies(sc) {
			continue
		}
		result := s.Evaluate(ctx, sc)
		result.Name = s.Name()
		allResults = append(allResults, result)
		emitSensorResult(result)
		if !result.Pass {
			escalate := p.escalation.CriticalIssues == "always_block" && hasEvalCritical(result.Issues)
			return EvalResult{
				Pass:     false,
				Round:    round,
				Track:    "inferential",
				Feedback: result.Feedback,
				Escalate: escalate,
				Results:  allResults,
			}
		}
	}

	return EvalResult{
		Pass:    true,
		Round:   round,
		Track:   "all",
		Results: allResults,
	}
}

func hasEvalCritical(issues []EvalIssue) bool {
	for _, i := range issues {
		if i.Severity == "critical" {
			return true
		}
	}
	return false
}
