// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"
)

func TestCollector_WriteInternalMetrics(t *testing.T) {
	coll := New()
	now := time.Unix(100, 0)
	coll.recordRefreshStats(refreshStats{
		hasDeviceCounts:   true,
		registeredDevices: 2,
		cachedDevices:     1,
		errors:            3,
		completedAt:       now.Add(-5 * time.Second),
		duration:          1500 * time.Millisecond,
	})

	managed, ok := metrix.AsCycleManagedStore(coll.MetricStore())
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	coll.writeInternalMetrics(now)
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	reader := coll.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, internalMetricPrefix+".devices_registered", 2)
	requireMetricValue(t, reader, internalMetricPrefix+".devices_cached", 1)
	requireMetricValue(t, reader, internalMetricPrefix+".last_refresh_age_seconds", 5)
	requireMetricValue(t, reader, internalMetricPrefix+".last_refresh_duration_seconds", 1.5)
	requireMetricValue(t, reader, internalMetricPrefix+".refresh_runs_total", 1)
	requireMetricValue(t, reader, internalMetricPrefix+".refresh_errors_total", 3)
}

func TestCollectorCollectWritesInternalMetrics(t *testing.T) {
	coll := New()
	coll.registeredDevices = func() []ddsnmp.DeviceConnectionInfo {
		t.Fatal("Collect must not poll SNMP devices")
		return nil
	}
	coll.recordRefreshStats(refreshStats{
		hasDeviceCounts:   true,
		registeredDevices: 1,
		cachedDevices:     1,
		completedAt:       time.Now(),
	})

	managed, ok := metrix.AsCycleManagedStore(coll.MetricStore())
	require.True(t, ok)
	managed.CycleController().BeginCycle()
	require.NoError(t, coll.Collect(context.Background()))
	require.NoError(t, managed.CycleController().CommitCycleSuccess())

	reader := coll.MetricStore().Read(metrix.ReadRaw())
	requireMetricValue(t, reader, internalMetricPrefix+".devices_registered", 1)
	requireMetricValue(t, reader, internalMetricPrefix+".devices_cached", 1)
	requireMetricValue(t, reader, internalMetricPrefix+".refresh_runs_total", 1)
	collecttest.AssertChartCoverage(t, coll, collecttest.ChartCoverageExpectation{
		RequiredContexts: map[string][]string{
			"netdata.go.plugin.collector.snmp_topology.devices":      {"cached", "registered"},
			"netdata.go.plugin.collector.snmp_topology.last_refresh": {"age", "duration"},
			"netdata.go.plugin.collector.snmp_topology.refreshes":    {"errors", "runs"},
		},
	})
}

func requireMetricValue(t *testing.T, reader metrix.Reader, name string, want metrix.SampleValue) {
	t.Helper()
	got, ok := reader.Value(name, nil)
	require.True(t, ok, "metric %s not found", name)
	require.Equal(t, want, got)
}
