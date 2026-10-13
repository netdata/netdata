// SPDX-License-Identifier: GPL-3.0-or-later
package journal

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"
	"testing/synctest"
	"time"

	sdk "github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestSparseHourlyRetentionUsesDailyFiles(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newTestStore(t)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
		for range 48 {
			appendEntry(t, s)
			require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
			time.Sleep(time.Hour)
		}
		files, err := filepath.Glob(filepath.Join(s.root, "*", "*.journal"))
		require.NoError(t, err)
		assert.Equal(t, 2, len(files), "24-hour spans should replace hourly fragmentation")
		assert.Equal(t, 48, countEntries(t, s))
	})
}

func TestRetentionCoversRetiredActiveAndArchivedMachines(t *testing.T) {
	for _, active := range []bool{false, true} {
		t.Run(map[bool]string{false: "archive", true: "active"}[active], func(t *testing.T) {
			root := t.TempDir()
			host, err := journalhost.Load(journalhost.LoadOptions{StateDir: filepath.Join(root, "identity")})
			require.NoError(t, err)
			machine, err := sdk.ParseUUID("11111111111111111111111111111111")
			require.NoError(t, err)
			require.NotEqual(t, host.MachineID(), machine)
			dir := filepath.Join(root, machine.String())
			require.NoError(t, os.MkdirAll(dir, 0750))
			path := filepath.Join(dir, "dem.journal")
			writer, err := sdk.Create(path, sdk.Options{MachineID: machine, BootID: host.BootID(), Compact: true})
			require.NoError(t, err)
			fields, err := appendEnvelope(testFields())
			require.NoError(t, err)
			opts := host.EntryOptions()
			opts.RealtimeUsec = uint64(time.Now().Add(-40 * 24 * time.Hour).UnixMicro())
			require.NoError(t, writer.Append(fields, opts))
			require.NoError(t, writer.Close())
			if !active {
				// Finalize through the public Log lifecycle; caller owns the file.
				log, err := sdk.NewLog(root, sdk.LogConfig{Source: "dem", StrictSystemdNaming: true, Options: sdk.Options{MachineID: machine, BootID: host.BootID(), Compact: true}})
				require.NoError(t, err)
				require.NoError(t, log.CloseWithoutRetention())
			}
			s, err := Open(context.Background(), root)
			require.NoError(t, err)
			t.Cleanup(func() { require.NoError(t, s.Close()) })
			require.Equal(t, 1, countEntries(t, s))
			require.NoError(t, s.EnforceHistoryRetention(context.Background(), 30, 1<<30))
			assert.Zero(t, countEntries(t, s))
		})
	}
}

func TestTailExpiryKeepsMixedHistoryAndPinnedSnapshot(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		s := newTestStore(t)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 1, 1<<30))
		appendNamed(t, s, "rum", "first")
		time.Sleep(23 * time.Hour)
		appendNamed(t, s, "synthetic", "last")
		time.Sleep(2 * time.Hour)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 1, 1<<30))
		assert.Equal(t, []string{"first", "last"}, retainedNames(t, s), "age uses the newest saved record, not the file head or observation time")
		reader, release, err := s.OpenReader(ctx)
		require.NoError(t, err)
		defer release()
		time.Sleep(23 * time.Hour)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 1, 1<<30))
		assert.Empty(t, retainedNames(t, s))
		assert.Empty(t, s.log.ActivePath(), "idle expiry leaves the next file lazy")
		status, err := s.Status(ctx)
		require.NoError(t, err)
		require.NotNil(t, status.Inventory)
		assert.Equal(t, Inventory{}, *status.Inventory, "directory-visible bytes exclude an unlinked query-pinned file")
		assert.Equal(t, []string{"first", "last"}, snapshotNames(t, reader))
	})
}

func TestSharedSizePressureRemovesYoungerHistoryOnAppend(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	appendNamed(t, s, "rum", "rum-old-file")
	appendNamed(t, s, "synthetic", "synthetic-old-file")
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, MinHistoryBytes))
	appendNamed(t, s, "rum", "new-file")
	// Creating the successor enforces the shared byte target without waiting for
	// the hourly worker. Both domains in the older file are evicted together.
	assert.Equal(t, []string{"new-file"}, retainedNames(t, s))
	status, err := s.Status(ctx)
	require.NoError(t, err)
	require.NotNil(t, status.Inventory)
	assert.Equal(t, Inventory{Bytes: MinHistoryBytes, Files: 1}, *status.Inventory)
	assert.Empty(t, status.Cleanup.Error)
}

func TestStatusDistinguishesUnknownInventoryAndSafeCleanupFailure(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		ctx := context.Background()
		root := t.TempDir()
		s, err := Open(ctx, root)
		require.NoError(t, err)
		t.Cleanup(func() { require.NoError(t, s.Close()) })
		status, err := s.Status(ctx)
		require.NoError(t, err)
		assert.Nil(t, status.Policy, "no loaded policy has been applied yet")
		require.NotNil(t, status.Inventory)
		assert.Equal(t, Inventory{}, *status.Inventory)
		require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
		appendNamed(t, s, "rum", "before")
		healthy, err := s.Status(ctx)
		require.NoError(t, err)
		require.NotNil(t, healthy.Inventory)
		assert.Equal(t, Inventory{Bytes: MinHistoryBytes, Files: 1}, *healthy.Inventory)
		assert.False(t, healthy.Cleanup.LastSuccessfulAt.IsZero())
		damaged := filepath.Join(filepath.Dir(s.log.ActivePath()), "dem@damaged.journal~")
		require.NoError(t, os.WriteFile(damaged, []byte("preserve evidence"), 0600))
		time.Sleep(time.Hour)
		require.Error(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
		appendNamed(t, s, "rum", "after")
		status, err = s.Status(ctx)
		require.NoError(t, err)
		assert.Nil(t, status.Inventory)
		assert.NotEmpty(t, status.InventoryError)
		assert.NotEmpty(t, status.Cleanup.Error)
		assert.Empty(t, status.WriterError, "safe cleanup failure must not fail a healthy append")
		assert.Equal(t, healthy.Cleanup.LastSuccessfulAt, status.Cleanup.LastSuccessfulAt)
		assert.True(t, status.Cleanup.AttemptedAt.After(healthy.Cleanup.AttemptedAt))
		assert.Equal(t, RetentionPolicy{Days: 30, MaxBytes: 1 << 30}, *status.Policy)
		_, _, err = s.OpenReader(ctx)
		require.Error(t, err, "queries share the unsafe-file rejection")
		bytes, err := os.ReadFile(damaged)
		require.NoError(t, err)
		assert.Equal(t, "preserve evidence", string(bytes))
	})
}

func TestInvalidRetentionLeavesEffectivePolicyUnchanged(t *testing.T) {
	ctx := context.Background()
	s := newTestStore(t)
	require.NoError(t, s.EnforceHistoryRetention(ctx, 30, 1<<30))
	appendEntry(t, s)
	for _, input := range []RetentionPolicy{{Days: 0, MaxBytes: 1 << 30}, {Days: 366, MaxBytes: 1 << 30}, {Days: 30, MaxBytes: MinHistoryBytes - 1}} {
		require.Error(t, s.EnforceHistoryRetention(ctx, input.Days, input.MaxBytes))
		status, err := s.Status(ctx)
		require.NoError(t, err)
		require.NotNil(t, status.Policy)
		assert.Equal(t, RetentionPolicy{Days: 30, MaxBytes: 1 << 30}, *status.Policy)
		assert.Equal(t, 1, countEntries(t, s))
	}
}

func appendNamed(t *testing.T, s *Store, kind, name string) {
	t.Helper()
	clock := "DEM_OBSERVED_US"
	if kind == "synthetic" {
		clock = "DEM_STARTED_US"
	}
	attempted, err := s.Append(context.Background(), []sdk.Field{
		sdk.StringField("DEM_KIND", kind), sdk.StringField(clock, "123456789"), sdk.StringField("MESSAGE", name),
	})
	require.True(t, attempted)
	require.NoError(t, err)
}

func retainedNames(t *testing.T, s *Store) []string {
	t.Helper()
	reader, release, err := s.OpenReader(context.Background())
	require.NoError(t, err)
	defer release()
	return snapshotNames(t, reader)
}

func snapshotNames(t *testing.T, reader *Snapshot) []string {
	t.Helper()
	var names []string
	require.NoError(t, reader.VisitEntries(func(entry *sdk.SnapshotEntry) error {
		return entry.VisitPayloads(func(payload []byte) error {
			name, value, _ := bytes.Cut(payload, []byte("="))
			if string(name) == "MESSAGE" {
				names = append(names, string(value))
			}
			return nil
		})
	}))
	return names
}
