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
