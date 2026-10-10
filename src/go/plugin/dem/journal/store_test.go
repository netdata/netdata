// SPDX-License-Identifier: GPL-3.0-or-later

package journal

import (
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
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
	attempted, err := s.Append(context.Background(), testFields())
	require.True(t, attempted)
	require.NoError(t, err)
}
func testFields() []journal.Field {
	return []journal.Field{journal.StringField("DEM_KIND", "rum"), journal.StringField("DEM_OBSERVED_US", "123456789"), journal.StringField("MESSAGE", "entry")}
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
	require.NoError(t, reader.VisitEntries(func(*journal.SnapshotEntry) error { n++; return nil }))
	return n
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
	count := 0
	err = reader.VisitEntries(func(*journal.SnapshotEntry) error { count++; cancel(); return nil })
	assert.ErrorIs(t, err, context.Canceled)
	assert.Equal(t, 1, count)
	release()
	reader, release, err = s.OpenReader(context.Background())
	require.NoError(t, err)
	defer release()
	appendEntry(t, s)
	count = 0
	require.NoError(t, reader.VisitEntries(func(*journal.SnapshotEntry) error { count++; return nil }))
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

func TestPrivateCleanupPreservesUnsafeFiles(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close(); require.NoError(t, os.RemoveAll(s.root)) })
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	appendEntry(t, s)
	damaged := filepath.Join(filepath.Dir(s.log.ActivePath()), "dem@damaged.journal~")
	require.NoError(t, os.WriteFile(damaged, []byte("preserve evidence"), 0600))
	require.Error(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	require.Error(t, s.Close())
	data, err := os.ReadFile(damaged)
	require.NoError(t, err)
	assert.Equal(t, "preserve evidence", string(data))
}

func TestRelaxedRetentionDoesNotApplyPreviousAge(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newTestStore(t)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 1, 1<<30))
		appendEntry(t, s)
		time.Sleep(2 * 24 * time.Hour)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
		assert.Equal(t, 1, countEntries(t, s), "a relaxed policy must not enforce the retired shorter age")
	})
}

func TestOrdinaryRetentionPreservesActiveFile(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	appendEntry(t, s)
	active := s.log.ActivePath()
	require.NotEmpty(t, active)
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	assert.Equal(t, active, s.log.ActivePath())
	files, err := filepath.Glob(filepath.Join(s.root, "*", "*.journal"))
	require.NoError(t, err)
	assert.Equal(t, []string{active}, files)
}

func TestAppendEnvelopeRejectsIncompatibleFieldsBeforeAttempt(t *testing.T) {
	for name, fields := range map[string][]journal.Field{
		"missing kind":    {journal.StringField("MESSAGE", "unknown")},
		"missing clock":   {journal.StringField("DEM_KIND", "rum")},
		"duplicate clock": append(testFields(), journal.StringField("DEM_OBSERVED_US", "5")),
		"wrong clock":     append(testFields(), journal.StringField("DEM_STARTED_US", "5")),
		"reserved schema": append(testFields(), journal.StringField("DEM_SCHEMA", "1")),
		"reserved bucket": append(testFields(), journal.StringField("DEM_RUM_MINUTE", "2")),
		"noncanonical":    {journal.StringField("DEM_KIND", "rum"), journal.StringField("DEM_OBSERVED_US", "0123")},
	} {
		t.Run(name, func(t *testing.T) {
			s := newTestStore(t)
			attempted, err := s.Append(context.Background(), fields)
			assert.False(t, attempted)
			require.Error(t, err)
			assert.Zero(t, countEntries(t, s))
			appendEntry(t, s)
			assert.Equal(t, 1, countEntries(t, s), "validation failure leaves writer usable")
		})
	}
}

func TestEnvelopeChecksSchemaClockAndBucket(t *testing.T) {
	fields := []journal.Field{journal.StringField("DEM_SCHEMA", "1"), journal.StringField("DEM_KIND", "rum"), journal.StringField("DEM_OBSERVED_US", "60000001"), journal.StringField("DEM_RUM_MINUTE", "1")}
	var valid Envelope
	for _, field := range fields {
		require.NoError(t, valid.Observe([]byte(field.Name), field.Value))
	}
	kind, timestamp, err := valid.Validate()
	require.NoError(t, err)
	assert.Equal(t, "rum", kind)
	assert.Equal(t, int64(60000001), timestamp)
	t.Run("duplicate", func(t *testing.T) {
		candidate := valid
		assert.ErrorContains(t, candidate.Observe([]byte("DEM_RUM_MINUTE"), []byte("1")), "duplicate history field")
	})
	t.Run("unsupported schema", func(t *testing.T) {
		var candidate Envelope
		assert.ErrorContains(t, candidate.Observe([]byte("DEM_SCHEMA"), []byte("0")), "incompatible DEM history schema")
	})
	t.Run("noncanonical", func(t *testing.T) {
		var candidate Envelope
		for _, field := range fields[:3] {
			require.NoError(t, candidate.Observe([]byte(field.Name), field.Value))
		}
		assert.ErrorContains(t, candidate.Observe([]byte("DEM_RUM_MINUTE"), []byte("01")), "expected canonical nonnegative decimal")
	})
	fields[3] = journal.StringField("DEM_RUM_MINUTE", "2")
	var mismatch Envelope
	for _, field := range fields {
		require.NoError(t, mismatch.Observe([]byte(field.Name), field.Value))
	}
	_, _, err = mismatch.Validate()
	assert.Error(t, err)
}

func TestStartupRejectsInterruptedActiveWithoutChangingFiles(t *testing.T) {
	for _, foreign := range []bool{false, true} {
		t.Run(map[bool]string{false: "current machine", true: "retained machine"}[foreign], func(t *testing.T) {
			root := t.TempDir()
			host, err := journalhost.Load(journalhost.LoadOptions{StateDir: filepath.Join(root, "identity")})
			require.NoError(t, err)
			machine := host.MachineID()
			if foreign {
				machine, err = journal.ParseUUID("12345678901234567890123456789012")
				require.NoError(t, err)
			}
			dir := filepath.Join(root, machine.String())
			require.NoError(t, os.MkdirAll(dir, 0750))
			path := filepath.Join(dir, "dem.journal")
			writer, err := journal.Create(path, journal.Options{MachineID: machine, BootID: host.BootID(), Compact: true})
			require.NoError(t, err)
			fields, err := appendEnvelope(testFields())
			require.NoError(t, err)
			require.NoError(t, writer.Append(fields, host.EntryOptions()))
			require.NoError(t, writer.Append(fields, host.EntryOptions()))
			require.NoError(t, writer.Close())
			data, err := os.ReadFile(path)
			require.NoError(t, err)
			// n_entries is the systemd header's committed population at byte 152.
			// Leave both native postings present, modelling an interrupted publication.
			binary.LittleEndian.PutUint64(data[152:160], 1)
			require.NoError(t, os.WriteFile(path, data, 0600))
			_, err = Open(context.Background(), root)
			require.Error(t, err)
			assert.Contains(t, err.Error(), "verify active history")
			preserved, readErr := os.ReadFile(path)
			require.NoError(t, readErr)
			assert.Equal(t, data, preserved)
			files, err := os.ReadDir(dir)
			require.NoError(t, err)
			require.Len(t, files, 1)
			assert.Equal(t, "dem.journal", files[0].Name())
		})
	}
}

func TestStartupRejectsUnversionedArchiveWithoutRemoval(t *testing.T) {
	root := t.TempDir()
	host, err := journalhost.Load(journalhost.LoadOptions{StateDir: filepath.Join(root, "identity")})
	require.NoError(t, err)
	log, err := journal.NewLog(root, journal.LogConfig{Source: "dem", StrictSystemdNaming: true, Options: journal.Options{MachineID: host.MachineID(), BootID: host.BootID(), Compact: true}})
	require.NoError(t, err)
	require.NoError(t, log.Append(testFields(), host.EntryOptions()))
	require.NoError(t, log.Close())
	files, err := filepath.Glob(filepath.Join(root, "*", "dem@*.journal"))
	require.NoError(t, err)
	require.Len(t, files, 1)
	before, err := os.ReadFile(files[0])
	require.NoError(t, err)
	_, err = Open(context.Background(), root)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "incompatible DEM history schema")
	after, err := os.ReadFile(files[0])
	require.NoError(t, err)
	assert.Equal(t, before, after)
}

func TestFailedJournalRejectsQueriesAndRetention(t *testing.T) {
	ctx := context.Background()
	s, err := Open(ctx, "")
	require.NoError(t, err)
	t.Cleanup(func() { _ = s.Close(); require.NoError(t, os.RemoveAll(s.root)) })
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	appendEntry(t, s)
	active := s.log.ActivePath()
	dir := filepath.Dir(active)
	// Readable metadata lets preflight succeed, while a non-writable directory
	// makes the archive rename fail after its header has been finalized/synced.
	probe := filepath.Join(dir, "permission-probe")
	require.NoError(t, os.WriteFile(probe, nil, 0600))
	require.NoError(t, os.Chmod(dir, 0550))
	t.Cleanup(func() { require.NoError(t, os.Chmod(dir, 0750)) })
	if err := os.Rename(probe, probe+"-renamed"); err == nil {
		t.Skip("filesystem/user does not enforce directory write permissions")
	} else {
		require.ErrorIs(t, err, os.ErrPermission)
	}
	err = s.EnforceHistoryRetention(ctx, 30, MinHistoryBytes)
	require.ErrorIs(t, err, journal.ErrWriterFailed)
	before, err := os.ReadFile(active)
	require.NoError(t, err)
	_, _, err = s.OpenReader(ctx)
	assert.ErrorIs(t, err, journal.ErrWriterFailed)
	attempted, err := s.Append(ctx, testFields())
	assert.False(t, attempted)
	assert.ErrorIs(t, err, journal.ErrWriterFailed)
	assert.ErrorIs(t, s.Sync(ctx), journal.ErrWriterFailed)
	assert.ErrorIs(t, s.EnforceHistoryRetention(ctx, 30, MinHistoryBytes), journal.ErrWriterFailed)
	status, err := s.Status(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, status.WriterError)
	assert.NotEmpty(t, status.Cleanup.Error)
	// Closing a failed writer releases resources and preserves uncertain bytes.
	assert.ErrorIs(t, s.Close(), journal.ErrWriterFailed)
	after, err := os.ReadFile(active)
	require.NoError(t, err, "failed private history is preserved")
	assert.Equal(t, before, after, "close must not rewrite uncertain metadata")
}
