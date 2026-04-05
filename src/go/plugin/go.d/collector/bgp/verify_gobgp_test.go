// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"net"
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
	bgpGoBGPIntegrationEnableEnv = "BGP_GOBGP_ENABLE_INTEGRATION"
	bgpGoBGPIntegrationImageEnv  = "BGP_GOBGP_DOCKER_IMAGE"
	bgpGoBGPSourcePathEnv        = "BGP_GOBGP_SOURCE_PATH"

	defaultGoBGPIntegrationImage      = "debian:bookworm-slim"
	defaultGoBGPIntegrationSourcePath = "/opt/baddisk/monitoring/bgp/osrg__gobgp"
)

func TestIntegration_GoBGPLiveCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GoBGP live integration test requires Linux")
	}

	cfg := getGoBGPIntegrationConfig(t)
	requireDocker(t)

	harness := newGoBGPIntegrationHarness(t, cfg)
	harness.waitReady(t)
	harness.seedRuntime(t)

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

	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_established"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_admin_down"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_down"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_configured"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_charted"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_received"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_sent"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_prefixes_received"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_rib_routes"])

	assert.Equal(t, int64(0), mx["family_red_ipv4_unicast_peers_established"])
	assert.Equal(t, int64(0), mx["family_red_ipv4_unicast_peers_admin_down"])
	assert.Equal(t, int64(1), mx["family_red_ipv4_unicast_peers_down"])
	assert.Equal(t, int64(1), mx["family_red_ipv4_unicast_peers_configured"])
	assert.Equal(t, int64(1), mx["family_red_ipv4_unicast_peers_charted"])
	assert.Equal(t, int64(0), mx["family_red_ipv4_unicast_messages_received"])
	assert.Equal(t, int64(0), mx["family_red_ipv4_unicast_messages_sent"])
	assert.Equal(t, int64(0), mx["family_red_ipv4_unicast_prefixes_received"])
	assert.Equal(t, int64(1), mx["family_red_ipv4_unicast_rib_routes"])

	require.NotNil(t, collr.Charts().Get("family_default_ipv4_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get("family_red_ipv4_unicast_peer_states"))

	defaultPeerChart := findChartByLabels(collr.Charts(), "bgp.peer_messages", map[string]string{
		"backend": backendGoBGP,
		"vrf":     "default",
		"peer":    "192.0.2.2",
		"peer_as": "65002",
	})
	require.NotNil(t, defaultPeerChart)

	redNeighborChart := findChartByLabels(collr.Charts(), "bgp.neighbor_transitions", map[string]string{
		"backend": backendGoBGP,
		"vrf":     "red",
		"peer":    "198.51.100.2",
		"peer_as": "65003",
	})
	require.NotNil(t, redNeighborChart)

	collectorChart := collr.Charts().Get("collector_status")
	require.NotNil(t, collectorChart)
	assert.Equal(t, harness.grpcAddress(), chartLabelValue(collectorChart, "target"))
}

type gobgpIntegrationConfig struct {
	image      string
	sourcePath string
}

func getGoBGPIntegrationConfig(t *testing.T) gobgpIntegrationConfig {
	t.Helper()

	switch strings.ToLower(strings.TrimSpace(os.Getenv(bgpGoBGPIntegrationEnableEnv))) {
	case "1", "true", "yes", "on":
	default:
		t.Skipf("%s is not enabled", bgpGoBGPIntegrationEnableEnv)
	}

	image := strings.TrimSpace(os.Getenv(bgpGoBGPIntegrationImageEnv))
	if image == "" {
		image = defaultGoBGPIntegrationImage
	}

	sourcePath := strings.TrimSpace(os.Getenv(bgpGoBGPSourcePathEnv))
	if sourcePath == "" {
		sourcePath = defaultGoBGPIntegrationSourcePath
	}
	info, err := os.Stat(sourcePath)
	require.NoError(t, err, "GoBGP source path is required")
	require.True(t, info.IsDir(), "GoBGP source path must be a directory")

	return gobgpIntegrationConfig{
		image:      image,
		sourcePath: sourcePath,
	}
}

type gobgpIntegrationHarness struct {
	cfg           gobgpIntegrationConfig
	containerName string
	workDir       string
	grpcHostPort  string
}

func newGoBGPIntegrationHarness(t *testing.T, cfg gobgpIntegrationConfig) *gobgpIntegrationHarness {
	return newGoBGPIntegrationHarnessWithConfig(t, cfg, goBGPIntegrationConfigFile)
}

func newGoBGPIntegrationHarnessWithConfig(t *testing.T, cfg gobgpIntegrationConfig, config string) *gobgpIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	buildGoIntegrationBinary(t, cfg.sourcePath, "gobgp", filepath.Join(workDir, "gobgp"))
	buildGoIntegrationBinary(t, cfg.sourcePath, "gobgpd", filepath.Join(workDir, "gobgpd"))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "gobgpd.conf"), []byte(config), 0o644))

	h := &gobgpIntegrationHarness{
		cfg:           cfg,
		containerName: fmt.Sprintf("netdata-bgp-gobgp-it-%d", time.Now().UnixNano()),
		workDir:       workDir,
	}

	_, err := runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--publish", "127.0.0.1::50051",
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.cfg.image,
		"/work/gobgpd",
		"-f", "/work/gobgpd.conf",
		"--api-hosts", ":50051",
		"--pprof-disable",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("GoBGP container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("GoBGP neighbors:\n%s", h.dockerOutput("exec", h.containerName, "/work/gobgp", "neighbor"))
			t.Logf("GoBGP global rib:\n%s", h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib"))
			t.Logf("GoBGP red vrf rib:\n%s", h.dockerOutput("exec", h.containerName, "/work/gobgp", "vrf", "red", "rib"))
			t.Logf("GoBGP config:\n%s", h.dockerOutput("exec", h.containerName, "cat", "/work/gobgpd.conf"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *gobgpIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global"); err != nil {
			return false
		}

		addr, err := h.mappedGRPCAddress()
		if err != nil {
			return false
		}

		conn, err := net.DialTimeout("tcp", addr, time.Second)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 90*time.Second, 500*time.Millisecond, "GoBGP container did not become ready")
}

func (h *gobgpIntegrationHarness) seedRuntime(t *testing.T) {
	t.Helper()

	_, err := runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "neighbor", "add", "192.0.2.2", "as", "65002")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "add", "red", "rd", "65012:100", "rt", "both", "65012:100")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "neighbor", "add", "198.51.100.2", "as", "65003", "vrf", "red")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4", "203.0.113.0/24")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "red", "rib", "add", "-a", "ipv4", "198.18.0.0/24")
	require.NoError(t, err)

	require.Eventually(t, func() bool {
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "neighbor"); err != nil || !strings.Contains(out, "192.0.2.2") || !strings.Contains(out, "198.51.100.2") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib"); err != nil || !strings.Contains(out, "203.0.113.0/24") {
			return false
		}
		if out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "red", "rib"); err != nil || !strings.Contains(out, "198.18.0.0/24") {
			return false
		}
		return true
	}, 45*time.Second, 500*time.Millisecond, "GoBGP runtime state was not seeded")
}

func (h *gobgpIntegrationHarness) grpcAddress() string {
	return h.grpcHostPort
}

func (h *gobgpIntegrationHarness) mappedGRPCAddress() (string, error) {
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

	h.grpcHostPort = addr
	return h.grpcHostPort, nil
}

func (h *gobgpIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func buildGoIntegrationBinary(t *testing.T, sourcePath, name, outPath string) {
	t.Helper()

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	cmd := exec.CommandContext(ctx, "go", "build", "-o", outPath, "./cmd/"+name)
	cmd.Dir = sourcePath
	cmd.Env = append(os.Environ(),
		"CGO_ENABLED=0",
		"GOOS=linux",
		"GOARCH="+runtime.GOARCH,
	)
	output, err := cmd.CombinedOutput()
	if ctx.Err() == context.DeadlineExceeded {
		t.Fatalf("building %s timed out: %s", name, strings.TrimSpace(string(output)))
	}
	require.NoErrorf(t, err, "building %s failed: %s", name, strings.TrimSpace(string(output)))
}

func findChartByLabels(charts *Charts, ctx string, labels map[string]string) *Chart {
	if charts == nil {
		return nil
	}

	for _, chart := range *charts {
		if chart == nil || chart.Ctx != ctx {
			continue
		}

		match := true
		for key, want := range labels {
			if chartLabelValue(chart, key) != want {
				match = false
				break
			}
		}
		if match {
			return chart
		}
	}

	return nil
}

const goBGPIntegrationConfigFile = `[global.config]
  as = 64512
  router-id = "192.0.2.254"
  port = -1
`
