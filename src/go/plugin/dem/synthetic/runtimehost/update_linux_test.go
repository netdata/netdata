// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"crypto/ed25519"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/stretchr/testify/require"
)

type qualifyFunc func(context.Context, runner.Source) (*runner.Qualification, error)

func (f qualifyFunc) Qualify(ctx context.Context, source runner.Source) (*runner.Qualification, error) {
	return f(ctx, source)
}

func publishedBundle(t *testing.T, version string, sequence uint64) (runtime.Release, []byte) {
	t.Helper()
	m, data := testBundle(t, version)
	m.Archive.URL = "https://artifacts.example.invalid/runtime.tar.zst"
	return runtime.Release{Format: 1, Sequence: sequence, Manifest: m,
		Sources: runtime.ReleaseSource{URL: "https://artifacts.example.invalid/sources.tar.zst", SHA256: strings.Repeat("a", 64), Bytes: 100}}, data
}

func selectPublished(t *testing.T, store *runtime.Store, release runtime.Release, data []byte, policy runtime.UpdatePolicy) {
	t.Helper()
	// Non-executable bytes test acquisition/ownership; this does not qualify a browser.
	path := filepath.Join(t.TempDir(), "archive.tar.zst")
	require.NoError(t, os.WriteFile(path, data, 0600))
	candidate, err := store.BeginPublishedInstall(context.Background(), runtime.ReleaseInstallOptions{Operation: runtime.InstallRelease, Updates: policy}, func(context.Context) (runtime.Release, error) { return release, nil })
	require.NoError(t, err)
	require.NoError(t, candidate.Import(context.Background(), path))
	require.NoError(t, candidate.Select())
	require.NoError(t, candidate.Close())
}

func TestUpdaterPreparesOnceAndRequiresRestart(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, runtime.AutomaticUpdates)
	host := New(Config{Store: store})
	defer host.Close()
	next, data := publishedBundle(t, "second", 2)
	var archiveRequests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		archiveRequests.Add(1)
		_, _ = w.Write(data)
	}))
	defer server.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = previous }()
	next.Manifest.Archive.URL = server.URL
	public, private, err := ed25519.GenerateKey(nil)
	require.NoError(t, err)
	envelope, err := runtime.SignRelease(next, private)
	require.NoError(t, err)
	load := func(context.Context) (runtime.Release, error) {
		return runtime.VerifyRelease(envelope, []ed25519.PublicKey{public})
	}
	calls := 0
	u := NewUpdater(host, qualifyFunc(func(_ context.Context, source runner.Source) (*runner.Qualification, error) {
		calls++
		status, err := store.Inspect()
		require.NoError(t, err)
		require.Equal(t, 1, status.CandidateClaims, "candidate protects gaps between qualification calls")
		use, err := source.BeginUse()
		require.NoError(t, err)
		require.Equal(t, "second", use.Paths().Identity.Version)
		require.NoError(t, use.Finish(true))
		return &runner.Qualification{}, nil
	}), load)
	require.NoError(t, u.check(context.Background()))
	status := host.Status()
	require.Equal(t, "first", status.Running.Version)
	require.Equal(t, "second", status.Storage.Selected.Version)
	require.True(t, status.RestartRequired)
	require.Positive(t, status.Update.LastSuccessUS)
	require.False(t, status.Update.Checking)
	use, err := host.BeginUse()
	require.NoError(t, err)
	require.Equal(t, "first", use.Paths().Identity.Version)
	require.NoError(t, use.Finish(true))
	require.NoError(t, u.check(context.Background()))
	require.Equal(t, 1, calls)
	require.Equal(t, int32(1), archiveRequests.Load())
	newHost := New(Config{Store: store})
	defer newHost.Close()
	require.Equal(t, "second", newHost.Status().Running.Version)
	require.False(t, newHost.Status().RestartRequired)
}

func TestUpdaterPreservesInstallationAfterQualificationFailure(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, runtime.AutomaticUpdates)
	host := New(Config{Store: store})
	defer host.Close()
	next, data := publishedBundle(t, "second", 2)
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }))
	defer server.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = previous }()
	next.Manifest.Archive.URL = server.URL
	failure := errors.New("sandbox qualification failed")
	u := NewUpdater(host, qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) { return nil, failure }),
		func(context.Context) (runtime.Release, error) { return next, nil })
	require.ErrorIs(t, u.check(context.Background()), failure)
	status := host.Status()
	require.Equal(t, first.Manifest, *status.Storage.Selected)
	require.False(t, status.RestartRequired)
	require.Contains(t, status.MaintenanceError, failure.Error())
	require.Zero(t, status.Storage.CandidateClaims)
	require.NoError(t, host.Ready())
	host.sweep()
	require.Contains(t, host.Status().MaintenanceError, failure.Error(), "storage cleanup cannot clear update health")
}

func TestUpdaterJoinsCancelledAcquisition(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, runtime.AutomaticUpdates)
	host := New(Config{Store: store})
	defer host.Close()
	entered := make(chan struct{})
	u := NewUpdater(host, qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) {
		t.Error("unexpected qualification")
		return nil, nil
	}), func(ctx context.Context) (runtime.Release, error) {
		close(entered)
		<-ctx.Done()
		return runtime.Release{}, ctx.Err()
	})
	u.delay = func(bool) time.Duration { return 0 }
	go u.Run(context.Background())
	<-entered
	require.True(t, host.Status().Update.Checking)
	require.NoError(t, u.Close())
	status := host.Status()
	require.False(t, status.Update.Checking)
	require.Empty(t, status.MaintenanceError)
	require.Equal(t, first.Manifest, *status.Storage.Selected)
	require.NoError(t, host.Close())
	require.NoError(t, store.Uninstall(context.Background()), "cancelled updater must release installation exclusion")
}

func TestUpdaterWithoutInstallationDoesNotLoadOrQualify(t *testing.T) {
	store := testStore(t)
	host := New(Config{Store: store})
	defer host.Close()
	u := NewUpdater(host, qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) {
		t.Error("unexpected qualification")
		return nil, nil
	}),
		func(context.Context) (runtime.Release, error) {
			t.Error("unexpected discovery")
			return runtime.Release{}, nil
		})
	require.NoError(t, u.check(context.Background()))
	require.Zero(t, host.Status().Update.LastCheckUS)
}
