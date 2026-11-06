// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"errors"
	"strconv"
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
	pdus       []gosnmp.SnmpPDU
	failPrefix string
}

func newHuaweiIndexCollector(t *testing.T, input *huaweiWalkInput) *Collector {
	t.Helper()
	profile := mustLoadTypedBGPProfile(t, "_huawei-bgp-vpn", func(row ddprofiledefinition.BGPConfig) bool { return row.Table.OID != "" })
	_, handler := setupMockHandler(t)
	handler.EXPECT().Version().Return(gosnmp.Version2c).AnyTimes()
	handler.EXPECT().BulkWalkAll(gomock.Any()).DoAndReturn(func(oid string) ([]gosnmp.SnmpPDU, error) {
		if input.failPrefix != "" && strings.HasPrefix(oid, input.failPrefix) {
			return nil, errors.New("synthetic dependency WALK failure")
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
				assert.Equal(t, peer.description, row.Descriptors.Description)
				assert.Equal(t, strconv.Itoa(peer.version), row.Descriptors.BGPVersion)
				assert.Equal(t, strconv.Itoa(peer.remoteAS), row.Identity.RemoteAS)
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

func TestHuaweiIndexIdentity_ScopedDuplicateRows(t *testing.T) {
	for name, tc := range map[string]struct {
		otherInstance               bool
		conflictAS, conflictVersion bool
		wantStats                   int
		wantVersion                 string
	}{
		"same VRF AFIs agree":               {wantStats: 1, wantVersion: "4"},
		"same VRF AFIs disagree on AS":      {conflictAS: true, wantStats: 0},
		"same VRF AFIs disagree on version": {conflictVersion: true, wantStats: 1, wantVersion: ""},
		"other VRF conflict ignored":        {otherInstance: true, conflictAS: true, conflictVersion: true, wantStats: 2, wantVersion: "4"},
	} {
		t.Run(name, func(t *testing.T) {
			peer := huaweiIPv4TestPeer()
			other := peer
			other.familyIndex = "7.2.1.1.4.192.0.2.1"
			if tc.otherInstance {
				other.familyIndex = "9.2.1.1.4.192.0.2.1"
				other.statisticsIndex = "2.9.4.192.0.2.1"
				other.sessionIndex = "9.1.4.192.0.2.1"
				other.instance = "9"
				other.vrfName = "vrf-9"
			}
			if tc.conflictAS {
				other.remoteAS = 65099
			}
			if tc.conflictVersion {
				other.version = 5
			}
			pdus := huaweiIndexPDUs(peer, other)
			if !tc.otherInstance {
				// Only one statistics and session row exists for duplicate AFI sources.
				pdus = nil
				pdus = append(pdus, huaweiIndexPDUs(peer)...)
				for _, pdu := range huaweiIndexPDUs(other) {
					if !strings.HasPrefix(pdu.Name, huaweiTestRoot+"7.") && !strings.HasPrefix(pdu.Name, huaweiTestRoot+"8.") {
						pdus = append(pdus, pdu)
					}
				}
			}
			collector := newHuaweiIndexCollector(t, &huaweiWalkInput{pdus: pdus})
			rows := collectHuaweiIndexRows(t, collector)
			var stats []ddsnmp.BGPRow
			for _, row := range rows {
				if row.Table == "hwBgpPeerStatisticTable" {
					stats = append(stats, row)
				}
			}
			require.Len(t, stats, tc.wantStats)
			if tc.wantStats > 0 {
				row := requireBGPRowMatching(t, stats, func(row ddsnmp.BGPRow) bool { return row.Identity.RoutingInstance == "7" })
				assert.Equal(t, "65007", row.Identity.RemoteAS)
				assert.Equal(t, tc.wantVersion, row.Descriptors.BGPVersion)
				assert.Equal(t, peer.description, row.Descriptors.Description)
			}
		})
	}
}

func TestHuaweiIndexIdentity_MissingScopedTargetAndRecovery(t *testing.T) {
	for _, mode := range []string{"wrong VRF only", "absent target"} {
		t.Run(mode, func(t *testing.T) {
			peer := huaweiIPv4TestPeer()
			input := &huaweiWalkInput{}
			for _, pdu := range huaweiIndexPDUs(peer) {
				if strings.HasPrefix(pdu.Name, huaweiTestRoot+"2.") {
					if mode == "absent target" {
						continue
					}
					pdu.Name = strings.Replace(pdu.Name, ".7.1.1.1.4.", ".9.1.1.1.4.", 1)
				}
				input.pdus = append(input.pdus, pdu)
			}
			collector := newHuaweiIndexCollector(t, input)
			rows := collectHuaweiIndexRows(t, collector)
			for _, row := range rows {
				assert.NotEqual(t, "hwBgpPeerStatisticTable", row.Table, "missing scoped target must not supply required identity")
			}
			input.pdus = huaweiIndexPDUs(peer)
			rows = collectHuaweiIndexRows(t, collector)
			row := requireBGPRowMatching(t, rows, func(row ddsnmp.BGPRow) bool { return row.Table == "hwBgpPeerStatisticTable" })
			assert.Equal(t, "65007", row.Identity.RemoteAS)
		})
	}
}

func TestHuaweiIndexIdentity_OptionalDescriptionAvailability(t *testing.T) {
	for _, mode := range []string{"absent", "empty", "noSuch", "WALK error"} {
		t.Run(mode, func(t *testing.T) {
			peer := huaweiIPv4TestPeer()
			input := &huaweiWalkInput{pdus: huaweiIndexPDUs(peer)}
			collector := newHuaweiIndexCollector(t, input)
			healthy := collectHuaweiIndexRows(t, collector)
			require.Len(t, healthy, 4)
			switch mode {
			case "absent":
				input.pdus = input.pdus[:len(input.pdus)-1]
			case "empty":
				input.pdus[len(input.pdus)-1] = createStringPDU(huaweiTestDescription+"."+peer.sessionIndex, "")
			case "noSuch":
				input.pdus[len(input.pdus)-1] = createPDU(huaweiTestDescription+"."+peer.sessionIndex, gosnmp.NoSuchInstance, nil)
			case "WALK error":
				input.failPrefix = huaweiTestRoot + "8"
			}
			rows := collectHuaweiIndexRows(t, collector)
			require.Len(t, rows, 4)
			for i := range healthy {
				healthy[i].Descriptors.Description = ""
			}
			assert.ElementsMatch(t, healthy, rows, "optional description loss must preserve every identity and signal")
			input.failPrefix = ""
			input.pdus = huaweiIndexPDUs(peer)
			rows = collectHuaweiIndexRows(t, collector)
			require.Len(t, rows, 4)
			for _, row := range rows {
				assert.Equal(t, peer.description, row.Descriptors.Description)
			}
		})
	}
}

func TestHuaweiIndexIdentity_SharedRequiredDependencyFailure(t *testing.T) {
	peer := huaweiIPv4TestPeer()
	input := &huaweiWalkInput{pdus: huaweiIndexPDUs(peer), failPrefix: huaweiTestRoot + "2"}
	collector := newHuaweiIndexCollector(t, input)
	results, err := collector.Collect()
	require.NoError(t, err, "the collector reports per-profile BGP failures without failing ordinary collection")
	require.Len(t, results, 1)
	require.Empty(t, results[0].BGPRows, "peer table is shared by required remote AS and optional version")
	input.failPrefix = ""
	require.Len(t, collectHuaweiIndexRows(t, collector), 4)
}
