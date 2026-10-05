// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	s, err := Open(context.Background(), "")
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	return s
}
func appendEntry(t *testing.T, s *Store) {
	t.Helper()
	attempted, err := s.Append(context.Background(), []journal.Field{journal.StringField("MESSAGE", "entry")})
	require.True(t, attempted)
	require.NoError(t, err)
}
func countEntries(t *testing.T, s *Store) int {
	t.Helper()
	reader, release, err := s.OpenReader(context.Background())
	require.NoError(t, err)
	defer release()
	if reader == nil {
		return 0
	}
	n := 0
	for {
		more, err := reader.Step()
		require.NoError(t, err)
		if !more {
			return n
		}
		n++
	}
}
func TestCancelledAdmissionDoesNotAppend(t *testing.T) {
	s := newTestStore(t)
	require.NoError(t, s.acquire(context.Background()))
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	attempted, err := s.Append(ctx, []journal.Field{journal.StringField("MESSAGE", "cancelled")})
	assert.False(t, attempted)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	_, _, err = s.OpenReader(ctx)
	assert.ErrorIs(t, err, context.DeadlineExceeded)
	assert.ErrorIs(t, s.Sync(ctx), context.DeadlineExceeded)
	assert.ErrorIs(t, s.EnforceHistoryRetention(ctx, 1, 1), context.DeadlineExceeded)
	s.release()
	assert.Zero(t, countEntries(t, s))
	appendEntry(t, s)
	assert.Equal(t, 1, countEntries(t, s))
}
func TestScanCancellationAndSnapshotBoundaries(t *testing.T) {
	s := newTestStore(t)
	for range 4 {
		appendEntry(t, s)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	reader, release, err := s.OpenReader(ctx)
	require.NoError(t, err)
	more, err := reader.Step()
	require.NoError(t, err)
	require.True(t, more)
	cancel()
	_, err = reader.Step()
	assert.ErrorIs(t, err, context.Canceled)
	release()
	reader, release, err = s.OpenReader(context.Background())
	require.NoError(t, err)
	defer release()
	appendEntry(t, s)
	count := 0
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
	_, release, err := s.OpenReader(context.Background())
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
	attempted, err := s.Append(context.Background(), []journal.Field{journal.StringField("MESSAGE", "closed")})
	assert.False(t, attempted)
	assert.ErrorIs(t, err, os.ErrClosed)
	_, _, err = s.OpenReader(context.Background())
	assert.ErrorIs(t, err, os.ErrClosed)
	require.NoError(t, s.Close())
}
func TestPrivateCleanup(t *testing.T) {
	s := newTestStore(t)
	root := s.root
	assert.NotEmpty(t, s.temporary)
	appendEntry(t, s)
	require.NoError(t, s.Close())
	_, err := os.Stat(root)
	assert.ErrorIs(t, err, os.ErrNotExist)
}

// A short SDK age models an archive crossing the old limit between sweeps.
// Relaxing retention must preserve it under the newly loaded policy.
func TestRelaxedRetentionDoesNotApplyPreviousAge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	saved := time.Now()
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(root, "identity"),
	})
	require.NoError(t, err)
	config := journal.LogConfig{
		Source: "dem",
		Options: journal.Options{
			MachineID: host.MachineID(),
			BootID:    host.BootID(),
			Compact:   true,
		},
	}
	log, err := journal.NewLog(root, config)
	require.NoError(t, err)
	opts := host.EntryOptions()
	opts.RealtimeUsec = uint64(saved.UnixMicro())
	require.NoError(t, log.Append([]journal.Field{journal.StringField("MESSAGE", "retained")}, opts))
	require.NoError(t, log.Close())
	config.OpenMode = journal.LogOpenEager
	config.RetentionPolicy = journal.RetentionPolicy{}.WithMaxAge(time.Second).WithMaxBytes(1 << 30)
	log, err = journal.NewLog(root, config)
	require.NoError(t, err)
	s := &Store{
		gate:   make(chan struct{}, 1),
		root:   root,
		host:   host,
		config: config,
		log:    log,
	}
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	require.Equal(t, 1, countEntries(t, s))
	appendEntry(t, s)
	time.Sleep(time.Until(saved.Add(time.Second + 50*time.Millisecond)))
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	assert.Equal(t, 2, countEntries(t, s), "an increased allowance must not enforce the retired shorter age")
}

func TestIdleRetentionLeavesWriterLazy(t *testing.T) {
	s := newTestStore(t)
	appendEntry(t, s)
	require.NoError(t, s.EnforceHistoryRetention(context.Background(), 30, 1<<30))
	assert.Empty(t, s.log.ActivePath(), "idle sweep must leave the new chain lazy")
}
