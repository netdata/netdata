// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"net/http/httptest"
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

func TestIntegration_OpenBGPDLiveNegotiatedFamilyIntersection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD live integration test requires Linux")
	}

	image := getOpenBGPDIntegrationImage(t)
	requireDocker(t)

	harness := newOpenBGPDEstablishedIntegrationHarness(t, image)
	harness.waitReady(t)

	neighborsBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/neighbors")
	require.NoError(t, err)

	families, neighbors, err := parseOpenBGPDNeighbors(neighborsBody)
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Len(t, neighbors, 1)

	family := families[0]
	assert.Equal(t, "default_ipv4_unicast", family.ID)
	assert.Equal(t, int64(1), family.PeersEstablished)
	assert.Equal(t, int64(1), family.ConfiguredPeers)
	assert.Greater(t, family.MessagesReceived, int64(0))
	assert.Greater(t, family.MessagesSent, int64(0))
	assert.Equal(t, int64(0), family.PrefixesReceived)

	neighbor := neighbors[0]
	assert.Equal(t, int64(0), neighbor.UpdatesReceived)
	assert.Equal(t, int64(0), neighbor.UpdatesSent)
	assert.Equal(t, int64(0), neighbor.WithdrawsReceived)
	assert.Equal(t, int64(0), neighbor.WithdrawsSent)
	assert.Greater(t, neighbor.KeepalivesReceived, int64(0))
	assert.Greater(t, neighbor.KeepalivesSent, int64(0))

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.APIURL = harness.apiURL()
	collr.CollectRIBSummaries = false
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxFamilies = 10
	collr.MaxPeers = 10

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	familyID := "default_ipv4_unicast"
	peerID := family.Peers[0].ID
	neighborID := neighbor.ID

	assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_established"])
	assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
	assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_down"])
	assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
	assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
	assert.Greater(t, mx["family_"+familyID+"_messages_received"], int64(0))
	assert.Greater(t, mx["family_"+familyID+"_messages_sent"], int64(0))
	assert.Equal(t, int64(0), mx["family_"+familyID+"_prefixes_received"])
	assert.Equal(t, int64(0), mx["family_"+familyID+"_rib_routes"])

	assert.Equal(t, int64(peerStateUp), mx["peer_"+peerID+"_state"])
	assert.GreaterOrEqual(t, mx["peer_"+peerID+"_uptime_seconds"], int64(0))
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_prefixes_received"])
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_prefixes_advertised"])

	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_updates_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_updates_sent"])
	assert.Greater(t, mx["neighbor_"+neighborID+"_keepalives_received"], int64(0))
	assert.Greater(t, mx["neighbor_"+neighborID+"_keepalives_sent"], int64(0))

	assert.NotContains(t, mx, "family_default_ipv6_unicast_peers_established")
	assert.NotContains(t, mx, "family_default_ipv4_vpn_peers_established")

	require.NotNil(t, collr.Charts().Get("family_"+familyID+"_peer_states"))
	require.Nil(t, collr.Charts().Get("family_default_ipv6_unicast_peer_states"))
	require.Nil(t, collr.Charts().Get("family_default_ipv4_vpn_peer_states"))
}

type openbgpdEstablishedIntegrationHarness struct {
	image         string
	containerName string
	peerName      string
	workDir       string
	peerWorkDir   string
	fcgiSocket    string
	server        *httptest.Server
	networkName   string
	localIP       string
	remoteIP      string
	peerAS        int
}

func newOpenBGPDEstablishedIntegrationHarness(t *testing.T, image string) *openbgpdEstablishedIntegrationHarness {
	t.Helper()

	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, "openbgpd-a")
	peerWorkDir := filepath.Join(baseDir, "openbgpd-b")
	for _, dir := range []string{workDir, peerWorkDir} {
		require.NoError(t, os.MkdirAll(dir, 0o777))
		require.NoError(t, os.Chmod(dir, 0o777))
	}

	subnet, localIP, remoteIP := createOpenBGPDEstablishedSubnet()
	localAS := 65010
	peerAS := 65020

	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bgpd.conf"), []byte(openbgpdEstablishedIntegrationConfig(
		"192.0.2.254", localIP, localAS, remoteIP, peerAS, "198.51.100.0/24",
		[]string{"IPv4 unicast", "IPv6 unicast", "IPv4 vpn"},
	)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(peerWorkDir, "bgpd.conf"), []byte(openbgpdEstablishedIntegrationConfig(
		"192.0.2.253", remoteIP, peerAS, localIP, localAS, "203.0.113.0/24",
		[]string{"IPv4 unicast"},
	)), 0o644))

	h := &openbgpdEstablishedIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-openbgpd-established-a-%d", time.Now().UnixNano()),
		peerName:      fmt.Sprintf("netdata-bgp-openbgpd-established-b-%d", time.Now().UnixNano()),
		workDir:       workDir,
		peerWorkDir:   peerWorkDir,
		fcgiSocket:    filepath.Join(workDir, "bgplgd.sock"),
		networkName:   fmt.Sprintf("netdata-bgp-openbgpd-established-%d", time.Now().UnixNano()),
		localIP:       localIP,
		remoteIP:      remoteIP,
		peerAS:        peerAS,
	}

	_, err := runDockerCommand(30*time.Second, "network", "create", "--subnet", subnet, h.networkName)
	require.NoError(t, err)

	_, err = runDockerCommand(3*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--network", h.networkName,
		"--ip", h.localIP,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.image,
		"sh", "-lc", openbgpdIntegrationRunCommand,
	)
	require.NoError(t, err)

	_, err = runDockerCommand(3*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.peerName,
		"--network", h.networkName,
		"--ip", h.remoteIP,
		"--volume", fmt.Sprintf("%s:/work", h.peerWorkDir),
		h.image,
		"sh", "-lc", openbgpdPeerRunCommand,
	)
	require.NoError(t, err)

	h.server = httptest.NewServer(newOpenBGPDFastCGIProxy(h.fcgiSocket, 15*time.Second))

	t.Cleanup(func() {
		if h.server != nil {
			h.server.Close()
		}

		if t.Failed() {
			t.Logf("OpenBGPD established logs (%s):\n%s", h.containerName, h.dockerOutput("logs", h.containerName))
			t.Logf("OpenBGPD established logs (%s):\n%s", h.peerName, h.dockerOutput("logs", h.peerName))
			t.Logf("OpenBGPD established show neighbor (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show neighbor || true"))
			t.Logf("OpenBGPD established show neighbor (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show neighbor || true"))
			t.Logf("OpenBGPD established show rib (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show rib detail || true"))
			t.Logf("OpenBGPD established config (%s):\n%s", h.containerName, h.hostFile("bgpd.conf"))
			t.Logf("OpenBGPD established config (%s):\n%s", h.peerName, h.peerHostFile("bgpd.conf"))
			t.Logf("OpenBGPD established bgpd log (%s):\n%s", h.containerName, h.hostFile("bgpd.log"))
			t.Logf("OpenBGPD established bgpd log (%s):\n%s", h.peerName, h.peerHostFile("bgpd.log"))
			t.Logf("OpenBGPD established bgplgd log (%s):\n%s", h.containerName, h.hostFile("bgplgd.log"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName, h.peerName)
		_, _ = runDockerCommand(15*time.Second, "network", "rm", h.networkName)
	})

	return h
}

func (h *openbgpdEstablishedIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := os.Stat(h.fcgiSocket); err != nil {
			return false
		}
		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"chmod 666 /work/bgplgd.sock >/dev/null 2>&1 || true",
		); err != nil {
			return false
		}

		neighborsJSON, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"bgpctl -j -s /run/bgpd.rsock show neighbor",
		)
		if err != nil {
			return false
		}

		families, _, err := parseOpenBGPDNeighbors([]byte(neighborsJSON))
		if err != nil || len(families) != 1 {
			return false
		}

		return families[0].ID == "default_ipv4_unicast" && families[0].PeersEstablished == 1
	}, 3*time.Minute, 500*time.Millisecond, "OpenBGPD established peers did not reach the expected negotiated routed state")
}

func (h *openbgpdEstablishedIntegrationHarness) apiURL() string {
	if h.server == nil {
		return ""
	}
	return h.server.URL
}

func (h *openbgpdEstablishedIntegrationHarness) peerAddress() string {
	return h.remoteIP
}

func (h *openbgpdEstablishedIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func (h *openbgpdEstablishedIntegrationHarness) hostFile(name string) string {
	data, err := os.ReadFile(filepath.Join(h.workDir, name))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

func (h *openbgpdEstablishedIntegrationHarness) peerHostFile(name string) string {
	data, err := os.ReadFile(filepath.Join(h.peerWorkDir, name))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

func createOpenBGPDEstablishedSubnet() (subnet, localIP, remoteIP string) {
	subnet = fmt.Sprintf("10.240.%d.0/24", time.Now().UnixNano()%200)
	localIP = strings.TrimSuffix(subnet, "0/24") + "2"
	remoteIP = strings.TrimSuffix(subnet, "0/24") + "3"
	return subnet, localIP, remoteIP
}

func openbgpdEstablishedIntegrationConfig(routerID, listenIP string, localAS int, neighborIP string, remoteAS int, networkPrefix string, families []string) string {
	return openbgpdEstablishedIntegrationConfigWithExtra(routerID, listenIP, localAS, neighborIP, remoteAS, networkPrefix, families, "")
}

func openbgpdEstablishedIntegrationConfigWithExtra(routerID, listenIP string, localAS int, neighborIP string, remoteAS int, networkPrefix string, families []string, extra string) string {
	var b strings.Builder
	fmt.Fprintf(&b, "AS %d\n", localAS)
	fmt.Fprintf(&b, "router-id %s\n", routerID)
	b.WriteString("fib-update no\n")
	b.WriteString("socket \"/run/bgpd.rsock\" restricted\n")
	fmt.Fprintf(&b, "listen on %s port 179\n", listenIP)
	fmt.Fprintf(&b, "network %s\n", networkPrefix)
	fmt.Fprintf(&b, "neighbor %s {\n", neighborIP)
	fmt.Fprintf(&b, "    remote-as %d\n", remoteAS)
	b.WriteString("    descr \"test-peer\"\n")
	for _, family := range families {
		fmt.Fprintf(&b, "    announce %s\n", family)
	}
	b.WriteString("}\n")
	if extra = strings.TrimSpace(extra); extra != "" {
		b.WriteString(extra)
		b.WriteString("\n")
	}
	return b.String()
}

const openbgpdPeerRunCommand = `
set -e
apt-get update >/dev/null
DEBIAN_FRONTEND=noninteractive apt-get install -y openbgpd >/dev/null
mkdir -p /run/openbgpd
chown _openbgpd:_openbgpd /run/openbgpd
bgpd -dvf /work/bgpd.conf >/work/bgpd.log 2>&1 &
bgpd_pid=$!
trap 'kill "$bgpd_pid" 2>/dev/null || true' EXIT INT TERM
wait "$bgpd_pid"
`
