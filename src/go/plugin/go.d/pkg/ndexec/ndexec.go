// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

const stderrLimit = 8 << 10 // 8 KiB

// RunUnprivileged runs binPath via nd-run with a timeout.
// Returns stdout. On error, wraps the original error and includes a trimmed stderr snippet.
func RunUnprivileged(log *logger.Logger, timeout time.Duration, binPath string, args ...string) ([]byte, error) {
	ndrun := filepath.Join(buildinfo.NetdataBinDir, "nd-run")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	argv := append([]string{binPath}, args...)
	ex := exec.CommandContext(ctx, ndrun, argv...)

	log.Debugf("executing: %v", ex)

	var stderr bytes.Buffer
	ex.Stderr = &stderr

	out, err := ex.Output()
	if err != nil {
		s := stderr.String()
		if len(s) > stderrLimit {
			s = s[:stderrLimit] + "… (truncated)"
		}
		return nil, fmt.Errorf("RunUnprivileged: %v: %w (stderr: %s)", ex, err, strings.TrimSpace(s))
	}
	return out, nil
}

// RunNDSudo runs cmd via ndsudo with a timeout.
// Returns stdout. On error, wraps the original error and includes a trimmed stderr snippet.
func RunNDSudo(log *logger.Logger, timeout time.Duration, cmd string, args ...string) ([]byte, error) {
	ndsudo := filepath.Join(buildinfo.PluginsDir, "ndsudo")
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	argv := append([]string{cmd}, args...)
	ex := exec.CommandContext(ctx, ndsudo, argv...)

	log.Debugf("executing: %v", ex)

	var stderr bytes.Buffer
	ex.Stderr = &stderr

	out, err := ex.Output()
	if err != nil {
		s := stderr.String()
		if len(s) > stderrLimit {
			s = s[:stderrLimit] + "… (truncated)"
		}
		return nil, fmt.Errorf("RunNDSudo: %v: %w (stderr: %s)", ex, err, strings.TrimSpace(s))
	}
	return out, nil
}

func CommandUnprivileged(ctx context.Context, logger *logger.Logger, arg ...string) *exec.Cmd {
	ndrunPath := filepath.Join(buildinfo.NetdataBinDir, "nd-run")

	cmd := exec.CommandContext(ctx, ndrunPath, arg...)
	if logger != nil {
		logger.Debugf("executing '%s'", cmd)
	}

	return cmd
}

// CommandNDSudo runs a command via the ndsudo helper and returns the exec.Cmd instance.
//
// ctx is a context.Context to use to run the command. logger is a Logger
// instance to use to log the command to be executed.  timeout indicates
// the timeout for the command. arg is a list of the command arguments,
// with the first string in the slice being the command to run.
//
// This invokes the command and logs a debug message that the command
// is being executed, and then returns the exec.Cmd object for the command.
func CommandNDSudo(ctx context.Context, logger *logger.Logger, arg ...string) *exec.Cmd {
	ndsudoPath := filepath.Join(buildinfo.PluginsDir, "ndsudo")

	cmd := exec.CommandContext(ctx, ndsudoPath, arg...)
	if logger != nil {
		logger.Debugf("executing '%s'", cmd)
	}

	return cmd
}
