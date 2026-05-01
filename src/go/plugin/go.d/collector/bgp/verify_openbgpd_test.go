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

const (
	bgpOpenBGPDIntegrationEnableEnv = "BGP_OPENBGPD_ENABLE_INTEGRATION"
	bgpOpenBGPDIntegrationImageEnv  = "BGP_OPENBGPD_DOCKER_IMAGE"

	defaultOpenBGPDIntegrationImage = "debian:bookworm-slim"
)

func TestIntegration_OpenBGPDLiveCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD live integration test requires Linux")
	}

	image := getOpenBGPDIntegrationImage(t)
	requireDocker(t)

	harness := newOpenBGPDIntegrationHarness(t, image)
	harness.waitReady(t)

	neighborsBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/neighbors")
	require.NoError(t, err)
	families, neighbors, err := parseOpenBGPDNeighbors(neighborsBody)
	require.NoError(t, err)
	require.Len(t, families, 1)
	require.Len(t, neighbors, 1)

	ribBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/rib")
	require.NoError(t, err)
	ribSummaries, err := parseOpenBGPDRIBSummaries(ribBody)
	require.NoError(t, err)
	require.Len(t, ribSummaries, 1)

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

	peerID := makePeerIDWithScope("default_ipv4_unicast", "192.0.2.2", "65020")
	neighborID := makeNeighborIDWithScope("default", "192.0.2.2", "65020")

	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_established"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_admin_down"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_down"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_configured"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_charted"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_received"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_sent"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_prefixes_received"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_correctness_valid"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_correctness_invalid"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_not_found"])

	assert.Equal(t, int64(0), mx["peer_"+peerID+"_messages_received"])
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_messages_sent"])
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_prefixes_received"])
	assert.Equal(t, int64(peerStateDown), mx["peer_"+peerID+"_state"])
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_uptime_seconds"])
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_prefixes_advertised"])

	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_updates_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_updates_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_notifications_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_notifications_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_keepalives_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_keepalives_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_route_refresh_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_route_refresh_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_updates_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_updates_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_withdraws_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_withdraws_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_notifications_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_notifications_sent"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_route_refresh_received"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_route_refresh_sent"])

	require.NotNil(t, collr.Charts().Get("family_default_ipv4_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get(familyCorrectnessChartID("default_ipv4_unicast")))
	require.NotNil(t, collr.Charts().Get("peer_"+peerID+"_messages"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_churn"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_message_types"))

	peerChart := collr.Charts().Get("peer_" + peerID + "_messages")
	require.NotNil(t, peerChart)
	assert.Equal(t, "test-peer", chartLabelValue(peerChart, "peer_desc"))

	collectorChart := collr.Charts().Get("collector_status")
	require.NotNil(t, collectorChart)
	assert.Equal(t, harness.apiURL(), chartLabelValue(collectorChart, "target"))
}

func TestIntegration_OpenBGPDLiveConfiguredDualStackFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD live integration test requires Linux")
	}

	image := getOpenBGPDIntegrationImage(t)
	requireDocker(t)

	harness := newOpenBGPDIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfigDualStack)
	harness.waitReady(t)

	neighborsBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/neighbors")
	require.NoError(t, err)

	families, neighbors, err := parseOpenBGPDNeighbors(neighborsBody)
	require.NoError(t, err)
	require.Len(t, families, 2)
	require.Len(t, neighbors, 1)

	assert.Equal(t, "default_ipv4_unicast", families[0].ID)
	assert.Equal(t, "default_ipv6_unicast", families[1].ID)
	for _, family := range families {
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
}

func getOpenBGPDIntegrationImage(t *testing.T) string {
	t.Helper()

	switch strings.ToLower(strings.TrimSpace(os.Getenv(bgpOpenBGPDIntegrationEnableEnv))) {
	case "1", "true", "yes", "on":
	default:
		t.Skipf("%s is not enabled", bgpOpenBGPDIntegrationEnableEnv)
	}

	image := strings.TrimSpace(os.Getenv(bgpOpenBGPDIntegrationImageEnv))
	if image == "" {
		image = defaultOpenBGPDIntegrationImage
	}
	return image
}

type openbgpdIntegrationHarness struct {
	image         string
	containerName string
	workDir       string
	fcgiSocket    string
	server        *httptest.Server
}

func newOpenBGPDIntegrationHarness(t *testing.T, image string) *openbgpdIntegrationHarness {
	return newOpenBGPDIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfig)
}

func newOpenBGPDIntegrationHarnessWithConfig(t *testing.T, image, config string) *openbgpdIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bgpd.conf"), []byte(config), 0o644))

	h := &openbgpdIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-openbgpd-it-%d", time.Now().UnixNano()),
		workDir:       workDir,
		fcgiSocket:    filepath.Join(workDir, "bgplgd.sock"),
	}

	_, err := runDockerCommand(3*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.image,
		"sh", "-lc", openbgpdIntegrationRunCommand,
	)
	require.NoError(t, err)

	h.server = httptest.NewServer(newOpenBGPDFastCGIProxy(h.fcgiSocket, 15*time.Second))

	t.Cleanup(func() {
		if h.server != nil {
			h.server.Close()
		}

		if t.Failed() {
			t.Logf("OpenBGPD container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("OpenBGPD show neighbor:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show neighbor || true"))
			t.Logf("OpenBGPD show rib detail:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show rib detail || true"))
			t.Logf("OpenBGPD config:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/bgpd.conf || true"))
			t.Logf("OpenBGPD bgpd log:\n%s", h.hostFile("bgpd.log"))
			t.Logf("OpenBGPD bgplgd log:\n%s", h.hostFile("bgplgd.log"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *openbgpdIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"test -S /run/bgpd.rsock && test -S /work/bgplgd.sock && bgpctl -j -s /run/bgpd.rsock show neighbor >/dev/null && bgpctl -j -s /run/bgpd.rsock show rib detail >/dev/null",
		); err != nil {
			return false
		}
		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"chmod 666 /work/bgplgd.sock >/dev/null 2>&1 || true",
		); err != nil {
			return false
		}
		return true
	}, 3*time.Minute, 500*time.Millisecond, "OpenBGPD container did not become ready")
}

func (h *openbgpdIntegrationHarness) apiURL() string {
	if h.server == nil {
		return ""
	}
	return h.server.URL
}

func (h *openbgpdIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func (h *openbgpdIntegrationHarness) hostFile(name string) string {
	data, err := os.ReadFile(filepath.Join(h.workDir, name))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

const openbgpdIntegrationRunCommand = `
set -e
apt-get update >/dev/null
DEBIAN_FRONTEND=noninteractive apt-get install -y openbgpd >/dev/null
mkdir -p /run/openbgpd /var/www/run
chown _openbgpd:_openbgpd /run/openbgpd
chown _bgplgd:_bgplgd /var/www/run
bgpd -dvf /work/bgpd.conf >/work/bgpd.log 2>&1 &
bgpd_pid=$!
bgplgd -d -s /work/bgplgd.sock -S /run/bgpd.rsock >/work/bgplgd.log 2>&1 &
bgplgd_pid=$!
for _ in $(seq 1 100); do
    if [ -S /work/bgplgd.sock ]; then
        chmod 666 /work/bgplgd.sock || true
        break
    fi
    sleep 0.1
done
trap 'kill "$bgpd_pid" "$bgplgd_pid" 2>/dev/null || true' EXIT INT TERM
wait "$bgpd_pid" "$bgplgd_pid"
`

const openbgpdIntegrationConfig = `AS 65010
router-id 192.0.2.254
fib-update no
socket "/run/bgpd.rsock" restricted
listen on 127.0.0.1 port 1179
network 203.0.113.0/24
neighbor 192.0.2.2 {
    remote-as 65020
    passive
    descr "test-peer"
}
`

const openbgpdIntegrationConfigDualStack = `AS 65010
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
}
`
