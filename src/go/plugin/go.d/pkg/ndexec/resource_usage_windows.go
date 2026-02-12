//go:build windows
// +build windows

// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"os"
	"time"
)

// ResourceUsage captures OS-reported resource counters for a completed process.
type ResourceUsage struct {
	User        time.Duration
	System      time.Duration
	MaxRSSBytes int64
	ReadBytes   int64
	WriteBytes  int64
}

func (u ResourceUsage) totalCPU() time.Duration {
	return u.User + u.System
}

func extractUsage(ps *os.ProcessState) ResourceUsage {
	// Windows doesn't expose POSIX rusage fields; return zeroed usage.
	return ResourceUsage{}
}

func convertMaxRSS(raw int64) int64 {
	return raw
}

const blockSize = 512

func blocksToBytes(blocks int64) int64 {
	if blocks <= 0 {
		return 0
	}
	return blocks * blockSize
}
