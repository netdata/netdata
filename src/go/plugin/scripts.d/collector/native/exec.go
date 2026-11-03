// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"slices"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// Metadata has its own startup-only cutoff for a broken producer, independent
// of operational messages. Leave generous room for embedded templates and forms
// instead of sizing this budget from existing packages. See NATIVE.md.
const (
	maxDescriptionBytes = 64 << 20
	describeTimeout     = 5 * time.Second
)

// operationArgs appends the protocol operation to the configured arguments.
func operationArgs(command []string, operation string) []string {
	return append(slices.Clone(command[1:]), operation)
}

// runOneshot runs one collect or function invocation. Input is configuration, a
// Function request, both, or EOF for schema-free collection.
func runOneshot(ctx context.Context, command []string, operation string, input []byte) ([]byte, error) {
	data, err := runOwned(ctx, command, operation, input, maxMessageBytes)
	if err != nil && !errors.Is(err, errResponseTooLarge) {
		return nil, fmt.Errorf("script command: %w", err)
	}
	return data, err
}

// runDescribe runs a self-contained package's describe operation within its own
// startup budget. Errors carry fixed categories only.
func runDescribe(ctx context.Context, command []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()
	data, err := runOwned(ctx, command, opDescribe, nil, maxDescriptionBytes)
	switch {
	case err == nil:
		return data, nil
	case errors.Is(err, errResponseTooLarge):
		return nil, errors.New("package description exceeds 64 MiB")
	case ctx.Err() != nil:
		return nil, ctx.Err()
	default:
		return nil, fmt.Errorf("describe command: %s", commandFailureReason(err))
	}
}

// runOwned runs one operation as an owned process: completion, cancellation or
// leader exit terminates its contained descendants, and all stdio uses files, so
// no host goroutine depends on a descendant closing an inherited stream. It
// returns at most limit bytes of stdout, or errResponseTooLarge. The exit status
// and output decide the outcome; a script need not read all of its input.
func runOwned(ctx context.Context, command []string, operation string, input []byte, limit int) ([]byte, error) {
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create stdout pipe: %w", err)
	}
	defer stdout.Close()
	opts := ndexec.ProcessOptions{
		Stdout: childStdout,
	}
	var stdin *os.File
	if len(input) > 0 {
		childStdin, w, err := os.Pipe()
		if err != nil {
			_ = childStdout.Close()
			return nil, fmt.Errorf("create stdin pipe: %w", err)
		}
		opts.Stdin, stdin = childStdin, w
	}
	process, err := ndexec.StartUnprivilegedProcess(ctx, opts, command[0], operationArgs(command, operation)...)
	_ = childStdout.Close()
	if opts.Stdin != nil {
		_ = opts.Stdin.Close()
	}
	if err != nil {
		if stdin != nil {
			_ = stdin.Close()
		}
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	defer process.Close()
	if stdin != nil {
		// Write concurrently with reading stdout, so neither pipe can fill and block
		// the other. On early return, terminate the process tree before joining the
		// writer, then close the write end in case an escaped holder keeps stdin open.
		written := make(chan struct{})
		go func() {
			defer close(written)
			_, _ = stdin.Write(input)
			_ = stdin.Close()
		}()
		defer func() {
			_ = process.Close()
			_ = stdin.Close()
			<-written
		}()
	}
	// Closing our read end also bounds escaped descendants retaining stdout.
	stop := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer stop()
	data, readErr := io.ReadAll(io.LimitReader(stdout, int64(limit)+1))
	if len(data) > limit {
		return nil, errResponseTooLarge
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if readErr != nil {
		return nil, fmt.Errorf("read output: %w", readErr)
	}
	if err := process.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, err
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return data, nil
}

// commandFailureReason classifies a command error for logs. Never log the
// original error: command errors can include paths, and output belongs to the
// script. Numeric exit status and fixed categories are safe.
func commandFailureReason(err error) string {
	var exitErr *exec.ExitError
	switch {
	case errors.Is(err, context.Canceled):
		return "caller canceled"
	case errors.Is(err, context.DeadlineExceeded):
		return "caller deadline exceeded"
	case errors.Is(err, errResponseTooLarge):
		return "response exceeds 64 MiB"
	case errors.As(err, &exitErr):
		if exitErr.ExitCode() < 0 {
			return "command terminated by signal"
		}
		return fmt.Sprintf("command exited with status %d", exitErr.ExitCode())
	default:
		return "command execution failed"
	}
}
