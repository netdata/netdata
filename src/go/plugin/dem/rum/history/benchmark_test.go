// SPDX-License-Identifier: GPL-3.0-or-later

package history

import (
	"context"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	demjournal "github.com/netdata/netdata/go/plugins/plugin/dem/journal"

	"github.com/netdata/systemd-journal-sdk/go/journal"
	"github.com/netdata/systemd-journal-sdk/go/journalhost"
)

// 10,000 retained rows span 200 sessions, seven error fingerprints, and 20
// archives. Saved time spans 10,000 seconds; the recent window selects 900 rows.
// Snapshot admission stores O(retained rows) offsets. Reducers then retain only
// distinct summary keys or the requested timeline output, scanning outside the
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
			Source: "dem",
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
			Site:      "shop",
			SessionID: fmt.Sprintf("session-%d", i%200),
			TSUnixUS:  saved.Add(-10 * time.Second).UnixMicro(),
			Type:      "pageview",
			Page:      fmt.Sprintf("/page/%d", i%20),
			Browser:   fmt.Sprintf("browser-%d", i%3),
			Country:   "DE",
			Device:    "desktop",
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
		if err := log.Append(eventFields(r), opts); err != nil {
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
			_, err := s.QuerySessions(context.Background(), "shop", now.Add(-15*time.Minute).Unix(), now.Unix(), 0)
			return err
		}},
		{"sessions_all", func() error { _, err := s.QuerySessions(context.Background(), "shop", 0, now.Unix(), 0); return err }},
		{"errors_overview_all", func() error { _, err := s.QueryErrors(context.Background(), "shop", "", 0, now.Unix()); return err }},
		{"error_selected_all", func() error {
			_, err := s.QueryErrors(context.Background(), "shop", "fingerprint-1", 0, now.Unix())
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
