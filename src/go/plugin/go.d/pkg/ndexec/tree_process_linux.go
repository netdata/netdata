// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"os"
	"os/exec"
	"syscall"
)

// StartUnprivilegedProcessTree starts an unprivileged command through nd-run's
// opt-in Linux subreaper. Compatible procfs/child-enumeration support is checked
// by the helper before it launches the target. There is no weaker fallback.
// Nil stdio uses the null device; the caller retains ownership of supplied files.
func StartUnprivilegedProcessTree(
	ctx context.Context,
	opts ProcessOptions,
	binPath string,
	args ...string,
) (*TreeProcess, error) {
	return startProcessTree(ctx, defaultRunner.ndRunPath, opts, binPath, args...)
}

func startProcessTree(
	ctx context.Context,
	helper string,
	opts ProcessOptions,
	binPath string,
	args ...string,
) (*TreeProcess, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	controlRead, controlWrite, err := os.Pipe()
	if err != nil {
		return nil, treeStartError(err)
	}
	defer controlRead.Close()
	statusRead, statusWrite, err := os.Pipe()
	if err != nil {
		controlWrite.Close()
		return nil, treeStartError(err)
	}
	defer statusWrite.Close()
	started := false
	defer func() {
		if !started {
			controlWrite.Close()
			statusRead.Close()
		}
	}()
	if opts.Stdin == nil || opts.Stdout == nil || opts.Stderr == nil {
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return nil, treeStartError(err)
		}
		defer null.Close()
		if opts.Stdin == nil {
			opts.Stdin = null
		}
		if opts.Stdout == nil {
			opts.Stdout = null
		}
		if opts.Stderr == nil {
			opts.Stderr = null
		}
	}
	argv := append([]string{"--supervise-tree", "3", "4", "--", binPath}, args...)
	cmd := exec.Command(helper, argv...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = opts.Stdin, opts.Stdout, opts.Stderr
	cmd.ExtraFiles = []*os.File{controlRead, statusWrite}
	cmd.SysProcAttr = &syscall.SysProcAttr{
		Setpgid: true,
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := cmd.Start(); err != nil {
		return nil, treeStartError(err)
	}
	started = true
	return ownTreeProcess(ctx, cmd, controlWrite, statusRead), nil
}
