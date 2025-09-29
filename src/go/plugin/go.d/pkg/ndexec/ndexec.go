// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
)

// CommandUnprivileged runs a command without any extra privileges and
// returns the exec.Cmd instance.
//
// ctx is a context.Context to use to run the command. logger is a Logger
// instance to use to log the command to be executed.  timeout indicates
// the timeout for the command. arg is a list of the command arguments,
// with the first string in the slice being the command to run.
//
// This invokes the command and logs a debug message that the command
// is being executed, and then returns the exec.Cmd object for the command.
func CommandUnprivileged(ctx context.Context, logger *logger.Logger, arg ...string) *exec.Cmd {
	ndrunPath := filepath.Join(buildinfo.NetdataBinDir, "nd-run")

	cmd := exec.CommandContext(ctx, ndrunPath, arg...)
	if logger != nil {
		logger.Debugf("executing '%s'", cmd)
	}

	return cmd
}

// RunUnprivileged runs a command without any inherited privileges via
// the nd-run helper.
//
// logger is a Logger instance to use to log the command to be executed.
// timeout indicates the timeout for the command. arg is a list of the
// command arguments, with the first string in the slice being the command
// to run.
//
// This handles constructing the context for execution, logs a debug
// message that the command is being executed, and checks for errors in
// the command invocation, then returns the command output.
func RunUnprivileged(logger *logger.Logger, timeout time.Duration, arg ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := CommandUnprivileged(ctx, logger, arg...)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
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

// RunNDSudo runs a command via the ndsudo helper.
//
// logger is a Logger instance to use to log the command to be executed.
// timeout indicates the timeout for the command. arg is a list of the
// command arguments, with the first string in the slice being the command
// to run.
//
// This handles constructing the context for execution, logs a debug
// message that the command is being executed, and checks for errors in
// the command invocation, then returns the command output.
//
// The command to be run must also be properly handled by ndsudo.
func RunNDSudo(logger *logger.Logger, timeout time.Duration, arg ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := CommandNDSudo(ctx, logger, arg...)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
