// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_GoBGPLiveEstablishedMixedFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GoBGP live integration test requires Linux")
	}

	cfg := getGoBGPIntegrationConfig(t)
	requireDocker(t)

	harness := newGoBGPEstablishedIntegrationHarness(t, cfg)
	harness.waitReady(t)
	harness.waitEstablished(t)
	harness.seedRuntime(t)
	harness.waitRoutesLearned(t)

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
		"default_ipv6_unicast",
		"default_ipv4_flowspec",
		"default_ipv6_flowspec",
		"default_l2vpn_evpn",
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

type gobgpEstablishedIntegrationHarness struct {
	cfg           gobgpIntegrationConfig
	containerName string
	peerName      string
	networkName   string
	workDir       string
	localIP       string
	remoteIP      string
	grpcHostPort  string
}

func newGoBGPEstablishedIntegrationHarness(t *testing.T, cfg gobgpIntegrationConfig) *gobgpEstablishedIntegrationHarness {
	return newGoBGPEstablishedIntegrationHarnessWithConfigs(
		t,
		cfg,
		goBGPEstablishedIntegrationConfigFile(localIPPlaceholder, remoteIPPlaceholder, 64512, 64513, "192.0.2.1", true),
		goBGPEstablishedIntegrationConfigFile(localIPPlaceholder, remoteIPPlaceholder, 64513, 64512, "192.0.2.2", false),
	)
}

func newGoBGPEstablishedIntegrationHarnessWithConfigs(t *testing.T, cfg gobgpIntegrationConfig, configA, configB string) *gobgpEstablishedIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	buildGoIntegrationBinary(t, cfg.sourcePath, "gobgp", filepath.Join(workDir, "gobgp"))
	buildGoIntegrationBinary(t, cfg.sourcePath, "gobgpd", filepath.Join(workDir, "gobgpd"))

	subnet, localIP, remoteIP := createFRREstablishedSubnet(t)
	configA = strings.ReplaceAll(configA, localIPPlaceholder, localIP)
	configA = strings.ReplaceAll(configA, remoteIPPlaceholder, remoteIP)
	configB = strings.ReplaceAll(configB, localIPPlaceholder, remoteIP)
	configB = strings.ReplaceAll(configB, remoteIPPlaceholder, localIP)
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "gobgpd-a.conf"), []byte(configA), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "gobgpd-b.conf"), []byte(configB), 0o644))

	h := &gobgpEstablishedIntegrationHarness{
		cfg:           cfg,
		containerName: fmt.Sprintf("netdata-bgp-gobgp-established-a-%d", time.Now().UnixNano()),
		peerName:      fmt.Sprintf("netdata-bgp-gobgp-established-b-%d", time.Now().UnixNano()),
		networkName:   fmt.Sprintf("netdata-bgp-gobgp-established-%d", time.Now().UnixNano()),
		workDir:       workDir,
		localIP:       localIP,
		remoteIP:      remoteIP,
	}

	_, err := runDockerCommand(30*time.Second, "network", "create", "--subnet", subnet, h.networkName)
	require.NoError(t, err)

	_, err = runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--network", h.networkName,
		"--ip", h.localIP,
		"--publish", "127.0.0.1::50051",
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.cfg.image,
		"/work/gobgpd",
		"-f", "/work/gobgpd-a.conf",
		"--api-hosts", ":50051",
		"--pprof-disable",
	)
	require.NoError(t, err)

	_, err = runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.peerName,
		"--network", h.networkName,
		"--ip", h.remoteIP,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.cfg.image,
		"/work/gobgpd",
		"-f", "/work/gobgpd-b.conf",
		"--api-hosts", ":50051",
		"--pprof-disable",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("GoBGP established logs (%s):\n%s", h.containerName, h.dockerOutput("logs", h.containerName))
			t.Logf("GoBGP established logs (%s):\n%s", h.peerName, h.dockerOutput("logs", h.peerName))
			t.Logf("GoBGP established neighbor (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "neighbor"))
			t.Logf("GoBGP established neighbor (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "/work/gobgp", "neighbor"))
			t.Logf("GoBGP established ipv4 rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib"))
			t.Logf("GoBGP established ipv6 rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6"))
			t.Logf("GoBGP established ipv4-flowspec rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv4-flowspec"))
			t.Logf("GoBGP established ipv6-flowspec rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6-flowspec"))
			t.Logf("GoBGP established evpn rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "evpn"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName, h.peerName)
		_, _ = runDockerCommand(15*time.Second, "network", "rm", h.networkName)
	})

	return h
}

func (h *gobgpEstablishedIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global"); err != nil {
			return false
		}
		if _, err := runDockerCommand(10*time.Second, "exec", h.peerName, "/work/gobgp", "global"); err != nil {
			return false
		}
		addr, err := h.mappedGRPCAddress()
		if err != nil {
			return false
		}
		h.grpcHostPort = addr
		return true
	}, 90*time.Second, 500*time.Millisecond, "GoBGP established containers did not become ready")
}

func (h *gobgpEstablishedIntegrationHarness) waitEstablished(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		outA, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "neighbor")
		if err != nil || !containsAll(outA, h.remoteIP, "Establ") {
			return false
		}
		outB, err := runDockerCommand(10*time.Second, "exec", h.peerName, "/work/gobgp", "neighbor")
		if err != nil || !containsAll(outB, h.localIP, "Establ") {
			return false
		}
		return true
	}, 90*time.Second, 500*time.Millisecond, "GoBGP peers did not establish the expected mixed-family session")
}

func (h *gobgpEstablishedIntegrationHarness) seedRuntime(t *testing.T) {
	t.Helper()

	for _, cmd := range [][]string{
		{"exec", h.peerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4", "203.0.113.0/24"},
		{"exec", h.peerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv6", "2001:db8:10::/64"},
		{"exec", h.peerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4-flowspec", "match", "destination", "203.0.113.0/24", "then", "discard"},
		{"exec", h.peerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv6-flowspec", "match", "destination", "2001:db8:30::/64", "then", "discard"},
		{"exec", h.peerName, "/work/gobgp", "global", "rib", "add", "-a", "evpn", "multicast", "10.0.0.1", "etag", "100", "rd", "100:100"},
	} {
		_, err := runDockerCommand(15*time.Second, cmd...)
		require.NoError(t, err)
	}
}

func (h *gobgpEstablishedIntegrationHarness) waitRoutesLearned(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib"); err != nil || !containsAll(out, "203.0.113.0/24") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6"); err != nil || !containsAll(out, "2001:db8:10::/64") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv4-flowspec"); err != nil || !containsAll(out, "203.0.113.0/24", "discard") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv6-flowspec"); err != nil || !containsAll(out, "2001:db8:30::/64", "discard") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "evpn"); err != nil || !containsAll(out, "10.0.0.1") {
			return false
		}
		return true
	}, 90*time.Second, 500*time.Millisecond, "GoBGP established session did not learn the expected mixed-family routes")
}

func (h *gobgpEstablishedIntegrationHarness) grpcAddress() string {
	return h.grpcHostPort
}

func (h *gobgpEstablishedIntegrationHarness) mappedGRPCAddress() (string, error) {
	if h.grpcHostPort != "" {
		return h.grpcHostPort, nil
	}

	out, err := runDockerCommand(10*time.Second, "port", h.containerName, "50051/tcp")
	if err != nil {
		return "", err
	}

	addr := strings.TrimSpace(out)
	if idx := strings.LastIndex(addr, "\n"); idx >= 0 {
		addr = strings.TrimSpace(addr[idx+1:])
	}
	if addr == "" {
		return "", fmt.Errorf("empty mapped GoBGP gRPC address")
	}
	return addr, nil
}

func (h *gobgpEstablishedIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func goBGPEstablishedIntegrationConfigFile(localIP, remoteIP string, localAS, remoteAS int, routerID string, passive bool) string {
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
      afi-safi-name = "ipv4-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv6-unicast"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv4-flowspec"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "ipv6-flowspec"
  [[neighbors.afi-safis]]
    [neighbors.afi-safis.config]
      afi-safi-name = "l2vpn-evpn"
`, localAS, routerID, remoteIP, remoteAS, localIP, passiveLine)
}

const (
	localIPPlaceholder  = "__LOCAL_IP__"
	remoteIPPlaceholder = "__REMOTE_IP__"
)
