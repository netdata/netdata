// SPDX-License-Identifier: GPL-3.0-or-later

package redfish

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/redfishruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
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
	collector := New(redfishruntime.New())
	collector.Logger = logger.NewWithWriter(&output)
	collector.Config = Config{
		URL:        "https://bmc.example.test",
		NodeMode:   "local",
		AuthMethod: "none",
		Logs:       LogsConfig{Enabled: new(false)},
	}
	collector.SetJobName("endpoint-a")
	collector.newClient = func(Config, *http.Client) (endpointClient, error) {
		return client, nil
	}
	require.NoError(t, collector.Init(context.Background()))
	t.Cleanup(func() { collector.Cleanup(context.Background()) })

	require.NoError(t, collector.Check(context.Background()))
	require.NoError(t, collector.Check(context.Background()))
	assert.Equal(t, 1, strings.Count(output.String(), "Redfish authentication method selected: basic"))
}

func TestCollectorLogsCollectionDiagnosticsOnceWithinFixedBound(t *testing.T) {
	var output bytes.Buffer
	collector := New(redfishruntime.New())
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

func TestCollectorReportsLogBackendRouteTransitionsWithoutSpam(t *testing.T) {
	var output bytes.Buffer
	runtime := redfishruntime.New()
	collector := New(runtime)
	collector.Logger = logger.NewWithWriter(&output)
	collector.Config.Logs.Enabled = new(true)

	var cycle cycleMetrics
	collector.applyLogRouteState(&cycle)
	require.Equal(t, "unavailable", cycle.LogsRoute)
	collector.applyLogRouteState(&cycle)
	require.Equal(t, 1, strings.Count(output.String(), "is unavailable or not ready"))

	registration, err := runtime.RegisterBackend(
		"default",
		"test-key",
		t.TempDir(),
		&recordingLogBackend{contains: make(map[string]bool)},
	)
	require.NoError(t, err)
	collector.applyLogRouteState(&cycle)
	require.Equal(t, "ready", cycle.LogsRoute)
	require.Equal(t, 1, strings.Count(output.String(), "is ready; log polling can resume"))

	require.NoError(t, registration.Close(context.Background()))
	collector.applyLogRouteState(&cycle)
	require.Equal(t, "unavailable", cycle.LogsRoute)
	require.Equal(t, 1, strings.Count(output.String(), "is unavailable or not ready"))
}

func TestCollectorRejectsOversizedJobNameBeforeClientConstruction(t *testing.T) {
	collector := New(redfishruntime.New())
	collector.Config = Config{
		URL:        "https://bmc.example.test",
		NodeMode:   "local",
		AuthMethod: "none",
		Logs:       LogsConfig{Enabled: new(false)},
	}
	collector.SetJobName(strings.Repeat("x", promotedLabelLimit+1))
	collector.newClient = func(Config, *http.Client) (endpointClient, error) {
		t.Fatal("oversized job name reached client construction")
		return nil, nil
	}

	err := collector.Init(context.Background())
	require.ErrorContains(t, err, "job name must not exceed 256 bytes")
}

func TestCollectorPublishesAndRemovesFunctionInventory(t *testing.T) {
	runtime := redfishruntime.New()
	client := &staticEndpointClient{result: collectionResult{
		ObservedAt: time.Now(),
		Complete:   true,
		Metrics: cycleMetrics{
			Status: "success", SelectedSystem: "present",
		},
		Inventory: []map[string]any{{
			"sort_key":      "01",
			"host_uri":      "/redfish/v1/Systems/1",
			"host_name":     "System One",
			"resource_kind": "fan",
			"resource_uri":  "/redfish/v1/Chassis/1/Fans/1",
			"name":          "Fan One",
			"health":        "ok",
		}},
	}}
	collector := New(runtime)
	collector.Config = Config{
		URL:        "https://bmc.example.test",
		NodeMode:   "local",
		AuthMethod: "none",
		Logs:       LogsConfig{Enabled: new(false)},
	}
	collector.SetJobName("endpoint-a")
	collector.newClient = func(Config, *http.Client) (endpointClient, error) {
		return client, nil
	}
	require.NoError(t, collector.Init(context.Background()))
	require.NoError(t, collector.Check(context.Background()))

	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	collecttest.AssertChartCoverage(t, collector, collecttest.ChartCoverageExpectation{})
	rows := collectRuntimeInventoryRows(
		t, runtime, "endpoint-a", "/redfish/v1/Systems/1", "fan",
	)
	require.Len(t, rows, 1)
	assert.Equal(t, "endpoint-a", rows[0]["endpoint_job"])
	assert.Equal(t, collector.endpointKey, rows[0]["endpoint_key"])

	handler, ok := redfishFunctionHandler(runtime)(nil).(funcapi.RawMethodHandler)
	require.True(t, ok)
	response := handler.HandleRaw(context.Background(), funcapi.RawMethodRequest{
		Method: "inventory",
		Payload: selectionPayloadForCollectorTest(
			"endpoint-a",
			"/redfish/v1/Systems/1",
			"fan",
		),
	})
	require.NotNil(t, response.RawResponse, response.Message)
	require.Equal(t, 200, response.RawResponse["status"])
	data, ok := response.RawResponse["data"].([]json.RawMessage)
	require.True(t, ok)
	require.Len(t, data, 1)

	collector.Cleanup(context.Background())
	collector.Cleanup(context.Background())
	assertRuntimeInventoryJobCount(t, runtime, 0)
	assert.True(t, client.closed)
}

func TestCollectorCollectionErrorCanAbortMetricCycle(t *testing.T) {
	sentinel := errors.New("endpoint unavailable")
	collector := New(redfishruntime.New())
	collector.jobName = "endpoint-a"
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
	collector := New(redfishruntime.New())
	collector.UpdateEvery = 12
	collector.jobName = "endpoint-a"
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
			Inventory: []map[string]any{{
				"host_uri":      "/redfish/v1/Systems/1",
				"resource_kind": "temperature",
			}},
		},
		err: context.DeadlineExceeded,
	}
	runtime := redfishruntime.New()
	collector := New(runtime)
	collector.jobName = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client

	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	assertRuntimeInventoryJobCount(t, runtime, 1)
}

func TestCollectorIdentityFailureRetainsPriorInventorySnapshot(t *testing.T) {
	runtime := redfishruntime.New()
	client := &staticEndpointClient{result: collectionResult{
		ObservedAt: time.Now(),
		Complete:   true,
		Metrics:    cycleMetrics{Status: "success"},
		Inventory: []map[string]any{{
			"host_uri":      "/redfish/v1/Systems/1",
			"resource_kind": "fan",
			"resource_key":  "known-good",
		}},
	}}
	collector := New(runtime)
	collector.jobName = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client
	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()

	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	rows := collectRuntimeInventoryRows(t, runtime, "endpoint-a", "/redfish/v1/Systems/1", "fan")
	require.Equal(t, "known-good", rows[0]["resource_key"])

	client.result = collectionResult{
		ObservedAt: time.Now(),
		Metrics:    cycleMetrics{Status: "partial"},
		Inventory: []map[string]any{{
			"host_uri":      "/redfish/v1/Systems/1",
			"resource_kind": "fan",
			"resource_key":  "ambiguous",
		}},
	}
	client.err = fmt.Errorf("%w: test collision", errIdentityIntegrity)
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())
	rows = collectRuntimeInventoryRows(t, runtime, "endpoint-a", "/redfish/v1/Systems/1", "fan")
	require.Equal(t, "known-good", rows[0]["resource_key"])
}

func TestCollectorUnavailableEndpointRetainsPriorInventorySnapshot(t *testing.T) {
	runtime := redfishruntime.New()
	client := &staticEndpointClient{result: collectionResult{
		ObservedAt: time.Now(),
		Complete:   true,
		Metrics:    cycleMetrics{Status: "success"},
		Inventory: []map[string]any{{
			"host_uri":      "/redfish/v1/Systems/1",
			"resource_kind": "fan",
			"resource_key":  "known-good",
		}},
	}}
	collector := New(runtime)
	collector.jobName = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client
	managed, ok := metrix.AsCycleManagedStore(collector.store)
	require.True(t, ok)
	cycle := managed.CycleController()

	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())

	client.result = collectionResult{
		ObservedAt: time.Now(),
		Metrics:    cycleMetrics{Status: "unavailable"},
	}
	client.err = errors.New("ServiceRoot unavailable")
	cycle.BeginCycle()
	require.NoError(t, collector.Collect(context.Background()))
	require.NoError(t, cycle.CommitCycleSuccess())

	rows := collectRuntimeInventoryRows(t, runtime, "endpoint-a", "/redfish/v1/Systems/1", "fan")
	require.Equal(t, "known-good", rows[0]["resource_key"])
}

func TestCollectorParentCancellationAbortsPartialResult(t *testing.T) {
	client := &staticEndpointClient{result: collectionResult{
		Metrics: cycleMetrics{Status: "partial"},
	}}
	collector := New(redfishruntime.New())
	collector.jobName = "endpoint-a"
	collector.endpointKey = "endpoint-key"
	collector.client = client

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	cancel()
	require.ErrorIs(t, collector.Collect(ctx), context.Canceled)
	assertRuntimeInventoryJobCount(t, collector.runtime, 0)
}

func collectRuntimeInventoryRows(
	t *testing.T,
	runtime *redfishruntime.Runtime,
	job, host, kind string,
) []map[string]any {
	t.Helper()
	var rows []map[string]any
	total, found := runtime.VisitInventorySlice(
		context.Background(),
		job,
		host,
		kind,
		func(row map[string]any) bool {
			rows = append(rows, row)
			return true
		},
	)
	require.True(t, found)
	require.Equal(t, total, len(rows))
	return rows
}

func assertRuntimeInventoryJobCount(t *testing.T, runtime *redfishruntime.Runtime, want int) {
	t.Helper()
	count := 0
	require.True(t, runtime.VisitInventoryCatalog(
		context.Background(),
		max(want, 1_000),
		func(string) bool {
			count++
			return true
		},
		func(string, string) bool { return true },
		func(string) bool { return true },
	))
	require.Equal(t, want, count)
}

func selectionPayloadForCollectorTest(job, host, kind string) []byte {
	return []byte(
		`{"selections":{"__job":["` + job +
			`"],"host":["` + host +
			`"],"resource_kind":["` + kind + `"]}}`,
	)
}
