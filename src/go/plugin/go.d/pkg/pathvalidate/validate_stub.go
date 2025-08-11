// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !unix

package pathvalidate

// ValidateBinaryPath checks if a binary path is secure for execution.
// This is a stub implementation for non-Unix platforms.
func ValidateBinaryPath(path string) (string, error) {
	return path, nil
}
