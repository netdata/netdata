// SPDX-License-Identifier: GPL-3.0-or-later

package snmp

import (
	"context"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	snmpmock "github.com/gosnmp/gosnmp/mocks"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddsnmpcollector"
)

func TestNormalizeCollectorMetrics_BGPPeerAndDeviceSummaries(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile:    pm,
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
				},
				{
					Profile: pm,
					Name:    "bgpPeerAdminStatus",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{
						"start": 1,
						"stop":  0,
					},
				},
				{
					Profile: pm,
					Name:    "bgpPeerState",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{
						"idle":        0,
						"connect":     0,
						"active":      0,
						"opensent":    0,
						"openconfirm": 0,
						"established": 1,
					},
				},
				{
					Profile: pm,
					Name:    "bgpPeerInUpdates",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   5,
				},
				{
					Profile: pm,
					Name:    "bgpPeerOutUpdates",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   7,
				},
				{
					Profile: pm,
					Name:    "bgpPeerFsmEstablishedTime",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   123,
				},
				{
					Profile: pm,
					Name:    "bgpPeerInTotalMessages",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   10,
				},
				{
					Profile: pm,
					Name:    "bgpPeerOutTotalMessages",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					Value:   20,
				},
			},
		},
	})

	availability := requireMetric(t, metrics, "bgp.peers.availability", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

	state := requireMetric(t, metrics, "bgp.peers.connection_state", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.EqualValues(t, 1, state.MultiValue["established"])

	uptime := requireMetric(t, metrics, "bgp.peers.established_uptime", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"uptime": 123}, uptime.MultiValue)

	messages := requireMetric(t, metrics, "bgp.peers.message_traffic", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"received": 10, "sent": 20}, messages.MultiValue)

	counts := requireMetric(t, metrics, "bgp.devices.peer_counts", nil)
	assert.Equal(t, map[string]int64{"configured": 1, "admin_enabled": 1, "established": 1}, counts.MultiValue)

	states := requireMetric(t, metrics, "bgp.devices.peer_states", nil)
	assert.EqualValues(t, 1, states.MultiValue["established"])
	assert.EqualValues(t, 0, states.MultiValue["idle"])

	assertMetricNameAbsent(t, metrics, "bgpPeerAvailability")
	assertMetricNameAbsent(t, metrics, "bgpPeerAdminStatus")
	assertMetricNameAbsent(t, metrics, "bgpPeerState")
	assertMetricNameAbsent(t, metrics, "bgpPeerInUpdates")
	assertMetricNameAbsent(t, metrics, "bgpPeerOutUpdates")
}

func TestNormalizeCollectorMetrics_DeviceSummariesUseMergedPeerIdentity(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile:    pm,
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
				},
				{
					Profile:    pm,
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
				},
				{
					Profile: pm,
					Name:    "bgpPeerState",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{
						"idle":        0,
						"connect":     0,
						"active":      0,
						"opensent":    0,
						"openconfirm": 0,
						"established": 1,
					},
				},
				{
					Profile: pm,
					Name:    "bgpPeerState",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{
						"idle":        0,
						"connect":     0,
						"active":      0,
						"opensent":    0,
						"openconfirm": 0,
						"established": 1,
					},
				},
			},
		},
	})

	counts := requireMetric(t, metrics, "bgp.devices.peer_counts", nil)
	assert.Equal(t, map[string]int64{"configured": 1, "admin_enabled": 1, "established": 1}, counts.MultiValue)

	states := requireMetric(t, metrics, "bgp.devices.peer_states", nil)
	assert.EqualValues(t, 1, states.MultiValue["established"])
}

func TestNormalizeCollectorMetrics_BGPPublicInformationalLabelsDoNotAffectIdentity(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile:    pm,
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001", "local_address": "198.51.100.1", "peer_description": "Transit A"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
				},
				{
					Profile:    pm,
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001", "local_address": "198.51.100.2", "peer_description": "Transit B"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
				},
			},
		},
	})

	availability := requireMetric(t, metrics, "bgp.peers.availability", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.Equal(t, "bgp.peers.availability_192.0.2.1_65001", tableMetricKey(*availability))
	assert.Equal(t, "198.51.100.1", availability.Tags["_local_address"])
	assert.Equal(t, "Transit A", availability.Tags["_peer_description"])
	assert.NotContains(t, availability.Tags, "local_address")
	assert.NotContains(t, availability.Tags, "peer_description")

	var count int
	for _, metric := range metrics {
		if metric.Name == "bgp.peers.availability" {
			count++
		}
	}
	assert.Equal(t, 1, count)
}

func TestNormalizeCollectorMetrics_HuaweiPeerFamiliesAndCounts(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile: pm,
					Name:    "huawei.hwBgpPeerState",
					IsTable: true,
					Table:   "hwBgpPeerTable",
					Tags: map[string]string{
						"_routing_instance":          "Public",
						"_neighbor":                  "10.45.2.2",
						"_remote_as":                 "26479",
						"_address_family":            "ipv4",
						"_subsequent_address_family": "unicast",
						"_neighbor_address_type":     "ipv4",
					},
					MultiValue: map[string]int64{
						"idle":        0,
						"connect":     0,
						"active":      0,
						"opensent":    0,
						"openconfirm": 0,
						"established": 1,
					},
				},
				{
					Profile: pm,
					Name:    "bgpPeerUpdates",
					IsTable: true,
					Table:   "hwBgpPeerStatisticTable",
					Tags: map[string]string{
						"_routing_instance":          "0",
						"_neighbor":                  "10.45.2.2",
						"_remote_as":                 "26479",
						"_address_family":            "all",
						"_subsequent_address_family": "all",
						"_neighbor_address_type":     "ipv4",
					},
					MultiValue: map[string]int64{"received": 70063, "sent": 971},
				},
				{Profile: pm, Name: "huawei.hwBgpPeerSessionNum", Value: 12},
				{Profile: pm, Name: "huawei.hwIBgpPeerSessionNum", Value: 4},
				{Profile: pm, Name: "huawei.hwEBgpPeerSessionNum", Value: 8},
			},
		},
	})

	state := requireMetric(t, metrics, "bgp.peer_families.connection_state", map[string]string{
		"routing_instance":          "Public",
		"neighbor":                  "10.45.2.2",
		"remote_as":                 "26479",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 1, state.MultiValue["established"])
	assert.Equal(t, "ipv4", state.Tags["_neighbor_address_type"])
	assert.NotContains(t, state.Tags, "neighbor_address_type")

	updates := requireMetric(t, metrics, "bgp.peers.update_traffic", map[string]string{
		"routing_instance": "0",
		"neighbor":         "10.45.2.2",
		"remote_as":        "26479",
	})
	assert.Equal(t, map[string]int64{"received": 70063, "sent": 971}, updates.MultiValue)
	assert.Equal(t, "ipv4", updates.Tags["_neighbor_address_type"])
	assert.NotContains(t, updates.Tags, "neighbor_address_type")
	assert.NotContains(t, updates.Tags, "address_family")
	assert.NotContains(t, updates.Tags, "subsequent_address_family")

	counts := requireMetric(t, metrics, "bgp.devices.peer_counts", nil)
	assert.Equal(t, map[string]int64{"configured": 12, "ibgp": 4, "ebgp": 8}, counts.MultiValue)

	assert.Nil(t, findMetric(metrics, "bgp.devices.peer_states", nil))
}

func TestNormalizeCollectorMetrics_AlcatelRouteFamilyInference(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile: pm,
					Name:    "bgpPeerPrefixesReceived",
					IsTable: true,
					Table:   "alaBgpPeer6Table",
					Tags: map[string]string{
						"neighbor":  "2001:db8::1",
						"remote_as": "65001",
					},
					Value: 77,
				},
			},
		},
	})

	routes := requireMetric(t, metrics, "bgp.peer_families.route_counts.current", map[string]string{
		"neighbor":                  "2001:db8::1",
		"remote_as":                 "65001",
		"address_family":            "ipv6",
		"subsequent_address_family": "unicast",
	})
	assert.Equal(t, map[string]int64{"received": 77}, routes.MultiValue)
	assert.Equal(t, "ipv6 unicast", routes.Tags["_address_family_name"])
	assert.NotContains(t, routes.Tags, "address_family_name")
}

func TestNormalizeCollectorMetrics_RouteDimKeepsFirstEquivalentSource(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile: pm,
					Name:    "bgpPeerInTotalMessages",
					IsTable: true,
					Table:   "aristaBgp4V2PeerCountersTable",
					Tags: map[string]string{
						"routing_instance": "default",
						"neighbor":         "192.0.2.1",
						"remote_as":        "65001",
					},
					Value: 20,
				},
				{
					Profile: pm,
					Name:    "bgpPeerInTotalMessages",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags: map[string]string{
						"routing_instance": "default",
						"neighbor":         "192.0.2.1",
						"remote_as":        "65001",
					},
					Value: 10,
				},
				{
					Profile: pm,
					Name:    "bgpPeerOutTotalMessages",
					IsTable: true,
					Table:   "aristaBgp4V2PeerCountersTable",
					Tags: map[string]string{
						"routing_instance": "default",
						"neighbor":         "192.0.2.1",
						"remote_as":        "65001",
					},
					Value: 30,
				},
				{
					Profile: pm,
					Name:    "bgpPeerOutTotalMessages",
					IsTable: true,
					Table:   "bgpPeerTable",
					Tags: map[string]string{
						"routing_instance": "default",
						"neighbor":         "192.0.2.1",
						"remote_as":        "65001",
					},
					Value: 15,
				},
			},
		},
	})

	messages := requireMetric(t, metrics, "bgp.peers.message_traffic", map[string]string{
		"routing_instance": "default",
		"neighbor":         "192.0.2.1",
		"remote_as":        "65001",
	})
	assert.Equal(t, map[string]int64{"received": 20, "sent": 30}, messages.MultiValue)
}

func TestNormalizeCollectorMetrics_HuaweiTotalMessagesKeepPeerAndPeerFamilyScopesSeparate(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile: pm,
					Name:    "huawei.hwBgpPeerInTotalMsgCounter",
					IsTable: true,
					Table:   "hwBgpPeerMessageTable",
					Tags: map[string]string{
						"_routing_instance":          "Public",
						"_neighbor":                  "10.45.2.2",
						"_remote_as":                 "26479",
						"_address_family":            "ipv4",
						"_subsequent_address_family": "unicast",
						"_neighbor_address_type":     "ipv4",
					},
					Value: 100,
				},
				{
					Profile: pm,
					Name:    "huawei.hwBgpPeerOutTotalMsgCounter",
					IsTable: true,
					Table:   "hwBgpPeerMessageTable",
					Tags: map[string]string{
						"_routing_instance":          "Public",
						"_neighbor":                  "10.45.2.2",
						"_remote_as":                 "26479",
						"_address_family":            "ipv4",
						"_subsequent_address_family": "unicast",
						"_neighbor_address_type":     "ipv4",
					},
					Value: 80,
				},
				{
					Profile: pm,
					Name:    "huawei.hwBgpPeerInTotalMsgs",
					IsTable: true,
					Table:   "hwBgpPeerStatisticTable",
					Tags: map[string]string{
						"_routing_instance":          "0",
						"_neighbor":                  "10.45.2.2",
						"_remote_as":                 "26479",
						"_address_family":            "all",
						"_subsequent_address_family": "all",
						"_neighbor_address_type":     "ipv4",
					},
					Value: 200,
				},
				{
					Profile: pm,
					Name:    "huawei.hwBgpPeerOutTotalMsgs",
					IsTable: true,
					Table:   "hwBgpPeerStatisticTable",
					Tags: map[string]string{
						"_routing_instance":          "0",
						"_neighbor":                  "10.45.2.2",
						"_remote_as":                 "26479",
						"_address_family":            "all",
						"_subsequent_address_family": "all",
						"_neighbor_address_type":     "ipv4",
					},
					Value: 160,
				},
			},
		},
	})

	peerMessages := requireMetric(t, metrics, "bgp.peers.message_traffic", map[string]string{
		"routing_instance": "0",
		"neighbor":         "10.45.2.2",
		"remote_as":        "26479",
	})
	assert.Equal(t, map[string]int64{"received": 200, "sent": 160}, peerMessages.MultiValue)
	assert.Equal(t, "ipv4", peerMessages.Tags["_neighbor_address_type"])
	assert.NotContains(t, peerMessages.Tags, "neighbor_address_type")
	assert.NotContains(t, peerMessages.Tags, "address_family")
	assert.NotContains(t, peerMessages.Tags, "subsequent_address_family")

	peerFamilyMessages := requireMetric(t, metrics, "bgp.peer_families.message_traffic", map[string]string{
		"routing_instance":          "Public",
		"neighbor":                  "10.45.2.2",
		"remote_as":                 "26479",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.Equal(t, map[string]int64{"received": 100, "sent": 80}, peerFamilyMessages.MultiValue)
	assert.Equal(t, "ipv4", peerFamilyMessages.Tags["_neighbor_address_type"])
	assert.Equal(t, "ipv4 unicast", peerFamilyMessages.Tags["_address_family_name"])
	assert.NotContains(t, peerFamilyMessages.Tags, "neighbor_address_type")
	assert.NotContains(t, peerFamilyMessages.Tags, "address_family_name")
}

func TestNormalizeCollectorMetrics_AlcatelIPv4AndIPv6AliasesSharePublicSurface(t *testing.T) {
	pm := &ddsnmp.ProfileMetrics{Tags: map[string]string{}}
	metrics := normalizeCollectorMetrics([]*ddsnmp.ProfileMetrics{
		{
			Metrics: []ddsnmp.Metric{
				{
					Profile:    pm,
					Name:       "bgpPeerAvailability",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 1},
				},
				{
					Profile:    pm,
					Name:       "alcatel.bgpPeerAvailability",
					IsTable:    true,
					Table:      "alaBgpPeer6Table",
					Tags:       map[string]string{"neighbor": "2001:db8::1", "remote_as": "65001"},
					MultiValue: map[string]int64{"admin_enabled": 1, "established": 0},
				},
				{
					Profile:    pm,
					Name:       "bgpPeerUpdates",
					IsTable:    true,
					Table:      "bgpPeerTable",
					Tags:       map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
					MultiValue: map[string]int64{"received": 5, "sent": 7},
				},
				{
					Profile:    pm,
					Name:       "alcatel.bgpPeerUpdates",
					IsTable:    true,
					Table:      "alaBgpPeer6Table",
					Tags:       map[string]string{"neighbor": "2001:db8::1", "remote_as": "65001"},
					MultiValue: map[string]int64{"received": 9, "sent": 11},
				},
			},
		},
	})

	v4Availability := requireMetric(t, metrics, "bgp.peers.availability", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, v4Availability.MultiValue)

	v6Availability := requireMetric(t, metrics, "bgp.peers.availability", map[string]string{
		"neighbor":  "2001:db8::1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 0}, v6Availability.MultiValue)

	v4Updates := requireMetric(t, metrics, "bgp.peers.update_traffic", map[string]string{
		"neighbor":  "192.0.2.1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"received": 5, "sent": 7}, v4Updates.MultiValue)

	v6Updates := requireMetric(t, metrics, "bgp.peers.update_traffic", map[string]string{
		"neighbor":  "2001:db8::1",
		"remote_as": "65001",
	})
	assert.Equal(t, map[string]int64{"received": 9, "sent": 11}, v6Updates.MultiValue)
}

func TestCollector_Collect_UsesBGPPublicMetricIDs(t *testing.T) {
	mockCtl := gomock.NewController(t)
	defer mockCtl.Finish()

	mockSNMP := snmpmock.NewMockHandler(mockCtl)
	setMockClientInitExpect(mockSNMP)
	setMockClientSysInfoExpect(mockSNMP)

	collr := New()
	collr.Config = prepareV2Config()
	collr.CreateVnode = false
	collr.Ping.Enabled = false
	collr.snmpProfiles = []*ddsnmp.Profile{{}}
	collr.newSnmpClient = func() gosnmp.Handler { return mockSNMP }
	collr.newDdSnmpColl = func(ddsnmpcollector.Config) ddCollector {
		return &mockDdSnmpCollector{pms: []*ddsnmp.ProfileMetrics{
			{
				Source: "test",
				Metrics: []ddsnmp.Metric{
					{
						Name:    "bgpPeerAvailability",
						IsTable: true,
						Table:   "bgpPeerTable",
						Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001", "local_address": "198.51.100.1", "peer_description": "Transit A"},
						MultiValue: map[string]int64{
							"admin_enabled": 1,
							"established":   1,
						},
						Profile: &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
					},
					{
						Name:    "bgpPeerState",
						IsTable: true,
						Table:   "bgpPeerTable",
						Tags:    map[string]string{"neighbor": "192.0.2.1", "remote_as": "65001"},
						MultiValue: map[string]int64{
							"idle":        0,
							"connect":     0,
							"active":      0,
							"opensent":    0,
							"openconfirm": 0,
							"established": 1,
						},
						Profile: &ddsnmp.ProfileMetrics{Tags: map[string]string{}},
					},
				},
			},
		}}
	}

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)

	assert.EqualValues(t, 1, mx["snmp_bgp_peers_availability_192_0_2_1_65001_admin_enabled"])
	assert.EqualValues(t, 1, mx["snmp_bgp_peers_availability_192_0_2_1_65001_established"])
	assert.EqualValues(t, 1, mx["snmp_bgp_devices_peer_counts_configured"])
	assert.NotContains(t, mx, "snmp_device_prof_bgp.peers.availability_192.0.2.1_65001_admin_enabled")

	peerChart := collr.Charts().Get("snmp_bgp_peers_availability_192_0_2_1_65001")
	require.NotNil(t, peerChart)
	assert.Equal(t, "snmp.bgp.peers.availability", peerChart.Ctx)
	peerLabels := chartLabels(peerChart)
	assert.Equal(t, "198.51.100.1", peerLabels["local_address"])
	assert.Equal(t, "Transit A", peerLabels["peer_description"])

	deviceChart := collr.Charts().Get("snmp_bgp_devices_peer_counts")
	require.NotNil(t, deviceChart)
	assert.Equal(t, "snmp.bgp.devices.peer_counts", deviceChart.Ctx)
}

func findMetric(metrics []ddsnmp.Metric, name string, tags map[string]string) *ddsnmp.Metric {
	for i := range metrics {
		if metrics[i].Name != name {
			continue
		}
		if len(tags) == 0 {
			if len(metrics[i].Tags) == 0 {
				return &metrics[i]
			}
			continue
		}
		if tagsContained(metrics[i].Tags, tags) {
			return &metrics[i]
		}
	}
	return nil
}

func requireMetric(t *testing.T, metrics []ddsnmp.Metric, name string, tags map[string]string) *ddsnmp.Metric {
	t.Helper()
	metric := findMetric(metrics, name, tags)
	require.NotNil(t, metric, "expected metric %s with tags %v", name, tags)
	return metric
}

func assertMetricNameAbsent(t *testing.T, metrics []ddsnmp.Metric, name string) {
	t.Helper()
	for i := range metrics {
		if metrics[i].Name == name {
			t.Fatalf("unexpected metric %s with tags %v", name, metrics[i].Tags)
		}
	}
}

func tagsContained(have, want map[string]string) bool {
	for key, wantValue := range want {
		if have[key] != wantValue {
			return false
		}
	}
	return true
}
