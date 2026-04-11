package store

import (
	"testing"
	"time"

	"github.com/google/uuid"
)

func TestPairingVerificationDefaults(t *testing.T) {
	v := &PairingVerification{
		UserID:      uuid.New(),
		Email:       "user@example.com",
		Code:        "123456",
		ChannelType: "telegram",
		SenderID:    "12345",
		ChatID:      "12345",
		ExpiresAt:   time.Now().Add(10 * time.Minute),
		TenantID:    MasterTenantID,
	}

	if v.Email != "user@example.com" {
		t.Errorf("Email = %q, want %q", v.Email, "user@example.com")
	}
	if v.Code != "123456" {
		t.Errorf("Code = %q, want %q", v.Code, "123456")
	}
	if v.Attempts != 0 {
		t.Errorf("Attempts = %d, want 0", v.Attempts)
	}
	if v.VerifiedAt != nil {
		t.Errorf("VerifiedAt should be nil for new verification")
	}
	if v.ExpiresAt.Before(time.Now()) {
		t.Error("ExpiresAt should be in the future")
	}
}

func TestPairingVerificationExpiry(t *testing.T) {
	// Verify that a verification created with past expiry would be considered expired.
	v := &PairingVerification{
		UserID:    uuid.New(),
		Email:     "test@test.com",
		Code:      "654321",
		ExpiresAt: time.Now().Add(-1 * time.Minute),
	}

	if !v.ExpiresAt.Before(time.Now()) {
		t.Error("Expected verification to be expired")
	}
}
