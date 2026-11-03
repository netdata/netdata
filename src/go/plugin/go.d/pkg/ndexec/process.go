// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
	"syscall"
)

// ProcessOptions supplies child stdio. Nil files use the null device. The caller
// retains ownership of its files and may close child-side pipe ends after Start.
// Files avoid copy goroutines whose lifetime could depend on surviving children.
type ProcessOptions struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

// Process owns termination and reaping of a command and its contained descendants,
// started through nd-run or ndsudo. It must not be copied. Wait and Close may run concurrently.
type Process struct {
	handle      processHandle
	mu          sync.Mutex
	stopped     bool
	stopErr     error
	done        chan struct{}
	watcherDone chan struct{}
	err         error
}

type processHandle interface {
	observeExit() error // MUST NOT reap the leader or release its identity.
	terminate() error
	reap() error
	release() error
}

// StartUnprivilegedProcess starts a command through nd-run with owned process
// containment. Cancellation, Close, or leader exit terminates contained children.
// Wait reports completion after termination requests and leader reaping. Unlike exec.Cmd, this
// API never exposes an independent reaper or a reusable numeric process ID.
func StartUnprivilegedProcess(
	ctx context.Context,
	opts ProcessOptions,
	binPath string,
	args ...string,
) (*Process, error) {
	return startProcess(ctx, opts, defaultRunner.ndRunPath, append([]string{binPath}, args...), nil)
}

// StartNDSudoProcess starts a command exposed through ndsudo with the ownership of
// StartUnprivilegedProcess. ndsudo runs the command as root, and an unprivileged caller
// cannot signal it; the kernel's denial of termination is not an error. With a pipe as
// Stdout whose read end only the caller holds, the command ends when it exits or dies of
// SIGPIPE at its next write after the caller closes that read end, unless it handles
// SIGPIPE or EPIPE itself. Root descendants that outlive the command are neither
// terminated nor reported. Until the command ends, Wait and Close block, so a caller that
// must not hang bounds its own wait. A caller allowed to signal root processes terminates
// the command and its descendants as StartUnprivilegedProcess does.
func StartNDSudoProcess(
	ctx context.Context,
	opts ProcessOptions,
	command string,
	args ...string,
) (*Process, error) {
	signalsDenied := ndsudoSignalsDenied
	return startProcess(ctx, opts, defaultRunner.ndSudoPath, append([]string{command}, args...),
		func(handle processHandle) processHandle {
			return ndsudoProcess{
				processHandle: handle,
				signalsDenied: signalsDenied,
			}
		})
}

// ndsudoSignalsDenied is set by DenyNDSudoSignalsForTests.
var ndsudoSignalsDenied bool

// ndsudoProcess is the handle of a command that ndsudo runs as root.
type ndsudoProcess struct {
	processHandle
	signalsDenied bool
}

func (p ndsudoProcess) terminate() error {
	if p.signalsDenied {
		return nil
	}
	// A denied group kill means no group member could be signaled, so the direct
	// kill of the leader joined to it was denied as well.
	if err := p.processHandle.terminate(); !errors.Is(err, syscall.EPERM) {
		return err
	}
	return nil
}

// startProcess starts helper with argv under owned containment. wrap, when non-nil,
// adapts the handle's termination to the helper.
func startProcess(
	ctx context.Context,
	opts ProcessOptions,
	helper string,
	argv []string,
	wrap func(processHandle) processHandle,
) (*Process, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if opts.Stdin == nil || opts.Stdout == nil || opts.Stderr == nil {
		null, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
		if err != nil {
			return nil, err
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
	handle, err := startOwnedProcess(helper, argv, opts)
	if err != nil {
		return nil, fmt.Errorf("start owned command: %w", err)
	}
	if wrap != nil {
		handle = wrap(handle)
	}
	return ownProcess(ctx, handle), nil
}

func ownProcess(ctx context.Context, handle processHandle) *Process {
	p := &Process{
		handle:      handle,
		done:        make(chan struct{}),
		watcherDone: make(chan struct{}),
	}
	go func() {
		defer close(p.watcherDone)
		select {
		case <-ctx.Done():
			_ = p.stop()
		case <-p.done:
		}
	}()
	go func() {
		observeErr := handle.observeExit()
		// Group identity remains pinned until termination has permanently disarmed
		// further signals. Only this goroutine may then reap and release the leader.
		stopErr := p.stop()
		reapErr := handle.reap()
		releaseErr := handle.release()
		p.err = errors.Join(observeErr, stopErr, reapErr, releaseErr)
		close(p.done)
	}()
	return p
}

func (p *Process) stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()
	if !p.stopped {
		p.stopped = true
		p.stopErr = p.handle.terminate()
	}
	return p.stopErr
}

// Wait joins leader reaping, termination requests, and the cancellation watcher.
// It does not independently await each descendant. Repeated calls return the same
// exit/cleanup error. A nonzero exit is an exec.ExitError.
func (p *Process) Wait() error {
	<-p.done
	<-p.watcherDone
	return p.err
}

// Close requests termination of the owned tree and then calls Wait. It is idempotent;
// a process killed by Close normally produces an exit error, also returned by Wait.
func (p *Process) Close() error {
	_ = p.stop()
	return p.Wait()
}
