package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// DepartmentData represents a department in the org hierarchy.
type DepartmentData struct {
	ID          uuid.UUID       `json:"id" db:"id"`
	TenantID    uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Name        string          `json:"name" db:"name"`
	Slug        string          `json:"slug" db:"slug"`
	ParentID    *uuid.UUID      `json:"parent_id,omitempty" db:"parent_id"`
	HeadUserID  *uuid.UUID      `json:"head_user_id,omitempty" db:"head_user_id"`
	Description *string         `json:"description,omitempty" db:"description"`
	Metadata    json.RawMessage `json:"metadata" db:"metadata"`
	CreatedAt   time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt   time.Time       `json:"updated_at" db:"updated_at"`
}

// DepartmentMemberData represents a user's membership in a department.
type DepartmentMemberData struct {
	ID           uuid.UUID `json:"id" db:"id"`
	DepartmentID uuid.UUID `json:"department_id" db:"department_id"`
	UserID       uuid.UUID `json:"user_id" db:"user_id"`
	Role         string    `json:"role" db:"role"`
	Title        *string   `json:"title,omitempty" db:"title"`
	CreatedAt    time.Time `json:"created_at" db:"created_at"`
}

// DepartmentStore manages departments and their members.
type DepartmentStore interface {
	// Create creates a new department.
	Create(ctx context.Context, dept *DepartmentData) (*DepartmentData, error)

	// Get returns a department by ID.
	Get(ctx context.Context, id uuid.UUID) (*DepartmentData, error)

	// List returns all departments for the current tenant.
	List(ctx context.Context) ([]DepartmentData, error)

	// Update updates a department's mutable fields.
	Update(ctx context.Context, id uuid.UUID, updates map[string]any) error

	// Delete removes a department by ID.
	Delete(ctx context.Context, id uuid.UUID) error

	// AddMember adds a user to a department with a role.
	AddMember(ctx context.Context, member *DepartmentMemberData) (*DepartmentMemberData, error)

	// RemoveMember removes a user from a department.
	RemoveMember(ctx context.Context, departmentID, userID uuid.UUID) error

	// ListMembers returns all members of a department.
	ListMembers(ctx context.Context, departmentID uuid.UUID) ([]DepartmentMemberData, error)

	// ListUserDepartments returns all departments a user belongs to.
	ListUserDepartments(ctx context.Context, userID uuid.UUID) ([]DepartmentMemberData, error)
}
