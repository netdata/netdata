// SPDX-License-Identifier: GPL-3.0-or-later

package testutil

import (
	"context"
	"os/exec"
	"runtime"
	"testing"
	"time"
)

// Bash returns an installed Bash4+ for owned custom-function fixtures; never installs one.
func Bash(t testing.TB) string {
	t.Helper()
	if runtime.GOOS != "linux" && runtime.GOOS != "darwin" {
		t.Skip("custom runtime supports Linux and macOS")
	}
	discovered, _ := exec.LookPath("bash")
	for _, path := range []string{discovered, "/opt/homebrew/bin/bash", "/usr/local/bin/bash", "/bin/bash"} {
		if path == "" {
			continue
		}
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		cmd := exec.CommandContext(ctx, path, "--noprofile", "--norc", "-c", `((BASH_VERSINFO[0] >= 4))`)
		cmd.Env = []string{"PATH=/usr/bin:/bin"}
		err := cmd.Run()
		cancel()
		if err == nil {
			return path
		}
	}
	t.Skip("Bash4+ is not installed")
	return ""
}
