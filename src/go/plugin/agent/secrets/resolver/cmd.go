// SPDX-License-Identifier: GPL-3.0-or-later

package secretresolver

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

func (r *Resolver) resolveCmd(ctx context.Context, cmdLine, original string) (string, error) {
	parts := strings.Fields(cmdLine)
	if len(parts) == 0 {
		return "", fmt.Errorf("resolving secret '%s': empty command", original)
	}
	if !filepath.IsAbs(parts[0]) {
		return "", fmt.Errorf("resolving secret '%s': command path must be absolute, got '%s'", original, parts[0])
	}

	if ctx == nil {
		ctx = context.Background()
	}
	timeout := r.cmdTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, parts[0], parts[1:]...)
	cmd.Stderr = io.Discard
	out, err := cmd.Output()
	if err != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("resolving secret '%s': command timed out after %s", original, timeout)
		}
		return "", fmt.Errorf("resolving secret '%s': command failed: %w", original, err)
	}

	value := strings.TrimSpace(string(out))
	logResolved(ctx, "resolved secret via command '%s'", parts[0])
	return value, nil
}
