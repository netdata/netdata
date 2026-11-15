// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/browserruntime"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runner"
	"github.com/stretchr/testify/require"
)

type qualifyFunc func(context.Context, runner.Source) (*runner.Qualification, error)

func (f qualifyFunc) Qualify(ctx context.Context, source runner.Source) (*runner.Qualification, error) {
	return f(ctx, source)
}

func TestUpdaterPreparesOnceAndRequiresRestart(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, browserruntime.AutomaticUpdates)
	host := NewManaged(store)
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
	verifiedNext := verifiedRelease(t, next)
	load := func(context.Context) (browserruntime.VerifiedRelease, error) { return verifiedNext, nil }
	calls := 0
	u := host.NewUpdater(qualifyFunc(func(_ context.Context, source runner.Source) (*runner.Qualification, error) {
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
	status, err := store.Inspect()
	require.NoError(t, err)
	require.Equal(t, next.Manifest, status.Release.Manifest, "the next process runs the prepared release")
	use, err := host.BeginUse()
	require.NoError(t, err)
	require.Equal(t, "first", use.Paths().Identity.Version)
	require.NoError(t, use.Finish(true))
	require.NoError(t, u.check(context.Background()))
	require.Equal(t, 1, calls)
	require.Equal(t, int32(1), archiveRequests.Load())
	newHost := NewManaged(store)
	defer newHost.Close()
	require.Equal(t, next.Manifest.Identity(), *newHost.identity)
}

func TestUpdaterPreservesInstallationAfterQualificationFailure(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, browserruntime.AutomaticUpdates)
	host := NewManaged(store)
	defer host.Close()
	next, data := publishedBundle(t, "second", 2)
	server := httptest.NewTLSServer(
		http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write(data) }),
	)
	defer server.Close()
	previous := http.DefaultTransport
	http.DefaultTransport = server.Client().Transport
	defer func() { http.DefaultTransport = previous }()
	next.Manifest.Archive.URL = server.URL
	verifiedFirst, verifiedNext := verifiedRelease(t, first), verifiedRelease(t, next)
	failure := errors.New("sandbox qualification failed")
	var logs healthLog
	host.log = logger.NewWithWriter(&logs)
	u := host.NewUpdater(
		qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) { return nil, failure }),
		func(context.Context) (browserruntime.VerifiedRelease, error) { return verifiedNext, nil },
	)
	require.ErrorIs(t, u.check(context.Background()), failure)
	status, err := store.Inspect()
	require.NoError(t, err)
	require.Equal(t, first.Manifest, status.Release.Manifest)
	require.Zero(t, status.CandidateClaims)
	require.NoError(t, host.Ready())
	require.ErrorIs(t, u.check(context.Background()), failure)
	require.Equal(t, 1, strings.Count(logs.String(), "DEM runtime update failed: sandbox qualification failed"))
	host.sweep()
	require.NotContains(t, logs.String(), "update recovered", "storage cleanup cannot clear update health")
	// A later check that finds no newer release clears update health.
	u.load = func(context.Context) (browserruntime.VerifiedRelease, error) { return verifiedFirst, nil }
	require.NoError(t, u.check(context.Background()))
	require.Equal(t, 1, strings.Count(logs.String(), "DEM runtime update recovered"))
}

func TestUpdaterJoinsCancelledAcquisition(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, browserruntime.AutomaticUpdates)
	host := NewManaged(store)
	defer host.Close()
	entered := make(chan struct{})
	u := host.NewUpdater(qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) {
		t.Error("unexpected qualification")
		return nil, nil
	}), func(ctx context.Context) (browserruntime.VerifiedRelease, error) {
		close(entered)
		<-ctx.Done()
		return browserruntime.VerifiedRelease{}, ctx.Err()
	})
	u.delay = func(bool) time.Duration { return 0 }
	var logs healthLog
	u.log = logger.NewWithWriter(&logs)
	go u.Run(context.Background())
	<-entered
	require.NoError(t, u.Close())
	require.Empty(t, logs.String(), "a cancelled check records no update outcome")
	status, err := store.Inspect()
	require.NoError(t, err)
	require.Equal(t, first.Manifest, status.Release.Manifest)
	require.NoError(t, host.Close())
	require.NoError(t, store.Uninstall(context.Background()), "cancelled updater must release installation exclusion")
}

func TestHostCloseJoinsAttachedUpdater(t *testing.T) {
	store := testStore(t)
	first, data := publishedBundle(t, "first", 1)
	selectPublished(t, store, first, data, browserruntime.AutomaticUpdates)
	host := NewManaged(store)
	entered := make(chan struct{})
	u := host.NewUpdater(qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) {
		t.Error("unexpected qualification")
		return nil, nil
	}), func(ctx context.Context) (browserruntime.VerifiedRelease, error) {
		close(entered)
		<-ctx.Done()
		return browserruntime.VerifiedRelease{}, ctx.Err()
	})
	u.delay = func(bool) time.Duration { return 0 }
	go u.Run(context.Background())
	<-entered
	require.NoError(t, host.Close())
	select {
	case <-u.done:
	default:
		t.Fatal("host released protection before joining its updater")
	}
	require.NoError(t, store.Uninstall(context.Background()), "closed host and updater release the installation")
}

func TestUpdaterWithoutInstallationDoesNotLoadOrQualify(t *testing.T) {
	store := testStore(t)
	host := NewManaged(store)
	defer host.Close()
	u := host.NewUpdater(qualifyFunc(func(context.Context, runner.Source) (*runner.Qualification, error) {
		t.Error("unexpected qualification")
		return nil, nil
	}),
		func(context.Context) (browserruntime.VerifiedRelease, error) {
			t.Error("unexpected discovery")
			return browserruntime.VerifiedRelease{}, nil
		})
	require.NoError(t, u.check(context.Background()))
}
