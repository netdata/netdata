// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package nagios

// rewriteScriptCommand is a no-op on non-Windows platforms.
// On Linux/macOS, scripts should be directly executable (chmod +x).
func rewriteScriptCommand(pluginPath string, args []string) (string, []string, error) {
	return pluginPath, args, nil
}
