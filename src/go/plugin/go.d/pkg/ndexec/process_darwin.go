// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"errors"
	"fmt"
	"syscall"

	"golang.org/x/sys/unix"
)

type exitObserver struct{ fd int }

func newExitObserver() (exitObserver, error) {
	// Kqueue has no atomic close-on-exec flag on Darwin.
	syscall.ForkLock.RLock()
	defer syscall.ForkLock.RUnlock()
	fd, err := unix.Kqueue()
	if err == nil {
		unix.CloseOnExec(fd)
	}
	return exitObserver{
		fd: fd,
	}, err
}
func (o exitObserver) close() error { return unix.Close(o.fd) }
func (o exitObserver) wait(pid int) error {
	change := unix.Kevent_t{
		Ident:  uint64(pid),
		Filter: unix.EVFILT_PROC,
		Flags:  unix.EV_ADD | unix.EV_ONESHOT,
		Fflags: unix.NOTE_EXIT,
	}
	changes := []unix.Kevent_t{change}
	events := make([]unix.Kevent_t, 1)
	for {
		n, err := unix.Kevent(o.fd, changes, events, nil)
		if errors.Is(err, unix.EINTR) {
			continue
		}
		// An already-exiting child may no longer accept a filter. It is still
		// unreaped, so its identity remains reserved for the owner's cleanup.
		if errors.Is(err, unix.ESRCH) {
			return nil
		}
		if err != nil {
			return err
		}
		changes = nil
		if n == 0 {
			continue
		}
		event := events[0]
		if event.Flags&unix.EV_ERROR != 0 {
			if event.Data == int64(unix.ESRCH) {
				return nil
			}
			return fmt.Errorf("observe process exit: %w", unix.Errno(event.Data))
		}
		if event.Fflags&unix.NOTE_EXIT != 0 {
			return nil
		}
	}
}

// Darwin killpg1 skips zombies and returns EPERM when none of the group can
// receive a signal. Suppress that case only with a successful group snapshot
// containing our pinned zombie leader and no potentially live member.
func normalizeGroupKillError(pid int, err error) error {
	if !errors.Is(err, unix.EPERM) {
		return err
	}
	members, queryErr := unix.SysctlKinfoProcSlice("kern.proc.pgrp", pid)
	if queryErr != nil {
		return err
	}
	const zombie = 5 // SZOMB in Darwin sys/proc.h.
	leaderFound := false
	for _, member := range members {
		if member.Proc.P_stat != zombie {
			return err
		}
		if int(member.Proc.P_pid) == pid {
			leaderFound = true
		}
	}
	if leaderFound {
		return nil
	}
	return err
}
