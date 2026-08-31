// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/stretchr/testify/require"
)

func TestTopologyDiagnosticArchivePathUsesAgentVarLib(t *testing.T) {
	path := topologyDiagnosticArchivePath(filepath.Join("agent", "varlib"))
	require.Equal(
		t,
		filepath.Join("agent", "varlib", "snmp-topology", "diagnostics", "netdata-snmp-topology-diagnostics.zst"),
		path,
	)
}

func TestWriteTopologyDiagnosticArchiveFileCreatesValidatedArchiveWithPlatformPermissions(t *testing.T) {
	path := filepath.Join(t.TempDir(), "nested", "latest.zst")
	diagnostics := publicationTestDiagnostics(7)

	require.NoError(t, writeTopologyDiagnosticArchiveFile(path, diagnostics, replaceTopologyDiagnosticArchiveFile))

	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	archive, err := ReadDiagnosticArchive(file, DefaultDiagnosticArchiveReadLimits())
	require.NoError(t, err)
	summary, err := archive.Summary()
	require.NoError(t, err)
	require.EqualValues(t, 7, summary.Lifecycle.Sequence)
	require.NoFileExists(t, path+".tmp")

	if runtime.GOOS != "windows" {
		dirInfo, err := os.Stat(filepath.Dir(path))
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o700), dirInfo.Mode().Perm())
		fileInfo, err := os.Stat(path)
		require.NoError(t, err)
		require.Equal(t, os.FileMode(0o600), fileInfo.Mode().Perm())
	}
}

func TestWriteTopologyDiagnosticArchiveFileReplacesOneStableFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.zst")
	require.NoError(t, writeTopologyDiagnosticArchiveFile(
		path,
		publicationTestDiagnostics(1),
		replaceTopologyDiagnosticArchiveFile,
	))

	require.NoError(t, writeTopologyDiagnosticArchiveFile(
		path,
		publicationTestDiagnostics(2),
		replaceTopologyDiagnosticArchiveFile,
	))

	files, err := os.ReadDir(filepath.Dir(path))
	require.NoError(t, err)
	require.Len(t, files, 1)
	require.Equal(t, "latest.zst", files[0].Name())

	file, err := os.Open(path)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, file.Close()) })
	archive, err := ReadDiagnosticArchive(file, DefaultDiagnosticArchiveReadLimits())
	require.NoError(t, err)
	summary, err := archive.Summary()
	require.NoError(t, err)
	require.EqualValues(t, 2, summary.Lifecycle.Sequence)
}

func TestWriteTopologyDiagnosticArchiveFilePreservesPreviousArchiveOnFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.zst")
	require.NoError(t, writeTopologyDiagnosticArchiveFile(
		path,
		publicationTestDiagnostics(1),
		replaceTopologyDiagnosticArchiveFile,
	))
	want, err := os.ReadFile(path)
	require.NoError(t, err)

	t.Run("archive writer", func(t *testing.T) {
		invalid := publicationTestDiagnostics(2)
		invalid.lifecycle.state = diagnosticCaptureState(255)
		err := writeTopologyDiagnosticArchiveFile(path, invalid, replaceTopologyDiagnosticArchiveFile)
		require.ErrorContains(t, err, "write temporary SNMP topology diagnostic archive")
		requirePreviousDiagnosticArchive(t, path, want)
	})

	t.Run("close", func(t *testing.T) {
		closeErr := errors.New("close failed")
		replaceCalled := false
		err := writeTopologyDiagnosticArchiveFileWithClose(
			path,
			publicationTestDiagnostics(2),
			func(file *os.File) error {
				require.NoError(t, file.Close())
				return closeErr
			},
			func(_, _ string) error {
				replaceCalled = true
				return nil
			},
		)
		require.ErrorIs(t, err, closeErr)
		require.False(t, replaceCalled)
		requirePreviousDiagnosticArchive(t, path, want)
	})

	t.Run("replace", func(t *testing.T) {
		replaceErr := errors.New("replace failed")
		err := writeTopologyDiagnosticArchiveFile(path, publicationTestDiagnostics(2), func(_, _ string) error {
			return replaceErr
		})
		require.ErrorIs(t, err, replaceErr)
		requirePreviousDiagnosticArchive(t, path, want)
	})
}

func TestWriteTopologyDiagnosticArchiveFileRemovesStaleTemporaryFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "latest.zst")
	require.NoError(t, os.WriteFile(path+".tmp", []byte("stale"), 0o600))

	require.NoError(t, writeTopologyDiagnosticArchiveFile(
		path,
		publicationTestDiagnostics(1),
		replaceTopologyDiagnosticArchiveFile,
	))
	require.NoFileExists(t, path+".tmp")
}

func TestRunTopologyDiagnosticArchivePublisherPublishesImmediatelyAndSerially(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	ticks := make(chan time.Time)
	refreshes := make(chan struct{}, 1)
	started := make(chan struct{}, 2)
	release := make(chan struct{}, 2)
	done := make(chan struct{})
	var calls atomic.Int32
	var active atomic.Int32
	var maximum atomic.Int32

	go func() {
		defer close(done)
		runTopologyDiagnosticArchivePublisher(ctx, ticks, refreshes, func(bool) bool {
			calls.Add(1)
			current := active.Add(1)
			for {
				previous := maximum.Load()
				if current <= previous || maximum.CompareAndSwap(previous, current) {
					break
				}
			}
			started <- struct{}{}
			<-release
			active.Add(-1)
			return calls.Load() > 1
		})
	}()

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("initial publication did not start")
	}
	refreshes <- struct{}{}
	select {
	case refreshes <- struct{}{}:
		t.Fatal("more than one refresh event must not queue while publication is blocked")
	default:
	}
	release <- struct{}{}

	select {
	case <-started:
	case <-time.After(time.Second):
		t.Fatal("coalesced publication did not start")
	}
	cancel()
	release <- struct{}{}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("publisher did not stop")
	}
	require.EqualValues(t, 2, calls.Load())
	require.EqualValues(t, 1, maximum.Load())
}

func TestRunTopologyDiagnosticArchivePublisherDoesNotPublishAfterCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls atomic.Int32

	runTopologyDiagnosticArchivePublisher(ctx, make(chan time.Time), make(chan struct{}), func(bool) bool {
		calls.Add(1)
		return true
	})

	require.Zero(t, calls.Load())
}

func TestRunTopologyDiagnosticArchivePublisherRetriesRefreshesUntilMeaningfulSuccess(t *testing.T) {
	ctx := t.Context()
	ticks := make(chan time.Time)
	refreshes := make(chan struct{}, 1)
	called := make(chan publicationCall, 4)
	var calls atomic.Int32

	go runTopologyDiagnosticArchivePublisher(ctx, ticks, refreshes, func(requireMeaningful bool) bool {
		call := int(calls.Add(1))
		called <- publicationCall{number: call, requireMeaningful: requireMeaningful}
		return call == 3
	})

	require.Equal(t, publicationCall{number: 1}, receivePublicationCall(t, called))
	refreshes <- struct{}{}
	require.Equal(t, publicationCall{number: 2, requireMeaningful: true}, receivePublicationCall(t, called))
	refreshes <- struct{}{}
	require.Equal(t, publicationCall{number: 3, requireMeaningful: true}, receivePublicationCall(t, called))

	refreshes <- struct{}{}
	select {
	case call := <-called:
		t.Fatalf("refresh publication remained enabled after meaningful success: call %d", call.number)
	case <-time.After(50 * time.Millisecond):
	}

	ticks <- time.Now()
	require.Equal(t, publicationCall{number: 4}, receivePublicationCall(t, called))
}

func TestCollectorDiagnosticArchiveEveryUsesEffectiveRefreshCadence(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.UpdateEvery = 60
	coll.RefreshEvery = confopt.LongDuration(30 * time.Minute)
	require.Equal(t, 30*time.Minute, coll.diagnosticArchiveEvery())

	coll.UpdateEvery = 120
	coll.RefreshEvery = confopt.LongDuration(time.Minute)
	require.Equal(t, 2*time.Minute, coll.diagnosticArchiveEvery())
}

func TestPublishTopologyDiagnosticArchiveDoesNotUseRefreshLock(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	published := make(chan struct{})
	coll.publishDiagnosticArchiveFile = func(string, topologyDiagnostics) error {
		close(published)
		return nil
	}

	coll.refreshMu.Lock()
	defer coll.refreshMu.Unlock()
	done := make(chan struct{})
	meaningful := make(chan bool, 1)
	go func() {
		defer close(done)
		meaningful <- coll.publishTopologyDiagnosticArchiveRecovering(false)
	}()

	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("diagnostic publication waited for the refresh lock")
	}
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("diagnostic publication did not complete")
	}
	require.False(t, <-meaningful)
}

func TestPublishTopologyDiagnosticArchiveReportsOnlyMeaningfulSuccessfulWrites(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	var writes atomic.Int32
	coll.publishDiagnosticArchiveFile = func(string, topologyDiagnostics) error {
		writes.Add(1)
		return nil
	}

	require.False(t, coll.publishTopologyDiagnosticArchiveRecovering(false))
	require.EqualValues(t, 1, writes.Load())
	require.False(t, coll.publishTopologyDiagnosticArchiveRecovering(true))
	require.EqualValues(t, 1, writes.Load())

	store.RegisterJob("job", ddsnmp.DeviceLifecycleInfo{Hostname: "switch.example"})
	require.True(t, coll.publishTopologyDiagnosticArchiveRecovering(true))
	require.EqualValues(t, 2, writes.Load())

	coll.publishDiagnosticArchiveFile = func(string, topologyDiagnostics) error {
		return errors.New("write failed")
	}
	require.False(t, coll.publishTopologyDiagnosticArchiveRecovering(true))
}

func TestCollectorDoesNotPublishDiagnosticArchiveWhenDisabled(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.publishDiagnosticArchive = false
	coll.UpdateEvery = 1
	coll.RefreshEvery = confopt.LongDuration(time.Hour)

	refreshes := make(chan struct{}, 2)
	coll.projectTopologyDiagnosticCut = func(input topologyDiagnosticCutInput) (*topologySweepDiagnosticCut, error) {
		refreshes <- struct{}{}
		return projectTopologyDiagnosticCut(input)
	}
	var writes atomic.Int32
	coll.publishDiagnosticArchiveFile = func(_ string, _ topologyDiagnostics) error {
		writes.Add(1)
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- coll.Run(ctx) }()

	for range 2 {
		select {
		case <-refreshes:
		case <-time.After(2 * time.Second):
			t.Fatal("topology refresh did not run")
		}
	}
	require.Zero(t, writes.Load())

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("collector did not stop")
	}
}

func TestCollectorStartsDiagnosticPublisherAfterInitialRefresh(t *testing.T) {
	coll := newTestSNMPTopologyCollector()
	coll.UpdateEvery = 3600
	coll.RefreshEvery = confopt.LongDuration(time.Hour)

	refreshStarted := make(chan struct{})
	releaseRefresh := make(chan struct{})
	published := make(chan struct{}, 1)
	coll.projectTopologyDiagnosticCut = func(input topologyDiagnosticCutInput) (*topologySweepDiagnosticCut, error) {
		close(refreshStarted)
		<-releaseRefresh
		return projectTopologyDiagnosticCut(input)
	}
	coll.publishDiagnosticArchiveFile = func(_ string, _ topologyDiagnostics) error {
		published <- struct{}{}
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- coll.Run(ctx) }()

	select {
	case <-refreshStarted:
	case <-time.After(time.Second):
		t.Fatal("initial refresh did not start")
	}
	select {
	case <-published:
		t.Fatal("diagnostic archive was published before the initial refresh completed")
	default:
	}
	close(releaseRefresh)
	select {
	case <-published:
	case <-time.After(time.Second):
		t.Fatal("diagnostic archive was not published after the initial refresh")
	}

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("collector did not stop")
	}
}

func TestCollectorPublishesFirstLifecycleRegistrationAfterRefresh(t *testing.T) {
	coll, store := newTestSNMPTopologyCollectorWithStore()
	coll.UpdateEvery = 1
	coll.RefreshEvery = confopt.LongDuration(time.Hour)
	published := make(chan topologyDiagnostics, 4)
	coll.publishDiagnosticArchiveFile = func(_ string, diagnostics topologyDiagnostics) error {
		published <- diagnostics
		return nil
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- coll.Run(ctx) }()

	initial := receivePublishedDiagnostics(t, published)
	require.Empty(t, initial.lifecycle.cut.Entries)

	store.RegisterJob("job", ddsnmp.DeviceLifecycleInfo{Hostname: "switch.example"})
	meaningful := receivePublishedDiagnostics(t, published)
	require.Len(t, meaningful.lifecycle.cut.Entries, 1)
	require.False(t, meaningful.lifecycle.cut.Entries[0].TopologyReady)

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("collector did not stop")
	}
}

func publicationTestDiagnostics(sequence uint64) topologyDiagnostics {
	return topologyDiagnostics{
		lifecycle: topologyJobLifecycleDiagnosticCut{
			state:  diagnosticCaptureAvailable,
			reason: diagnosticCaptureReasonNone,
			cut: ddsnmp.DeviceLifecycleCut{
				Sequence:   sequence,
				CapturedAt: time.Unix(int64(sequence), 0).UTC(),
			},
		},
		producerScopeID: "publication-test",
	}
}

func requirePreviousDiagnosticArchive(t *testing.T, path string, want []byte) {
	t.Helper()
	got, err := os.ReadFile(path)
	require.NoError(t, err)
	require.True(t, bytes.Equal(want, got))
	require.NoFileExists(t, path+".tmp")
}

type publicationCall struct {
	number            int
	requireMeaningful bool
}

func receivePublicationCall(t *testing.T, calls <-chan publicationCall) publicationCall {
	t.Helper()
	select {
	case call := <-calls:
		return call
	case <-time.After(time.Second):
		t.Fatal("diagnostic publication did not run")
		return publicationCall{}
	}
}

func receivePublishedDiagnostics(t *testing.T, published <-chan topologyDiagnostics) topologyDiagnostics {
	t.Helper()
	select {
	case diagnostics := <-published:
		return diagnostics
	case <-time.After(2 * time.Second):
		t.Fatal("diagnostic archive was not published")
		return topologyDiagnostics{}
	}
}
