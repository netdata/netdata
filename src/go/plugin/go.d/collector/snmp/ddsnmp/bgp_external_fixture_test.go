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

func Test_LibreNMSCumulusIdentity_MatchesStandardBGPProjection(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "cumulus_bgp_identity.json", "nvidia-cumulus-linux-switch.yaml")

	require.NotEmpty(t, profile.Definition.BGP)
	assert.True(t, profile.HasExtension("_std-bgp4-mib.yaml"))

	row := profile.Definition.BGP[0]
	assert.Equal(t, "_std-bgp4-mib.yaml", row.OriginProfileID)
	assert.Equal(t, "bgp4-peer", row.ID)
	assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
	assert.Equal(t, "bgpPeerTable", row.Table.Name)
	assert.Equal(t, "1.3.6.1.2.1.15.3", row.Table.OID)
}

func Test_LibreNMSJuniperVMXFixture_MatchesJuniperMXBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "junos_vmx_identity.json", "juniper-mx.yaml")

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
}

func Test_LibreNMSTiMOSIXRSFixture_MatchesNokiaSROSBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "timos_ixr_s_identity.json", "nokia-service-router-os.yaml")

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
}

func Test_LibreNMSCiscoIOSXRNCS540Fixture_MatchesCiscoNCSBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "iosxr_ncs540_identity.json", "cisco-ncs.yaml")

	require.Len(t, profile.Definition.BGP, 2)
	assert.True(t, profile.HasExtension("_cisco-bgp4-mib.yaml"))

	rowIndex := slices.IndexFunc(profile.Definition.BGP, func(row ddprofiledefinition.BGPConfig) bool {
		return row.ID == "cisco-bgp-peer-family"
	})
	require.NotEqual(t, -1, rowIndex, "expected Cisco NCS profile to include typed Cisco peer-family row")

	row := profile.Definition.BGP[rowIndex]
	assert.Equal(t, "_cisco-bgp4-mib.yaml", row.OriginProfileID)
	assert.Equal(t, ddprofiledefinition.BGPRowKindPeerFamily, row.Kind)
	assert.Equal(t, "cbgpPeer2AddrFamilyPrefixTable", row.Table.Name)
	assert.Equal(t, "cbgpPeer2Table", row.Identity.RemoteAS.Table)
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
