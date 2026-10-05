// SPDX-License-Identifier: GPL-3.0-or-later

package ndexec

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
)

var (
	ErrTreeSupervisionUnsupported = errors.New("process tree supervision is unsupported on this platform")
	ErrTreeSupervisionUnavailable = errors.New("process tree supervision setup is unavailable")
	ErrTreeNotDrained             = errors.New("process tree cleanup was not verified")
)

// TreeResult records command, cancellation, setup and supervisor/protocol errors
// independently of verified descendant cleanup. A failed or canceled command can
// be Drained. A false Drained requires the caller to retain
// resource ownership and stop further admission; it is not an ordinary job error.
type TreeResult struct {
	Err     error
	Drained bool
}

// TreeProcess owns a per-run supervisor, its cancellation channel and completion
// evidence. It must not be copied. The target cannot inherit these channels.
type TreeProcess struct {
	control     *os.File
	mu          sync.Mutex
	finished    bool
	cancelCause error
	controlErr  error
	result      TreeResult
	done        chan struct{}
	watcherDone chan struct{}
}

func ownTreeProcess(ctx context.Context, cmd *exec.Cmd, control, status *os.File) *TreeProcess {
	p := &TreeProcess{
		control:     control,
		done:        make(chan struct{}),
		watcherDone: make(chan struct{}),
	}
	go func() {
		defer close(p.watcherDone)
		select {
		case <-ctx.Done():
			p.requestStop(ctx.Err())
		case <-p.done:
		}
	}()
	go func() {
		waitErr := cmd.Wait()
		// The real helper closes the private status descriptor in every payload
		// before exec. After helper exit there are no inherited pipe writers.
		data, readErr := io.ReadAll(io.LimitReader(status, 64))
		closeErr := status.Close()
		result := treeCompletion(data, waitErr)
		if readErr != nil || closeErr != nil {
			result.Drained = false
			result.Err = errors.Join(result.Err, ErrTreeNotDrained, readErr, closeErr)
		}
		p.mu.Lock()
		p.finished = true
		if p.control != nil {
			p.controlErr = errors.Join(p.controlErr, p.control.Close())
			p.control = nil
		}
		result.Err = errors.Join(result.Err, p.cancelCause, p.controlErr)
		p.result = result
		p.mu.Unlock()
		close(p.done)
	}()
	return p
}

func (p *TreeProcess) requestStop(cause error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	if p.finished || p.control == nil {
		return
	}
	p.cancelCause = cause
	// EOF requests drain. Killing the supervisor would destroy its ability to
	// adopt/reap detached descendants and prove completion.
	p.controlErr = p.control.Close()
	p.control = nil
}

// Wait joins the supervisor and its cancellation watcher. It has no cleanup
// timeout: a live uninterruptible descendant cannot truthfully be called drained.
// Repeated and concurrent calls return the same immutable result.
func (p *TreeProcess) Wait() TreeResult {
	<-p.done
	<-p.watcherDone
	return p.result
}

// Close requests cancellation and then waits for verified cleanup. It never
// SIGKILLs the supervisor. A host deadline may require fail-stop while Wait is
// unfinished; the caller must not release resources as if cleanup succeeded.
func (p *TreeProcess) Close() TreeResult {
	p.requestStop(context.Canceled)
	return p.Wait()
}

func treeCompletion(data []byte, waitErr error) TreeResult {
	invalid := func() TreeResult {
		return TreeResult{
			Err: errors.Join(waitErr, ErrTreeNotDrained),
		}
	}
	const prefix = "NDTREE1 "
	frame := string(data)
	if !strings.HasPrefix(frame, prefix) || !strings.HasSuffix(frame, "\n") {
		return invalid()
	}
	actualExit := 0
	if waitErr != nil {
		var exitErr *exec.ExitError
		if !errors.As(waitErr, &exitErr) {
			return invalid()
		}
		actualExit = exitErr.ExitCode()
	}
	if frame == "NDTREE1 unavailable\n" {
		if actualExit != 126 {
			return invalid()
		}
		return TreeResult{
			Err:     errors.Join(waitErr, ErrTreeSupervisionUnavailable),
			Drained: true,
		}
	}
	rawText := strings.TrimSuffix(strings.TrimPrefix(frame, prefix), "\n")
	raw, err := strconv.ParseUint(rawText, 10, 16)
	if err != nil || strconv.FormatUint(raw, 10) != rawText {
		return invalid()
	}
	exitCode := 0
	switch {
	case raw&0xff == 0:
		exitCode = int(raw >> 8)
	case raw & ^uint64(0xff) == 0 && raw&0x7f > 0 && raw&0x7f < 0x7f:
		exitCode = 128 + int(raw&0x7f)
	default:
		return invalid()
	}
	if actualExit != exitCode {
		return invalid()
	}
	return TreeResult{
		Err:     waitErr,
		Drained: true,
	}
}

// The helper's location is inherited from normal ndexec discovery. Errors never
// include command arguments or captured output.
func treeStartError(err error) error { return fmt.Errorf("start supervised command: %w", err) }
