// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || darwin || freebsd

package ndexec

import (
	"errors"
	"os/exec"
	"syscall"
)

type unixProcess struct {
	cmd      *exec.Cmd
	observer exitObserver
}

func startOwnedProcess(path string, args []string, opts ProcessOptions) (processHandle, error) {
	observer, err := newExitObserver()
	if err != nil {
		return nil, err
	}
	cmd := exec.Command(path, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Stdin, opts.Stdout, opts.Stderr
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	if err := cmd.Start(); err != nil {
		_ = observer.close()
		return nil, err
	}
	return &unixProcess{
		cmd:      cmd,
		observer: observer,
	}, nil
}

func (p *unixProcess) observeExit() error { return p.observer.wait(p.cmd.Process.Pid) }
func (p *unixProcess) reap() error        { return p.cmd.Wait() }
func (p *unixProcess) release() error     { return p.observer.close() }
func (p *unixProcess) terminate() error {
	// The sole reaper cannot run until Process.stop disarms all future signals.
	// The unreaped leader reserves this PGID even if no live member remains.
	err := normalizeGroupKillError(p.cmd.Process.Pid, syscall.Kill(-p.cmd.Process.Pid, syscall.SIGKILL))
	if errors.Is(err, syscall.ESRCH) {
		return nil
	}
	if err != nil {
		return errors.Join(err, p.cmd.Process.Kill())
	}
	return nil
}
