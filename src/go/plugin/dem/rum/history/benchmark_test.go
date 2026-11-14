// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"
	"github.com/netdata/netdata/go/plugins/plugin/dem/rum/aggregate"
	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
)

// 100,000 rich records span 100 interleaved sites and 24 receipt hours in one
// journal. Repeated metadata models adjacent browser reports; varying metadata
// changes every row, including long text with a shared prefix and optional fixed suffix. All exercise
// broad and narrow reads without changing native candidate selection.
func BenchmarkHistoryMultiSiteQueries(b *testing.B) {
	const rows = 100_000
	const start = int64(1_700_000_000)
	const end = start + 86400
	for _, workload := range []struct {
		name, suffix string
		varying      bool
	}{
		{name: "repeated"},
		{name: "varying", varying: true},
		{name: "varying_same_suffix", suffix: "!", varying: true},
	} {
		b.Run(workload.name, func(b *testing.B) {
			ctx := context.Background()
			owner, err := demjournal.Open(ctx, "")
			if err != nil {
				b.Fatal(err)
			}
			defer owner.Close()
			store := NewStore(owner)
			text := strings.Repeat("browser fixture detail ", 32)
			for i := range rows {
				sequence := i / 100
				if workload.varying {
					sequence = i
				}
				value := func(base string) string { return fmt.Sprintf("%s-%d%s", base, sequence, workload.suffix) }
				r := EventRecord{
					Site:         fmt.Sprintf("site-%02d", i%100),
					VnodeGUID:    fmt.Sprintf("%08x-1111-4111-8111-111111111111", i%100/5+1),
					SessionID:    value("session"),
					ObservedUS:   start*1_000_000 + int64(i)*86400*1_000_000/rows,
					Type:         "error",
					Page:         value("/page"),
					Browser:      value("browser"),
					Device:       value("device"),
					Country:      value("country"),
					Version:      value("release"),
					UserID:       value("user"),
					ExperienceID: fmt.Sprintf("document-%d", i),
					View:         value("view"),
					ViewID:       value("view-id"),
					MetricID:     value("metric"),
					Revision:     1,
					Text:         value(text),
					TraceID:      value("trace"),
					Fingerprint:  "fingerprint",
					ErrorType:    value("error-type"),
					Message:      value("message"),
					SampleStack:  value(text),
				}
				if _, err := store.AppendEvent(ctx, r); err != nil {
					b.Fatal(err)
				}
			}
			if err := store.Sync(ctx); err != nil {
				b.Fatal(err)
			}
			for _, tc := range []struct {
				name string
				run  func() error
			}{
				{"sessions_global", func() error { _, err := store.QuerySessions(ctx, "", "", start, end, 0, ""); return err }},
				{"sessions_site", func() error { _, err := store.QuerySessions(ctx, "site-42", "", start, end, 0, ""); return err }},
				{"sessions_recent", func() error { _, err := store.QuerySessions(ctx, "site-42", "", end-900, end, 0, ""); return err }},
				{"errors_guid", func() error {
					_, err := store.QueryErrors(ctx, "", "fingerprint", start, end, "00000009-1111-4111-8111-111111111111")
					return err
				}},
			} {
				b.Run(tc.name, func(b *testing.B) {
					b.ReportAllocs()
					for b.Loop() {
						if err := tc.run(); err != nil {
							b.Fatal(err)
						}
					}
				})
			}
		})
	}
}

// 10,000 retained rows span 200 sessions, seven error fingerprints, and 20
// archives. Receipt time spans 10,000 seconds; the recent window selects 891 rows.
// Snapshot admission captures bounded index metadata per file. Reducers retain only
// distinct summary keys or the requested timeline output, traversing outside the
// writer gate. Selected fingerprint details retain its distinct session/page/
// browser values. Timings are development-machine observations, not CI gates.
func BenchmarkHistoryQueries(b *testing.B) {
	root := b.TempDir()
	host, err := journalhost.Load(journalhost.LoadOptions{
		StateDir: filepath.Join(root, "identity"),
	})
	if err != nil {
		b.Fatal(err)
	}
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
			RotationPolicy: journal.RotationPolicy{}.WithMaxEntries(500),
		},
	)
	if err != nil {
		b.Fatal(err)
	}
	now := time.Now().Truncate(time.Second)
	for i := 0; i < 10000; i++ {
		saved := now.Add(time.Duration(i-9999) * time.Second)
		r := EventRecord{
			VnodeGUID:  "11111111-1111-4111-8111-111111111111",
			Site:       "shop",
			SessionID:  fmt.Sprintf("session-%d", i%200),
			ObservedUS: saved.Add(-10 * time.Second).UnixMicro(),
			Type:       "pageview",
			Page:       fmt.Sprintf("/page/%d", i%20),
			Browser:    fmt.Sprintf("browser-%d", i%3),
			Country:    "DE",
			Device:     "desktop",
		}
		if i%4 == 0 {
			r.Type = "error"
			r.Fingerprint = fmt.Sprintf("fingerprint-%d", i%7)
			r.ErrorType = "TypeError"
			r.Message = "failed request"
			r.SampleStack = "at checkout (app.js:10)"
		}
		opts := host.EntryOptions()
		opts.RealtimeUsec = uint64(saved.UnixMicro())
		if err := log.Append(storedEventFields(r), opts); err != nil {
			b.Fatal(err)
		}
	}
	if err := log.Close(); err != nil {
		b.Fatal(err)
	}
	owner, err := demjournal.Open(context.Background(), root)
	s := NewStore(owner)
	if err != nil {
		b.Fatal(err)
	}
	defer owner.Close()
	for _, tc := range []struct {
		name string
		run  func() error
	}{
		{"snapshot_admission", func() error {
			_, closeReader, err := owner.OpenReader(context.Background())
			if err == nil {
				closeReader()
			}
			return err
		}},
		{"sessions_15m", func() error {
			_, err := s.QuerySessions(context.Background(), "shop", "", now.Add(-15*time.Minute).Unix(), now.Unix(), 0, "")
			return err
		}},
		{"sessions_all", func() error {
			_, err := s.QuerySessions(context.Background(), "shop", "", 0, now.Unix(), 0, "")
			return err
		}},
		{"errors_overview_all", func() error { _, err := s.QueryErrors(context.Background(), "shop", "", 0, now.Unix(), ""); return err }},
		{"error_selected_all", func() error {
			_, err := s.QueryErrors(context.Background(), "shop", "fingerprint-1", 0, now.Unix(), "")
			return err
		}},
	} {
		b.Run(tc.name, func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				if err := tc.run(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}

// Measures synchronous AppendEvent validation, encoding and SDK append. It excludes
// queueing, periodic Sync, rotation and retention. The journal grows throughout each
// run; use a fixed count (e.g. -benchtime=50000x) when comparing implementations.
func BenchmarkHistoryEventAppend(b *testing.B) {
	ctx := context.Background()
	owner, err := demjournal.Open(ctx, "")
	if err != nil {
		b.Fatal(err)
	}
	defer owner.Close()
	store := NewStore(owner)
	event := EventRecord{
		VnodeGUID:   "11111111-1111-4111-8111-111111111111",
		Revision:    1,
		Site:        "shop",
		SessionID:   "session",
		Type:        "error",
		Page:        "/checkout",
		Browser:     "Firefox",
		Country:     "GR",
		Device:      "desktop",
		Version:     "release-1",
		UserID:      "user-id",
		Fingerprint: "fingerprint",
		ErrorType:   "TypeError",
		Message:     "request failed",
		SampleStack: "app.js:10",
	}
	b.ReportAllocs()
	b.ResetTimer()
	for i := range b.N {
		event.ObservedUS = 1_700_000_000_000_000 + int64(i)*1_000_000
		if _, err := store.AppendEvent(ctx, event); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
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

type discardEventWriter struct{}

func (discardEventWriter) AppendEvent(context.Context, EventRecord) (bool, error) { return true, nil }

func (discardEventWriter) Sync(context.Context) error { return nil }

type discardCounters struct{}

func (discardCounters) Add(string, uint64) {}

// Measures the writer's per-batch work without journal I/O: admission checks,
// accounting and one batch sync per full batch. Cost is O(batch) with no
// allocation. Timings are local trends, not CI gates.
func BenchmarkWriterFlush(b *testing.B) {
	w := NewWriter(nil, discardCounters{})
	w.st = discardEventWriter{}
	batch := make([]aggregate.HistoryEvent, maxBatch)
	for i := range batch {
		batch[i] = aggregate.HistoryEvent{
			VnodeGUID:   "11111111-1111-4111-8111-111111111111",
			Revision:    1,
			Site:        "shop",
			SessionID:   "session",
			ObservedUS:  1_700_000_000_000_000 + int64(i),
			Type:        "error",
			Page:        "/checkout",
			Browser:     "Firefox",
			Country:     "GR",
			Device:      "desktop",
			Version:     "release-1",
			UserID:      "user-id",
			Fingerprint: "fingerprint",
			ErrorType:   "TypeError",
			Message:     "request failed",
			SampleStack: "app.js:10",
		}
	}
	ctx := context.Background()
	b.ReportAllocs()
	for b.Loop() {
		if n := w.flush(ctx, batch); n != len(batch) {
			b.Fatalf("flushed %d of %d", n, len(batch))
		}
	}
}
