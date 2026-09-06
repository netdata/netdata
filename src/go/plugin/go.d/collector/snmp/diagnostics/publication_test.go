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
		filepath.Join("varlib", "snmp-topology", "diagnostics", "netdata-snmp-topology-diagnostics.zst"),
		ArchivePath("varlib"),
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
	for _, failure := range []string{"encode", "close", "replace", "cancel"} {
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

type testTopologySource struct {
	snapshot Snapshot
	capture  func() (Snapshot, error)
}

func (s *testTopologySource) Capture() (Snapshot, error) {
	if s.capture != nil {
		return s.capture()
	}
	return s.snapshot, nil
}
func testPublisher(t *testing.T) (*Publisher, *ddsnmp.DeviceStore) {
	t.Helper()
	store := ddsnmp.NewDeviceStore()
	p := NewPublisher(store, t.TempDir())
	p.path = filepath.Join(t.TempDir(), "latest.zst")
	return p, store
}
func TestPublisherWorksWithoutTopologyAndRetainsPreviousOnProviderFailure(t *testing.T) {
	p, store := testPublisher(t)
	require.False(t, p.publish(t.Context(), false))
	require.FileExists(t, p.path)
	store.ReplaceJob(
		"",
		"device",
		ddsnmp.DeviceLifecycleInfo{
			Hostname:    "switch.example",
			Port:        161,
			SNMPVersion: "2c",
		},
		ddsnmp.DeviceLifecycleStatus{
			Phase:   ddsnmp.DeviceLifecyclePhaseInit,
			Outcome: ddsnmp.DeviceLifecycleOutcomeFailed,
		},
		nil,
	)
	require.True(t, p.publish(t.Context(), true))
	d := readFile(t, p.path)
	require.Nil(t, d.Snapshot.Topology)
	require.Len(t, d.Snapshot.Lifecycle.Cut.Entries, 1)
	require.Equal(t, "failed", d.Snapshot.Lifecycle.Cut.Entries[0].LastCompleted.Outcome)
	before, err := os.ReadFile(p.path)
	require.NoError(t, err)
	for _, panicCapture := range []bool{false, true} {
		source := &testTopologySource{
			capture: func() (Snapshot, error) {
				if panicCapture {
					panic("failure")
				}
				return Snapshot{}, errors.New("failure")
			},
		}
		p.SetTopology("topology", source, time.Minute)
		require.False(t, p.publish(t.Context(), false))
		after, err := os.ReadFile(p.path)
		require.NoError(t, err)
		require.Equal(t, before, after)
	}
}
func TestPublisherFencesReplacementRemovalAndRetiredCleanup(t *testing.T) {
	p, store := testPublisher(t)
	store.RegisterJob("device", ddsnmp.DeviceLifecycleInfo{
		Hostname: "switch.example",
	})
	snapshot := Snapshot{
		Lifecycle:       CaptureLifecycle(store, MaxRecords, MaxLogicalBytes),
		ProducerScopeID: "incumbent",
	}
	incumbent := &testTopologySource{
		snapshot: snapshot,
	}
	p.SetTopology("same-config", incumbent, DefaultInterval)
	require.True(t, p.publish(t.Context(), false))
	successor := &testTopologySource{
		snapshot: snapshot,
	}
	successor.snapshot.ProducerScopeID = "successor"
	// No SetTopology for a rejected candidate: incumbent remains selected.
	p.ReleaseTopology(successor)
	require.True(t, p.publish(t.Context(), false))
	require.Equal(t, "incumbent", readFile(t, p.path).Snapshot.ProducerScopeID)
	p.SetTopology("same-config", successor, 2*time.Minute)
	p.ReleaseTopology(incumbent)
	require.True(t, p.publish(t.Context(), false))
	require.Equal(t, "successor", readFile(t, p.path).Snapshot.ProducerScopeID)
	p.RemoveTopology("different-config")
	require.Equal(t, 2*time.Minute, p.currentInterval())
	p.RemoveTopology("same-config")
	require.True(t, p.publish(t.Context(), false))
	require.Empty(t, readFile(t, p.path).Snapshot.ProducerScopeID)
	require.Equal(t, DefaultInterval, p.currentInterval())
}
func TestPublisherRejectsOwnershipChangeBeforeRename(t *testing.T) {
	for _, change := range []string{"provider", "inventory"} {
		t.Run(change, func(t *testing.T) {
			p, store := testPublisher(t)
			p.publish(t.Context(), false)
			before, err := os.ReadFile(p.path)
			require.NoError(t, err)
			p.writeFile = func(ctx context.Context, path string, d Document, replace func(string, string) error) error {
				if change == "provider" {
					p.SetTopology("new", &testTopologySource{}, DefaultInterval)
				} else {
					store.ReplaceJob("", "new", ddsnmp.DeviceLifecycleInfo{}, ddsnmp.DeviceLifecycleStatus{}, nil)
				}
				return writeArchiveFile(ctx, path, d, replace)
			}
			require.False(t, p.publish(t.Context(), false))
			after, err := os.ReadFile(p.path)
			require.NoError(t, err)
			require.Equal(t, before, after)
		})
	}
}
func TestPublisherRunSerializesAndStopsAfterCancellation(t *testing.T) {
	p, _ := testPublisher(t)
	var active, maximum, calls atomic.Int32
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	p.writeFile = func(ctx context.Context, _ string, _ Document, _ func(string, string) error) error {
		n := active.Add(1)
		defer active.Add(-1)
		maximum.Store(max(maximum.Load(), n))
		calls.Add(1)
		entered <- struct{}{}
		<-release
		return ctx.Err()
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan struct{})
	go func() { p.Run(ctx); close(done) }()
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("publisher not started")
	}
	for range 10 {
		p.notify()
	}
	cancel()
	close(release)
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher did not join")
	}
	require.EqualValues(t, 1, maximum.Load())
	require.EqualValues(t, 1, calls.Load())
	p.Run(ctx)
	require.EqualValues(t, 1, calls.Load())
}

func TestPublisherReplacementDoesNotBlockCollection(t *testing.T) {
	p, store := testPublisher(t)
	writer := store.ReplaceJob("", "device", ddsnmp.DeviceLifecycleInfo{}, ddsnmp.DeviceLifecycleStatus{}, nil)
	entered, release := make(chan struct{}), make(chan struct{})
	p.rename = func(from, to string) error { close(entered); <-release; return os.Rename(from, to) }
	done := make(chan struct{})
	go func() { p.publish(t.Context(), false); close(done) }()
	<-entered
	defer func() { close(release); <-done }()
	notified := make(chan struct{})
	go func() { p.TopologyUpdated(); close(notified) }()
	select {
	case <-notified:
	case <-time.After(time.Second):
		t.Error("topology notification blocked behind filesystem replacement")
	}
	polled := make(chan struct{})
	go func() { writer.RecordLifecycle(ddsnmp.DeviceLifecycleStatus{}); close(polled) }()
	select {
	case <-polled:
	case <-time.After(time.Second):
		t.Error("normal poll blocked behind filesystem replacement")
	}
}

func TestPublisherReplacementDoesNotBlockOwnershipChanges(t *testing.T) {
	p, store := testPublisher(t)
	store.ReplaceJob("", "device", ddsnmp.DeviceLifecycleInfo{}, ddsnmp.DeviceLifecycleStatus{}, nil)
	incumbent := &testTopologySource{
		snapshot: Snapshot{
			ProducerScopeID: "incumbent",
			Topology:        &Sweep{},
			Lifecycle:       CaptureLifecycle(store, MaxRecords, MaxLogicalBytes),
		},
	}
	successor := &testTopologySource{
		snapshot: Snapshot{
			ProducerScopeID: "successor",
			Topology:        &Sweep{},
		},
	}
	p.SetTopology("same-config", incumbent, DefaultInterval)
	entered, release := make(chan struct{}), make(chan struct{})
	p.rename = func(from, to string) error { close(entered); <-release; return os.Rename(from, to) }
	published := make(chan struct{})
	go func() { p.publish(t.Context(), false); close(published) }()
	<-entered
	removed, replaced := make(chan struct{}), make(chan struct{})
	go func() { store.Unregister("device"); close(removed) }()
	go func() { p.SetTopology("same-config", successor, DefaultInterval); close(replaced) }()
	for _, done := range []<-chan struct{}{removed, replaced} {
		select {
		case <-done:
		case <-time.After(time.Second):
			t.Error("ownership transition blocked behind filesystem replacement")
		}
	}
	close(release)
	<-published
	<-removed
	<-replaced
	// The file is a historical checkpoint. An ownership change during rename
	// does not mark the successor's first checkpoint as already published.
	require.Equal(t, "incumbent", readFile(t, p.path).Snapshot.ProducerScopeID)
	require.Empty(t, store.LifecycleCut().Entries)
	require.True(t, p.needsInitialTopology())
	p.rename = os.Rename
	p.publish(t.Context(), false)
	require.Equal(t, "successor", readFile(t, p.path).Snapshot.ProducerScopeID)
	require.False(t, p.needsInitialTopology())
}
