// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenBGPDLiveConfiguredVPNFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD live integration test requires Linux")
	}

	image := getOpenBGPDIntegrationImage(t)
	requireDocker(t)

	harness := newOpenBGPDIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfigVPN)
	harness.waitReady(t)

	neighborsBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/neighbors")
	require.NoError(t, err)

	families, neighbors, err := parseOpenBGPDNeighbors(neighborsBody)
	require.NoError(t, err)
	require.Len(t, families, 4)
	require.Len(t, neighbors, 1)

	ids := make([]string, 0, len(families))
	for _, family := range families {
		ids = append(ids, family.ID)
		assert.Equal(t, int64(0), family.PeersEstablished)
		assert.Equal(t, int64(0), family.PeersAdminDown)
		assert.Equal(t, int64(1), family.PeersDown)
		assert.Equal(t, int64(1), family.ConfiguredPeers)
		assert.Equal(t, int64(0), family.MessagesReceived)
		assert.Equal(t, int64(0), family.MessagesSent)
		assert.Equal(t, int64(0), family.PrefixesReceived)
		assert.Len(t, family.Peers, 1)
		assert.Equal(t, int64(peerStateDown), family.Peers[0].State)
	}

	assert.Equal(t, []string{
		"default_ipv4_unicast",
		"default_ipv4_vpn",
		"default_ipv6_unicast",
		"default_ipv6_vpn",
	}, ids)

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.APIURL = harness.apiURL()
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxFamilies = 10
	collr.MaxPeers = 10

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	for _, familyID := range ids {
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_established"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_messages_received"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_messages_sent"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_prefixes_received"])
	}

	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_correctness_valid"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_correctness_invalid"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_not_found"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_unicast_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_unicast_correctness_valid"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_unicast_correctness_invalid"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_unicast_correctness_not_found"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_vpn_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_vpn_correctness_valid"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_vpn_correctness_invalid"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_vpn_correctness_not_found"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_vpn_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_vpn_correctness_valid"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_vpn_correctness_invalid"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_vpn_correctness_not_found"])
}

const openbgpdIntegrationConfigVPN = `AS 65010
router-id 192.0.2.254
fib-update no
socket "/run/bgpd.rsock" restricted
listen on 127.0.0.1 port 1179
network 203.0.113.0/24
neighbor 192.0.2.2 {
    remote-as 65020
    passive
    descr "test-peer"
    announce IPv4 unicast
    announce IPv6 unicast
    announce IPv4 vpn
    announce IPv6 vpn
}
`
