// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin

package notifier

import (
	"errors"
	"os"
	"os/exec"
	"syscall"
)

func configureCommandProcess(cmd *exec.Cmd) error {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		// Foreground commands must wait for their children and must not detach from this group.
		err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
		if errors.Is(err, syscall.ESRCH) {
			return os.ErrProcessDone
		}
		return err
	}
	return nil
}
