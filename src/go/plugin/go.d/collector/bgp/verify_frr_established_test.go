// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_FRRLiveEstablishedCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FRR live established-session integration test requires Linux")
	}

	image := getFRRIntegrationImage(t)
	requireDocker(t)

	harness := newFRREstablishedIntegrationHarness(t, image)
	harness.waitReady(t)

	collr := New()
	collr.SocketPath = harness.socketPath()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxPeers = 10
	collr.DeepPeerPrefixMetrics = true

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	familyID := makeFamilyID("default", "ipv4", "unicast")
	peerID := makePeerID(familyID, harness.peerAddress())
	neighborID := makeNeighborID("default", harness.peerAddress())

	assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_established"])
	assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
	assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_down"])
	assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
	assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
	assert.Equal(t, int64(1), mx["family_"+familyID+"_prefixes_received"])
	assert.GreaterOrEqual(t, mx["family_"+familyID+"_rib_routes"], int64(2))

	assert.Equal(t, int64(1), mx["peer_"+peerID+"_prefixes_received"])
	assert.Equal(t, int64(1), mx["peer_"+peerID+"_prefixes_accepted"])
	assert.Equal(t, int64(0), mx["peer_"+peerID+"_prefixes_filtered"])
	assert.Greater(t, mx["peer_"+peerID+"_prefixes_advertised"], int64(0))
	assert.Equal(t, int64(peerStateUp), mx["peer_"+peerID+"_state"])
	assert.Greater(t, mx["peer_"+peerID+"_uptime_seconds"], int64(0))

	assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_connections_established"])
	assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_connections_dropped"])
	assert.Greater(t, mx["neighbor_"+neighborID+"_updates_received"], int64(0))
	assert.Greater(t, mx["neighbor_"+neighborID+"_updates_sent"], int64(0))
	assert.Greater(t, mx["neighbor_"+neighborID+"_keepalives_received"], int64(0))
	assert.Greater(t, mx["neighbor_"+neighborID+"_keepalives_sent"], int64(0))

	require.NotNil(t, collr.Charts().Get("family_"+familyID+"_peer_states"))
	require.NotNil(t, collr.Charts().Get("peer_"+peerID+"_prefixes_policy"))
	require.NotNil(t, collr.Charts().Get("peer_"+peerID+"_prefixes_advertised"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_message_types"))
}

func TestIntegration_FRRLiveEstablishedPeerRouteCommands(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FRR live established-session integration test requires Linux")
	}

	image := getFRRIntegrationImage(t)
	requireDocker(t)

	harness := newFRREstablishedIntegrationHarness(t, image)
	harness.waitReady(t)

	client := &frrClient{
		socketPath: harness.socketPath(),
		timeout:    5 * time.Second,
	}
	t.Cleanup(func() { _ = client.Close() })

	routes, err := client.PeerRoutes("default", "ipv4", "unicast", harness.peerAddress())
	require.NoError(t, err)

	accepted, err := parseFRRPrefixCounter(routes)
	require.NoError(t, err)
	assert.Equal(t, int64(1), accepted)

	advertised, err := client.PeerAdvertisedRoutes("default", "ipv4", "unicast", harness.peerAddress())
	require.NoError(t, err)

	advertisedCount, err := parseFRRPrefixCounter(advertised)
	require.NoError(t, err)
	assert.Greater(t, advertisedCount, int64(0))
}

func TestIntegration_FRRLiveWithdrawActivityStillHasNoWithdrawCounters(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FRR live established-session integration test requires Linux")
	}

	image := getFRRIntegrationImage(t)
	requireDocker(t)

	harness := newFRREstablishedIntegrationHarness(t, image)
	harness.waitReady(t)

	collr := New()
	collr.SocketPath = harness.socketPath()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxPeers = 10
	collr.DeepPeerPrefixMetrics = true

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	familyID := makeFamilyID("default", "ipv4", "unicast")
	peerID := makePeerID(familyID, harness.peerAddress())
	neighborID := makeNeighborID("default", harness.peerAddress())

	first := collr.Collect(context.Background())
	require.NotNil(t, first)
	first = assertAndStripCollectorMetrics(t, first, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(1), first["family_"+familyID+"_prefixes_received"])
	assert.Equal(t, int64(1), first["peer_"+peerID+"_prefixes_received"])

	harness.withdrawPeerNetwork(t, "203.0.113.0/24")

	var neighborsJSON string
	require.Eventually(t, func() bool {
		summary, err := runDockerCommand(10*time.Second, "exec", harness.containerName, "vtysh", "-c", "show bgp ipv4 unicast summary json")
		if err != nil {
			return false
		}
		neighbors, err := runDockerCommand(10*time.Second, "exec", harness.containerName, "vtysh", "-c", "show bgp vrf all neighbors json")
		if err != nil {
			return false
		}
		if !strings.Contains(summary, "\"state\":\"Established\"") || !strings.Contains(summary, "\"pfxRcd\":0") {
			return false
		}
		neighborsJSON = neighbors
		return true
	}, 90*time.Second, 500*time.Millisecond, "FRR peer did not withdraw the announced route while staying established")

	assert.Contains(t, neighborsJSON, "\"withdrawn\":0")

	second := collr.Collect(context.Background())
	require.NotNil(t, second)
	second = assertAndStripCollectorMetrics(t, second, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	assert.Equal(t, int64(0), second["family_"+familyID+"_prefixes_received"])
	assert.Equal(t, int64(0), second["peer_"+peerID+"_prefixes_received"])
	assert.Equal(t, int64(0), second["peer_"+peerID+"_prefixes_accepted"])
	assert.Greater(t, second["neighbor_"+neighborID+"_updates_received"], first["neighbor_"+neighborID+"_updates_received"])
	assert.Equal(t, int64(0), second["neighbor_"+neighborID+"_churn_withdraws_received"])
	assert.Equal(t, int64(0), second["neighbor_"+neighborID+"_churn_withdraws_sent"])
}

type frrEstablishedIntegrationHarness struct {
	image         string
	containerName string
	peerName      string
	etcDir        string
	runDir        string
	peerEtcDir    string
	peerRunDir    string
	networkName   string
	localIP       string
	remoteIP      string
}

var frrEstablishedAdvertisedPrefixesRe = regexp.MustCompile(`"pfxSnt":[1-9][0-9]*`)
var frrEstablishedNeighborSentPrefixesRe = regexp.MustCompile(`"sentPrefixCounter":[1-9][0-9]*`)

func newFRREstablishedIntegrationHarness(t *testing.T, image string) *frrEstablishedIntegrationHarness {
	t.Helper()

	baseDir := t.TempDir()
	etcDir := filepath.Join(baseDir, "etc-frr")
	runDir := filepath.Join(baseDir, "run-frr")
	peerEtcDir := filepath.Join(baseDir, "peer-etc-frr")
	peerRunDir := filepath.Join(baseDir, "peer-run-frr")

	for _, dir := range []string{etcDir, runDir, peerEtcDir, peerRunDir} {
		perm := os.FileMode(0o755)
		if strings.Contains(dir, "run-frr") {
			perm = 0o777
		}
		require.NoError(t, os.MkdirAll(dir, perm))
	}
	require.NoError(t, os.Chmod(runDir, 0o777))
	require.NoError(t, os.Chmod(peerRunDir, 0o777))

	subnet, localIP, remoteIP := createFRREstablishedSubnet(t)

	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "daemons"), []byte(frrEstablishedIntegrationDaemons), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(peerEtcDir, "daemons"), []byte(frrEstablishedIntegrationDaemons), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "frr.conf"), []byte(frrEstablishedIntegrationConfig(image, "frr-a", 65001, 65002, "10.0.0.1", remoteIP, "198.51.100.0/24")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(peerEtcDir, "frr.conf"), []byte(frrEstablishedIntegrationConfig(image, "frr-b", 65002, 65001, "10.0.0.2", localIP, "203.0.113.0/24")), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "vtysh.conf"), nil, 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(peerEtcDir, "vtysh.conf"), nil, 0o644))

	h := &frrEstablishedIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-frr-established-a-%d", time.Now().UnixNano()),
		peerName:      fmt.Sprintf("netdata-bgp-frr-established-b-%d", time.Now().UnixNano()),
		etcDir:        etcDir,
		runDir:        runDir,
		peerEtcDir:    peerEtcDir,
		peerRunDir:    peerRunDir,
		networkName:   fmt.Sprintf("netdata-bgp-established-%d", time.Now().UnixNano()),
		localIP:       localIP,
		remoteIP:      remoteIP,
	}

	_, err := runDockerCommand(30*time.Second, "network", "create", "--subnet", subnet, h.networkName)
	require.NoError(t, err)

	_, err = runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--privileged",
		"--network", h.networkName,
		"--ip", h.localIP,
		"--volume", fmt.Sprintf("%s:/etc/frr", h.etcDir),
		"--volume", fmt.Sprintf("%s:/var/run/frr", h.runDir),
		h.image,
	)
	require.NoError(t, err)

	_, err = runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.peerName,
		"--privileged",
		"--network", h.networkName,
		"--ip", h.remoteIP,
		"--volume", fmt.Sprintf("%s:/etc/frr", h.peerEtcDir),
		"--volume", fmt.Sprintf("%s:/var/run/frr", h.peerRunDir),
		h.image,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("FRR established container logs (%s):\n%s", h.containerName, h.dockerOutput("logs", h.containerName))
			t.Logf("FRR established container logs (%s):\n%s", h.peerName, h.dockerOutput("logs", h.peerName))
			t.Logf("FRR established config (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /etc/frr/daemons; printf '\\n'; cat /etc/frr/frr.conf"))
			t.Logf("FRR established config (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "sh", "-lc", "cat /etc/frr/daemons; printf '\\n'; cat /etc/frr/frr.conf"))
			t.Logf("FRR summary (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "vtysh", "-c", "show bgp ipv4 unicast summary json"))
			t.Logf("FRR neighbors (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "vtysh", "-c", "show bgp vrf all neighbors json"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName, h.peerName)
		_, _ = runDockerCommand(15*time.Second, "network", "rm", h.networkName)
	})

	return h
}

func (h *frrEstablishedIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	socketPath := h.socketPath()
	require.Eventually(t, func() bool {
		if _, err := os.Stat(socketPath); err != nil {
			return false
		}

		summary, err := runDockerCommand(10*time.Second, "exec", h.containerName, "vtysh", "-c", "show bgp ipv4 unicast summary json")
		if err != nil {
			return false
		}
		neighbors, err := runDockerCommand(10*time.Second, "exec", h.containerName, "vtysh", "-c", "show bgp vrf all neighbors json")
		if err != nil {
			return false
		}

		if _, err := runDockerCommand(10*time.Second, "exec", h.containerName, "sh", "-lc", "chmod 0777 /var/run/frr && chmod 0666 /var/run/frr/*.vty"); err != nil {
			return false
		}

		return strings.Contains(summary, "\"state\":\"Established\"") &&
			strings.Contains(summary, "\"pfxRcd\":1") &&
			frrEstablishedAdvertisedPrefixesRe.MatchString(summary) &&
			strings.Contains(neighbors, "\"acceptedPrefixCounter\":1") &&
			frrEstablishedNeighborSentPrefixesRe.MatchString(neighbors)
	}, 90*time.Second, 500*time.Millisecond, "FRR established integration peers did not reach the expected routed state")
}

func (h *frrEstablishedIntegrationHarness) socketPath() string {
	return filepath.Join(h.runDir, "bgpd.vty")
}

func (h *frrEstablishedIntegrationHarness) peerAddress() string {
	return h.remoteIP
}

func (h *frrEstablishedIntegrationHarness) withdrawPeerNetwork(t *testing.T, prefix string) {
	t.Helper()

	_, err := runDockerCommand(15*time.Second,
		"exec", h.peerName, "vtysh",
		"-c", "configure terminal",
		"-c", "router bgp 65002",
		"-c", "address-family ipv4 unicast",
		"-c", "no network "+prefix,
	)
	require.NoError(t, err)
}

func (h *frrEstablishedIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(10*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func createFRREstablishedSubnet(t *testing.T) (subnet, localIP, remoteIP string) {
	t.Helper()

	rng := rand.New(rand.NewSource(time.Now().UnixNano()))
	for i := 0; i < 16; i++ {
		subnet = fmt.Sprintf("10.%d.%d.0/24", rng.Intn(200)+20, rng.Intn(200))
		localIP = strings.TrimSuffix(subnet, "0/24") + "2"
		remoteIP = strings.TrimSuffix(subnet, "0/24") + "3"
		return subnet, localIP, remoteIP
	}

	t.Fatalf("failed to generate FRR integration subnet")
	return "", "", ""
}

func frrEstablishedIntegrationConfig(image, hostname string, localAS, remoteAS int, routerID, neighborIP, networkPrefix string) string {
	version := strings.TrimSpace(strings.TrimPrefix(image, "quay.io/frrouting/frr:"))
	if version == "" || version == image {
		version = "10.6.0"
	}

	return fmt.Sprintf(`frr version %s
frr defaults traditional
hostname %s
service integrated-vtysh-config
route-map ALLOW-ALL permit 10
ip route %s Null0
router bgp %d
 bgp router-id %s
 no bgp ebgp-requires-policy
 neighbor %s remote-as %d
 address-family ipv4 unicast
  network %s
  neighbor %s activate
  neighbor %s route-map ALLOW-ALL in
  neighbor %s route-map ALLOW-ALL out
 exit-address-family
line vty
`, version, hostname, networkPrefix, localAS, routerID, neighborIP, remoteAS, networkPrefix, neighborIP, neighborIP, neighborIP)
}

const frrEstablishedIntegrationDaemons = `zebra=yes
bgpd=yes
ospfd=no
ospf6d=no
ripd=no
ripngd=no
isisd=no
pimd=no
pim6d=no
ldpd=no
nhrpd=no
eigrpd=no
babeld=no
sharpd=no
pbrd=no
bfdd=no
fabricd=no
vrrpd=no
pathd=no
vtysh_enable=yes
zebra_options="  -A 127.0.0.1 -s 90000000"
mgmtd_options="  -A 127.0.0.1"
bgpd_options="   -A 127.0.0.1"
staticd_options="-A 127.0.0.1"
`
