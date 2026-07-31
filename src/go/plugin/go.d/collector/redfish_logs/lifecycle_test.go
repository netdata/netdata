// SPDX-License-Identifier: GPL-3.0-or-later

package redfish_logs

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const asyncTestTimeout = 10 * time.Second

func TestCollectorLifecycle(t *testing.T) {
	logDir := t.TempDir()
	t.Setenv("NETDATA_LOG_DIR", logDir)
	runtime := redfishruntime.New()
	collector := New(runtime)
	collector.SetJobName("default")

	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- collector.Run(ctx)
	}()
	require.Eventually(t, collector.ready.Load, asyncTestTimeout, 10*time.Millisecond)
	assert.True(t, runtime.BackendAvailable("default"))

	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	collecttest.AssertChartCoverage(t, collector, collecttest.ChartCoverageExpectation{})

	cancel()
	select {
	case err := <-done:
		require.NoError(t, err)
	case <-time.After(asyncTestTimeout):
		t.Fatal("collector run did not finish after cancellation")
	}
	collector.Cleanup(context.Background())
	collector.Cleanup(context.Background())
	assert.False(t, runtime.BackendAvailable("default"))
	assert.False(t, collector.ready.Load())
}

func TestCollectorCloseFinishesAfterTimedOutBackendDrain(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "backend")
	require.NoError(t, ensurePrivateDirectory(dir))
	locker, err := acquireBackendLock(dir)
	require.NoError(t, err)
	backend, err := newJournalBackend(dir, 20<<20, fixedJournalHost{})
	require.NoError(t, err)

	runtime := redfishruntime.New()
	key, _ := backendDigest("default")
	registration, err := runtime.RegisterBackend("default", key, dir, backend)
	require.NoError(t, err)
	lease, ok := runtime.AcquireBackend("default")
	require.True(t, ok)

	collector := New(runtime)
	collector.jobName = "default"
	collector.key = key
	collector.dir = dir
	collector.backend = backend
	collector.registration = registration
	collector.locker = locker
	collector.ready.Store(true)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	err = collector.close(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.False(t, runtime.BackendAvailable("default"))
	assert.False(t, collector.ready.Load())

	lease.Release()

	require.Eventually(t, func() bool {
		backend.mu.Lock()
		closed := backend.log == nil
		backend.mu.Unlock()
		if !closed {
			return false
		}
		reopened, err := acquireBackendLock(dir)
		if err != nil {
			return false
		}
		reopened.UnlockAll()
		return true
	}, asyncTestTimeout, 10*time.Millisecond)
}
