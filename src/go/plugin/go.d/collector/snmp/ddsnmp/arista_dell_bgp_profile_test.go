// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

func Test_AristaAndDellBGPProfilesUseTypedRows(t *testing.T) {
	tests := map[string]struct {
		sysObjectID  string
		profileFile  string
		legacyTables []string
		bgpTables    map[string]string
	}{
		"Arista switch": {
			sysObjectID:  "1.3.6.1.4.1.30065.1.3011.7010.427.48",
			profileFile:  "arista-switch.yaml",
			legacyTables: []string{"bgpPeerTable", "aristaBgp4V2PeerTable", "aristaBgp4V2PeerCountersTable", "aristaBgp4V2PrefixGaugesTable"},
			bgpTables: map[string]string{
				"arista-bgp-peer":        "aristaBgp4V2PeerTable",
				"arista-bgp-peer-family": "aristaBgp4V2PrefixGaugesTable",
			},
		},
		"Dell OS10": {
			sysObjectID:  "1.3.6.1.4.1.674.11000.5000.100.2.1.1",
			profileFile:  "dell-os10.yaml",
			legacyTables: []string{"dell.os10bgp4V2PeerTable", "dell.os10bgp4V2PeerCountersTable", "dell.os10bgp4V2PrefixGaugesTable"},
			bgpTables: map[string]string{
				"dell-os10-bgp-peer":        "dell.os10bgp4V2PeerTable",
				"dell-os10-bgp-peer-family": "dell.os10bgp4V2PrefixGaugesTable",
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			matched := FindProfiles(tc.sysObjectID, "", nil)
			index := slices.IndexFunc(matched, func(p *Profile) bool {
				return strings.HasSuffix(p.SourceFile, tc.profileFile)
			})
			require.NotEqual(t, -1, index, "expected %s profile to match", tc.profileFile)

			profile := matched[index]
			for _, table := range tc.legacyTables {
				assertNoMetricTable(t, profile, table)
			}
			assertNoVirtualMetric(t, profile, "bgpPeerAvailability")
			assertNoVirtualMetric(t, profile, "bgpPeerUpdates")
			for rowID, table := range tc.bgpTables {
				assertBGPTableForRowID(t, profile, rowID, table)
			}

			for rowID := range tc.bgpTables {
				if strings.Contains(rowID, "peer-family") {
					continue
				}
				peer := requireBGPRowByID(t, profile, rowID)
				assertBGPSixStateMapping(t, peer.State)
				if peer.Previous.IsSet() {
					assertBGPSixStateMapping(t, peer.Previous)
				}
			}
		})
	}
}
