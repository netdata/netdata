// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"fmt"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
	"github.com/stretchr/testify/require"
)

func TestStockFirewallIdentitySurvivesChartSerialization(t *testing.T) {
	cases := map[string][][2]string{
		"component_boundary":  {{"c", "a_2_b"}, {"b_2_c", "a"}},
		"chart_normalization": {{"filter", "a.b"}, {"filter", "a_b"}},
		"spaces":              {{"filter", "a b"}, {"filter", "a_b"}},
		"empty_components":    {{"", "x"}, {"x", ""}},
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
			metrics := stockIdentityAcquire(t, profile, pdus)
			require.Len(t, metrics, 2)
			c := newTestSNMPCollector()
			c.sysInfo = &snmputils.SysInfo{}
			samples := map[string]int64{}
			c.collectProfileTableMetrics(samples, metrics)
			require.Len(t, c.tableMetricCharts, 2, "distinct MIB INDEX tuples require separate final charts")
			ids := map[string]bool{}
			for _, chart := range c.tableMetricCharts {
				require.False(t, ids[chart.ID], "chart IDs must be unique")
				ids[chart.ID] = true
				require.Len(t, chart.Dims, 1)
				require.Contains(t, []int64{100, 101}, samples[chart.Dims[0].ID])
				require.True(t, c.Charts().Has(chart.ID))
			}
			require.Len(t, samples, 2)
			// Polling and recreating a collector must produce the same identities.
			for _, next := range []*Collector{c, newTestSNMPCollector()} {
				next.sysInfo = &snmputils.SysInfo{}
				nextSamples := map[string]int64{}
				next.collectProfileTableMetrics(nextSamples, metrics)
				require.Equal(t, samples, nextSamples)
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
			for _, chart := range c.tableMetricCharts {
				require.Len(t, chart.Dims, 5)
				for _, dim := range chart.Dims {
					require.Equal(t, rows[0].MultiValue[dim.Name], samples[dim.ID])
				}
			}
		})
	}
}
