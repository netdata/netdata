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
	var status unix.WaitStatus
	for {
		_, err := unix.Wait4(pid, &status, unix.WNOWAIT, nil)
		if !errors.Is(err, unix.EINTR) {
			return err
		}
	}
}
