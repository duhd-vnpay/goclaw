// internal/ardenn/sensors/regex_sensor.go
package sensors

import (
	"context"
	"regexp"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// RegexPattern defines a single pattern to match against output.
type RegexPattern struct {
	Regex    string `json:"regex"`
	Severity string `json:"severity"` // "critical","high","medium","low"
	Message  string `json:"message"`
}

type compiledPattern struct {
	re       *regexp.Regexp
	severity string
	message  string
}

// RegexSensor matches output against regex patterns (secrets, PCI data, API keys, etc).
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

func (s *RegexSensor) Name() string                        { return s.name }
func (s *RegexSensor) Kind() ardenn.SensorKind             { return ardenn.SensorComputational }
func (s *RegexSensor) Applies(_ ardenn.SensorContext) bool  { return len(s.patterns) > 0 }

func (s *RegexSensor) Evaluate(_ context.Context, sc ardenn.SensorContext) ardenn.SensorResult {
	start := time.Now()
	var issues []ardenn.EvalIssue

	for _, p := range s.patterns {
		if loc := p.re.FindStringIndex(sc.Output); loc != nil {
			issues = append(issues, ardenn.EvalIssue{
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

	return ardenn.SensorResult{
		Name: s.name, Pass: pass, Feedback: feedback, Issues: issues,
		Kind: ardenn.SensorComputational, Duration: time.Since(start),
	}
}

// DefaultSecretPatterns returns commonly used patterns for detecting secrets in output.
func DefaultSecretPatterns() []RegexPattern {
	return []RegexPattern{
		{Regex: `(?i)AKIA[0-9A-Z]{16}`, Severity: "critical", Message: "AWS Access Key detected"},
		{Regex: `(?i)sk-[a-zA-Z0-9]{20,}`, Severity: "critical", Message: "Potential API key (sk-) detected"},
		{Regex: `(?i)-----BEGIN (RSA |EC )?PRIVATE KEY-----`, Severity: "critical", Message: "Private key detected"},
		{Regex: `\b\d{4}[\s-]?\d{4}[\s-]?\d{4}[\s-]?\d{4}\b`, Severity: "high", Message: "Potential card number (PCI) detected"},
	}
}
