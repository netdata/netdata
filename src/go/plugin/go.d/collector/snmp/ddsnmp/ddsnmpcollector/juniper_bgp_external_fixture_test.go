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

func TestCollector_Collect_JuniperBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.2636.1.1.1.2.57", "juniper-mx.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"jnxBgpM2PeerTable": {
			"bgpPeerAdminStatus",
			"bgpPeerState",
		},
		"jnxBgpM2PeerEventTimesTable": {
			"bgpPeerFsmEstablishedTime",
			"bgpPeerInUpdateElapsedTime",
		},
		"jnxBgpM2PeerCountersTable": {
			"bgpPeerInUpdates",
			"bgpPeerOutUpdates",
			"bgpPeerInTotalMessages",
			"bgpPeerOutTotalMessages",
		},
		"jnxBgpM2PrefixCountersTable": {
			"bgpPeerPrefixesReceived",
			"bgpPeerPrefixesAccepted",
			"bgpPeerPrefixesAdvertised",
			"bgpPeerPrefixesActive",
		},
	}, []string{"bgpPeerAvailability", "bgpPeerUpdates"})

	pdus := loadSnmprecPDUs(t, "librenms/juniper_mx_bgp_peer_table.snmprec")

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"jnxBgpM2PeerTable":           snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.1.1."),
		"jnxBgpM2PeerEventTimesTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.4.1."),
		"jnxBgpM2PeerCountersTable":   snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.6.1."),
		"jnxBgpM2PrefixCountersTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.6.2."),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.57",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "0",
		"neighbor":         "192.168.99.25",
		"remote_as":        "64513",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
	assert.Equal(t, "192.168.99.24", availability.Tags["local_address"])

	updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"routing_instance": "0",
		"neighbor":         "192.168.99.25",
	})
	assert.Equal(t, map[string]int64{"received": 56, "sent": 1}, updates.MultiValue)
	assert.Equal(t, "192.168.99.24", updates.Tags["local_address"])

	inTotal := requireMetricWithTags(t, metrics, "bgpPeerInTotalMessages", map[string]string{
		"neighbor": "192.168.99.25",
	})
	assert.EqualValues(t, 13199, inTotal.Value)

	outTotal := requireMetricWithTags(t, metrics, "bgpPeerOutTotalMessages", map[string]string{
		"neighbor": "192.168.99.25",
	})
	assert.EqualValues(t, 13351, outTotal.Value)

	establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"neighbor": "192.168.99.25",
	})
	assert.EqualValues(t, 361781, establishedTime.Value)

	updateElapsed := requireMetricWithTags(t, metrics, "bgpPeerInUpdateElapsedTime", map[string]string{
		"neighbor": "192.168.99.25",
	})
	assert.EqualValues(t, 49798, updateElapsed.Value)

	unicastAccepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "192.168.99.25",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 20, unicastAccepted.Value)
	assert.Equal(t, "0", unicastAccepted.Tags["routing_instance"])

	multicastReceived := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceived", map[string]string{
		"neighbor":                  "192.168.99.25",
		"address_family":            "ipv4",
		"subsequent_address_family": "multicast",
	})
	assert.EqualValues(t, 4, multicastReceived.Value)

	vplsReceived := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceived", map[string]string{
		"neighbor":                  "192.168.99.25",
		"address_family":            "l2vpn",
		"subsequent_address_family": "vpls",
	})
	assert.EqualValues(t, 0, vplsReceived.Value)

	unicastAdvertised := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAdvertised", map[string]string{
		"neighbor":                  "192.168.99.25",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 0, unicastAdvertised.Value)

	unicastActive := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
		"neighbor":                  "192.168.99.25",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 6, unicastActive.Value)
}

func TestCollector_Collect_JuniperVMXBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.2636.1.1.1.2.108", "juniper-mx.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"jnxBgpM2PeerTable": {
			"bgpPeerAdminStatus",
			"bgpPeerState",
		},
		"jnxBgpM2PeerEventTimesTable": {
			"bgpPeerFsmEstablishedTime",
			"bgpPeerInUpdateElapsedTime",
		},
		"jnxBgpM2PeerCountersTable": {
			"bgpPeerInUpdates",
			"bgpPeerOutUpdates",
			"bgpPeerInTotalMessages",
			"bgpPeerOutTotalMessages",
		},
		"jnxBgpM2PrefixCountersTable": {
			"bgpPeerPrefixesReceived",
			"bgpPeerPrefixesAccepted",
			"bgpPeerPrefixesRejected",
			"bgpPeerPrefixesAdvertised",
			"bgpPeerPrefixesActive",
		},
	}, []string{"bgpPeerAvailability", "bgpPeerUpdates"})

	pdus := loadSnmprecPDUs(t, "librenms/juniper_vmx_bgp_peer_table.snmprec")

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"jnxBgpM2PeerTable":           snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.1.1."),
		"jnxBgpM2PeerErrorsTable":     snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.2.1."),
		"jnxBgpM2PeerEventTimesTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.4.1."),
		"jnxBgpM2PeerCountersTable":   snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.6.1."),
		"jnxBgpM2PrefixCountersTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2636.5.1.1.2.6.2."),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.108",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	firstAvailability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "9",
		"neighbor":         "10.1.1.1",
		"remote_as":        "62371",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, firstAvailability.MultiValue)
	assert.Equal(t, "10.1.1.2", firstAvailability.Tags["local_address"])
	assert.Equal(t, "172.16.0.10", firstAvailability.Tags["peer_identifier"])

	secondAvailability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "9",
		"neighbor":         "10.1.1.5",
		"remote_as":        "62371",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, secondAvailability.MultiValue)
	assert.Equal(t, "10.1.1.6", secondAvailability.Tags["local_address"])
	assert.Equal(t, "172.16.0.11", secondAvailability.Tags["peer_identifier"])

	firstUpdates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"routing_instance": "9",
		"neighbor":         "10.1.1.1",
	})
	assert.Equal(t, map[string]int64{"received": 2, "sent": 2}, firstUpdates.MultiValue)

	secondUpdates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"routing_instance": "9",
		"neighbor":         "10.1.1.5",
	})
	assert.Equal(t, map[string]int64{"received": 2, "sent": 2}, secondUpdates.MultiValue)

	firstInTotal := requireMetricWithTags(t, metrics, "bgpPeerInTotalMessages", map[string]string{
		"neighbor": "10.1.1.1",
	})
	assert.EqualValues(t, 468, firstInTotal.Value)

	secondOutTotal := requireMetricWithTags(t, metrics, "bgpPeerOutTotalMessages", map[string]string{
		"neighbor": "10.1.1.5",
	})
	assert.EqualValues(t, 477, secondOutTotal.Value)

	firstEstablishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"neighbor": "10.1.1.1",
	})
	assert.EqualValues(t, 12859, firstEstablishedTime.Value)

	secondUpdateElapsed := requireMetricWithTags(t, metrics, "bgpPeerInUpdateElapsedTime", map[string]string{
		"neighbor": "10.1.1.5",
	})
	assert.EqualValues(t, 12757, secondUpdateElapsed.Value)

	firstAccepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "10.1.1.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 1, firstAccepted.Value)
	assert.Equal(t, "9", firstAccepted.Tags["routing_instance"])

	secondRejected := requireMetricWithTags(t, metrics, "bgpPeerPrefixesRejected", map[string]string{
		"neighbor":                  "10.1.1.5",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 0, secondRejected.Value)

	firstAdvertised := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAdvertised", map[string]string{
		"neighbor":                  "10.1.1.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 2, firstAdvertised.Value)

	secondActive := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
		"neighbor":                  "10.1.1.5",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 1, secondActive.Value)
}
