// SPDX-License-Identifier: GPL-3.0-or-later

package diagnostics

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

func testDocument(sequence uint64) Document {
	return Document{
		Format:  Format,
		Version: Version,
		Kind:    KindLifecycle,
		Snapshot: Snapshot{
			Lifecycle: Lifecycle{
				State:  "available",
				Reason: "none",
				Cut: LifecycleCut{
					Sequence: sequence,
				},
			},
		},
	}
}
func readFile(t *testing.T, path string) Document {
	t.Helper()
	f, err := os.Open(path)
	require.NoError(t, err)
	defer f.Close()
	d, err := Read(f, DefaultReadLimits())
	require.NoError(t, err)
	return d
}
func TestArchivePublicationPathPermissionsReplacement(t *testing.T) {
	require.Equal(
		t,
		filepath.Join("varlib", "snmp", "diagnostics"),
		DirectoryPath("varlib"),
	)
	path := filepath.Join(t.TempDir(), "diagnostics", "latest.zst")
	for _, seq := range []uint64{1, 2} {
		require.NoError(t, writeArchiveFile(t.Context(), path, testDocument(seq), os.Rename))
		require.Equal(t, seq, readFile(t, path).Snapshot.Lifecycle.Cut.Sequence)
	}
	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, files, 1)
	if runtime.GOOS != "windows" {
		info, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), info.Mode().Perm())
		info, err = os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), info.Mode().Perm())
	}
	require.NoError(t, os.WriteFile(path+".tmp", []byte("stale"), 0o600))
	require.NoError(t, writeArchiveFile(t.Context(), path, testDocument(3), os.Rename))
	require.NoFileExists(t, path+".tmp")
}
func TestArchivePublicationFailuresPreservePreviousFile(t *testing.T) {
	for failure := range map[string]struct{}{"encode": {}, "close": {}, "replace": {}, "cancel": {}} {
		t.Run(failure, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "latest.zst")
			require.NoError(t, writeArchiveFile(t.Context(), path, testDocument(1), os.Rename))
			before, err := os.ReadFile(path)
			require.NoError(t, err)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			injected := errors.New("injected failure")
			closeFile := func(f *os.File) error {
				require.NoError(t, f.Close())
				if failure == "close" {
					return injected
				}
				if failure == "cancel" {
					cancel()
				}
				return nil
			}
			replace := func(a, b string) error {
				if failure == "replace" {
					return injected
				}
				return os.Rename(a, b)
			}
			document := testDocument(2)
			if failure == "encode" {
				document.Snapshot.Lifecycle.Cut.CapturedAt = time.Date(10000, 1, 1, 0, 0, 0, 0, time.UTC)
			}
			require.Error(t, writeArchiveFileWithClose(ctx, path, document, closeFile, replace))
			after, err := os.ReadFile(path)
			require.NoError(t, err)
			require.Equal(t, before, after)
			require.NoFileExists(t, path+".tmp")
		})
	}
}

type testCheckpoint struct {
	id      uint64
	capture func() (Snapshot, error)
}

func (c *testCheckpoint) ID() uint64 { return c.id }
func (c *testCheckpoint) Capture() (Snapshot, error) {
	if c.capture != nil {
		return c.capture()
	}
	return Snapshot{Topology: &Sweep{Sequence: c.id}, Lifecycle: testDocument(c.id).Snapshot.Lifecycle}, nil
}

type testTopologySource struct{ checkpoints []Checkpoint }

func (s *testTopologySource) Checkpoints() []Checkpoint {
	return append([]Checkpoint(nil), s.checkpoints...)
}
func (s *testTopologySource) add(id uint64) {
	if len(s.checkpoints) == CheckpointRetention {
		s.checkpoints = s.checkpoints[1:]
	}
	s.checkpoints = append(s.checkpoints, &testCheckpoint{id: id})
}
func testPublisher(t *testing.T) (*Publisher, *ddsnmp.DeviceStore) {
	t.Helper()
	store := ddsnmp.NewDeviceStore()
	return NewPublisher(store, t.TempDir()), store
}
func topologyFiles(t *testing.T, p *Publisher) []CheckpointFile {
	t.Helper()
	entries, err := ListCheckpoints(p.directory)
	require.NoError(t, err)
	return entries
}
func topologyDocuments(t *testing.T, p *Publisher) []Document {
	t.Helper()
	var docs []Document
	for _, entry := range topologyFiles(t, p) {
		docs = append(docs, readFile(t, filepath.Join(p.directory, TopologyDirectory, entry.Filename)))
	}
	return docs
}
func publishTestCheckpoint(t *testing.T, p *Publisher, source *testTopologySource, id uint64) {
	t.Helper()
	source.add(id)
	p.TopologyUpdated(source)
	p.flush(t.Context())
}
func TestPublisherIndependentLifecycleAndTopology(t *testing.T) {
	for name, tc := range map[string]struct{ panicCapture bool }{"capture error": {}, "capture panic": {true}} {
		t.Run(name, func(t *testing.T) {
			p, store := testPublisher(t)
			source := &testTopologySource{checkpoints: []Checkpoint{&testCheckpoint{id: 1, capture: func() (Snapshot, error) {
				if tc.panicCapture {
					panic("injected")
				}
				return Snapshot{}, errors.New("injected")
			}}}}
			p.SetTopology("topology", source)
			writer := store.ReplaceJob("", "device", ddsnmp.DeviceLifecycleInfo{Hostname: "switch.example"}, ddsnmp.DeviceLifecycleStatus{}, nil)
			p.flush(t.Context())
			path := filepath.Join(p.directory, LifecycleFilename)
			d := readFile(t, path)
			require.Equal(t, KindLifecycle, d.Kind)
			require.True(t, d.TopologyActive)
			require.Nil(t, d.Snapshot.Topology)
			require.Len(t, d.Snapshot.Lifecycle.Cut.Entries, 1)
			writer.RecordLifecycle(ddsnmp.DeviceLifecycleStatus{Phase: ddsnmp.DeviceLifecyclePhaseCollect, Outcome: ddsnmp.DeviceLifecycleOutcomeFailed})
			p.flush(t.Context())
			require.Equal(t, "failed", readFile(t, path).Snapshot.Lifecycle.Cut.Entries[0].LastCompleted.Outcome)
			require.Empty(t, topologyFiles(t, p))
		})
	}
}
func TestPublisherWritesOnlyChangedStream(t *testing.T) {
	p, store := testPublisher(t)
	writer := store.ReplaceJob("", "device", ddsnmp.DeviceLifecycleInfo{}, ddsnmp.DeviceLifecycleStatus{}, nil)
	source := &testTopologySource{}
	p.SetTopology("topology", source)
	var writes []string
	p.writeFile = func(ctx context.Context, path string, d Document, replace func(string, string) error) error {
		writes = append(writes, d.Kind)
		return writeArchiveFile(ctx, path, d, replace)
	}
	publishTestCheckpoint(t, p, source, 1)
	require.Equal(t, []string{KindLifecycle, KindTopology}, writes)
	writes = nil
	status := ddsnmp.DeviceLifecycleStatus{Phase: ddsnmp.DeviceLifecyclePhaseCollect, Outcome: ddsnmp.DeviceLifecycleOutcomeSuccess, CompletedAt: time.Now()}
	writer.RecordLifecycle(status)
	p.flush(t.Context())
	require.Equal(t, []string{KindLifecycle}, writes)
	writes = nil
	for range 10 {
		status.CompletedAt = status.CompletedAt.Add(time.Second)
		writer.RecordLifecycle(status)
		p.TopologyUpdated(source)
		p.flush(t.Context())
	}
	require.Empty(t, writes)
	publishTestCheckpoint(t, p, source, 2)
	require.Equal(t, []string{KindTopology}, writes)
}
func TestPublisherCheckpointRotationAndRestart(t *testing.T) {
	p, store := testPublisher(t)
	source := &testTopologySource{}
	p.SetTopology("topology", source)
	for id := uint64(1); id <= 4; id++ {
		publishTestCheckpoint(t, p, source, id)
	}
	docs := topologyDocuments(t, p)
	require.Len(t, docs, 3)
	for i, d := range docs {
		require.Equal(t, uint64(i+2), d.Checkpoint)
		require.Equal(t, d.Checkpoint, d.Snapshot.Topology.Sequence)
		require.NotEmpty(t, d.Producer.RunID)
		require.False(t, d.PublishedAt.IsZero())
	}
	require.NoError(t, os.WriteFile(filepath.Join(p.directory, TopologyDirectory, "unrelated.zst"), []byte("keep"), 0600))
	next := NewPublisher(store, filepath.Dir(filepath.Dir(p.directory)))
	require.Equal(t, p.directory, next.directory)
	next.flush(t.Context())
	require.Len(t, topologyDocuments(t, next), 3, "startup preserves historical topology")
	require.NotEqual(t, p.runID, readFile(t, filepath.Join(next.directory, LifecycleFilename)).Producer.RunID)
	nextSource := &testTopologySource{}
	next.SetTopology("topology", nextSource)
	publishTestCheckpoint(t, next, nextSource, 1)
	docs = topologyDocuments(t, next)
	require.Len(t, docs, 3)
	require.Equal(t, uint64(5), docs[2].Checkpoint)
	require.Equal(t, next.runID, docs[2].Producer.RunID)
	require.Equal(t, p.runID, docs[1].Producer.RunID)
	require.FileExists(t, filepath.Join(p.directory, TopologyDirectory, "unrelated.zst"))
}
func TestPublisherRestartRetriesCheckpointPruning(t *testing.T) {
	for name, tc := range map[string]struct {
		completed    uint64
		retryFailure bool
	}{
		"retained history":                    {3, false},
		"unfinished rotation":                 {4, false},
		"cleanup still failing after restart": {4, true},
	} {
		t.Run(name, func(t *testing.T) {
			p, store := testPublisher(t)
			source := &testTopologySource{}
			p.SetTopology("topology", source)
			p.remove = func(string) error { return errors.New("transient cleanup failure") }
			for id := uint64(1); id <= tc.completed; id++ {
				publishTestCheckpoint(t, p, source, id)
			}
			require.Len(t, topologyFiles(t, p), int(tc.completed))
			unrelated := filepath.Join(p.directory, TopologyDirectory, "unrelated.zst")
			require.NoError(t, os.WriteFile(unrelated, []byte("keep"), 0600))
			restarted := NewPublisher(store, filepath.Dir(filepath.Dir(p.directory)))
			if tc.retryFailure {
				restarted.remove = func(string) error { return errors.New("cleanup remains unavailable") }
				restarted.flush(t.Context())
				require.Len(t, topologyFiles(t, restarted), int(tc.completed))
				restarted.remove = os.Remove
			}
			restarted.flush(t.Context())
			entries := topologyFiles(t, restarted)
			require.Len(t, entries, CheckpointRetention, "startup must recover rotation without a topology producer")
			require.Equal(t, tc.completed-2, entries[0].Sequence)
			require.Equal(t, tc.completed, entries[2].Sequence)
			require.FileExists(t, unrelated)
			nextSource := &testTopologySource{}
			restarted.SetTopology("topology", nextSource)
			publishTestCheckpoint(t, restarted, nextSource, 1)
			entries = topologyFiles(t, restarted)
			require.Len(t, entries, CheckpointRetention)
			require.Equal(t, tc.completed+1, entries[2].Sequence, "cleanup must preserve restart ordering")
		})
	}
}

func TestPublisherCheckpointFailures(t *testing.T) {
	for name, tc := range map[string]struct{ prune bool }{"write failure": {}, "prune failure": {true}} {
		t.Run(name, func(t *testing.T) {
			p, _ := testPublisher(t)
			source := &testTopologySource{}
			p.SetTopology("topology", source)
			for id := uint64(1); id <= 3; id++ {
				publishTestCheckpoint(t, p, source, id)
			}
			injected := errors.New("injected")
			if tc.prune {
				p.remove = func(string) error { return injected }
			} else {
				p.rename = func(string, string) error { return injected }
			}
			publishTestCheckpoint(t, p, source, 4)
			docs := topologyDocuments(t, p)
			if tc.prune {
				require.Len(t, docs, 4)
				require.Equal(t, uint64(4), docs[3].Checkpoint)
			} else {
				require.Len(t, docs, 3)
				require.Equal(t, uint64(1), docs[0].Checkpoint)
			}
			p.remove, p.rename = os.Remove, os.Rename
			p.flush(t.Context()) // retries need no new producer event
			docs = topologyDocuments(t, p)
			require.Len(t, docs, 3)
			require.Equal(t, uint64(2), docs[0].Checkpoint)
			require.Equal(t, uint64(4), docs[2].Checkpoint)
		})
	}
}
func TestPublisherAdmitsAcceptedHistoryAcrossRelease(t *testing.T) {
	p, _ := testPublisher(t)
	incumbent, candidate := &testTopologySource{}, &testTopologySource{}
	incumbent.add(1)
	candidate.add(99)
	p.SetTopology("same-config", incumbent)
	p.TopologyUpdated(candidate)
	p.ReleaseTopology(candidate)
	p.SetTopology("same-config", incumbent) // duplicate reconciliation is not history
	p.ReleaseTopology(incumbent)
	p.TopologyUpdated(incumbent)
	p.flush(t.Context())
	docs := topologyDocuments(t, p)
	require.Len(t, docs, 1)
	require.Equal(t, uint64(1), docs[0].Snapshot.Topology.Sequence)
	require.False(t, readFile(t, filepath.Join(p.directory, LifecycleFilename)).TopologyActive)
	p.SetTopology("same-config", candidate)
	p.ReleaseTopology(incumbent)
	p.flush(t.Context())
	docs = topologyDocuments(t, p)
	require.Len(t, docs, 2)
	require.Equal(t, uint64(99), docs[1].Snapshot.Topology.Sequence)
	require.True(t, readFile(t, filepath.Join(p.directory, LifecycleFilename)).TopologyActive)
}
func TestPublisherSlowWriterRetainsNewestThree(t *testing.T) {
	for name, tc := range map[string]struct {
		blockedKind  string
		preload      uint64
		newest       uint64
		wantCaptured []uint64
	}{
		"lifecycle write": {KindLifecycle, 0, 5, []uint64{3, 4, 5}},
		"topology write":  {KindTopology, 3, 6, []uint64{1, 4, 5, 6}},
	} {
		t.Run(name, func(t *testing.T) {
			p, _ := testPublisher(t)
			source := &testTopologySource{}
			var captured []uint64
			add := func(id uint64) {
				source.add(id)
				source.checkpoints[len(source.checkpoints)-1].(*testCheckpoint).capture = func() (Snapshot, error) {
					captured = append(captured, id)
					return Snapshot{Topology: &Sweep{Sequence: id}, Lifecycle: testDocument(id).Snapshot.Lifecycle}, nil
				}
			}
			for id := uint64(1); id <= tc.preload; id++ {
				add(id)
			}
			p.SetTopology("topology", source)
			entered, release := make(chan struct{}), make(chan struct{})
			var once atomic.Bool
			var completed atomic.Uint64
			p.writeFile = func(ctx context.Context, path string, d Document, replace func(string, string) error) error {
				if d.Kind == tc.blockedKind && !once.Swap(true) {
					close(entered)
					<-release
				}
				err := writeArchiveFile(ctx, path, d, replace)
				if err == nil && d.Kind == KindTopology {
					completed.Store(d.Snapshot.Topology.Sequence)
				}
				return err
			}
			ctx, cancel := context.WithCancel(t.Context())
			done := make(chan struct{})
			go func() { defer close(done); p.Run(ctx) }()
			<-entered
			for id := tc.preload + 1; id <= tc.newest; id++ {
				add(id)
				p.TopologyUpdated(source)
			}
			p.ReleaseTopology(source)
			close(release)
			defer func() { cancel(); <-done }()
			require.Eventually(t, func() bool { return completed.Load() == tc.newest }, time.Second, time.Millisecond)
			cancel()
			<-done
			require.Equal(t, tc.wantCaptured, captured, "evicted checkpoints must not be projected after a slow write")
			require.Empty(t, p.pending, "completed files no longer need publisher-owned raw references")
			docs := topologyDocuments(t, p)
			require.Len(t, docs, 3)
			for i, d := range docs {
				require.Equal(t, tc.newest-2+uint64(i), d.Snapshot.Topology.Sequence)
			}
		})
	}
}
func TestPublisherRunSerializesAndStopsAfterCancellation(t *testing.T) {
	p, store := testPublisher(t)
	var active, maximum, calls atomic.Int32
	entered, release := make(chan struct{}), make(chan struct{})
	p.writeFile = func(ctx context.Context, _ string, _ Document, _ func(string, string) error) error {
		n := active.Add(1)
		defer active.Add(-1)
		maximum.Store(max(maximum.Load(), n))
		calls.Add(1)
		close(entered)
		<-release
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	<-entered
	updates := make(chan struct{})
	go func() {
		store.RegisterJob("device", ddsnmp.DeviceLifecycleInfo{})
		p.RemoveTopology("topology")
		close(updates)
	}()
	select {
	case <-updates:
	case <-time.After(time.Second):
		t.Error("IO blocked collection or ownership")
	}
	cancel()
	close(release)
	<-done
	require.EqualValues(t, 1, maximum.Load())
	require.EqualValues(t, 1, calls.Load())
	p.Run(ctx)
	require.EqualValues(t, 1, calls.Load())
}

func TestPublisherNewSubmissionsDoNotStarveLifecycle(t *testing.T) {
	p, store := testPublisher(t)
	writer := store.ReplaceJob("", "device", ddsnmp.DeviceLifecycleInfo{}, ddsnmp.DeviceLifecycleStatus{}, nil)
	source := &testTopologySource{}
	source.add(1)
	p.SetTopology("topology", source)
	var writes []string
	p.writeFile = func(ctx context.Context, path string, d Document, replace func(string, string) error) error {
		writes = append(writes, d.Kind)
		if d.Kind == KindTopology && d.Snapshot.Topology.Sequence < 4 {
			source.add(d.Snapshot.Topology.Sequence + 1)
			p.TopologyUpdated(source)
			writer.RecordLifecycle(ddsnmp.DeviceLifecycleStatus{Phase: ddsnmp.DeviceLifecyclePhaseCollect, Outcome: ddsnmp.DeviceLifecycleOutcomeFailed})
		}
		return writeArchiveFile(ctx, path, d, replace)
	}
	p.flush(t.Context())
	require.Equal(t, []string{KindLifecycle, KindTopology}, writes)
	p.flush(t.Context())
	require.Equal(t, []string{KindLifecycle, KindTopology, KindLifecycle, KindTopology}, writes)
	require.Equal(t, "failed", readFile(t, filepath.Join(p.directory, LifecycleFilename)).Snapshot.Lifecycle.Cut.Entries[0].LastCompleted.Outcome)
}
