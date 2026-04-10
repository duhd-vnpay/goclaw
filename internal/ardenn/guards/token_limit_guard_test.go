// internal/ardenn/guards/token_limit_guard_test.go
package guards

import (
	"context"
	"testing"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestTokenLimitGuard_WithinLimit(t *testing.T) {
	g := NewTokenLimitGuard(func(s string) int { return 100 })
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "short task",
		Metadata: map[string]any{"token_limit": float64(500)},
	})
	if !result.Pass {
		t.Fatal("expected pass")
	}
}

func TestTokenLimitGuard_ExceedsLimit(t *testing.T) {
	g := NewTokenLimitGuard(func(s string) int { return 1000 })
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "very long task...",
		Metadata: map[string]any{"token_limit": float64(500)},
	})
	if result.Pass {
		t.Fatal("expected failure")
	}
}

func TestTokenLimitGuard_DefaultLimit(t *testing.T) {
	g := NewTokenLimitGuard(func(s string) int { return 50 })
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "normal task",
		Metadata: map[string]any{},
	})
	if !result.Pass {
		t.Fatal("expected pass with default limit")
	}
}

func TestTokenLimitGuard_NilEstimator(t *testing.T) {
	g := NewTokenLimitGuard(nil) // should use len/4 heuristic
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "test",
		Metadata: map[string]any{"token_limit": float64(10)},
	})
	if !result.Pass {
		t.Fatal("expected pass for short string with len/4 estimator")
	}
}
