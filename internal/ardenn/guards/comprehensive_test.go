package guards

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

func TestPermissionGuard_EmptyPermission(t *testing.T) {
	g := NewPermissionGuard(&mockPermChecker{perms: map[string]bool{}})
	userID := uuid.New()
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		TenantID: uuid.New(), UserID: &userID,
		Metadata: map[string]any{"required_permission": ""},
	})
	if !result.Pass {
		t.Fatal("expected pass when required_permission is empty string")
	}
}

func TestTokenLimitGuard_ExactLimit(t *testing.T) {
	// estimator returns exactly the limit value
	g := NewTokenLimitGuard(func(s string) int { return 500 })
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "exactly at limit",
		Metadata: map[string]any{"token_limit": float64(500)},
	})
	if !result.Pass {
		t.Fatal("expected pass when tokens == limit (not exceeded)")
	}
}

func TestTokenLimitGuard_ZeroLimit(t *testing.T) {
	// When token_limit is 0 in metadata, the guard reads it as int(0).
	// The implementation uses a type switch on float64/int:
	//   case float64: limit = int(v) → limit = 0
	// With limit=0, any input > 0 tokens would fail.
	// But with limit=0 as float64(0), it gets int(0).
	// The intent here is: if someone sets limit=0, it should use default (100000).
	// Let's verify actual behavior.
	g := NewTokenLimitGuard(func(s string) int { return 50 })

	// Case 1: token_limit not in metadata → uses defaultTokenLimit (100000)
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "normal task",
		Metadata: map[string]any{},
	})
	if !result.Pass {
		t.Fatal("expected pass with default limit (100000)")
	}

	// Case 2: token_limit = 0 explicitly → limit becomes 0, so 50 > 0 → fail
	// This verifies the behavior: zero limit means everything is blocked
	result2 := g.Check(context.Background(), ardenn.ConstraintContext{
		Input:    "normal task",
		Metadata: map[string]any{"token_limit": float64(0)},
	})
	if result2.Pass {
		t.Fatal("expected failure when token_limit=0 (everything exceeds 0)")
	}
}
