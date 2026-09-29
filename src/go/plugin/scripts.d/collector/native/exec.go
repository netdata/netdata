// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
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

// runOneshot runs one collect or function invocation. Cancellation terminates
// the command's process group; the one-shot contract requires scripts to finish
// their own children.
func runOneshot(ctx context.Context, command []string, operation string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	output := limitedBuffer{
		cancel: cancel,
	}
	cmd := ndexec.UnprivilegedCommandContext(ctx, command[0], operationArgs(command, operation)...)
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	// Input is configuration, a Function request, both, or EOF for schema-free collection.
	if len(input) > 0 {
		cmd.Stdin = bytes.NewReader(input)
	}
	err := cmd.Run()
	if output.exceeded {
		return nil, errResponseTooLarge
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("script command: %w", ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("script command: %w", err)
	}
	return output.buffer.Bytes(), nil
}

// limitedBuffer collects stdout up to maxMessageBytes and cancels the command
// as soon as the output exceeds it.
type limitedBuffer struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	exceeded bool
}

func (b *limitedBuffer) Write(data []byte) (int, error) {
	if len(data) > maxMessageBytes-b.buffer.Len() {
		b.exceeded = true
		b.cancel()
		return 0, errResponseTooLarge
	}
	return b.buffer.Write(data)
}

// runDescribe runs a self-contained package's describe operation as an owned
// process: completion also terminates contained descendants.
func runDescribe(ctx context.Context, command []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create description pipe: %w", err)
	}
	defer stdout.Close()
	process, err := ndexec.StartUnprivilegedProcess(
		ctx,
		ndexec.ProcessOptions{
			Stdout: childStdout,
		},
		command[0],
		operationArgs(command, opDescribe)...)
	_ = childStdout.Close()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("describe command: %s", commandFailureReason(err))
	}
	defer process.Close()
	// Closing our read end also bounds escaped descendants retaining stdout.
	stop := context.AfterFunc(ctx, func() { _ = stdout.Close() })
	defer stop()
	data, readErr := io.ReadAll(io.LimitReader(stdout, maxDescriptionBytes+1))
	if len(data) > maxDescriptionBytes {
		return nil, errors.New("package description exceeds 64 MiB")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if readErr != nil {
		return nil, errors.New("read package description failed")
	}
	if err := process.Wait(); err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, fmt.Errorf("describe command: %s", commandFailureReason(err))
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
