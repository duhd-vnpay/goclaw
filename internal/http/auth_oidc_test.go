package http

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// Security 2026-07-02 (audit P1#2): isSafeRedirectTarget must reject
// cross-origin targets to close the open-redirect + token-leak path at the
// OIDC callback (state → "#access_token=..." → http.Redirect).
func TestIsSafeRedirectTarget(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "https://goclaw.x.vnshop.cloud/v1/auth/callback", nil)
	req.Host = "goclaw.x.vnshop.cloud"

	cases := []struct {
		name   string
		target string
		want   bool
	}{
		{"relative path", "/dashboard", true},
		{"same-origin absolute URL", "https://goclaw.x.vnshop.cloud/auth/callback", true},
		{"cross-origin absolute URL", "https://evil.com", false},
		{"cross-origin with path", "https://evil.com/steal", false},
		{"protocol-relative bypass attempt", "//evil.com", false},
		{"scheme-relative with different host", "http://goclaw.x.vnshop.cloud.evil.com", false},
		{"empty host after parse", "://broken", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := isSafeRedirectTarget(tc.target, req); got != tc.want {
				t.Errorf("isSafeRedirectTarget(%q) = %v, want %v", tc.target, got, tc.want)
			}
		})
	}
}

func TestIsSafeRedirectTarget_XForwardedHost(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "http://127.0.0.1:18790/v1/auth/callback", nil)
	req.Host = "127.0.0.1:18790"
	req.Header.Set("X-Forwarded-Host", "goclaw.x.vnshop.cloud")

	if !isSafeRedirectTarget("https://goclaw.x.vnshop.cloud/dashboard", req) {
		t.Error("should trust X-Forwarded-Host over r.Host behind a reverse proxy")
	}
	if isSafeRedirectTarget("https://127.0.0.1:18790/dashboard", req) {
		t.Error("should not fall back to r.Host when X-Forwarded-Host is set")
	}
}
