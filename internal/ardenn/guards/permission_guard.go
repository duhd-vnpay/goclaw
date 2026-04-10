// internal/ardenn/guards/permission_guard.go
package guards

import (
	"context"

	"github.com/nextlevelbuilder/goclaw/internal/ardenn"
)

// PermissionGuard checks that the user has a required permission for the step.
// Permission comes from the step's Constraints map: {"required_permission": "can_deploy"}.
type PermissionGuard struct {
	// checker resolves whether a user has a given permission in a tenant.
	checker PermissionChecker
}

// PermissionChecker abstracts RBAC permission lookup.
type PermissionChecker interface {
	HasPermission(ctx context.Context, tenantID, userID string, permission string) bool
}

// NewPermissionGuard creates a permission guard with the given checker.
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

	if cc.UserID == nil {
		return ardenn.GuardResult{
			Name: g.Name(), Pass: false,
			Reason:   "no user context — cannot check permission",
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
