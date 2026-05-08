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

func TestCollector_Collect_AristaBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.30065.1.3011.7050.1958.128", "arista-switch.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"aristaBgp4V2PeerTable": {
			"aristaBgp4V2PeerAdminStatus",
			"aristaBgp4V2PeerState",
		},
		"aristaBgp4V2PeerErrorsTable": {
			"bgpPeerLastErrorCode",
			"bgpPeerLastErrorSubcode",
		},
		"aristaBgp4V2PeerEventTimesTable": {
			"bgpPeerFsmEstablishedTime",
		},
		"aristaBgp4V2PeerCountersTable": {
			"bgpPeerInUpdates",
			"bgpPeerOutUpdates",
			"bgpPeerInTotalMessages",
			"bgpPeerOutTotalMessages",
		},
		"aristaBgp4V2PrefixGaugesTable": {
			"bgpPeerPrefixesReceived",
			"bgpPeerPrefixesAccepted",
		},
	}, []string{"bgpPeerAvailability", "bgpPeerUpdates"})

	pdus := loadSnmprecPDUs(t, "librenms/arista_eos_bgp_peer_table.snmprec")

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"aristaBgp4V2PeerTable":           snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.30065.4.1.1.2."),
		"aristaBgp4V2PeerErrorsTable":     snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.30065.4.1.1.3."),
		"aristaBgp4V2PeerEventTimesTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.30065.4.1.1.4."),
		"aristaBgp4V2PeerCountersTable":   snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.30065.4.1.1.7."),
		"aristaBgp4V2PrefixGaugesTable":   snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.30065.4.1.1.8."),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.30065.1.3011.7050.1958.128",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "1",
		"neighbor":         "192.168.0.2",
		"remote_as":        "65000",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
	assert.Equal(t, "peer.example.com EBGP", availability.Tags["peer_description"])
	assert.Equal(t, "192.168.0.1", availability.Tags["local_address"])

	updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"neighbor": "192.168.0.2",
	})
	assert.Equal(t, map[string]int64{"received": 13362513, "sent": 13579799}, updates.MultiValue)

	inTotal := requireMetricWithTags(t, metrics, "bgpPeerInTotalMessages", map[string]string{
		"neighbor": "192.168.0.2",
	})
	assert.EqualValues(t, 13579799, inTotal.Value)

	outTotal := requireMetricWithTags(t, metrics, "bgpPeerOutTotalMessages", map[string]string{
		"neighbor": "192.168.0.2",
	})
	assert.EqualValues(t, 17644316, outTotal.Value)

	establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"neighbor": "192.168.0.2",
	})
	assert.EqualValues(t, 13090321, establishedTime.Value)

	lastErrorCode := requireMetricWithTags(t, metrics, "bgpPeerLastErrorCode", map[string]string{
		"neighbor": "192.168.0.2",
	})
	assert.EqualValues(t, 0, lastErrorCode.Value)

	accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "192.168.0.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 540339, accepted.Value)
	assert.Equal(t, "peer.example.com EBGP", accepted.Tags["peer_description"])
	assert.Equal(t, "192.168.0.1", accepted.Tags["local_address"])

	received := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceived", map[string]string{
		"neighbor":                  "192.168.0.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 540336, received.Value)
}

func TestCollector_Collect_DellBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.674.11000.5000.100.2.1.7", "dell-os10.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"dell.os10bgp4V2PeerTable": {
			"dell.os10bgp4V2PeerAdminStatus",
			"dell.os10bgp4V2PeerState",
		},
		"dell.os10bgp4V2PeerErrorsTable": {
			"bgpPeerLastErrorCode",
			"bgpPeerLastErrorSubcode",
		},
		"dell.os10bgp4V2PeerEventTimesTable": {
			"bgpPeerFsmEstablishedTime",
			"bgpPeerInUpdateElapsedTime",
		},
		"dell.os10bgp4V2PeerCountersTable": {
			"bgpPeerInUpdates",
			"bgpPeerOutUpdates",
			"bgpPeerInTotalMessages",
			"bgpPeerOutTotalMessages",
			"bgpPeerFsmEstablishedTransitions",
		},
		"dell.os10bgp4V2PrefixGaugesTable": {
			"bgpPeerPrefixesReceived",
			"bgpPeerPrefixesAccepted",
			"bgpPeerPrefixesAdvertised",
		},
	}, []string{"bgpPeerAvailability", "bgpPeerUpdates"})

	pdus := loadSnmprecPDUs(t, "librenms/dell_os10_bgp_peer_table.snmprec")

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"dell.os10bgp4V2PeerTable":           snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.674.11000.5000.200.1.1.2."),
		"dell.os10bgp4V2PeerErrorsTable":     snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.674.11000.5000.200.1.1.3."),
		"dell.os10bgp4V2PeerEventTimesTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.674.11000.5000.200.1.1.4."),
		"dell.os10bgp4V2PeerCountersTable":   snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.674.11000.5000.200.1.1.7."),
		"dell.os10bgp4V2PrefixGaugesTable":   snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.674.11000.5000.200.1.1.8."),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.674.11000.5000.100.2.1.7",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "1",
		"neighbor":         "169.254.247.1",
		"remote_as":        "64513",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 0, "established": 1}, availability.MultiValue)
	assert.Equal(t, "DIRECT CONNECT", availability.Tags["peer_description"])
	assert.Equal(t, "169.254.247.2", availability.Tags["local_address"])
	assert.Equal(t, "192.168.255.10", availability.Tags["local_identifier"])
	assert.Equal(t, "54.240.205.233", availability.Tags["peer_identifier"])

	updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.Equal(t, map[string]int64{"received": 17, "sent": 4830}, updates.MultiValue)

	inTotal := requireMetricWithTags(t, metrics, "bgpPeerInTotalMessages", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.EqualValues(t, 430177, inTotal.Value)

	outTotal := requireMetricWithTags(t, metrics, "bgpPeerOutTotalMessages", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.EqualValues(t, 489554, outTotal.Value)

	transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.EqualValues(t, 7, transitions.Value)

	establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.EqualValues(t, 28063000, establishedTime.Value)

	updateElapsed := requireMetricWithTags(t, metrics, "bgpPeerInUpdateElapsedTime", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.EqualValues(t, 27997600, updateElapsed.Value)

	lastErrorCode := requireMetricWithTags(t, metrics, "bgpPeerLastErrorCode", map[string]string{
		"neighbor": "169.254.247.1",
	})
	assert.EqualValues(t, 0, lastErrorCode.Value)

	accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "169.254.247.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 27, accepted.Value)
	assert.Equal(t, "DIRECT CONNECT", accepted.Tags["peer_description"])

	received := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceived", map[string]string{
		"neighbor":                  "169.254.247.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 27, received.Value)

	advertised := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAdvertised", map[string]string{
		"neighbor":                  "169.254.247.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 69, advertised.Value)
}
