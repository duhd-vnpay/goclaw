// internal/ardenn/sensors/schema_sensor.go
package sensors

import (
	"context"
	"encoding/json"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// SchemaSensor validates that JSON-like output is well-formed.
// If the output doesn't look like JSON, it passes (no-op for plain text).
type SchemaSensor struct {
	name string
}

// NewSchemaSensor creates a JSON validation sensor.
func NewSchemaSensor(name string) *SchemaSensor {
	return &SchemaSensor{name: name}
}

func (s *SchemaSensor) Name() string                        { return s.name }
func (s *SchemaSensor) Kind() ardenn.SensorKind             { return ardenn.SensorComputational }
func (s *SchemaSensor) Applies(_ ardenn.SensorContext) bool  { return true }

func (s *SchemaSensor) Evaluate(_ context.Context, sc ardenn.SensorContext) ardenn.SensorResult {
	start := time.Now()
	output := sc.Output

	// Only validate if output looks like JSON (starts with { or [).
	if len(output) < 2 || (output[0] != '{' && output[0] != '[') {
		return ardenn.SensorResult{
			Name: s.name, Pass: true,
			Kind: ardenn.SensorComputational, Duration: time.Since(start),
		}
	}

	if !json.Valid([]byte(output)) {
		return ardenn.SensorResult{
			Name: s.name, Pass: false,
			Feedback: "Output appears to be JSON but is malformed. Check for trailing commas, missing quotes, or unescaped characters.",
			Issues: []ardenn.EvalIssue{{
				Severity: "high", Location: "output",
				Problem: "Invalid JSON", Fix: "Fix JSON syntax errors",
			}},
			Kind: ardenn.SensorComputational, Duration: time.Since(start),
		}
	}

	return ardenn.SensorResult{
		Name: s.name, Pass: true,
		Kind: ardenn.SensorComputational, Duration: time.Since(start),
	}
}
