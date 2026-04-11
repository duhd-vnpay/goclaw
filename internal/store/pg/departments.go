package pg

import (
	"context"
	"database/sql"
	"encoding/json"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGDepartmentStore implements store.DepartmentStore backed by PostgreSQL.
type PGDepartmentStore struct {
	db *sql.DB
}

// NewPGDepartmentStore creates a new PostgreSQL-backed department store.
func NewPGDepartmentStore(db *sql.DB) *PGDepartmentStore {
	return &PGDepartmentStore{db: db}
}

func (s *PGDepartmentStore) Create(ctx context.Context, dept *store.DepartmentData) (*store.DepartmentData, error) {
	tid := deptTenantID(ctx)
	if dept.TenantID == uuid.Nil {
		dept.TenantID = tid
	}
	if dept.ID == uuid.Nil {
		dept.ID = uuid.New()
	}
	meta := dept.Metadata
	if len(meta) == 0 {
		meta = json.RawMessage(`{}`)
	}
	now := time.Now()

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO departments (id, tenant_id, name, slug, parent_id, head_user_id, description, metadata, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $9)
		RETURNING id, tenant_id, name, slug, parent_id, head_user_id, description, metadata, created_at, updated_at
	`, dept.ID, dept.TenantID, dept.Name, dept.Slug, dept.ParentID, dept.HeadUserID, dept.Description, meta, now)

	return scanDepartment(row)
}

func (s *PGDepartmentStore) Get(ctx context.Context, id uuid.UUID) (*store.DepartmentData, error) {
	tid := deptTenantID(ctx)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, tenant_id, name, slug, parent_id, head_user_id, description, metadata, created_at, updated_at
		FROM departments
		WHERE id = $1 AND tenant_id = $2
	`, id, tid)
	return scanDepartment(row)
}

func (s *PGDepartmentStore) List(ctx context.Context) ([]store.DepartmentData, error) {
	tid := deptTenantID(ctx)
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, tenant_id, name, slug, parent_id, head_user_id, description, metadata, created_at, updated_at
		FROM departments
		WHERE tenant_id = $1
		ORDER BY name
	`, tid)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var depts []store.DepartmentData
	for rows.Next() {
		d, err := scanDepartmentFromRows(rows)
		if err != nil {
			return nil, err
		}
		depts = append(depts, *d)
	}
	return depts, rows.Err()
}

func (s *PGDepartmentStore) Update(ctx context.Context, id uuid.UUID, updates map[string]any) error {
	tid := deptTenantID(ctx)

	// Build dynamic SET clause from allowed fields.
	allowed := map[string]string{
		"name":         "name",
		"slug":         "slug",
		"parent_id":    "parent_id",
		"head_user_id": "head_user_id",
		"description":  "description",
		"metadata":     "metadata",
	}

	setClauses := "updated_at = NOW()"
	args := []any{}
	argIdx := 1

	for key, col := range allowed {
		if val, ok := updates[key]; ok {
			setClauses += ", " + col + " = $" + itoa(argIdx)
			args = append(args, val)
			argIdx++
		}
	}

	args = append(args, id, tid)
	query := "UPDATE departments SET " + setClauses + " WHERE id = $" + itoa(argIdx) + " AND tenant_id = $" + itoa(argIdx+1)

	_, err := s.db.ExecContext(ctx, query, args...)
	return err
}

func (s *PGDepartmentStore) Delete(ctx context.Context, id uuid.UUID) error {
	tid := deptTenantID(ctx)
	_, err := s.db.ExecContext(ctx, `DELETE FROM departments WHERE id = $1 AND tenant_id = $2`, id, tid)
	return err
}

func (s *PGDepartmentStore) AddMember(ctx context.Context, member *store.DepartmentMemberData) (*store.DepartmentMemberData, error) {
	if member.ID == uuid.Nil {
		member.ID = uuid.New()
	}
	now := time.Now()

	row := s.db.QueryRowContext(ctx, `
		INSERT INTO department_members (id, department_id, user_id, role, title, created_at)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (department_id, user_id) DO UPDATE SET
			role  = EXCLUDED.role,
			title = EXCLUDED.title
		RETURNING id, department_id, user_id, role, title, created_at
	`, member.ID, member.DepartmentID, member.UserID, member.Role, member.Title, now)

	return scanDepartmentMember(row)
}

func (s *PGDepartmentStore) RemoveMember(ctx context.Context, departmentID, userID uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		DELETE FROM department_members WHERE department_id = $1 AND user_id = $2
	`, departmentID, userID)
	return err
}

func (s *PGDepartmentStore) ListMembers(ctx context.Context, departmentID uuid.UUID) ([]store.DepartmentMemberData, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, department_id, user_id, role, title, created_at
		FROM department_members
		WHERE department_id = $1
		ORDER BY role, created_at
	`, departmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []store.DepartmentMemberData
	for rows.Next() {
		m, err := scanDepartmentMemberFromRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *m)
	}
	return members, rows.Err()
}

func (s *PGDepartmentStore) ListUserDepartments(ctx context.Context, userID uuid.UUID) ([]store.DepartmentMemberData, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, department_id, user_id, role, title, created_at
		FROM department_members
		WHERE user_id = $1
		ORDER BY created_at
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var members []store.DepartmentMemberData
	for rows.Next() {
		m, err := scanDepartmentMemberFromRows(rows)
		if err != nil {
			return nil, err
		}
		members = append(members, *m)
	}
	return members, rows.Err()
}

// --- helpers ---

func deptTenantID(ctx context.Context) uuid.UUID {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		return store.MasterTenantID
	}
	return tid
}

func scanDepartment(row *sql.Row) (*store.DepartmentData, error) {
	var d store.DepartmentData
	err := row.Scan(
		&d.ID, &d.TenantID, &d.Name, &d.Slug, &d.ParentID, &d.HeadUserID,
		&d.Description, &d.Metadata, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func scanDepartmentFromRows(rows *sql.Rows) (*store.DepartmentData, error) {
	var d store.DepartmentData
	err := rows.Scan(
		&d.ID, &d.TenantID, &d.Name, &d.Slug, &d.ParentID, &d.HeadUserID,
		&d.Description, &d.Metadata, &d.CreatedAt, &d.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &d, nil
}

func scanDepartmentMember(row *sql.Row) (*store.DepartmentMemberData, error) {
	var m store.DepartmentMemberData
	err := row.Scan(&m.ID, &m.DepartmentID, &m.UserID, &m.Role, &m.Title, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

func scanDepartmentMemberFromRows(rows *sql.Rows) (*store.DepartmentMemberData, error) {
	var m store.DepartmentMemberData
	err := rows.Scan(&m.ID, &m.DepartmentID, &m.UserID, &m.Role, &m.Title, &m.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// itoa is defined in config_permissions.go — reused here.
