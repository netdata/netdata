// SPDX-License-Identifier: GPL-3.0-or-later

package s3check

import (
	"maps"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/s3check/internal/contract"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMetricRendererPreservesGapsAndCountsLogicalCalls(t *testing.T) {
	store := metrix.NewCollectorStore()
	renderer := newMetricRenderer(store)
	renderer.reset("site-a", "site-b")
	cc := mustCycleController(t, store)
	result := contract.Result{
		Mode: contract.ModeCephMultisite,
		Operations: []contract.OperationResult{
			{
				Name: contract.OperationRead, Endpoint: contract.EndpointDestination,
				Status: contract.StatusSuccess, Reason: contract.ReasonNone,
				Duration: 2 * time.Second,
			},
			{
				Name: contract.OperationRead, Endpoint: contract.EndpointDestination,
				Status: contract.StatusFailed, Reason: contract.ReasonRequest,
				Duration: time.Second,
			},
		},
		Cleanup: contract.CleanupResult{
			Pending:      2,
			Backpressure: true,
		},
		Probe: &contract.ProbeResult{
			Status: contract.StatusWaiting,
			Reason: contract.ReasonNone,
			WriteVisibility: contract.ObjectiveResult{
				Performed: true,
				Status:    contract.StatusSuccess,
				Lag:       3 * time.Second,
			},
		},
	}

	cc.BeginCycle()
	renderer.write(result)
	cc.CommitCycleSuccess()
	reader := store.Read(metrix.ReadRaw())
	common := metrix.Labels{
		"mode":        string(contract.ModeCephMultisite),
		"source":      "site-a",
		"destination": "site-b",
	}
	assertMetricValue(t, reader, "cleanup_pending_objects", common, 2)
	assertMetricValue(t, reader, "mutation_backpressure", common, 1)
	assertMetricValue(t, reader, "write_visibility_lag_seconds", withReason(common, contract.ReasonNone), 3)
	_, ok := reader.Value("delete_visibility_lag_seconds", withReason(common, contract.ReasonNone))
	assert.False(t, ok, "unperformed delete visibility must remain a gap")
	_, ok = reader.Value("payload_mismatch", withReason(common, contract.ReasonNone))
	assert.False(t, ok, "unperformed payload comparison must remain a gap")
	operationLabels := metrix.Labels{
		"mode":        string(contract.ModeCephMultisite),
		"source":      "site-a",
		"destination": "site-b",
		"endpoint":    string(contract.EndpointDestination),
		"operation":   string(contract.OperationRead),
	}
	assertMetricValue(t, reader, "operation_calls_total", operationLabels, 2)
	assertMetricValue(t, reader, "operation_failures_total", operationLabels, 1)
	failedStatusLabels := withReason(operationLabels, contract.ReasonRequest)
	status, ok := reader.StateSet("operation_status", failedStatusLabels)
	require.True(t, ok)
	assert.True(t, status.States[string(contract.StatusFailed)])
	assert.False(t, status.States[string(contract.StatusSuccess)])
	successStatusLabels := withReason(operationLabels, contract.ReasonNone)
	_, ok = reader.StateSet("operation_status", successStatusLabels)
	assert.False(t, ok, "one logical operation must not report success and failure in the same cycle")
}

func TestMetricRendererUsesLastTerminalDuringBackpressure(t *testing.T) {
	store := metrix.NewCollectorStore()
	renderer := newMetricRenderer(store)
	renderer.reset("source", "destination")
	cc := mustCycleController(t, store)
	result := contract.Result{
		Mode: contract.ModeAWSReplication,
		Cleanup: contract.CleanupResult{
			Pending:      8,
			Backpressure: true,
		},
		LastTerminal: &contract.ProbeResult{
			Status:          contract.StatusFailed,
			Reason:          contract.ReasonPayloadMismatch,
			PayloadCompared: true,
			PayloadMismatch: true,
		},
	}

	cc.BeginCycle()
	renderer.write(result)
	cc.CommitCycleSuccess()
	assertMetricValue(t, store.Read(metrix.ReadRaw()), "payload_mismatch", metrix.Labels{
		"mode":        string(contract.ModeAWSReplication),
		"source":      "source",
		"destination": "destination",
		"reason":      string(contract.ReasonPayloadMismatch),
	}, 1)
}

func TestMetricChartCoverage(t *testing.T) {
	c := New()
	c.metrics.reset("source", "destination")
	cc := mustCycleController(t, c.store)
	result := contract.Result{
		Mode: contract.ModeAWSReplication,
		Operations: []contract.OperationResult{{
			Name: contract.OperationRead, Endpoint: contract.EndpointDestination,
			Status: contract.StatusSuccess, Reason: contract.ReasonNone,
			Duration: time.Second,
		}},
		Probe: &contract.ProbeResult{
			Status:          contract.StatusSuccess,
			Reason:          contract.ReasonNone,
			PayloadCompared: true,
			WriteVisibility: contract.ObjectiveResult{
				Performed: true,
				Status:    contract.StatusSuccess,
				Lag:       time.Second,
			},
			DeleteVisibility: contract.ObjectiveResult{
				Performed: true,
				Status:    contract.StatusSuccess,
				Lag:       time.Second,
			},
		},
	}
	cc.BeginCycle()
	c.metrics.write(result)
	cc.CommitCycleSuccess()

	collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
}

func mustCycleController(t *testing.T, store metrix.CollectorStore) metrix.CycleController {
	t.Helper()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	return managed.CycleController()
}

func assertMetricValue(t *testing.T, reader metrix.Reader, name string, labels metrix.Labels, want float64) {
	t.Helper()
	got, ok := reader.Value(name, labels)
	require.Truef(t, ok, "metric %s labels=%v is missing", name, labels)
	assert.InDelta(t, want, got, 1e-9)
}

func withReason(labels metrix.Labels, reason contract.Reason) metrix.Labels {
	result := make(metrix.Labels, len(labels)+1)
	maps.Copy(result, labels)
	result["reason"] = string(reason)
	return result
}
