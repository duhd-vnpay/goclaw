package access

import (
	"errors"
	"path/filepath"
	"runtime"
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
	sep := string(filepath.Separator)
	for _, p := range prefixes {
		cleanP := filepath.Clean(p)
		// On Windows, paths are case-insensitive
		pathCmp, prefixCmp := path, cleanP
		if runtime.GOOS == "windows" {
			pathCmp = strings.ToLower(pathCmp)
			prefixCmp = strings.ToLower(prefixCmp)
		}
		// Check if path is exactly the prefix or within it
		if pathCmp == prefixCmp || strings.HasPrefix(pathCmp, prefixCmp+sep) {
			return true
		}
	}
	return false
}
