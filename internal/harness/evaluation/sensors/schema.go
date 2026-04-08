package sensors

import (
	"encoding/json"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/harness/evaluation"
)

// SchemaSensor validates that JSON-like output is well-formed.
type SchemaSensor struct {
	name string
}

// NewSchemaSensor creates a JSON validation sensor.
func NewSchemaSensor(name string) *SchemaSensor {
	return &SchemaSensor{name: name}
}

func (s *SchemaSensor) Name() string                         { return s.name }
func (s *SchemaSensor) Kind() evaluation.SensorKind          { return evaluation.Computational }

func (s *SchemaSensor) Applies(_ evaluation.EvalContext) bool { return true }

func (s *SchemaSensor) Evaluate(ctx evaluation.EvalContext) evaluation.SensorResult {
	start := time.Now()
	output := ctx.Output

	if len(output) < 2 || (output[0] != '{' && output[0] != '[') {
		return evaluation.SensorResult{Pass: true, Kind: evaluation.Computational, Duration: time.Since(start)}
	}

	if !json.Valid([]byte(output)) {
		return evaluation.SensorResult{
			Pass:     false,
			Feedback: "Output appears to be JSON but is malformed. Check for trailing commas, missing quotes, or unescaped characters.",
			Issues: []evaluation.Issue{{
				Severity: "high", Location: "output",
				Problem: "Invalid JSON", Fix: "Fix JSON syntax errors",
			}},
			Kind:     evaluation.Computational,
			Duration: time.Since(start),
		}
	}

	return evaluation.SensorResult{Pass: true, Kind: evaluation.Computational, Duration: time.Since(start)}
}
