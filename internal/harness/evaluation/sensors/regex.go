package sensors

import (
	"regexp"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/harness/evaluation"
)

// RegexPattern defines a single pattern to match against output.
type RegexPattern struct {
	Regex    string `json:"regex"`
	Severity string `json:"severity"`
	Message  string `json:"message"`
}

type compiledPattern struct {
	re       *regexp.Regexp
	severity string
	message  string
}

// RegexSensor matches output against regex patterns (secrets, PCI data, etc).
type RegexSensor struct {
	name     string
	patterns []compiledPattern
}

// NewRegexSensor creates a sensor from the given patterns. Invalid regexes are skipped.
func NewRegexSensor(name string, patterns []RegexPattern) *RegexSensor {
	compiled := make([]compiledPattern, 0, len(patterns))
	for _, p := range patterns {
		re, err := regexp.Compile(p.Regex)
		if err != nil {
			continue
		}
		compiled = append(compiled, compiledPattern{re: re, severity: p.Severity, message: p.Message})
	}
	return &RegexSensor{name: name, patterns: compiled}
}

func (s *RegexSensor) Name() string                            { return s.name }
func (s *RegexSensor) Kind() evaluation.SensorKind             { return evaluation.Computational }
func (s *RegexSensor) Applies(_ evaluation.EvalContext) bool    { return true }

func (s *RegexSensor) Evaluate(ctx evaluation.EvalContext) evaluation.SensorResult {
	start := time.Now()
	var issues []evaluation.Issue

	for _, p := range s.patterns {
		if loc := p.re.FindStringIndex(ctx.Output); loc != nil {
			issues = append(issues, evaluation.Issue{
				Severity: p.severity,
				Location: "output",
				Problem:  p.message,
				Fix:      "Remove or mask the detected pattern before outputting.",
			})
		}
	}

	pass := len(issues) == 0
	feedback := ""
	if !pass {
		feedback = "Regex validation failed: " + issues[0].Problem
	}

	return evaluation.SensorResult{
		Pass: pass, Feedback: feedback, Issues: issues,
		Kind: evaluation.Computational, Duration: time.Since(start),
	}
}
