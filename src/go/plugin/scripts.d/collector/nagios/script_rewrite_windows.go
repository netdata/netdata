// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package nagios

import (
	"fmt"
	"os/exec"
	"strings"
)

// rewriteScriptCommand detects Windows script files (.ps1, .bat, .cmd)
// and rewrites the command to invoke them through the appropriate interpreter.
// This allows users to set plugin directly to a script path without manually
// configuring the interpreter.
func rewriteScriptCommand(pluginPath string, args []string) (string, []string, error) {
	lower := strings.ToLower(pluginPath)

	switch {
	case strings.HasSuffix(lower, ".ps1"):
		psPath, err := exec.LookPath("powershell.exe")
		if err != nil {
			return "", nil, fmt.Errorf("plugin '%s' is a PowerShell script but powershell.exe is not in PATH: %w", pluginPath, err)
		}
		newArgs := make([]string, 0, 5+len(args))
		newArgs = append(newArgs, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", pluginPath)
		newArgs = append(newArgs, args...)
		return psPath, newArgs, nil

	case strings.HasSuffix(lower, ".bat"), strings.HasSuffix(lower, ".cmd"):
		cmdPath, err := exec.LookPath("cmd.exe")
		if err != nil {
			return "", nil, fmt.Errorf("plugin '%s' is a batch script but cmd.exe is not in PATH: %w", pluginPath, err)
		}
		newArgs := make([]string, 0, 2+len(args))
		newArgs = append(newArgs, "/c", pluginPath)
		newArgs = append(newArgs, args...)
		return cmdPath, newArgs, nil
	}

	return pluginPath, args, nil
}
