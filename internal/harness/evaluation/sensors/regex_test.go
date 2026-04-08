package sensors

import (
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/harness/evaluation"
	"github.com/stretchr/testify/assert"
)

func TestRegexSensor_DetectsPattern(t *testing.T) {
	s := NewRegexSensor("pci_scan", []RegexPattern{
		{Regex: `\b\d{13,19}\b`, Severity: "critical", Message: "Possible PAN detected"},
	})
	result := s.Evaluate(evaluation.EvalContext{Output: "Card number: 4111111111111111"})
	assert.False(t, result.Pass)
	assert.Equal(t, "critical", result.Issues[0].Severity)
}

func TestRegexSensor_PassClean(t *testing.T) {
	s := NewRegexSensor("pci_scan", []RegexPattern{
		{Regex: `\b\d{13,19}\b`, Severity: "critical", Message: "Possible PAN"},
	})
	result := s.Evaluate(evaluation.EvalContext{Output: "Payment token: tok_abc123"})
	assert.True(t, result.Pass)
}

func TestSchemaSensor_ValidJSON(t *testing.T) {
	s := NewSchemaSensor("json_check")
	result := s.Evaluate(evaluation.EvalContext{Output: `{"key": "value"}`})
	assert.True(t, result.Pass)
}

func TestSchemaSensor_InvalidJSON(t *testing.T) {
	s := NewSchemaSensor("json_check")
	result := s.Evaluate(evaluation.EvalContext{Output: `{"key": "value",}`})
	assert.False(t, result.Pass)
	assert.Contains(t, result.Feedback, "malformed")
}

func TestSchemaSensor_NonJSON(t *testing.T) {
	s := NewSchemaSensor("json_check")
	result := s.Evaluate(evaluation.EvalContext{Output: "Hello world"})
	assert.True(t, result.Pass) // non-JSON output is fine
}
