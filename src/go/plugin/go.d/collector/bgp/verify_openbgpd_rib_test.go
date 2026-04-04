// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"runtime"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenBGPDLiveFilteredRIBEndpoints(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD live integration test requires Linux")
	}

	image := getOpenBGPDIntegrationImage(t)
	requireDocker(t)

	harness := newOpenBGPDIntegrationHarness(t, image)
	harness.waitReady(t)

	ipv4Body, err := readOpenBGPDFastCGIBodyWithQuery(harness.fcgiSocket, 15*time.Second, "/rib", "af=ipv4")
	require.NoError(t, err)
	ipv4Summary, err := parseOpenBGPDRIBSummary(ipv4Body)
	require.NoError(t, err)
	assert.Equal(t, openbgpdRIBSummary{
		RIBRoutes: 1,
		NotFound:  1,
	}, ipv4Summary)

	ipv6Body, err := readOpenBGPDFastCGIBodyWithQuery(harness.fcgiSocket, 15*time.Second, "/rib", "af=ipv6")
	require.NoError(t, err)
	ipv6Summary, err := parseOpenBGPDRIBSummary(ipv6Body)
	require.NoError(t, err)
	assert.Equal(t, openbgpdRIBSummary{}, ipv6Summary)

	for _, query := range []string{"af=vpnv4", "af=vpnv6"} {
		body, err := readOpenBGPDFastCGIBodyWithQuery(harness.fcgiSocket, 15*time.Second, "/rib", query)
		require.NoError(t, err)
		summary, err := parseOpenBGPDRIBSummary(body)
		require.NoError(t, err)
		assert.Equal(t, openbgpdRIBSummary{}, summary, query)
	}
}

func TestIntegration_OpenBGPDPortableLiveRIBExcludesLocalFlowSpec(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)
	harness := newOpenBGPDPortableIntegrationHarnessWithConfig(t, image, openbgpdIntegrationConfigPortableFlowSpecLocal)
	waitOpenBGPDPortableReady(t, harness)

	ribBody, err := readOpenBGPDFastCGIBody(harness.fcgiSocket, 15*time.Second, "/rib")
	require.NoError(t, err)
	summaries, err := parseOpenBGPDRIBSummaries(ribBody)
	require.NoError(t, err)
	assert.Equal(t, map[string]openbgpdRIBSummary{
		"default_ipv4_unicast": {
			RIBRoutes: 1,
			NotFound:  1,
		},
	}, summaries)

	flowspecJSON := harness.dockerOutput("exec", harness.containerName, "sh", "-lc", "/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock flowspec show inet || true")
	assert.Contains(t, flowspecJSON, "flow-rate 65010:0")
}

const openbgpdIntegrationConfigPortableFlowSpecLocal = `AS 65010
router-id 192.0.2.254
fib-update no
socket "/run/bgpd.rsock" restricted
network 203.0.113.0/24
flowspec inet from any to 203.0.113.0/24 port 53 set ext-community flow-rate 65010:0
`
