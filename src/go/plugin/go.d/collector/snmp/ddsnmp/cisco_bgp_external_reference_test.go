// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/multipath"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func Test_CiscoBgpPeer3Profile_AlignedWithOfficialMIBAndCheckmk(t *testing.T) {
	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	matched := FindProfiles("1.3.6.1.4.1.9.1.923", "", nil) // ciscoASR1002
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "cisco-asr.yaml")
	})
	require.NotEqual(t, -1, index, "expected cisco-asr profile to match")

	profile := matched[index]
	metricIndex := slices.IndexFunc(profile.Definition.Metrics, func(m ddprofiledefinition.MetricsConfig) bool {
		return m.Table.Name == "cbgpPeer3Table" && m.Table.OID == "1.3.6.1.4.1.9.9.187.1.2.9"
	})
	require.NotEqual(t, -1, metricIndex, "expected merged Cisco profile to include cbgpPeer3Table")

	metric := profile.Definition.Metrics[metricIndex]

	var symbolNames []string
	for _, sym := range metric.Symbols {
		symbolNames = append(symbolNames, sym.Name)
	}

	var tagNames []string
	for _, tag := range metric.MetricTags {
		tagNames = append(tagNames, tag.Tag)
	}

	// Official CISCO-BGP4-MIB defines cbgpPeer3Entry indexed by:
	// cbgpPeer3VrfId, cbgpPeer3Type, cbgpPeer3RemoteAddr.
	assert.Contains(t, tagNames, "routing_instance_id")
	assert.Contains(t, tagNames, "neighbor_address_type")
	assert.Contains(t, tagNames, "neighbor")

	// Checkmk's cisco_bgp_peerv3 section independently uses cbgpPeer3Table for:
	// localAddr, localIdentifier, remoteAs, remoteIdentifier, adminStatus,
	// state, establishedTime, vrfName, and OID-end remoteAddr extraction.
	assert.Contains(t, tagNames, "routing_instance")
	assert.Contains(t, tagNames, "local_address")
	assert.Contains(t, tagNames, "remote_as")
	assert.Contains(t, tagNames, "local_identifier")
	assert.Contains(t, tagNames, "peer_identifier")
	assert.Contains(t, symbolNames, "bgpPeerAdminStatus")
	assert.Contains(t, symbolNames, "bgpPeerState")
	assert.Contains(t, symbolNames, "bgpPeerFsmEstablishedTime")

	// Netdata intentionally prefers numeric last-error code/subcode over the
	// vendor-specific text field, so operators get stable alertable metrics.
	assert.Contains(t, symbolNames, "bgpPeerLastErrorCode")
	assert.Contains(t, symbolNames, "bgpPeerLastErrorSubcode")
	assert.Contains(t, symbolNames, "bgpPeerPreviousState")
}
