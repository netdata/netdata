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
	rowIndex := slices.IndexFunc(profile.Definition.BGP, func(row ddprofiledefinition.BGPConfig) bool {
		return row.Table.Name == "cbgpPeer3Table" && row.Table.OID == "1.3.6.1.4.1.9.9.187.1.2.9"
	})
	require.NotEqual(t, -1, rowIndex, "expected merged Cisco profile to include typed cbgpPeer3Table row")

	// Official CISCO-BGP4-MIB defines cbgpPeer3Entry indexed by:
	// cbgpPeer3VrfId, cbgpPeer3Type, cbgpPeer3RemoteAddr.
	row := profile.Definition.BGP[rowIndex]
	assert.Equal(t, "cbgpPeer3VrfName", row.Identity.RoutingInstance.Symbol.Name)
	assert.Equal(t, "cbgpPeer3RemoteAddr", row.Identity.Neighbor.Symbol.Name)
	assert.EqualValues(t, 2, row.Descriptors.PeerType.Index)

	// Checkmk's cisco_bgp_peerv3 section independently uses cbgpPeer3Table for:
	// localAddr, localIdentifier, remoteAs, remoteIdentifier, adminStatus,
	// state, establishedTime, vrfName, and OID-end remoteAddr extraction.
	assert.Equal(t, "cbgpPeer3LocalAddr", row.Descriptors.LocalAddress.Symbol.Name)
	assert.Equal(t, "cbgpPeer3RemoteAs", row.Identity.RemoteAS.Symbol.Name)
	assert.Equal(t, "cbgpPeer3LocalIdentifier", row.Descriptors.LocalIdentifier.Symbol.Name)
	assert.Equal(t, "cbgpPeer3RemoteIdentifier", row.Descriptors.PeerIdentifier.Symbol.Name)
	assert.Equal(t, "cbgpPeer3AdminStatus", row.Admin.Enabled.Symbol.Name)
	assert.Equal(t, "cbgpPeer3State", row.State.Symbol.Name)
	assert.Equal(t, "cbgpPeer3FsmEstablishedTime", row.Connection.EstablishedUptime.Symbol.Name)

	// Netdata intentionally prefers numeric last-error code/subcode over the
	// vendor-specific text field, so operators get stable alertable metrics.
	assert.Equal(t, "cbgpPeer3LastErrorCode", row.LastError.Code.Symbol.Name)
	assert.Equal(t, "cbgpPeer3LastErrorSubcode", row.LastError.Subcode.Symbol.Name)
	assert.Equal(t, "cbgpPeer3PrevState", row.Previous.Symbol.Name)
}
