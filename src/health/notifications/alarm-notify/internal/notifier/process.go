// SPDX-License-Identifier: GPL-3.0-or-later

package notifier

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"sync"
	"time"
)

// Only admitted subprocess calls delay Run's return, never blocked input or HTTP reads.
type commandProcesses struct {
	mu     sync.Mutex
	closed bool
	wg     sync.WaitGroup
}

func (p *commandProcesses) closeAndWait() {
	p.mu.Lock()
	p.closed = true
	p.mu.Unlock()
	p.wg.Wait()
}

func (p *commandProcesses) run(ctx context.Context, executable string, args, env []string, input io.Reader) error {
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

	cmd := exec.CommandContext(ctx, executable, args...)
	cmd.Env = env
	cmd.Stdin = input
	// Nil stdout/stderr connect directly to the null device: no buffering or copying pipes.
	// Bound stdin copying if a misbehaving child exits while descendants retain its pipe.
	cmd.WaitDelay = 250 * time.Millisecond
	if err := configureCommandProcess(cmd); err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		return errors.New("could not start command; check executable and permissions")
	}
	err := cmd.Wait()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err == nil {
		return nil
	}
	var exit *exec.ExitError
	if errors.As(err, &exit) {
		if exit.ExitCode() >= 0 {
			return fmt.Errorf("command exited with status %d", exit.ExitCode())
		}
		return errors.New("command terminated by signal")
	}
	return errors.New("command input did not complete")
}
