package channels

import (
	"context"
	"crypto/rand"
	"fmt"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	// otpTTL is how long an OTP code is valid.
	otpTTL = 10 * time.Minute
	// otpMaxAttempts is the maximum number of verification attempts per code.
	otpMaxAttempts = 3
	// otpRateWindow is the window for rate limiting OTP requests per sender.
	otpRateWindow = 5 * time.Minute
	// otpRateLimit is the maximum pending verifications per sender within the rate window.
	otpRateLimit = 3
)

// PairingState tracks where a user is in the email-OTP pairing flow.
type PairingState int

const (
	// PairingStateNone means no active pairing flow.
	PairingStateNone PairingState = iota
	// PairingStateAwaitEmail means we asked the user for their email.
	PairingStateAwaitEmail
	// PairingStateAwaitCode means OTP was sent, waiting for user to enter code.
	PairingStateAwaitCode
)

// PairingFlowEntry tracks the state for a single sender's pairing flow.
type PairingFlowEntry struct {
	State       PairingState
	ChannelType string
	SenderID    string
	ChatID      string
	StartedAt   time.Time
}

// PairingHandler manages the email-OTP channel pairing flow.
// It is channel-agnostic — channels call into it and handle platform-specific messaging.
type PairingHandler struct {
	orgUsers     store.OrgUserStore
	verifications store.PairingVerificationStore
	pairing      store.PairingStore
}

// NewPairingHandler creates a new PairingHandler.
func NewPairingHandler(
	orgUsers store.OrgUserStore,
	verifications store.PairingVerificationStore,
	pairing store.PairingStore,
) *PairingHandler {
	return &PairingHandler{
		orgUsers:      orgUsers,
		verifications: verifications,
		pairing:       pairing,
	}
}

// HandlePairCommand initiates the pairing flow. Returns a reply message for the user.
func (h *PairingHandler) HandlePairCommand(ctx context.Context, channelType, senderID, chatID string) string {
	// Check if already paired via verified_user_id on paired_devices.
	if h.pairing != nil {
		paired, _ := h.pairing.IsPaired(ctx, senderID, channelType)
		if paired {
			return "You are already paired. Send a message to start chatting."
		}
	}

	return "Please enter your organization email address to pair this account."
}

// HandleEmailInput processes an email address from the user during pairing flow.
// Returns (reply message, ok). ok=false means the email was not valid or user not found.
func (h *PairingHandler) HandleEmailInput(ctx context.Context, channelType, senderID, chatID, email string) string {
	email = strings.TrimSpace(strings.ToLower(email))

	// Basic email validation.
	if !strings.Contains(email, "@") || !strings.Contains(email, ".") {
		return "Invalid email format. Please enter a valid email address (e.g., user@company.com)."
	}

	// Rate limit check.
	count, err := h.verifications.CountRecent(ctx, senderID, otpRateWindow)
	if err != nil {
		slog.Warn("pairing.rate_limit_check_failed", "sender_id", senderID, "error", err)
	}
	if count >= otpRateLimit {
		return fmt.Sprintf("Too many verification attempts. Please wait %d minutes and try again.", int(otpRateWindow.Minutes()))
	}

	// Lookup org user by email.
	orgUser, err := h.orgUsers.GetByEmail(ctx, email)
	if err != nil {
		slog.Info("pairing.email_not_found", "email", email, "sender_id", senderID)
		return "Email not found in the organization. Please check and try again, or contact your administrator."
	}

	// Generate secure 6-digit OTP.
	code, err := generateOTP()
	if err != nil {
		slog.Error("pairing.otp_generation_failed", "error", err)
		return "An error occurred. Please try again."
	}

	// Create verification record.
	v := &store.PairingVerification{
		UserID:      orgUser.ID,
		Email:       email,
		Code:        code,
		ChannelType: channelType,
		SenderID:    senderID,
		ChatID:      chatID,
		ExpiresAt:   time.Now().Add(otpTTL),
	}
	if err := h.verifications.Create(ctx, v); err != nil {
		slog.Error("pairing.verification_create_failed", "error", err)
		return "An error occurred. Please try again."
	}

	// Log OTP (Keycloak SMTP integration later).
	slog.Info("pairing.otp_generated",
		"email", email,
		"sender_id", senderID,
		"channel_type", channelType,
		"code", code,
	)

	return fmt.Sprintf(
		"A verification code has been sent to %s.\n"+
			"Please enter the 6-digit code to complete pairing.\n"+
			"(Code expires in %d minutes)",
		email, int(otpTTL.Minutes()),
	)
}

// HandleCodeInput processes a verification code from the user.
// Returns (reply message, paired bool).
func (h *PairingHandler) HandleCodeInput(ctx context.Context, channelType, senderID, chatID, code string) (string, bool) {
	code = strings.TrimSpace(code)
	if len(code) != 6 {
		return "Please enter the 6-digit verification code.", false
	}

	// Get pending verification.
	pending, err := h.verifications.GetPending(ctx, senderID, channelType)
	if err != nil {
		return "No pending verification found. Please start over with /pair.", false
	}

	// Check max attempts.
	if pending.Attempts >= otpMaxAttempts {
		return "Maximum verification attempts exceeded. Please start over with /pair.", false
	}

	// Increment attempts first.
	if err := h.verifications.IncrementAttempts(ctx, pending.ID); err != nil {
		slog.Error("pairing.increment_attempts_failed", "error", err)
	}

	// Verify code.
	if !strings.EqualFold(pending.Code, code) {
		remaining := otpMaxAttempts - pending.Attempts - 1
		if remaining <= 0 {
			return "Incorrect code. Maximum attempts exceeded. Please start over with /pair.", false
		}
		return fmt.Sprintf("Incorrect code. %d attempt(s) remaining.", remaining), false
	}

	// Mark verified.
	if err := h.verifications.MarkVerified(ctx, pending.ID); err != nil {
		slog.Error("pairing.mark_verified_failed", "error", err)
		return "An error occurred. Please try again.", false
	}

	// Approve pairing via existing PairingStore (creates paired_device entry).
	if h.pairing != nil {
		meta := map[string]string{"email": pending.Email}
		_, err := h.pairing.RequestPairing(ctx, senderID, channelType, chatID, "default", meta)
		if err != nil {
			slog.Warn("pairing.request_pairing_failed", "error", err)
		}
		// Auto-approve with the email as approver.
		devices := h.pairing.ListPending(ctx)
		for _, d := range devices {
			if d.SenderID == senderID && d.Channel == channelType {
				_, err := h.pairing.ApprovePairing(ctx, d.Code, pending.Email)
				if err != nil {
					slog.Warn("pairing.auto_approve_failed", "code", d.Code, "error", err)
				}
				break
			}
		}
	}

	// Link identity to channel_contacts and paired_devices.
	if err := h.verifications.LinkContactIdentity(ctx, channelType, senderID, pending.Email, pending.UserID); err != nil {
		slog.Warn("pairing.link_contact_failed", "error", err)
	}
	if err := h.verifications.LinkDeviceIdentity(ctx, senderID, channelType, pending.Email, pending.UserID); err != nil {
		slog.Warn("pairing.link_device_failed", "error", err)
	}

	// Resolve display name for welcome message.
	orgUser, err := h.orgUsers.GetByID(ctx, pending.UserID)
	name := pending.Email
	role := ""
	if err == nil && orgUser != nil {
		if orgUser.DisplayName != nil && *orgUser.DisplayName != "" {
			name = *orgUser.DisplayName
		}
		role = orgUser.Status
	}

	if role != "" {
		return fmt.Sprintf("Paired! Welcome %s (%s)", name, role), true
	}
	return fmt.Sprintf("Paired! Welcome %s", name), true
}

// generateOTP generates a secure 6-digit numeric OTP using crypto/rand.
func generateOTP() (string, error) {
	var code strings.Builder
	for i := 0; i < 6; i++ {
		n, err := rand.Int(rand.Reader, big.NewInt(10))
		if err != nil {
			return "", fmt.Errorf("generate OTP digit: %w", err)
		}
		code.WriteString(fmt.Sprintf("%d", n.Int64()))
	}
	return code.String(), nil
}

// IsPossibleOTP returns true if the text looks like a 6-digit OTP code.
func IsPossibleOTP(text string) bool {
	text = strings.TrimSpace(text)
	if len(text) != 6 {
		return false
	}
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// IsPossibleEmail returns true if the text looks like an email address.
func IsPossibleEmail(text string) bool {
	text = strings.TrimSpace(text)
	return strings.Contains(text, "@") && strings.Contains(text, ".") && !strings.Contains(text, " ") && len(text) > 5
}

