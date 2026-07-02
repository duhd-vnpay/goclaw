package bgalert

import (
	"strings"
	"testing"
)

// Security 2026-07-02 (audit P2#12): sanitizeErrorMessage must scrub both
// bare API keys AND the credential segment of connection-string DSNs before
// an error message reaches the Telegram alert.
func TestSanitizeErrorMessage(t *testing.T) {
	cases := []struct {
		name       string
		msg        string
		mustNotHave string
	}{
		{
			name:        "openai style key",
			msg:         "upstream error: sk-abcdefghijklmnopqrst rejected",
			mustNotHave: "sk-abcdefghijklmnopqrst",
		},
		{
			name:        "x-api-key header",
			msg:         `request failed: header "x-api-key: sk_live_abcdef123456" invalid`,
			mustNotHave: "sk_live_abcdef123456",
		},
		{
			name:        "postgres DSN",
			msg:         "dial error: postgres://goclaw:sup3rSecret@postgres:5432/goclaw connection refused",
			mustNotHave: "sup3rSecret",
		},
		{
			name:        "redis DSN with empty user",
			msg:         "redis dial: redis://:hunter2@redis:6379/0 timeout",
			mustNotHave: "hunter2",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := sanitizeErrorMessage(tc.msg)
			if strings.Contains(got, tc.mustNotHave) {
				t.Errorf("sanitizeErrorMessage(%q) = %q, still contains secret %q", tc.msg, got, tc.mustNotHave)
			}
		})
	}
}

// TestSanitizeErrorMessage_DSNKeepsUserAndHost verifies the DSN redaction
// only masks the password, keeping user/host visible for diagnostics.
func TestSanitizeErrorMessage_DSNKeepsUserAndHost(t *testing.T) {
	got := sanitizeErrorMessage("postgres://goclaw:sup3rSecret@postgres:5432/goclaw")
	if !strings.Contains(got, "goclaw:****@postgres") {
		t.Errorf("expected user+host preserved with password masked, got %q", got)
	}
}
