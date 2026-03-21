package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal-path.jpg", "normal-path.jpg"},
		{"path\nwith\nnewlines", "path_with_newlines"},
		{"path\rwith\rCR", "path_with_CR"},
		{"path\x00with\x01ctrl", "path_with_ctrl"},
		{"clean/path/file.png", "clean/path/file.png"},
	}
	for _, tt := range tests {
		got := sanitizePath(tt.input)
		if got != tt.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestResolveAndValidate_StaysInBase(t *testing.T) {
	base := t.TempDir()
	s := &Store{baseDir: base}

	// Create a valid file inside base
	sessionDir := filepath.Join(base, "abc123")
	os.MkdirAll(sessionDir, 0755)
	testFile := filepath.Join(sessionDir, "test-id.jpg")
	os.WriteFile(testFile, []byte("test"), 0644)

	// Should succeed — file is within baseDir
	resolved, err := s.resolveAndValidate("test-id", testFile)
	if err != nil {
		t.Fatalf("resolveAndValidate failed for valid path: %v", err)
	}
	if resolved == "" {
		t.Fatal("resolved path is empty")
	}
}

func TestResolveAndValidate_BlocksTraversal(t *testing.T) {
	base := t.TempDir()
	s := &Store{baseDir: base}

	// Create a file outside base
	outsideDir := t.TempDir()
	outsideFile := filepath.Join(outsideDir, "secret.jpg")
	os.WriteFile(outsideFile, []byte("secret"), 0644)

	// Create symlink inside base pointing outside
	sessionDir := filepath.Join(base, "abc123")
	os.MkdirAll(sessionDir, 0755)
	symlink := filepath.Join(sessionDir, "evil-id.jpg")
	err := os.Symlink(outsideFile, symlink)
	if err != nil {
		t.Skip("symlinks not supported on this OS/filesystem")
	}

	// Should fail — resolved path is outside baseDir
	_, err = s.resolveAndValidate("evil-id", symlink)
	if err == nil {
		t.Fatal("expected error for symlink traversal, got nil")
	}
}

func TestLoadPathAny_RequiresAdmin(t *testing.T) {
	base := t.TempDir()
	s, _ := NewStore(base)

	// No admin context — should fail
	ctx := context.Background()
	_, err := s.LoadPathAny("test-id", ctx)
	if err == nil {
		t.Fatal("expected error for non-admin context, got nil")
	}

	// With admin context — should return not found (no files)
	ctx = context.WithValue(ctx, AdminCtxKey, true)
	_, err = s.LoadPathAny("nonexistent", ctx)
	if err == nil {
		t.Fatal("expected not found error, got nil")
	}
}
