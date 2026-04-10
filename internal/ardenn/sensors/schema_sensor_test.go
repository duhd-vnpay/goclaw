// internal/ardenn/sensors/schema_sensor_test.go
package sensors

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestSchemaSensor_ValidJSON(t *testing.T) {
	s := NewSchemaSensor("json-check")
	sc := ardenn.SensorContext{Output: `{"key": "value", "count": 42}`}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatal("expected pass for valid JSON")
	}
}

func TestSchemaSensor_InvalidJSON(t *testing.T) {
	s := NewSchemaSensor("json-check")
	sc := ardenn.SensorContext{Output: `{"key": "value",}`} // trailing comma
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for invalid JSON")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues")
	}
}

func TestSchemaSensor_PlainText(t *testing.T) {
	s := NewSchemaSensor("json-check")
	sc := ardenn.SensorContext{Output: "Just plain text, not JSON"}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatal("expected pass for non-JSON output")
	}
}

func TestSchemaSensor_ValidArray(t *testing.T) {
	s := NewSchemaSensor("json-check")
	sc := ardenn.SensorContext{Output: `[1, 2, 3]`}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatal("expected pass for valid JSON array")
	}
}

func TestSchemaSensor_EmptyOutput(t *testing.T) {
	s := NewSchemaSensor("json-check")
	sc := ardenn.SensorContext{Output: ""}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatal("expected pass for empty output")
	}
}
