// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"fmt"
	"strings"
	"testing"

	"github.com/golang/mock/gomock"
	"github.com/gosnmp/gosnmp"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

const huaweiTestRoot = "1.3.6.1.4.1.2011.5.25.177.1.1."
const huaweiTestDescription = huaweiTestRoot + "8.1.11"

// These independently specified suffixes follow HUAWEI-BGP-VPN-MIB INDEX clauses.
// The readable session-extension description and its [instance,type,length,address]
// suffix are also observed in LibreNMS vrp_ce12804-withvrf.snmprec at revision
// b787b2e9d38a87f39d95a6a077c9382261262336. Values below are synthetic.
type huaweiIndexPeer struct {
	familyIndex, statisticsIndex, sessionIndex string
	neighbor, instance, vrfName, description   string
	remoteAS, version                          int
}

func huaweiIPv4TestPeer() huaweiIndexPeer {
	return huaweiIndexPeer{
		familyIndex: "7.1.1.1.4.192.0.2.1", statisticsIndex: "1.7.4.192.0.2.1", sessionIndex: "7.1.4.192.0.2.1",
		neighbor: "192.0.2.1", instance: "7", vrfName: "vrf-7", description: "synthetic-blue-peer", remoteAS: 65007, version: 4,
	}
}

func huaweiIndexPDUs(peers ...huaweiIndexPeer) []gosnmp.SnmpPDU {
	var pdus []gosnmp.SnmpPDU
	for _, peer := range peers {
		for _, column := range []struct {
			oid string
			pdu gosnmp.SnmpPDU
		}{
			{huaweiTestRoot + "1.1.6", createStringPDU("", peer.vrfName)},
			{huaweiTestRoot + "2.1.1", createGauge32PDU("", uint(peer.version))},
			{huaweiTestRoot + "2.1.2", createGauge32PDU("", uint(peer.remoteAS))},
			{huaweiTestRoot + "2.1.4", createStringPDU("", peer.neighbor)},
			{huaweiTestRoot + "2.1.5", createIntegerPDU("", 6)},
			{huaweiTestRoot + "3.1.1", createCounter32PDU("", 11)},
			{huaweiTestRoot + "4.1.1", createCounter32PDU("", 22)},
		} {
			column.pdu.Name = column.oid + "." + peer.familyIndex
			pdus = append(pdus, column.pdu)
		}
		pdus = append(pdus, createCounter32PDU(huaweiTestRoot+"7.1.4."+peer.statisticsIndex, 33))
		pdus = append(pdus, createStringPDU(huaweiTestDescription+"."+peer.sessionIndex, peer.description))
	}
	return pdus
}

type huaweiWalkInput struct {
	pdus            []gosnmp.SnmpPDU
	forbiddenPrefix string
}

func newHuaweiIndexCollector(t *testing.T, input *huaweiWalkInput) *Collector {
	t.Helper()
	profile := mustLoadTypedBGPProfile(t, "_huawei-bgp-vpn", func(row ddprofiledefinition.BGPConfig) bool { return row.Table.OID != "" })
	_, handler := setupMockHandler(t)
	handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	handler.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) {
		if input.forbiddenPrefix != "" {
			require.False(t, strings.HasPrefix(oid, input.forbiddenPrefix), "unproved dependency WALK: %s", oid)
		}
		return snmprecPDUsWithPrefix(input.pdus, oid+"."), nil
	}).AnyTimes()
	return New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
}

func collectHuaweiIndexRows(t *testing.T, collector *Collector) []ddsnmp.BGPRow {
	t.Helper()
	result, err := collector.Collect()
	require.NoError(t, err)
	require.Len(t, result, 1)
	return result[0].BGPRows
}

func TestHuaweiIndexIdentity_AllLoadedRowGroups(t *testing.T) {
	ipv4 := huaweiIPv4TestPeer()
	red := huaweiIndexPeer{
		familyIndex: "9.1.1.1.4.192.0.2.1", statisticsIndex: "1.9.4.192.0.2.1", sessionIndex: "9.1.4.192.0.2.1",
		neighbor: "192.0.2.1", instance: "9", vrfName: "vrf-9", description: "synthetic-red-peer", remoteAS: 65009, version: 5,
	}
	ipv6 := huaweiIndexPeer{
		familyIndex:     "7.2.1.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1",
		statisticsIndex: "2.7.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1",
		sessionIndex:    "7.2.16.32.1.13.184.0.0.0.0.0.0.0.0.0.0.0.1",
		neighbor:        "2001:db8::1", instance: "7", vrfName: "vrf-7", description: "synthetic-ipv6-peer", remoteAS: 65017, version: 4,
	}
	input := &huaweiWalkInput{pdus: huaweiIndexPDUs(ipv4, red, ipv6)}
	collector := newHuaweiIndexCollector(t, input)
	rows := collectHuaweiIndexRows(t, collector)
	require.Len(t, rows, 12)
	for _, peer := range []huaweiIndexPeer{ipv4, red, ipv6} {
		for _, table := range []string{"hwBgpPeerTable", "hwBgpPeerRouteTable", "hwBgpPeerMessageTable", "hwBgpPeerStatisticTable"} {
			t.Run(peer.description+"/"+table, func(t *testing.T) {
				wantInstance := peer.vrfName
				if table == "hwBgpPeerStatisticTable" {
					wantInstance = peer.instance
				}
				row := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool {
					return row.Table == table && row.Identity.Neighbor == peer.neighbor && row.Identity.RoutingInstance == wantInstance
				})
				assert.Empty(t, row.Descriptors.Description, "session-extension VPN identity is not a proven family-instance join")
				if table == "hwBgpPeerStatisticTable" {
					assert.NotEmpty(t, row.Identity.ProcessID)
					assert.Empty(t, row.Descriptors.BGPVersion)
					assert.Empty(t, row.Identity.RemoteAS)
				} else {
					assert.Empty(t, row.Identity.ProcessID)
					assert.Equal(t, fmt.Sprint(peer.version), row.Descriptors.BGPVersion)
					assert.Equal(t, fmt.Sprint(peer.remoteAS), row.Identity.RemoteAS)
				}
				switch table {
				case "hwBgpPeerTable":
					assert.True(t, row.State.Has)
				case "hwBgpPeerRouteTable":
					assert.EqualValues(t, 11, row.Routes.Total.Received.Value)
				case "hwBgpPeerMessageTable":
					assert.EqualValues(t, 22, row.Traffic.Messages.Received.Value)
				case "hwBgpPeerStatisticTable":
					assert.EqualValues(t, 33, row.Transitions.Established.Value)
				}
			})
		}
	}
}

// Target tables cannot prove process ownership, even when numeric instance IDs
// and peer addresses happen to match. Statistics must not depend on them.
func TestHuaweiIndexIdentity_StatisticsIndependentOfOtherTables(t *testing.T) {
	for name, tc := range map[string]struct{ pdus []gosnmp.SnmpPDU }{
		"absent":                {},
		"matching numeric IDs":  {pdus: huaweiIndexPDUs(huaweiIPv4TestPeer())},
		"different numeric IDs": {pdus: huaweiIndexPDUs(huaweiIndexPeer{familyIndex: "99.1.1.1.4.192.0.2.1", sessionIndex: "99.1.4.192.0.2.1", neighbor: "192.0.2.1", remoteAS: 65099, version: 5, description: "unrelated"})},
	} {
		t.Run(name, func(t *testing.T) {
			profile := mustLoadTypedBGPProfile(t, "_huawei-bgp-vpn", func(row ddprofiledefinition.BGPConfig) bool { return row.ID == "huawei-bgp-peer-statistics" })
			pdus := append([]gosnmp.SnmpPDU{}, tc.pdus...)
			for process := 0; process < 3; process++ {
				for column := 4; column <= 9; column++ {
					pdus = append(pdus, createCounter32PDU(fmt.Sprintf("%s7.1.%d.%d.7.4.192.0.2.1", huaweiTestRoot, column, process), uint(process*100+column)))
				}
			}
			_, handler := setupMockHandler(t)
			handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
			handler.EXPECT().BulkWalkAll(huaweiTestRoot + "7").DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) { return snmprecPDUsWithPrefix(pdus, oid+"."), nil }).AnyTimes()
			// No WALK for peer/family/session tables is allowed by this expectation.
			collector := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
			rows := collectHuaweiIndexRows(t, collector)
			require.Len(t, rows, 3)
			for process := 0; process < 3; process++ {
				row := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool { return row.Identity.ProcessID == fmt.Sprint(process) })
				assert.Equal(t, "7", row.Identity.RoutingInstance)
				assert.Equal(t, "192.0.2.1", row.Identity.Neighbor)
				assert.Empty(t, row.Identity.RemoteAS)
				assert.Empty(t, row.Descriptors.BGPVersion)
				assert.Empty(t, row.Descriptors.Description)
				for i, value := range []ddsnmp.BGPInt64{row.Transitions.Established, row.Transitions.Down, row.Traffic.Updates.Received, row.Traffic.Updates.Sent, row.Traffic.Messages.Received, row.Traffic.Messages.Sent} {
					require.True(t, value.Has)
					assert.EqualValues(t, process*100+i+4, value.Value)
				}
			}
		})
	}
}

func TestHuaweiIndexIdentity_UnprovedSessionDescriptionIsNeverRead(t *testing.T) {
	peer := huaweiIPv4TestPeer()
	input := &huaweiWalkInput{pdus: huaweiIndexPDUs(peer), forbiddenPrefix: huaweiTestRoot + "8"}
	rows := collectHuaweiIndexRows(t, newHuaweiIndexCollector(t, input))
	require.Len(t, rows, 4)
	for _, row := range rows {
		assert.Empty(t, row.Descriptors.Description)
	}
}

func TestHuaweiIndexIdentity_ConfiguredProcessCannotFallBackToAS(t *testing.T) {
	profile := mustLoadTypedBGPProfile(t, "_huawei-bgp-vpn", func(row ddprofiledefinition.BGPConfig) bool { return row.ID == "huawei-bgp-peer-statistics" })
	profile.Definition.BGP[0].Identity.ProcessID = ddprofiledefinition.BGPValueConfig{Index: 100}
	profile.Definition.BGP[0].Identity.RemoteAS = ddprofiledefinition.BGPValueConfig{Value: "65001"}
	_, handler := setupMockHandler(t)
	handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	handler.EXPECT().BulkWalkAll(huaweiTestRoot+"7").Return([]gosnmp.SnmpPDU{createCounter32PDU(huaweiTestRoot+"7.1.4.0.7.4.192.0.2.1", 11)}, nil).AnyTimes()
	collector := New(Config{SnmpClient: handler, Profiles: []*ddsnmp.Profile{profile}, Log: logger.New()})
	assert.Empty(t, collectHuaweiIndexRows(t, collector))
}
