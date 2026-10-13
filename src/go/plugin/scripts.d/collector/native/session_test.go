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
	"testing/iotest"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// The frame limit includes the terminating LF.
func TestFrameSplitter_Limit(t *testing.T) {
	for _, size := range []int{1, maxMessageBytes - 1, maxMessageBytes, maxMessageBytes + 1} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			scanner := bufio.NewScanner(strings.NewReader(strings.Repeat(" ", size-1) + "\n"))
			scanner.Buffer(make([]byte, 4096), maxMessageBytes+1)
			scanner.Split(newFrameSplitter())
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
}

// Frames do not depend on how reads divide the stream, including a frame that
// follows a longer one.
func TestFrameSplitter_ReadBoundaries(t *testing.T) {
	oneRead := func(s string) io.Reader { return strings.NewReader(s) }
	byteByByte := func(s string) io.Reader { return iotest.OneByteReader(strings.NewReader(s)) }
	tests := map[string]struct {
		reader     func(string) io.Reader
		stream     string
		wantFrames []string
		wantErr    error
	}{
		"one read": {
			reader:     oneRead,
			stream:     "abc\r\n\nd\n",
			wantFrames: []string{"abc\r", "", "d"},
		},
		"byte by byte": {
			reader:     byteByByte,
			stream:     "abc\r\n\nd\n",
			wantFrames: []string{"abc\r", "", "d"},
		},
		"unterminated, one read": {
			reader:     oneRead,
			stream:     "abc\nd",
			wantFrames: []string{"abc"},
			wantErr:    io.ErrUnexpectedEOF,
		},
		"unterminated, byte by byte": {
			reader:     byteByByte,
			stream:     "abc\nd",
			wantFrames: []string{"abc"},
			wantErr:    io.ErrUnexpectedEOF,
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			scanner := bufio.NewScanner(tc.reader(tc.stream))
			scanner.Split(newFrameSplitter())
			var frames []string
			for scanner.Scan() {
				frames = append(frames, scanner.Text())
			}
			assert.Equal(t, tc.wantFrames, frames)
			if tc.wantErr != nil {
				require.ErrorIs(t, scanner.Err(), tc.wantErr)
			} else {
				require.NoError(t, scanner.Err())
			}
		})
	}
}

// Reads one frame delivered in pipe-sized chunks. Cost is O(frame bytes): the LF
// search does not rescan the pending frame after each read.
func BenchmarkFrameSplitter(b *testing.B) {
	for _, bc := range []struct {
		name string
		size int
	}{
		{name: "64KiB", size: 64 << 10},
		{name: "1MiB", size: 1 << 20},
		{name: "16MiB", size: 16 << 20},
	} {
		frame := []byte(strings.Repeat(" ", bc.size-1) + "\n")
		b.Run(bc.name, func(b *testing.B) {
			b.SetBytes(int64(bc.size))
			b.ReportAllocs()
			for b.Loop() {
				scanner := bufio.NewScanner(&chunkReader{data: frame, chunk: 64 << 10})
				scanner.Buffer(make([]byte, 4096), maxMessageBytes+1)
				scanner.Split(newFrameSplitter())
				if !scanner.Scan() {
					b.Fatal(scanner.Err())
				}
			}
		})
	}
}

// chunkReader returns at most chunk bytes per Read, like a pipe.
type chunkReader struct {
	data  []byte
	chunk int
}

func (r *chunkReader) Read(p []byte) (int, error) {
	if len(r.data) == 0 {
		return 0, io.EOF
	}
	n := copy(p[:min(len(p), r.chunk)], r.data)
	r.data = r.data[n:]
	return n, nil
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
