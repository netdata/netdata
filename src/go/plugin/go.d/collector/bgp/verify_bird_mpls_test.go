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

func TestIntegration_BIRDLiveMPLSVPNFamilies(t *testing.T) {
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

	harness := newBIRDMPLSFamiliesHarness(t, image)
	harness.waitReady(t)

	raw := harness.dockerOutput("exec", harness.containerName, "sh", "-lc", "birdc -s /work/netdata.ctl show protocols all")
	assert.Contains(t, raw, "Channel ipv4-mpls")
	assert.Contains(t, raw, "Channel ipv6-mpls")
	assert.Contains(t, raw, "Channel vpn4-mpls")
	assert.Contains(t, raw, "Channel vpn6-mpls")
	assert.Contains(t, raw, "Channel mpls")

	collr := New()
	collr.Backend = backendBIRD
	collr.SocketPath = harness.collectorSocketPath()
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxPeers = 10
	collr.MaxFamilies = 8

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	for _, familyID := range []string{
		"label4_ipv4_label",
		"label6_ipv6_label",
		"vpn4tab_ipv4_vpn",
		"vpn6tab_ipv6_vpn",
	} {
		assert.Equalf(t, int64(1), mx["family_"+familyID+"_peers_configured"], "configured peers for %s", familyID)
		assert.Equalf(t, int64(1), mx["family_"+familyID+"_peers_charted"], "charted peers for %s", familyID)
		assert.Equalf(t, int64(1), mx["family_"+familyID+"_peers_down"], "down peers for %s", familyID)
		assert.Equalf(t, int64(0), mx["family_"+familyID+"_peers_established"], "established peers for %s", familyID)
	}

	vpn4Chart := collr.Charts().Get("family_vpn4tab_ipv4_vpn_peer_states")
	require.NotNil(t, vpn4Chart)
	assert.Equal(t, "vpn4tab", chartLabelValue(vpn4Chart, "vrf"))
	assert.Equal(t, "ipv4", chartLabelValue(vpn4Chart, "afi"))
	assert.Equal(t, "vpn", chartLabelValue(vpn4Chart, "safi"))

	label4PeerID := makePeerIDWithScope("label4_ipv4_label", "198.51.100.2", "bgp_mpls")
	peerChart := collr.Charts().Get("peer_" + label4PeerID + "_messages")
	require.NotNil(t, peerChart)
	assert.Equal(t, "label4", chartLabelValue(peerChart, "table"))
	assert.Equal(t, "bgp_mpls", chartLabelValue(peerChart, "protocol"))
}

type birdMPLSFamiliesHarness struct {
	image         string
	containerName string
	workDir       string
}

func newBIRDMPLSFamiliesHarness(t *testing.T, image string) *birdMPLSFamiliesHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bird.conf"), []byte(birdMPLSFamiliesConfig()), 0o644))

	h := &birdMPLSFamiliesHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-bird-mpls-it-%d", time.Now().UnixNano()),
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
			t.Logf("bird3 mpls container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("bird3 mpls show protocols all:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "birdc -s /work/netdata.ctl show protocols all || true"))
			t.Logf("bird3 mpls config:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/bird.conf || true"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *birdMPLSFamiliesHarness) waitReady(t *testing.T) {
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
	}, 2*time.Minute, 500*time.Millisecond, "bird3 mpls container did not become ready")
}

func (h *birdMPLSFamiliesHarness) collectorSocketPath() string {
	return filepath.Join(h.workDir, "netdata.ctl")
}

func (h *birdMPLSFamiliesHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func birdMPLSFamiliesConfig() string {
	return `router id 192.0.2.1;

mpls domain mdom;
mpls table mtab;

ipv4 table label4;
ipv6 table label6;
vpn4 table vpn4tab;
vpn6 table vpn6tab;

cli "/work/netdata.ctl" { restrict; };

protocol device {}

protocol bgp bgp_mpls {
  description "MPLS VPN probe";
  local 192.0.2.1 as 64512;
  neighbor 198.51.100.2 as 65001;
  multihop;
  ipv4 mpls {
    table label4;
    import all;
    export all;
  };
  ipv6 mpls {
    table label6;
    import all;
    export all;
  };
  vpn4 mpls {
    table vpn4tab;
    import all;
    export all;
  };
  vpn6 mpls {
    table vpn6tab;
    import all;
    export all;
  };
  mpls {
    table mtab;
    label policy aggregate;
  };
}
`
}
