// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package bgp

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIntegration_OpenBGPDPortableLiveSingleFamilyFlowSpecStaysLocal(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("OpenBGPD portable live integration test requires Linux")
	}

	cfg := getOpenBGPDPortableIntegrationConfig(t)
	requireDocker(t)

	image := buildOpenBGPDPortableIntegrationImage(t, cfg)

	tests := []struct {
		name       string
		families   []string
		flowFamily string
		familyID   string
		marker     string
		peerExtra  string
	}{
		{
			name:       "ipv4",
			families:   []string{"IPv4 flowspec"},
			flowFamily: "inet",
			familyID:   "default_ipv4_flowspec",
			marker:     "flow-rate 65020:0",
			peerExtra:  "flowspec inet from any to 203.0.113.0/24 port 53 set ext-community flow-rate 65020:0",
		},
		{
			name:       "ipv6",
			families:   []string{"IPv6 flowspec"},
			flowFamily: "inet6",
			familyID:   "default_ipv6_flowspec",
			marker:     "flow-rate 65020:0",
			peerExtra: strings.Join([]string{
				"network 2001:db8:30::/64",
				"flowspec inet6 from any to 2001:db8:30::/64 port 53 set ext-community flow-rate 65020:0",
			}, "\n"),
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			harness := newOpenBGPDPortableEstablishedIntegrationHarnessWithConfig(t, image, tc.families, tc.families, "", tc.peerExtra)
			t.Cleanup(func() {
				if !t.Failed() {
					return
				}
				t.Logf("OpenBGPD portable flowspec show (%s, sender):\n%s", tc.name, harness.dockerOutput("exec", harness.peerName, "sh", "-lc", fmt.Sprintf("/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock flowspec show %s || true", tc.flowFamily)))
				t.Logf("OpenBGPD portable flowspec show (%s, receiver):\n%s", tc.name, harness.dockerOutput("exec", harness.containerName, "sh", "-lc", fmt.Sprintf("/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock flowspec show %s || true", tc.flowFamily)))
			})

			waitOpenBGPDPortableEstablishedSingleFamilyReady(t, harness, tc.familyID)
			waitOpenBGPDPortableLocalFlowspecInstalled(t, harness, tc.flowFamily, tc.marker)
			assertOpenBGPDPortableDoesNotLearnFlowspec(t, harness, tc.flowFamily, tc.marker, tc.familyID)

			collr := New()
			collr.Backend = backendOpenBGPD
			collr.APIURL = harness.apiURL()
			collr.CollectRIBSummaries = false
			collr.Timeout = confopt.Duration(5 * time.Second)
			collr.MaxFamilies = 10
			collr.MaxPeers = 10

			require.NoError(t, collr.Init(context.Background()))
			require.NoError(t, collr.Check(context.Background()))
			t.Cleanup(func() { collr.Cleanup(context.Background()) })

			mx := collr.Collect(context.Background())
			require.NotNil(t, mx)
			mx = assertAndStripCollectorMetrics(t, mx, collectorStatusOK, 0, 0, 0, 0, 0, 0)

			assert.Equal(t, int64(1), mx["family_"+tc.familyID+"_peers_established"])
			assert.Equal(t, int64(0), mx["family_"+tc.familyID+"_peers_admin_down"])
			assert.Equal(t, int64(0), mx["family_"+tc.familyID+"_peers_down"])
			assert.Equal(t, int64(1), mx["family_"+tc.familyID+"_peers_configured"])
			assert.Equal(t, int64(1), mx["family_"+tc.familyID+"_peers_charted"])
			assert.Equal(t, int64(0), mx["family_"+tc.familyID+"_prefixes_received"])
			require.NotNil(t, collr.Charts().Get("family_"+tc.familyID+"_peer_states"))
		})
	}
}

func waitOpenBGPDPortableEstablishedSingleFamilyReady(t *testing.T, h *openbgpdEstablishedIntegrationHarness, familyID string) {
	t.Helper()

	require.Eventually(t, func() bool {
		if _, err := readOpenBGPDFastCGIBody(h.fcgiSocket, 5*time.Second, "/neighbors"); err != nil {
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
		if err != nil || len(families) != 1 {
			return false
		}

		return families[0].ID == familyID && families[0].PeersEstablished == 1
	}, 5*time.Minute, 500*time.Millisecond, "portable OpenBGPD peers did not negotiate the expected single flowspec family")
}

func waitOpenBGPDPortableLocalFlowspecInstalled(t *testing.T, h *openbgpdEstablishedIntegrationHarness, family, match string) {
	t.Helper()

	require.Eventually(t, func() bool {
		out, err := runDockerCommand(15*time.Second,
			"exec", h.peerName, "sh", "-lc",
			fmt.Sprintf("/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock flowspec show %s", family),
		)
		return err == nil && strings.Contains(out, match)
	}, 30*time.Second, 500*time.Millisecond, "portable OpenBGPD sender side did not install the expected flowspec rule")
}

func assertOpenBGPDPortableDoesNotLearnFlowspec(t *testing.T, h *openbgpdEstablishedIntegrationHarness, family, match, familyID string) {
	t.Helper()

	require.Never(t, func() bool {
		out, err := runDockerCommand(15*time.Second,
			"exec", h.containerName, "sh", "-lc",
			fmt.Sprintf("/build/src/bgpctl/bgpctl -j -s /run/bgpd.rsock flowspec show %s", family),
		)
		if err == nil && strings.Contains(out, match) {
			return true
		}

		neighborsBody, err := readOpenBGPDFastCGIBody(h.fcgiSocket, 15*time.Second, "/neighbors")
		if err != nil {
			return false
		}
		families, _, err := parseOpenBGPDNeighbors(neighborsBody)
		if err != nil || len(families) != 1 {
			return false
		}

		return families[0].ID == familyID && families[0].PrefixesReceived > 0
	}, 20*time.Second, 500*time.Millisecond, "portable OpenBGPD unexpectedly propagated a single-family flowspec rule to the collector side")
}
