// internal/store/pg/org_users.go
package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGOrgUserStore implements store.OrgUserStore backed by PostgreSQL.
type PGOrgUserStore struct {
	db *sql.DB
}

// NewPGOrgUserStore creates a new PostgreSQL-backed org user store.
func NewPGOrgUserStore(db *sql.DB) *PGOrgUserStore {
	return &PGOrgUserStore{db: db}
}

func (s *PGOrgUserStore) GetByID(ctx context.Context, id uuid.UUID) (*store.OrgUserData, error) {
	tid := orgUserTenantID(ctx)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, display_name, avatar_url, auth_provider,
		       profile, status, last_login_at, created_at, updated_at
		FROM org_users
		WHERE id = $1 AND tenant_id = $2
	`, id, tid)
	return scanOrgUser(row)
}

func (s *PGOrgUserStore) GetByEmail(ctx context.Context, email string) (*store.OrgUserData, error) {
	tid := orgUserTenantID(ctx)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, email, display_name, avatar_url, auth_provider,
		       profile, status, last_login_at, created_at, updated_at
		FROM org_users
		WHERE email = $1 AND tenant_id = $2
	`, email, tid)
	return scanOrgUser(row)
}

func (s *PGOrgUserStore) Upsert(ctx context.Context, user *store.OrgUserData) (*store.OrgUserData, error) {
	tid := orgUserTenantID(ctx)
	if user.TenantID == uuid.Nil {
		user.TenantID = tid
	}

	profileJSON := user.Profile
	if len(profileJSON) == 0 {
		profileJSON = json.RawMessage(`{}`)
	}
	if user.Status == "" {
		user.Status = "active"
	}
	now := time.Now()

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO org_users (id, tenant_id, email, display_name, avatar_url, auth_provider, profile, status, last_login_at, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10)
		ON CONFLICT (tenant_id, email) DO UPDATE SET
			display_name  = COALESCE(EXCLUDED.display_name, org_users.display_name),
			avatar_url    = COALESCE(EXCLUDED.avatar_url, org_users.avatar_url),
			auth_provider = COALESCE(EXCLUDED.auth_provider, org_users.auth_provider),
			profile       = EXCLUDED.profile,
			last_login_at = EXCLUDED.last_login_at,
			updated_at    = $10
		RETURNING id, tenant_id, email, display_name, avatar_url, auth_provider,
		          profile, status, last_login_at, created_at, updated_at
	`, user.ID, user.TenantID, user.Email, user.DisplayName, user.AvatarURL,
		user.AuthProvider, profileJSON, user.Status, user.LastLoginAt, now)

	return scanOrgUser(row)
}

func (s *PGOrgUserStore) List(ctx context.Context) ([]store.OrgUserData, error) {
	tid := orgUserTenantID(ctx)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, email, display_name, avatar_url, auth_provider,
		       profile, status, last_login_at, created_at, updated_at
		FROM org_users
		WHERE tenant_id = $1
		ORDER BY COALESCE(display_name, email)
	`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []store.OrgUserData
	for rows.Next() {
		u, err := scanOrgUserFromRows(rows)
		if err != nil {
			return nil, err
		}
		users = append(users, *u)
	}
	return users, rows.Err()
}

func (s *PGOrgUserStore) UpdateStatus(ctx context.Context, id uuid.UUID, status string) error {
	tid := orgUserTenantID(ctx)
	_, err := s.db.ExecContext(ctx, `
		UPDATE org_users SET status = $1, updated_at = NOW()
		WHERE id = $2 AND tenant_id = $3
	`, status, id, tid)
	return err
}

// orgUserTenantID extracts tenant_id from context. Falls back to MasterTenantID.
func orgUserTenantID(ctx context.Context) uuid.UUID {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		return store.MasterTenantID
	}
	return tid
}

// scanOrgUser scans a single row into OrgUserData.
func scanOrgUser(row *sql.Row) (*store.OrgUserData, error) {
	var u store.OrgUserData
	err := row.Scan(
		&u.ID, &u.TenantID, &u.Email, &u.DisplayName, &u.AvatarURL,
		&u.AuthProvider, &u.Profile, &u.Status, &u.LastLoginAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}

// scanOrgUserFromRows scans a row from sql.Rows into OrgUserData.
func scanOrgUserFromRows(rows *sql.Rows) (*store.OrgUserData, error) {
	var u store.OrgUserData
	err := rows.Scan(
		&u.ID, &u.TenantID, &u.Email, &u.DisplayName, &u.AvatarURL,
		&u.AuthProvider, &u.Profile, &u.Status, &u.LastLoginAt,
		&u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &u, nil
}
