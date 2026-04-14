package store

import (
	"context"

	"github.com/google/uuid"
)

// DepartmentMembership represents a user's membership in a department,
// resolved from department_members JOIN departments.
type DepartmentMembership struct {
	DepartmentName string // e.g. "Engineering"
	Role           string // e.g. "lead", "member", "head"
	Title          string // e.g. "Backend Lead"
}

// UserProfile is the resolved identity context for a paired channel user.
// Built from org_users + department_members + departments + project_members.
// Injected into RunContext and system prompt so agents know who they're talking to.
type UserProfile struct {
	ID               uuid.UUID
	Email            string
	DisplayName      string
	TenantRole       string                // owner/admin/operator/member/viewer (from tenant_users)
	ProjectRole      string                // per current project (from project_members)
	Permissions      map[string]bool       // {can_deploy, can_approve, ...}
	Departments      []DepartmentMembership
	Expertise        []string              // from org_users.profile
	Timezone         string                // from org_users.profile
	Availability     string                // available/busy/dnd/offline
	PreferredChannel string                // from org_users.profile
}

// ProfileResolver resolves a full UserProfile from a paired channel sender.
// Implementations should cache resolved profiles in memory (30min TTL).
type ProfileResolver interface {
	// ResolveFromPairedDevice looks up the verified_user_id from paired_devices
	// for the given sender_id + channel_type, then resolves the full UserProfile
	// by joining org_users + department_members + departments.
	// Returns nil, nil if the sender is not paired (anonymous).
	// Returns nil, err only on actual DB errors (graceful degradation: callers treat as anonymous).
	ResolveFromPairedDevice(ctx context.Context, senderID, channelType string) (*UserProfile, error)

	// ResolveByUserID looks up a UserProfile by org_users.id directly.
	// Used by Ardenn engine when a workflow run is triggered by a known user
	// (no channel hop required). Returns nil, nil if the user does not exist.
	ResolveByUserID(ctx context.Context, userID uuid.UUID) (*UserProfile, error)

	// IncrementWorkload bumps the user's active_step_runs counter inside
	// org_users.profile JSONB. Called by Ardenn StepExecutor when dispatching
	// a step to a UserHand. Best-effort: errors should be logged, not fatal.
	IncrementWorkload(ctx context.Context, userID uuid.UUID) error

	// DecrementWorkload reverses IncrementWorkload. Called when a user step
	// completes, fails, or is cancelled. Best-effort.
	DecrementWorkload(ctx context.Context, userID uuid.UUID) error

	// InvalidateCache removes a cached profile for the given sender key.
	// Called when pairing status changes (pair/unpair).
	InvalidateCache(senderID, channelType string)
}
