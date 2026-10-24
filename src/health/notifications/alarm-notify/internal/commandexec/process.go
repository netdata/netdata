// SPDX-License-Identifier: GPL-3.0-or-later

package commandexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"sync"
	"time"
)

// Runner owns admitted subprocesses and their cleanup. Its zero value is ready to use.
// A Runner must not be copied after first use.
type Runner struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

// CloseAndWait rejects new calls and waits for all admitted subprocess cleanup.
// It does not cancel running commands; callers must cancel their contexts.
func (p *Runner) CloseAndWait() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.wg.Wait()
}

// Run executes a foreground command and discards its output.
func (p *Runner) Run(ctx context.Context, executable string, args, env []string, input io.Reader) error {
	return p.runProcess(ctx, executable, args, env, input, false, nil)
}

// RunWithPrivateHome executes a command with a temporary HOME removed before return.
func (p *Runner) RunWithPrivateHome(ctx context.Context, executable string, args, env []string, input io.Reader) error {
	return p.runProcess(ctx, executable, args, env, input, true, nil)
}

// RunSession owns a foreground protocol exchange; both directions close before admission is released.
func (p *Runner) RunSession(ctx context.Context, executable string, args, env []string, exchange func(io.Reader, io.WriteCloser) error) error {
	return p.runProcess(ctx, executable, args, env, nil, false, exchange)
}

func (p *Runner) runProcess(ctx context.Context, executable string, args, env []string, input io.Reader, privateHome bool, exchange func(io.Reader, io.WriteCloser) error) (result error) {
	p.mu.Lock()
	if err := ctx.Err(); err != nil {
		p.mu.Unlock()
		return err
	}
	if p.closed {
		p.mu.Unlock()
		return context.Canceled
	}
	p.wg.Add(1)
	p.mu.Unlock()
	defer p.wg.Done()

	processCtx, cancel := context.WithCancel(ctx)
	defer cancel()
	cmd := exec.CommandContext(processCtx, executable, args...)
	cmd.Env = env
	cmd.Stdin = input
	// Nil stdout/stderr connect directly to the null device: no buffering or copying pipes.
	// Bound stdin copying if a misbehaving child exits while descendants retain its pipe.
	cmd.WaitDelay = 250 * time.Millisecond
	if err := configureCommandProcess(cmd); err != nil {
		return err
	}
	var childInput, sessionInput, sessionOutput, childOutput *os.File
	if exchange != nil {
		var err error
		childInput, sessionInput, err = os.Pipe()
		if err != nil {
			return errors.New("could not create command input pipe")
		}
		defer childInput.Close()
		defer sessionInput.Close()
		sessionOutput, childOutput, err = os.Pipe()
		if err != nil {
			return errors.New("could not create command output pipe")
		}
		defer sessionOutput.Close()
		defer childOutput.Close()
		// OS files avoid exec's copying goroutines: this call owns and joins all session I/O.
		cmd.Stdin, cmd.Stdout = childInput, childOutput
	}
	if privateHome {
		home, err := os.MkdirTemp("", "alarm-notify-home-")
		if err != nil {
			return errors.New("could not create private command home")
		}
		// Remove credential caches before the admitted call releases CloseAndWait.
		defer func() {
			if err := os.RemoveAll(home); err != nil {
				result = errors.Join(result, errors.New("could not remove private command home"))
			}
		}()
		cmd.Env = append(cmd.Env, "HOME="+home)
	}
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("could not start command; check executable and permissions")
	}
	var sessionDone chan error
	if exchange != nil {
		_ = childInput.Close()
		_ = childOutput.Close()
		sessionDone = make(chan error, 1)
		go func() {
			err := exchange(sessionOutput, sessionInput)
			_ = sessionInput.Close()
			_ = sessionOutput.Close()
			if err != nil {
				cancel()
			}
			sessionDone <- err
		}()
	}
	err := cmd.Wait()
	var sessionErr error
	if sessionDone != nil {
		// A misbehaving child may exit while descendants keep its pipes open.
		timer := time.NewTimer(cmd.WaitDelay)
		defer timer.Stop()
		select {
		case sessionErr = <-sessionDone:
		case <-timer.C:
			_ = sessionInput.Close()
			_ = sessionOutput.Close()
			<-sessionDone
			sessionErr = errors.New("command protocol did not complete")
		}
	}
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return sessionErr
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if exit.ExitCode() >= 0 {
			err = fmt.Errorf("command exited with status %d", exit.ExitCode())
		} else {
			err = errors.New("command terminated by signal")
		}
	} else {
		err = errors.New("command input did not complete")
	}
	if sessionErr != nil {
		return fmt.Errorf("%w; %w", sessionErr, err)
	}
	return err
}
