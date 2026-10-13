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

func Test_CiscoBGPProfilesIncludeTypedRows(t *testing.T) {
	tests := map[string]struct {
		sysObjectID string
		profileFile string
		tableName   string
		tableOID    string
		validate    func(t *testing.T, profile *Profile, row ddprofiledefinition.BGPConfig)
	}{
		"ASR has Cisco peer row": {
			sysObjectID: "1.3.6.1.4.1.9.1.923", // ciscoASR1002
			profileFile: "cisco-asr.yaml",
			tableName:   "cbgpPeer3Table",
			tableOID:    "1.3.6.1.4.1.9.9.187.1.2.9",
			validate: func(t *testing.T, _ *Profile, row ddprofiledefinition.BGPConfig) {
				assert.Equal(t, "_cisco-bgp4-mib.yaml", row.OriginProfileID)
				assert.Equal(t, "cisco-bgp-peer", row.ID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
				assert.Equal(t, "cbgpPeer3RemoteAddr", row.Identity.Neighbor.Symbol.Name)
				assert.Equal(t, "cbgpPeer3RemoteAs", row.Identity.RemoteAS.Symbol.Name)
				assert.Equal(t, "cbgpPeer3AdminStatus", row.Admin.Enabled.Symbol.Name)
				assert.Equal(t, "cbgpPeer3State", row.State.Symbol.Name)
				assert.Equal(t, "cbgpPeer3InUpdates", row.Traffic.Updates.Received.Symbol.Name)
				assert.Equal(t, "cbgpPeer3OutTotalMessages", row.Traffic.Messages.Sent.Symbol.Name)
				assert.Equal(t, "cbgpPeer3LastErrorCode", row.LastError.Code.Symbol.Name)
				assert.Equal(t, "cbgpPeer3LastErrorSubcode", row.LastError.Subcode.Symbol.Name)
				assert.Equal(t, "cbgpPeer3FsmEstablishedTransitions", row.Transitions.Established.Symbol.Name)
				assert.Equal(t, "cbgpPeer3FsmEstablishedTime", row.Connection.EstablishedUptime.Symbol.Name)
				assert.Equal(t, "cbgpPeer3InUpdateElapsedTime", row.Connection.LastReceivedUpdateAge.Symbol.Name)
				assert.Equal(t, "cbgpPeer3PrevState", row.Previous.Symbol.Name)
			},
		},
		"ASR has Cisco peer-family row": {
			sysObjectID: "1.3.6.1.4.1.9.1.923", // ciscoASR1002
			profileFile: "cisco-asr.yaml",
			tableName:   "cbgpPeer2AddrFamilyPrefixTable",
			tableOID:    "1.3.6.1.4.1.9.9.187.1.2.8",
			validate: func(t *testing.T, _ *Profile, row ddprofiledefinition.BGPConfig) {
				assert.Equal(t, "_cisco-bgp4-mib.yaml", row.OriginProfileID)
				assert.Equal(t, "cisco-bgp-peer-family", row.ID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, row.Kind)
				assert.Equal(t, "cbgpPeer2RemoteAs", row.Identity.RemoteAS.Symbol.Name)
				assert.Equal(t, "cbgpPeer2Table", row.Identity.RemoteAS.Table)
				assert.Equal(t, "cbgpPeer2AcceptedPrefixes", row.Routes.Current.Accepted.Symbol.Name)
				assert.Equal(t, "cbgpPeer2DeniedPrefixes", row.Routes.Current.Rejected.Symbol.Name)
				assert.Equal(t, "cbgpPeer2PrefixAdminLimit", row.RouteLimits.Limit.Symbol.Name)
				assert.Equal(t, "cbgpPeer2PrefixThreshold", row.RouteLimits.Threshold.Symbol.Name)
				assert.Equal(t, "cbgpPeer2PrefixClearThreshold", row.RouteLimits.ClearThreshold.Symbol.Name)
				assert.Equal(t, "cbgpPeer2AdvertisedPrefixes", row.Routes.Current.Advertised.Symbol.Name)
				assert.Equal(t, "cbgpPeer2SuppressedPrefixes", row.Routes.Current.Suppressed.Symbol.Name)
				assert.Equal(t, "cbgpPeer2WithdrawnPrefixes", row.Routes.Current.Withdrawn.Symbol.Name)
				assert.Equal(t, uint(2), row.Identity.AddressFamily.IndexFromEnd)
				assert.Equal(t, uint(1), row.Identity.SubsequentAddressFamily.IndexFromEnd)
			},
		},
		"Catalyst has Cisco peer row": {
			sysObjectID: "1.3.6.1.4.1.9.1.2435", // catC930024T
			profileFile: "cisco-catalyst.yaml",
			tableName:   "cbgpPeer3Table",
			tableOID:    "1.3.6.1.4.1.9.9.187.1.2.9",
			validate: func(t *testing.T, profile *Profile, row ddprofiledefinition.BGPConfig) {
				assert.True(t, profile.HasExtension("_cisco-bgp4-mib.yaml"))
				assert.Equal(t, "_cisco-bgp4-mib.yaml", row.OriginProfileID)
			},
		},
	}

	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedProfileByFileForTest(t, tc.sysObjectID, tc.profileFile)
			row := requireBGPRowByTable(t, profile, tc.tableName, tc.tableOID)
			tc.validate(t, profile, row)
		})
	}
}

func Test_CiscoGenericProfilesDoNotInheritBGP(t *testing.T) {
	tests := map[string]struct {
		sysObjectID string
		profileFile string
	}{
		"generic Cisco profile has no BGP rows": {
			sysObjectID: "1.3.6.1.4.1.9.1",
			profileFile: "cisco.yaml",
		},
		"Cisco base mixin has no BGP rows": {
			sysObjectID: "1.3.6.1.4.1.9.1",
			profileFile: "_cisco-base.yaml",
		},
	}

	dir, err := filepath.Abs("../../../config/go.d/snmp.profiles/default")
	require.NoError(t, err)

	_, err = loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile, err := loadProfile(filepath.Join(dir, tc.profileFile), multipath.New(dir))
			require.NoError(t, err)
			require.True(t, strings.HasSuffix(profile.SourceFile, tc.profileFile))
			assert.Empty(t, profile.Definition.BGP)
		})
	}
}

func matchedProfileByFileForTest(t *testing.T, sysObjectID, profileFile string) *Profile {
	t.Helper()

	matched := FindProfiles(sysObjectID, "", nil)
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, profileFile)
	})
	require.NotEqual(t, -1, index, "expected %s profile to match", profileFile)

	return matched[index]
}

func requireBGPRowByTable(t *testing.T, profile *Profile, tableName, tableOID string) ddprofiledefinition.BGPConfig {
	t.Helper()

	rowIndex := slices.IndexFunc(profile.Definition.BGP, func(row ddprofiledefinition.BGPConfig) bool {
		return row.Table.Name == tableName && row.Table.OID == tableOID
	})
	require.NotEqual(t, -1, rowIndex, "expected merged profile to include typed %s row", tableName)

	return profile.Definition.BGP[rowIndex]
}
