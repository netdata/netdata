// SPDX-License-Identifier: GPL-3.0-or-later

package runtimehost

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gofrs/flock"
	runtime "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestPrepareContentionDoesNotBlockReadiness(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	s, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	h, _, _, _ := testHost(t, Config{Store: s})
	lock := flock.New(filepath.Join(path, "coordination.lock"))
	require.NoError(t, lock.Lock())
	defer lock.Close()
	type result struct {
		intent collectorapi.PreparedJobConfigIntent
		err    error
	}
	prepared := make(chan result, 1)
	go func() {
		intent, err := h.Prepare(collectorapi.JobConfigIdentity{}, collectorapi.JobConfigIdentity{1}, nil)
		prepared <- result{intent, err}
	}()
	// Reservation must remain pending while another process holds coordination.
	select {
	case <-prepared:
		t.Fatal("reservation bypassed coordination")
	case <-time.After(50 * time.Millisecond):
	}
	ready := make(chan error, 1)
	go func() { ready <- h.Ready() }()
	responsive := false
	select {
	case <-ready:
		responsive = true
	case <-time.After(500 * time.Millisecond):
	}
	require.NoError(t, lock.Close())
	got := <-prepared
	require.NoError(t, got.err)
	require.NoError(t, got.intent.Abort())
	require.True(t, responsive, "disk contention must not hold the host state mutex")
}

func TestAbortQueuesCleanupWithoutCoordination(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	s, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	h, _, _, _ := testHost(t, Config{Store: s})
	candidate := enable(t, h, 0, 1)
	lock := flock.New(filepath.Join(path, "coordination.lock"))
	require.NoError(t, lock.Lock())
	defer lock.Close()
	aborted := make(chan error, 1)
	go func() { aborted <- candidate.Abort() }()
	responsive := false
	select {
	case err := <-aborted:
		require.NoError(t, err)
		responsive = true
	case <-time.After(500 * time.Millisecond):
	}
	require.NoError(t, lock.Close())
	if !responsive {
		require.NoError(t, <-aborted)
	}
	require.True(t, responsive, "Abort must transfer cleanup to its process owner")
	// No worker was started: final Close owns queued cleanup after transitions join.
	require.NoError(t, h.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.PendingClaims)
}

func TestQueuedAbortReportsPhysicalCleanupFailure(t *testing.T) {
	path := filepath.Join(t.TempDir(), "store")
	s, err := runtime.Open(runtime.DefaultConfig(path))
	require.NoError(t, err)
	h, _, _, _ := testHost(t, Config{Store: s})
	candidate := enable(t, h, 0, 1)
	coordination := filepath.Join(path, "coordination.lock")
	require.NoError(t, os.Rename(coordination, coordination+".saved"))
	require.NoError(t, os.Mkdir(coordination, 0700))
	require.NoError(t, candidate.Abort(), "logical abort transfers release ownership")
	require.Zero(t, h.reconcileClaims())
	h.mu.Lock()
	maintenanceErr := h.maintenanceErrorLocked()
	h.mu.Unlock()
	require.Error(t, maintenanceErr)
	require.NoError(t, os.Rename(coordination, coordination+".blocked"))
	require.NoError(t, os.Rename(coordination+".saved", coordination))
	require.NoError(t, s.Sweep())
	require.Zero(t, h.reconcileClaims())
	status := h.Status()
	require.Empty(t, status.MaintenanceError)
	require.Empty(t, status.Storage.CleanupError)
	require.Zero(t, status.Storage.PendingClaims)
}

func TestLateAbortAfterWorkerJoinReleasedByClose(t *testing.T) {
	h, s, _, _ := testHost(t, Config{})
	candidate := enable(t, h, 0, 1)
	ctx, cancel := context.WithCancel(context.Background())
	go h.Run(ctx)
	require.Eventually(t, func() bool { h.mu.Lock(); defer h.mu.Unlock(); return h.started }, time.Second, time.Millisecond)
	cancel()
	<-h.done
	require.NoError(t, candidate.Abort())
	require.NoError(t, h.Close())
	status, err := s.Inspect()
	require.NoError(t, err)
	require.Zero(t, status.PendingClaims)
}
