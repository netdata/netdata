// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/agg"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/beacon"
	"github.com/netdata/netdata/go/plugins/plugin/dem/store"
	"github.com/stretchr/testify/require"
)

func newTestStore(t *testing.T) *store.Store {
	t.Helper()
	dir := t.TempDir()
	s, err := store.Open(context.Background(), filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// fakeCounters records Add calls, standing in for *agg.Aggregator.
type fakeCounters struct {
	mu   sync.Mutex
	adds map[string]uint64 // site+"/"+counter -> total
}

func newFakeCounters() *fakeCounters {
	return &fakeCounters{
		adds: map[string]uint64{},
	}
}

func (f *fakeCounters) Add(site, counter string, n uint64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.adds[site+"/"+counter] += n
}

func (f *fakeCounters) get(site, counter string) uint64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.adds[site+"/"+counter]
}

// markerRedactor replaces a fixed marker, so tests can prove redaction ran
// without pulling in internal/secrets' real patterns.
type markerRedactor struct{ marker string }

func (r markerRedactor) Apply(s string) string {
	return strings.ReplaceAll(s, r.marker, "[REDACTED]")
}

func TestWriterFlushesOnTimer(t *testing.T) {
	st := newTestStore(t)
	counters := newFakeCounters()
	w := New(st, counters, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	w.Session(agg.HistorySession{
		Site:      "s",
		SessionID: "sess1",
		StartedAt: 1,
		LastAt:    1,
	})
	w.SessionEvent(
		agg.HistorySessionEvent{
			Site:      "s",
			SessionID: "sess1",
			TSUnixUS:  1_000_000,
			Type:      "pageview",
			Page:      "/a",
		},
	)
	w.ErrorGroup(
		agg.HistoryErrorGroup{
			Site:        "s",
			Fingerprint: "fp1",
			Type:        "E",
			Message:     "m",
			FirstSeen:   1,
			LastSeen:    1,
		},
	)
	w.ErrorOccurrence(agg.HistoryErrorOccurrence{
		Site:        "s",
		Fingerprint: "fp1",
		TS:          1,
		SessionID:   "sess1",
	})

	deadline := time.Now().Add(10 * time.Second)
	for {
		sessions, err := st.QueryRumSessions(context.Background(), "s", 0, 1000, 10)
		if err != nil {
			t.Fatal(err)
		}
		if len(sessions) == 1 {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the timer flush")
		}
		time.Sleep(50 * time.Millisecond)
	}
	if got := counters.get("s", agg.CounterHistoryWritten); got == 0 {
		t.Fatal("expected written counter to be bumped")
	}
	cancel()
	<-done
}

func TestWriterFlushesOnBatchSize(t *testing.T) {
	st := newTestStore(t)
	counters := newFakeCounters()
	w := New(st, counters, nil)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	for i := 0; i < maxBatch; i++ {
		w.ErrorOccurrence(agg.HistoryErrorOccurrence{
			Site:        "s",
			Fingerprint: "fp1",
			TS:          int64(i),
		})
	}
	deadline := time.Now().Add(10 * time.Second)
	for {
		got, err := st.QueryRumErrors(context.Background(), "s", 0, int64(maxBatch)+10)
		if err != nil {
			t.Fatal(err)
		}
		if len(got) == 1 && got[0].CountWindow == maxBatch {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("timed out waiting for size-triggered flush: %+v", got)
		}
		time.Sleep(20 * time.Millisecond)
	}
	cancel()
	<-done
}

func TestWriterFlushesOnShutdown(t *testing.T) {
	st := newTestStore(t)
	counters := newFakeCounters()
	w := New(st, counters, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	w.Session(agg.HistorySession{
		Site:      "s",
		SessionID: "sess1",
		StartedAt: 1,
		LastAt:    1,
	})
	cancel() // before the 5s ticker would ever fire
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return after ctx cancellation")
	}
	sessions, err := st.QueryRumSessions(context.Background(), "s", 0, 1000, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(sessions) != 1 {
		t.Fatalf("expected the queued session to be flushed on shutdown, got %+v", sessions)
	}
}

func TestWriterDropsOnFullChannelWithoutBlocking(t *testing.T) {
	st := newTestStore(t)
	counters := newFakeCounters()
	w := New(st, counters, nil)
	// Run is never started: the channel fills up and every send after
	// capacity must return immediately (never block the caller/ingest).
	for i := 0; i < queueCap+10; i++ {
		w.ErrorOccurrence(agg.HistoryErrorOccurrence{
			Site:        "s",
			Fingerprint: "fp1",
			TS:          int64(i),
		})
	}
	if got := w.chanDrops["s"]; got != 10 {
		t.Fatalf("chanDrops[s] = %d, want 10", got)
	}

	// Now start Run: the accumulated drops must surface on the next flush,
	// even though the drops themselves happened before Run existed.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()
	deadline := time.Now().Add(10 * time.Second)
	for counters.get("s", agg.CounterHistoryDropped) == 0 {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for dropped counter")
		}
		time.Sleep(20 * time.Millisecond)
	}
	if got := counters.get("s", agg.CounterHistoryDropped); got != 10 {
		t.Fatalf("history_dropped = %d, want 10", got)
	}
	cancel()
	<-done
}

// TestWriterConcurrentProducersRace exercises many goroutines calling the
// HistorySink methods (as Aggregator.Ingest would, from whichever
// goroutine handles a given beacon) concurrently with Run's flush loop —
// the scenario that would catch a data race on chanDrops between enqueue
// (producer side) and flush (Run's own goroutine).
func TestWriterConcurrentProducersRace(t *testing.T) {
	st := newTestStore(t)
	counters := newFakeCounters()
	w := New(st, counters, nil)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	var wg sync.WaitGroup
	for g := 0; g < 20; g++ {
		wg.Add(1)
		go func(g int) {
			defer wg.Done()
			for i := 0; i < 200; i++ {
				w.ErrorOccurrence(agg.HistoryErrorOccurrence{
					Site:        "s",
					Fingerprint: "fp1",
					TS:          int64(g*1000 + i),
				})
			}
		}(g)
	}
	wg.Wait()
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}
}

func TestWriterRedactsBeforeStorage(t *testing.T) {
	st := newTestStore(t)
	counters := newFakeCounters()
	w := New(st, counters, markerRedactor{
		marker: "SECRET",
	})

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() { w.Run(ctx); close(done) }()

	w.ErrorGroup(
		agg.HistoryErrorGroup{
			Site:        "s",
			Fingerprint: "fp1",
			Type:        "E",
			Message:     "token=SECRET leaked",
			SampleStack: "at f() SECRET",
			FirstSeen:   1,
			LastSeen:    1,
		},
	)
	w.ErrorOccurrence(agg.HistoryErrorOccurrence{
		Site:        "s",
		Fingerprint: "fp1",
		TS:          1,
		SessionID:   "sess1",
	})
	w.SessionEvent(
		agg.HistorySessionEvent{
			Site:      "s",
			SessionID: "sess1",
			TSUnixUS:  1,
			Type:      "error",
			Text:      "boom SECRET",
		},
	)
	cancel()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("Run did not return")
	}

	events, err := st.QueryRumSessionEvents(context.Background(), "s", "sess1")
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || strings.Contains(events[0].Text, "SECRET") {
		t.Fatalf("session event text must be redacted before storage: %+v", events)
	}

	errs, err := st.QueryRumErrors(context.Background(), "s", 0, 1000)
	if err != nil {
		t.Fatal(err)
	}
	if len(errs) != 1 {
		t.Fatalf("expected 1 aggregated error group, got %+v", errs)
	}
	if strings.Contains(errs[0].Message, "SECRET") || strings.Contains(errs[0].SampleStack, "SECRET") {
		t.Fatalf("message/sample_stack must be redacted before storage: %+v", errs[0])
	}
}

// A browser session persists across independent site runtime replacements.
// Contributions from a new runtime must add to, never replace, earlier history.
func TestSessionHistorySurvivesRuntimeReplacement(t *testing.T) {
	st := newTestStore(t)
	for i := range 2 {
		a := agg.New(5 * time.Minute)
		a.Configure(5*time.Minute, []agg.SiteCfg{{Key: "shop", Name: "Shop", PageGroups: 20, Countries: 20}})
		w := New(st, a, nil)
		a.SetHistorySink(w)
		for j := range 3 {
			a.Ingest(
				&beacon.Beacon{
					Site:      "shop",
					SessionID: "same-browser",
					Path:      fmt.Sprintf("/page-%d-%d", i, j),
					PageGroup: "/page",
					Received:  time.Unix(1000+int64(i*100+j*20), 0),
				},
			)
		}
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		w.Run(ctx)
	}
	rows, err := st.QueryRumSessions(context.Background(), "shop", 0, 10000, 10)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	require.EqualValues(t, 6, rows[0].Pageviews)
	require.EqualValues(t, 1000, rows[0].StartedAt)
	require.EqualValues(t, 1140, rows[0].LastAt)
}

// Site retirement must not remove the credential redactor's protection from
// stored browser metadata, which process Functions read without a live job.
func TestWriterRedactsPersistedMetadata(t *testing.T) {
	st := newTestStore(t)
	w := New(st, newFakeCounters(), markerRedactor{
		marker: "SECRET",
	})
	w.Session(
		agg.HistorySession{
			Site:      "s",
			SessionID: "browser",
			StartedAt: 1,
			LastAt:    2,
			Browser:   "SECRET",
			Device:    "SECRET",
			Country:   "SECRET",
			City:      "SECRET",
			Version:   "SECRET",
			EntryPage: "/SECRET",
			LastPage:  "/SECRET",
			UserID:    "SECRET",
		},
	)
	w.SessionEvent(
		agg.HistorySessionEvent{
			Site:      "s",
			SessionID: "browser",
			TSUnixUS:  1000000,
			Type:      "SECRET",
			Page:      "/SECRET",
			Text:      "SECRET",
		},
	)
	w.ErrorGroup(
		agg.HistoryErrorGroup{
			Site:        "s",
			Fingerprint: "fp",
			Type:        "SECRET",
			Message:     "SECRET",
			FirstSeen:   1,
			LastSeen:    1,
		},
	)
	w.ErrorOccurrence(
		agg.HistoryErrorOccurrence{
			Site:        "s",
			Fingerprint: "fp",
			TS:          1,
			SessionID:   "browser",
			Page:        "/SECRET",
			Browser:     "SECRET",
		},
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	w.Run(ctx)
	sessions, err := st.QueryRumSessions(context.Background(), "s", 0, 10, 10)
	require.NoError(t, err)
	require.Len(t, sessions, 1)
	require.NotContains(t, fmt.Sprint(sessions), "SECRET")
	events, err := st.QueryRumSessionEvents(context.Background(), "s", "browser")
	require.NoError(t, err)
	require.Len(t, events, 1)
	require.NotContains(t, fmt.Sprint(events), "SECRET")
	groups, err := st.QueryRumErrors(context.Background(), "s", 0, 10)
	require.NoError(t, err)
	require.Len(t, groups, 1)
	require.NotContains(t, fmt.Sprint(groups), "SECRET")
}
