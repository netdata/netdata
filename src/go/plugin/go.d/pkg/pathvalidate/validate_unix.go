// SPDX-License-Identifier: GPL-3.0-or-later

//go:build unix

package pathvalidate

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"
)

// ValidateBinaryPath checks if a binary path is secure for execution.
// It verifies ownership, permissions, and directory security.
func ValidateBinaryPath(path string) error {
	// Step 1: Resolve full symlink path
	resolvedPath, err := filepath.EvalSymlinks(path)
	if err != nil {
		return fmt.Errorf("failed to resolve symlink for %s: %w", path, err)
	}

	// Step 2: Resolve to absolute path
	absPath, err := filepath.Abs(resolvedPath)
	if err != nil {
		return fmt.Errorf("failed to resolve absolute path for %s: %w", resolvedPath, err)
	}

	// Step 3: Stat the resolved file
	fileInfo, err := os.Stat(absPath)
	if err != nil {
		return fmt.Errorf("binary stat error for %s: %w", absPath, err)
	}

	// Step 4: Check that it is a regular file
	if !fileInfo.Mode().IsRegular() {
		return fmt.Errorf("binary at %s must be a regular file, not %s", absPath, fileInfo.Mode().String())
	}

	// Step 5: Check file ownership and permissions
	fileStat, ok := fileInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to get file stat information for %s", absPath)
	}
	if fileStat.Uid != 0 {
		return fmt.Errorf("binary at %s must be owned by root (current uid: %d)", absPath, fileStat.Uid)
	}

	if fileInfo.Mode().Perm()&0022 != 0 {
		return fmt.Errorf("binary at %s must not be writable by group/others (current: %04o)",
			absPath, fileInfo.Mode().Perm())
	}

	// Step 6: Check executable bit
	if fileInfo.Mode().Perm()&0111 == 0 {
		return fmt.Errorf("binary at %s must be executable", absPath)
	}

	// Step 7: Check parent directory
	dir := filepath.Dir(absPath)
	dirInfo, err := os.Stat(dir)
	if err != nil {
		return fmt.Errorf("directory stat error for %s: %w", dir, err)
	}

	dirStat, ok := dirInfo.Sys().(*syscall.Stat_t)
	if !ok {
		return fmt.Errorf("unable to get directory stat information for %s", dir)
	}
	if dirStat.Uid != 0 {
		return fmt.Errorf("directory %s must be owned by root (current uid: %d)", dir, dirStat.Uid)
	}

	perm := dirInfo.Mode().Perm()
	if perm&0022 != 0 {
		return fmt.Errorf("directory %s must not be writable by group/others (current permissions: %s / %04o)",
			dir, dirInfo.Mode().String(), perm)
	}

	return nil
}
