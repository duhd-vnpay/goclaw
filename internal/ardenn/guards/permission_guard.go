// internal/ardenn/guards/permission_guard.go
package guards

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// PermissionGuard checks that the user has a required permission for the step.
// Permission comes from the step's Constraints map: {"required_permission": "can_deploy"}.
//
// Resolution order:
//  1. If cc.UserPermissions is non-nil (Identity-integrated path), check the
//     map directly — no external lookup.
//  2. Otherwise fall back to the configured PermissionChecker (standalone).
//  3. If neither is available, fail closed.
type PermissionGuard struct {
	// checker resolves whether a user has a given permission in a tenant.
	// Optional — may be nil when guard is used in Identity-integrated mode
	// where ConstraintContext.UserPermissions carries the answer inline.
	checker PermissionChecker
}

// PermissionChecker abstracts RBAC permission lookup.
type PermissionChecker interface {
	HasPermission(ctx context.Context, tenantID, userID string, permission string) bool
}

// NewPermissionGuard creates a permission guard with the given checker.
// checker may be nil if the guard will only be used with inline
// ConstraintContext.UserPermissions (Identity-integrated path).
func NewPermissionGuard(checker PermissionChecker) *PermissionGuard {
	return &PermissionGuard{checker: checker}
}

func (g *PermissionGuard) Name() string { return "permission" }

func (g *PermissionGuard) Check(ctx context.Context, cc ardenn.ConstraintContext) ardenn.GuardResult {
	requiredPerm, ok := cc.Metadata["required_permission"].(string)
	if !ok || requiredPerm == "" {
		// No permission required — pass.
		return ardenn.GuardResult{Name: g.Name(), Pass: true, Severity: "block"}
	}

	// Identity-integrated path: permissions resolved upstream and passed
	// inline. Trust the map even if UserID is nil (e.g. system-triggered
	// runs with explicit permission grants).
	if cc.UserPermissions != nil {
		if cc.UserPermissions[requiredPerm] {
			return ardenn.GuardResult{Name: g.Name(), Pass: true, Severity: "block"}
		}
		return ardenn.GuardResult{
			Name: g.Name(), Pass: false,
			Reason:   "user lacks required permission: " + requiredPerm,
			Severity: "block",
		}
	}

	// Standalone path: fall back to checker callback.
	if cc.UserID == nil {
		return ardenn.GuardResult{
			Name: g.Name(), Pass: false,
			Reason:   "no user context — cannot check permission",
			Severity: "block",
		}
	}
	if g.checker == nil {
		return ardenn.GuardResult{
			Name: g.Name(), Pass: false,
			Reason:   "no permission checker configured and no inline permissions",
			Severity: "block",
		}
	}

	has := g.checker.HasPermission(ctx, cc.TenantID.String(), cc.UserID.String(), requiredPerm)
	if !has {
		return ardenn.GuardResult{
			Name: g.Name(), Pass: false,
			Reason:   "user lacks required permission: " + requiredPerm,
			Severity: "block",
		}
	}

	return ardenn.GuardResult{Name: g.Name(), Pass: true, Severity: "block"}
}
