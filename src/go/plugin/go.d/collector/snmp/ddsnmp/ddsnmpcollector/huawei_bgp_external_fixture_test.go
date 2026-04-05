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

func TestCollector_Collect_HuaweiBGP_FromLibreNMSFixture(t *testing.T) {
	profile := matchedProfileByFile(t, "1.3.6.1.4.1.2011.2.224.279", "huawei-routers.yaml")
	filterProfileForAlertSurface(profile, map[string][]string{
		"hwBgpPeerTable": {
			"huawei.hwBgpPeerAdminStatus",
			"huawei.hwBgpPeerState",
			"huawei.hwBgpPeerFsmEstablishedCounter",
			"huawei.hwBgpPeerLastErrorCode",
			"huawei.hwBgpPeerLastErrorSubcode",
		},
		"hwBgpPeerRouteTable": {
			"huawei.hwBgpPeerPrefixRcvCounter",
			"huawei.hwBgpPeerPrefixActiveCounter",
			"huawei.hwBgpPeerPrefixAdvCounter",
		},
		"hwBgpPeerStatisticTable": {
			"huawei.hwBgpPeerDownCounts",
			"huawei.hwBgpPeerInUpdateMsgs",
			"huawei.hwBgpPeerOutUpdateMsgs",
			"huawei.hwBgpPeerInTotalMsgs",
			"huawei.hwBgpPeerOutTotalMsgs",
		},
	}, []string{"bgpPeerAvailability", "bgpPeerFsmEstablishedTransitions", "bgpPeerUpdates"})

	pdus := loadSnmprecPDUs(t, "librenms/huawei_vrp_ne8000_bgp_peer_table.snmprec")

	ctrl, mockHandler := setupMockHandler(t)
	defer ctrl.Finish()

	setProfileWalkExpectations(t, mockHandler, profile, map[string][]gosnmp.SnmpPDU{
		"hwBgpPeerAddrFamilyTable": snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2011.5.25.177.1.1.1."),
		"hwBgpPeerTable":           snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2011.5.25.177.1.1.2."),
		"hwBgpPeerRouteTable":      snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2011.5.25.177.1.1.3."),
		"hwBgpPeerStatisticTable":  snmprecPDUsWithPrefix(pdus, "1.3.6.1.4.1.2011.5.25.177.1.1.7."),
	})

	collector := New(Config{
		SnmpClient:  mockHandler,
		Profiles:    []*ddsnmp.Profile{profile},
		Log:         logger.New(),
		SysObjectID: "1.3.6.1.4.1.2011.2.224.279",
	})

	results, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, results, 1)

	metrics := stripProfilePointers(results[0].Metrics)

	availability := requireMetricWithTags(t, metrics, "bgpPeerAvailability", map[string]string{
		"routing_instance":          "Public",
		"neighbor":                  "10.45.2.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.Equal(t, map[string]int64{"admin_enabled": 1, "established": 1}, availability.MultiValue)
	assert.Equal(t, "26479", availability.Tags["remote_as"])

	updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"routing_instance":          "0",
		"neighbor":                  "10.45.2.2",
		"address_family":            "all",
		"subsequent_address_family": "all",
		"neighbor_address_type":     "ipv4",
	})
	assert.Equal(t, map[string]int64{"received": 70063, "sent": 971}, updates.MultiValue)
	assert.Equal(t, "26479", updates.Tags["remote_as"])

	transitions := requireMetricWithTags(t, metrics, "bgpPeerFsmEstablishedTransitions", map[string]string{
		"routing_instance":          "Public",
		"neighbor":                  "10.45.2.2",
		"address_family":            "ipv4",
		"subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 2, transitions.Value)

	lastErrorCode := requireMetricWithTags(t, metrics, "huawei.hwBgpPeerLastErrorCode", map[string]string{
		"_routing_instance": "Public",
		"_neighbor":         "10.45.2.2",
	})
	assert.EqualValues(t, 5, lastErrorCode.Value)

	receivedPrefixes := requireMetricWithTags(t, metrics, "huawei.hwBgpPeerPrefixRcvCounter", map[string]string{
		"_routing_instance":          "Public",
		"_neighbor":                  "10.45.2.2",
		"_address_family":            "ipv4",
		"_subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 13970, receivedPrefixes.Value)

	inTotal := requireMetricWithTags(t, metrics, "huawei.hwBgpPeerInTotalMsgs", map[string]string{
		"_routing_instance":          "0",
		"_neighbor":                  "10.45.2.2",
		"_address_family":            "all",
		"_subsequent_address_family": "all",
	})
	assert.EqualValues(t, 99928, inTotal.Value)

	ipv6Updates := requireMetricWithTags(t, metrics, "bgpPeerUpdates", map[string]string{
		"routing_instance":          "0",
		"neighbor":                  "2001:12F8::223:253",
		"address_family":            "all",
		"subsequent_address_family": "all",
		"neighbor_address_type":     "ipv6",
	})
	assert.Equal(t, map[string]int64{"received": 3318661, "sent": 6}, ipv6Updates.MultiValue)
	assert.Equal(t, "26162", ipv6Updates.Tags["remote_as"])

	ipv6ReceivedPrefixes := requireMetricWithTags(t, metrics, "huawei.hwBgpPeerPrefixRcvCounter", map[string]string{
		"_routing_instance":          "PublicV6",
		"_neighbor":                  "2001:12F8::223:253",
		"_address_family":            "ipv6",
		"_subsequent_address_family": "unicast",
	})
	assert.EqualValues(t, 0, ipv6ReceivedPrefixes.Value)
}
