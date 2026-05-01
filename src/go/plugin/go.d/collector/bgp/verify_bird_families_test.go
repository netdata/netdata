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

func TestIntegration_BIRDLiveAdvancedFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("BIRD live integration test requires Linux")
	}

	switch strings.ToLower(strings.TrimSpace(os.Getenv(bgpBIRDIntegrationEnableEnv))) {
	case "1", "true", "yes", "on":
	default:
		t.Skipf("%s is not enabled", bgpBIRDIntegrationEnableEnv)
	}

	requireDocker(t)

	image := strings.TrimSpace(os.Getenv(bgpBIRD3IntegrationImageEnv))
	if image == "" {
		image = defaultBIRD3IntegrationImage
	}

	harness := newBIRDAdvancedFamiliesHarness(t, image)
	harness.waitReady(t)

	raw := harness.dockerOutput("exec", harness.containerName, "sh", "-lc", "birdc -s /work/netdata.ctl show protocols all")
	assert.Contains(t, raw, "Channel ipv4-mc")
	assert.Contains(t, raw, "Channel ipv6-mc")
	assert.Contains(t, raw, "Channel flow4")
	assert.Contains(t, raw, "Channel flow6")

	collr := New()
	collr.Backend = backendBIRD
	collr.SocketPath = harness.collectorSocketPath()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxPeers = 10
	collr.MaxFamilies = 16

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	for _, familyID := range []string{
		"master4_ipv4_unicast",
		"master6_ipv6_unicast",
		"mcast4_ipv4_multicast",
		"mcast6_ipv6_multicast",
		"flow4tab_ipv4_flowspec",
		"flow6tab_ipv6_flowspec",
	} {
		assert.Equalf(t, int64(1), mx["family_"+familyID+"_peers_configured"], "configured peers for %s", familyID)
		assert.Equalf(t, int64(1), mx["family_"+familyID+"_peers_charted"], "charted peers for %s", familyID)
		assert.Equalf(t, int64(1), mx["family_"+familyID+"_peers_down"], "down peers for %s", familyID)
		assert.Equalf(t, int64(0), mx["family_"+familyID+"_peers_established"], "established peers for %s", familyID)
	}

	flow4Chart := collr.Charts().Get("family_flow4tab_ipv4_flowspec_peer_states")
	require.NotNil(t, flow4Chart)
	assert.Equal(t, "flow4tab", chartLabelValue(flow4Chart, "vrf"))
	assert.Equal(t, "ipv4", chartLabelValue(flow4Chart, "afi"))
	assert.Equal(t, "flowspec", chartLabelValue(flow4Chart, "safi"))

	flow6PeerID := makePeerIDWithScope("flow6tab_ipv6_flowspec", "198.51.100.2", "bgp_adv")
	peerChart := collr.Charts().Get("peer_" + flow6PeerID + "_messages")
	require.NotNil(t, peerChart)
	assert.Equal(t, "flow6tab", chartLabelValue(peerChart, "table"))
	assert.Equal(t, "bgp_adv", chartLabelValue(peerChart, "protocol"))
}

type birdAdvancedFamiliesHarness struct {
	image         string
	containerName string
	workDir       string
}

func newBIRDAdvancedFamiliesHarness(t *testing.T, image string) *birdAdvancedFamiliesHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bird.conf"), []byte(birdAdvancedFamiliesConfig()), 0o644))

	h := &birdAdvancedFamiliesHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-bird-advanced-it-%d", time.Now().UnixNano()),
		workDir:       workDir,
	}

	_, err := runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.image,
		"sh", "-lc", birdIntegrationRunCommand("bird3"),
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("bird3 advanced container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("bird3 advanced show protocols all:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "birdc -s /work/netdata.ctl show protocols all || true"))
			t.Logf("bird3 advanced config:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/bird.conf || true"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *birdAdvancedFamiliesHarness) waitReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := os.Stat(h.collectorSocketPath()); err != nil {
			return false
		}

		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"birdc -s /work/netdata.ctl show protocols all >/dev/null",
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
	}, 2*time.Minute, 500*time.Millisecond, "bird3 advanced-families container did not become ready")
}

func (h *birdAdvancedFamiliesHarness) collectorSocketPath() string {
	return filepath.Join(h.workDir, "netdata.ctl")
}

func (h *birdAdvancedFamiliesHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func birdAdvancedFamiliesConfig() string {
	return `router id 192.0.2.1;

ipv4 table master4;
ipv6 table master6;
ipv4 table mcast4;
ipv6 table mcast6;
flow4 table flow4tab;
flow6 table flow6tab;

cli "/work/netdata.ctl" { restrict; };

protocol device {}

protocol static static4 {
  ipv4;
  route 203.0.113.0/24 blackhole;
}

protocol static static6 {
  ipv6;
  route 2001:db8:100::/64 blackhole;
}

protocol bgp bgp_adv {
  description "Advanced families";
  local 192.0.2.1 as 64512;
  neighbor 198.51.100.2 as 65001;
  multihop;
  ipv4 {
    table master4;
    import all;
    export all;
  };
  ipv6 {
    table master6;
    import all;
    export all;
  };
  ipv4 multicast {
    table mcast4;
    import all;
    export all;
  };
  ipv6 multicast {
    table mcast6;
    import all;
    export all;
  };
  flow4 {
    table flow4tab;
    import all;
    export all;
  };
  flow6 {
    table flow6tab;
    import all;
    export all;
  };
}
`
}
