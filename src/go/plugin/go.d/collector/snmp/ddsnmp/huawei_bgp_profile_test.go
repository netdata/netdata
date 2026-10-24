// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func Test_HuaweiBGPProfileUsesTypedRows(t *testing.T) {
	matched := FindProfiles("1.3.6.1.4.1.2011.2.224.279", "", nil)
	var profile *Profile
	for _, candidate := range matched {
		if strings.HasSuffix(candidate.SourceFile, "huawei-routers.yaml") {
			profile = candidate
			break
		}
	}
	require.NotNil(t, profile, "expected huawei-routers.yaml profile to match")

	require.Len(t, profile.Definition.BGP, 5)
	assertNoMetricTable(t, profile, "hwBgpPeerTable")
	assertNoMetricTable(t, profile, "hwBgpPeerRouteTable")
	assertNoMetricTable(t, profile, "hwBgpPeerMessageTable")
	assertNoMetricTable(t, profile, "hwBgpPeerStatisticTable")
	assertNoVirtualMetric(t, profile, "bgpPeerAvailability")
	assertNoVirtualMetric(t, profile, "bgpPeerUpdates")
	assertNoVirtualMetric(t, profile, "bgpPeerFsmEstablishedTransitions")

	device := requireBGPRowByID(t, profile, "huawei-bgp-device-counts")
	assert.Equal(t, ddprofiledefinition.BGPRowKindDevice, device.Kind)
	assert.Equal(t, "huawei.hwBgpPeerSessionNum", device.Device.Peers.Symbol.Name)
	assert.Equal(t, "huawei.hwIBgpPeerSessionNum", device.Device.InternalPeers.Symbol.Name)
	assert.Equal(t, "huawei.hwEBgpPeerSessionNum", device.Device.ExternalPeers.Symbol.Name)

	peerFamily := requireBGPRowByID(t, profile, "huawei-bgp-peer-family")
	assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, peerFamily.Kind)
	assert.Equal(t, "hwBgpPeerTable", peerFamily.Table.Name)
	assert.Equal(t, "hwBgpPeerAddrFamilyTable", peerFamily.Identity.RoutingInstance.Table)
	assert.Equal(t, "huawei.hwBgpPeerRemoteAddr", peerFamily.Identity.Neighbor.Symbol.Name)
	assert.Equal(t, map[string]string{"1": "stop", "2": "start"}, peerFamily.Admin.Enabled.Symbol.Mapping.Items)
	assert.Equal(t, map[string]string{
		"1": "idle",
		"2": "connect",
		"3": "active",
		"4": "opensent",
		"5": "openconfirm",
		"6": "established",
	}, peerFamily.State.Symbol.Mapping.Items)

	rows := map[string]struct {
		id    string
		kind  ddprofiledefinition.BGPRowKind
		table string
	}{
		"routes": {
			id:    "huawei-bgp-peer-family-routes",
			kind:  ddprofiledefinition.BGPRowKindPeerFamily,
			table: "hwBgpPeerRouteTable",
		},
		"messages": {
			id:    "huawei-bgp-peer-family-messages",
			kind:  ddprofiledefinition.BGPRowKindPeerFamily,
			table: "hwBgpPeerMessageTable",
		},
		"statistics": {
			id:    "huawei-bgp-peer-statistics",
			kind:  ddprofiledefinition.BGPRowKindPeer,
			table: "hwBgpPeerStatisticTable",
		},
	}

	for name, tc := range rows {
		t.Run(name, func(t *testing.T) {
			row := requireBGPRowByID(t, profile, tc.id)
			assert.Equal(t, tc.kind, row.Kind)
			assert.Equal(t, tc.table, row.Table.Name)
		})
	}
}
