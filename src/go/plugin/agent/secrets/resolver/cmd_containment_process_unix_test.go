//go:build !windows

package secretresolver

import (
	"errors"
	"os"
	"syscall"
)

func resolverContainmentSupported() bool {
	return true
}

func resolverProcessGone(pid int) bool {
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH) ||
		errors.Is(err, os.ErrProcessDone)
}
