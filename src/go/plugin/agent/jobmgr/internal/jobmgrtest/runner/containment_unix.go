//go:build aix || android || darwin || dragonfly || freebsd || illumos || ios || linux || netbsd || openbsd || solaris

package runner

import (
	"errors"
	"os/exec"
	"syscall"
)

func configureContainment(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
}

func killContained(command *exec.Cmd) error {
	if command.Process == nil {
		return nil
	}
	err := syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	return err
}
