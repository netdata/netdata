// SPDX-License-Identifier: GPL-3.0-or-later

package store

import (
	"context"
	"os"
	"path/filepath"
	"strings"
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

func appendEvent(t *testing.T, s *Store, r RumEventRecord) {
	t.Helper()
	attempted, err := s.AppendRumEvent(context.Background(), r)
	require.True(t, attempted)
	require.NoError(t, err)
}

func TestJournalReopenAndPrivateCleanup(t *testing.T) {
	ctx := context.Background()
	path := filepath.Join(t.TempDir(), "history")
	s, err := Open(ctx, path)
	require.NoError(t, err)
	event := RumEventRecord{
		Site:        "shop",
		SessionID:   "session",
		TSUnixUS:    time.Now().UnixMicro(),
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
	require.NoError(t, s.Close())
	s, err = Open(ctx, path)
	require.NoError(t, err)
	defer s.Close()
	events, err := s.QueryRumSessionEvents(ctx, "shop", "session")
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, event.TSUnixUS, events[0].TSUnixUS)
	assert.Equal(t, "trace", events[0].TraceID)
	groups, err := s.QueryRumErrors(ctx, "shop", "fp", 0, time.Now().Unix()+1)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.Equal(t, "stack", groups[0].SampleStack)
	assert.Equal(t, "broken", groups[0].Message)
	assert.Equal(t, 1, groups[0].CountWindow)
	private := newTestStore(t)
	privatePath := private.root
	assert.NotEmpty(t, private.temporary)
	appendEvent(t, private, event)
	require.NoError(t, private.Close())
	_, err = os.Stat(privatePath)
	assert.ErrorIs(t, err, os.ErrNotExist)
	_, err = os.Stat(path)
	assert.NoError(t, err)
}

func TestSavedTimeFiltersAndOriginalTimeSummaries(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	saved := time.Now()
	old := saved.Add(-10 * time.Minute)
	records := []RumEventRecord{
		{
			Site:      "shop",
			SessionID: "s",
			TSUnixUS:  old.Add(time.Minute).UnixMicro(),
			Type:      "error",
			Page:      "/last",
			Browser:   "latest",
			UserID:    "u",
		},
		{Site: "shop", SessionID: "s", TSUnixUS: old.UnixMicro(), Type: "pageview", Page: "/first", Browser: "early"},
		{Site: "shop", SessionID: "s", TSUnixUS: old.UnixMicro(), Type: "pageview", Page: "/first", Browser: "early"},
		{Site: "shop", SessionID: "s", TSUnixUS: old.Add(time.Second).UnixMicro(), Type: "frustration", Page: "/first"},
		{Site: "shop", SessionID: "activity", TSUnixUS: old.UnixMicro(), Type: "activity", Page: "/idle"},
		{Site: "other", SessionID: "s", TSUnixUS: old.UnixMicro(), Type: "pageview"},
	}
	for _, r := range records {
		appendEvent(t, s, r)
	}
	sessions, err := s.QueryRumSessions(ctx, "shop", saved.Unix()-1, saved.Unix()+5, 0)
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
	assert.Equal(t, "u", got.UserID)
	events, err := s.QueryRumSessionEvents(ctx, "shop", "s")
	require.NoError(t, err)
	require.Len(t, events, 4)
	assert.Equal(t, "pageview", events[0].Type)
	assert.Equal(t, events[0], events[1], "identical records are separate immutable events")
	assert.Equal(t, "error", events[3].Type)
	events, err = s.QueryRumSessionEvents(ctx, "shop", "activity")
	require.NoError(t, err)
	assert.Empty(t, events)
	sessions, err = s.QueryRumSessions(ctx, "shop", old.Unix()-1, old.Unix()+100, 0)
	require.NoError(t, err)
	assert.Empty(t, sessions, "saved-time filter must not match delayed original timestamps")
	sessions, err = s.QueryRumSessions(ctx, "", 0, saved.Unix()+5, 1)
	require.NoError(t, err)
	assert.Len(t, sessions, 1)
}

func TestSelfContainedErrorsAndSelectedDetails(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	now := time.Now()
	records := []RumEventRecord{
		{
			Site:        "shop",
			TSUnixUS:    now.Add(-time.Minute).UnixMicro(),
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
			TSUnixUS:    now.UnixMicro(),
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
			TSUnixUS:    now.UnixMicro(),
			Type:        "error",
			Fingerprint: "fp",
			Page:        "/a",
			Browser:     "z",
		},
		{
			Site:        "shop",
			SessionID:   "two",
			TSUnixUS:    now.UnixMicro(),
			Type:        "error",
			Fingerprint: "other",
			Page:        "/other",
		},
	}
	for _, r := range records {
		appendEvent(t, s, r)
	}
	groups, err := s.QueryRumErrors(ctx, "shop", "", now.Unix()-1, now.Unix()+5)
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
	groups, err = s.QueryRumErrors(ctx, "shop", "fp", now.Unix()-1, now.Unix()+5)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	assert.True(t, groups[0].Details)
	assert.Equal(t, 1, groups[0].SessionsAffected)
	assert.Equal(t, "/a", groups[0].TopPage)
	assert.Equal(t, []string{"z", "a"}, groups[0].Browsers)
	groups, err = s.QueryRumErrors(ctx, "shop", "missing", 0, now.Unix()+5)
	require.NoError(t, err)
	assert.Empty(t, groups)
}

// Seed an actual old journal through the SDK's lazy writer. An eager file's
// name uses creation time even when its first event is backdated.
func seedOldJournal(t *testing.T, root string, event RumEventRecord, saved time.Time) {
	t.Helper()
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(root, "identity"),
	})
	require.NoError(t, err)
	log, err := journal.NewLog(
		root,
		journal.LogConfig{
			Source: "dem",
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
	require.NoError(t, log.Append(eventFields(event), opts))
	require.NoError(t, log.Close())
}

func TestRetentionIdleExpiryAndRetainedCounts(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	old := time.Now().Add(-48 * time.Hour)
	seedOldJournal(
		t,
		root,
		RumEventRecord{
			Site:      "shop",
			SessionID: "s",
			Type:      "pageview",
			TSUnixUS:  old.UnixMicro(),
			Page:      "/expired",
		},
		old,
	)
	s, err := Open(ctx, root)
	require.NoError(t, err)
	defer s.Close()
	sessions, err := s.QueryRumSessions(ctx, "shop", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 1, 0))
	sessions, err = s.QueryRumSessions(ctx, "shop", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	assert.Empty(t, sessions, "expiry must run without a new event or enabled site")
	fresh := time.Now()
	appendEvent(
		t,
		s,
		RumEventRecord{
			Site:      "shop",
			SessionID: "s",
			Type:      "pageview",
			TSUnixUS:  fresh.UnixMicro(),
			Page:      "/retained",
		},
	)
	sessions, err = s.QueryRumSessions(ctx, "shop", 0, fresh.Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, int64(1), sessions[0].Pageviews)
	assert.Equal(t, fresh.Unix(), sessions[0].StartedAt)
	assert.Equal(t, "/retained", sessions[0].EntryPage)
}

func TestRetentionWholeFilesAndLiveWriter(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
		TSUnixUS:  time.Now().UnixMicro(),
	})
	reader, release, err := s.openReader(ctx)
	require.NoError(t, err)
	defer release()
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 0, 1))
	files, err := filepath.Glob(filepath.Join(s.root, "*", "dem@*.journal"))
	require.NoError(t, err)
	assert.Empty(t, files, "the idle sweep has no active file to protect")
	has, err := reader.Step()
	require.NoError(t, err)
	assert.True(t, has, "opened snapshot survives archive unlink")
	sessions, err := s.QueryRumSessions(ctx, "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	assert.Empty(t, sessions)
	appendEvent(
		t,
		s,
		RumEventRecord{
			Site:      "shop",
			SessionID: "fresh",
			Type:      "activity",
			TSUnixUS:  time.Now().UnixMicro(),
		},
	)
	require.NoError(t, s.Sync(ctx))
	files, err = filepath.Glob(filepath.Join(s.root, "*", "dem@*.journal"))
	require.NoError(t, err)
	require.Len(t, files, 1, "a new append creates a protected active file")
	info, err := os.Stat(files[0])
	require.NoError(t, err)
	assert.Greater(t, info.Size(), int64(1), "maxBytes is not a strict cap")
	sessions, err = s.QueryRumSessions(ctx, "", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, "fresh", sessions[0].SessionID)
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
			Source: "dem",
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
			eventFields(
				RumEventRecord{
					Site:      "shop",
					SessionID: "s",
					Type:      "pageview",
					TSUnixUS:  time.Now().UnixMicro(),
				},
			),
			opts,
		),
	)
	firstPath := log.ActivePath()
	require.NoError(t, log.Close())
	first, err := journal.OpenFile(firstPath)
	require.NoError(t, err)
	sequenceID, sequence := first.Header().SeqnumID(), first.Header().TailEntrySeqnum()
	require.NoError(t, first.Close())
	s, err := Open(ctx, root)
	require.NoError(t, err)
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
		TSUnixUS:  time.Now().UnixMicro(),
	})
	secondPath := s.log.ActivePath()
	require.NoError(t, s.Close())
	second, err := journal.OpenFile(secondPath)
	require.NoError(t, err)
	defer second.Close()
	assert.Equal(t, sequenceID, second.Header().SeqnumID())
	assert.Equal(t, sequence+1, second.Header().HeadEntrySeqnum())
	assert.Equal(t, host.BootID(), second.Header().TailEntryBootID())
	s, err = Open(ctx, root)
	require.NoError(t, err)
	defer s.Close()
	sessions, err := s.QueryRumSessions(ctx, "shop", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	assert.Equal(t, int64(2), sessions[0].Pageviews)
}

func TestAppendReportsAttemptedOnFilesystemFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 0, 1))
	appendEvent(
		t,
		s,
		RumEventRecord{
			Site:      "shop",
			SessionID: "s",
			Type:      "pageview",
			TSUnixUS:  time.Now().UnixMicro(),
			Text:      strings.Repeat("x", 600*1024),
		},
	)
	machineDir := filepath.Dir(s.log.ActivePath())
	moved := machineDir + ".moved"
	require.NoError(t, os.Rename(machineDir, moved))
	attempted, err := s.AppendRumEvent(
		ctx,
		RumEventRecord{
			Site:      "shop",
			SessionID: "s",
			Type:      "error",
			TSUnixUS:  time.Now().UnixMicro(),
		},
	)
	// Restore our task-owned directory before assertions and normal cleanup.
	require.NoError(t, os.Rename(moved, machineDir))
	assert.True(t, attempted, "the SDK was called even though archive publication failed")
	assert.Error(t, err)
	require.NoError(t, s.Sync(ctx))
}

func TestCorruptArchiveReturnsErrorInsteadOfPartialCounts(t *testing.T) {
	s := newTestStore(t)
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
	})
	// This filename is in the owned journal namespace but contains no journal.
	path := filepath.Join(
		filepath.Dir(s.log.ActivePath()),
		"dem@ffffffffffffffffffffffffffffffff-0000000000000001-0000000000000001.journal",
	)
	require.NoError(t, os.WriteFile(path, []byte("invalid journal"), 0600))
	_, err := s.QueryRumSessions(context.Background(), "shop", 0, time.Now().Unix()+5, 0)
	assert.Error(t, err)
	// Keep corrupt files out of retention's SDK chain parser on test shutdown.
	require.NoError(t, os.Rename(path, path+".test-invalid"))
}

func BenchmarkAppendRumEvent(b *testing.B) {
	s, err := Open(context.Background(), b.TempDir())
	if err != nil {
		b.Fatal(err)
	}
	defer s.Close()
	r := RumEventRecord{
		Site:      "shop",
		SessionID: "session",
		TSUnixUS:  time.Now().UnixMicro(),
		Type:      "pageview",
		Page:      "/cart",
		Browser:   "Firefox",
		Device:    "desktop",
		Country:   "DE",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		if _, err := s.AppendRumEvent(context.Background(), r); err != nil {
			b.Fatal(err)
		}
	}
}

func TestDamagedJournalIsReportedAndUnrelatedSourcesAreIgnored(t *testing.T) {
	s := newTestStore(t)
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
	})
	dir := filepath.Dir(s.log.ActivePath())
	require.NoError(t, os.WriteFile(filepath.Join(dir, "unrelated@broken.journal"), []byte("invalid"), 0600))
	sessions, err := s.QueryRumSessions(context.Background(), "shop", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	damaged := filepath.Join(dir, "dem@damaged.journal~")
	require.NoError(t, os.WriteFile(damaged, []byte("quarantined"), 0600))
	_, err = s.QueryRumSessions(context.Background(), "shop", 0, time.Now().Unix()+5, 0)
	require.ErrorContains(t, err, "operator recovery is required")
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "fresh",
		Type:      "activity",
	})
	require.NoError(t, s.EnforceRumHistoryRetention(context.Background(), 1, 100000000))
	data, err := os.ReadFile(damaged)
	require.NoError(t, err)
	assert.Equal(t, "quarantined", string(data))
}

func TestMissingJournalDirectoryReturnsErrorInsteadOfEmptyHistory(t *testing.T) {
	for _, scope := range []string{"root", "machine"} {
		t.Run(scope, func(t *testing.T) {
			s := newTestStore(t)
			appendEvent(t, s, RumEventRecord{
				Site:      "shop",
				SessionID: "s",
				Type:      "pageview",
			})
			path := s.root
			if scope == "machine" {
				path = filepath.Dir(s.log.ActivePath())
			}
			moved := path + ".temporarily-moved"
			require.NoError(t, os.Rename(path, moved))
			_, err := s.QueryRumSessions(context.Background(), "shop", 0, time.Now().Unix()+5, 0)
			require.NoError(t, os.Rename(moved, path))
			assert.ErrorIs(t, err, os.ErrNotExist)
			sessions, err := s.QueryRumSessions(context.Background(), "shop", 0, time.Now().Unix()+5, 0)
			require.NoError(t, err)
			require.Len(t, sessions, 1)
		})
	}
}

// A short SDK age models an archive crossing the old limit between hourly
// sweeps. Relaxing retention must preserve it under the newly loaded policy.
func TestRelaxedRetentionDoesNotApplyPreviousAge(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()
	saved := time.Now()
	seedOldJournal(t, root, RumEventRecord{
		Site:      "shop",
		SessionID: "retained",
		Type:      "pageview",
		TSUnixUS:  saved.UnixMicro(),
	}, saved)
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(root, "identity"),
	})
	require.NoError(t, err)
	config := journal.LogConfig{
		Source:   "dem",
		OpenMode: journal.LogOpenEager,
		Options: journal.Options{
			MachineID: host.MachineID(),
			BootID:    host.BootID(),
			Compact:   true,
		},
		RetentionPolicy: journal.RetentionPolicy{}.WithMaxAge(time.Second).WithMaxBytes(1 << 30),
	}
	log, err := journal.NewLog(root, config)
	require.NoError(t, err)
	s := &Store{
		gate:   make(chan struct{}, 1),
		root:   root,
		host:   host,
		config: config,
		log:    log,
	}
	t.Cleanup(func() { require.NoError(t, s.Close()) })
	before, err := s.QueryRumSessions(ctx, "shop", 0, saved.Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, before, 1)
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "current",
		Type:      "pageview",
		TSUnixUS:  saved.UnixMicro(),
	})
	time.Sleep(time.Until(saved.Add(time.Second + 50*time.Millisecond)))
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 30, 1<<30))
	after, err := s.QueryRumSessions(ctx, "shop", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, after, 2, "an increased allowance must not enforce the retired shorter age")
}

func TestIdleRetentionDoesNotCreateActiveJournals(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
		TSUnixUS:  time.Now().UnixMicro(),
	})
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 30, 1<<30))
	assert.Empty(t, s.log.ActivePath(), "idle sweep must leave the new chain lazy")
	files, err := s.journalPaths(ctx)
	require.NoError(t, err)
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 30, 1<<30))
	after, err := s.journalPaths(ctx)
	require.NoError(t, err)
	assert.Equal(t, files, after, "repeated idle sweeps must not manufacture empty archives")
}

func TestRetentionRecoversAfterArchiveDirectoryFailure(t *testing.T) {
	s := newTestStore(t)
	ctx := context.Background()
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
		TSUnixUS:  time.Now().UnixMicro(),
	})
	machineDir := s.log.JournalDirectory()
	moved := machineDir + ".moved"
	require.NoError(t, os.Rename(machineDir, moved))
	// Force the real SDK archive directory sync to fail after closing its writer.
	require.NoError(t, os.WriteFile(machineDir, []byte("obstruction"), 0600))
	require.Error(t, s.EnforceRumHistoryRetention(ctx, 30, 1<<30))
	_, err := s.AppendRumEvent(ctx, RumEventRecord{
		Site:     "shop",
		Type:     "activity",
		TSUnixUS: time.Now().UnixMicro(),
	})
	require.Error(t, err)
	require.NoError(t, os.Remove(machineDir))
	require.NoError(t, os.Rename(moved, machineDir))
	require.NoError(t, s.EnforceRumHistoryRetention(ctx, 30, 1<<30))
	appendEvent(t, s, RumEventRecord{
		Site:      "shop",
		SessionID: "s",
		Type:      "pageview",
		TSUnixUS:  time.Now().UnixMicro(),
	})
	rows, err := s.QueryRumSessions(ctx, "shop", 0, time.Now().Unix()+5, 0)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	assert.EqualValues(t, 2, rows[0].Pageviews, "recovery preserves earlier history and resumes appends")
}
