// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

// Bound encoded messages, including envelopes and whitespace, generously enough
// for complete snapshots, tables and base64 Function payloads. This is a runaway
// output cutoff, not a per-job memory budget; buffers still grow on demand.
const maxMessageBytes = 64 << 20

var errResponseTooLarge = errors.New("script response exceeds 64 MiB")

func runCommand(ctx context.Context, timeout time.Duration, argv []string, config []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	return runOwnedCommand(ctx, cancel, argv, "collect", config)
}

func runOperation(ctx context.Context, argv []string, operation string, input []byte) ([]byte, error) {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	return runOwnedCommand(ctx, cancel, argv, operation, input)
}

func runOwnedCommand(
	ctx context.Context,
	cancel context.CancelFunc,
	argv []string,
	operation string,
	input []byte,
) ([]byte, error) {
	output := responseBuffer{
		cancel: cancel,
	}
	args := append(append([]string(nil), argv[1:]...), operation)
	cmd := ndexec.UnprivilegedCommandContext(ctx, argv[0], args...)
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

type responseBuffer struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	exceeded bool
}

func (b *responseBuffer) Write(data []byte) (int, error) {
	if len(data) > maxMessageBytes-b.buffer.Len() {
		b.exceeded = true
		b.cancel()
		return 0, errResponseTooLarge
	}
	return b.buffer.Write(data)
}
