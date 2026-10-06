// SPDX-License-Identifier: GPL-3.0-or-later
package otelcolpoc

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Run with the built distribution and the real nd-run helper (see POC Makefile).
func TestWorkerRuntime(t *testing.T) {
	worker := os.Getenv("OTEL_POC_WORKER")
	if worker == "" {
		t.Skip("set OTEL_POC_WORKER to run the real Collector lifecycle")
	}
	c := configured(t, "hostmetrics")
	c.worker = worker
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	ctx, cancel := context.WithCancel(context.Background())
	ready, done := make(chan struct{}), make(chan error, 1)
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	t.Cleanup(cancel)
	select {
	case <-ready:
	case err := <-done:
		t.Fatalf("startup failed: %v", err)
	case <-time.After(15 * time.Second):
		t.Fatal("startup timed out")
	}
	values, err := collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.NoError(t, err)
	assert.GreaterOrEqual(t, values["worker_uptime"], float64(0))
	collecttest.AssertChartCoverage(
		t,
		c,
		collecttest.ChartCoverageExpectation{
			RequiredContexts: map[string][]string{"otel_orchestrator.worker_uptime": {"uptime"}},
		},
	)
	cancel()
	select {
	case err := <-done:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(7 * time.Second):
		t.Fatal("worker shutdown timed out")
	}
	_, err = collecttest.CollectScalarSeries(c, metrix.ReadRaw())
	require.Error(t, err, "stopped worker cannot publish stale uptime")
	c.Cleanup(context.Background())
	// A startup failure must not signal readiness or retain a running process.
	c.worker = worker + "-missing"
	require.Error(t, c.Check(context.Background()))
	called := false
	require.Error(t, c.Run(context.Background(), func() { called = true }))
	assert.False(t, called)
}

func TestFilelogsPreflightWithoutState(t *testing.T) {
	worker := os.Getenv("OTEL_POC_WORKER")
	if worker == "" {
		t.Skip("set OTEL_POC_WORKER to test real distribution validation")
	}
	c := configured(t, "filelogs")
	c.worker = worker
	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))
	_, err := os.Stat(c.storageDir())
	require.True(t, os.IsNotExist(err), "Check must not acquire runtime storage")
}
