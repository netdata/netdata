// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package logger

import (
	"os"
	"strconv"
	"strings"
	"syscall"
)

func isStderrConnectedToJournal() bool {
	stream := os.Getenv("JOURNAL_STREAM")
	if stream == "" {
		return false
	}

	idx := strings.IndexByte(stream, ':')
	if idx <= 0 {
		return false
	}

	dev, ino := stream[:idx], stream[idx+1:]

	var stat syscall.Stat_t
	if err := syscall.Fstat(int(os.Stderr.Fd()), &stat); err != nil {
		return false
	}

	return dev == strconv.Itoa(int(stat.Dev)) && ino == strconv.FormatUint(stat.Ino, 10)
}
