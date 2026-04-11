// internal/store/org_user_store.go
package store

import (
	"context"
	"encoding/json"
	"time"

	"github.com/google/uuid"
)

// OrgUserData represents a cached Keycloak user in the org_users table.
// The ID field is the Keycloak user UUID (not auto-generated).
type OrgUserData struct {
	ID           uuid.UUID       `json:"id" db:"id"`
	TenantID     uuid.UUID       `json:"tenant_id" db:"tenant_id"`
	Email        string          `json:"email" db:"email"`
	DisplayName  *string         `json:"display_name,omitempty" db:"display_name"`
	AvatarURL    *string         `json:"avatar_url,omitempty" db:"avatar_url"`
	AuthProvider *string         `json:"auth_provider,omitempty" db:"auth_provider"`
	Profile      json.RawMessage `json:"profile" db:"profile"`
	Status       string          `json:"status" db:"status"`
	LastLoginAt  *time.Time      `json:"last_login_at,omitempty" db:"last_login_at"`
	CreatedAt    time.Time       `json:"created_at" db:"created_at"`
	UpdatedAt    time.Time       `json:"updated_at" db:"updated_at"`
}

// OrgUserStore manages org_users (thin Keycloak cache).
type OrgUserStore interface {
	// GetByID returns a user by Keycloak UUID.
	GetByID(ctx context.Context, id uuid.UUID) (*OrgUserData, error)

	// GetByEmail returns a user by email within the current tenant scope.
	GetByEmail(ctx context.Context, email string) (*OrgUserData, error)

	// Upsert creates or updates a user (id = keycloak UUID).
	// On conflict (tenant_id, email): updates display_name, avatar_url, auth_provider, profile, last_login_at.
	Upsert(ctx context.Context, user *OrgUserData) (*OrgUserData, error)

	// List returns all org users for the current tenant, ordered by display_name.
	List(ctx context.Context) ([]OrgUserData, error)

	// UpdateStatus sets the status (active/suspended) for a user.
	UpdateStatus(ctx context.Context, id uuid.UUID, status string) error
}
