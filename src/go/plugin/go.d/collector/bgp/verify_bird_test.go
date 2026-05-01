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

const (
	bgpBIRDIntegrationEnableEnv = "BGP_BIRD_ENABLE_INTEGRATION"
	bgpBIRD2IntegrationImageEnv = "BGP_BIRD2_DOCKER_IMAGE"
	bgpBIRD3IntegrationImageEnv = "BGP_BIRD3_DOCKER_IMAGE"

	defaultBIRD2IntegrationImage = "debian:bookworm-slim"
	defaultBIRD3IntegrationImage = "debian:trixie-slim"
)

type birdIntegrationCase struct {
	name              string
	image             string
	packageName       string
	collectorSocket   string
	extraSocketConfig string
}

func TestIntegration_BIRDLiveCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("BIRD live integration test requires Linux")
	}

	requireDocker(t)

	for _, tc := range getBIRDIntegrationCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			harness := newBIRDIntegrationHarness(t, tc)
			harness.waitReady(t)

			collr := New()
			collr.Backend = backendBIRD
			collr.SocketPath = harness.collectorSocketPath()
			collr.Timeout = confopt.Duration(5 * time.Second)
			collr.MaxPeers = 10

			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))
			t.Cleanup(func() { collr.Cleanup(context.Background()) })

			mx := collr.Collect(context.Background())
			require.NotNil(t, mx)
			mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

			peer4ID := makePeerIDWithScope("master4_ipv4_unicast", "198.51.100.2", "bgp4")
			peer6ID := makePeerIDWithScope("master6_ipv6_unicast", "2001:db8::2", "bgp6")
			neighbor4ID := makeNeighborIDWithScope("master4", "198.51.100.2", "bgp4")
			neighbor6ID := makeNeighborIDWithScope("master6", "2001:db8::2", "bgp6")

			assert.Equal(t, int64(0), mx["family_master4_ipv4_unicast_peers_established"])
			assert.Equal(t, int64(0), mx["family_master4_ipv4_unicast_peers_admin_down"])
			assert.Equal(t, int64(1), mx["family_master4_ipv4_unicast_peers_down"])
			assert.Equal(t, int64(1), mx["family_master4_ipv4_unicast_peers_configured"])
			assert.Equal(t, int64(1), mx["family_master4_ipv4_unicast_peers_charted"])
			assert.Equal(t, int64(0), mx["family_master4_ipv4_unicast_messages_received"])
			assert.Equal(t, int64(0), mx["family_master4_ipv4_unicast_messages_sent"])
			assert.Equal(t, int64(0), mx["family_master4_ipv4_unicast_prefixes_received"])
			assert.Equal(t, int64(0), mx["family_master4_ipv4_unicast_rib_routes"])

			assert.Equal(t, int64(0), mx["family_master6_ipv6_unicast_peers_established"])
			assert.Equal(t, int64(0), mx["family_master6_ipv6_unicast_peers_admin_down"])
			assert.Equal(t, int64(1), mx["family_master6_ipv6_unicast_peers_down"])
			assert.Equal(t, int64(1), mx["family_master6_ipv6_unicast_peers_configured"])
			assert.Equal(t, int64(1), mx["family_master6_ipv6_unicast_peers_charted"])
			assert.Equal(t, int64(0), mx["family_master6_ipv6_unicast_messages_received"])
			assert.Equal(t, int64(0), mx["family_master6_ipv6_unicast_messages_sent"])
			assert.Equal(t, int64(0), mx["family_master6_ipv6_unicast_prefixes_received"])
			assert.Equal(t, int64(0), mx["family_master6_ipv6_unicast_rib_routes"])

			assert.Equal(t, int64(0), mx["peer_"+peer4ID+"_messages_received"])
			assert.Equal(t, int64(0), mx["peer_"+peer4ID+"_messages_sent"])
			assert.Equal(t, int64(0), mx["peer_"+peer4ID+"_prefixes_received"])
			assert.Equal(t, int64(0), mx["peer_"+peer4ID+"_accepted"])
			assert.Equal(t, int64(0), mx["peer_"+peer4ID+"_filtered"])
			assert.Equal(t, int64(0), mx["peer_"+peer4ID+"_prefixes_advertised"])
			assert.Equal(t, int64(peerStateDown), mx["peer_"+peer4ID+"_state"])
			assert.Contains(t, mx, "peer_"+peer4ID+"_uptime_seconds")

			assert.Equal(t, int64(0), mx["peer_"+peer6ID+"_messages_received"])
			assert.Equal(t, int64(0), mx["peer_"+peer6ID+"_messages_sent"])
			assert.Equal(t, int64(0), mx["peer_"+peer6ID+"_prefixes_received"])
			assert.Equal(t, int64(0), mx["peer_"+peer6ID+"_accepted"])
			assert.Equal(t, int64(0), mx["peer_"+peer6ID+"_filtered"])
			assert.Equal(t, int64(0), mx["peer_"+peer6ID+"_prefixes_advertised"])
			assert.Equal(t, int64(peerStateDown), mx["peer_"+peer6ID+"_state"])
			assert.Contains(t, mx, "peer_"+peer6ID+"_uptime_seconds")

			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_updates_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_updates_sent"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_withdraws_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_withdraws_sent"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_notifications_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_notifications_sent"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_route_refresh_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor4ID+"_churn_route_refresh_sent"])

			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_updates_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_updates_sent"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_withdraws_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_withdraws_sent"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_notifications_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_notifications_sent"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_route_refresh_received"])
			assert.Equal(t, int64(0), mx["neighbor_"+neighbor6ID+"_churn_route_refresh_sent"])

			require.NotNil(t, collr.Charts().Get("family_master4_ipv4_unicast_peer_states"))
			require.NotNil(t, collr.Charts().Get("family_master6_ipv6_unicast_peer_states"))
			require.NotNil(t, collr.Charts().Get("peer_"+peer4ID+"_messages"))
			require.NotNil(t, collr.Charts().Get(peerAdvertisedPrefixesChartID(peer4ID)))
			require.NotNil(t, collr.Charts().Get(peerPolicyChartID(peer4ID)))
			require.NotNil(t, collr.Charts().Get("neighbor_"+neighbor4ID+"_churn"))

			peerChart := collr.Charts().Get("peer_" + peer4ID + "_messages")
			require.NotNil(t, peerChart)
			assert.Equal(t, "master4", chartLabelValue(peerChart, "table"))
			assert.Equal(t, "bgp4", chartLabelValue(peerChart, "protocol"))

			neighborChart := collr.Charts().Get("neighbor_" + neighbor4ID + "_churn")
			require.NotNil(t, neighborChart)
			assert.Equal(t, "master4", chartLabelValue(neighborChart, "table"))
			assert.Equal(t, "bgp4", chartLabelValue(neighborChart, "protocol"))
		})
	}
}

func getBIRDIntegrationCases(t *testing.T) []birdIntegrationCase {
	t.Helper()

	switch strings.ToLower(strings.TrimSpace(os.Getenv(bgpBIRDIntegrationEnableEnv))) {
	case "1", "true", "yes", "on":
	default:
		t.Skipf("%s is not enabled", bgpBIRDIntegrationEnableEnv)
	}

	bird2Image := strings.TrimSpace(os.Getenv(bgpBIRD2IntegrationImageEnv))
	if bird2Image == "" {
		bird2Image = defaultBIRD2IntegrationImage
	}

	bird3Image := strings.TrimSpace(os.Getenv(bgpBIRD3IntegrationImageEnv))
	if bird3Image == "" {
		bird3Image = defaultBIRD3IntegrationImage
	}

	return []birdIntegrationCase{
		{
			name:            "bird2",
			image:           bird2Image,
			packageName:     "bird2",
			collectorSocket: "main.ctl",
		},
		{
			name:              "bird3",
			image:             bird3Image,
			packageName:       "bird3",
			collectorSocket:   "netdata.ctl",
			extraSocketConfig: `cli "/work/netdata.ctl" { restrict; };`,
		},
	}
}

type birdIntegrationHarness struct {
	tc            birdIntegrationCase
	containerName string
	workDir       string
}

func newBIRDIntegrationHarness(t *testing.T, tc birdIntegrationCase) *birdIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bird.conf"), []byte(birdIntegrationConfig(tc)), 0o644))

	h := &birdIntegrationHarness{
		tc:            tc,
		containerName: fmt.Sprintf("netdata-bgp-%s-it-%d", tc.name, time.Now().UnixNano()),
		workDir:       workDir,
	}

	_, err := runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.tc.image,
		"sh", "-lc", birdIntegrationRunCommand(h.tc.packageName),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("%s container logs:\n%s", h.tc.name, h.dockerOutput("logs", h.containerName))
			t.Logf("%s show protocols all:\n%s", h.tc.name, h.dockerOutput("exec", h.containerName, "sh", "-lc", fmt.Sprintf("birdc -s /work/%s show protocols all || true", h.tc.collectorSocket)))
			t.Logf("%s config:\n%s", h.tc.name, h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/bird.conf || true"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *birdIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := os.Stat(h.collectorSocketPath()); err != nil {
			return false
		}

		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			fmt.Sprintf("birdc -s /work/%s show protocols all >/dev/null", h.tc.collectorSocket),
		); err != nil {
			return false
		}

		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"chmod 0777 /work && chmod 0666 /work/*.ctl",
		); err != nil {
			return false
		}

		return true
	}, 2*time.Minute, 500*time.Millisecond, "BIRD container %s did not become ready", h.tc.name)
}

func (h *birdIntegrationHarness) collectorSocketPath() string {
	return filepath.Join(h.workDir, h.tc.collectorSocket)
}

func (h *birdIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func birdIntegrationRunCommand(pkg string) string {
	return fmt.Sprintf(
		"DEBIAN_FRONTEND=noninteractive apt-get update >/dev/null && "+
			"DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends %s >/dev/null && "+
			"exec bird -f -c /work/bird.conf -s /work/main.ctl",
		pkg,
	)
}

func birdIntegrationConfig(tc birdIntegrationCase) string {
	extraSocket := ""
	if tc.extraSocketConfig != "" {
		extraSocket = tc.extraSocketConfig + "\n\n"
	}

	return fmt.Sprintf(`router id 192.0.2.1;

ipv4 table master4;
ipv6 table master6;

protocol device {}

protocol static static4 {
  ipv4;
  route 203.0.113.0/24 blackhole;
}

protocol static static6 {
  ipv6;
  route 2001:db8:100::/64 blackhole;
}

%sprotocol bgp bgp4 {
  description "IPv4 upstream";
  local 192.0.2.1 as 64512;
  neighbor 198.51.100.2 as 65001;
  multihop;
  ipv4 {
    table master4;
    import all;
    export all;
  };
}

protocol bgp bgp6 {
  description "IPv6 upstream";
  local 2001:db8::1 as 64512;
  neighbor 2001:db8::2 as 65001;
  multihop;
  ipv6 {
    table master6;
    import all;
    export all;
  };
}
`, extraSocket)
}
