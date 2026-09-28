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

// v1 bounds one complete response, including whitespace. Cancel a producer that
// exceeds it so accidental endless output cannot consume unbounded memory.
const maxResponseBytes = 1 << 20

var errResponseTooLarge = errors.New("collect response exceeds 1 MiB")

func runCommand(ctx context.Context, timeout time.Duration, argv []string, config []byte) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	output := responseBuffer{
		cancel: cancel,
	}
	args := append(append([]string(nil), argv[1:]...), "collect")
	cmd := ndexec.UnprivilegedCommandContext(ctx, argv[0], args...)
	cmd.Stdout, cmd.Stderr = &output, io.Discard
	// Schema-free packages retain EOF; configured packages receive exactly one envelope.
	if len(config) > 0 {
		cmd.Stdin = bytes.NewReader(config)
	}
	err := cmd.Run()
	if output.exceeded {
		return nil, errResponseTooLarge
	}
	if ctx.Err() != nil {
		return nil, fmt.Errorf("collect command: %w", ctx.Err())
	}
	if err != nil {
		return nil, fmt.Errorf("collect command: %w", err)
	}
	return output.buffer.Bytes(), nil
}

type responseBuffer struct {
	buffer   bytes.Buffer
	cancel   context.CancelFunc
	exceeded bool
}

func (b *responseBuffer) Write(data []byte) (int, error) {
	if len(data) > maxResponseBytes-b.buffer.Len() {
		b.exceeded = true
		b.cancel()
		return 0, errResponseTooLarge
	}
	return b.buffer.Write(data)
}
