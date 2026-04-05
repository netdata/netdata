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
	bgpStayRTRSourcePathEnv       = "BGP_STAYRTR_SOURCE_PATH"
	defaultStayRTRIntegrationPath = "/opt/baddisk/monitoring/bgp/bgp__stayrtr"
)

func TestIntegration_GoBGPLiveCollectionWithRPKI(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("GoBGP live integration test requires Linux")
	}

	cfg := getGoBGPIntegrationConfig(t)
	stayrtrSourcePath := getStayRTRIntegrationSourcePath(t)
	requireDocker(t)

	harness := newGoBGPRPKIIntegrationHarness(t, cfg, stayrtrSourcePath)
	harness.waitReady(t)
	harness.waitRPKIReady(t)
	harness.seedRPKIRuntime(t)
	harness.waitValidationApplied(t)

	collr := New()
	collr.Backend = backendGoBGP
	collr.SocketPath = ""
	collr.Address = harness.grpcAddress()
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

	cacheName := gobgpRPKICacheName("127.0.0.1", 3323)
	cacheID := idPart(cacheName)

	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_established"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_peers_admin_down"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_down"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_configured"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_peers_charted"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_received"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_messages_sent"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_prefixes_received"])
	assert.Equal(t, int64(3), mx["family_default_ipv4_unicast_rib_routes"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_valid"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_invalid"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_not_found"])

	assert.Equal(t, int64(1), mx["family_red_ipv4_unicast_peers_configured"])
	assert.Equal(t, int64(1), mx["family_red_ipv4_unicast_rib_routes"])
	assert.NotContains(t, mx, "family_red_ipv4_unicast_correctness_valid")
	assert.NotContains(t, mx, "family_red_ipv4_unicast_correctness_invalid")
	assert.NotContains(t, mx, "family_red_ipv4_unicast_correctness_not_found")
	assert.Equal(t, int64(1), mx["rpki_"+cacheID+"_up"])
	assert.Equal(t, int64(0), mx["rpki_"+cacheID+"_down"])
	_, ok := mx["rpki_"+cacheID+"_uptime_seconds"]
	assert.True(t, ok)
	assert.Equal(t, int64(3), mx["rpki_"+cacheID+"_record_ipv4"])
	assert.Equal(t, int64(0), mx["rpki_"+cacheID+"_record_ipv6"])
	assert.Equal(t, int64(3), mx["rpki_"+cacheID+"_prefix_ipv4"])
	assert.Equal(t, int64(0), mx["rpki_"+cacheID+"_prefix_ipv6"])

	require.NotNil(t, collr.Charts().Get("family_default_ipv4_unicast_peer_states"))
	require.NotNil(t, collr.Charts().Get(familyCorrectnessChartID("default_ipv4_unicast")))
	require.Nil(t, collr.Charts().Get(familyCorrectnessChartID("red_ipv4_unicast")))
	require.NotNil(t, collr.Charts().Get(rpkiCacheStateChartID(cacheID)))
	require.NotNil(t, collr.Charts().Get(rpkiCacheRecordsChartID(cacheID)))
	require.NotNil(t, collr.Charts().Get(rpkiCachePrefixesChartID(cacheID)))

	cacheChart := findChartByLabels(collr.Charts(), "bgp.rpki_cache_state", map[string]string{
		"backend": backendGoBGP,
		"cache":   cacheName,
	})
	require.NotNil(t, cacheChart)
	assert.Equal(t, "up", chartLabelValue(cacheChart, "state_text"))

	collectorChart := collr.Charts().Get("collector_status")
	require.NotNil(t, collectorChart)
	assert.Equal(t, harness.grpcAddress(), chartLabelValue(collectorChart, "target"))
}

func getStayRTRIntegrationSourcePath(t *testing.T) string {
	t.Helper()

	sourcePath := strings.TrimSpace(os.Getenv(bgpStayRTRSourcePathEnv))
	if sourcePath == "" {
		sourcePath = defaultStayRTRIntegrationPath
	}
	info, err := os.Stat(sourcePath)
	require.NoError(t, err, "StayRTR source path is required")
	require.True(t, info.IsDir(), "StayRTR source path must be a directory")
	return sourcePath
}

func newGoBGPRPKIIntegrationHarness(t *testing.T, cfg gobgpIntegrationConfig, stayrtrSourcePath string) *gobgpIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	buildGoIntegrationBinary(t, cfg.sourcePath, "gobgp", filepath.Join(workDir, "gobgp"))
	buildGoIntegrationBinary(t, cfg.sourcePath, "gobgpd", filepath.Join(workDir, "gobgpd"))
	buildGoIntegrationBinary(t, stayrtrSourcePath, "stayrtr", filepath.Join(workDir, "stayrtr"))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "gobgpd.conf"), []byte(goBGPRPKIIntegrationConfigFile), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "run.sh"), []byte(goBGPRPKIIntegrationRunScript), 0o755))
	require.NoError(t, writeGoBGPRPKIFixture(workDir))

	h := &gobgpIntegrationHarness{
		cfg:           cfg,
		containerName: fmt.Sprintf("netdata-bgp-gobgp-rpki-it-%d", time.Now().UnixNano()),
		workDir:       workDir,
	}

	_, err := runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--publish", "127.0.0.1::50051",
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		cfg.image,
		"/work/run.sh",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("GoBGP container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("GoBGP global rib:\n%s", h.dockerOutput("exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv4"))
			t.Logf("GoBGP neighbors:\n%s", h.dockerOutput("exec", h.containerName, "/work/gobgp", "neighbor"))
			t.Logf("GoBGP config:\n%s", h.dockerOutput("exec", h.containerName, "cat", "/work/gobgpd.conf"))
			t.Logf("StayRTR log:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/stayrtr.log || true"))
			t.Logf("RPKI fixture:\n%s", h.hostFixtureFile("stayrtr_rpki.json"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *gobgpIntegrationHarness) waitRPKIReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		logs := h.dockerOutput("logs", h.containerName)
		return strings.Contains(logs, "ROA server is connected")
	}, 90*time.Second, 500*time.Millisecond, "GoBGP RPKI session did not become ready")
}

func (h *gobgpIntegrationHarness) seedRPKIRuntime(t *testing.T) {
	t.Helper()

	_, err := runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "neighbor", "add", "192.0.2.2", "as", "65002")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "add", "red", "rd", "65012:100", "rt", "both", "65012:100")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "neighbor", "add", "198.51.100.2", "as", "65003", "vrf", "red")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4", "203.0.113.0/24", "aspath", "65010")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4", "203.0.114.0/24", "aspath", "65011")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "add", "-a", "ipv4", "198.18.0.0/25", "aspath", "65030")
	require.NoError(t, err)
	_, err = runDockerCommand(15*time.Second, "exec", h.containerName, "/work/gobgp", "vrf", "red", "rib", "add", "-a", "ipv4", "198.19.0.0/24", "aspath", "65031")
	require.NoError(t, err)
}

func (h *gobgpIntegrationHarness) waitValidationApplied(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "/work/gobgp", "global", "rib", "-a", "ipv4")
		if err != nil {
			return false
		}
		return strings.Contains(out, "V*>203.0.113.0/24") &&
			strings.Contains(out, "I*>198.18.0.0/25") &&
			strings.Contains(out, "N*>203.0.114.0/24")
	}, 60*time.Second, 500*time.Millisecond, "GoBGP did not apply RPKI validation to seeded routes")
}

func (h *gobgpIntegrationHarness) hostFixtureFile(name string) string {
	data, err := os.ReadFile(filepath.Join(h.workDir, name))
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(string(data))
}

func writeGoBGPRPKIFixture(workDir string) error {
	_, fileName, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate GoBGP RPKI integration test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(fileName), "testdata", "gobgp", "stayrtr_rpki.json"))
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(workDir, "stayrtr_rpki.json"), data, 0o644)
}

const goBGPRPKIIntegrationConfigFile = `[global.config]
  as = 64512
  router-id = "192.0.2.254"
  port = -1

[[rpki-servers]]
  [rpki-servers.config]
    address = "127.0.0.1"
    port = 3323
`

const goBGPRPKIIntegrationRunScript = `#!/bin/sh
set -eu
/work/stayrtr -bind 127.0.0.1:3323 -cache /work/stayrtr_rpki.json -checktime=false -refresh 3600 -metrics.addr="" >/work/stayrtr.log 2>&1 &
exec /work/gobgpd -f /work/gobgpd.conf --api-hosts :50051 --pprof-disable
`
