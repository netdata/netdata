// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_GoBGPLiveEstablishedVPNFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GoBGP live integration test requires Linux")
	}

	cfg := getGoBGPIntegrationConfig(t)
	requireDocker(t)

	harness := newGoBGPEstablishedVPNIntegrationHarness(t, cfg)
	harness.waitReady(t)
	harness.addVRFs(t)
	harness.waitEstablished(t)
	harness.seedVPNRuntime(t)
	harness.waitVPNRoutesLearned(t)

	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = harness.grpcAddress()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxFamilies = 10
	collr.MaxPeers = 10

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expectedFamilies := []string{
		"default_ipv4_vpn",
		"default_ipv6_vpn",
	}

	for _, familyID := range expectedFamilies {
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_established"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_prefixes_received"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_rib_routes"])
		require.NotNil(t, collr.Charts().Get("family_"+familyID+"_peer_states"))
	}
}

func newGoBGPEstablishedVPNIntegrationHarness(t *testing.T, cfg gobgpIntegrationConfig) *gobgpEstablishedIntegrationHarness {
	t.Helper()

	h := newGoBGPEstablishedIntegrationHarnessWithConfigs(
		t,
		cfg,
		goBGPEstablishedVPNIntegrationConfigFile(localIPPlaceholder, remoteIPPlaceholder, 64512, 64513, "192.0.2.11", true),
		goBGPEstablishedVPNIntegrationConfigFile(localIPPlaceholder, remoteIPPlaceholder, 64513, 64512, "192.0.2.12", false),
	)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("GoBGP established VPN vpnv4 rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv4"))
			t.Logf("GoBGP established VPN vpnv6 rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv6"))
			t.Logf("GoBGP established VPN vrf red rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "vrf", "red", "rib"))
			t.Logf("GoBGP established VPN vrf red ipv6 rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "vrf", "red", "rib", "-a", "ipv6"))
			t.Logf("GoBGP established VPN vpnv4 rib (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "/work/gobgp", "global", "rib", "-a", "vpnv4"))
			t.Logf("GoBGP established VPN vpnv6 rib (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "/work/gobgp", "global", "rib", "-a", "vpnv6"))
			t.Logf("GoBGP established VPN vrf red rib (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "/work/gobgp", "vrf", "red", "rib"))
			t.Logf("GoBGP established VPN vrf red ipv6 rib (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "/work/gobgp", "vrf", "red", "rib", "-a", "ipv6"))
		}
	})

	return h
}

func (h *gobgpEstablishedIntegrationHarness) addVRFs(t *testing.T) {
	t.Helper()

	for _, cmd := range [][]string{
		{"exec", h.containerName, "/work/gobgp", "vrf", "add", "red", "rd", "64512:100", "rt", "both", "100:100"},
		{"exec", h.peerName, "/work/gobgp", "vrf", "add", "red", "rd", "64513:100", "rt", "both", "100:100"},
	} {
		_, err := runDockerCommand(15*time.Second, cmd...)
		require.NoError(t, err)
	}
}

func (h *gobgpEstablishedIntegrationHarness) seedVPNRuntime(t *testing.T) {
	t.Helper()

	for _, cmd := range [][]string{
		{"exec", h.peerName, "/work/gobgp", "vrf", "red", "rib", "add", "198.51.100.0/24"},
		{"exec", h.peerName, "/work/gobgp", "vrf", "red", "rib", "add", "2001:db8:50::/64", "-a", "ipv6"},
	} {
		_, err := runDockerCommand(15*time.Second, cmd...)
		require.NoError(t, err)
	}
}

func (h *gobgpEstablishedIntegrationHarness) waitVPNRoutesLearned(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv4"); err != nil || !containsAll(out, "198.51.100.0/24") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv6"); err != nil || !containsAll(out, "2001:db8:50::/64") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "red", "rib"); err != nil || !containsAll(out, "198.51.100.0/24") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "red", "rib", "-a", "ipv6"); err != nil || !containsAll(out, "2001:db8:50::/64") {
			return false
		}
		return true
	}, 90*time.Second, 500*time.Millisecond, "GoBGP established VPN session did not learn the expected vpnv4/vpnv6 routes")
}

func goBGPEstablishedVPNIntegrationConfigFile(localIP, remoteIP string, localAS, remoteAS int, routerID string, passive bool) string {
	passiveLine := ""
	if passive {
		passiveLine = "    passive-mode = true\n"
	}

	return fmt.Sprintf(`[global.config]
  as = %d
  router-id = %q
  port = 1179

[[neighbors]]
  [neighbors.config]
    neighbor-address = %q
    peer-as = %d
  [neighbors.transport.config]
    local-address = %q
    remote-port = 1179
%s  [neighbors.timers.config]
    connect-retry = 1
    hold-time = 9
    keepalive-interval = 3
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "l3vpn-ipv4-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "l3vpn-ipv6-unicast"
`, localAS, routerID, remoteIP, remoteAS, localIP, passiveLine)
}
