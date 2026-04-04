// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var dataBIRDProtocolsAllMPLSVPNAliases []byte

func TestParseBIRDProtocolsAllMPLSVPNAliases(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 18, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllMPLSVPNAliases)
	require.NoError(t, err)
	require.Len(t, protocols, 2)

	bgpProto := protocols[1]
	assert.Equal(t, "bgp_mpls", bgpProto.Name)
	assert.Equal(t, "BGP", bgpProto.Proto)
	assert.Equal(t, "Active        Socket: Connection closed", bgpProto.Info)
	assert.Equal(t, "MPLS VPN probe", bgpProto.Description)
	assert.Equal(t, "Active", bgpProto.BGPState)
	assert.Equal(t, "198.51.100.2", bgpProto.PeerAddress)
	assert.Equal(t, int64(65001), bgpProto.RemoteAS)
	assert.Equal(t, int64(64512), bgpProto.LocalAS)
	assert.Equal(t, int64(929), bgpProto.UptimeSecs)
	require.Len(t, bgpProto.Channels, 5)

	assert.Equal(t, "ipv4-mpls", bgpProto.Channels[0].Name)
	assert.Equal(t, "label4", bgpProto.Channels[0].Table)
	assert.Equal(t, "ipv6-mpls", bgpProto.Channels[1].Name)
	assert.Equal(t, "label6", bgpProto.Channels[1].Table)
	assert.Equal(t, "vpn4-mpls", bgpProto.Channels[2].Name)
	assert.Equal(t, "vpn4tab", bgpProto.Channels[2].Table)
	assert.Equal(t, "vpn6-mpls", bgpProto.Channels[3].Name)
	assert.Equal(t, "vpn6tab", bgpProto.Channels[3].Table)
	assert.Equal(t, "mpls", bgpProto.Channels[4].Name)
	assert.Equal(t, "mtab", bgpProto.Channels[4].Table)
}

func TestBuildBIRDFamiliesMPLSVPNAliases(t *testing.T) {
	restore := setBIRDNowForTest(time.Date(2026, 4, 3, 18, 0, 0, 0, time.Local))
	defer restore()

	protocols, err := parseBIRDProtocolsAll(dataBIRDProtocolsAllMPLSVPNAliases)
	require.NoError(t, err)

	families := buildBIRDFamilies(protocols)
	require.Len(t, families, 4)

	want := map[string]struct {
		afi   string
		safi  string
		table string
	}{
		"label4_ipv4_label": {afi: "ipv4", safi: "label", table: "label4"},
		"label6_ipv6_label": {afi: "ipv6", safi: "label", table: "label6"},
		"vpn4tab_ipv4_vpn":  {afi: "ipv4", safi: "vpn", table: "vpn4tab"},
		"vpn6tab_ipv6_vpn":  {afi: "ipv6", safi: "vpn", table: "vpn6tab"},
	}

	for _, family := range families {
		expected, ok := want[family.ID]
		require.Truef(t, ok, "unexpected family id %q", family.ID)
		assert.Equal(t, expected.afi, family.AFI)
		assert.Equal(t, expected.safi, family.SAFI)
		assert.Equal(t, expected.table, family.Table)
		assert.Equal(t, int64(1), family.ConfiguredPeers)
		assert.Equal(t, int64(1), family.PeersDown)
		assert.Equal(t, int64(0), family.PeersEstablished)
		require.Len(t, family.Peers, 1)
		assert.Equal(t, "198.51.100.2", family.Peers[0].Address)
	}
}
