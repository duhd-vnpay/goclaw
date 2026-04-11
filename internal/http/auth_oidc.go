// internal/http/auth_oidc.go
package http

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/google/uuid"

	"github.com/nextlevelbuilder/goclaw/internal/auth"
	"github.com/nextlevelbuilder/goclaw/internal/config"
	"github.com/nextlevelbuilder/goclaw/internal/store"
)

const (
	authCookieName   = "goclaw_session"
	authCookieMaxAge = 24 * 60 * 60 // 24 hours in seconds
)

// AuthOIDCHandler handles Keycloak OIDC login/callback/me/logout.
type AuthOIDCHandler struct {
	cfg       config.KeycloakConfig
	validator *auth.JWTValidator
	orgUsers  store.OrgUserStore
	tenants   store.TenantStore
}

// NewAuthOIDCHandler creates a new OIDC auth handler.
func NewAuthOIDCHandler(
	cfg config.KeycloakConfig,
	validator *auth.JWTValidator,
	orgUsers store.OrgUserStore,
	tenants store.TenantStore,
) *AuthOIDCHandler {
	return &AuthOIDCHandler{
		cfg:       cfg,
		validator: validator,
		orgUsers:  orgUsers,
		tenants:   tenants,
	}
}

// RegisterRoutes registers all auth routes on the given mux.
func (h *AuthOIDCHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/auth/login", h.handleLogin)
	mux.HandleFunc("GET /v1/auth/callback", h.handleCallback)
	mux.HandleFunc("GET /v1/auth/me", h.handleMe)
	mux.HandleFunc("POST /v1/auth/logout", h.handleLogout)
	mux.HandleFunc("GET /v1/auth/status", h.handleStatus)
}

// handleLogin redirects to the Keycloak authorization endpoint.
func (h *AuthOIDCHandler) handleLogin(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled() {
		writeAuthError(w, http.StatusServiceUnavailable, "keycloak not configured")
		return
	}

	redirectURI := h.cfg.CallbackURL
	if redirectURI == "" {
		// Auto-detect from request
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		redirectURI = scheme + "://" + r.Host + "/v1/auth/callback"
	}

	params := url.Values{
		"client_id":     {h.cfg.ClientID},
		"redirect_uri":  {redirectURI},
		"response_type": {"code"},
		"scope":         {"openid email profile"},
	}

	// Pass through ?redirect= for post-login navigation
	if redir := r.URL.Query().Get("redirect"); redir != "" {
		params.Set("state", redir)
	}

	authURL := h.cfg.AuthorizationURL() + "?" + params.Encode()
	http.Redirect(w, r, authURL, http.StatusFound)
}

// handleCallback processes the Keycloak OIDC callback with authorization code.
func (h *AuthOIDCHandler) handleCallback(w http.ResponseWriter, r *http.Request) {
	if !h.cfg.Enabled() {
		writeAuthError(w, http.StatusServiceUnavailable, "keycloak not configured")
		return
	}

	code := r.URL.Query().Get("code")
	if code == "" {
		errMsg := r.URL.Query().Get("error_description")
		if errMsg == "" {
			errMsg = r.URL.Query().Get("error")
		}
		if errMsg == "" {
			errMsg = "missing authorization code"
		}
		writeAuthError(w, http.StatusBadRequest, errMsg)
		return
	}

	redirectURI := h.cfg.CallbackURL
	if redirectURI == "" {
		scheme := "https"
		if r.TLS == nil {
			scheme = "http"
		}
		redirectURI = scheme + "://" + r.Host + "/v1/auth/callback"
	}

	// Exchange code for tokens
	tokenResp, err := auth.ExchangeCode(
		r.Context(),
		h.cfg.TokenURL(),
		h.cfg.ClientID,
		h.cfg.ClientSecret,
		code,
		redirectURI,
	)
	if err != nil {
		slog.Error("auth.callback: code exchange failed", "error", err)
		writeAuthError(w, http.StatusBadGateway, "login service error")
		return
	}

	// Validate access token and extract claims
	claims, err := h.validator.Validate(tokenResp.AccessToken)
	if err != nil {
		slog.Error("auth.callback: JWT validation failed", "error", err)
		writeAuthError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	// Parse keycloak UUID
	keycloakID, err := uuid.Parse(claims.KeycloakID)
	if err != nil {
		slog.Error("auth.callback: invalid keycloak_id", "sub", claims.KeycloakID, "error", err)
		writeAuthError(w, http.StatusInternalServerError, "invalid user identity")
		return
	}

	// Resolve tenant (default to master for now)
	ctx := store.WithTenantID(r.Context(), store.MasterTenantID)

	// UPSERT org_users
	now := time.Now()
	displayName := claims.Name
	authProvider := claims.AuthProvider
	if authProvider == "" {
		authProvider = "keycloak"
	}

	orgUser, err := h.orgUsers.Upsert(ctx, &store.OrgUserData{
		ID:           keycloakID,
		TenantID:     store.MasterTenantID,
		Email:        claims.Email,
		DisplayName:  &displayName,
		AuthProvider: &authProvider,
		LastLoginAt:  &now,
		Status:       "active",
	})
	if err != nil {
		slog.Error("auth.callback: org_users upsert failed", "email", claims.Email, "error", err)
		writeAuthError(w, http.StatusInternalServerError, "user creation failed")
		return
	}

	// UPSERT tenant_users (map highest realm role)
	role := resolveHighestRole(claims.RealmRoles)
	if h.tenants != nil {
		_, err = h.tenants.CreateTenantUserReturning(ctx, store.MasterTenantID, claims.Email, claims.Name, role)
		if err != nil {
			slog.Warn("auth.callback: tenant_user upsert failed", "email", claims.Email, "error", err)
			// Non-fatal: user can still use the system
		}
	}

	// Set session cookie with access token
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    tokenResp.AccessToken,
		Path:     "/",
		MaxAge:   authCookieMaxAge,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// Build response
	resp := map[string]any{
		"user": map[string]any{
			"id":            orgUser.ID,
			"email":         orgUser.Email,
			"display_name":  orgUser.DisplayName,
			"avatar_url":    orgUser.AvatarURL,
			"auth_provider": orgUser.AuthProvider,
			"role":          role,
		},
		"access_token": tokenResp.AccessToken,
	}

	// If state param has a redirect URL, redirect to the frontend with token
	if state := r.URL.Query().Get("state"); state != "" {
		// Redirect to frontend with token in fragment (not query -- avoids server logs)
		frontendURL := state + "#access_token=" + url.QueryEscape(tokenResp.AccessToken)
		http.Redirect(w, r, frontendURL, http.StatusFound)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleMe returns the current user profile from a valid JWT.
func (h *AuthOIDCHandler) handleMe(w http.ResponseWriter, r *http.Request) {
	tokenStr := extractTokenFromRequest(r)
	if tokenStr == "" {
		writeAuthError(w, http.StatusUnauthorized, "missing token")
		return
	}

	claims, err := h.validator.Validate(tokenStr)
	if err != nil {
		writeAuthError(w, http.StatusUnauthorized, "invalid token")
		return
	}

	keycloakID, err := uuid.Parse(claims.KeycloakID)
	if err != nil {
		writeAuthError(w, http.StatusInternalServerError, "invalid user identity")
		return
	}

	ctx := store.WithTenantID(r.Context(), store.MasterTenantID)
	orgUser, err := h.orgUsers.GetByID(ctx, keycloakID)
	if err != nil {
		writeAuthError(w, http.StatusNotFound, "user not found")
		return
	}

	role := resolveHighestRole(claims.RealmRoles)
	resp := map[string]any{
		"id":            orgUser.ID,
		"email":         orgUser.Email,
		"display_name":  orgUser.DisplayName,
		"avatar_url":    orgUser.AvatarURL,
		"auth_provider": orgUser.AuthProvider,
		"role":          role,
		"realm_roles":   claims.RealmRoles,
		"groups":        claims.Groups,
		"status":        orgUser.Status,
		"last_login_at": orgUser.LastLoginAt,
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// handleLogout clears the session cookie and optionally redirects to Keycloak logout.
func (h *AuthOIDCHandler) handleLogout(w http.ResponseWriter, r *http.Request) {
	// Clear cookie
	http.SetCookie(w, &http.Cookie{
		Name:     authCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		Secure:   r.TLS != nil,
		SameSite: http.SameSiteLaxMode,
	})

	// If Keycloak is configured, provide logout URL
	if h.cfg.Enabled() {
		resp := map[string]string{
			"logout_url": h.cfg.LogoutURL(),
			"status":     "logged_out",
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "logged_out"})
}

// handleStatus returns whether Keycloak OIDC is enabled.
func (h *AuthOIDCHandler) handleStatus(w http.ResponseWriter, r *http.Request) {
	resp := map[string]any{
		"keycloak_enabled": h.cfg.Enabled(),
		"login_url":        "/v1/auth/login",
	}
	if h.cfg.Enabled() {
		resp["realm_url"] = h.cfg.RealmURL
		resp["client_id"] = h.cfg.ClientID
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(resp)
}

// extractTokenFromRequest gets the JWT from the Authorization header or session cookie.
func extractTokenFromRequest(r *http.Request) string {
	// Try Authorization header first
	if bearer := extractBearerToken(r); bearer != "" {
		return bearer
	}
	// Try session cookie
	cookie, err := r.Cookie(authCookieName)
	if err != nil {
		return ""
	}
	return cookie.Value
}

// resolveHighestRole returns the highest-priority role from a list of realm roles.
// Priority order: owner > admin > operator > member > viewer.
func resolveHighestRole(roles []string) string {
	priority := map[string]int{
		"owner":    5,
		"admin":    4,
		"operator": 3,
		"member":   2,
		"viewer":   1,
	}
	highest := "viewer"
	highestPri := 0
	for _, r := range roles {
		if p, ok := priority[r]; ok && p > highestPri {
			highest = r
			highestPri = p
		}
	}
	return highest
}

// writeAuthError writes a JSON error response for auth endpoints.
func writeAuthError(w http.ResponseWriter, code int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(map[string]string{"error": message})
}
