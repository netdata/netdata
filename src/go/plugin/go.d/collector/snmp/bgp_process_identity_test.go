// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"fmt"
	"slices"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/snmputils"
)

func TestHuaweiProcessesRetainAllCountersThroughChartsAndFunctions(t *testing.T) {
	profile, err := ddsnmp.LoadProfileByName("_huawei-bgp-vpn.yaml")
	require.NoError(t, err)
	profile.Definition.BGP = slices.DeleteFunc(
		profile.Definition.BGP,
		func(row ddprofiledefinition.BGPConfig) bool { return row.ID != "huawei-bgp-peer-statistics" },
	)
	const root = "1.3.6.1.4.1.2011.5.25.177.1.1.7"
	var pdus []gosnmp.SnmpPDU
	for process := 0; process < 3; process++ {
		for column := 4; column <= 9; column++ {
			pdus = append(
				pdus,
				gosnmp.SnmpPDU{
					Name:  fmt.Sprintf("%s.1.%d.%d.7.4.10.0.4.1", root, column, process),
					Type:  gosnmp.Counter32,
					Value: uint(process*100 + column),
				},
			)
		}
	}
	handler := snmpmock.NewMockHandler(gomock.NewController(t))
	handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	handler.EXPECT().BulkWalkAll(root).Return(pdus, nil).AnyTimes()

	acquisition := ddsnmpcollector.New(
		ddsnmpcollector.Config{
			SnmpClient: handler,
			Profiles:   []*ddsnmp.Profile{profile},
			Log:        logger.New(),
		},
	)
	for _, phase := range []string{"first WALK", "repeated WALK", "profile clone"} {
		t.Run(phase, func(t *testing.T) {
			if phase == "profile clone" {
				cloned := *profile
				cloned.Definition = profile.Definition.Clone()
				acquisition = ddsnmpcollector.New(
					ddsnmpcollector.Config{
						SnmpClient: handler,
						Profiles:   []*ddsnmp.Profile{&cloned},
						Log:        logger.New(),
					},
				)
			}
			pms, err := acquisition.Collect()
			require.NoError(t, err)
			require.Len(t, pms, 1)
			require.NoError(t, pms[0].BGPCollectError)
			require.Len(t, pms[0].BGPRows, 3)
			metrics := typedBGPMetricsFromProfileMetrics(pms)
			collr := newTestSNMPCollector()
			collr.sysInfo = &snmputils.SysInfo{}
			samples := make(map[string]int64)
			var peerMetrics []ddsnmp.Metric
			for _, metric := range metrics {
				if metric.IsTable {
					peerMetrics = append(peerMetrics, metric)
				}
			}
			require.Len(t, peerMetrics, 12)
			collr.collectProfileTableMetrics(samples, peerMetrics)
			require.Len(t, collr.tableMetricCharts, 12)
			require.Len(t, samples, 18)
			expected := make(map[int64]bool)
			for process := 0; process < 3; process++ {
				for column := 4; column <= 9; column++ {
					expected[int64(process*100+column)] = true
				}
			}
			for _, value := range samples {
				require.True(t, expected[value], "unexpected or merged sample %d", value)
				delete(expected, value)
			}
			require.Empty(t, expected)
			for _, chart := range *collr.Charts() {
				labels := chartLabels(chart)
				assert.Contains(t, []string{"0", "1", "2"}, labels["process_id"])
				assert.Equal(t, "7", labels["routing_instance"])
				assert.Equal(t, "10.0.4.1", labels["neighbor"])
				assert.Empty(t, labels["remote_as"])
			}
			cache := newBGPPeerCache()
			cache.reset()
			for _, row := range pms[0].BGPRows {
				cache.updateRow("huawei", row)
			}
			cache.finalize()
			response := newTestFuncBGPPeers(cache).Handle(context.Background(), bgpPeersMethodID, nil)
			rows := responseRows(t, response)
			require.Len(t, rows, 3)
			processes := map[any]bool{}
			for _, row := range rows {
				processes[row[findBGPPeerColIdx("Process ID")]] = true
				assert.Equal(t, "", row[findBGPPeerColIdx("Remote AS")])
			}
			assert.Equal(t, map[any]bool{"0": true, "1": true, "2": true}, processes)
		})
	}
}

func TestBGPProcessFallbackIdentity(t *testing.T) {
	base := map[string]string{"routing_instance": "7", "neighbor": "192.0.2.1", "remote_as": "65001"}
	key := bgpPeerEntryKey("peers", base)
	require.NotEmpty(t, key)
	base["process_id"] = ""
	assert.Equal(t, key, bgpPeerEntryKey("peers", base), "absent process does not change existing fallback identity")
	base["process_id"] = "0"
	withProcess := bgpPeerEntryKey("peers", base)
	assert.NotEqual(t, key, withProcess)
	base["remote_as"] = ""
	require.NotEmpty(t, bgpPeerEntryKey("peers", base), "process and VPN identity do not require an unproved AS")
	base["routing_instance"] = ""
	assert.Empty(t, bgpPeerEntryKey("peers", base))
	base["process_id"] = ""
	assert.Empty(t, bgpPeerEntryKey("peers", base), "other vendors still require AS")
}
