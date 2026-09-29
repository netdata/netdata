// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// Metadata has its own startup-only cutoff for a broken producer, independent
// of operational messages. Leave generous room for embedded templates and forms
// instead of sizing this budget from existing packages. See NATIVE.md.
const maxDescriptionBytes = 64 << 20
const describeTimeout = 5 * time.Second

func describePackage(ctx context.Context, command []string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, describeTimeout)
	defer cancel()
	stdout, childStdout, err := os.Pipe()
	if err != nil {
		return nil, fmt.Errorf("create description pipe: %w", err)
	}
	defer stdout.Close()
	args := append(append([]string(nil), command[1:]...), "describe")
	process, err := ndexec.StartUnprivilegedProcess(
		ctx,
		ndexec.ProcessOptions{
			Stdout: childStdout,
		},
		command[0],
		args...)
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
		return nil, fmt.Errorf("package description exceeds 64 MiB")
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	if readErr != nil {
		return nil, fmt.Errorf("read package description failed")
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
