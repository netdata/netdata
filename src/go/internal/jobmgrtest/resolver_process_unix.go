//go:build !windows

package jobmgrtest

import (
	"errors"
	"os"
	"syscall"
)

func ResolverDriverSupported() bool {
	return true
}

func resolverProcessGone(pid int) bool {
	err := syscall.Kill(pid, 0)
	return errors.Is(err, syscall.ESRCH) ||
		errors.Is(err, os.ErrProcessDone)
}
