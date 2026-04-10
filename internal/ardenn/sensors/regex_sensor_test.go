// internal/ardenn/sensors/regex_sensor_test.go
package sensors

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestRegexSensor_Clean(t *testing.T) {
	s := NewRegexSensor("secrets", DefaultSecretPatterns())
	sc := ardenn.SensorContext{Output: "This is a clean output with no secrets."}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatal("expected pass for clean output")
	}
}

func TestRegexSensor_DetectsAWSKey(t *testing.T) {
	s := NewRegexSensor("secrets", DefaultSecretPatterns())
	sc := ardenn.SensorContext{Output: "Key: AKIAIOSFODNN7EXAMPLE"}
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for AWS key")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues")
	}
	if result.Issues[0].Severity != "critical" {
		t.Errorf("severity = %q, want critical", result.Issues[0].Severity)
	}
}

func TestRegexSensor_DetectsCardNumber(t *testing.T) {
	s := NewRegexSensor("pci", DefaultSecretPatterns())
	sc := ardenn.SensorContext{Output: "Card: 4111 1111 1111 1111"}
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for card number")
	}
}

func TestRegexSensor_InvalidRegexSkipped(t *testing.T) {
	s := NewRegexSensor("test", []RegexPattern{
		{Regex: "[invalid", Severity: "high", Message: "bad"},
		{Regex: "good", Severity: "low", Message: "found good"},
	})
	sc := ardenn.SensorContext{Output: "this has good text"}
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for 'good' pattern")
	}
}

func TestRegexSensor_EmptyPatterns(t *testing.T) {
	s := NewRegexSensor("empty", nil)
	if s.Applies(ardenn.SensorContext{}) {
		t.Fatal("expected Applies=false for empty patterns")
	}
}
