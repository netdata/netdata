// SPDX-License-Identifier: GPL-3.0-or-later

package main

import (
	"context"
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

// The private channel carries only EOF, never a payload. Polling hangup avoids
// blocking reads and changing status flags shared with inherited stdin. A
// separate wakeup pipe joins the watcher on return even with a live writer.
func runtimeInputContext(parent context.Context, input *os.File) (context.Context, func(), error) {
	raw, err := input.SyscallConn()
	if err != nil {
		return nil, nil, err
	}
	fd := -1
	var dupErr error
	if err := raw.Control(func(inputFD uintptr) {
		fd, dupErr = unix.FcntlInt(inputFD, unix.F_DUPFD_CLOEXEC, 0)
	}); err != nil {
		return nil, nil, err
	}
	if dupErr != nil {
		return nil, nil, dupErr
	}
	var stat unix.Stat_t
	if err := unix.Fstat(fd, &stat); err != nil {
		return nil, nil, errors.Join(err, unix.Close(fd))
	}
	mode := stat.Mode & unix.S_IFMT
	if mode == unix.S_IFSOCK {
		socketType, err := unix.GetsockoptInt(fd, unix.SOL_SOCKET, unix.SO_TYPE)
		if err != nil {
			return nil, nil, errors.Join(err, unix.Close(fd))
		}
		if socketType != unix.SOCK_STREAM {
			mode = 0
		}
	}
	if mode != unix.S_IFIFO && mode != unix.S_IFSOCK {
		return nil, nil, errors.Join(errors.New("runtime stdin control channel must be a pipe or stream socket"), unix.Close(fd))
	}
	var wakeup [2]int
	if err := unix.Pipe2(wakeup[:], unix.O_CLOEXEC); err != nil {
		return nil, nil, errors.Join(err, unix.Close(fd))
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})
	go func() {
		defer close(done)
		defer cancel()
		fds := []unix.PollFd{
			{Fd: int32(fd), Events: unix.POLLRDHUP},
			{Fd: int32(wakeup[0]), Events: unix.POLLIN},
		}
		for {
			if _, err := unix.Poll(fds, -1); err != nil {
				if errors.Is(err, unix.EINTR) {
					continue
				}
				return
			}
			if fds[0].Revents != 0 || fds[1].Revents != 0 {
				return
			}
		}
	}()
	// The command calls finish once on every return. Wake and join the watcher
	// before closing its descriptors so Poll cannot observe reused descriptor IDs.
	return ctx, func() {
		cancel()
		_ = unix.Close(wakeup[1])
		<-done
		_ = unix.Close(fd)
		_ = unix.Close(wakeup[0])
	}, nil
}
