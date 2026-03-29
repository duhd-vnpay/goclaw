package http

import (
	"context"
	"log/slog"
	"mime"
	"net/http"
	"path/filepath"
	"regexp"
	"time"

	"github.com/nextlevelbuilder/goclaw/internal/access"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
	"github.com/nextlevelbuilder/goclaw/internal/media"
)

// validMediaID matches safe media identifiers: alphanumeric, hyphens, underscores, dots.
// Minimum 1 char. Rejects path separators, traversal sequences, and special characters.
var validMediaID = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]*$`)

// MediaServeHandler serves persisted media files by ID.
type MediaServeHandler struct {
	store        *media.Store
	gatewayToken string
	tokenSigner  *access.TokenSigner
	checker      access.AccessChecker
	rateLimiter  *RateLimiter
}

// NewMediaServeHandler creates a media serve handler.
// If signer is nil, only gateway token auth is used (backward compat).
func NewMediaServeHandler(store *media.Store, gatewayToken string, signer *access.TokenSigner, checker access.AccessChecker) *MediaServeHandler {
	return &MediaServeHandler{
		store:        store,
		gatewayToken: gatewayToken,
		tokenSigner:  signer,
		checker:      checker,
		rateLimiter:  NewRateLimiter(100, time.Minute), // 100 req/min per IP
	}
}

// RegisterRoutes registers the media serve endpoint with rate limiting.
func (h *MediaServeHandler) RegisterRoutes(mux *http.ServeMux) {
	handler := http.HandlerFunc(h.handleServe)
	mux.Handle("GET /v1/media/{id}", RateLimitMiddleware(handler, h.rateLimiter))
}

func (h *MediaServeHandler) handleServe(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)
	id := r.PathValue("id")
	if !validMediaID.MatchString(id) {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": i18n.T(locale, i18n.MsgInvalidRequest, "invalid media id")})
		return
	}

	// Priority 0: short-lived signed file token (?ft=) — decoupled from gateway token.
	if ft := r.URL.Query().Get("ft"); ft != "" {
		if !VerifyFileToken(ft, "/v1/media/"+id, FileSigningKey()) {
			http.Error(w, "invalid or expired file token", http.StatusUnauthorized)
			return
		}
		// File token verified — serve without further auth checks.
		h.serveMedia(w, r, id)
		return
	}

	provided := extractBearerToken(r)
	if provided == "" {
		provided = r.URL.Query().Get("token")
	}
	if provided == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "token required"})
		return
	}

	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	var filePath string
	var err error

	if h.tokenSigner != nil && access.IsScopedToken(provided) {
		// Scoped token path
		sessionHash, verifyErr := h.tokenSigner.Verify(provided, id)
		if verifyErr != nil {
			slog.Debug("media serve: scoped token invalid", "id", id, "error", verifyErr)
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid or expired token"})
			return
		}

		filePath, err = h.store.LoadPathScoped(id, sessionHash)
		if err != nil {
			slog.Debug("media serve: not found in session", "id", id, "session", sessionHash)
			http.NotFound(w, r)
			return
		}

		if h.checker != nil {
			h.checker.RecordAccess(r.Context(), access.AccessRequest{
				SubjectID:    extractUserID(r),
				SessionHash:  sessionHash,
				Resource:     filePath,
				ResourceType: "media",
				Action:       access.ActionRead,
				Source:       "http",
				IPAddress:    r.RemoteAddr,
			}, true)
		}
	} else {
		// Gateway token path (legacy/admin)
		if !tokenMatch(provided, h.gatewayToken) {
			writeJSON(w, http.StatusUnauthorized, map[string]string{"error": "invalid token"})
			return
		}

		ctx := context.WithValue(r.Context(), media.AdminCtxKey, true)
		filePath, err = h.store.LoadPathAny(id, ctx)
		if err != nil {
			http.NotFound(w, r)
			return
		}

		slog.Warn("media serve: legacy gateway token used", "id", id)
		if h.checker != nil {
			h.checker.RecordAccess(r.Context(), access.AccessRequest{
				SubjectID:    "admin",
				Resource:     filePath,
				ResourceType: "media",
				Action:       access.ActionRead,
				Source:       "http",
				IPAddress:    r.RemoteAddr,
				IsAdmin:      true,
			}, true)
		}
	}

	ext := filepath.Ext(filePath)
	ct := mime.TypeByExtension(ext)
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")

	http.ServeFile(w, r, filePath)
}

// serveMedia serves a media file by ID without additional auth checks.
// Used when auth is already verified (e.g., via HMAC file token).
func (h *MediaServeHandler) serveMedia(w http.ResponseWriter, r *http.Request, id string) {
	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	ctx := context.WithValue(r.Context(), media.AdminCtxKey, true)
	filePath, err := h.store.LoadPathAny(id, ctx)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	ext := filepath.Ext(filePath)
	ct := mime.TypeByExtension(ext)
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}
	w.Header().Set("Cache-Control", "private, max-age=3600")

	http.ServeFile(w, r, filePath)
}
