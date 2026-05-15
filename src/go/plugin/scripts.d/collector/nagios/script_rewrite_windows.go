// SPDX-License-Identifier: GPL-3.0-or-later

//go:build windows

package nagios

import (
	"fmt"
	"os"
	"path/filepath"
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
		psPath, err := findPowerShell()
		if err != nil {
			return "", nil, fmt.Errorf("plugin '%s' is a PowerShell script but powershell.exe was not found: %w", pluginPath, err)
		}
		newArgs := make([]string, 0, 5+len(args))
		newArgs = append(newArgs, "-NoProfile", "-ExecutionPolicy", "Bypass", "-File", pluginPath)
		newArgs = append(newArgs, args...)
		return psPath, newArgs, nil

	case strings.HasSuffix(lower, ".bat"), strings.HasSuffix(lower, ".cmd"):
		cmdPath, err := findCmd()
		if err != nil {
			return "", nil, fmt.Errorf("plugin '%s' is a batch script but cmd.exe was not found: %w", pluginPath, err)
		}
		newArgs := make([]string, 0, 2+len(args))
		newArgs = append(newArgs, "/c", pluginPath)
		newArgs = append(newArgs, args...)
		return cmdPath, newArgs, nil
	}

	return pluginPath, args, nil
}

func findPowerShell() (string, error) {
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	path := filepath.Join(sysRoot, "System32", "WindowsPowerShell", "v1.0", "powershell.exe")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("powershell.exe not found at %s: %w", path, err)
	}
	return path, nil
}

func findCmd() (string, error) {
	// ComSpec is the standard Windows env var pointing to cmd.exe.
	if comSpec := os.Getenv("ComSpec"); comSpec != "" {
		if _, err := os.Stat(comSpec); err == nil {
			return comSpec, nil
		}
	}
	sysRoot := os.Getenv("SystemRoot")
	if sysRoot == "" {
		sysRoot = `C:\Windows`
	}
	path := filepath.Join(sysRoot, "System32", "cmd.exe")
	if _, err := os.Stat(path); err != nil {
		return "", fmt.Errorf("cmd.exe not found at %s: %w", path, err)
	}
	return path, nil
}
