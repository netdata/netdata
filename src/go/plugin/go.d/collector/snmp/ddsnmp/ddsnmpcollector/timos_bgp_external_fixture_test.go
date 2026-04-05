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

func TestCollector_Collect_TiMOSBGP_FromLibreNMSFixture(t *testing.T) {
	metrics := collectTiMOSMetricsFromFixture(t, "1.3.6.1.4.1.6527.1.3.17", "librenms/timos_7750_bgp_peer_table.snmprec")

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "Base",
		"neighbor":         "192.168.119.2",
		"remote_as":        "12345",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
	assert.Equal(t, "router.name", availability.Tags["peer_description"])

	establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"routing_instance": "Base",
		"neighbor":         "192.168.119.2",
	})
	assert.EqualValues(t, 4656225, establishedTime.Value)

	ipv4Received := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceivedTotal", map[string]string{
		"routing_instance":          "Base",
		"neighbor":                  "192.168.119.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 13, ipv4Received.Value)
	assert.Equal(t, "ipv4 unicast", ipv4Received.Tags["address_family_name"])
	assert.Equal(t, "router.name", ipv4Received.Tags["peer_description"])

	ipv4Active := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
		"neighbor":                  "192.168.119.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 6, ipv4Active.Value)

	vpnv4Active := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
		"neighbor":                  "192.168.119.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 25, vpnv4Active.Value)

	vpnv6Received := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceivedTotal", map[string]string{
		"neighbor":                  "192.168.119.2",
		"address_family":            "ipv6",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 18, vpnv6Received.Value)

	vplsReceived := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceivedTotal", map[string]string{
		"neighbor":                  "192.168.119.2",
		"address_family":            "l2vpn",
		"subsequent_address_family": "vpls",
	})
	assert.EqualValues(t, 2, vplsReceived.Value)

	evpnRejected := requireMetricWithTags(t, metrics, "bgpPeerPrefixesRejectedTotal", map[string]string{
		"neighbor":                  "192.168.119.2",
		"address_family":            "l2vpn",
		"subsequent_address_family": "evpn",
	})
	assert.EqualValues(t, 0, evpnRejected.Value)
}

func TestCollector_Collect_TiMOSIXRSBGP_FromLibreNMSFixture(t *testing.T) {
	metrics := collectTiMOSMetricsFromFixture(t, "1.3.6.1.4.1.6527.1.3.23", "librenms/timos_ixr_s_bgp_peer_table.snmprec")

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance": "Base",
		"neighbor":         "172.30.0.17",
		"remote_as":        "393227",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)

	establishedTime := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTime", map[string]string{
		"routing_instance": "Base",
		"neighbor":         "172.30.0.17",
	})
	assert.EqualValues(t, 1800, establishedTime.Value)

	ipv4Received := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceivedTotal", map[string]string{
		"neighbor":                  "172.30.0.17",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 0, ipv4Received.Value)
	assert.Equal(t, "ipv4 unicast", ipv4Received.Tags["address_family_name"])

	vpnv4Received := requireMetricWithTags(t, metrics, "bgpPeerPrefixesReceivedTotal", map[string]string{
		"neighbor":                  "172.30.0.17",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 184, vpnv4Received.Value)

	vpnv4Advertised := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAdvertisedTotal", map[string]string{
		"neighbor":                  "172.30.0.17",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 1, vpnv4Advertised.Value)

	vpnv4Active := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
		"neighbor":                  "172.30.0.17",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 174, vpnv4Active.Value)

	secondPeerVPNv4Active := requireMetricWithTags(t, metrics, "bgpPeerPrefixesActive", map[string]string{
		"neighbor":                  "172.30.0.18",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 9, secondPeerVPNv4Active.Value)

	evpnRejected := requireMetricWithTags(t, metrics, "bgpPeerPrefixesRejectedTotal", map[string]string{
		"neighbor":                  "172.30.0.17",
		"address_family":            "l2vpn",
		"subsequent_address_family": "evpn",
	})
	assert.EqualValues(t, 0, evpnRejected.Value)
}

func collectTiMOSMetricsFromFixture(t *testing.T, sysObjectID, fixturePath string) []ddsnmp.Metric {
	t.Helper()

	profile := matchedProfileByFile(t, sysObjectID, "nokia-service-router-os.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"tBgpPeerNgTable": {
			"bgpPeerAdminStatus",
			"bgpPeerState",
		},
		"tBgpPeerNgOperTable": {
			"bgpPeerFsmEstablishedTime",
			"bgpPeerPrefixesReceivedTotal",
			"bgpPeerPrefixesAdvertisedTotal",
			"bgpPeerPrefixesActive",
			"bgpPeerPrefixesRejectedTotal",
		},
	}, []string{"bgpPeerAvailability"})

	pdus := loadSnmprecPDUs(t, fixturePath)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"vRtrConfTable":       snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.6527.3.1.2.3."),
		"tBgpPeerNgTable":     snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.6527.3.1.2.14.4.7."),
		"tBgpPeerNgOperTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.6527.3.1.2.14.4.8."),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: sysObjectID,
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	return stripProfilePointers(results[0].Metrics)
}
