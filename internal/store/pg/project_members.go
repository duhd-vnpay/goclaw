package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGProjectMemberStore implements store.ProjectMemberStore backed by PostgreSQL.
type PGProjectMemberStore struct {
	db *sql.DB
}

// NewPGProjectMemberStore creates a new PostgreSQL-backed project member store.
func NewPGProjectMemberStore(db *sql.DB) *PGProjectMemberStore {
	return &PGProjectMemberStore{db: db}
}

func (s *PGProjectMemberStore) Add(ctx context.Context, member *store.ProjectMemberData) (*store.ProjectMemberData, error) {
	if member.ID == uuid.Nil {
		member.ID = uuid.New()
	}
	perms := member.Permissions
	if len(perms) == 0 {
		perms = json.RawMessage(`{}`)
	}
	now := time.Now()

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO project_members (id, project_id, user_id, role, permissions, joined_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (project_id, user_id) DO UPDATE SET
			role        = EXCLUDED.role,
			permissions = EXCLUDED.permissions
		RETURNING id, project_id, user_id, role, permissions, joined_at
	`, member.ID, member.ProjectID, member.UserID, member.Role, perms, now)

	return scanProjectMember(row)
}

func (s *PGProjectMemberStore) Remove(ctx context.Context, projectID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM project_members WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	return err
}

func (s *PGProjectMemberStore) List(ctx context.Context, projectID uuid.UUID) ([]store.ProjectMemberData, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, user_id, role, permissions, joined_at
		FROM project_members
		WHERE project_id = $1
		ORDER BY role, joined_at
	`, projectID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []store.ProjectMemberData
	for rows.Next() {
		m, err := scanProjectMemberFromRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *m)
	}
	return members, rows.Err()
}

func (s *PGProjectMemberStore) UpdateRole(ctx context.Context, projectID, userID uuid.UUID, role string, permissions json.RawMessage) error {
	if len(permissions) == 0 {
		permissions = json.RawMessage(`{}`)
	}
	_, err := s.db.ExecContext(ctx, `
		UPDATE project_members SET role = $1, permissions = $2
		WHERE project_id = $3 AND user_id = $4
	`, role, permissions, projectID, userID)
	return err
}

func (s *PGProjectMemberStore) GetMember(ctx context.Context, projectID, userID uuid.UUID) (*store.ProjectMemberData, error) {
	row := s.db.QueryRowContext(ctx, `
		SELECT id, project_id, user_id, role, permissions, joined_at
		FROM project_members
		WHERE project_id = $1 AND user_id = $2
	`, projectID, userID)
	return scanProjectMember(row)
}

func (s *PGProjectMemberStore) ListUserProjects(ctx context.Context, userID uuid.UUID) ([]store.ProjectMemberData, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, project_id, user_id, role, permissions, joined_at
		FROM project_members
		WHERE user_id = $1
		ORDER BY joined_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []store.ProjectMemberData
	for rows.Next() {
		m, err := scanProjectMemberFromRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *m)
	}
	return members, rows.Err()
}

// --- scan helpers ---

func scanProjectMember(row *sql.Row) (*store.ProjectMemberData, error) {
	var m store.ProjectMemberData
	err := row.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &m.Permissions, &m.JoinedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func scanProjectMemberFromRows(rows *sql.Rows) (*store.ProjectMemberData, error) {
	var m store.ProjectMemberData
	err := rows.Scan(&m.ID, &m.ProjectID, &m.UserID, &m.Role, &m.Permissions, &m.JoinedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}
