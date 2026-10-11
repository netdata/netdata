// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	goruntime "runtime"
	"sync/atomic"
	"testing"
	"time"

	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

// The fixture goes through the real authenticated import and binding path; its
// synthetic executable bytes are never launched.
func testBundle(t *testing.T) (runtime.Manifest, []byte) {
	t.Helper()
	slot := make([]byte, 1024)
	files := []struct {
		name string
		data []byte
		mode int64
	}{
		{"node", slot, 0755}, {"browser", slot, 0755}, {"loader", []byte("loader"), 0755},
		{"dependencies/package.json", []byte("{}"), 0644}, {"fonts/font", []byte("font"), 0644},
	}
	var raw, compressed bytes.Buffer
	tw := tar.NewWriter(&raw)
	var expanded int64
	for _, f := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{Name: f.name, Mode: f.mode, Size: int64(len(f.data)), Typeflag: tar.TypeReg}))
		_, err := tw.Write(f.data)
		require.NoError(t, err)
		expanded += int64(len(f.data))
	}
	require.NoError(t, tw.Close())
	gz := gzip.NewWriter(&compressed)
	_, err := gz.Write(raw.Bytes())
	require.NoError(t, err)
	require.NoError(t, gz.Close())
	m := runtime.Manifest{Format: 1, Version: "host-test", Platform: "linux/" + goruntime.GOARCH,
		Archive:  runtime.Archive{SHA256: fmt.Sprintf("%x", sha256.Sum256(compressed.Bytes())), Bytes: int64(compressed.Len()), TarBytes: int64(raw.Len()), ExpandedBytes: expanded, Entries: len(files)},
		Layout:   runtime.Layout{Node: "node", Browser: "browser", Loader: "loader", Dependencies: "dependencies", Fonts: "fonts"},
		Bindings: []runtime.Binding{{Path: "node", Expected: slot}, {Path: "browser", Expected: slot}}}
	require.NoError(t, m.Validate())
	return m, compressed.Bytes()
}

func testHost(t *testing.T, config Config) (*Host, *runtime.Store, runtime.Manifest, []byte) {
	t.Helper()
	m, data := testBundle(t)
	store := config.Store
	if store == nil {
		var err error
		store, err = runtime.Open(runtime.DefaultConfig(filepath.Join(t.TempDir(), "store")))
		require.NoError(t, err)
	}
	config.Store = store
	config.Manifest = &m
	h := New(config)
	h.pollInterval = 10 * time.Millisecond
	h.retryInterval = 20 * time.Millisecond
	h.sweepInterval = time.Hour
	t.Cleanup(func() { require.NoError(t, h.Close()); require.NoError(t, store.Close()) })
	return h, store, m, data
}

func startHost(t *testing.T, h *Host) {
	t.Helper()
	go h.Run(context.Background())
	t.Cleanup(func() { require.NoError(t, h.Close()) })
}
func waitReady(t *testing.T, h *Host) {
	t.Helper()
	require.Eventually(t, func() bool { return h.Ready() == nil }, 3*time.Second, 5*time.Millisecond)
}
func enable(t *testing.T, h *Host, previous, next byte) collectorapi.PreparedJobConfigIntent {
	t.Helper()
	p, err := h.Prepare(collectorapi.JobConfigIdentity{previous}, collectorapi.JobConfigIdentity{next}, nil)
	require.NoError(t, err)
	return p
}
func importBundle(t *testing.T, s *runtime.Store, m runtime.Manifest, data []byte) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "archive")
	require.NoError(t, os.WriteFile(file, data, 0600))
	require.NoError(t, s.Import(context.Background(), m, file))
}

func TestIntentAbortAndCommitUseRealStore(t *testing.T) {
	h, s, m, data := testHost(t, Config{})
	importBundle(t, s, m, data)
	startHost(t, h)
	waitReady(t, h)
	candidate := enable(t, h, 0, 1)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.PendingClaims)
	require.Zero(t, status.DemandGenerations)
	require.NoError(t, candidate.Abort())
	require.Zero(t, h.Status().Enabled)
	enabled := enable(t, h, 0, 1)
	enabled.Commit()
	waitReady(t, h)
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.DemandGenerations)
	require.Zero(t, status.PendingClaims)
	replacement := enable(t, h, 1, 2)
	require.NoError(t, replacement.Abort())
	require.Equal(t, 1, h.Status().Enabled)
	replacement = enable(t, h, 1, 2)
	replacement.Commit()
	waitReady(t, h)
	require.Equal(t, 1, h.Status().Enabled)
	use, err := h.BeginUse()
	require.NoError(t, err)
	require.NotNil(t, use.Completion())
	require.FileExists(t, use.Paths().Node)
	enable(t, h, 2, 0).Commit()
	require.Eventually(t, func() bool { status, err := s.Inspect(); return err == nil && status.DemandGenerations == 0 }, time.Second, 5*time.Millisecond)
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.ExecutionPins)
	require.NoError(t, use.Finish(false))
	require.NoError(t, h.Close())
	status, err = s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.UncertainPins)
}

func TestSharedStoreImportWakesOverlappingSubscribers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	store, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	h, _, m, data := testHost(t, Config{Store: store})
	var oldCalls, newCalls atomic.Int32
	unsub := h.SubscribeDependencies(func(kind, name string) {
		require.Equal(t, DependencyKind, kind)
		require.Equal(t, DependencyName, name)
		_ = h.Ready()
		oldCalls.Add(1)
	})
	h.SubscribeDependencies(func(string, string) { newCalls.Add(1) })
	startHost(t, h)
	enable(t, h, 0, 1).Commit()
	require.Eventually(t, func() bool { return h.Status().State == StateUnavailable }, time.Second, 5*time.Millisecond)
	unsub()
	oldBefore := oldCalls.Load()
	other, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	defer other.Close()
	importBundle(t, other, m, data)
	waitReady(t, h)
	require.Equal(t, oldBefore, oldCalls.Load())
	require.Positive(t, newCalls.Load())
	require.Empty(t, h.Status().SetupError)
}

func TestUnavailableSourceAcceptsIntentWithoutStoppingService(t *testing.T) {
	h := New(Config{Unavailable: fmt.Errorf("manifest not installed")})
	startHost(t, h)
	enable(t, h, 0, 1).Commit()
	require.Equal(t, 1, h.Status().Enabled)
	require.ErrorContains(t, h.Ready(), "manifest not installed")
	require.NoError(t, enable(t, h, 1, 2).Abort())
	enable(t, h, 1, 0).Commit()
	require.Zero(t, h.Status().Enabled)
}

func TestDownloadOnlyAfterCommitAndCancellationJoins(t *testing.T) {
	m, _ := testBundle(t)
	var requests atomic.Int32
	started := make(chan struct{}, 1)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		started <- struct{}{}
		<-r.Context().Done()
	}))
	defer server.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	h, s, _, _ := testHost(t, Config{AllowDownload: true})
	m.Archive.URL = server.URL
	h.manifest = &m
	startHost(t, h)
	candidate := enable(t, h, 0, 1)
	require.NoError(t, candidate.Abort())
	require.Zero(t, requests.Load())
	enable(t, h, 0, 1).Commit()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("download did not start")
	}
	require.NoError(t, h.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.DemandGenerations)
	require.Zero(t, status.ReservedBytes)
	require.ErrorIs(t, s.Available(m), runtime.ErrUnavailable)
}

func TestDownloadFailureRetriesThenPublishes(t *testing.T) {
	m, data := testBundle(t)
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if requests.Add(1) == 1 {
			w.WriteHeader(http.StatusServiceUnavailable)
			return
		}
		_, _ = w.Write(data)
	}))
	defer server.Close()
	oldTransport := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	t.Cleanup(func() { http.DefaultTransport = oldTransport })
	h, s, _, _ := testHost(t, Config{AllowDownload: true})
	m.Archive.URL = server.URL
	h.manifest = &m
	startHost(t, h)
	enable(t, h, 0, 1).Commit()
	waitReady(t, h)
	require.GreaterOrEqual(t, requests.Load(), int32(2))
	require.NoError(t, s.Available(m))
	require.Empty(t, h.Status().SetupError)
}

func TestOverlappingRunIdentitiesRetainDemand(t *testing.T) {
	h, s, m, data := testHost(t, Config{})
	importBundle(t, s, m, data)
	startHost(t, h)
	enable(t, h, 0, 1).Commit()
	enable(t, h, 0, 1).Commit()
	waitReady(t, h)
	require.Equal(t, 2, h.Status().Enabled)
	enable(t, h, 1, 0).Commit()
	waitReady(t, h)
	require.Equal(t, 1, h.Status().Enabled)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.DemandGenerations)
	enable(t, h, 1, 0).Commit()
	require.Eventually(t, func() bool { status, err := s.Inspect(); return err == nil && status.DemandGenerations == 0 }, time.Second, 5*time.Millisecond)
}

func TestAcceptedCommitFailureRetainsIntentUntilStorageRecovers(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	store, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	h, s, m, data := testHost(t, Config{Store: store})
	importBundle(t, s, m, data)
	startHost(t, h)
	waitReady(t, h)
	candidate := enable(t, h, 0, 1)
	owners, err := filepath.Glob(filepath.Join(path, "claims", "*", "owner.json"))
	require.NoError(t, err)
	require.Len(t, owners, 1)
	owner := owners[0]
	require.NoError(t, os.Rename(owner, owner+".saved"))
	require.NoError(t, os.Mkdir(owner, 0700))
	candidate.Commit()
	require.Eventually(t, func() bool { return h.Status().State == StateUnavailable }, time.Second, 5*time.Millisecond)
	require.Equal(t, 1, h.Status().Enabled)
	require.Error(t, h.Ready())
	require.NoError(t, os.Rename(owner, owner+".failed"))
	require.NoError(t, os.Rename(owner+".saved", owner))
	waitReady(t, h)
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Equal(t, 1, status.DemandGenerations)
}

func TestMaintenanceRetiresIdleRuntimeWithoutJobs(t *testing.T) {
	config := runtime.DefaultConfig(filepath.Join(t.TempDir(), "store"))
	config.IdleRetention = 500 * time.Millisecond
	config.PreviousRetention = 500 * time.Millisecond
	store, err := runtime.Open(config)
	require.NoError(t, err)
	h, s, m, data := testHost(t, Config{Store: store})
	h.sweepInterval = 20 * time.Millisecond
	importBundle(t, s, m, data)
	startHost(t, h)
	waitReady(t, h)
	require.Eventually(t, func() bool { return errors.Is(s.Available(m), runtime.ErrUnavailable) }, 3*time.Second, 5*time.Millisecond)
	require.Eventually(t, func() bool { return h.Status().State == StateMissing }, time.Second, 5*time.Millisecond)
	require.Zero(t, h.Status().Enabled)
}
