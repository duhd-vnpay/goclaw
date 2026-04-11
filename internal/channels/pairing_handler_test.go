package channels

import (
	"testing"
)

func TestGenerateOTP(t *testing.T) {
	// Generate multiple OTPs and verify format.
	seen := make(map[string]bool)
	for i := 0; i < 100; i++ {
		code, err := generateOTP()
		if err != nil {
			t.Fatalf("generateOTP() error: %v", err)
		}
		if len(code) != 6 {
			t.Errorf("OTP length = %d, want 6", len(code))
		}
		for _, r := range code {
			if r < '0' || r > '9' {
				t.Errorf("OTP contains non-digit: %q", code)
				break
			}
		}
		seen[code] = true
	}
	// With 100 random 6-digit codes, we should have at least 50 unique ones.
	if len(seen) < 50 {
		t.Errorf("OTP randomness seems low: only %d unique codes in 100 attempts", len(seen))
	}
}

func TestIsPossibleOTP(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"123456", true},
		{"000000", true},
		{"999999", true},
		{" 123456 ", true}, // spaces trimmed
		{"12345", false},   // too short
		{"1234567", false}, // too long
		{"12345a", false},  // contains letter
		{"", false},
		{"abcdef", false},
		{"12 345", false}, // spaces in middle
	}
	for _, tt := range tests {
		got := IsPossibleOTP(tt.input)
		if got != tt.want {
			t.Errorf("IsPossibleOTP(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}

func TestIsPossibleEmail(t *testing.T) {
	tests := []struct {
		input string
		want  bool
	}{
		{"user@example.com", true},
		{"a@b.c", false},         // too short (5 chars after trim)
		{"user@company.vn", true},
		{"noemail", false},
		{"@missing.com", true},   // has @ and . and > 5 chars
		{"user@", false},         // no dot
		{"user.com", false},      // no @
		{"has space@x.com", false},
		{"", false},
	}
	for _, tt := range tests {
		got := IsPossibleEmail(tt.input)
		if got != tt.want {
			t.Errorf("IsPossibleEmail(%q) = %v, want %v", tt.input, got, tt.want)
		}
	}
}
