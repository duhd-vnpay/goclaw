package media

import (
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/uuid"
)

// Store provides persistent media file storage scoped by session.
// Files are organized as: {baseDir}/{sessionHash}/{uuid}.{ext}
type Store struct {
	baseDir string
}

// NewStore creates a media store rooted at baseDir.
// The directory is created if it doesn't exist.
func NewStore(baseDir string) (*Store, error) {
	if err := os.MkdirAll(baseDir, 0755); err != nil {
		return nil, fmt.Errorf("media: create base dir: %w", err)
	}
	return &Store{baseDir: baseDir}, nil
}

// SaveFile moves or copies a file to persistent storage.
// Returns the unique media ID and the destination path.
func (s *Store) SaveFile(sessionKey, srcPath, mime string) (id string, dstPath string, err error) {
	dir := s.sessionDir(sessionKey)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return "", "", fmt.Errorf("media: create session dir: %w", err)
	}

	mediaID := uuid.New().String()
	ext := extFromMime(mime)
	if ext == "" {
		ext = filepath.Ext(srcPath)
	}
	dstPath = filepath.Join(dir, mediaID+ext)

	// Try rename first (fast, same filesystem).
	if err := os.Rename(srcPath, dstPath); err == nil {
		return mediaID, dstPath, nil
	}

	// Fallback: copy + remove source.
	if err := copyFile(srcPath, dstPath); err != nil {
		return "", "", fmt.Errorf("media: copy file: %w", err)
	}
	_ = os.Remove(srcPath) // best-effort cleanup of source
	return mediaID, dstPath, nil
}

// LoadPath returns the filesystem path for a media ID.
// Returns an error if the file doesn't exist.
func (s *Store) LoadPath(id string) (string, error) {
	// Media files are stored as {sessionHash}/{id}.{ext}.
	// Since we don't know the session hash, glob for the ID across all session dirs.
	matches, err := filepath.Glob(filepath.Join(s.baseDir, "*", id+".*"))
	if err != nil {
		return "", fmt.Errorf("media: glob for %s: %w", id, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("media: file not found: %s", id)
	}
	return s.resolveAndValidate(id, matches[0])
}

// AdminCtxKey is the context key for admin bypass flag.
// Exported for use by HTTP handlers.
type ContextKey string

const AdminCtxKey ContextKey = "isAdmin"

// SessionHash returns the 12 hex char session directory hash (exported).
// Uses same algorithm as internal sessionDir().
func (s *Store) SessionHash(sessionKey string) string {
	h := sha256.Sum256([]byte(sessionKey))
	return fmt.Sprintf("%x", h[:6])
}

// LoadPathScoped returns the filesystem path for a media ID scoped to a session hash.
// Only searches within the specific session directory — prevents cross-session access.
// Used by HTTP handlers with scoped tokens.
func (s *Store) LoadPathScoped(id string, sessionHash string) (string, error) {
	pattern := filepath.Join(s.baseDir, sessionHash, id+".*")
	matches, err := filepath.Glob(pattern)
	if err != nil {
		return "", fmt.Errorf("media: glob for %s in %s: %w", id, sessionHash, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("media: file not found: %s in session %s", id, sessionHash)
	}
	return s.resolveAndValidate(id, matches[0])
}

// LoadPathAny returns the filesystem path for a media ID across ALL sessions.
// ADMIN ONLY — defense-in-depth check. Legacy/migration backward compat.
// Will be deprecated after migration window.
func (s *Store) LoadPathAny(id string, ctx context.Context) (string, error) {
	if admin, _ := ctx.Value(AdminCtxKey).(bool); !admin {
		return "", fmt.Errorf("media: LoadPathAny requires admin context")
	}
	slog.Warn("media: legacy LoadPathAny called", "id", sanitizePath(id))
	matches, err := filepath.Glob(filepath.Join(s.baseDir, "*", id+".*"))
	if err != nil {
		return "", fmt.Errorf("media: glob for %s: %w", id, err)
	}
	if len(matches) == 0 {
		return "", fmt.Errorf("media: file not found: %s", id)
	}
	return s.resolveAndValidate(id, matches[0])
}

// resolveAndValidate resolves symlinks and ensures the path stays within baseDir.
func (s *Store) resolveAndValidate(id, path string) (string, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", fmt.Errorf("media: resolve symlink for %s: %w", id, err)
	}
	absBase, _ := filepath.Abs(s.baseDir)
	absResolved, _ := filepath.Abs(resolved)
	if !strings.HasPrefix(absResolved, absBase+string(filepath.Separator)) {
		slog.Warn("media: path traversal attempt blocked",
			"id", sanitizePath(id),
			"resolved", sanitizePath(absResolved),
			"base", absBase,
		)
		return "", fmt.Errorf("media: path outside base dir: %s", id)
	}
	return resolved, nil
}

// sanitizePath removes newlines and control characters from paths before logging.
func sanitizePath(path string) string {
	return strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r < 32 {
			return '_'
		}
		return r
	}, path)
}

// DeleteSession removes all media files for a session.
func (s *Store) DeleteSession(sessionKey string) error {
	dir := s.sessionDir(sessionKey)
	if err := os.RemoveAll(dir); err != nil {
		slog.Warn("media: failed to delete session dir", "dir", dir, "error", err)
		return err
	}
	return nil
}

// sessionDir returns the directory path for a session's media files.
// Uses first 12 chars of SHA-256 hash of sessionKey for filesystem safety.
func (s *Store) sessionDir(sessionKey string) string {
	h := sha256.Sum256([]byte(sessionKey))
	hash := fmt.Sprintf("%x", h[:6]) // 12 hex chars
	return filepath.Join(s.baseDir, hash)
}

// extFromMime returns a file extension (with dot) for a MIME type.
func extFromMime(mime string) string {
	switch {
	case strings.HasPrefix(mime, "image/jpeg"):
		return ".jpg"
	case strings.HasPrefix(mime, "image/png"):
		return ".png"
	case strings.HasPrefix(mime, "image/gif"):
		return ".gif"
	case strings.HasPrefix(mime, "image/webp"):
		return ".webp"
	case strings.HasPrefix(mime, "video/mp4"):
		return ".mp4"
	case strings.HasPrefix(mime, "audio/ogg"), strings.HasPrefix(mime, "audio/opus"):
		return ".ogg"
	case strings.HasPrefix(mime, "audio/mpeg"):
		return ".mp3"
	case strings.HasPrefix(mime, "audio/wav"):
		return ".wav"
	case strings.HasPrefix(mime, "application/pdf"):
		return ".pdf"
	case mime == "application/vnd.openxmlformats-officedocument.wordprocessingml.document":
		return ".docx"
	case mime == "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet":
		return ".xlsx"
	default:
		return ""
	}
}

// FileProcessor allows plugging in file transformations (e.g., encryption).
// Phase A: NoOpProcessor. Future: AESProcessor.
type FileProcessor interface {
	BeforeSave(data []byte, meta FileMeta) ([]byte, error)
	BeforeServe(data []byte, meta FileMeta) ([]byte, error)
}

// FileMeta provides context about the file being processed.
type FileMeta struct {
	MediaID     string
	SessionHash string
	MimeType    string
	Size        int64
}

// NoOpProcessor is a pass-through processor (no transformation).
type NoOpProcessor struct{}

func (NoOpProcessor) BeforeSave(data []byte, _ FileMeta) ([]byte, error)  { return data, nil }
func (NoOpProcessor) BeforeServe(data []byte, _ FileMeta) ([]byte, error) { return data, nil }

// copyFile copies src to dst using buffered I/O.
func copyFile(src, dst string) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()

	out, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer out.Close()

	if _, err := io.Copy(out, in); err != nil {
		return err
	}
	return out.Close()
}
