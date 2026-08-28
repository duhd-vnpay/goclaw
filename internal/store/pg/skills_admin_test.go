package pg

import (
	"context"
	"database/sql"
	"testing"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// TestUpsertSystemSkill_SelectUsestenantFilter verifies the initial SELECT
// query uses tenant_id + is_system filtering (multi-tenant isolation) and
// fetches all 5 columns (id, file_hash, file_path, description, name).
func TestUpsertSystemSkill_SelectUsesTenantFilter(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewPGSkillStore(db, t.TempDir())
	ctx := context.Background()
	slug := "test-skill"
	hash := "abc123"

	// Expect the SELECT with tenant_id and is_system filtering + 5 columns.
	mock.ExpectQuery(`SELECT id, file_hash, file_path, description, name FROM skills`).
		WithArgs(slug, store.MasterTenantID).
		WillReturnError(sql.ErrNoRows)

	// After ErrNoRows: check custom skill collision query.
	mock.ExpectQuery(`SELECT id, COALESCE`).
		WithArgs(slug, store.MasterTenantID).
		WillReturnError(sql.ErrNoRows)

	// New skill — expect INSERT with tenant_id.
	mock.ExpectExec(`INSERT INTO skills`).
		WillReturnResult(sqlmock.NewResult(1, 1))

	_, _, _, err = s.UpsertSystemSkill(ctx, store.SkillCreateParams{
		Name:     "Test Skill",
		Slug:     slug,
		Status:   "active",
		Version:  1,
		FilePath: "/skills/test-skill/1",
		FileHash: &hash,
	})
	if err != nil {
		t.Fatalf("UpsertSystemSkill() error = %v", err)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// TestUpsertSystemSkill_MetadataDriftUpdate verifies that when hash is unchanged
// but description/name differ, a metadata UPDATE is issued (the merged behavior
// from the conflict resolution: tenant filter + description/name columns).
func TestUpsertSystemSkill_MetadataDriftUpdate(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewPGSkillStore(db, t.TempDir())
	ctx := context.Background()

	existingID := uuid.New()
	slug := "drift-skill"
	hash := "same-hash"
	oldDesc := "old description"
	oldName := "Old Name"

	// SELECT returns existing row with same hash but different name/description.
	rows := sqlmock.NewRows([]string{"id", "file_hash", "file_path", "description", "name"}).
		AddRow(existingID, hash, "/skills/drift-skill/1", oldDesc, oldName)

	mock.ExpectQuery(`SELECT id, file_hash, file_path, description, name FROM skills`).
		WithArgs(slug, store.MasterTenantID).
		WillReturnRows(rows)

	// Hash matches, but name differs → expect metadata UPDATE.
	mock.ExpectExec(`UPDATE skills SET description = .*, name = .*, updated_at = NOW\(\) WHERE id = .*`).
		WithArgs(sqlmock.AnyArg(), "New Name", existingID).
		WillReturnResult(sqlmock.NewResult(0, 1))

	newDesc := "new description"
	retID, changed, filePath, err := s.UpsertSystemSkill(ctx, store.SkillCreateParams{
		Name:        "New Name",
		Slug:        slug,
		Status:      "active",
		Version:     1,
		FilePath:    "/skills/drift-skill/2", // should be ignored — hash unchanged
		FileHash:    &hash,
		Description: &newDesc,
	})
	if err != nil {
		t.Fatalf("UpsertSystemSkill() error = %v", err)
	}
	if changed {
		t.Error("expected changed=false for hash-unchanged metadata drift")
	}
	if retID != existingID {
		t.Errorf("ID = %s, want %s", retID, existingID)
	}
	if filePath != "/skills/drift-skill/1" {
		t.Errorf("filePath = %q, want existing path", filePath)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}

// TestUpsertSystemSkill_HashUnchangedNoMetadataDrift verifies no UPDATE is
// issued when hash, description, AND name all match.
func TestUpsertSystemSkill_HashUnchangedNoMetadataDrift(t *testing.T) {
	db, mock, err := sqlmock.New()
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()

	s := NewPGSkillStore(db, t.TempDir())
	ctx := context.Background()

	existingID := uuid.New()
	slug := "stable-skill"
	hash := "stable-hash"
	desc := "same desc"
	name := "Same Name"

	rows := sqlmock.NewRows([]string{"id", "file_hash", "file_path", "description", "name"}).
		AddRow(existingID, hash, "/skills/stable-skill/1", desc, name)

	mock.ExpectQuery(`SELECT id, file_hash, file_path, description, name FROM skills`).
		WithArgs(slug, store.MasterTenantID).
		WillReturnRows(rows)

	// No UPDATE expected — everything matches.

	retID, changed, filePath, err := s.UpsertSystemSkill(ctx, store.SkillCreateParams{
		Name:        name,
		Slug:        slug,
		Status:      "active",
		Version:     1,
		FilePath:    "/skills/stable-skill/2",
		FileHash:    &hash,
		Description: &desc,
	})
	if err != nil {
		t.Fatalf("UpsertSystemSkill() error = %v", err)
	}
	if changed {
		t.Error("expected changed=false")
	}
	if retID != existingID {
		t.Errorf("ID = %s, want %s", retID, existingID)
	}
	if filePath != "/skills/stable-skill/1" {
		t.Errorf("filePath = %q, want existing path", filePath)
	}

	if err := mock.ExpectationsWereMet(); err != nil {
		t.Errorf("unmet mock expectations: %v", err)
	}
}
