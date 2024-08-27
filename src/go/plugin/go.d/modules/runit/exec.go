// SPDX-License-Identifier: GPL-3.0-or-later

package runit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os/exec"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

func newSvCliExec(ndsudoPath string, timeout time.Duration, log *logger.Logger) *svCliExec {
	return &svCliExec{
		Logger:     log,
		ndsudoPath: ndsudoPath,
		timeout:    timeout,
	}
}

type svCliExec struct {
	*logger.Logger

	ndsudoPath string
	timeout    time.Duration
}

func (e *svCliExec) StatusAll(dir string) ([]byte, error) {
	return e.execute("sv-status-all", "--serviceDir", dir)
}

// `sv` always output "warning: " at beginning of each line on stderr.
var svStderrPrefix = []byte("warning: ")

func (e *svCliExec) execute(args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), e.timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, e.ndsudoPath, args...)
	e.Debugf("executing '%s'", cmd)

	bs, err := cmd.Output()
	if err != nil {
		// Exit codes:
		// - None: means context deadline exceeded.
		// - 1-6 are used by `ndsudo`: means permanent fatal error.
		// - 0-99 are used by `sv`: means (partially) okay.
		// - 100 is used by `sv`: means temporary fatal error.
		// - 126, 127 is used by `sh`: means missing `sv` executable.
		// - 129-255: means killed by a signal.
		// - Other: means unknown error.
		// Stderr output:
		// - `ndsudo` never begins line with "warning: ".
		// - `sv status` always begins line with "warning: ".
		var errExit *exec.ExitError
		if errors.As(err, &errExit) {
			e.Warning(string(errExit.Stderr))
		}
		switch {
		case errExit == nil: // Context deadline exceeded while running ndsudo or sh or sv.
			e.Errorf("error on '%s': %v", cmd, err)
		case errExit.ExitCode() > 128: // ndsudo or sh or sv was killed by a signal.
			e.Errorf("killed on '%s': %v", cmd, err)
		case len(errExit.Stderr) > 0 && !bytes.HasPrefix(errExit.Stderr, svStderrPrefix): // ndsudo error.
			return nil, fmt.Errorf("ndsudo error on '%s': %w", cmd, err)
		case errExit.ExitCode() == 126 || errExit.ExitCode() == 127: // sh error: command not found.
			return nil, fmt.Errorf("shell error on '%s': %w", cmd, err)
		case errExit.ExitCode() <= 100: // sv partial/temporary error.
			// Ignore.
		default: // Unknown error.
			return nil, fmt.Errorf("unknown error on '%s': %w", cmd, err)
		}
	}

	return bs, nil
}
