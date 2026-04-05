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

func TestCollector_Collect_CiscoBgpPeer2Prefixes_FromLibreNMSFixture(t *testing.T) {
	metrics := collectCiscoPeer2PrefixMetricsFromFixture(t,
		"1.3.6.1.4.1.9.1.1639",
		"cisco-asr.yaml",
		"librenms/cisco_iosxr_asr9001_bgp_peer2_prefix_table.snmprec",
	)

	accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "10.246.0.3",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 5, accepted.Value)
	assert.Equal(t, "VPNv4 Unicast", accepted.Tags["address_family_name"])
	assert.Equal(t, "10.246.0.1", accepted.Tags["local_address"])
	assert.Equal(t, "65056", accepted.Tags["remote_as"])

	adminLimit := requireMetricWithTags(t, metrics, "bgpPeerPrefixAdminLimit", map[string]string{
		"neighbor":                  "10.246.0.3",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 2097152, adminLimit.Value)

	threshold := requireMetricWithTags(t, metrics, "bgpPeerPrefixThreshold", map[string]string{
		"neighbor":                  "10.246.0.3",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 75, threshold.Value)

	advertised := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAdvertised", map[string]string{
		"neighbor":                  "10.246.0.3",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 25, advertised.Value)

	withdrawn := requireMetricWithTags(t, metrics, "bgpPeerPrefixesWithdrawn", map[string]string{
		"neighbor":                  "10.246.0.3",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 7, withdrawn.Value)

	idleAccepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "10.246.0.4",
		"address_family":            "ipv4",
		"subsequent_address_family": "vpn",
	})
	assert.EqualValues(t, 0, idleAccepted.Value)
	assert.Equal(t, "0.0.0.0", idleAccepted.Tags["local_address"])
	assert.Equal(t, "65056", idleAccepted.Tags["remote_as"])
}

func TestCollector_Collect_CiscoBgpPeer2Prefixes_FromNCS540LibreNMSFixture(t *testing.T) {
	metrics := collectCiscoPeer2PrefixMetricsFromFixture(t,
		"1.3.6.1.4.1.9.1.2984",
		"cisco.yaml",
		"librenms/cisco_iosxr_ncs540_bgp_peer2_prefix_table.snmprec",
	)

	accepted := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAccepted", map[string]string{
		"neighbor":                  "10.255.9.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 30, accepted.Value)
	assert.Equal(t, "IPv4 Unicast", accepted.Tags["address_family_name"])
	assert.Equal(t, "10.255.9.31", accepted.Tags["local_address"])
	assert.Equal(t, "65011", accepted.Tags["remote_as"])

	adminLimit := requireMetricWithTags(t, metrics, "bgpPeerPrefixAdminLimit", map[string]string{
		"neighbor":                  "10.255.9.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 4294967295, adminLimit.Value)

	threshold := requireMetricWithTags(t, metrics, "bgpPeerPrefixThreshold", map[string]string{
		"neighbor":                  "10.255.9.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 75, threshold.Value)

	clearThreshold := requireMetricWithTags(t, metrics, "bgpPeerPrefixClearThreshold", map[string]string{
		"neighbor":                  "10.255.9.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 75, clearThreshold.Value)

	advertised := requireMetricWithTags(t, metrics, "bgpPeerPrefixesAdvertised", map[string]string{
		"neighbor":                  "10.255.9.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 8, advertised.Value)

	withdrawn := requireMetricWithTags(t, metrics, "bgpPeerPrefixesWithdrawn", map[string]string{
		"neighbor":                  "10.255.9.1",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 0, withdrawn.Value)
}

func collectCiscoPeer2PrefixMetricsFromFixture(t *testing.T, sysObjectID, profileFile, fixturePath string) []ddsnmp.Metric {
	t.Helper()

	profile := matchedProfileByFile(t, sysObjectID, profileFile)
	filterProfileForAlertSurface(profile, map[string][]string{
		"cbgpPeer2AddrFamilyPrefixTable": {
			"bgpPeerPrefixesAccepted",
			"bgpPeerPrefixAdminLimit",
			"bgpPeerPrefixThreshold",
			"bgpPeerPrefixClearThreshold",
			"bgpPeerPrefixesAdvertised",
			"bgpPeerPrefixesWithdrawn",
		},
	}, nil)

	pdus := loadSnmprecPDUs(t, fixturePath)

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"cbgpPeer2AddrFamilyPrefixTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.9.9.187.1.2.8."),
		"cbgpPeer2Table":                 snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.9.9.187.1.2.5."),
		"cbgpPeer2AddrFamilyTable":       snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.9.9.187.1.2.7."),
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
