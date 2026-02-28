// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestKeyScheduler_CompleteSkipsInvalidQueuedEntries(t *testing.T) {
	tests := map[string]struct {
		setup func(*keyScheduler)
		check func(*testing.T, *keyScheduler)
	}{
		"promotes next valid request after invalid queue heads": {
			setup: func(s *keyScheduler) {
				s.pending = 3
				s.lanes["k"] = &scheduleLane{
					ownerUID: "owner",
					queue: []*invocationRequest{
						nil,
						{},
						{
							fn:          &Function{UID: "next"},
							scheduleKey: "k",
						},
					},
				}
			},
			check: func(t *testing.T, s *keyScheduler) {
				t.Helper()
				lane, ok := s.lanes["k"]
				require.True(t, ok)
				require.NotNil(t, lane)
				assert.Equal(t, "next", lane.ownerUID)
				assert.Empty(t, lane.queue)
				require.Len(t, s.ready, 1)
				assert.Equal(t, "next", s.ready[0].fn.UID)
				assert.Equal(t, 1, s.pending)

				req, ok := s.next()
				require.True(t, ok)
				require.NotNil(t, req)
				assert.Equal(t, "next", req.fn.UID)
				assert.Equal(t, 0, s.pending)
			},
		},
		"drops invalid-only queued entries without pending leak": {
			setup: func(s *keyScheduler) {
				s.pending = 2
				s.lanes["k"] = &scheduleLane{
					ownerUID: "owner",
					queue: []*invocationRequest{
						nil,
						{},
					},
				}
			},
			check: func(t *testing.T, s *keyScheduler) {
				t.Helper()
				_, ok := s.lanes["k"]
				assert.False(t, ok)
				assert.Empty(t, s.ready)
				assert.Equal(t, 0, s.pending)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := newKeyScheduler(10)
			tc.setup(s)
			s.complete("k", "owner")
			tc.check(t, s)
		})
	}
}

func TestKeyScheduler_EnqueueValidation(t *testing.T) {
	tests := map[string]struct {
		req    *invocationRequest
		adjust func(*keyScheduler)
		want   error
	}{
		"nil request returns invalid error": {
			req:  nil,
			want: errSchedulerInvalid,
		},
		"nil function returns invalid error": {
			req: &invocationRequest{
				scheduleKey: "k",
			},
			want: errSchedulerInvalid,
		},
		"empty uid returns invalid error": {
			req: &invocationRequest{
				fn:          &Function{UID: ""},
				scheduleKey: "k",
			},
			want: errSchedulerInvalid,
		},
		"empty schedule key returns invalid error": {
			req: &invocationRequest{
				fn:          &Function{UID: "tx1"},
				scheduleKey: "",
			},
			want: errSchedulerInvalid,
		},
		"stopping scheduler returns stopping error": {
			req: &invocationRequest{
				fn:          &Function{UID: "tx1"},
				scheduleKey: "k",
			},
			adjust: func(s *keyScheduler) { s.stopAccepting() },
			want:   errSchedulerStopping,
		},
		"full scheduler returns queue full error": {
			req: &invocationRequest{
				fn:          &Function{UID: "tx1"},
				scheduleKey: "k",
			},
			adjust: func(s *keyScheduler) {
				s.pending = 1
			},
			want: errSchedulerQueueFull,
		},
		"valid request is admitted": {
			req: &invocationRequest{
				fn:          &Function{UID: "tx1"},
				scheduleKey: "k",
			},
			want: nil,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			s := newKeyScheduler(1)
			if tc.adjust != nil {
				tc.adjust(s)
			}

			err := s.enqueue(tc.req)
			if tc.want == nil {
				require.NoError(t, err)
				return
			}
			require.ErrorIs(t, err, tc.want)
		})
	}
}

func TestKeyScheduler_StopPaths(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"stopAccepting on drained scheduler makes next return false": {
			run: func(t *testing.T) {
				s := newKeyScheduler(1)
				s.stopAccepting()

				req, ok := s.next()
				assert.False(t, ok)
				assert.Nil(t, req)
			},
		},
		"stop wakes blocked next waiter": {
			run: func(t *testing.T) {
				s := newKeyScheduler(1)
				done := make(chan struct{})

				go func() {
					defer close(done)
					req, ok := s.next()
					assert.False(t, ok)
					assert.Nil(t, req)
				}()

				time.Sleep(20 * time.Millisecond)
				s.stop()

				select {
				case <-done:
				case <-time.After(time.Second):
					t.Fatal("timed out waiting for next() waiter to exit after stop()")
				}
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}

func TestKeyScheduler_QueueFullRecovery(t *testing.T) {
	tests := map[string]struct {
		run func(t *testing.T)
	}{
		"full queue recovers after dequeue and completion": {
			run: func(t *testing.T) {
				s := newKeyScheduler(1)
				req1 := &invocationRequest{
					fn:          &Function{UID: "tx1"},
					scheduleKey: "k",
				}
				req2 := &invocationRequest{
					fn:          &Function{UID: "tx2"},
					scheduleKey: "k",
				}

				require.NoError(t, s.enqueue(req1))
				require.ErrorIs(t, s.enqueue(req2), errSchedulerQueueFull)

				got, ok := s.next()
				require.True(t, ok)
				require.NotNil(t, got)
				require.Equal(t, "tx1", got.fn.UID)

				s.complete("k", "tx1")
				require.NoError(t, s.enqueue(req2))
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, tc.run)
	}
}
