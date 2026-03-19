package access

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestSafeResolvePath_AllowedPath(t *testing.T) {
	dir := t.TempDir()
	allowed := filepath.Join(dir, "media")
	os.MkdirAll(allowed, 0755)
	f := filepath.Join(allowed, "test.jpg")
	os.WriteFile(f, []byte("img"), 0644)

	resolved, err := SafeResolvePath(f, []string{allowed})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Compare with EvalSymlinks to handle OS normalization
	expected, _ := filepath.EvalSymlinks(f)
	if resolved != expected {
		t.Errorf("resolved = %q, want %q", resolved, expected)
	}
}

func TestSafeResolvePath_TraversalBlocked(t *testing.T) {
	dir := t.TempDir()
	allowed := filepath.Join(dir, "media")
	os.MkdirAll(allowed, 0755)

	_, err := SafeResolvePath(filepath.Join(allowed, "..", "secret.txt"), []string{allowed})
	if err == nil {
		t.Fatal("expected error for path traversal")
	}
}

func TestSafeResolvePath_SymlinkBlocked(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("symlink creation requires elevated privileges on Windows")
	}
	dir := t.TempDir()
	allowed := filepath.Join(dir, "media")
	outside := filepath.Join(dir, "secret")
	os.MkdirAll(allowed, 0755)
	os.MkdirAll(outside, 0755)
	os.WriteFile(filepath.Join(outside, "key.pem"), []byte("secret"), 0644)

	os.Symlink(filepath.Join(outside, "key.pem"), filepath.Join(allowed, "link.pem"))

	_, err := SafeResolvePath(filepath.Join(allowed, "link.pem"), []string{allowed})
	if err == nil {
		t.Fatal("expected error for symlink escape")
	}
}

func TestSafeResolvePath_NoMatch(t *testing.T) {
	_, err := SafeResolvePath("/etc/passwd", []string{"/app/workspace"})
	if err == nil {
		t.Fatal("expected error for disallowed path")
	}
}
