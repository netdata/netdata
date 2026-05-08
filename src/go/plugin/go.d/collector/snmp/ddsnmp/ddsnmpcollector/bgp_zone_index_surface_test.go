// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"

	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
)

func TestCollector_Collect_AristaBGP_IPv6ZoneIndexTags(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.30065.1.3011.7010.427.48", "arista-switch.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"aristaBgp4V2PeerTable":         {"aristaBgp4V2PeerAdminStatus", "aristaBgp4V2PeerState"},
		"aristaBgp4V2PeerCountersTable": {"bgpPeerInUpdates", "bgpPeerOutUpdates", "bgpPeerFsmEstablishedTransitions"},
		"aristaBgp4V2PrefixGaugesTable": {"bgpPeerPrefixesAccepted"},
	}, []string{"bgpPeerAvailability", "bgpPeerUpdates"})

	idx := "7.4.20.254.128.1.2.0.0.0.0.194.213.130.253.254.123.34.167.0.0.14.132"
	prefixIdx := idx + ".2.1"
	localAddr := []byte{
		254, 128, 1, 2, 0, 0, 0, 0, 194, 213, 130, 253, 254, 123, 34, 168,
		0, 0, 14, 133,
	}

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"aristaBgp4V2PeerTable": {
			createIntegerPDU(oidWithIndex(requireSymbolOID(t, profile, "aristaBgp4V2PeerTable", "aristaBgp4V2PeerAdminStatus"), idx), 2),
			createIntegerPDU(oidWithIndex(requireSymbolOID(t, profile, "aristaBgp4V2PeerTable", "aristaBgp4V2PeerState"), idx), 6),
			createGauge32PDU(oidWithIndex(requireMetricTagOID(t, profile, "aristaBgp4V2PeerTable", "local_as"), idx), 65000),
			createGauge32PDU(oidWithIndex(requireMetricTagOID(t, profile, "aristaBgp4V2PeerTable", "remote_as"), idx), 65001),
			createPDU(oidWithIndex(requireMetricTagOID(t, profile, "aristaBgp4V2PeerTable", "local_identifier"), idx), gosnmp.IPAddress, "198.51.100.12"),
			createPDU(oidWithIndex(requireMetricTagOID(t, profile, "aristaBgp4V2PeerTable", "peer_identifier"), idx), gosnmp.IPAddress, "203.0.113.12"),
			createStringPDU(oidWithIndex(requireMetricTagOID(t, profile, "aristaBgp4V2PeerTable", "peer_description"), idx), "link-local-upstream"),
			createPDU(oidWithIndex(requireMetricTagOID(t, profile, "aristaBgp4V2PeerTable", "local_address"), idx), gosnmp.OctetString, localAddr),
		},
		"aristaBgp4V2PeerCountersTable": {
			createCounter32PDU(oidWithIndex(requireSymbolOID(t, profile, "aristaBgp4V2PeerCountersTable", "bgpPeerInUpdates"), idx), 16),
			createCounter32PDU(oidWithIndex(requireSymbolOID(t, profile, "aristaBgp4V2PeerCountersTable", "bgpPeerOutUpdates"), idx), 17),
			createCounter32PDU(oidWithIndex(requireSymbolOID(t, profile, "aristaBgp4V2PeerCountersTable", "bgpPeerFsmEstablishedTransitions"), idx), 2),
		},
		"aristaBgp4V2PrefixGaugesTable": {
			createGauge32PDU(oidWithIndex(requireSymbolOID(t, profile, "aristaBgp4V2PrefixGaugesTable", "bgpPeerPrefixesAccepted"), prefixIdx), 1301),
		},
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.30065.1.3011.7010.427.48",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance":      "7",
		"neighbor_address_type": "ipv6z",
		"neighbor":              "fe80:102::c2d5:82fd:fe7b:22a7%0.0.14.132",
		"remote_as":             "65001",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
	assert.Equal(t, "fe80:102::c2d5:82fd:fe7b:22a8%0.0.14.133", availability.Tags["local_address"])

	updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"neighbor_address_type": "ipv6z",
		"neighbor":              "fe80:102::c2d5:82fd:fe7b:22a7%0.0.14.132",
	})
	assert.Equal(t, map[string]int64{"received": 16, "sent": 17}, updates.MultiValue)

	accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor_address_type":     "ipv6z",
		"neighbor":                  "fe80:102::c2d5:82fd:fe7b:22a7%0.0.14.132",
		"address_family":            "ipv6",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 1301, accepted.Value)
	assert.Equal(t, "fe80:102::c2d5:82fd:fe7b:22a8%0.0.14.133", accepted.Tags["local_address"])
}

func TestCollector_Collect_CiscoBgpPeer3_IPv6ZoneIndexTags(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.9.1.923", "cisco-asr.yaml")
	filterProfileForTypedBGPByID(t, profile, "cisco-bgp-peer")

	idx := "7.4.20.254.128.1.2.0.0.0.0.194.213.130.253.254.123.34.167.0.0.14.132"
	remoteAddr := []byte{
		254, 128, 1, 2, 0, 0, 0, 0, 194, 213, 130, 253, 254, 123, 34, 167,
		0, 0, 14, 132,
	}
	localAddr := []byte{
		254, 128, 1, 2, 0, 0, 0, 0, 194, 213, 130, 253, 254, 123, 34, 168,
		0, 0, 14, 133,
	}

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	expectSNMPWalk(mockHandler, gosnmp.Version2c, "1.3.6.1.4.1.9.9.187.1.2.9", []gosnmp.SnmpPDU{
		createStringPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.4", idx), "blue"),
		createPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.3", idx), gosnmp.OctetString, remoteAddr),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.5", idx), 6),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.6", idx), 2),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.7", idx), 4),
		createPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.8", idx), gosnmp.OctetString, localAddr),
		createGauge32PDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.10", idx), 65000),
		createPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.11", idx), gosnmp.IPAddress, "198.51.100.10"),
		createGauge32PDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.13", idx), 65001),
		createPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.14", idx), gosnmp.IPAddress, "203.0.113.10"),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.15", idx), 21),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.16", idx), 22),
		createPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.19", idx), gosnmp.OctetString, []byte{0x02, 0x03}),
		createCounter32PDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.20", idx), 5),
		createIntegerPDU(oidWithIndex("1.3.6.1.4.1.9.9.187.1.2.9.1.31", idx), 5),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.9.1.923",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)
	require.Empty(t, results[0].Metrics)
	require.Len(t, results[0].BGPRows, 1)

	row := results[0].BGPRows[0]
	assert.Equal(t, "blue", row.Identity.RoutingInstance)
	assert.Equal(t, "ipv6z", row.Descriptors.PeerType)
	assert.Equal(t, "fe80:102::c2d5:82fd:fe7b:22a7%0.0.14.132", row.Identity.Neighbor)
	assert.Equal(t, "65001", row.Identity.RemoteAS)
	assert.Equal(t, "fe80:102::c2d5:82fd:fe7b:22a8%0.0.14.133", row.Descriptors.LocalAddress)
	require.True(t, row.Admin.Enabled.Has)
	assert.True(t, row.Admin.Enabled.Value)
	require.True(t, row.State.Has)
	assert.Equal(t, "established", string(row.State.State))
	assert.EqualValues(t, 21, row.Traffic.Updates.Received.Value)
	assert.EqualValues(t, 22, row.Traffic.Updates.Sent.Value)
	assert.EqualValues(t, 5, row.Transitions.Established.Value)
	assert.Equal(t, "openconfirm", string(row.Previous.State))
	assert.EqualValues(t, 2, row.LastError.Code.Value)
	assert.EqualValues(t, 3, row.LastError.Subcode.Value)
}
