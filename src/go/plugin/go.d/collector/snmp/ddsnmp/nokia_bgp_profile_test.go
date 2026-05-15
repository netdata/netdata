// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func Test_NokiaBGPProfileMergedIntoNokiaSROS(t *testing.T) {
	profile := mustLoadNokiaSROSBGPProfile(t)

	require.Len(t, profile.Definition.BGP, 7)
	assert.True(t, profile.HasExtension("_nokia-timetra-bgp.yaml"))
	assert.False(t, profile.HasExtension("_std-bgp4-mib.yaml"))

	peer := requireBGPRowByID(t, profile, "nokia-timos-bgp-peer")
	assert.Equal(t, "_nokia-timetra-bgp.yaml", peer.OriginProfileID)
	assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, peer.Kind)
	assert.Equal(t, "tBgpPeerNgTable", peer.Table.Name)
	assert.Equal(t, "1.3.6.1.4.1.6527.3.1.2.14.4.7", peer.Table.OID)
	assert.Equal(t, "vRtrConfTable", peer.Identity.RoutingInstance.Table)
	assert.Equal(t, "tBgpPeerNgAddress", peer.Identity.Neighbor.Name)
	assert.Equal(t, "tBgpPeerNgPeerAS4Byte", peer.Identity.RemoteAS.Symbol.Name)
	assert.Equal(t, "tBgpPeerNgOperTable", peer.Connection.EstablishedUptime.Table)
	assert.Equal(t, "tBgpPeerNgOperTable", peer.Traffic.Updates.Received.Table)
	assert.Equal(t, "tBgpPeerNgOperTable", peer.LastError.Code.Table)
	assert.Equal(t, map[string]string{"1": "stop", "2": "start"}, peer.Admin.Enabled.Symbol.Mapping.Items)
	assert.Equal(t, map[string]string{
		"1": "idle",
		"2": "connect",
		"3": "active",
		"4": "opensent",
		"5": "openconfirm",
		"6": "established",
	}, peer.State.Symbol.Mapping.Items)

	values := map[string]struct {
		value  ddprofiledefinition.BGPValueConfig
		table  string
		symbol string
	}{
		"peer_identifier": {
			value:  peer.Descriptors.PeerIdentifier,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerIdentifier",
		},
		"last_received_update_age": {
			value:  peer.Connection.LastReceivedUpdateAge,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerInUpdateElapsedTime",
		},
		"messages.received": {
			value:  peer.Traffic.Messages.Received,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerInTotalMessages",
		},
		"messages.sent": {
			value:  peer.Traffic.Messages.Sent,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerOutTotalMessages",
		},
		"notifications.received": {
			value:  peer.Traffic.Notifications.Received,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerInNotifications",
		},
		"notifications.sent": {
			value:  peer.Traffic.Notifications.Sent,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerOutNotifications",
		},
		"transitions.flaps": {
			value:  peer.Transitions.Flaps,
			table:  "tBgpPeerNgOperTable",
			symbol: "tBgpPeerNgOperFlaps",
		},
		"timers.configured.connect_retry": {
			value:  peer.Timers.Configured.ConnectRetry,
			symbol: "tBgpPeerNgConnectRetry",
		},
		"timers.configured.hold_time": {
			value:  peer.Timers.Configured.HoldTime,
			symbol: "tBgpPeerNgHoldTime",
		},
		"timers.configured.keepalive_time": {
			value:  peer.Timers.Configured.KeepaliveTime,
			symbol: "tBgpPeerNgKeepAlive",
		},
		"timers.configured.min_route_advertisement_interval": {
			value:  peer.Timers.Configured.MinRouteAdvertisementInterval,
			symbol: "tBgpPeerNgMinRouteAdvertisement",
		},
		"last_error.subcode": {
			value:  peer.LastError.Subcode,
			table:  "tBgpPeerNgOperTable",
			symbol: "bgpPeerLastErrorSubcode",
		},
	}

	for name, tc := range values {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.table, tc.value.Table)
			assert.Equal(t, tc.symbol, tc.value.Symbol.Name)
		})
	}

	tests := map[string]struct {
		id     string
		af     ddprofiledefinition.BGPAddressFamily
		safi   ddprofiledefinition.BGPSubsequentAddressFamily
		label  string
		recv   string
		active string
		reject string
	}{
		"ipv4 unicast": {
			id:     "nokia-timos-bgp-peer-family-ipv4-unicast",
			af:     ddprofiledefinition.BGPAddressFamilyIPv4,
			safi:   ddprofiledefinition.BGPSubsequentAddressFamilyUnicast,
			label:  "ipv4 unicast",
			recv:   "tBgpPeerNgOperReceivedPrefixes",
			active: "tBgpPeerNgOperActivePrefixes",
			reject: "tBgpPeerNgOperIpv4RejPfxs",
		},
		"ipv4 vpn": {
			id:     "nokia-timos-bgp-peer-family-ipv4-vpn",
			af:     ddprofiledefinition.BGPAddressFamilyIPv4,
			safi:   ddprofiledefinition.BGPSubsequentAddressFamilyVPN,
			label:  "vpnv4 unicast",
			recv:   "tBgpPeerNgOperVpnRecvPrefixes",
			active: "tBgpPeerNgOperVpnActivePrefixes",
			reject: "tBgpPeerNgOperVpnIpv4RejPfxs",
		},
		"ipv6 unicast": {
			id:     "nokia-timos-bgp-peer-family-ipv6-unicast",
			af:     ddprofiledefinition.BGPAddressFamilyIPv6,
			safi:   ddprofiledefinition.BGPSubsequentAddressFamilyUnicast,
			label:  "ipv6 unicast",
			recv:   "tBgpPeerNgOperV6ReceivedPrefixes",
			active: "tBgpPeerNgOperV6ActivePrefixes",
			reject: "tBgpPeerNgOperIpv6RejPfxs",
		},
		"ipv6 vpn": {
			id:     "nokia-timos-bgp-peer-family-ipv6-vpn",
			af:     ddprofiledefinition.BGPAddressFamilyIPv6,
			safi:   ddprofiledefinition.BGPSubsequentAddressFamilyVPN,
			label:  "vpnv6 unicast",
			recv:   "tBgpPeerNgOperVpnIpv6RecvPfxs",
			active: "tBgpPeerNgOperVpnIpv6ActivePfxs",
			reject: "tBgpPeerNgOperVpnIpv6RejPfxs",
		},
		"l2vpn vpls": {
			id:     "nokia-timos-bgp-peer-family-l2vpn-vpls",
			af:     ddprofiledefinition.BGPAddressFamilyL2VPN,
			safi:   ddprofiledefinition.BGPSubsequentAddressFamilyVPLS,
			label:  "l2vpn vpls",
			recv:   "tBgpPeerNgOperl2VpnRecvPfxs",
			active: "tBgpPeerNgOperl2VpnActivePfxs",
			reject: "tBgpPeerNgOperl2VpnRejPfxs",
		},
		"l2vpn evpn": {
			id:     "nokia-timos-bgp-peer-family-l2vpn-evpn",
			af:     ddprofiledefinition.BGPAddressFamilyL2VPN,
			safi:   ddprofiledefinition.BGPSubsequentAddressFamilyEVPN,
			label:  "l2vpn evpn",
			recv:   "tBgpPeerNgOperEvpnRecvPfxs",
			active: "tBgpPeerNgOperEvpnActivePfxs",
			reject: "tBgpPeerNgOperEvpnRejPfxs",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			row := requireBGPRowByID(t, profile, tc.id)
			assert.Equal(t, "_nokia-timetra-bgp.yaml", row.OriginProfileID)
			assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, row.Kind)
			assert.Equal(t, "tBgpPeerNgOperTable", row.Table.Name)
			assert.Equal(t, "1.3.6.1.4.1.6527.3.1.2.14.4.8", row.Table.OID)
			assert.Equal(t, "vRtrConfTable", row.Identity.RoutingInstance.Table)
			assert.Equal(t, "tBgpPeerNgAddress", row.Identity.Neighbor.Name)
			assert.Equal(t, "tBgpPeerNgTable", row.Identity.RemoteAS.Table)
			assert.Equal(t, tc.af, ddprofiledefinition.BGPAddressFamily(row.Identity.AddressFamily.Value))
			assert.Equal(t, tc.safi, ddprofiledefinition.BGPSubsequentAddressFamily(row.Identity.SubsequentAddressFamily.Value))
			assert.True(t, hasStaticTag(row.StaticTags, "address_family_name", tc.label))
			assert.Equal(t, tc.recv, row.Routes.Total.Received.Symbol.Name)
			assert.Equal(t, tc.active, row.Routes.Current.Active.Symbol.Name)
			assert.Equal(t, tc.reject, row.Routes.Total.Rejected.Symbol.Name)
		})
	}
}

func mustLoadNokiaSROSBGPProfile(t *testing.T) *Profile {
	t.Helper()

	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.6527.1.3.17", "", nil)
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "nokia-service-router-os.yaml")
	})
	require.NotEqual(t, -1, index, "expected nokia-service-router-os profile to match")

	return matched[index]
}

func hasStaticTag(tags []ddprofiledefinition.StaticMetricTagConfig, name, value string) bool {
	return slices.ContainsFunc(tags, func(tag ddprofiledefinition.StaticMetricTagConfig) bool {
		return tag.Tag == name && tag.Value == value
	})
}
