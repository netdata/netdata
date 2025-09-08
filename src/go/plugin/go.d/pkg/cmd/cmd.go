// SPDX-License-Identifier: GPL-3.0-or-later

package cmd

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/buildinfo"
	"github.com/netdata/netdata/go/plugins/logger"
)

// RunUnprivileged runs a command without additional privielges.
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

	cmd := exec.CommandContext(ctx, arg[0], arg[1:]...)
	if logger != nil {
		logger.Debugf("executing '%s'", cmd)
	}

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
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
func RunNDSudo(logger *logger.Logger, timeout time.Duration, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	ndsudoPath := filepath.Join(buildinfo.NetdataBinDir, "ndsudo")

	cmd := exec.CommandContext(ctx, ndsudoPath, args...)
	logger.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("error on '%s': %v", cmd, err)
	}

	return bs, nil
}
