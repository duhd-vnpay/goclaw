// internal/ardenn/guards/permission_guard_test.go
package guards

import (
	"context"
	"testing"

	"github.com/google/uuid"
	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

type mockPermChecker struct {
	perms map[string]bool
}

func (m *mockPermChecker) HasPermission(_ context.Context, _, _ string, perm string) bool {
	return m.perms[perm]
}

func TestPermissionGuard_NoPermRequired(t *testing.T) {
	g := NewPermissionGuard(&mockPermChecker{})
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Metadata: map[string]any{},
	})
	if !result.Pass {
		t.Fatal("expected pass when no permission required")
	}
}

func TestPermissionGuard_HasPerm(t *testing.T) {
	g := NewPermissionGuard(&mockPermChecker{perms: map[string]bool{"can_deploy": true}})
	userID := uuid.New()
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		TenantID: uuid.New(), UserID: &userID,
		Metadata: map[string]any{"required_permission": "can_deploy"},
	})
	if !result.Pass {
		t.Fatal("expected pass")
	}
}

func TestPermissionGuard_MissingPerm(t *testing.T) {
	g := NewPermissionGuard(&mockPermChecker{perms: map[string]bool{}})
	userID := uuid.New()
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		TenantID: uuid.New(), UserID: &userID,
		Metadata: map[string]any{"required_permission": "can_deploy"},
	})
	if result.Pass {
		t.Fatal("expected failure")
	}
}

func TestPermissionGuard_NoUser(t *testing.T) {
	g := NewPermissionGuard(&mockPermChecker{})
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Metadata: map[string]any{"required_permission": "can_deploy"},
	})
	if result.Pass {
		t.Fatal("expected failure when no user")
	}
}

// TestPermissionGuard_InlineUserPermissions_Allow verifies that when
// ConstraintContext.UserPermissions is populated (Identity-integrated path),
// the guard checks the map directly without touching the checker callback.
func TestPermissionGuard_InlineUserPermissions_Allow(t *testing.T) {
	// nil checker proves the inline path doesn't use it.
	g := NewPermissionGuard(nil)
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Metadata:        map[string]any{"required_permission": "can_deploy"},
		UserPermissions: map[string]bool{"can_deploy": true, "can_approve": false},
	})
	if !result.Pass {
		t.Fatalf("expected pass via inline permissions, got %+v", result)
	}
}

// TestPermissionGuard_InlineUserPermissions_Deny verifies the inline path
// blocks when the required permission is absent or false.
func TestPermissionGuard_InlineUserPermissions_Deny(t *testing.T) {
	g := NewPermissionGuard(nil)
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Metadata:        map[string]any{"required_permission": "can_deploy"},
		UserPermissions: map[string]bool{"can_view": true},
	})
	if result.Pass {
		t.Fatal("expected failure when permission missing from inline map")
	}
	if result.Severity != "block" {
		t.Errorf("severity = %q, want block", result.Severity)
	}
}

// TestPermissionGuard_InlinePathBeatsChecker verifies the inline map takes
// precedence over the checker callback (so once Identity has resolved the
// permission set, the engine never round-trips back to RBAC store).
func TestPermissionGuard_InlinePathBeatsChecker(t *testing.T) {
	// Checker would say "no" — but inline says "yes". Inline wins.
	g := NewPermissionGuard(&mockPermChecker{perms: map[string]bool{}})
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		Metadata:        map[string]any{"required_permission": "can_deploy"},
		UserPermissions: map[string]bool{"can_deploy": true},
	})
	if !result.Pass {
		t.Fatal("expected inline UserPermissions to override checker callback")
	}
}

// TestPermissionGuard_NilCheckerNoInline verifies the fail-closed branch
// when neither inline permissions nor a checker are available.
func TestPermissionGuard_NilCheckerNoInline(t *testing.T) {
	g := NewPermissionGuard(nil)
	userID := uuid.New()
	result := g.Check(context.Background(), ardenn.ConstraintContext{
		TenantID: uuid.New(), UserID: &userID,
		Metadata: map[string]any{"required_permission": "can_deploy"},
	})
	if result.Pass {
		t.Fatal("expected fail-closed when no checker and no inline permissions")
	}
}
