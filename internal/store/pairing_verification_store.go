package store

import (
	"context"
	"time"

	"github.com/google/uuid"
)

// PairingVerification represents an email OTP verification for channel pairing.
type PairingVerification struct {
	ID          uuid.UUID  `json:"id" db:"id"`
	UserID      uuid.UUID  `json:"user_id" db:"user_id"`
	Email       string     `json:"email" db:"email"`
	Code        string     `json:"code" db:"code"`
	ChannelType string     `json:"channel_type" db:"channel_type"`
	SenderID    string     `json:"sender_id" db:"sender_id"`
	ChatID      string     `json:"chat_id" db:"chat_id"`
	Attempts    int        `json:"attempts" db:"attempts"`
	ExpiresAt   time.Time  `json:"expires_at" db:"expires_at"`
	VerifiedAt  *time.Time `json:"verified_at,omitempty" db:"verified_at"`
	TenantID    uuid.UUID  `json:"tenant_id" db:"tenant_id"`
	CreatedAt   time.Time  `json:"created_at" db:"created_at"`
}

// PairingVerificationStore manages email OTP channel pairing verifications.
type PairingVerificationStore interface {
	// Create inserts a new pairing verification record.
	Create(ctx context.Context, v *PairingVerification) error

	// GetPending returns the most recent non-expired, non-verified verification
	// for the given sender on the given channel type.
	GetPending(ctx context.Context, senderID, channelType string) (*PairingVerification, error)

	// IncrementAttempts increments the attempt counter for a verification.
	IncrementAttempts(ctx context.Context, id uuid.UUID) error

	// MarkVerified marks a verification as successfully verified.
	MarkVerified(ctx context.Context, id uuid.UUID) error

	// CountRecent counts pending (unverified) verifications for a sender within a duration window.
	// Used for rate limiting (max 3 per 5 minutes).
	CountRecent(ctx context.Context, senderID string, since time.Duration) (int, error)

	// LinkContactIdentity updates channel_contacts with the verified email and org user ID.
	LinkContactIdentity(ctx context.Context, channelType, senderID, email string, userID uuid.UUID) error

	// LinkDeviceIdentity updates paired_devices with the verified email and org user ID.
	LinkDeviceIdentity(ctx context.Context, senderID, channel, email string, userID uuid.UUID) error
}
