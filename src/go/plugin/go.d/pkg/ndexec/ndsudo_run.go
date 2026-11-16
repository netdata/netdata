// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
)

// ErrPreviousRunNotExited reports that an earlier RunNDSudo call with the same command and arguments timed out and its
// command has not exited yet. No further instance is started meanwhile.
var ErrPreviousRunNotExited = errors.New("a previous run that timed out has not exited")

// ndsudoExitWait bounds two waits of an ndsudo run: for the command to exit after the timeout, and for its output to
// end after it exits. It matches the WaitDelay of the other Run helpers.
const ndsudoExitWait = 250 * time.Millisecond

// ndsudoUnreaped counts, per command line, the timed-out ndsudo runs whose commands are still running, so a command
// that hangs on every call does not accumulate root processes.
var ndsudoUnreaped = struct {
	mu   sync.Mutex
	runs map[string]int
}{
	runs: make(map[string]int),
}

func runNDSudo(log *logger.Logger, timeout time.Duration, argv []string) ([]byte, string, error) {
	const label = "RunNDSudo"
	cmdStr := strings.Join(append([]string{defaultRunner.ndSudoPath}, argv...), " ")
	if log != nil {
		log.Debugf("executing: %s", cmdStr)
	}
	key := strings.Join(argv, "\x00")
	ndsudoUnreaped.mu.Lock()
	_, unreaped := ndsudoUnreaped.runs[key]
	ndsudoUnreaped.mu.Unlock()
	if unreaped {
		return nil, cmdStr, runError(label, cmdStr, ErrPreviousRunNotExited, nil)
	}

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	stdout, stderr, err := newCaptures()
	if err != nil {
		return nil, cmdStr, runError(label, cmdStr, err, nil)
	}
	// Canceling ctx at the timeout terminates the command where the caller may signal it.
	p, err := StartNDSudoProcess(ctx, ProcessOptions{
		Stdout: stdout.w,
		Stderr: stderr.w,
	}, argv[0], argv[1:]...)
	stdout.startCopy()
	stderr.startCopy()
	if err != nil {
		stdout.close()
		stderr.close()
		return nil, cmdStr, runError(label, cmdStr, err, nil)
	}
	var waitErr error
	exited := make(chan struct{})
	go func() {
		waitErr = p.Wait()
		close(exited)
	}()

	select {
	case <-exited:
		// A descendant can hold the output open after the command exits.
		if !endOutput(ndsudoExitWait, stdout, stderr) && waitErr == nil {
			waitErr = exec.ErrWaitDelay
		}
	case <-ctx.Done():
		// From the timeout until the command exits, runs of the same command line are refused.
		ndsudoUnreaped.mu.Lock()
		ndsudoUnreaped.runs[key]++
		ndsudoUnreaped.mu.Unlock()
		release := sync.OnceFunc(func() {
			ndsudoUnreaped.mu.Lock()
			if ndsudoUnreaped.runs[key]--; ndsudoUnreaped.runs[key] == 0 {
				delete(ndsudoUnreaped.runs, key)
			}
			ndsudoUnreaped.mu.Unlock()
		})
		go func() {
			<-exited
			release()
		}()
		// A writer that cannot be signaled dies of SIGPIPE at its next write, unless it handles SIGPIPE or EPIPE
		// itself. The output is cut here, so the run reports the timeout even if the command exits during the wait.
		stdout.close()
		stderr.close()
		select {
		case <-exited:
			release()
		case <-time.After(ndsudoExitWait):
		}
		return stdout.buf.Bytes(), cmdStr, runError(label, cmdStr, contextError(ctx), stderr.buf.Bytes())
	}
	if waitErr != nil {
		if ctx.Err() != nil {
			waitErr = contextError(ctx)
		}
		return stdout.buf.Bytes(), cmdStr, runError(label, cmdStr, waitErr, stderr.buf.Bytes())
	}
	return stdout.buf.Bytes(), cmdStr, nil
}

// capture collects what a command writes to one pipe.
type capture struct {
	r, w *os.File
	buf  bytes.Buffer
	done chan struct{}
	once sync.Once
}

func newCaptures() (*capture, *capture, error) {
	stdout, err := newCapture()
	if err != nil {
		return nil, nil, err
	}
	stderr, err := newCapture()
	if err != nil {
		_ = stdout.r.Close()
		_ = stdout.w.Close()
		return nil, nil, err
	}
	return stdout, stderr, nil
}

func newCapture() (*capture, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, err
	}
	return &capture{
		r:    r,
		w:    w,
		done: make(chan struct{}),
	}, nil
}

// startCopy closes the caller's copy of the write end, which the started command now holds, and collects the output.
func (c *capture) startCopy() {
	_ = c.w.Close()
	go func() {
		defer close(c.done)
		_, _ = io.Copy(&c.buf, c.r)
	}()
}

// close closes the read end, ending the copy, and waits for the copy. It is idempotent.
func (c *capture) close() {
	c.once.Do(func() { _ = c.r.Close() })
	<-c.done
}

// endOutput waits up to wait for the copies to reach the end of the output, then closes them. It reports whether the
// output ended by itself.
func endOutput(wait time.Duration, captures ...*capture) bool {
	timer := time.NewTimer(wait)
	defer timer.Stop()
	ended := true
	for _, c := range captures {
		select {
		case <-c.done:
		case <-timer.C:
			ended = false
		}
		if !ended {
			break
		}
	}
	for _, c := range captures {
		c.close()
	}
	return ended
}
