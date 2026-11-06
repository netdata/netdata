// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"bytes"
	"context"
	"maps"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestCollector_TableChartLabelLifecycle(t *testing.T) {
	for name, multiValue := range map[string]map[string]int64{
		"single dimension":    nil,
		"multiple dimensions": {"allocated": 20, "consumption": 7},
	} {
		t.Run(name, func(t *testing.T) {
			c := newTestSNMPCollector()
			c.Hostname = "192.0.2.1"
			c.sysInfo = &snmputils.SysInfo{
				Name: "switch",
			}
			m := ddsnmp.Metric{
				Name:       "port_power",
				IsTable:    true,
				Value:      7,
				MultiValue: multiValue,
				Profile: &ddsnmp.ProfileMetrics{
					Tags: map[string]string{"profile": "catalyst"},
				},
			}
			var original *collectorapi.Chart
			var dimensions collectorapi.Dims
			var firstSamples map[string]int64
			for _, step := range []struct {
				name   string
				labels map[string]string
				redraw bool
			}{
				{name: "unresolved", redraw: true},
				{name: "resolved", labels: map[string]string{"_interface": "Gi1/0/1", "_if_alias": "endpoint-a"}, redraw: true},
				{name: "unchanged", labels: map[string]string{"_interface": "Gi1/0/1", "_if_alias": "endpoint-a"}},
				{name: "changed", labels: map[string]string{"_interface": "Gi1/0/2", "_if_alias": "endpoint-b"}, redraw: true},
				{name: "unresolved again", redraw: true},
				{name: "recovered", labels: map[string]string{"_interface": "Gi1/0/1", "_if_alias": "endpoint-a"}, redraw: true},
			} {
				t.Run(step.name, func(t *testing.T) {
					m.Tags = map[string]string{"pse_group_index": "1", "pse_port_index": "1"}
					maps.Copy(m.Tags, step.labels)
					samples := make(map[string]int64)
					c.collectProfileTableMetrics(samples, []ddsnmp.Metric{m})
					require.Len(t, *c.Charts(), 1)
					chart := (*c.Charts())[0]
					if original == nil {
						original, dimensions, firstSamples = chart, chart.Dims, samples
					}
					assert.Same(t, original, chart)
					assert.Equal(t, "snmp_device_prof_port_power_44676bb68dcbc62d6e2a3dc700bb8fdac433f8477fe44cea0c87448c2d671e80", chart.ID)
					assert.Equal(t, firstSamples, samples)
					assert.Equal(t, dimensions, chart.Dims)
					for i := range dimensions {
						assert.Same(t, dimensions[i], chart.Dims[i])
					}
					want := map[string]string{
						"address":         "192.0.2.1",
						"sysName":         "switch",
						"profile":         "catalyst",
						"pse_group_index": "1",
						"pse_port_index":  "1",
					}
					for key, value := range step.labels {
						want[strings.TrimPrefix(key, "_")] = value
					}
					assert.Equal(t, want, chartLabels(chart))
					assert.Equal(t, step.redraw, !chart.IsCreated())
					chart.SetCreated(true) // Acknowledgment supplied by the renderer after successful output.
				})
			}
		})
	}
}

func TestCollector_TableChartLabelPrecedence(t *testing.T) {
	c := newTestSNMPCollector()
	c.Hostname = "192.0.2.1"
	c.sysInfo = &snmputils.SysInfo{
		Name: "switch",
	}
	m := ddsnmp.Metric{
		Name:    "power",
		IsTable: true,
		Value:   7,
		Profile: &ddsnmp.ProfileMetrics{
			Tags: map[string]string{"sysName": "profile-switch", "interface": "", "profile": "catalyst"},
		},
		Tags: map[string]string{
			"port":       "1",
			"interface":  "Gi1/0/1",
			"_interface": "ignored",
			"_sysName":   "ignored",
			"_address":   "ignored",
			"_profile":   "ignored",
			"_":          "ignored",
			"_empty_a":   "",
		},
	}
	want := map[string]string{
		"sysName":   "profile-switch",
		"address":   "192.0.2.1",
		"profile":   "catalyst",
		"port":      "1",
		"interface": "Gi1/0/1",
		"empty_a":   "",
	}
	c.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{m})
	chart := (*c.Charts())[0]
	assert.Equal(t, want, chartLabels(chart))
	chart.SetCreated(true)
	m.Tags["_interface"] = "still ignored"
	c.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{m})
	assert.True(t, chart.IsCreated(), "shadowed tag changes must not redraw the chart")

	// Equal label counts and empty values must still detect a replaced key.
	delete(m.Tags, "_empty_a")
	m.Tags["_empty_b"] = ""
	delete(want, "empty_a")
	want["empty_b"] = ""
	c.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{m})
	assert.False(t, chart.IsCreated())
	assert.Equal(t, want, chartLabels(chart))
	chart.SetCreated(true)

	// Removing a profile override restores the base label even with unchanged row tags.
	delete(m.Profile.Tags, "sysName")
	want["sysName"] = "switch"
	c.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{m})
	assert.False(t, chart.IsCreated())
	assert.Equal(t, want, chartLabels(chart))
}

func TestCollector_TableChartLabelsSharedKey(t *testing.T) {
	c := newTestSNMPCollector()
	c.sysInfo = &snmputils.SysInfo{
		Name: "switch",
	}
	metrics := []ddsnmp.Metric{
		{
			Name:    "power",
			IsTable: true,
			Value:   3,
			Profile: &ddsnmp.ProfileMetrics{},
			Tags:    map[string]string{"group": "1", "_if_alias": "first"},
		},
		{
			Name:    "power",
			IsTable: true,
			Value:   4,
			Profile: &ddsnmp.ProfileMetrics{},
			Tags:    map[string]string{"group": "1", "_if_alias": "second"},
		},
	}
	for _, step := range []struct {
		alias  string
		redraw bool
	}{
		{alias: "first", redraw: true},
		{alias: "first"},
		{alias: "changed", redraw: true},
		{alias: "changed"},
	} {
		metrics[0].Tags["_if_alias"] = step.alias
		samples := make(map[string]int64)
		c.collectProfileTableMetrics(samples, metrics)
		require.Len(t, *c.Charts(), 1)
		chart := (*c.Charts())[0]
		assert.Equal(t, step.alias, chartLabels(chart)["if_alias"], "the first sample for a key owns its chart labels")
		assert.Equal(t, map[string]int64{"snmp_device_prof_power_22b8062f5aedcbe6e8a8779a53bb4497336830dfd90437a83e97ac628e8eab51": 7}, samples)
		assert.Equal(t, step.redraw, !chart.IsCreated())
		chart.SetCreated(true)
	}
}

type chartLabelOutput struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *chartLabelOutput) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *chartLabelOutput) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

// Use the real managed renderer to verify replacement labels and retirement on
// the wire. The collector input stands in for the preceding SNMP acquisition.
func TestCollector_TableChartLabelOutput(t *testing.T) {
	c := newTestSNMPCollector()
	c.Hostname = "192.0.2.1"
	c.sysInfo = &snmputils.SysInfo{
		Name: "switch",
	}
	var mu sync.Mutex
	var metrics []ddsnmp.Metric
	module := &collectorapi.MockCollectorV1{
		ChartsFunc: c.Charts,
		CollectFunc: func(context.Context) map[string]int64 {
			mu.Lock()
			defer mu.Unlock()
			samples := make(map[string]int64)
			c.collectProfileTableMetrics(samples, metrics)
			return samples
		},
	}
	out := &chartLabelOutput{}
	job := jobruntime.NewJob(jobruntime.JobConfig{
		PluginName: "go.d",
		Name:       "fixture",
		ModuleName: "snmp",
		FullName:   "snmp_fixture",
		Module:     module,
		Out:        out,
	})
	require.NoError(t, job.AutoDetectionManaged(t.Context()))
	run := jobruntime.NewManagedRun(t.Context(), nil)
	done := make(chan struct{})
	go func() { defer close(done); job.StartManaged(run) }()
	t.Cleanup(func() {
		run.Stop(nil)
		assert.True(t, waitForChartJobSignal(done), "managed job did not stop")
	})
	require.True(t, waitForChartJobSignal(run.StartupDone()), "managed startup blocked")
	require.NoError(t, run.StartupErr())

	for _, step := range []struct {
		name, alias    string
		redraw, absent bool
	}{
		{name: "unresolved", redraw: true},
		{name: "resolved", alias: "endpoint-a", redraw: true},
		{name: "unchanged", alias: "endpoint-a"},
		{name: "changed", alias: "endpoint-b", redraw: true},
		{name: "unresolved again", redraw: true},
		{name: "recovered", alias: "endpoint-a", redraw: true},
		{name: "row removed", absent: true, redraw: true},
		{name: "row returned", alias: "endpoint-b", redraw: true},
	} {
		t.Run(step.name, func(t *testing.T) {
			before := out.String()
			mu.Lock()
			metrics = nil
			if !step.absent {
				tags := map[string]string{"port": "1"}
				if step.alias != "" {
					tags["_if_alias"] = step.alias
				}
				metrics = []ddsnmp.Metric{
					{Name: "power", IsTable: true, Value: 7, Profile: &ddsnmp.ProfileMetrics{}, Tags: tags},
				}
			}
			mu.Unlock()
			status := "_data_collection_status'"
			count := strings.Count(before, status)
			require.Eventually(t, func() bool {
				if strings.Count(out.String(), status) > count {
					return true
				}
				job.Tick(0)
				return false
			}, 5*time.Second, 10*time.Millisecond)
			wire := strings.TrimPrefix(out.String(), before)
			chart := "CHART 'snmp_fixture.snmp_device_prof_power_9fb28e1383b7b124087181687e55702c9adc56e5e6d16900e94b2a763a5bc86a'"
			assert.Equal(t, step.redraw, strings.Contains(wire, chart), wire)
			if step.absent {
				assert.Contains(t, wire, "obsolete")
				assert.NotContains(t, wire, "BEGIN 'snmp_fixture.snmp_device_prof_power_9fb28e1383b7b124087181687e55702c9adc56e5e6d16900e94b2a763a5bc86a'")
				return
			}
			assert.Contains(t, wire, "BEGIN 'snmp_fixture.snmp_device_prof_power_9fb28e1383b7b124087181687e55702c9adc56e5e6d16900e94b2a763a5bc86a'")
			assert.Contains(t, wire, " = 7\n")
			if step.redraw {
				assert.Contains(t, wire, "CLABEL_COMMIT\n")
				if step.alias == "" {
					assert.NotContains(t, wire, "CLABEL 'if_alias'")
				} else {
					assert.Contains(t, wire, "CLABEL 'if_alias' '"+step.alias+"' '1'\n")
				}
			}
		})
	}
}

func waitForChartJobSignal(signal <-chan struct{}) bool {
	select {
	case <-signal:
		return true
	case <-time.After(5 * time.Second):
		return false
	}
}
