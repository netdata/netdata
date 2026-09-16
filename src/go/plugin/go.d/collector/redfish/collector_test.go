// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

type staticEndpointClient struct {
	result      collectionResult
	err         error
	closed      bool
	deadline    time.Time
	hasDeadline bool
	auth        string
}

func (c *staticEndpointClient) Check(context.Context) error { return c.err }
func (c *staticEndpointClient) Collect(ctx context.Context) (collectionResult, error) {
	c.deadline, c.hasDeadline = ctx.Deadline()
	return c.result, c.err
}
func (c *staticEndpointClient) Close(context.Context) error {
	c.closed = true
	return nil
}
func (c *staticEndpointClient) selectedAuthenticationMethod() string { return c.auth }

func TestCollectorLogsSelectedAuthenticationMethodOnce(t *testing.T) {
	var output bytes.Buffer
	client := &staticEndpointClient{auth: "basic"}
	collector := New()
	collector.Logger = logger.NewWithWriter(&output)
	collector.Config = Config{
		URL:        "https://bmc.example.test",
		AuthMethod: "none",
	}
	collector.Name = "endpoint-a"
	collector.newClient = func(Config, *http.Client) (endpointClient, error) {
		return client, nil
	}
	require.NoError(t, collector.Init(context.Background()))
	t.Cleanup(func() { collector.Cleanup(context.Background()) })

	require.NoError(t, collector.Check(context.Background()))
	require.NotContains(t, output.String(), "Redfish authentication method selected")
	managed, ok := metrix.AsCycleManagedStore(collector.MetricStore())
	require.True(t, ok)
	for range 2 {
		managed.CycleController().BeginCycle()
		require.NoError(t, collector.Collect(context.Background()))
		require.NoError(t, managed.CycleController().CommitCycleSuccess())
	}
	assert.Equal(t, 1, strings.Count(output.String(), "Redfish authentication method selected: basic"))
}

func TestCollectorLogsCollectionDiagnosticsOnceWithinFixedBound(t *testing.T) {
	var output bytes.Buffer
	collector := New()
	collector.Logger = logger.NewWithWriter(&output)

	collector.warnCollectionDiagnostics([]string{"first diagnostic", "first diagnostic", ""})
	collector.warnCollectionDiagnostics([]string{"first diagnostic", "second diagnostic"})
	assert.Equal(t, 1, strings.Count(output.String(), "first diagnostic"))
	assert.Equal(t, 1, strings.Count(output.String(), "second diagnostic"))

	for index := len(collector.warnedDiagnostics); index <= maxLoggedDiagnostics; index++ {
		collector.warnCollectionDiagnostics([]string{fmt.Sprintf("diagnostic-%d", index)})
	}
	collector.warnCollectionDiagnostics([]string{"another-overflow"})
	assert.Len(t, collector.warnedDiagnostics, maxLoggedDiagnostics)
	assert.Equal(t, 1, strings.Count(output.String(), "additional distinct diagnostics are suppressed"))
}

func TestCollectorRejectsOversizedJobNameBeforeClientConstruction(t *testing.T) {
	collector := New()
	collector.Config = Config{
		URL:        "https://bmc.example.test",
		AuthMethod: "none",
	}
	collector.Name = strings.Repeat("x", promotedLabelLimit+1)
	collector.newClient = func(Config, *http.Client) (endpointClient, error) {
		t.Fatal("oversized job name reached client construction")
		return nil, nil
	}

	err := collector.Init(context.Background())
	require.ErrorContains(t, err, "job name must not exceed 256 bytes")
}

func TestCollectorCollectionErrorCanAbortMetricCycle(t *testing.T) {
	sentinel := errors.New("endpoint unavailable")
	collector := New()
	collector.Name = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = &staticEndpointClient{err: sentinel}

	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	err := collector.Collect(context.Background())
	require.ErrorIs(t, err, sentinel)
	cycle.AbortCycle()
}

func TestCollectorDerivesCollectionDeadlineFromUpdateEvery(t *testing.T) {
	client := &staticEndpointClient{result: collectionResult{
		Metrics: cycleMetrics{Status: "success"},
	}}
	collector := New()
	collector.UpdateEvery = 12
	collector.Name = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client

	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	started := time.Now()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())

	require.True(t, client.hasDeadline)
	assert.WithinDuration(t, started.Add(12*time.Second), client.deadline, time.Second)
}

func TestCollectorPublishesPartialResultAtCycleDeadline(t *testing.T) {
	client := &staticEndpointClient{
		result: collectionResult{
			ObservedAt: time.Now(),
			Metrics:    cycleMetrics{Status: "partial"},
		},
		err: context.DeadlineExceeded,
	}
	collector := New()
	collector.Name = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client

	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	point, ok := collector.store.Read().StateSet("collection_status", metrix.Labels{"endpoint_key": "endpoint-key", "endpoint_job": "endpoint-a"})
	require.True(t, ok)
	assert.True(t, point.States["partial"])
}

func TestCollectorParentCancellationAbortsPartialResult(t *testing.T) {
	client := &staticEndpointClient{result: collectionResult{
		Metrics: cycleMetrics{Status: "partial"},
	}}
	collector := New()
	collector.Name = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	require.ErrorIs(t, collector.Collect(ctx), context.Canceled)
}
