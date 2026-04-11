package pg

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

// PGPairingVerificationStore implements store.PairingVerificationStore backed by PostgreSQL.
type PGPairingVerificationStore struct {
	db *sql.DB
}

// NewPGPairingVerificationStore creates a new PostgreSQL-backed pairing verification store.
func NewPGPairingVerificationStore(db *sql.DB) *PGPairingVerificationStore {
	return &PGPairingVerificationStore{db: db}
}

func (s *PGPairingVerificationStore) Create(ctx context.Context, v *store.PairingVerification) error {
	tid := pairingVerifTenantID(ctx)
	if v.ID == uuid.Nil {
		v.ID = uuid.Must(uuid.NewV7())
	}
	v.TenantID = tid

	_, err := s.db.ExecContext(ctx, `
		INSERT INTO pairing_verifications
			(id, user_id, email, code, channel_type, sender_id, chat_id, attempts, expires_at, tenant_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)`,
		v.ID, v.UserID, v.Email, v.Code, v.ChannelType, v.SenderID, v.ChatID,
		v.Attempts, v.ExpiresAt, v.TenantID,
	)
	if err != nil {
		return fmt.Errorf("create pairing verification: %w", err)
	}
	return nil
}

func (s *PGPairingVerificationStore) GetPending(ctx context.Context, senderID, channelType string) (*store.PairingVerification, error) {
	tid := pairingVerifTenantID(ctx)
	row := s.db.QueryRowContext(ctx, `
		SELECT id, user_id, email, code, channel_type, sender_id, chat_id,
		       attempts, expires_at, verified_at, tenant_id, created_at
		FROM pairing_verifications
		WHERE sender_id = $1
		  AND channel_type = $2
		  AND tenant_id = $3
		  AND verified_at IS NULL
		  AND expires_at > NOW()
		ORDER BY created_at DESC
		LIMIT 1`,
		senderID, channelType, tid,
	)
	return scanPairingVerification(row)
}

func (s *PGPairingVerificationStore) IncrementAttempts(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE pairing_verifications
		SET attempts = attempts + 1
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("increment pairing attempts: %w", err)
	}
	return nil
}

func (s *PGPairingVerificationStore) MarkVerified(ctx context.Context, id uuid.UUID) error {
	_, err := s.db.ExecContext(ctx, `
		UPDATE pairing_verifications
		SET verified_at = NOW()
		WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("mark pairing verified: %w", err)
	}
	return nil
}

func (s *PGPairingVerificationStore) CountRecent(ctx context.Context, senderID string, since time.Duration) (int, error) {
	cutoff := time.Now().Add(-since)
	var count int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*)
		FROM pairing_verifications
		WHERE sender_id = $1
		  AND created_at > $2
		  AND verified_at IS NULL`,
		senderID, cutoff,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("count recent pairing verifications: %w", err)
	}
	return count, nil
}

func (s *PGPairingVerificationStore) LinkContactIdentity(ctx context.Context, channelType, senderID, email string, userID uuid.UUID) error {
	tid := pairingVerifTenantID(ctx)
	_, err := s.db.ExecContext(ctx, `
		UPDATE channel_contacts
		SET email = $1, verified_user_id = $2
		WHERE channel_type = $3 AND sender_id = $4 AND tenant_id = $5`,
		email, userID, channelType, senderID, tid,
	)
	if err != nil {
		return fmt.Errorf("link contact identity: %w", err)
	}
	return nil
}

func (s *PGPairingVerificationStore) LinkDeviceIdentity(ctx context.Context, senderID, channel, email string, userID uuid.UUID) error {
	tid := pairingVerifTenantID(ctx)
	_, err := s.db.ExecContext(ctx, `
		UPDATE paired_devices
		SET email = $1, verified_user_id = $2
		WHERE sender_id = $3 AND channel = $4 AND tenant_id = $5`,
		email, userID, senderID, channel, tid,
	)
	if err != nil {
		return fmt.Errorf("link device identity: %w", err)
	}
	return nil
}

// pairingVerifTenantID extracts tenant_id from context, falls back to MasterTenantID.
func pairingVerifTenantID(ctx context.Context) uuid.UUID {
	tid := store.TenantIDFromContext(ctx)
	if tid == uuid.Nil {
		return store.MasterTenantID
	}
	return tid
}

// scanPairingVerification scans a single row into PairingVerification.
func scanPairingVerification(row *sql.Row) (*store.PairingVerification, error) {
	var v store.PairingVerification
	err := row.Scan(
		&v.ID, &v.UserID, &v.Email, &v.Code, &v.ChannelType, &v.SenderID,
		&v.ChatID, &v.Attempts, &v.ExpiresAt, &v.VerifiedAt, &v.TenantID, &v.CreatedAt,
	)
	if err != nil {
		return nil, err
	}
	return &v, nil
}
