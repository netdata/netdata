// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func aliasTestMetric(name string, tags map[string]string) ddsnmp.Metric {
	return ddsnmp.Metric{
		Name:    name,
		IsTable: true,
		Value:   1,
		Tags:    tags,
		Profile: &ddsnmp.ProfileMetrics{},
	}
}

func ifaceTags(iface, alias string) map[string]string {
	return map[string]string{tagInterface: iface, tagIfAlias: alias}
}

func TestCollector_TableChartNames(t *testing.T) {
	tests := map[string]struct {
		metrics   []ddsnmp.Metric
		wantNames map[string]string
	}{
		"alias becomes the display name": {
			metrics: []ddsnmp.Metric{aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "Uplink to core"))},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "ifTraffic_Uplink_to_core",
			},
		},
		"no alias keeps the chart id as name": {
			metrics: []ddsnmp.Metric{
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "")),
				aliasTestMetric("ifErrors", map[string]string{tagInterface: "Gi0/2"}),
			},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "",
				"snmp_device_prof_ifErrors_Gi0/2":  "",
			},
		},
		"whitespace-only alias is ignored": {
			metrics: []ddsnmp.Metric{aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "  "))},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "",
			},
		},
		"duplicate aliases get the interface appended": {
			metrics: []ddsnmp.Metric{
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "uplink")),
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/2", "uplink")),
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/3", "server")),
				aliasTestMetric("ifErrors", ifaceTags("Gi0/1", "uplink")),
			},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "ifTraffic_uplink_Gi0/1",
				"snmp_device_prof_ifTraffic_Gi0/2": "ifTraffic_uplink_Gi0/2",
				"snmp_device_prof_ifTraffic_Gi0/3": "ifTraffic_server",
				"snmp_device_prof_ifErrors_Gi0/1":  "ifErrors_uplink",
			},
		},
		"other key tags are kept": {
			metrics: []ddsnmp.Metric{
				aliasTestMetric("hsrpState", map[string]string{tagInterface: "Vlan10", "hsrp_group": "1", tagIfAlias: "users"}),
			},
			wantNames: map[string]string{
				"snmp_device_prof_hsrpState_1_Vlan10": "hsrpState_1_users",
			},
		},
		"protocol-unsafe characters are removed": {
			metrics: []ddsnmp.Metric{aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "it's\n a.b\\c,d|e"))},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "ifTraffic_it_s_a_b/c_d_e",
			},
		},
		"aliases equal after sanitizing are treated as duplicates": {
			metrics: []ddsnmp.Metric{
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "Uplink core")),
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/2", "Uplink_core")),
				aliasTestMetric("ifTraffic", ifaceTags("Gi0/3", "Uplink.core")),
			},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "ifTraffic_Uplink_core_Gi0/1",
				"snmp_device_prof_ifTraffic_Gi0/2": "ifTraffic_Uplink_core_Gi0/2",
				"snmp_device_prof_ifTraffic_Gi0/3": "ifTraffic_Uplink_core_Gi0/3",
			},
		},
		"duplicate suffix stays next to the alias": {
			metrics: []ddsnmp.Metric{
				aliasTestMetric("pethPort", map[string]string{tagInterface: "Gi1/0/1", "pse_port_index": "1", tagIfAlias: "ap"}),
				aliasTestMetric("pethPort", map[string]string{tagInterface: "Gi2/0/1", "pse_port_index": "1", tagIfAlias: "ap"}),
			},
			wantNames: map[string]string{
				"snmp_device_prof_pethPort_Gi1/0/1_1": "pethPort_ap_Gi1/0/1_1",
				"snmp_device_prof_pethPort_Gi2/0/1_1": "pethPort_ap_Gi2/0/1_1",
			},
		},
		"non-ascii aliases are kept": {
			metrics: []ddsnmp.Metric{aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "Büro  Nord"))},
			wantNames: map[string]string{
				"snmp_device_prof_ifTraffic_Gi0/1": "ifTraffic_Büro_Nord",
			},
		},
		"charts without an interface tag keep the chart id as name": {
			metrics: []ddsnmp.Metric{
				aliasTestMetric("ipIfStats", map[string]string{"interface_index": "3", "_interface": "Gi0/3", tagIfAlias: "x"}),
			},
			wantNames: map[string]string{
				"snmp_device_prof_ipIfStats_3": "",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := newTestSNMPCollector()
			collr.sysInfo = &snmputils.SysInfo{Name: "switch"}

			collr.collectProfileTableMetrics(make(map[string]int64), tc.metrics)

			got := make(map[string]string)
			for id := range tc.wantNames {
				chart := collr.Charts().Get(id)
				require.NotNilf(t, chart, "chart %s", id)
				got[id] = chart.OverID
			}
			assert.Equal(t, tc.wantNames, got)
		})
	}
}

func collectAliases(collr *Collector, metric string, aliases map[string]string) {
	var metrics []ddsnmp.Metric
	for iface, alias := range aliases {
		metrics = append(metrics, aliasTestMetric(metric, ifaceTags(iface, alias)))
	}
	collr.collectProfileTableMetrics(make(map[string]int64), metrics)
}

func chartNames(t *testing.T, collr *Collector, ids ...string) map[string]string {
	t.Helper()
	names := make(map[string]string)
	for _, id := range ids {
		chart := collr.Charts().Get(id)
		require.NotNilf(t, chart, "chart %s", id)
		names[id] = chart.OverID
	}
	return names
}

func markAllCreated(collr *Collector) {
	for _, chart := range *collr.Charts() {
		chart.SetCreated(true)
	}
}

func TestCollector_TableChartAliasChangeResendsChart(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.Hostname = "192.0.2.1"
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	const id = "snmp_device_prof_ifTraffic_Gi0/1"
	const other = "snmp_device_prof_ifTraffic_Gi0/2"

	collect := func(alias string) {
		collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": alias, "Gi0/2": "server"})
	}

	collect("old")
	chart := collr.Charts().Get(id)
	require.NotNil(t, chart)
	markAllCreated(collr)

	collect("old")
	assert.True(t, chart.IsCreated(), "unchanged tags must not re-send the chart")

	collect("new")
	assert.False(t, chart.IsCreated(), "changed alias must re-send the chart")
	assert.True(t, collr.Charts().Get(other).IsCreated(), "unrelated charts must not be re-sent")
	assert.Equal(t, "", chart.OverID, "a renamed chart is reset to its ID first")
	assert.Equal(t, map[string]string{
		"address":   "192.0.2.1",
		"if_alias":  "new",
		"interface": "Gi0/1",
		"sysName":   "switch",
	}, chartLabels(chart))

	markAllCreated(collr)
	collect("new")
	assert.False(t, chart.IsCreated(), "the new name must be sent on the next collection")
	assert.Equal(t, "ifTraffic_new", chart.OverID)

	markAllCreated(collr)
	collect("new")
	assert.True(t, chart.IsCreated(), "a settled name must not be re-sent")

	collect("")
	assert.False(t, chart.IsCreated(), "cleared alias must re-send the chart")
	assert.Equal(t, "", chart.OverID)
	assert.Equal(t, map[string]string{
		"address":   "192.0.2.1",
		"if_alias":  "",
		"interface": "Gi0/1",
		"sysName":   "switch",
	}, chartLabels(chart))
}

func TestCollector_TableChartOtherUnderscoreTagChangeResendsChart(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	collect := func(ifType string) {
		collr.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{
			aliasTestMetric("ifTraffic", map[string]string{tagInterface: "Gi0/1", tagIfType: ifType}),
		})
	}

	collect("ethernetCsmacd")
	chart := collr.Charts().Get("snmp_device_prof_ifTraffic_Gi0/1")
	require.NotNil(t, chart)
	chart.SetCreated(true)

	collect("ieee8023adLag")
	assert.False(t, chart.IsCreated())
	assert.Equal(t, map[string]string{
		"address":   "",
		"if_type":   "ieee8023adLag",
		"interface": "Gi0/1",
		"sysName":   "switch",
	}, chartLabels(chart))
}

func TestCollector_TableChartSwappedAliasesBothApply(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	ids := []string{"snmp_device_prof_ifTraffic_Gi0/1", "snmp_device_prof_ifTraffic_Gi0/2"}

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "x", "Gi0/2": "y"})
	require.Equal(t, map[string]string{ids[0]: "ifTraffic_x", ids[1]: "ifTraffic_y"}, chartNames(t, collr, ids...))

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "y", "Gi0/2": "x"})
	assert.Equal(t, map[string]string{ids[0]: "", ids[1]: ""}, chartNames(t, collr, ids...))

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "y", "Gi0/2": "x"})
	assert.Equal(t, map[string]string{ids[0]: "ifTraffic_y", ids[1]: "ifTraffic_x"}, chartNames(t, collr, ids...))
}

func TestCollector_TableChartNameHeldByAnotherChartIsDeferred(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	ids := []string{"snmp_device_prof_ifTraffic_Gi0/1", "snmp_device_prof_ifTraffic_Gi0/2"}

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "", "Gi0/2": "x"})
	require.Equal(t, map[string]string{ids[0]: "", ids[1]: "ifTraffic_x"}, chartNames(t, collr, ids...))

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "x", "Gi0/2": "y"})
	assert.Equal(t, map[string]string{ids[0]: "", ids[1]: ""}, chartNames(t, collr, ids...))

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "x", "Gi0/2": "y"})
	assert.Equal(t, map[string]string{ids[0]: "ifTraffic_x", ids[1]: "ifTraffic_y"}, chartNames(t, collr, ids...))
}

func TestCollector_TableChartNameDuplicateResolvedWhenPeerRemoved(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	const id = "snmp_device_prof_ifTraffic_Gi0/1"

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "uplink", "Gi0/2": "uplink"})
	require.Equal(t, map[string]string{id: "ifTraffic_uplink_Gi0/1"}, chartNames(t, collr, id))

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "uplink"})
	assert.Equal(t, map[string]string{id: ""}, chartNames(t, collr, id))

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "uplink"})
	assert.Equal(t, map[string]string{id: "ifTraffic_uplink"}, chartNames(t, collr, id))
}

func TestCollector_TableChartRowsSharingKeyDoNotFlap(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	metrics := []ddsnmp.Metric{
		aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "a")),
		aliasTestMetric("ifTraffic", ifaceTags("Gi0/1", "b")),
	}

	collr.collectProfileTableMetrics(make(map[string]int64), metrics)
	chart := collr.Charts().Get("snmp_device_prof_ifTraffic_Gi0/1")
	require.NotNil(t, chart)
	chart.SetCreated(true)

	for range 3 {
		collr.collectProfileTableMetrics(make(map[string]int64), metrics)
		assert.True(t, chart.IsCreated())
	}
}

func TestCollector_TableChartRowsSharingKeyInAnyOrderDoNotFlap(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	a := aliasTestMetric("ifTraffic", ifaceTags("eth0", "uplink-a"))
	b := aliasTestMetric("ifTraffic", ifaceTags("eth0", "uplink-b"))

	collr.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{a, b})
	collr.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{b, a})
	chart := collr.Charts().Get("snmp_device_prof_ifTraffic_eth0")
	require.NotNil(t, chart)
	name := chart.OverID
	chart.SetCreated(true)

	for i := range 10 {
		rows := []ddsnmp.Metric{a, b}
		if i%2 == 1 {
			rows = []ddsnmp.Metric{b, a}
		}
		collr.collectProfileTableMetrics(make(map[string]int64), rows)
		assert.Truef(t, chart.IsCreated(), "cycle %d re-sent the chart", i)
		assert.Equalf(t, name, chart.OverID, "cycle %d renamed the chart", i)
	}
}

func TestCollector_TableChartChangedRowsPickTheSameRowInAnyOrder(t *testing.T) {
	for name, rows := range map[string][]ddsnmp.Metric{
		"a then b": {
			aliasTestMetric("ifTraffic", ifaceTags("eth0", "uplink-a")),
			aliasTestMetric("ifTraffic", ifaceTags("eth0", "uplink-b")),
		},
		"b then a": {
			aliasTestMetric("ifTraffic", ifaceTags("eth0", "uplink-b")),
			aliasTestMetric("ifTraffic", ifaceTags("eth0", "uplink-a")),
		},
	} {
		t.Run(name, func(t *testing.T) {
			collr := newTestSNMPCollector()
			collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
			collectAliases(collr, "ifTraffic", map[string]string{"eth0": "old"})

			collr.collectProfileTableMetrics(make(map[string]int64), rows)

			chart := collr.Charts().Get("snmp_device_prof_ifTraffic_eth0")
			require.NotNil(t, chart)
			assert.Equal(t, "uplink-a", chartLabels(chart)["if_alias"])
		})
	}
}

func TestCollector_TableChartRemovedUnderscoreTagResendsChart(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "uplink"})
	chart := collr.Charts().Get("snmp_device_prof_ifTraffic_Gi0/1")
	require.NotNil(t, chart)
	chart.SetCreated(true)

	collr.collectProfileTableMetrics(make(map[string]int64), []ddsnmp.Metric{
		aliasTestMetric("ifTraffic", map[string]string{tagInterface: "Gi0/1"}),
	})

	assert.False(t, chart.IsCreated())
	assert.Equal(t, map[string]string{"address": "", "interface": "Gi0/1", "sysName": "switch"}, chartLabels(chart))
}

func TestCollector_TableChartNamesDisabled(t *testing.T) {
	collr := newTestSNMPCollector()
	collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
	collr.AliasChartNames = false

	collectAliases(collr, "ifTraffic", map[string]string{"Gi0/1": "uplink"})

	chart := collr.Charts().Get("snmp_device_prof_ifTraffic_Gi0/1")
	require.NotNil(t, chart)
	assert.Equal(t, "", chart.OverID)
	assert.Equal(t, "uplink", chartLabels(chart)["if_alias"])
}

func TestCollector_TableChartRowsSharingKeyPickTheFunctionRow(t *testing.T) {
	tests := map[string]struct {
		prev    map[string]string
		aliases []string
		want    string
	}{
		"new chart, a then b":                  {aliases: []string{"uplink-a", "uplink-b"}, want: "uplink-a"},
		"new chart, b then a":                  {aliases: []string{"uplink-b", "uplink-a"}, want: "uplink-a"},
		"new chart, empty alias first":         {aliases: []string{"", "uplink"}, want: "uplink"},
		"new chart, empty alias last":          {aliases: []string{"uplink", ""}, want: "uplink"},
		"existing empty alias, peer gains one": {prev: map[string]string{"eth0": ""}, aliases: []string{"", "uplink"}, want: "uplink"},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := newTestSNMPCollector()
			collr.sysInfo = &snmputils.SysInfo{Name: "switch"}
			if test.prev != nil {
				collectAliases(collr, "ifTraffic", test.prev)
			}

			var rows []ddsnmp.Metric
			for _, alias := range test.aliases {
				rows = append(rows, aliasTestMetric("ifTraffic", ifaceTags("eth0", alias)))
			}
			collr.collectProfileTableMetrics(make(map[string]int64), rows)

			chart := collr.Charts().Get("snmp_device_prof_ifTraffic_eth0")
			require.NotNil(t, chart)
			assert.Equal(t, test.want, chartLabels(chart)["if_alias"])
			assert.Equal(t, "ifTraffic_"+test.want, chart.OverID)
		})
	}
}
