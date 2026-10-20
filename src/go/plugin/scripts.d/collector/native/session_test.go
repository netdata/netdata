// SPDX-License-Identifier: GPL-3.0-or-later

package native

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The frame limit includes the terminating LF.
func TestSplitFrame(t *testing.T) {
	for _, size := range []int{1, maxMessageBytes - 1, maxMessageBytes, maxMessageBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(strings.Repeat(" ", size-1) + "\n"))
			scanner.Buffer(make([]byte, 4096), maxMessageBytes+1)
			scanner.Split(splitFrame)
			if size > maxMessageBytes {
				assert.False(t, scanner.Scan())
				require.ErrorIs(t, scanner.Err(), errResponseTooLarge)
				return
			}
			require.True(t, scanner.Scan())
			assert.Len(t, scanner.Bytes(), size-1)
			assert.False(t, scanner.Scan())
			require.NoError(t, scanner.Err())
		})
	}
	t.Run("unterminated at EOF", func(t *testing.T) {
		_, _, err := splitFrame([]byte("{}"), true)
		require.ErrorIs(t, err, io.ErrUnexpectedEOF)
	})
}

// A real pipe with no consumer forces the same blocked Write path as a peer
// that stops reading stdin. Cancellation must join the writer.
func TestScriptSession_BlockedWriteCancellation(t *testing.T) {
	input, output, err := os.Pipe()
	require.NoError(t, err)
	defer input.Close()
	defer output.Close()
	s := &scriptSession{
		ctx:    context.Background(),
		stdin:  output,
		exited: make(chan struct{}),
		frames: make(chan scriptFrame),
	}
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	finished := make(chan error, 1)
	go func() { _, err := s.exchange(ctx, []byte(strings.Repeat("1", 1<<20))); finished <- err }()
	select {
	case err := <-finished:
		require.ErrorIs(t, err, context.DeadlineExceeded)
	case <-time.After(time.Second):
		t.Fatal("blocked writer was not joined")
	}
}
