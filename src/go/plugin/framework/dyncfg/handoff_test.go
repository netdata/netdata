// SPDX-License-Identifier: GPL-3.0-or-later

package dyncfg

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestBoundedSend(t *testing.T) {
	tests := map[string]struct {
		run  func(t *testing.T) BoundedSendResult
		want BoundedSendResult
	}{
		"buffered channel send succeeds": {
			run: func(t *testing.T) BoundedSendResult {
				t.Helper()
				ch := make(chan int, 1)
				got := BoundedSend(context.Background(), ch, 42, 50*time.Millisecond)
				assert.Equal(t, 42, <-ch)
				return got
			},
			want: BoundedSendOK,
		},
		"unbuffered channel send succeeds with receiver": {
			run: func(t *testing.T) BoundedSendResult {
				t.Helper()
				ch := make(chan int)
				received := make(chan int, 1)
				go func() { received <- <-ch }()
				got := BoundedSend(context.Background(), ch, 77, 50*time.Millisecond)
				assert.Equal(t, 77, <-received)
				return got
			},
			want: BoundedSendOK,
		},
		"context canceled before send returns context-done": {
			run: func(t *testing.T) BoundedSendResult {
				t.Helper()
				ctx, cancel := context.WithCancel(context.Background())
				cancel()
				ch := make(chan int)
				return BoundedSend(ctx, ch, 1, 50*time.Millisecond)
			},
			want: BoundedSendContextDone,
		},
		"nil context uses background and times out": {
			run: func(t *testing.T) BoundedSendResult {
				t.Helper()
				ch := make(chan int)
				return BoundedSend[int](nil, ch, 1, 20*time.Millisecond)
			},
			want: BoundedSendTimeout,
		},
		"unbuffered channel without receiver times out": {
			run: func(t *testing.T) BoundedSendResult {
				t.Helper()
				ch := make(chan int)
				return BoundedSend(context.Background(), ch, 1, 20*time.Millisecond)
			},
			want: BoundedSendTimeout,
		},
		"expired context deadline returns context-done": {
			run: func(t *testing.T) BoundedSendResult {
				t.Helper()
				ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(-time.Second))
				defer cancel()
				ch := make(chan int)
				return BoundedSend(ctx, ch, 1, 50*time.Millisecond)
			},
			want: BoundedSendContextDone,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, tc.run(t))
		})
	}
}
