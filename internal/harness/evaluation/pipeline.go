package evaluation

import "strings"

// EscalationPolicy configures what happens when evaluation fails.
type EscalationPolicy struct {
	AfterMaxRounds string `json:"after_max_rounds"` // "human_decision","force_pass","abort"
	CriticalIssues string `json:"critical_issues"`  // "always_block","human_decision"
}

// EvalPipeline orchestrates two-track evaluation: computational first, then inferential.
type EvalPipeline struct {
	computational []Sensor
	inferential   []Sensor
	maxRounds     int
	escalation    EscalationPolicy
}

// NewEvalPipeline creates a pipeline with the given sensors and config.
func NewEvalPipeline(comp, inf []Sensor, maxRounds int, escalation EscalationPolicy) *EvalPipeline {
	if maxRounds <= 0 {
		maxRounds = 3
	}
	return &EvalPipeline{
		computational: comp,
		inferential:   inf,
		maxRounds:     maxRounds,
		escalation:    escalation,
	}
}

// RunOnce executes one round of the evaluation pipeline.
// Track 1 (computational): run all applicable sensors, collect failures.
// Track 2 (inferential): run only if Track 1 passes, stop on first failure.
func (p *EvalPipeline) RunOnce(ctx EvalContext) EvalResult {
	var allResults []SensorResult

	// Track 1: Computational (all applicable, collect failures)
	var compFeedback []string
	for _, s := range p.computational {
		if !s.Applies(ctx) {
			continue
		}
		result := s.Evaluate(ctx)
		result.Name = s.Name()
		allResults = append(allResults, result)
		if !result.Pass {
			compFeedback = append(compFeedback, result.Feedback)
		}
	}

	if len(compFeedback) > 0 {
		return EvalResult{
			Pass:     false,
			Track:    "computational",
			Feedback: strings.Join(compFeedback, "\n"),
			Results:  allResults,
		}
	}

	// Track 2: Inferential (sequential, stop on first failure)
	for _, s := range p.inferential {
		if !s.Applies(ctx) {
			continue
		}
		result := s.Evaluate(ctx)
		result.Name = s.Name()
		allResults = append(allResults, result)
		if !result.Pass {
			escalate := p.escalation.CriticalIssues == "always_block" && hasCritical(result.Issues)
			return EvalResult{
				Pass:     false,
				Track:    "inferential",
				Feedback: result.Feedback,
				Escalate: escalate,
				Results:  allResults,
			}
		}
	}

	return EvalResult{
		Pass:    true,
		Track:   "all",
		Results: allResults,
	}
}

// MaxRounds returns the configured maximum retry rounds.
func (p *EvalPipeline) MaxRounds() int {
	return p.maxRounds
}

func hasCritical(issues []Issue) bool {
	for _, i := range issues {
		if i.Severity == "critical" {
			return true
		}
	}
	return false
}
