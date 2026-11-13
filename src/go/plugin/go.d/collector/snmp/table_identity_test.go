// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

func TestStockFirewallIdentitySurvivesChartSerialization(t *testing.T) {
	cases := map[string][][2]string{
		"component_boundary":  {{"c", "a_2_b"}, {"b_2_c", "a"}},
		"chart_normalization": {{"filter", "a.b"}, {"filter", "a_b"}},
		"spaces":              {{"filter", "a b"}, {"filter", "a_b"}},
		"empty_components":    {{"", "x"}, {"x", ""}},
		"apostrophes":         {{"filter", "a'b"}, {"filter", "ab"}},
		"line_breaks":         {{"filter", "a\r\nb"}, {"filter", "a  b"}},
		"nul":                 {{"filter", "a\x00b"}, {"filter", "ab"}},
		"backslash":           {{"filter", "a\\"}, {"filter", "a/"}},
		"long_components": {
			{strings.Repeat("f", 127), strings.Repeat("c", 126) + "a"},
			{strings.Repeat("f", 127), strings.Repeat("c", 126) + "b"},
		},
	}
	suffix := func(filter, counter string) string {
		parts := []string{fmt.Sprint(len(filter))}
		for _, b := range []byte(filter) {
			parts = append(parts, fmt.Sprint(b))
		}
		parts = append(parts, fmt.Sprint(len(counter)))
		for _, b := range []byte(counter) {
			parts = append(parts, fmt.Sprint(b))
		}
		return strings.Join(append(parts, "2"), ".")
	}
	for name, rows := range cases {
		t.Run(name, func(t *testing.T) {
			profile, err := ddsnmp.LoadProfileByName("_juniper-firewall.yaml")
			require.NoError(t, err)
			tc := stockIdentityFixture{
				TableOID: "1.3.6.1.4.1.2636.3.5.2",
				Metric:   "jnxFWCounterPacketCount",
			}
			profile = stockIdentitySelectRow(t, profile, tc)
			var pdus []gosnmp.SnmpPDU
			for i, row := range rows {
				pdus = append(
					pdus,
					gosnmp.SnmpPDU{
						Name:  "1.3.6.1.4.1.2636.3.5.2.1.4." + suffix(row[0], row[1]),
						Type:  gosnmp.Counter64,
						Value: uint64(100 + i),
					},
				)
			}

			handler := snmpmock.NewMockHandler(gomock.NewController(t))
			handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
			handler.EXPECT().MaxOids().Return(60).AnyTimes()
			walks, gets := 0, 0
			handler.EXPECT().BulkWalkAll(tc.TableOID).DoAndReturn(func(string) ([]gosnmp.SnmpPDU, error) {
				walks++
				return pdus, nil
			}).AnyTimes()
			handler.EXPECT().Get(gomock.Any()).DoAndReturn(func(oids []string) (*gosnmp.SnmpPacket, error) {
				gets++
				packet := &gosnmp.SnmpPacket{}
				for _, oid := range oids {
					found := false
					for _, pdu := range pdus {
						if strings.Trim(oid, ".") == pdu.Name {
							packet.Variables = append(packet.Variables, pdu)
							found = true
						}
					}
					require.True(t, found, "cached acquisition requested unexpected OID %s", oid)
				}
				return packet, nil
			}).AnyTimes()
			acquire := func(p *ddsnmp.Profile) *ddsnmpcollector.Collector {
				return ddsnmpcollector.New(
					ddsnmpcollector.Config{
						SnmpClient: handler,
						Profiles:   []*ddsnmp.Profile{p},
						Log:        logger.New(),
					},
				)
			}
			acquisition := acquire(profile)
			c := newTestSNMPCollector()
			c.sysInfo = &snmputils.SysInfo{}
			var firstSamples map[string]int64
			for _, phase := range []string{"first WALK", "cached GET", "profile clone", "profile reload"} {
				t.Run(phase, func(t *testing.T) {
					switch phase {
					case "profile clone":
						clone := *profile
						clone.Definition = profile.Definition.Clone()
						acquisition = acquire(&clone)
					case "profile reload":
						reloaded, err := ddsnmp.LoadProfileByName("_juniper-firewall.yaml")
						require.NoError(t, err)
						acquisition = acquire(stockIdentitySelectRow(t, reloaded, tc))
					}
					beforeWalk, beforeGet := walks, gets
					results, err := acquisition.Collect()
					require.NoError(t, err)
					require.Len(t, results, 1)
					metrics := results[0].Metrics
					require.Len(t, metrics, 2)
					if phase == "cached GET" {
						require.Equal(t, beforeWalk, walks, "cached collection must not WALK")
						require.Greater(t, gets, beforeGet, "cached collection must GET")
					} else {
						require.Greater(t, walks, beforeWalk)
					}
					for _, metric := range metrics {
						row := rows[int(metric.Value)-100]
						require.Equal(t, row[0], metric.Tags["firewall_filter_name"])
						require.Equal(
							t,
							row[1],
							metric.Tags["counter_name"],
							"semantic identity bytes must survive acquisition",
						)
					}
					samples := map[string]int64{}
					c.collectProfileTableMetrics(samples, metrics)
					require.Len(t, c.tableMetricCharts, 2, "distinct MIB INDEX tuples require separate final charts")
					ids := map[string]bool{}
					for _, state := range c.tableMetricCharts {
						chart := state.chart
						require.False(t, ids[chart.ID], "chart IDs must be unique")
						ids[chart.ID] = true
						require.Regexp(t, `^[a-zA-Z0-9_]+$`, chart.ID)
						require.Len(t, chart.Dims, 1)
						require.Contains(t, []int64{100, 101}, samples[chart.Dims[0].ID])
						require.True(t, c.Charts().Has(chart.ID))
					}
					require.Len(t, samples, 2)
					if firstSamples == nil {
						firstSamples = samples
						wire := renderIdentityMetrics(t, metrics)
						require.Contains(t, wire, "DIMENSION '"+tc.Metric+"' '"+tc.Metric+"'")
						require.Contains(t, wire, "SET '"+tc.Metric+"' = 100\n")
						require.Contains(t, wire, "SET '"+tc.Metric+"' = 101\n")
						for id := range ids {
							require.NotContains(
								t,
								wire,
								"DIMENSION '"+id+"'",
								"job-scoped lookup keys must not replace named dimensions",
							)
							require.NotContains(t, wire, "SET '"+id+"'")
							require.Contains(t, wire, "CHART 'snmp_fixture."+id+"'")
							require.Contains(t, wire, "BEGIN 'snmp_fixture."+id+"'")
						}
						for _, row := range rows {
							require.Contains(
								t,
								wire,
								"CLABEL 'counter_name' '"+netdataapi.SanitizeWireValue(row[1])+"' '1'\n",
							)
						}
						require.NotContains(t, wire, "\x00")
						require.NotContains(t, wire, "\r")
						require.NotContains(t, wire, "\\")
						require.Contains(t, wire, " = 100\n")
						require.Contains(t, wire, " = 101\n")
					} else {
						require.Equal(t, firstSamples, samples, "polling and reload must retain final identities")
					}
				})
			}
		})
	}
}

func TestTableIdentityTuple(t *testing.T) {
	metric := func(name string, tags map[string]string) ddsnmp.Metric {
		return ddsnmp.Metric{
			Name:    name,
			Tags:    tags,
			IsTable: true,
		}
	}
	for name, pair := range map[string][2]ddsnmp.Metric{
		"tag_names":            {metric("value", map[string]string{"port": "1"}), metric("value", map[string]string{"slot": "1"})},
		"empty_is_not_missing": {metric("value", map[string]string{"a": "", "b": "1"}), metric("value", map[string]string{"b": "1"})},
		"metric_boundary":      {metric("a_b", map[string]string{"c": "d"}), metric("a", map[string]string{"c": "b_d"})},
		"metric_normalization": {metric("a.b", map[string]string{"x": "1"}), metric("a_b", map[string]string{"x": "1"})},
		"tag_boundaries":       {metric("value", map[string]string{"a": "x_y", "b": "z"}), metric("value", map[string]string{"a": "x", "b": "y_z"})},
		"long_metric_prefix":   {metric(strings.Repeat("x", 2000)+"a", map[string]string{"id": "1"}), metric(strings.Repeat("x", 2000)+"b", map[string]string{"id": "1"})},
		"unicode_and_controls": {metric("value", map[string]string{"id": "é\x00\n"}), metric("value", map[string]string{"id": "e__"})},
	} {
		t.Run(name, func(t *testing.T) {
			a, b := tableMetricKey(pair[0]), tableMetricKey(pair[1])
			require.NotEqual(t, a, b)
			require.NotEqual(t, metricIDFromKey(a), metricIDFromKey(b))
			for _, id := range []string{a, b, metricIDFromKey(a, strings.Repeat("dimension", 500))} {
				require.Less(t, len(id), 256)
				require.Regexp(t, `^[a-zA-Z0-9_]+$`, id)
			}
		})
	}
	a := metric("value", map[string]string{"b": "2", "a": "1", "_alias": "before"})
	b := metric("value", map[string]string{"a": "1", "b": "2", "_alias": "after"})
	require.Equal(
		t,
		tableMetricKey(a),
		tableMetricKey(b),
		"display metadata and map insertion order do not alter identity",
	)
	require.Empty(t, tableMetricKey(metric("", map[string]string{"id": "1"})))
}

func TestTableIdentityAggregationAndDimensions(t *testing.T) {
	for _, name := range []string{"value", "bgp.peers.message_traffic"} {
		t.Run(name, func(t *testing.T) {
			c := newTestSNMPCollector()
			c.sysInfo = &snmputils.SysInfo{}
			rows := []ddsnmp.Metric{
				{
					Name:    name,
					Tags:    map[string]string{"id": "a.b"},
					Profile: &ddsnmp.ProfileMetrics{},
					IsTable: true,
					Value:   7,
				},
				{
					Name:    name,
					Tags:    map[string]string{"id": "a.b", "_alias": "display"},
					Profile: &ddsnmp.ProfileMetrics{},
					IsTable: true,
					Value:   9,
				},
			}
			samples := map[string]int64{}
			c.collectProfileTableMetrics(samples, rows)
			require.Len(t, c.tableMetricCharts, 1)
			require.Len(t, samples, 1)
			for _, value := range samples {
				require.Equal(t, int64(16), value)
			}
			rows[0].MultiValue = map[string]int64{"a.b": 1, "a_b": 2, "a b": 3, "": 4, strings.Repeat("x", 2000): 5}
			c = newTestSNMPCollector()
			c.sysInfo = &snmputils.SysInfo{}
			samples = map[string]int64{}
			c.collectProfileTableMetrics(samples, rows[:1])
			require.Len(t, samples, 5)
			for _, state := range c.tableMetricCharts {
				chart := state.chart
				require.Len(t, chart.Dims, 5)
				for _, dim := range chart.Dims {
					require.Equal(t, rows[0].MultiValue[dim.Name], samples[dim.ID])
				}
			}
		})
	}
}

// Exercise the managed V1 renderer after real acquisition and chart construction.
// Labels may be normalized for the wire, but that must never merge machine IDs.
func renderIdentityMetrics(t *testing.T, metrics []ddsnmp.Metric) string {
	t.Helper()
	c := newTestSNMPCollector()
	c.sysInfo = &snmputils.SysInfo{}
	module := &collectorapi.MockCollectorV1{
		ChartsFunc: c.Charts,
		CollectFunc: func(context.Context) map[string]int64 {
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
	defer func() {
		run.Stop(nil)
		require.True(t, waitForChartJobSignal(done), "managed job did not stop")
	}()
	require.True(t, waitForChartJobSignal(run.StartupDone()), "managed startup blocked")
	require.NoError(t, run.StartupErr())
	require.Eventually(t, func() bool {
		if strings.Contains(out.String(), "_data_collection_status'") {
			return true
		}
		job.Tick(0)
		return false
	}, 5*time.Second, 10*time.Millisecond)
	return out.String()
}
