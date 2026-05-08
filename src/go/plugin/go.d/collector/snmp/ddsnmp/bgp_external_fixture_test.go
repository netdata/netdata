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

func Test_LibreNMSCumulusFixture_MatchesCumulusBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "cumulus_bgp_identity.json", "nvidia-cumulus-linux-switch.yaml")

	assertVirtualSources(t, profile, "bgpPeerAvailability", []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerAdminStatus", Table: "bgpPeerTable", As: "admin_enabled", Dim: "start"},
		{Metric: "bgpPeerState", Table: "bgpPeerTable", As: "established", Dim: "established"},
	})
	assertVirtualSources(t, profile, "bgpPeerUpdates", []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerInUpdates", Table: "bgpPeerTable", As: "received"},
		{Metric: "bgpPeerOutUpdates", Table: "bgpPeerTable", As: "sent"},
	})
}

func Test_LibreNMSJuniperVMXFixture_MatchesJuniperMXBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "junos_vmx_identity.json", "juniper-mx.yaml")

	assertVirtualSources(t, profile, "bgpPeerAvailability", []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerAdminStatus", Table: "jnxBgpM2PeerTable", As: "admin_enabled", Dim: "running"},
		{Metric: "bgpPeerState", Table: "jnxBgpM2PeerTable", As: "established", Dim: "established"},
	})
	assertVirtualSources(t, profile, "bgpPeerUpdates", []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerInUpdates", Table: "jnxBgpM2PeerCountersTable", As: "received"},
		{Metric: "bgpPeerOutUpdates", Table: "jnxBgpM2PeerCountersTable", As: "sent"},
	})
}

func Test_LibreNMSTiMOSIXRSFixture_MatchesNokiaSROSBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "timos_ixr_s_identity.json", "nokia-service-router-os.yaml")

	assertVirtualAlternativeSources(t, profile, "bgpPeerAvailability", 0, []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerAdminStatus", Table: "tBgpPeerNgTable", As: "admin_enabled", Dim: "start"},
		{Metric: "bgpPeerState", Table: "tBgpPeerNgTable", As: "established", Dim: "established"},
	})
	assertVirtualAlternativeSources(t, profile, "bgpPeerAvailability", 1, []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerAdminStatus", Table: "bgpPeerTable", As: "admin_enabled", Dim: "start"},
		{Metric: "bgpPeerState", Table: "bgpPeerTable", As: "established", Dim: "established"},
	})
	assertVirtualAlternativeSources(t, profile, "bgpPeerUpdates", 0, []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerInUpdates", Table: "tBgpPeerNgOperTable", As: "received"},
		{Metric: "bgpPeerOutUpdates", Table: "tBgpPeerNgOperTable", As: "sent"},
	})
	assertVirtualAlternativeSources(t, profile, "bgpPeerUpdates", 1, []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerInUpdates", Table: "bgpPeerTable", As: "received"},
		{Metric: "bgpPeerOutUpdates", Table: "bgpPeerTable", As: "sent"},
	})
}

func Test_LibreNMSCiscoIOSXRNCS540Fixture_MatchesGenericCiscoBGPProfile(t *testing.T) {
	profile := matchedProfileFromIdentityFixture(t, "iosxr_ncs540_identity.json", "cisco.yaml")

	assertVirtualSources(t, profile, "bgpPeerAvailability", []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerAdminStatus", Table: "cbgpPeer3Table", As: "admin_enabled", Dim: "start"},
		{Metric: "bgpPeerState", Table: "cbgpPeer3Table", As: "established", Dim: "established"},
	})
	assertVirtualSources(t, profile, "bgpPeerUpdates", []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "bgpPeerInUpdates", Table: "cbgpPeer3Table", As: "received"},
		{Metric: "bgpPeerOutUpdates", Table: "cbgpPeer3Table", As: "sent"},
	})
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

func assertVirtualSources(t *testing.T, profile *Profile, name string, want []ddprofiledefinition.VirtualMetricSourceConfig) {
	t.Helper()

	vm := requireVirtualMetric(t, profile, name)

	assert.Equal(t, want, vm.Sources)
}

func assertVirtualAlternativeSources(t *testing.T, profile *Profile, name string, alternative int, want []ddprofiledefinition.VirtualMetricSourceConfig) {
	t.Helper()

	vm := requireVirtualMetric(t, profile, name)
	require.Less(t, alternative, len(vm.Alternatives), "expected virtual metric %s alternative %d", name, alternative)

	assert.Equal(t, want, vm.Alternatives[alternative].Sources)
}

func requireVirtualMetric(t *testing.T, profile *Profile, name string) ddprofiledefinition.VirtualMetricConfig {
	t.Helper()

	vmIndex := slices.IndexFunc(profile.Definition.VirtualMetrics, func(vm ddprofiledefinition.VirtualMetricConfig) bool {
		return vm.Name == name
	})
	require.NotEqual(t, -1, vmIndex, "expected virtual metric %s", name)

	return profile.Definition.VirtualMetrics[vmIndex]
}
