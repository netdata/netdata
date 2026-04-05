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
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	bgpOpenBGPDPortableIntegrationEnableEnv = "BGP_OPENBGPD_PORTABLE_ENABLE_INTEGRATION"
	bgpOpenBGPDPortableBuildImageEnv        = "BGP_OPENBGPD_PORTABLE_BUILD_IMAGE"
	bgpOpenBGPDPortableTarballURLEnv        = "BGP_OPENBGPD_PORTABLE_TARBALL_URL"

	defaultOpenBGPDPortableBuildImage = "debian:bookworm-slim"
	defaultOpenBGPDPortableTarballURL = "https://cdn.openbsd.org/pub/OpenBSD/OpenBGPD/openbgpd-9.0.tar.gz"
)

type openbgpdPortableIntegrationConfig struct {
	buildImage string
	tarballURL string
}

func TestIntegration_OpenBGPDPortableLiveConfiguredExtendedFamilies(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)
	harness := newOpenBGPDPortableIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfigPortableExtendedFamilies)
	waitOpenBGPDPortableReady(t, harness)

	neighborsBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/neighbors")
	require.NoError(t, err)

	families, neighbors, err := parseOpenBGPDNeighbors(neighborsBody)
	require.NoError(t, err)
	require.Len(t, families, 7)
	require.Len(t, neighbors, 1)

	ids := make([]string, 0, len(families))
	for _, family := range families {
		ids = append(ids, family.ID)
		assert.Equal(t, int64(0), family.PeersEstablished)
		assert.Equal(t, int64(0), family.PeersAdminDown)
		assert.Equal(t, int64(1), family.PeersDown)
		assert.Equal(t, int64(1), family.ConfiguredPeers)
		assert.Equal(t, int64(0), family.MessagesReceived)
		assert.Equal(t, int64(0), family.MessagesSent)
		assert.Equal(t, int64(0), family.PrefixesReceived)
		assert.Len(t, family.Peers, 1)
		assert.Equal(t, int64(peerStateDown), family.Peers[0].State)
	}

	assert.Equal(t, []string{
		"default_ipv4_flowspec",
		"default_ipv4_unicast",
		"default_ipv4_vpn",
		"default_ipv6_flowspec",
		"default_ipv6_unicast",
		"default_ipv6_vpn",
		"default_l2vpn_evpn",
	}, ids)

	collr := New()
	collr.Backend = backendOpenBGPD
	collr.APIURL = harness.apiURL()
	collr.CollectRIBSummaries = true
	collr.RIBSummaryEvery = confopt.Duration(time.Minute)
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
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_established"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_peers_admin_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_down"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_configured"])
		assert.Equal(t, int64(1), mx["family_"+familyID+"_peers_charted"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_messages_received"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_messages_sent"])
		assert.Equal(t, int64(0), mx["family_"+familyID+"_prefixes_received"])
		require.NotNil(t, collr.Charts().Get("family_"+familyID+"_peer_states"))
	}

	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_correctness_valid"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_unicast_correctness_invalid"])
	assert.Equal(t, int64(1), mx["family_default_ipv4_unicast_correctness_not_found"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_unicast_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_vpn_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_vpn_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv4_flowspec_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_ipv6_flowspec_rib_routes"])
	assert.Equal(t, int64(0), mx["family_default_l2vpn_evpn_rib_routes"])

	collectorChart := collr.Charts().Get("collector_status")
	require.NotNil(t, collectorChart)
	assert.Equal(t, harness.apiURL(), chartLabelValue(collectorChart, "target"))
}

func getOpenBGPDPortableIntegrationConfig(t *testing.T) openbgpdPortableIntegrationConfig {
	t.Helper()

	switch strings.ToLower(strings.TrimSpace(os.Getenv(bgpOpenBGPDPortableIntegrationEnableEnv))) {
	case "1", "true", "yes", "on":
	default:
		t.Skipf("%s is not enabled", bgpOpenBGPDPortableIntegrationEnableEnv)
	}

	buildImage := strings.TrimSpace(os.Getenv(bgpOpenBGPDPortableBuildImageEnv))
	if buildImage == "" {
		buildImage = defaultOpenBGPDPortableBuildImage
	}

	tarballURL := strings.TrimSpace(os.Getenv(bgpOpenBGPDPortableTarballURLEnv))
	if tarballURL == "" {
		tarballURL = defaultOpenBGPDPortableTarballURL
	}

	return openbgpdPortableIntegrationConfig{
		buildImage: buildImage,
		tarballURL: tarballURL,
	}
}

func buildOpenBGPDPortableIntegrationImage(t *testing.T, cfg openbgpdPortableIntegrationConfig) string {
	t.Helper()

	buildDir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(buildDir, "Dockerfile"), []byte(openbgpdPortableIntegrationDockerfile), 0o644))

	image, err := runDockerCommand(15*time.Minute,
		"build",
		"--build-arg", "BASE_IMAGE="+cfg.buildImage,
		"--build-arg", "OPENBGPD_TARBALL_URL="+cfg.tarballURL,
		"--quiet",
		buildDir,
	)
	require.NoError(t, err)

	image = strings.TrimSpace(image)
	if idx := strings.LastIndex(image, "\n"); idx >= 0 {
		image = strings.TrimSpace(image[idx+1:])
	}
	require.NotEmpty(t, image, "portable OpenBGPD image id must not be empty")

	t.Cleanup(func() {
		_, _ = runDockerCommand(30*time.Second, "image", "rm", "--force", image)
	})

	return image
}

func newOpenBGPDPortableIntegrationHarnessWithConfig(t *testing.T, image, config string) *openbgpdIntegrationHarness {
	t.Helper()

	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bgpd.conf"), []byte(config), 0o644))

	h := &openbgpdIntegrationHarness{
		image:         image,
		containerName: fmt.Sprintf("netdata-bgp-openbgpd-portable-it-%d", time.Now().UnixNano()),
		workDir:       workDir,
		fcgiSocket:    filepath.Join(workDir, "bgplgd.sock"),
	}

	_, err := runDockerCommand(5*time.Minute,
		"run", "--rm", "--detach",
		"--name", h.containerName,
		"--volume", fmt.Sprintf("%s:/work", h.workDir),
		h.image,
		"sh", "-lc", openbgpdPortableIntegrationRunCommand,
	)
	require.NoError(t, err)

	h.server = httptest.NewServer(newOpenBGPDFastCGIProxy(h.fcgiSocket, 15*time.Second))

	t.Cleanup(func() {
		if h.server != nil {
			h.server.Close()
		}

		if t.Failed() {
			t.Logf("OpenBGPD portable container logs:\n%s", h.dockerOutput("logs", h.containerName))
			t.Logf("OpenBGPD portable show neighbor:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show neighbor || true"))
			t.Logf("OpenBGPD portable show rib detail:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "bgpctl -j -s /run/bgpd.rsock show rib detail || true"))
			t.Logf("OpenBGPD portable config:\n%s", h.dockerOutput("exec", h.containerName, "sh", "-lc", "cat /work/bgpd.conf || true"))
			t.Logf("OpenBGPD portable bgpd log:\n%s", h.hostFile("bgpd.log"))
			t.Logf("OpenBGPD portable bgplgd log:\n%s", h.hostFile("bgplgd.log"))
		}

		_, _ = runDockerCommand(15*time.Second, "rm", "--force", h.containerName)
	})

	return h
}

func waitOpenBGPDPortableReady(t *testing.T, h *openbgpdIntegrationHarness) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"test -S /run/bgpd.rsock && test -S /work/bgplgd.sock && /build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock show neighbor >/dev/null",
		); err != nil {
			return false
		}
		if _, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			"chmod 666 /work/bgplgd.sock >/dev/null 2>&1 || true",
		); err != nil {
			return false
		}
		if _, err := readOpenBGPDFastCGIBody(h.fcgiSocket, 5*time.Second, "/neighbors"); err != nil {
			return false
		}
		return true
	}, 5*time.Minute, 500*time.Millisecond, "portable OpenBGPD container did not become ready")
}

const openbgpdPortableIntegrationDockerfile = `
ARG BASE_IMAGE=debian:bookworm-slim
FROM ${BASE_IMAGE}

ARG OPENBGPD_TARBALL_URL

RUN apt-get update >/dev/null && \
    DEBIAN_FRONTEND=noninteractive apt-get install -y \
        bison \
        build-essential \
        ca-certificates \
        curl \
        file \
        libevent-dev \
        libmnl-dev \
        pkg-config \
        >/dev/null && \
    rm -rf /var/lib/apt/lists/*

WORKDIR /build
RUN curl -fsSL "${OPENBGPD_TARBALL_URL}" -o openbgpd.tar.gz && \
    tar -xzf openbgpd.tar.gz --strip-components=1 && \
    ./configure >/dev/null && \
    make -j"$(nproc)" >/dev/null

RUN groupadd -r _bgpd && \
    useradd -r -g _bgpd -s /usr/sbin/nologin -d /var/empty _bgpd && \
    groupadd -r _bgplgd && \
    useradd -r -g _bgplgd -s /usr/sbin/nologin -d /var/empty _bgplgd && \
    mkdir -p /run/openbgpd /usr/local/var/run /var/empty && \
    chown root:root /var/empty && \
    chmod 0755 /var/empty
`

const openbgpdPortableIntegrationRunCommand = `
set -e
mkdir -p /run/openbgpd /usr/local/var/run
chown _bgpd:_bgpd /run/openbgpd
chown _bgpd:_bgpd /usr/local/var/run
/build/src/bgpd/bgpd -dvf /work/bgpd.conf >/work/bgpd.log 2>&1 &
bgpd_pid=$!
/build/src/bgplgd/bgplgd -d -U _bgplgd -p /build/src/bgpctl/bgpctl -s /work/bgplgd.sock -S /run/bgpd.rsock >/work/bgplgd.log 2>&1 &
bgplgd_pid=$!
for _ in $(seq 1 100); do
    if [ -S /work/bgplgd.sock ]; then
        chmod 666 /work/bgplgd.sock || true
        break
    fi
    sleep 0.1
done
trap 'kill "$bgpd_pid" "$bgplgd_pid" 2>/dev/null || true' EXIT INT TERM
wait "$bgpd_pid" "$bgplgd_pid"
`

const openbgpdIntegrationConfigPortableExtendedFamilies = `AS 65010
router-id 192.0.2.254
fib-update no
socket "/run/bgpd.rsock" restricted
listen on 127.0.0.1 port 1179
network 203.0.113.0/24
neighbor 192.0.2.2 {
    remote-as 65020
    passive
    descr "test-peer"
    announce IPv4 unicast
    announce IPv6 unicast
    announce IPv4 vpn
    announce IPv6 vpn
    announce IPv4 flowspec
    announce IPv6 flowspec
    announce EVPN
}
`
