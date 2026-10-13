// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"testing/synctest"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) (*Store, string) {
	t.Helper()
	root := t.TempDir()
	owner, err := demjournal.Open(context.Background(), root)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, owner.Close()) })
	return NewStore(owner), root
}

func activeJournal(t *testing.T, root string) string {
	t.Helper()
	files, err := filepath.Glob(filepath.Join(root, "*", "dem.journal"))
	require.NoError(t, err)
	require.Len(t, files, 1, "active journal missing")
	latest := files[0]
	return latest
}

func appendEvent(t *testing.T, s *Store, r EventRecord) {
	t.Helper()
	attempted, err := s.AppendEvent(context.Background(), r)
	require.True(t, attempted)
	require.NoError(t, err)
}

func TestJournalReopen(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "history")
	owner, err := demjournal.Open(ctx, path)
	s := NewStore(owner)
	require.NoError(t, err)
	event := EventRecord{
		Site:        "shop",
		SessionID:   "session",
		ObservedUS:  time.Now().UnixMicro(),
		Type:        "error",
		Fingerprint: "fp",
		ErrorType:   "TypeError",
		Message:     "broken",
		SampleStack: "stack",
		Text:        "TypeError: broken",
		Browser:     "browser",
		Page:        "/cart",
		TraceID:     "trace",
	}
	appendEvent(t, s, event)
	require.NoError(t, s.Sync(ctx))
	require.NoError(t, s.journal.Close())
	owner, err = demjournal.Open(ctx, path)
	s = NewStore(owner)
	require.NoError(t, err)
	defer s.journal.Close()
	events, err := s.QuerySessionEvents(ctx, "shop", "session")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, event.ObservedUS, events[0].ObservedUS)
	assert.Equal(t, "trace", events[0].TraceID)
	groups, err := s.QueryErrors(ctx, "shop", "fp", 0, time.Now().Unix()+1)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "stack", groups[0].SampleStack)
	assert.Equal(t, "broken", groups[0].Message)
	assert.Equal(t, 1, groups[0].CountWindow)
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestReceiptTimeFiltersAndSelectedSummaries(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	saved := time.Now()
	old := saved.Add(-10 * time.Minute)
	records := []EventRecord{
		{
			Site:       "shop",
			SessionID:  "s",
			ObservedUS: old.Add(time.Minute).UnixMicro(),
			Type:       "error",
			Page:       "/last",
			Browser:    "latest",
			UserID:     "u",
		},
		{Site: "shop", SessionID: "s", ObservedUS: old.UnixMicro(), Type: "pageview", Page: "/first", Browser: "early"},
		{Site: "shop", SessionID: "s", ObservedUS: old.UnixMicro(), Type: "pageview", Page: "/first", Browser: "early"},
		{Site: "shop", SessionID: "s", ObservedUS: old.Add(time.Second).UnixMicro(), Type: "frustration", Page: "/first"},
		{Site: "shop", SessionID: "activity", ObservedUS: old.UnixMicro(), Type: "activity", Page: "/idle"},
		{Site: "other", SessionID: "s", ObservedUS: old.UnixMicro(), Type: "pageview"},
	}
	for _, r := range records {
		appendEvent(t, s, r)
	}
	sessions, err := s.QuerySessions(ctx, "shop", "", old.Unix()-1, old.Unix()+100, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 2)
	got := sessions[0]
	assert.Equal(t, "s", got.SessionID)
	assert.Equal(t, int64(2), got.Pageviews)
	assert.Equal(t, int64(1), got.Errors)
	assert.Equal(t, int64(1), got.Frustrations)
	assert.Equal(t, old.Unix(), got.StartedAt)
	assert.Equal(t, old.Add(time.Minute).Unix(), got.LastAt)
	assert.Equal(t, "/first", got.EntryPage)
	assert.Equal(t, "/last", got.LastPage)
	assert.Equal(t, "latest", got.Browser)
	assert.Equal(t, []string{"u"}, got.UserIDs)
	events, err := s.QuerySessionEvents(ctx, "shop", "s")
	require.NoError(t, err)
	require.Len(t, events, 4)
	assert.Equal(t, "pageview", events[0].Type)
	assert.Equal(t, events[0], events[1], "identical records are separate immutable events")
	assert.Equal(t, "error", events[3].Type)
	events, err = s.QuerySessionEvents(ctx, "shop", "activity")
	require.NoError(t, err)
	assert.Empty(t, events)
	sessions, err = s.QuerySessions(ctx, "shop", "", saved.Unix()-1, saved.Unix()+5, 0)
	require.NoError(t, err)
	assert.Empty(t, sessions, "save time must not change receipt-time membership")
	sessions, err = s.QuerySessions(ctx, "", "", 0, saved.Unix()+5, 1)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
}

func TestSelfContainedErrorsAndSelectedDetails(t *testing.T) {
	s, _ := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	records := []EventRecord{
		{
			Site:        "shop",
			ObservedUS:  now.Add(-time.Minute).UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			ErrorType:   "TypeError",
			Message:     "parentless",
			SampleStack: "stack",
			Page:        "/b",
			Browser:     "z",
		},
		{
			Site:        "shop",
			SessionID:   "one",
			ObservedUS:  now.UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			ErrorType:   "TypeError",
			Message:     "parentless",
			Page:        "/a",
			Browser:     "a",
		},
		{
			Site:        "shop",
			SessionID:   "one",
			ObservedUS:  now.UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			Page:        "/a",
			Browser:     "z",
		},
		{
			Site:        "shop",
			SessionID:   "two",
			ObservedUS:  now.UnixMicro(),
			Type:        "error",
			Fingerprint: "other",
			Page:        "/other",
		},
	}
	for _, r := range records {
		appendEvent(t, s, r)
	}
	groups, err := s.QueryErrors(ctx, "shop", "", now.Unix()-61, now.Unix()+5)
	require.NoError(t, err)
	require.Len(t, groups, 2)
	assert.Equal(t, 3, groups[0].CountWindow)
	assert.False(t, groups[0].Details)
	assert.Zero(t, groups[0].SessionsAffected)
	assert.Empty(t, groups[0].TopPage)
	assert.Nil(t, groups[0].Browsers)
	assert.Equal(t, "parentless", groups[0].Message)
	assert.Equal(t, "stack", groups[0].SampleStack)
	assert.Equal(t, now.Add(-time.Minute).Unix(), groups[0].FirstSeen)
	groups, err = s.QueryErrors(ctx, "shop", "fp", now.Unix()-61, now.Unix()+5)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.True(t, groups[0].Details)
	assert.Equal(t, 1, groups[0].SessionsAffected)
	assert.Equal(t, "/a", groups[0].TopPage)
	assert.Equal(t, []string{"z", "a"}, groups[0].Browsers)
	groups, err = s.QueryErrors(ctx, "shop", "missing", 0, now.Unix()+5)
	require.NoError(t, err)
	assert.Empty(t, groups)
}

// Seed an actual old journal through the SDK's lazy writer. An eager file's
// name uses creation time even when its first event is backdated.
func seedOldJournal(t *testing.T, root string, event EventRecord, saved time.Time) {
	t.Helper()
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(root, "identity"),
	})
	require.NoError(t, err)
	log, err := journal.NewLog(
		root,
		journal.LogConfig{
			Source:              "dem",
			StrictSystemdNaming: true,
			Options: journal.Options{
				MachineID: host.MachineID(),
				BootID:    host.BootID(),
				Compact:   true,
			},
		},
	)
	require.NoError(t, err)
	opts := host.EntryOptions()
	opts.RealtimeUsec = uint64(saved.UnixMicro())
	require.NoError(t, log.Append(storedEventFields(event), opts))
	require.NoError(t, log.Close())
}

func TestRetentionIdleExpiryAndRetainedCounts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	seedOldJournal(
		t,
		root,
		EventRecord{
			Site:       "shop",
			SessionID:  "s",
			Type:       "pageview",
			ObservedUS: old.UnixMicro(),
			Page:       "/expired",
		},
		old,
	)
	owner, err := demjournal.Open(ctx, root)
	s := NewStore(owner)
	require.NoError(t, err)
	defer s.journal.Close()
	sessions, err := s.QuerySessions(ctx, "shop", "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 1, demjournal.MinHistoryBytes))
	sessions, err = s.QuerySessions(ctx, "shop", "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	assert.Empty(t, sessions, "expiry must run without a new event or enabled site")
	fresh := time.Now()
	appendEvent(
		t,
		s,
		EventRecord{
			Site:       "shop",
			SessionID:  "s",
			Type:       "pageview",
			ObservedUS: fresh.UnixMicro(),
			Page:       "/retained",
		},
	)
	sessions, err = s.QuerySessions(ctx, "shop", "", 0, fresh.Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, int64(1), sessions[0].Pageviews)
	assert.Equal(t, fresh.Unix(), sessions[0].StartedAt)
	assert.Equal(t, "/retained", sessions[0].EntryPage)
}

func TestRetentionWholeFilesAndLiveWriter(t *testing.T) {
	synctest.Test(t, func(t *testing.T) {
		s, root := newTestStore(t)
		ctx := context.Background()
		appendEvent(t, s, EventRecord{
			Site:       "shop",
			SessionID:  "s",
			Type:       "pageview",
			ObservedUS: time.Now().UnixMicro(),
		})
		reader, release, err := s.journal.OpenReader(ctx)
		require.NoError(t, err)
		defer release()
		time.Sleep(48 * time.Hour)
		require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 1, demjournal.MinHistoryBytes))
		files, err := filepath.Glob(filepath.Join(root, "*", "dem@*.journal"))
		require.NoError(t, err)
		assert.Empty(t, files, "expired idle history is unlinked")
		var retained int
		require.NoError(t, reader.VisitEntries(func(*journal.SnapshotEntry) error { retained++; return nil }))
		assert.Equal(t, 1, retained, "opened snapshot survives archive unlink")
		sessions, err := s.QuerySessions(ctx, "", "", 0, time.Now().Unix()+5, 0)
		require.NoError(t, err)
		assert.Empty(t, sessions)
		appendEvent(
			t,
			s,
			EventRecord{
				Site:       "shop",
				SessionID:  "fresh",
				Type:       "activity",
				ObservedUS: time.Now().UnixMicro(),
			},
		)
		require.NoError(t, s.Sync(ctx))
		files, err = filepath.Glob(filepath.Join(root, "*", "dem.journal"))
		require.NoError(t, err)
		require.Len(t, files, 1, "a new append creates a protected active file")
		info, err := os.Stat(files[0])
		require.NoError(t, err)
		assert.GreaterOrEqual(t, info.Size(), int64(demjournal.MinHistoryBytes), "file lengths include allocation space")
		sessions, err = s.QuerySessions(ctx, "", "", 0, time.Now().Unix()+5, 0)
		require.NoError(t, err)
		require.Len(t, sessions, 1)
		assert.Equal(t, "fresh", sessions[0].SessionID)
	})
}

func TestChainContinuesAcrossBootIdentityChange(t *testing.T) {
	root := t.TempDir()
	ctx := context.Background()
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(root, "identity"),
	})
	require.NoError(t, err)
	oldBoot, err := journal.NewUUID()
	require.NoError(t, err)
	log, err := journal.NewLog(
		root,
		journal.LogConfig{
			Source:              "dem",
			StrictSystemdNaming: true,
			Options: journal.Options{
				MachineID: host.MachineID(),
				BootID:    oldBoot,
				Compact:   true,
			},
		},
	)
	require.NoError(t, err)
	opts := host.EntryOptions()
	opts.BootID = oldBoot
	require.NoError(
		t,
		log.Append(
			storedEventFields(
				EventRecord{
					Site:       "shop",
					SessionID:  "s",
					Type:       "pageview",
					ObservedUS: time.Now().UnixMicro(),
				},
			),
			opts,
		),
	)
	require.NoError(t, log.Close())
	archives, err := filepath.Glob(filepath.Join(root, "*", "dem@*.journal"))
	require.NoError(t, err)
	require.Len(t, archives, 1)
	firstPath := archives[0]
	first, err := journal.OpenFile(firstPath)
	require.NoError(t, err)
	sequenceID, sequence := first.Header().SeqnumID(), first.Header().TailEntrySeqnum()
	require.NoError(t, first.Close())
	owner, err := demjournal.Open(ctx, root)
	s := NewStore(owner)
	require.NoError(t, err)
	appendEvent(t, s, EventRecord{
		Site:       "shop",
		SessionID:  "s",
		Type:       "pageview",
		ObservedUS: time.Now().UnixMicro(),
	})
	second, err := journal.OpenFile(activeJournal(t, root))
	require.NoError(t, err)
	defer second.Close()
	assert.Equal(t, sequenceID, second.Header().SeqnumID())
	assert.Equal(t, sequence+1, second.Header().HeadEntrySeqnum())
	assert.Equal(t, host.BootID(), second.Header().TailEntryBootID())
	require.NoError(t, s.journal.Close())
	owner, err = demjournal.Open(ctx, root)
	s = NewStore(owner)
	require.NoError(t, err)
	defer s.journal.Close()
	sessions, err := s.QuerySessions(ctx, "shop", "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, int64(2), sessions[0].Pageviews)
}

func TestAppendReportsAttemptedOnFilesystemFailure(t *testing.T) {
	s, root := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 1, demjournal.MinHistoryBytes))
	// Obstruct creation of the first lazy file, before any writer mutation.
	moved := filepath.Join(t.TempDir(), "moved-root")
	require.NoError(t, os.Rename(root, moved))
	require.NoError(t, os.WriteFile(root, []byte("obstruction"), 0600))
	attempted, appendErr := s.AppendEvent(ctx, EventRecord{
		Site:       "shop",
		SessionID:  "failed",
		Type:       "pageview",
		ObservedUS: time.Now().UnixMicro(),
	})
	require.NoError(t, os.Remove(root))
	require.NoError(t, os.Rename(moved, root))
	assert.True(t, attempted, "the SDK was called even though lazy file creation failed")
	require.Error(t, appendErr)
	appendEvent(t, s, EventRecord{
		Site:       "shop",
		SessionID:  "fresh",
		Type:       "pageview",
		ObservedUS: time.Now().UnixMicro(),
	})
	require.NoError(t, s.Sync(ctx))
	sessions, err := s.QuerySessions(ctx, "shop", "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "fresh", sessions[0].SessionID)
}

func TestCorruptArchiveReturnsErrorInsteadOfPartialCounts(t *testing.T) {
	s, root := newTestStore(t)
	appendEvent(t, s, EventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
	})
	// This filename is in the owned journal namespace but contains no journal.
	path := filepath.Join(
		filepath.Dir(activeJournal(t, root)),
		"dem@ffffffffffffffffffffffffffffffff-0000000000000001-0000000000000001.journal",
	)
	require.NoError(t, os.WriteFile(path, []byte("invalid journal"), 0600))
	_, err := s.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+5, 0)
	assert.Error(t, err)
	// Keep corrupt files out of retention's SDK chain parser on test shutdown.
	require.NoError(t, os.Rename(path, path+".test-invalid"))
}

func BenchmarkAppendRumEvent(b *testing.B) {
	owner, err := demjournal.Open(context.Background(), b.TempDir())
	s := NewStore(owner)
	if err != nil {
		b.Fatal(err)
	}
	defer s.journal.Close()
	r := EventRecord{
		Site:       "shop",
		SessionID:  "session",
		ObservedUS: time.Now().UnixMicro(),
		Type:       "pageview",
		Page:       "/cart",
		Browser:    "Firefox",
		Device:     "desktop",
		Country:    "DE",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := s.AppendEvent(context.Background(), r); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDamagedJournalIsReportedAndUnrelatedSourcesAreIgnored(t *testing.T) {
	s, root := newTestStore(t)
	appendEvent(t, s, EventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
	})
	dir := filepath.Dir(activeJournal(t, root))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated@broken.journal"), []byte("invalid"), 0600))
	sessions, err := s.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	damaged := filepath.Join(dir, "dem@damaged.journal~")
	require.NoError(t, os.WriteFile(damaged, []byte("quarantined"), 0600))
	_, err = s.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+5, 0)
	require.Error(t, err)
	assert.Contains(t, err.Error(), filepath.Base(damaged))
	appendEvent(t, s, EventRecord{
		Site:      "shop",
		SessionID: "fresh",
		Type:      "activity",
	})
	require.Error(t, s.journal.EnforceHistoryRetention(context.Background(), 1, 100000000))
	status, statusErr := s.journal.Status(context.Background())
	require.NoError(t, statusErr)
	assert.NotEmpty(t, status.Cleanup.Error)
	assert.Empty(t, status.WriterError)
	appendEvent(t, s, EventRecord{
		Site:      "shop",
		SessionID: "after-cleanup-error",
		Type:      "activity",
	})
	data, err := os.ReadFile(damaged)
	require.NoError(t, err)
	assert.Equal(t, "quarantined", string(data))
}

func TestMissingJournalDirectoryReturnsErrorInsteadOfEmptyHistory(t *testing.T) {
	for _, scope := range []string{"root", "machine"} {
		t.Run(scope, func(t *testing.T) {
			s, root := newTestStore(t)
			appendEvent(t, s, EventRecord{
				Site:      "shop",
				SessionID: "s",
				Type:      "pageview",
			})
			path := root
			if scope == "machine" {
				path = filepath.Dir(activeJournal(t, root))
			}
			moved := filepath.Join(t.TempDir(), "temporarily-moved")
			require.NoError(t, os.Rename(path, moved))
			_, err := s.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+5, 0)
			require.NoError(t, os.Rename(moved, path))
			assert.ErrorIs(t, err, os.ErrNotExist)
			sessions, err := s.QuerySessions(context.Background(), "shop", "", 0, time.Now().Unix()+5, 0)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
		})
	}
}

func TestIdleRetentionDoesNotCreateActiveJournals(t *testing.T) {
	s, root := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 30, 1<<30))
	appendEvent(t, s, EventRecord{
		Site:       "shop",
		SessionID:  "s",
		Type:       "pageview",
		ObservedUS: time.Now().UnixMicro(),
	})
	active := activeJournal(t, root)
	require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 30, 1<<30))
	files, err := filepath.Glob(filepath.Join(root, "*", "dem@*.journal"))
	require.NoError(t, err)

	require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 30, 1<<30))
	after, err := filepath.Glob(filepath.Join(root, "*", "dem@*.journal"))
	require.NoError(t, err)
	assert.Empty(t, files, "healthy idle maintenance must not force an archive")
	assert.Equal(t, files, after, "repeated idle sweeps must not manufacture empty archives")
	assert.Equal(t, active, activeJournal(t, root), "healthy maintenance keeps the same writer file")
}

func TestRetentionRetriesAfterInventoryDirectoryFailure(t *testing.T) {
	s, root := newTestStore(t)
	ctx := context.Background()
	appendEvent(t, s, EventRecord{
		Site:       "shop",
		SessionID:  "s",
		Type:       "pageview",
		ObservedUS: time.Now().UnixMicro(),
	})
	machineDir := filepath.Dir(activeJournal(t, root))
	moved := filepath.Join(t.TempDir(), "moved-machine")
	require.NoError(t, os.Rename(machineDir, moved))
	require.NoError(t, os.WriteFile(machineDir, []byte("obstruction"), 0600))
	cleanupErr := s.journal.EnforceHistoryRetention(ctx, 30, 1<<30)
	attempted, appendErr := s.AppendEvent(ctx, EventRecord{
		Site:       "shop",
		SessionID:  "s",
		Type:       "pageview",
		ObservedUS: time.Now().UnixMicro(),
	})
	status, statusErr := s.journal.Status(ctx)
	require.NoError(t, os.Remove(machineDir))
	require.NoError(t, os.Rename(moved, machineDir))
	require.Error(t, cleanupErr)
	require.NoError(t, appendErr)
	assert.True(t, attempted)
	require.NoError(t, statusErr)
	assert.Empty(t, status.WriterError)
	assert.NotEmpty(t, status.Cleanup.Error)
	assert.NotEmpty(t, status.InventoryError)
	assert.Nil(t, status.Inventory)
	require.NoError(t, s.journal.EnforceHistoryRetention(ctx, 30, 1<<30))
	status, err := s.journal.Status(ctx)
	require.NoError(t, err)
	assert.Empty(t, status.Cleanup.Error)
	rows, err := s.QuerySessions(ctx, "shop", "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, rows[0].Pageviews, "inventory recovery preserves both appends without reopening")
}

// SDK fixtures independently encode the documented envelope; production appends derive it.
func storedEventFields(r EventRecord) []journal.Field {
	return append(eventFields(r), journal.StringField("DEM_SCHEMA", "1"),
		journal.StringField("DEM_RUM_MINUTE", strconv.FormatInt(r.ObservedUS/60_000_000, 10)))
}
