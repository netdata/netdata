// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

const stderrLimit = 8 << 10 // 8 KiB

// Runner holds helper paths for execution.
type runner struct {
	ndRunPath  string
	ndSudoPath string
}

func newRunnerFromBuildinfo() *runner {
	var sfx string
	if runtime.GOOS == "windows" {
		sfx = ".exe"
	}
	return &runner{
		ndRunPath:  filepath.Join(buildinfo.NetdataBinDir, "nd-run"+sfx),
		ndSudoPath: filepath.Join(buildinfo.PluginsDir, "ndsudo"+sfx),
	}
}

var defaultRunner = newRunnerFromBuildinfo()

// RunUnprivileged runs binPath via nd-run with a timeout.
// Returns stdout. On error, wraps the original error and includes a trimmed stderr snippet.
func RunUnprivileged(log *logger.Logger, timeout time.Duration, binPath string, args ...string) ([]byte, error) {
	out, _, err := RunUnprivilegedWithCmd(log, timeout, binPath, args...)
	return out, err
}

// RunNDSudo runs cmd via ndsudo with a timeout.
// Returns stdout. On error, wraps the original error and includes a trimmed stderr snippet
func RunNDSudo(log *logger.Logger, timeout time.Duration, cmd string, args ...string) ([]byte, error) {
	out, _, err := RunNDSudoWithCmd(log, timeout, cmd, args...)
	return out, err
}

// RunUnprivilegedWithCmd runs binPath via nd-run and also returns the formatted command string.
func RunUnprivilegedWithCmd(log *logger.Logger, timeout time.Duration, binPath string, args ...string) ([]byte, string, error) {
	argv := append([]string{binPath}, args...)
	return defaultRunner.run(log, timeout, defaultRunner.ndRunPath, "RunUnprivileged", argv...)
}

// RunNDSudoWithCmd runs cmd via ndsudo and also returns the formatted command string.
func RunNDSudoWithCmd(log *logger.Logger, timeout time.Duration, cmd string, args ...string) ([]byte, string, error) {
	argv := append([]string{cmd}, args...)
	return defaultRunner.run(log, timeout, defaultRunner.ndSudoPath, "RunNDSudo", argv...)
}

func (r *runner) run(log *logger.Logger, timeout time.Duration, helperPath, label string, argv ...string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ex := exec.CommandContext(ctx, helperPath, argv...) // argv comes from trusted sources; no shell, args passed separately

	log.Debugf("executing: %v", ex)

	var stderr bytes.Buffer
	ex.Stderr = &stderr

	cmdStr := ex.String()

	out, err := ex.Output()
	if err != nil {
		s := stderr.String()
		if len(s) > stderrLimit {
			s = s[:stderrLimit] + "… (truncated)"
		}
		// Normalize context-related errors so callers can errors.Is(..., context.DeadlineExceeded)
		if ctx.Err() != nil {
			err = ctx.Err()
		}

		return out, cmdStr, fmt.Errorf("%s: %v: %w (stderr: %s)", label, ex, err, strings.TrimSpace(s))
	}

	return out, cmdStr, nil
}
