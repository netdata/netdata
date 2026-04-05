// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_GoBGPLiveConfiguredExtendedFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GoBGP live integration test requires Linux")
	}

	cfg := getGoBGPIntegrationConfig(t)
	requireDocker(t)

	harness := newGoBGPIntegrationHarnessWithConfig(t, cfg, goBGPExtendedFamiliesIntegrationConfigFile)
	harness.waitReady(t)
	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("GoBGP extended neighbor:\n%s", harness.dockerOutput("exec", harness.containerName, "/work/gobgp", "neighbor", "192.0.2.2"))
			t.Logf("GoBGP extended vpnv4 rib:\n%s", harness.dockerOutput("exec", harness.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv4"))
			t.Logf("GoBGP extended vpnv6 rib:\n%s", harness.dockerOutput("exec", harness.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv6"))
			t.Logf("GoBGP extended ipv4-flowspec rib:\n%s", harness.dockerOutput("exec", harness.containerName, "/work/gobgp", "global", "rib", "-a", "ipv4-flowspec"))
			t.Logf("GoBGP extended ipv6-flowspec rib:\n%s", harness.dockerOutput("exec", harness.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6-flowspec"))
			t.Logf("GoBGP extended evpn rib:\n%s", harness.dockerOutput("exec", harness.containerName, "/work/gobgp", "global", "rib", "-a", "evpn"))
		}
	})
	seedGoBGPExtendedRuntime(t, harness)

	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = harness.grpcAddress()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxFamilies = 20
	collr.MaxPeers = 10

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	expectedFamilies := []string{
		"default_ipv4_unicast",
		"default_ipv4_vpn",
		"default_ipv4_flowspec",
		"default_ipv6_unicast",
		"default_ipv6_vpn",
		"default_ipv6_flowspec",
		"default_l2vpn_evpn",
	}

	for _, familyID := range expectedFamilies {
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_established"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_messages_received"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_messages_sent"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_prefixes_received"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_rib_routes"])
		require.NotNil(t, collr.Charts().Get("family_"+familyID+"_peer_states"))
	}
}

func seedGoBGPExtendedRuntime(t *testing.T, h *gobgpIntegrationHarness) {
	t.Helper()

	for _, cmd := range [][]string{
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4", "203.0.113.0/24"},
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "vpnv4", "198.51.100.0/24", "label", "10", "rd", "100:100", "rt", "100:200"},
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4-flowspec", "match", "destination", "203.0.113.0/24", "then", "discard"},
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv6", "2001:db8:10::/64"},
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "vpnv6", "2001:db8:20::/64", "label", "10", "rd", "100:100", "rt", "100:200"},
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv6-flowspec", "match", "destination", "2001:db8:30::/64", "then", "discard"},
		{"exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "evpn", "multicast", "10.0.0.1", "etag", "100", "rd", "100:100"},
	} {
		_, err := runDockerCommand(15*time.Second, cmd...)
		require.NoError(t, err)
	}

	require.Eventually(t, func() bool {
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib"); err != nil || !containsAll(out, "203.0.113.0/24") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv4"); err != nil || !containsAll(out, "198.51.100.0/24") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv4-flowspec"); err != nil || !containsAll(out, "203.0.113.0/24", "discard") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6"); err != nil || !containsAll(out, "2001:db8:10::/64") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "vpnv6"); err != nil || !containsAll(out, "2001:db8:20::/64") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6-flowspec"); err != nil || !containsAll(out, "2001:db8:30::/64", "discard") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "evpn"); err != nil || !containsAll(out, "10.0.0.1") {
			return false
		}
		return true
	}, 45*time.Second, 500*time.Millisecond, "GoBGP runtime did not expose the expected mixed-family routes")
}

func containsAll(s string, subs ...string) bool {
	for _, sub := range subs {
		if !strings.Contains(s, sub) {
			return false
		}
	}
	return true
}

const goBGPExtendedFamiliesIntegrationConfigFile = `[global.config]
  as = 64512
  router-id = "192.0.2.254"
  port = -1

[[neighbors]]
  [neighbors.config]
    neighbor-address = "192.0.2.2"
    peer-as = 65002
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv4-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "l3vpn-ipv4-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv4-flowspec"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv6-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "l3vpn-ipv6-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv6-flowspec"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "l2vpn-evpn"
`
