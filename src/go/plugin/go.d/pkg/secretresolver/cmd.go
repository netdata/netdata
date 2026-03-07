// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func resolveCmd(cmdLine, original string) (string, error) {
	// Split by whitespace
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return "", fmt.Errorf("resolving secret '%s': empty command", original)
	}
	// Security: require absolute path
	if !filepath.IsAbs(parts[0]) {
		return "", fmt.Errorf("resolving secret '%s': command path must be absolute, got '%s'", original, parts[0])
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	out, err := cmd.Output()
	if err != nil {
		if ctx.Err() == context.DeadlineExceeded {
			return "", fmt.Errorf("resolving secret '%s': command timed out after 10s", original)
		}
		return "", fmt.Errorf("resolving secret '%s': command failed: %w", original, err)
	}

	return strings.TrimSpace(string(out)), nil
}
