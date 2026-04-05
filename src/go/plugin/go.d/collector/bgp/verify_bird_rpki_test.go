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

func TestIntegration_BIRDRPKILiveCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("BIRD live RPKI integration test requires Linux")
	}

	requireDocker(t)
	stayrtrSourcePath := getStayRTRIntegrationSourcePath(t)

	for _, tc := range getBIRDIntegrationCases(t) {
		t.Run(tc.name, func(t *testing.T) {
			harness := newBIRDRPKIIntegrationHarness(t, tc, stayrtrSourcePath)
			harness.waitReady(t)
			harness.waitRPKIReady(t)

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

			assert.Equal(t, int64(1), mx["rpki_rpki4_up"])
			assert.Equal(t, int64(0), mx["rpki_rpki4_down"])
			assert.Greater(t, mx["rpki_rpki4_uptime_seconds"], int64(0))

			cacheChart := findChartByLabels(collr.Charts(), "bgp.rpki_cache_state", map[string]string{
				"backend": backendBIRD,
				"cache":   "rpki4",
			})
			require.NotNil(t, cacheChart)
			assert.Equal(t, "Local validator", chartLabelValue(cacheChart, "cache_desc"))
		})
	}
}

type birdRPKIIntegrationHarness struct {
	tc            birdIntegrationCase
	containerName string
	workDir       string
}

func newBIRDRPKIIntegrationHarness(t *testing.T, tc birdIntegrationCase, stayrtrSourcePath string) *birdRPKIIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	buildGoIntegrationBinary(t, stayrtrSourcePath, "stayrtr", filepath.Join(workDir, "stayrtr"))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bird.conf"), []byte(birdRPKIIntegrationConfig(tc)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "run.sh"), []byte(birdRPKIIntegrationRunCommand(tc.packageName)), 0o755))
	require.NoError(t, writeBIRDRPKIFixture(workDir))

	h := &birdRPKIIntegrationHarness{
		tc:            tc,
		containerName: fmt.Sprintf("netdata-bgp-%s-rpki-it-%d", tc.name, time.Now().UnixNano()),
		workDir:       workDir,
	}

	_, err := runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.tc.image,
		"sh", "-lc", "/work/run.sh",
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("%s container logs:\n%s", h.tc.name, h.dockerOutput("logs", h.containerName))
			t.Logf("%s show protocols all:\n%s", h.tc.name, h.dockerOutput("exec", h.containerName, "sh", "-lc", fmt.Sprintf("birdc -s /work/%s show protocols all || true", h.tc.collectorSocket)))
			t.Logf("%s config:\n%s", h.tc.name, h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/bird.conf || true"))
			t.Logf("%s stayrtr log:\n%s", h.tc.name, h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/stayrtr.log || true"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *birdRPKIIntegrationHarness) waitReady(t *testing.T) {
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
	}, 2*time.Minute, 500*time.Millisecond, "BIRD RPKI container %s did not become ready", h.tc.name)
}

func (h *birdRPKIIntegrationHarness) waitRPKIReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			fmt.Sprintf("birdc -s /work/%s show protocols all", h.tc.collectorSocket),
		)
		return err == nil && strings.Contains(out, "rpki4") && strings.Contains(out, "Established")
	}, 90*time.Second, 500*time.Millisecond, "BIRD RPKI protocol did not reach Established state")
}

func (h *birdRPKIIntegrationHarness) collectorSocketPath() string {
	return filepath.Join(h.workDir, h.tc.collectorSocket)
}

func (h *birdRPKIIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func birdRPKIIntegrationRunCommand(pkg string) string {
	return fmt.Sprintf(
		"#!/bin/sh\n"+
			"set -eu\n"+
			"DEBIAN_FRONTEND=noninteractive apt-get update >/dev/null\n"+
			"DEBIAN_FRONTEND=noninteractive apt-get install -y --no-install-recommends %s >/dev/null\n"+
			"/work/stayrtr -bind 127.0.0.1:3323 -cache /work/stayrtr_rpki.json -checktime=false -refresh 3600 -metrics.addr=\"\" >/work/stayrtr.log 2>&1 &\n"+
			"exec bird -f -c /work/bird.conf -s /work/main.ctl\n",
		pkg,
	)
}

func birdRPKIIntegrationConfig(tc birdIntegrationCase) string {
	extraSocket := ""
	if tc.extraSocketConfig != "" {
		extraSocket = tc.extraSocketConfig + "\n\n"
	}

	return fmt.Sprintf(`router id 192.0.2.1;

ipv4 table master4;

roa4 table r4;

%sprotocol rpki rpki4 {
  description "Local validator";
  roa4 { table r4; };
  remote 127.0.0.1 port 3323;
  retry keep 5;
  refresh keep 30;
  expire 600;
}

protocol device {}

protocol bgp bgp4 {
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
`, extraSocket)
}

func writeBIRDRPKIFixture(workDir string) error {
	_, fileName, _, ok := runtime.Caller(0)
	if !ok {
		return fmt.Errorf("locate BIRD RPKI integration test file")
	}

	data, err := os.ReadFile(filepath.Join(filepath.Dir(fileName), "testdata", "gobgp", "stayrtr_rpki.json"))
	if err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(workDir, "stayrtr_rpki.json"), data, 0o644)
}
