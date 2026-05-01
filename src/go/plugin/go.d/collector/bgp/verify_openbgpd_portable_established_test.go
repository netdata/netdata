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
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenBGPDPortableLiveNegotiatedExtendedFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)
	harness := newOpenBGPDPortableEstablishedIntegrationHarness(t, image,
		[]string{"IPv4 unicast", "IPv4 flowspec", "IPv6 flowspec", "EVPN"},
		[]string{"IPv4 unicast", "IPv4 flowspec", "IPv6 flowspec", "EVPN"},
	)
	waitOpenBGPDPortableEstablishedReady(t, harness)

	neighborsBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/neighbors")
	require.NoError(t, err)

	families, neighbors, err := parseOpenBGPDNeighbors(neighborsBody)
	require.NoError(t, err)
	require.Len(t, families, 4)
	require.Len(t, neighbors, 1)

	ids := make([]string, 0, len(families))
	for _, family := range families {
		ids = append(ids, family.ID)
		assert.Equal(t, int64(1), family.PeersEstablished)
		assert.Equal(t, int64(0), family.PeersAdminDown)
		assert.Equal(t, int64(0), family.PeersDown)
		assert.Equal(t, int64(1), family.ConfiguredPeers)
		assert.Len(t, family.Peers, 1)
		assert.Equal(t, int64(peerStateUp), family.Peers[0].State)
	}

	assert.Equal(t, []string{
		"default_ipv4_flowspec",
		"default_ipv4_unicast",
		"default_ipv6_flowspec",
		"default_l2vpn_evpn",
	}, ids)

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.APIURL = harness.apiURL()
	collr.CollectRIBSummaries = false
	collr.Timeout = confopt.Duration(5 * time.Second)
	collr.MaxFamilies = 20
	collr.MaxPeers = 10

	require.NoError(t, collr.Init(context.Background()))
	require.NoError(t, collr.Check(context.Background()))
	t.Cleanup(func() { collr.Cleanup(context.Background()) })

	mx := collr.Collect(context.Background())
	require.NotNil(t, mx)
	mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

	for _, familyID := range ids {
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_established"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
		require.NotNil(t, collr.Charts().Get("family_"+familyID+"_peer_states"))
	}
}

func newOpenBGPDPortableEstablishedIntegrationHarness(t *testing.T, image string, localFamilies, remoteFamilies []string) *openbgpdEstablishedIntegrationHarness {
	return newOpenBGPDPortableEstablishedIntegrationHarnessWithConfig(t, image, localFamilies, remoteFamilies, "", "")
}

func newOpenBGPDPortableEstablishedIntegrationHarnessWithConfig(t *testing.T, image string, localFamilies, remoteFamilies []string, localExtra, remoteExtra string) *openbgpdEstablishedIntegrationHarness {
	t.Helper()

	baseDir := t.TempDir()
	workDir := filepath.Join(baseDir, "openbgpd-a")
	peerWorkDir := filepath.Join(baseDir, "openbgpd-b")
	for _, dir := range []string{workDir, peerWorkDir} {
		require.NoError(t, os.MkdirAll(dir, 0o777))
		require.NoError(t, os.Chmod(dir, 0o777))
	}

	subnet, localIP, remoteIP := createOpenBGPDEstablishedSubnet()
	localAS := 65010
	peerAS := 65020

	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bgpd.conf"), []byte(openbgpdEstablishedIntegrationConfigWithExtra(
		"192.0.2.254", localIP, localAS, remoteIP, peerAS, "198.51.100.0/24", localFamilies, localExtra,
	)), 0o644))
	require.NoError(t, os.WriteFile(filepath.Join(peerWorkDir, "bgpd.conf"), []byte(openbgpdEstablishedIntegrationConfigWithExtra(
		"192.0.2.253", remoteIP, peerAS, localIP, localAS, "203.0.113.0/24", remoteFamilies, remoteExtra,
	)), 0o644))

	h := &openbgpdEstablishedIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-openbgpd-portable-established-a-%d", time.Now().UnixNano()),
		peerName:      fmt.Sprintf("netdata-bgp-openbgpd-portable-established-b-%d", time.Now().UnixNano()),
		workDir:       workDir,
		peerWorkDir:   peerWorkDir,
		fcgiSocket:    filepath.Join(workDir, "bgplgd.sock"),
		networkName:   fmt.Sprintf("netdata-bgp-openbgpd-portable-established-%d", time.Now().UnixNano()),
		localIP:       localIP,
		remoteIP:      remoteIP,
		peerAS:        peerAS,
	}

	_, err := runDockerCommand(30*time.Second, "network", "create", "--subnet", subnet, h.networkName)
	require.NoError(t, err)

	_, err = runDockerCommand(5*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--network", h.networkName,
		"--ip", h.localIP,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.image,
		"sh", "-lc", openbgpdPortableIntegrationRunCommand,
	)
	require.NoError(t, err)

	_, err = runDockerCommand(5*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.peerName,
		"--network", h.networkName,
		"--ip", h.remoteIP,
		"--volume", fmt.Sprintf("%s:/work", h.peerWorkDir),
		h.image,
		"sh", "-lc", openbgpdPortablePeerRunCommand,
	)
	require.NoError(t, err)

	h.server = httptest.NewServer(newOpenBGPDFastCGIProxy(h.fcgiSocket, 15*time.Second))

	t.Cleanup(func() {
		if h.server != nil {
			h.server.Close()
		}

		if t.Failed() {
			t.Logf("OpenBGPD portable established logs (%s):\n%s", h.containerName, h.dockerOutput("logs", h.containerName))
			t.Logf("OpenBGPD portable established logs (%s):\n%s", h.peerName, h.dockerOutput("logs", h.peerName))
			t.Logf("OpenBGPD portable established show neighbor (%s):\n%s", h.containerName, h.dockerOutput("exec", h.containerName, "sh", "-lc", "/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock show neighbor || true"))
			t.Logf("OpenBGPD portable established show neighbor (%s):\n%s", h.peerName, h.dockerOutput("exec", h.peerName, "sh", "-lc", "/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock show neighbor || true"))
			t.Logf("OpenBGPD portable established config (%s):\n%s", h.containerName, h.hostFile("bgpd.conf"))
			t.Logf("OpenBGPD portable established config (%s):\n%s", h.peerName, h.peerHostFile("bgpd.conf"))
			t.Logf("OpenBGPD portable established bgpd log (%s):\n%s", h.containerName, h.hostFile("bgpd.log"))
			t.Logf("OpenBGPD portable established bgpd log (%s):\n%s", h.peerName, h.peerHostFile("bgpd.log"))
			t.Logf("OpenBGPD portable established bgplgd log (%s):\n%s", h.containerName, h.hostFile("bgplgd.log"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName, h.peerName)
		_, _ = runDockerCommand(15*time.Second, "network", "rm", h.networkName)
	})

	return h
}

func waitOpenBGPDPortableEstablishedReady(t *testing.T, h *openbgpdEstablishedIntegrationHarness) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := os.Stat(h.fcgiSocket); err != nil {
			return false
		}
		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"chmod 666 /work/bgplgd.sock >/dev/null 2>&1 || true",
		); err != nil {
			return false
		}

		neighborsJSON, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock show neighbor",
		)
		if err != nil {
			return false
		}

		families, _, err := parseOpenBGPDNeighbors([]byte(neighborsJSON))
		if err != nil || len(families) != 4 {
			return false
		}

		expected := map[string]bool{
			"default_ipv4_flowspec": true,
			"default_ipv4_unicast":  true,
			"default_ipv6_flowspec": true,
			"default_l2vpn_evpn":    true,
		}

		for _, family := range families {
			if !expected[family.ID] {
				return false
			}
			if family.PeersEstablished != 1 {
				return false
			}
		}

		if _, err := readOpenBGPDFastCGIBody(h.fcgiSocket, 5*time.Second, "/neighbors"); err != nil {
			return false
		}
		return true
	}, 5*time.Minute, 500*time.Millisecond, "portable OpenBGPD peers did not negotiate the expected extended families")
}

const openbgpdPortablePeerRunCommand = `
set -e
mkdir -p /run/openbgpd /usr/local/var/run
chown _bgpd:_bgpd /run/openbgpd
chown _bgpd:_bgpd /usr/local/var/run
/build/src/bgpd/bgpd -dvf /work/bgpd.conf >/work/bgpd.log 2>&1 &
bgpd_pid=$!
trap 'kill "$bgpd_pid" 2>/dev/null || true' EXIT INT TERM
wait "$bgpd_pid"
`
