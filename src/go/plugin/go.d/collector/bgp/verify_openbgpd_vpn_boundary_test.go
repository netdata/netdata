// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenBGPDPortableLinuxRejectsL3VPNConfig(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)
	workDir := t.TempDir()
	require.NoError(t, os.Chmod(workDir, 0o777))
	require.NoError(t, os.WriteFile(filepath.Join(workDir, "bgpd.conf"), []byte(openbgpdIntegrationConfigPortableL3VPN), 0o644))

	output, err := runDockerCommand(2*time.Minute,
		"run", "--rm",
		"--volume", fmt.Sprintf("%s:/work", workDir),
		image,
		"sh", "-lc", openbgpdPortableConfigCheckRunCommand,
	)
	require.Error(t, err)
	assert.Contains(t, output, "troubles getting config of mpe1")
}

func TestIntegration_OpenBGPDPortableLiveDynamicVPNRouteRequiresL3VPN(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)
	harness := newOpenBGPDPortableIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfigPortableVPNFamilies)
	waitOpenBGPDPortableReady(t, harness)

	_, err := runDockerCommand(15*time.Second,
		"exec", harness.containerName,
		"sh", "-lc",
		"/build/src/bgpctl/bgpctl -s /run/bgpd.rsock network add 198.51.100.0/24 rd 65010:1",
	)
	require.NoError(t, err)

	body, err := readOpenBGPDFastCGIBodyWithQuery(harness.fcgiSocket, 15*time.Second, "/rib", "af=vpnv4")
	require.NoError(t, err)

	summary, err := parseOpenBGPDRIBSummary(body)
	require.NoError(t, err)
	assert.Equal(t, openbgpdRIBSummary{}, summary)
}

const openbgpdPortableConfigCheckRunCommand = `
set -e
mkdir -p /run/openbgpd /usr/local/var/run
chown _bgpd:_bgpd /run/openbgpd
chown _bgpd:_bgpd /usr/local/var/run
if /build/src/bgpd/bgpd -dvf /work/bgpd.conf >/work/bgpd.log 2>&1; then
    cat /work/bgpd.log
    exit 0
fi
cat /work/bgpd.log
exit 1
`

const openbgpdIntegrationConfigPortableL3VPN = `AS 65010
router-id 192.0.2.254
fib-update no
socket "/run/bgpd.rsock" restricted
vpn "test" on mpe1 {
    rd 65010:1
    import-target rt 65010:1
    export-target rt 65010:1
    fib-update no
    network 198.51.100.0/24
}
`

const openbgpdIntegrationConfigPortableVPNFamilies = `AS 65010
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
    announce IPv4 vpn
    announce IPv6 vpn
}
`
