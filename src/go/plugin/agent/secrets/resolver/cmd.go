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
	configureCommandProcessTree(cmd)
	cmd.Stderr = io.Discard
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return "", fmt.Errorf("resolving secret '%s': command stdout: %w", original, err)
	}
	if err := cmd.Start(); err != nil {
		return "", fmt.Errorf("resolving secret '%s': command failed: %w", original, err)
	}
	out, readErr := readBoundedSecret(stdout, MaximumAtomicResolvedBytes)
	if readErr != nil && cmd.Cancel != nil {
		_ = cmd.Cancel()
	}
	waitErr := cmd.Wait()
	if readErr != nil || waitErr != nil {
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return "", fmt.Errorf("resolving secret '%s': command timed out after %s", original, timeout)
		}
		return "", fmt.Errorf("resolving secret '%s': command failed: %w", original, errors.Join(readErr, waitErr))
	}

	value := strings.TrimSpace(string(out))
	logResolved(ctx, "resolved secret via command '%s'", parts[0])
	return value, nil
}
