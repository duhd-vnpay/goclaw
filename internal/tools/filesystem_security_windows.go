//go:build windows

package tools

// hasMutableSymlinkParent is a no-op on Windows (symlink attacks are Unix-specific).
func hasMutableSymlinkParent(path string) bool {
	return false
}

// checkHardlink is a no-op on Windows (hardlink nlink check requires syscall.Stat_t).
func checkHardlink(path string) error {
	return nil
}
