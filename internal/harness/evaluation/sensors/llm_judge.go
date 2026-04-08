package sensors

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/harness/evaluation"
)

// LLMClient abstracts the LLM provider for the judge sensor.
type LLMClient interface {
	Complete(prompt string, temperature float64) (string, error)
}

// LLMJudgeSensor uses a calibrated LLM to evaluate agent output against a rubric.
type LLMJudgeSensor struct {
	name        string
	rubric      evaluation.Rubric
	fewShot     []evaluation.FewShotExample
	temperature float64
	llmClient   LLMClient
}

// NewLLMJudgeSensor creates a judge sensor with rubric calibration.
func NewLLMJudgeSensor(name string, rubric evaluation.Rubric, fewShot []evaluation.FewShotExample, client LLMClient) *LLMJudgeSensor {
	return &LLMJudgeSensor{
		name:        name,
		rubric:      rubric,
		fewShot:     fewShot,
		temperature: 0.0,
		llmClient:   client,
	}
}

func (s *LLMJudgeSensor) Name() string                         { return s.name }
func (s *LLMJudgeSensor) Kind() evaluation.SensorKind          { return evaluation.Inferential }
func (s *LLMJudgeSensor) Applies(_ evaluation.EvalContext) bool { return true }

func (s *LLMJudgeSensor) Evaluate(ctx evaluation.EvalContext) evaluation.SensorResult {
	start := time.Now()

	prompt := s.buildPrompt(ctx)
	response, err := s.llmClient.Complete(prompt, s.temperature)
	if err != nil {
		return evaluation.SensorResult{
			Pass: false, Feedback: fmt.Sprintf("LLM judge error: %v", err),
			Kind: evaluation.Inferential, Duration: time.Since(start),
		}
	}

	return s.parseResponse(response, time.Since(start))
}

func (s *LLMJudgeSensor) buildPrompt(ctx evaluation.EvalContext) string {
	var b strings.Builder
	b.WriteString("You are an evaluator. Score the following output on these dimensions.\n\n## Rubric\n")

	for _, d := range s.rubric.Dimensions {
		fmt.Fprintf(&b, "### %s (weight: %.1f)\n%s\n", d.Name, d.Weight, d.Description)
		for _, a := range d.ScoreGuide {
			fmt.Fprintf(&b, "  - %.1f: %s\n", a.Score, a.Example)
		}
		b.WriteString("\n")
	}

	if len(s.fewShot) > 0 {
		b.WriteString("## Calibration Examples\n")
		for _, ex := range s.fewShot {
			fmt.Fprintf(&b, "Input: %s\nOutput: %s\nCorrect score: %.1f\nCorrect feedback: %s\n---\n",
				ex.Input, ex.Output, ex.Score, ex.Feedback)
		}
	}

	fmt.Fprintf(&b, "\n## Now evaluate:\nTask: %s\nOutput:\n%s\n\n", ctx.Task, ctx.Output)
	b.WriteString(`Respond in JSON:
{"dimensions": [{"name": "...", "score": 0.0, "reasoning": "..."}], "overall_score": 0.0, "pass": true, "issues": [{"severity": "...", "location": "...", "problem": "...", "fix": "..."}], "feedback": "..."}`)

	return b.String()
}

type judgeResponse struct {
	Dimensions []struct {
		Name      string  `json:"name"`
		Score     float64 `json:"score"`
		Reasoning string  `json:"reasoning"`
	} `json:"dimensions"`
	OverallScore float64            `json:"overall_score"`
	Pass         bool               `json:"pass"`
	Issues       []evaluation.Issue `json:"issues"`
	Feedback     string             `json:"feedback"`
}

func (s *LLMJudgeSensor) parseResponse(response string, duration time.Duration) evaluation.SensorResult {
	var jr judgeResponse
	if err := json.Unmarshal([]byte(response), &jr); err != nil {
		return evaluation.SensorResult{
			Pass: false, Feedback: "LLM judge returned unparseable response: " + err.Error(),
			Kind: evaluation.Inferential, Duration: duration,
		}
	}

	dimScores := make(map[string]float64)
	for _, d := range jr.Dimensions {
		dimScores[d.Name] = d.Score
	}
	computedScore := s.rubric.ComputeScore(dimScores)
	pass := s.rubric.Passes(computedScore)

	return evaluation.SensorResult{
		Pass:     pass,
		Score:    &computedScore,
		Feedback: jr.Feedback,
		Issues:   jr.Issues,
		Kind:     evaluation.Inferential,
		Duration: duration,
	}
}
