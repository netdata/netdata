//go:build !windows
// +build !windows

// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"os"
	"runtime"
	"syscall"
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
	if ps == nil {
		return ResourceUsage{}
	}
	usage := ResourceUsage{}
	if sys := ps.SysUsage(); sys != nil {
		if ru, ok := sys.(*syscall.Rusage); ok && ru != nil {
			usage.User = time.Duration(ru.Utime.Sec)*time.Second + time.Duration(ru.Utime.Usec)*time.Microsecond
			usage.System = time.Duration(ru.Stime.Sec)*time.Second + time.Duration(ru.Stime.Usec)*time.Microsecond
			usage.MaxRSSBytes = convertMaxRSS(int64(ru.Maxrss))
			usage.ReadBytes = blocksToBytes(int64(ru.Inblock))
			usage.WriteBytes = blocksToBytes(int64(ru.Oublock))
		}
	}
	return usage
}

// convertMaxRSS normalizes platform-specific MaxRSS units: Linux and BSD
// derivatives report KiB while Darwin already uses bytes.
func convertMaxRSS(raw int64) int64 {
	bytes := raw
	switch runtime.GOOS {
	case "linux", "android", "freebsd", "openbsd", "netbsd", "dragonfly":
		bytes *= 1024
	}
	return bytes
}

const blockSize = 512

func blocksToBytes(blocks int64) int64 {
	if blocks <= 0 {
		return 0
	}
	return blocks * blockSize
}
