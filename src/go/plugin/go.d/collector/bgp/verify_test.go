// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"os"
	"os/exec"
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
	bgpFRRIntegrationEnableEnv = "BGP_FRR_ENABLE_INTEGRATION"
	bgpFRRIntegrationImageEnv  = "BGP_FRR_DOCKER_IMAGE"
	defaultFRRIntegrationImage = "quay.io/frrouting/frr:10.6.0"
)

func TestIntegration_FRRLiveCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FRR live integration test requires Linux")
	}

	image := getFRRIntegrationImage(t)
	requireDocker(t)

	harness := newFRRIntegrationHarness(t, image)
	harness.waitReady(t)
	harness.setupEVPNVNI(t)

	collr := New()
	collr.SocketPath = harness.socketPath()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxPeers = 10
	collr.MaxVNIs = 10

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	peerID := makePeerID("default_ipv4_unicast", "192.0.2.2")
	neighborID := makeNeighborID("default", "192.0.2.2")
	evpnPeerID := makePeerID("default_l2vpn_evpn", "192.0.2.2")
	vniID := makeVNIID("default", 100, "l2")

	for i := 0; i < 2; i++ {
		mx := collr.Collect(context.Background())
		require.NotNil(t, mx)
		mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

		assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_established"])
		assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_admin_down"])
		assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_down"])
		assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_configured"])
		assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_charted"])
		assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_received"])
		assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_sent"])
		assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_prefixes_received"])
		assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_rib_routes"])
		assert.Equal(t, int64(0), mx["family_default_l2vpn_evpn_peers_established"])
		assert.Equal(t, int64(0), mx["family_default_l2vpn_evpn_peers_admin_down"])
		assert.Equal(t, int64(1), mx["family_default_l2vpn_evpn_peers_down"])
		assert.Equal(t, int64(1), mx["family_default_l2vpn_evpn_peers_configured"])
		assert.Equal(t, int64(1), mx["family_default_l2vpn_evpn_peers_charted"])
		assert.Equal(t, int64(0), mx["family_default_l2vpn_evpn_messages_received"])
		assert.Equal(t, int64(0), mx["family_default_l2vpn_evpn_messages_sent"])
		assert.Equal(t, int64(0), mx["family_default_l2vpn_evpn_prefixes_received"])
		assert.Equal(t, int64(1), mx["family_default_l2vpn_evpn_rib_routes"])

		assert.Equal(t, int64(0), mx["peer_"+peerID+"_messages_received"])
		assert.Equal(t, int64(0), mx["peer_"+peerID+"_messages_sent"])
		assert.Equal(t, int64(0), mx["peer_"+peerID+"_prefixes_received"])
		assert.Equal(t, int64(peerStateDown), mx["peer_"+peerID+"_state"])
		assert.Contains(t, mx, "peer_"+peerID+"_uptime_seconds")
		assert.Equal(t, int64(0), mx["peer_"+evpnPeerID+"_messages_received"])
		assert.Equal(t, int64(0), mx["peer_"+evpnPeerID+"_messages_sent"])
		assert.Equal(t, int64(0), mx["peer_"+evpnPeerID+"_prefixes_received"])
		assert.Equal(t, int64(peerStateDown), mx["peer_"+evpnPeerID+"_state"])
		assert.Contains(t, mx, "peer_"+evpnPeerID+"_uptime_seconds")
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_connections_established"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_connections_dropped"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_updates_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_updates_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_notifications_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_notifications_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_route_refresh_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_churn_route_refresh_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_updates_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_updates_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_notifications_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_notifications_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_keepalives_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_keepalives_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_route_refresh_received"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_route_refresh_sent"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_reset_never"])
		assert.Equal(t, int64(1), mx["neighbor_"+neighborID+"_last_reset_soft_or_unknown"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_reset_hard"])
		assert.GreaterOrEqual(t, mx["neighbor_"+neighborID+"_last_reset_age_seconds"], int64(0))
		assert.Greater(t, mx["neighbor_"+neighborID+"_last_reset_code"], int64(0))
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_error_code"])
		assert.Equal(t, int64(0), mx["neighbor_"+neighborID+"_last_error_subcode"])
		assert.Equal(t, int64(0), mx["vni_"+vniID+"_macs"])
		assert.Equal(t, int64(0), mx["vni_"+vniID+"_arp_nd"])
		assert.Equal(t, int64(0), mx["vni_"+vniID+"_remote_vteps"])
	}

	require.NotNil(t, collr.Charts().Get("family_default_ipv4_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get("family_default_l2vpn_evpn_peer_states"))
	require.NotNil(t, collr.Charts().Get("peer_"+peerID+"_messages"))
	require.NotNil(t, collr.Charts().Get("peer_"+evpnPeerID+"_messages"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_transitions"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_churn"))
	require.NotNil(t, collr.Charts().Get("neighbor_"+neighborID+"_message_types"))
	require.NotNil(t, collr.Charts().Get(neighborLastResetStateChartID(neighborID)))
	require.NotNil(t, collr.Charts().Get("vni_"+vniID+"_entries"))
	require.NotNil(t, collr.Charts().Get("vni_"+vniID+"_remote_vteps"))
}

func getFRRIntegrationImage(t *testing.T) string {
	t.Helper()

	switch strings.ToLower(strings.TrimSpace(os.Getenv(bgpFRRIntegrationEnableEnv))) {
	case "1", "true", "yes", "on":
	default:
		t.Skipf("%s is not enabled", bgpFRRIntegrationEnableEnv)
	}

	image := strings.TrimSpace(os.Getenv(bgpFRRIntegrationImageEnv))
	if image == "" {
		image = defaultFRRIntegrationImage
	}
	return image
}

func requireDocker(t *testing.T) {
	t.Helper()

	_, err := exec.LookPath("docker")
	require.NoError(t, err, "docker binary is required")
}

type frrIntegrationHarness struct {
	image         string
	containerName string
	etcDir        string
	runDir        string
}

func newFRRIntegrationHarness(t *testing.T, image string) *frrIntegrationHarness {
	t.Helper()

	baseDir := t.TempDir()
	etcDir := filepath.Join(baseDir, "etc-frr")
	runDir := filepath.Join(baseDir, "run-frr")

	require.NoError(t, os.MkdirAll(etcDir, 0o755))
	require.NoError(t, os.MkdirAll(runDir, 0o777))
	require.NoError(t, os.Chmod(runDir, 0o777))

	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "daemons"), []byte(frrIntegrationDaemons), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "frr.conf"), []byte(frrIntegrationConfig(image)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "vtysh.conf"), nil, 0o644))

	h := &frrIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-frr-it-%d", time.Now().UnixNano()),
		etcDir:        etcDir,
		runDir:        runDir,
	}

	_, err := runDockerCommand(30*time.Second,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--privileged",
		"--volume", fmt.Sprintf("%s:/etc/frr", h.etcDir),
		"--volume", fmt.Sprintf("%s:/var/run/frr", h.runDir),
		h.image,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("FRR container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("FRR runtime directory:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "ls -la /var/run/frr || true"))
			t.Logf("FRR config:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /etc/frr/daemons; printf '\\n'; cat /etc/frr/frr.conf"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *frrIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	socketPath := h.socketPath()
	require.Eventually(t, func() bool {
		if _, err := os.Stat(socketPath); err != nil {
			return false
		}

		if _, err := runDockerCommand(10*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"test -S /var/run/frr/bgpd.vty && vtysh -c 'show bgp vrf all ipv4 summary json' >/dev/null && vtysh -c 'show bgp vrf all l2vpn evpn summary json' >/dev/null",
		); err != nil {
			return false
		}

		if _, err := runDockerCommand(10*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"chmod 0777 /var/run/frr && chmod 0666 /var/run/frr/*.vty",
		); err != nil {
			return false
		}

		return true
	}, 45*time.Second, 500*time.Millisecond, "FRR container did not become ready")
}

func (h *frrIntegrationHarness) setupEVPNVNI(t *testing.T) {
	t.Helper()

	_, err := runDockerCommand(15*time.Second,
		"exec", h.containerName, "sh", "-lc",
		"ip addr add 10.0.0.1/32 dev lo || true && ip link add br100 type bridge && ip link set br100 up && ip link add vxlan100 type vxlan id 100 dstport 4789 local 10.0.0.1 nolearning && ip link set vxlan100 up && ip link set vxlan100 master br100",
	)
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "vtysh", "-c", "show evpn vni json")
		return err == nil && strings.Contains(out, "\"100\"")
	}, 20*time.Second, 500*time.Millisecond, "FRR did not expose EVPN VNI JSON")
}

func (h *frrIntegrationHarness) socketPath() string {
	return filepath.Join(h.runDir, "bgpd.vty")
}

func (h *frrIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(10*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func runDockerCommand(timeout time.Duration, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	output := strings.TrimSpace(string(out))

	if ctx.Err() == context.DeadlineExceeded {
		return output, fmt.Errorf("docker %s timed out: %w: %s", strings.Join(args, " "), ctx.Err(), output)
	}
	if err != nil {
		if output == "" {
			return "", fmt.Errorf("docker %s: %w", strings.Join(args, " "), err)
		}
		return output, fmt.Errorf("docker %s: %w: %s", strings.Join(args, " "), err, output)
	}

	return output, nil
}

func frrIntegrationConfig(image string) string {
	version := strings.TrimSpace(strings.TrimPrefix(image, "quay.io/frrouting/frr:"))
	if version == "" || version == image {
		version = "10.6.0"
	}

	return fmt.Sprintf(`frr version %s
frr defaults traditional
hostname frr-test
service integrated-vtysh-config
!
router bgp 65001
 bgp router-id 10.0.0.1
 no bgp ebgp-requires-policy
 neighbor 192.0.2.2 remote-as 65002
 !
 address-family ipv4 unicast
  neighbor 192.0.2.2 activate
 exit-address-family
 !
 address-family l2vpn evpn
  neighbor 192.0.2.2 activate
  advertise-all-vni
 exit-address-family
!
line vty
`, version)
}

const frrIntegrationDaemons = `zebra=yes
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
