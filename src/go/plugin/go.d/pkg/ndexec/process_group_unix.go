// SPDX-License-Identifier: GPL-3.0-or-later

//go:build !windows

package ndexec

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
	"time"
)

func configureCommandCancellation(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}

	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	cmd.WaitDelay = 250 * time.Millisecond
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil && !errors.Is(err, syscall.ESRCH) {
			return resolveDirectKillFallback(cmd.Process.Kill())
		}
		return nil
	}
}

func resolveDirectKillFallback(killErr error) error {
	if killErr == nil {
		return nil
	}
	if errors.Is(killErr, os.ErrProcessDone) {
		return os.ErrProcessDone
	}
	return killErr
}
