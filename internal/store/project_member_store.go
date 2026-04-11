package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// ProjectMemberData represents a user's membership in a project.
type ProjectMemberData struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	ProjectID   uuid.UUID       `json:"project_id" db:"project_id"`
	UserID      uuid.UUID       `json:"user_id" db:"user_id"`
	Role        string          `json:"role" db:"role"`
	Permissions json.RawMessage `json:"permissions" db:"permissions"`
	JoinedAt    time.Time       `json:"joined_at" db:"joined_at"`
}

// ProjectMemberStore manages project_members (user ↔ project junction).
type ProjectMemberStore interface {
	// Add adds a user to a project with a role and permissions.
	Add(ctx context.Context, member *ProjectMemberData) (*ProjectMemberData, error)

	// Remove removes a user from a project.
	Remove(ctx context.Context, projectID, userID uuid.UUID) error

	// List returns all members of a project.
	List(ctx context.Context, projectID uuid.UUID) ([]ProjectMemberData, error)

	// UpdateRole updates a member's role and permissions.
	UpdateRole(ctx context.Context, projectID, userID uuid.UUID, role string, permissions json.RawMessage) error

	// GetMember returns a specific member's data.
	GetMember(ctx context.Context, projectID, userID uuid.UUID) (*ProjectMemberData, error)

	// ListUserProjects returns all projects a user is a member of.
	ListUserProjects(ctx context.Context, userID uuid.UUID) ([]ProjectMemberData, error)
}
