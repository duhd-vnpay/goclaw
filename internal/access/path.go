package access

import (
	"errors"
	"path/filepath"
	"strings"
)

var ErrForbidden = errors.New("access: forbidden")

// SafeResolvePath applies 4-step normalization to prevent symlink traversal.
// 1. Clean path  2. Prefix check  3. EvalSymlinks  4. Re-check prefix
func SafeResolvePath(requestedPath string, allowedPrefixes []string) (string, error) {
	cleaned := filepath.Clean(requestedPath)

	if !matchesAnyPrefix(cleaned, allowedPrefixes) {
		return "", ErrForbidden
	}

	resolved, err := filepath.EvalSymlinks(cleaned)
	if err != nil {
		return "", err
	}

	if !matchesAnyPrefix(resolved, allowedPrefixes) {
		return "", ErrForbidden
	}

	return resolved, nil
}

func matchesAnyPrefix(path string, prefixes []string) bool {
	for _, p := range prefixes {
		cleanP := filepath.Clean(p)
		// Ensure prefix ends with separator to prevent /media matching /media-evil
		if !strings.HasSuffix(cleanP, string(filepath.Separator)) {
			cleanP += string(filepath.Separator)
		}
		if strings.HasPrefix(path, cleanP) || path == strings.TrimSuffix(cleanP, string(filepath.Separator)) {
			return true
		}
	}
	return false
}
