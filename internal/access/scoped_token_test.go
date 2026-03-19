// internal/access/scoped_token_test.go
package access

import (
	"strings"
	"testing"
	"time"
)

func TestTokenGenVerify(t *testing.T) {
	signer := NewTokenSigner("test-signing-key-must-be-32-bytes!", "", 1*time.Hour)
	mediaID := "550e8400-e29b-41d4-a716-446655440000"
	sessionHash := "a1b2c3d4e5f6"

	token, err := signer.Generate(mediaID, sessionHash)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	gotHash, err := signer.Verify(token, mediaID)
	if err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if gotHash != sessionHash {
		t.Errorf("session hash = %q, want %q", gotHash, sessionHash)
	}
}

func TestTokenExpired(t *testing.T) {
	signer := NewTokenSigner("test-signing-key-must-be-32-bytes!", "", 1*time.Millisecond)
	token, _ := signer.Generate("media1", "a1b2c3d4e5f6")
	time.Sleep(5 * time.Millisecond)

	_, err := signer.Verify(token, "media1")
	if err == nil {
		t.Fatal("expected expiry error")
	}
}

func TestTokenWrongMediaID(t *testing.T) {
	signer := NewTokenSigner("test-signing-key-must-be-32-bytes!", "", 1*time.Hour)
	token, _ := signer.Generate("media1", "a1b2c3d4e5f6")

	_, err := signer.Verify(token, "media-WRONG")
	if err == nil {
		t.Fatal("expected HMAC mismatch error")
	}
}

func TestTokenKeyRotation(t *testing.T) {
	oldKey := "old-signing-key-must-be-32-bytes!"
	newKey := "new-signing-key-must-be-32-bytes!"

	oldSigner := NewTokenSigner(oldKey, "", 1*time.Hour)
	token, _ := oldSigner.Generate("media1", "a1b2c3d4e5f6")

	newSigner := NewTokenSigner(newKey, oldKey, 1*time.Hour)
	hash, err := newSigner.Verify(token, "media1")
	if err != nil {
		t.Fatalf("rotation verify failed: %v", err)
	}
	if hash != "a1b2c3d4e5f6" {
		t.Errorf("hash = %q, want a1b2c3d4e5f6", hash)
	}
}

func TestTokenVersionPrefix(t *testing.T) {
	signer := NewTokenSigner("test-signing-key-must-be-32-bytes!", "", 1*time.Hour)
	token, _ := signer.Generate("media1", "a1b2c3d4e5f6")

	if !strings.Contains(token, ".") {
		t.Error("token must contain '.' separator")
	}
	if !IsScopedToken(token) {
		t.Error("generated token should be scoped")
	}
}

func TestIsScopedToken_GatewayToken(t *testing.T) {
	if IsScopedToken("plain-gateway-token-no-dot") {
		t.Error("gateway token should not be scoped")
	}
}

func TestKeyTooShort(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for short key")
		}
	}()
	NewTokenSigner("short", "", 1*time.Hour)
}
