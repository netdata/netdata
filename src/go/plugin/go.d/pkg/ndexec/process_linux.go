// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"errors"

	"golang.org/x/sys/unix"
)

type exitObserver struct{}

func newExitObserver() (exitObserver, error) { return exitObserver{}, nil }
func (exitObserver) close() error            { return nil }
func (exitObserver) wait(pid int) error {
	var info unix.Siginfo
	for {
		err := unix.Waitid(unix.P_PID, pid, &info, unix.WEXITED|unix.WNOWAIT, nil)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}
