//go:build !windows

package jobmgrtest

import "syscall"

func syscallSIGHUP() syscall.Signal {
	return syscall.SIGHUP
}

func syscallSIGTERM() syscall.Signal {
	return syscall.SIGTERM
}
