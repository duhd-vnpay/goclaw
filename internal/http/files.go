package http

import (
	"log/slog"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/nextlevelbuilder/goclaw/internal/access"
	"github.com/nextlevelbuilder/goclaw/internal/i18n"
)

// FilesHandler serves files over HTTP with Bearer token auth.
// Accepts absolute paths — the auth token protects against unauthorized access.
// Admin (gateway token) callers get unrestricted access; non-admin callers are
// restricted to the workspace-scoped allowed prefixes via SafeResolvePath.
type FilesHandler struct {
	token string
}

// NewFilesHandler creates a handler that serves files by absolute path.
func NewFilesHandler(token string) *FilesHandler {
	return &FilesHandler{token: token}
}

// RegisterRoutes registers the file serving route.
func (h *FilesHandler) RegisterRoutes(mux *http.ServeMux) {
	mux.HandleFunc("GET /v1/files/{path...}", h.handleServe)
}

// deniedFilePrefixes blocks access to sensitive system directories.
// Defense-in-depth: the auth token is the primary barrier, but restricting
// known-sensitive paths limits damage if a token leaks.
var deniedFilePrefixes = []string{
	"/etc/", "/proc/", "/sys/", "/dev/",
	"/root/", "/boot/", "/run/",
	"/var/run/", "/var/log/",
}

// allowedWorkspacePrefixes are the paths non-admin users may access.
var allowedWorkspacePrefixes = []string{
	"/app/workspace/.media/",
	"/app/workspace/teams/",
}

func (h *FilesHandler) handleServe(w http.ResponseWriter, r *http.Request) {
	locale := extractLocale(r)

	// Accept token via Bearer header or ?token= query param (for <img src>).
	provided := extractBearerToken(r)
	if provided == "" {
		provided = r.URL.Query().Get("token")
	}
	if provided == "" {
		w.Header().Set("WWW-Authenticate", "Bearer")
		http.Error(w, i18n.T(locale, i18n.MsgUnauthorized), http.StatusUnauthorized)
		return
	}
	if !requireAuthBearer(h.token, "", provided, w, r) {
		return
	}

	urlPath := r.PathValue("path")
	if urlPath == "" {
		http.Error(w, i18n.T(locale, i18n.MsgRequired, "path"), http.StatusBadRequest)
		return
	}

	// Prevent path traversal
	if strings.Contains(urlPath, "..") {
		slog.Warn("security.files_traversal", "path", urlPath)
		http.Error(w, i18n.T(locale, i18n.MsgInvalidPath), http.StatusBadRequest)
		return
	}

	// URL path is the absolute path with leading "/" stripped (e.g. "app/workspace/.media/file.png")
	absPath := filepath.Clean("/" + urlPath)

	// Block access to sensitive system directories
	for _, prefix := range deniedFilePrefixes {
		if strings.HasPrefix(absPath, prefix) {
			slog.Warn("security.files_denied_path", "path", absPath)
			http.Error(w, i18n.T(locale, i18n.MsgInvalidPath), http.StatusForbidden)
			return
		}
	}

	// User-scoped path restriction for non-admin users.
	// Gateway token holders (admin) get unrestricted access (existing behavior).
	// Non-admin users: restrict to allowed workspace paths via SafeResolvePath
	// which also guards against symlink traversal.
	isAdmin := tokenMatch(provided, h.token)
	if !isAdmin {
		if _, err := access.SafeResolvePath(absPath, allowedWorkspacePrefixes); err != nil {
			slog.Warn("security.files_acl_denied", "path", absPath)
			http.Error(w, i18n.T(locale, i18n.MsgInvalidPath), http.StatusForbidden)
			return
		}
	}

	info, err := os.Stat(absPath)
	if err != nil || info.IsDir() {
		http.NotFound(w, r)
		return
	}

	// Set Content-Type from extension
	ext := filepath.Ext(absPath)
	ct := mime.TypeByExtension(ext)
	if ct != "" {
		w.Header().Set("Content-Type", ct)
	}

	w.Header().Set("Referrer-Policy", "no-referrer")
	w.Header().Set("X-Content-Type-Options", "nosniff")

	http.ServeFile(w, r, absPath)
}
