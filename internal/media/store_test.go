package media

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestLoadPathScoped(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	src := filepath.Join(t.TempDir(), "test.jpg")
	os.WriteFile(src, []byte("img"), 0644)
	id, _, err := store.SaveFile("session-key-1", src, "image/jpeg")
	if err != nil {
		t.Fatal(err)
	}

	hash := store.SessionHash("session-key-1")

	// Correct hash
	path, err := store.LoadPathScoped(id, hash)
	if err != nil {
		t.Fatalf("LoadPathScoped correct hash: %v", err)
	}
	if path == "" {
		t.Fatal("returned empty path")
	}

	// Wrong hash
	_, err = store.LoadPathScoped(id, "wrong1234567")
	if err == nil {
		t.Fatal("LoadPathScoped with wrong hash should fail")
	}
}

func TestLoadPathAny_AdminOnly(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	src := filepath.Join(t.TempDir(), "test.jpg")
	os.WriteFile(src, []byte("img"), 0644)
	id, _, _ := store.SaveFile("session-key-1", src, "image/jpeg")

	// With admin context
	ctx := context.WithValue(context.Background(), AdminCtxKey, true)
	path, err := store.LoadPathAny(id, ctx)
	if err != nil {
		t.Fatalf("LoadPathAny admin: %v", err)
	}
	if path == "" {
		t.Fatal("admin returned empty path")
	}

	// Without admin context
	_, err = store.LoadPathAny(id, context.Background())
	if err == nil {
		t.Fatal("non-admin should fail")
	}
}

func TestSessionHash_Deterministic(t *testing.T) {
	store, _ := NewStore(t.TempDir())
	h1 := store.SessionHash("agent:default:telegram:direct:123")
	h2 := store.SessionHash("agent:default:telegram:direct:123")
	if h1 != h2 {
		t.Error("hash not deterministic")
	}
	if len(h1) != 12 {
		t.Errorf("hash length = %d, want 12", len(h1))
	}
}

func TestLoadPath_Unchanged(t *testing.T) {
	dir := t.TempDir()
	store, _ := NewStore(dir)

	src := filepath.Join(t.TempDir(), "test.jpg")
	os.WriteFile(src, []byte("img"), 0644)
	id, _, _ := store.SaveFile("session-key-1", src, "image/jpeg")

	// Existing LoadPath (no session hash) still works — backward compat
	path, err := store.LoadPath(id)
	if err != nil {
		t.Fatalf("LoadPath (compat): %v", err)
	}
	if path == "" {
		t.Fatal("LoadPath returned empty path")
	}
}
