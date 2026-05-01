// SPDX-License-Identifier: GPL-3.0-or-later

package bgp

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestBuildFRRSummaryCommand(t *testing.T) {
	tests := []struct {
		name    string
		afi     string
		safi    string
		want    string
		wantErr bool
	}{
		{name: "ipv4", afi: "ipv4", safi: "unicast", want: "show bgp vrf all ipv4 summary json"},
		{name: "ipv6", afi: "ipv6", safi: "unicast", want: "show bgp vrf all ipv6 summary json"},
		{name: "evpn", afi: "l2vpn", safi: "evpn", want: "show bgp vrf all l2vpn evpn summary json"},
		{name: "invalid", afi: "vpnv4", safi: "unicast", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := buildFRRSummaryCommand(tc.afi, tc.safi)
			if tc.wantErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, got)
		})
	}
}

func TestParseFRRSummaryEVPN(t *testing.T) {
	families, err := parseFRRSummary(dataFRREVPNSummary, "l2vpn", "evpn", backendFRR, nil)
	require.NoError(t, err)
	require.Len(t, families, 1)

	family := families[0]
	assert.Equal(t, "default_l2vpn_evpn", family.ID)
	assert.Equal(t, "default", family.VRF)
	assert.Equal(t, "l2vpn", family.AFI)
	assert.Equal(t, "evpn", family.SAFI)
	assert.Equal(t, int64(65001), family.LocalAS)
	assert.Equal(t, int64(5), family.RIBRoutes)
	assert.Equal(t, int64(1), family.ConfiguredPeers)
	assert.Equal(t, int64(1), family.PeersEstablished)
	require.Len(t, family.Peers, 1)
	assert.Equal(t, "192.168.0.2", family.Peers[0].Address)
	assert.Equal(t, int64(4), family.Peers[0].PrefixesReceived)
	assert.True(t, family.Peers[0].HasPrefixesSent)
	assert.Equal(t, int64(4), family.Peers[0].PrefixesSent)
}

func TestParseFRREVPNVNIs(t *testing.T) {
	vnis, err := parseFRREVPNVNIs(dataFRREVPNVNI, backendFRR)
	require.NoError(t, err)
	require.Len(t, vnis, 2)

	assert.Equal(t, makeVNIID("default", 172192, "l2"), vnis[0].ID)
	assert.Equal(t, int64(172192), vnis[0].VNI)
	assert.Equal(t, "l2", vnis[0].Type)
	assert.Equal(t, int64(23), vnis[0].ARPND)
	assert.Equal(t, int64(-1), vnis[0].RemoteVTEPs)

	assert.Equal(t, makeVNIID("default", 174374, "l2"), vnis[1].ID)
	assert.Equal(t, int64(174374), vnis[1].VNI)
	assert.Equal(t, int64(42), vnis[1].MACs)
	assert.Equal(t, int64(1), vnis[1].RemoteVTEPs)
}

func TestCollector_SelectVNIs(t *testing.T) {
	collr := New()
	collr.MaxVNIs = 1
	collr.SelectVNIs = matcher.SimpleExpr{Includes: []string{"=default/174374"}}
	require.NoError(t, collr.Init(context.Background()))

	selected := collr.selectVNIs([]vniStats{
		{ID: makeVNIID("default", 172192, "l2"), TenantVRF: "default", VNI: 172192},
		{ID: makeVNIID("default", 174374, "l2"), TenantVRF: "default", VNI: 174374},
	})

	assert.False(t, selected[makeVNIID("default", 172192, "l2")])
	assert.True(t, selected[makeVNIID("default", 174374, "l2")])
}

func TestCollector_SelectFamiliesWithEVPNWhenOverLimit(t *testing.T) {
	collr := New()
	collr.MaxFamilies = 1
	collr.SelectFamilies = matcher.SimpleExpr{Includes: []string{"=default/l2vpn/evpn"}}
	require.NoError(t, collr.Init(context.Background()))

	selected := collr.selectFamilies([]familyStats{
		{ID: "default_ipv4_unicast", VRF: "default", AFI: "ipv4", SAFI: "unicast"},
		{ID: "default_l2vpn_evpn", VRF: "default", AFI: "l2vpn", SAFI: "evpn"},
	})

	assert.False(t, selected["default_ipv4_unicast"])
	assert.True(t, selected["default_l2vpn_evpn"])
}

func TestCollector_CollectEVPNVNICharts(t *testing.T) {
	mock := &mockClient{
		responses: map[string][]byte{
			"ipv4":       dataFRREmptySummary,
			"ipv6":       dataFRREmptySummary,
			"l2vpn/evpn": dataFRREVPNSummary,
		},
		neighbors: dataFRRNeighborsEnriched,
		evpnVNI:   dataFRREVPNVNI,
	}

	collr := New()
	collr.newClient = func(Config) (bgpClient, error) { return mock, nil }
	collr.MaxVNIs = 1
	collr.SelectVNIs = matcher.SimpleExpr{Includes: []string{"=default/172192"}}
	require.NoError(t, collr.Init(context.Background()))

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	for k, v := range familyMetricSet("default_l2vpn_evpn", 1, 0, 0, 1, 1, 50, 55, 4, 5) {
		assert.Equalf(t, v, mx[k], "metric %s", k)
	}

	peerID := makePeerID("default_l2vpn_evpn", "192.168.0.2")
	for k, v := range peerMetricSet(peerID, 50, 55, 4, 3600, peerStateUp) {
		assert.Equalf(t, v, mx[k], "metric %s", k)
	}
	assert.Equal(t, int64(4), mx["peer_"+peerID+"_prefixes_advertised"])

	vniID := makeVNIID("default", 172192, "l2")
	assert.Equal(t, int64(0), mx["vni_"+vniID+"_macs"])
	assert.Equal(t, int64(23), mx["vni_"+vniID+"_arp_nd"])
	assert.Equal(t, int64(-1), mx["vni_"+vniID+"_remote_vteps"])

	require.NotNil(t, collr.Charts().Get("family_default_l2vpn_evpn_peer_states"))
	require.NotNil(t, collr.Charts().Get("peer_"+peerID+"_messages"))
	require.NotNil(t, collr.Charts().Get("vni_"+vniID+"_entries"))
	require.NotNil(t, collr.Charts().Get("vni_"+vniID+"_remote_vteps"))
}
