// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"testing/synctest"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNormalPublicationSchedule(t *testing.T) {
	type step struct {
		after           time.Duration
		device, attempt uint64
		remove, replace bool
		want            []uint64
	}
	for name, tc := range map[string]struct {
		duringWrite   bool
		slowNewWriter bool
		steps         []step
	}{
		"later registrations cannot overtake overdue evidence": {slowNewWriter: true, steps: []step{
			{after: 10 * time.Second, device: 1, attempt: 2, want: []uint64{1}},
			{after: 50 * time.Second, device: 2, attempt: 20, want: []uint64{1}},
			{after: 4*time.Minute + time.Second, want: []uint64{1, 20, 2, 30}},
		}},
		"coalescing and quiet deadline": {steps: []step{
			{after: 10 * time.Second, device: 1, attempt: 2, want: []uint64{1}},
			{after: 10 * time.Second, device: 1, attempt: 3, want: []uint64{1}},
			{after: 279 * time.Second, want: []uint64{1}},
			{after: time.Second, want: []uint64{1, 3}},
			{after: 5 * time.Minute, want: []uint64{1, 3}},
		}},
		"removed pending device stays removed": {steps: []step{
			{after: time.Second, device: 1, attempt: 2, want: []uint64{1}},
			{device: 1, remove: true, want: []uint64{1}},
			{after: 5 * time.Minute, device: 1, attempt: 3, want: []uint64{1}},
		}},
		"replacement gets prompt independent evidence": {steps: []step{
			{after: time.Second, device: 1, attempt: 2, want: []uint64{1}},
			{device: 1, replace: true, attempt: 9, want: []uint64{1, 9}},
			{after: 5 * time.Minute, want: []uint64{1, 9}},
		}},
		"devices keep separate deadlines": {steps: []step{
			{after: time.Minute, device: 2, attempt: 20, want: []uint64{1, 20}},
			{after: 10 * time.Second, device: 1, attempt: 2, want: []uint64{1, 20}},
			{after: 10 * time.Second, device: 2, attempt: 21, want: []uint64{1, 20}},
			{after: 220 * time.Second, want: []uint64{1, 20, 2}},
			{after: time.Minute, want: []uint64{1, 20, 2, 21}},
		}},
		"update during write waits for next deadline": {duringWrite: true, steps: []step{
			{after: 299 * time.Second, want: []uint64{1}},
			{after: time.Second, want: []uint64{1, 2}},
		}},
		"slow poll writes as soon as available": {steps: []step{
			{after: 6 * time.Minute, device: 1, attempt: 2, want: []uint64{1, 2}},
			{after: 5 * time.Minute, want: []uint64{1, 2}},
		}},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, _ := testPublisher(t)
				writers := map[uint64]*NormalWriter{1: p.ReplaceNormal("", "1", 1)}
				writers[1].Update(&normalTestCut{attempt: 1})
				var published []uint64
				var writesMu sync.Mutex
				publishedIDs := func() []uint64 { writesMu.Lock(); defer writesMu.Unlock(); return slices.Clone(published) }
				write := p.writeFile
				p.writeFile = func(ctx context.Context, path string, d Document, replace func(string, string) error) error {
					if tc.slowNewWriter && d.Normal != nil && d.Normal.RegistrationID == 2 {
						time.Sleep(4*time.Minute + time.Second)
					}
					if d.Normal != nil && tc.duringWrite && d.Normal.Latest.ID == 1 {
						writers[1].Update(&normalTestCut{attempt: 2})
					}
					if err := write(ctx, path, d, replace); err != nil {
						return err
					}
					if d.Normal != nil {
						writesMu.Lock()
						published = append(published, d.Normal.Latest.ID)
						writesMu.Unlock()
						if tc.slowNewWriter && d.Normal.RegistrationID == 2 {
							p.ReplaceNormal("", "late", 3).Update(&normalTestCut{attempt: 30})
						}
					}
					return nil
				}
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				go func() { defer close(done); p.Run(ctx) }()
				defer func() { cancel(); <-done }()
				synctest.Wait()
				require.Equal(t, []uint64{1}, publishedIDs())
				for _, s := range tc.steps {
					time.Sleep(s.after)
					owner := fmt.Sprint(s.device)
					if s.remove {
						p.RemoveNormal(owner)
					} else if s.device != 0 {
						if writers[s.device] == nil || s.replace {
							writers[s.device] = p.ReplaceNormal(owner, owner, s.device)
						}
						writers[s.device].Update(&normalTestCut{attempt: s.attempt})
					}
					synctest.Wait()
					require.Equal(t, s.want, publishedIDs())
					if s.remove {
						require.Empty(t, p.normal.queue)
						require.NoFileExists(t, filepath.Join(p.directory, NormalDirectory, p.runID, writers[s.device].filename()))
					}
				}
			})
		})
	}
}

func TestNormalScheduleFailuresAndIndependentStreams(t *testing.T) {
	for name, tc := range map[string]struct{ index bool }{
		"device retry": {}, "index retry does not rewrite device": {index: true},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, store := testPublisher(t)
				first := p.ReplaceNormal("", "first", 1)
				second := p.ReplaceNormal("", "second", 2)
				first.Update(&normalTestCut{attempt: 1})
				second.Update(&normalTestCut{attempt: 1})
				var fail atomic.Bool
				fail.Store(true)
				var writes []uint64
				var writesMu sync.Mutex
				writtenIDs := func() []uint64 { writesMu.Lock(); defer writesMu.Unlock(); return slices.Clone(writes) }
				write := p.writeFile
				p.writeFile = func(ctx context.Context, path string, d Document, replace func(string, string) error) error {
					if fail.Load() && !tc.index && d.Normal != nil && d.Normal.RegistrationID == 1 {
						return errors.New("device write unavailable")
					}
					if err := write(ctx, path, d, replace); err != nil {
						return err
					}
					if d.Normal != nil {
						writesMu.Lock()
						writes = append(writes, d.Normal.RegistrationID)
						writesMu.Unlock()
					}
					return nil
				}
				p.rename = func(from, to string) error {
					if fail.Load() && tc.index && filepath.Base(to) == normalRunsFilename {
						return errors.New("index unavailable")
					}
					return os.Rename(from, to)
				}
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				go func() { defer close(done); p.Run(ctx) }()
				defer func() { cancel(); <-done }()
				synctest.Wait()
				if tc.index {
					assert.ElementsMatch(t, []uint64{1, 2}, writtenIDs())
				} else {
					assert.Equal(t, []uint64{2}, writtenIDs())
				}
				// Neither a fresh poll nor unrelated stream events bypass the delay.
				time.Sleep(10 * time.Second)
				first.Update(&normalTestCut{attempt: 2})
				source := &testTopologySource{}
				source.add(1)
				p.SetTopology("topology", source)
				store.RegisterJob("other", ddsnmp.DeviceLifecycleInfo{})
				synctest.Wait()
				require.Len(t, topologyDocuments(t, p), 1)
				require.Len(t, readFile(t, filepath.Join(p.directory, LifecycleFilename)).Snapshot.Lifecycle.Cut.Entries, 1)
				before := len(writtenIDs())
				fail.Store(false)
				time.Sleep(50 * time.Second)
				synctest.Wait()
				files, err := ListNormalFiles(p.directory)
				require.NoError(t, err)
				require.Len(t, files, 2)
				if tc.index {
					assert.Len(t, writtenIDs(), before)
				} else {
					assert.Len(t, writtenIDs(), before+1)
				}
			})
		})
	}
}

func TestNormalFinalization(t *testing.T) {
	for name, tc := range map[string]struct{ expired, fail bool }{
		"flush latest and preserve through retirement": {},
		"expired shutdown budget":                      {expired: true},
		"one failed device does not prevent another":   {fail: true},
	} {
		t.Run(name, func(t *testing.T) {
			synctest.Test(t, func(t *testing.T) {
				p, _ := testPublisher(t)
				writers := []*NormalWriter{p.ReplaceNormal("", "first", 1), p.ReplaceNormal("", "second", 2)}
				for _, w := range writers {
					w.Update(&normalTestCut{attempt: 1})
				}
				ctx, cancel := context.WithCancel(t.Context())
				done := make(chan struct{})
				go func() { defer close(done); p.Run(ctx) }()
				synctest.Wait()
				for _, w := range writers {
					w.Update(&normalTestCut{attempt: 2})
				}
				synctest.Wait()
				cancel()
				<-done
				shutdown, finish := context.WithCancel(t.Context())
				defer finish()
				if tc.expired {
					finish()
				}
				if tc.fail {
					p.rename = func(from, to string) error {
						if filepath.Base(to) == writers[0].filename() {
							return errors.New("first device unavailable")
						}
						return os.Rename(from, to)
					}
				}
				err := p.Finalize(shutdown)
				if tc.expired || tc.fail {
					require.Error(t, err)
				} else {
					require.NoError(t, err)
				}
				for i, w := range writers {
					p.RemoveNormal(w.owner)
					w.Update(&normalTestCut{attempt: 99})
					d := readFile(t, filepath.Join(p.directory, NormalDirectory, p.runID, w.filename()))
					want := uint64(2)
					if tc.expired || (tc.fail && i == 0) {
						want = 1
					}
					assert.Equal(t, want, d.Normal.Latest.ID)
				}
				require.Nil(t, p.ReplaceNormal("first", "new", 1))
			})
		})
	}
}

func TestNormalCleanupTracksPublishedOwnership(t *testing.T) {
	for name, published := range map[string]struct{ first bool }{
		"no file was ever published":             {},
		"one published file still needs cleanup": {first: true},
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := testPublisher(t)
			writer := p.ReplaceNormal("", "device", 1)
			writer.Update(&normalTestCut{attempt: 1})
			if published.first {
				require.NoError(t, p.publishNormal(t.Context()))
			}
			denied := os.ErrPermission
			removals := 0
			p.writeFile = func(context.Context, string, Document, func(string, string) error) error { return denied }
			p.remove = func(string) error { removals++; return denied }
			for i := range 20 {
				writer = p.ReplaceNormal("device", "device", 1)
				writer.Update(&normalTestCut{attempt: uint64(i + 2)})
				before := removals
				require.ErrorIs(t, p.publishNormal(t.Context()), denied)
				want := 0
				if published.first {
					want = 1
				}
				assert.Equal(t, want, removals-before, "cleanup must follow actual files, not replacement history")
				require.Len(t, p.normal.retired, want)
			}
		})
	}
}
