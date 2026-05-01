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

func TestIntegration_FRRRPKILiveCollection(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("FRR live RPKI integration test requires Linux")
	}

	image := getFRRIntegrationImage(t)
	stayrtrSourcePath := getStayRTRIntegrationSourcePath(t)
	requireDocker(t)

	harness := newFRRRPKIIntegrationHarness(t, image, stayrtrSourcePath)
	harness.waitReady(t)
	harness.waitRPKIReady(t)

	collr := New()
	collr.SocketPath = harness.socketPath()
	collr.Timeout = confopt.Duration(5 * time.Second)

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	cacheName := "tcp 127.0.0.1:3323 pref 1"
	cacheID := idPart(cacheName)

	assert.Equal(t, int64(1), mx["rpki_"+cacheID+"_up"])
	assert.Equal(t, int64(0), mx["rpki_"+cacheID+"_down"])
	assert.NotContains(t, mx, "rpki_"+cacheID+"_uptime_seconds")
	assert.Equal(t, int64(3), mx["rpki_inventory_daemon_prefix_ipv4"])
	assert.Equal(t, int64(0), mx["rpki_inventory_daemon_prefix_ipv6"])

	require.NotNil(t, collr.Charts().Get(rpkiCacheStateChartID(cacheID)))
	require.Nil(t, collr.Charts().Get(rpkiCacheUptimeChartID(cacheID)))
	require.NotNil(t, collr.Charts().Get(rpkiInventoryPrefixesChartID("daemon")))

	cacheChart := findChartByLabels(collr.Charts(), "bgp.rpki_cache_state", map[string]string{
		"backend": backendFRR,
		"cache":   cacheName,
	})
	require.NotNil(t, cacheChart)
	assert.Equal(t, "connected", chartLabelValue(cacheChart, "state_text"))

	inventoryChart := findChartByLabels(collr.Charts(), "bgp.rpki_inventory_prefixes", map[string]string{
		"backend":         backendFRR,
		"inventory_scope": "daemon",
	})
	require.NotNil(t, inventoryChart)
}

type frrRPKIIntegrationHarness struct {
	image         string
	containerName string
	etcDir        string
	runDir        string
	workDir       string
}

func newFRRRPKIIntegrationHarness(t *testing.T, image, stayrtrSourcePath string) *frrRPKIIntegrationHarness {
	t.Helper()

	baseDir := t.TempDir()
	etcDir := filepath.Join(baseDir, "etc-frr")
	runDir := filepath.Join(baseDir, "run-frr")
	workDir := filepath.Join(baseDir, "work")

	require.NoError(t, os.MkdirAll(etcDir, 0o755))
	require.NoError(t, os.MkdirAll(runDir, 0o777))
	require.NoError(t, os.MkdirAll(workDir, 0o755))
	require.NoError(t, os.Chmod(runDir, 0o777))
	require.NoError(t, os.Chmod(workDir, 0o777))

	buildGoIntegrationBinary(t, stayrtrSourcePath, "stayrtr", filepath.Join(workDir, "stayrtr"))
	require.NoError(t, writeGoBGPRPKIFixture(workDir))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "daemons"), []byte(frrRPKIIntegrationDaemons), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "frr.conf"), []byte(frrRPKIIntegrationConfig(image)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(etcDir, "vtysh.conf"), nil, 0o644))

	h := &frrRPKIIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-frr-rpki-it-%d", time.Now().UnixNano()),
		etcDir:        etcDir,
		runDir:        runDir,
		workDir:       workDir,
	}

	_, err := runDockerCommand(2*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--privileged",
		"--volume", fmt.Sprintf("%s:/etc/frr", h.etcDir),
		"--volume", fmt.Sprintf("%s:/var/run/frr", h.runDir),
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.image,
	)
	require.NoError(t, err)

	t.Cleanup(func() {
		if t.Failed() {
			t.Logf("FRR RPKI container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("FRR RPKI config:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /etc/frr/daemons; printf '\\n'; cat /etc/frr/frr.conf"))
			t.Logf("FRR RPKI configuration JSON:\n%s", h.dockerOutput("exec", h.containerName, "vtysh", "-c", "show rpki configuration json"))
			t.Logf("FRR RPKI cache-server JSON:\n%s", h.dockerOutput("exec", h.containerName, "vtysh", "-c", "show rpki cache-server json"))
			t.Logf("FRR RPKI cache-connection JSON:\n%s", h.dockerOutput("exec", h.containerName, "vtysh", "-c", "show rpki cache-connection json"))
			t.Logf("FRR RPKI prefix-count JSON:\n%s", h.dockerOutput("exec", h.containerName, "vtysh", "-c", "show rpki prefix-count json"))
			t.Logf("StayRTR log:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/stayrtr.log || true"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func (h *frrRPKIIntegrationHarness) waitReady(t *testing.T) {
	t.Helper()

	socketPath := h.socketPath()
	require.Eventually(t, func() bool {
		if _, err := os.Stat(socketPath); err != nil {
			return false
		}

		if _, err := runDockerCommand(10*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"test -S /var/run/frr/bgpd.vty && vtysh -c 'show bgp vrf all ipv4 summary json' >/dev/null && vtysh -c 'show rpki configuration json' >/dev/null",
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
	}, 45*time.Second, 500*time.Millisecond, "FRR RPKI container did not become ready")

	_, err := runDockerCommand(15*time.Second,
		"exec", h.containerName, "sh", "-lc",
		"/work/stayrtr -bind 127.0.0.1:3323 -cache /work/stayrtr_rpki.json -checktime=false -refresh 3600 -metrics.addr=\"\" >/work/stayrtr.log 2>&1 &",
	)
	require.NoError(t, err)
}

func (h *frrRPKIIntegrationHarness) waitRPKIReady(t *testing.T) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := runDockerCommand(10*time.Second, "exec", h.containerName, "vtysh", "-c", "show rpki cache-connection json")
		return err == nil && strings.Contains(out, "\"state\":\"connected\"")
	}, 90*time.Second, 500*time.Millisecond, "FRR RPKI cache did not reach connected state")
}

func (h *frrRPKIIntegrationHarness) socketPath() string {
	return filepath.Join(h.runDir, "bgpd.vty")
}

func (h *frrRPKIIntegrationHarness) dockerOutput(args ...string) string {
	out, err := runDockerCommand(15*time.Second, args...)
	if err != nil {
		return err.Error()
	}
	return strings.TrimSpace(out)
}

func frrRPKIIntegrationConfig(image string) string {
	version := strings.TrimSpace(strings.TrimPrefix(image, "quay.io/frrouting/frr:"))
	if version == "" || version == image {
		version = "10.6.0"
	}

	return fmt.Sprintf(`frr version %s
frr defaults traditional
hostname frr-rpki-test
service integrated-vtysh-config
!
rpki
 rpki polling_period 60
 rpki retry_interval 1
 rpki cache tcp 127.0.0.1 3323 preference 1
 exit
!
router bgp 65001
 bgp router-id 10.0.0.1
!
line vty
`, version)
}

const frrRPKIIntegrationDaemons = `zebra=yes
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
bgpd_options="   -A 127.0.0.1 -M rpki"
staticd_options="-A 127.0.0.1"
`
