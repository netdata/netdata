// SPDX-License-Identifier: GPL-3.0-or-later

package nagios

import (
	"context"
	"errors"
)

var errNagiosCheckTimeout = errors.New("nagios: check timed out")

func exitCodeFromError(err error) int {
	if err == nil {
		return 0
	}
	var exitErr interface{ ExitCode() int }
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	return -1
}

func serviceStateFromExecution(exitCode int, err error) string {
	if errors.Is(err, errNagiosCheckTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return nagiosStateUnknown
	}
	switch exitCode {
	case 0:
		return nagiosStateOK
	case 1:
		return nagiosStateWarning
	case 2:
		return nagiosStateCritical
	case 3:
		return nagiosStateUnknown
	default:
		return nagiosStateUnknown
	}
}

func jobStateFromExecution(exitCode int, err error) string {
	if errors.Is(err, errNagiosCheckTimeout) || errors.Is(err, context.DeadlineExceeded) {
		return jobStateTimeout
	}
	return serviceStateFromExecution(exitCode, err)
}

func classifyRunError(ctx context.Context, exitCode int, err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, context.Canceled) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
		return err
	}
	if errors.Is(err, errNagiosCheckTimeout) {
		return nil
	}
	if exitCode >= 0 && exitCode <= 3 {
		return nil
	}
	if errors.Is(err, context.DeadlineExceeded) {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}
	}
	return err
}
