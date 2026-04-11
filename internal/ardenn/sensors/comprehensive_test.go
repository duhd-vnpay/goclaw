package sensors

import (
	"context"
	"strings"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestRegexSensor_DetectsPrivateKey(t *testing.T) {
	s := NewRegexSensor("secrets", DefaultSecretPatterns())
	sc := ardenn.SensorContext{
		Output: "Here is the key:\n-----BEGIN RSA PRIVATE KEY-----\nMIIEowIBAAK...\n-----END RSA PRIVATE KEY-----",
	}
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for private key")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues")
	}
	if result.Issues[0].Severity != "critical" {
		t.Errorf("severity = %q, want critical", result.Issues[0].Severity)
	}
	if !strings.Contains(result.Issues[0].Problem, "Private key") {
		t.Errorf("expected problem about private key, got %q", result.Issues[0].Problem)
	}
}

func TestRegexSensor_DetectsAPIKey(t *testing.T) {
	s := NewRegexSensor("secrets", DefaultSecretPatterns())
	sc := ardenn.SensorContext{
		Output: "API key: sk-abc123def456ghi789jkl012mnopqrst",
	}
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for API key (sk-)")
	}
	if len(result.Issues) == 0 {
		t.Fatal("expected issues")
	}
	if result.Issues[0].Severity != "critical" {
		t.Errorf("severity = %q, want critical", result.Issues[0].Severity)
	}
}

func TestRegexSensor_MultipleMatches(t *testing.T) {
	s := NewRegexSensor("secrets", DefaultSecretPatterns())
	sc := ardenn.SensorContext{
		Output: "AWS: AKIAIOSFODNN7EXAMPLE and card: 4111 1111 1111 1111",
	}
	result := s.Evaluate(context.Background(), sc)
	if result.Pass {
		t.Fatal("expected failure for multiple violations")
	}
	if len(result.Issues) < 2 {
		t.Fatalf("expected at least 2 issues, got %d", len(result.Issues))
	}
}

func TestSchemaSensor_NestedJSON(t *testing.T) {
	s := NewSchemaSensor("json-check")
	nested := `{"user": {"name": "Alice", "roles": ["admin", "dev"]}, "meta": {"version": 1, "tags": {"env": "prod"}}}`
	sc := ardenn.SensorContext{Output: nested}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatalf("expected pass for valid nested JSON, got feedback: %s", result.Feedback)
	}
}

func TestSchemaSensor_LargeOutput(t *testing.T) {
	s := NewSchemaSensor("json-check")
	// Build a valid JSON array with 500+ elements (well over 1KB)
	var builder strings.Builder
	builder.WriteString("[")
	for i := 0; i < 500; i++ {
		if i > 0 {
			builder.WriteString(",")
		}
		builder.WriteString(`{"id":`)
		builder.WriteString(strings.Repeat("1", 5))
		builder.WriteString(`,"name":"item`)
		builder.WriteString(strings.Repeat("a", 10))
		builder.WriteString(`"}`)
	}
	builder.WriteString("]")
	output := builder.String()

	if len(output) < 1024 {
		t.Fatalf("expected output > 1KB, got %d bytes", len(output))
	}

	sc := ardenn.SensorContext{Output: output}
	result := s.Evaluate(context.Background(), sc)
	if !result.Pass {
		t.Fatalf("expected pass for large valid JSON, got feedback: %s", result.Feedback)
	}
}
