// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"errors"
	"fmt"
	"os"
	"sync"
)

// ProcessOptions supplies child stdio. Nil files use the null device. The caller
// retains ownership of its files and may close child-side pipe ends after Start.
// Files avoid copy goroutines whose lifetime could depend on surviving children.
type ProcessOptions struct {
	Stdin  *os.File
	Stdout *os.File
	Stderr *os.File
}

// Process owns termination and reaping of an unprivileged command and its
// contained descendants. It must not be copied. Wait and Close may run concurrently.
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
	argv := append([]string{binPath}, args...)
	handle, err := startOwnedProcess(defaultRunner.ndRunPath, argv, opts)
	if err != nil {
		return nil, fmt.Errorf("start owned command: %w", err)
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
