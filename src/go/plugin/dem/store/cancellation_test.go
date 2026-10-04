// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCancelledAdmissionDoesNotAppend(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.acquire(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	attempted, err := s.AppendRumEvent(ctx, RumEventRecord{
		Type:      "activity",
		Site:      "shop",
		SessionID: "s",
	})
	assert.False(t, attempted)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = s.QueryRumSessions(ctx, "", 0, time.Now().Unix(), 0)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = s.QueryRumSessionEvents(ctx, "", "s")
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	_, err = s.QueryRumErrors(ctx, "", "", 0, time.Now().Unix())
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorIs(t, s.Sync(ctx), context.DeadlineExceeded)
	assert.ErrorIs(t, s.EnforceHistoryRetention(ctx, 1, 1), context.DeadlineExceeded)
	s.release()
	sessions, err := s.QueryRumSessions(context.Background(), "", 0, time.Now().Unix()+1, 0)
	require.NoError(t, err)
	assert.Empty(t, sessions)
	appendEvent(t, s, RumEventRecord{
		Type:      "activity",
		Site:      "shop",
		SessionID: "s",
	})
}

func TestScanCancellationAndSnapshotBoundaries(t *testing.T) {
	s := newTestStore(t)
	for range 4 {
		appendEvent(t, s, RumEventRecord{
			Type:      "pageview",
			Site:      "shop",
			SessionID: "s",
		})
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	count := 0
	err := s.scan(ctx, "shop", 0, time.Now().Unix()+1, func(RumEventRecord) { count++; cancel() })
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, count)
	reader, release, err := s.openReader(context.Background())
	require.NoError(t, err)
	defer release()
	appendEvent(t, s, RumEventRecord{
		Type:      "activity",
		Site:      "shop",
		SessionID: "s",
	})
	count = 0
	for {
		more, err := reader.Step()
		require.NoError(t, err)
		if !more {
			break
		}
		count++
	}
	assert.Equal(t, 4, count, "snapshot excludes later appends")
}

func TestCloseWaitsForOwnedReaderAndRejectsNewWork(t *testing.T) {
	s := newTestStore(t)
	_, release, err := s.openReader(context.Background())
	require.NoError(t, err)
	done := make(chan error, 1)
	go func() { done <- s.Close() }()
	select {
	case err := <-done:
		t.Fatalf("Close returned with an owned reader: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	release()
	require.NoError(t, <-done)
	attempted, err := s.AppendRumEvent(context.Background(), RumEventRecord{})
	assert.False(t, attempted)
	assert.ErrorIs(t, err, os.ErrClosed)
	_, err = s.QueryRumSessionEvents(context.Background(), "", "")
	assert.ErrorIs(t, err, os.ErrClosed)
	require.NoError(t, s.Close())
}
