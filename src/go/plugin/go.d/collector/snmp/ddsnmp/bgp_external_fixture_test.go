// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

type externalDeviceFixture struct {
	OS struct {
		Discovery struct {
			Devices []struct {
				SysObjectID string `json:"sysObjectID"`
				SysDescr    string `json:"sysDescr"`
			} `json:"devices"`
		} `json:"discovery"`
	} `json:"os"`
}

func Test_LibreNMSBGPIdentityFixtures_MatchTypedProfiles(t *testing.T) {
	tests := map[string]struct {
		fixtureFile string
		profileFile string
		validate    func(t *testing.T, profile *Profile)
	}{
		"Cumulus identity matches standard BGP projection": {
			fixtureFile: "cumulus_bgp_identity.json",
			profileFile: "nvidia-cumulus-linux-switch.yaml",
			validate: func(t *testing.T, profile *Profile) {
				require.NotEmpty(t, profile.Definition.BGP)
				assert.True(t, profile.HasExtension("_std-bgp4-mib.yaml"))

				row := profile.Definition.BGP[0]
				assert.Equal(t, "_std-bgp4-mib.yaml", row.OriginProfileID)
				assert.Equal(t, "bgp4-peer", row.ID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
				assert.Equal(t, "bgpPeerTable", row.Table.Name)
				assert.Equal(t, "1.3.6.1.2.1.15.3", row.Table.OID)
			},
		},
		"Juniper vMX identity matches Juniper MX BGP profile": {
			fixtureFile: "junos_vmx_identity.json",
			profileFile: "juniper-mx.yaml",
			validate: func(t *testing.T, profile *Profile) {
				require.Len(t, profile.Definition.BGP, 2)

				peer := requireBGPRowByID(t, profile, "juniper-bgp-peer")
				assert.Equal(t, "_juniper-bgp4-v2.yaml", peer.OriginProfileID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, peer.Kind)
				assert.Equal(t, "jnxBgpM2PeerTable", peer.Table.Name)
				assert.Equal(t, "jnxBgpM2PeerCountersTable", peer.Traffic.Updates.Received.Table)

				family := requireBGPRowByID(t, profile, "juniper-bgp-peer-family")
				assert.Equal(t, "_juniper-bgp4-v2.yaml", family.OriginProfileID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, family.Kind)
				assert.Equal(t, "jnxBgpM2PrefixCountersTable", family.Table.Name)
				assert.Equal(t, "jnxBgpM2PeerTable", family.Identity.RemoteAS.Table)
				assert.Equal(t, "jnxBgpM2PeerIndex", family.Identity.RemoteAS.LookupSymbol.Name)
			},
		},
		"TiMOS IXR-S identity matches Nokia SR OS BGP profile": {
			fixtureFile: "timos_ixr_s_identity.json",
			profileFile: "nokia-service-router-os.yaml",
			validate: func(t *testing.T, profile *Profile) {
				require.Len(t, profile.Definition.BGP, 7)
				assert.True(t, profile.HasExtension("_nokia-timetra-bgp.yaml"))
				assert.False(t, profile.HasExtension("_std-bgp4-mib.yaml"))

				peer := requireBGPRowByID(t, profile, "nokia-timos-bgp-peer")
				assert.Equal(t, "_nokia-timetra-bgp.yaml", peer.OriginProfileID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, peer.Kind)
				assert.Equal(t, "tBgpPeerNgTable", peer.Table.Name)
				assert.Equal(t, "tBgpPeerNgOperTable", peer.Connection.EstablishedUptime.Table)

				family := requireBGPRowByID(t, profile, "nokia-timos-bgp-peer-family-ipv4-vpn")
				assert.Equal(t, "_nokia-timetra-bgp.yaml", family.OriginProfileID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, family.Kind)
				assert.Equal(t, "tBgpPeerNgOperTable", family.Table.Name)
				assert.Equal(t, "tBgpPeerNgTable", family.Identity.RemoteAS.Table)
			},
		},
		"Cisco IOS XR NCS 540 identity matches Cisco NCS BGP profile": {
			fixtureFile: "iosxr_ncs540_identity.json",
			profileFile: "cisco-ncs.yaml",
			validate: func(t *testing.T, profile *Profile) {
				require.Len(t, profile.Definition.BGP, 2)
				assert.True(t, profile.HasExtension("_cisco-bgp4-mib.yaml"))

				row := requireBGPRowByID(t, profile, "cisco-bgp-peer-family")
				assert.Equal(t, "_cisco-bgp4-mib.yaml", row.OriginProfileID)
				assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, row.Kind)
				assert.Equal(t, "cbgpPeer2AddrFamilyPrefixTable", row.Table.Name)
				assert.Equal(t, "cbgpPeer2Table", row.Identity.RemoteAS.Table)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profile := matchedProfileFromIdentityFixture(t, tc.fixtureFile, tc.profileFile)
			tc.validate(t, profile)
		})
	}
}

func matchedProfileFromIdentityFixture(t *testing.T, fixtureFile, profileFile string) *Profile {
	t.Helper()

	path := filepath.Join("testdata", "librenms", fixtureFile)
	data, err := os.ReadFile(path)
	require.NoError(t, err)

	var fixture externalDeviceFixture
	require.NoError(t, json.Unmarshal(data, &fixture))
	require.Len(t, fixture.OS.Discovery.Devices, 1)

	device := fixture.OS.Discovery.Devices[0]
	sysObjectID := strings.TrimPrefix(device.SysObjectID, ".")
	matched := FindProfiles(sysObjectID, device.SysDescr, nil)

	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, profileFile)
	})
	require.NotEqual(t, -1, index, "expected %s profile to match", profileFile)

	return matched[index]
}

func requireBGPRowByID(t *testing.T, profile *Profile, id string) ddprofiledefinition.BGPConfig {
	t.Helper()

	rowIndex := slices.IndexFunc(profile.Definition.BGP, func(row ddprofiledefinition.BGPConfig) bool {
		return row.ID == id
	})
	require.NotEqual(t, -1, rowIndex, "expected typed BGP row %s", id)

	return profile.Definition.BGP[rowIndex]
}

func assertBGPSixStateMapping(t *testing.T, state ddprofiledefinition.BGPStateConfig) {
	t.Helper()
	assert.Equal(t, map[string]string{
		"1": "idle",
		"2": "connect",
		"3": "active",
		"4": "opensent",
		"5": "openconfirm",
		"6": "established",
	}, state.Symbol.Mapping.Items)
}
