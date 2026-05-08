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

func Test_BGPVirtualMetricContracts_ByProfile(t *testing.T) {
	dir, _ := filepath.Abs("../../../config/go.d/snmp.profiles/default")

	profiles, err := loadProfilesFromDir(dir, multipath.New(dir))
	require.NoError(t, err)
	require.NotEmpty(t, profiles)

	tests := []struct {
		name        string
		sysObjectID string
		profileFile string
		virtuals    map[string][]ddprofiledefinition.VirtualMetricSourceConfig
	}{
		{
			name:        "Cisco ASR uses VRF-aware peer3 tables for common contract",
			sysObjectID: "1.3.6.1.4.1.9.1.923",
			profileFile: "cisco-asr.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "bgpPeerAdminStatus", Table: "cbgpPeer3Table", As: "admin_enabled", Dim: "start"},
					{Metric: "bgpPeerState", Table: "cbgpPeer3Table", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "cbgpPeer3Table", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "cbgpPeer3Table", As: "sent"},
				},
			},
		},
		{
			name:        "Juniper MX overrides generic contract with Juniper peer tables",
			sysObjectID: "1.3.6.1.4.1.2636.1.1.1.2.21",
			profileFile: "juniper-mx.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "bgpPeerAdminStatus", Table: "jnxBgpM2PeerTable", As: "admin_enabled", Dim: "running"},
					{Metric: "bgpPeerState", Table: "jnxBgpM2PeerTable", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "jnxBgpM2PeerCountersTable", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "jnxBgpM2PeerCountersTable", As: "sent"},
				},
			},
		},
		{
			name:        "Nokia SR OS overrides generic contract with TiMOS peer tables",
			sysObjectID: "1.3.6.1.4.1.6527.1.3.17",
			profileFile: "nokia-service-router-os.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "bgpPeerAdminStatus", Table: "tBgpPeerNgTable", As: "admin_enabled", Dim: "start"},
					{Metric: "bgpPeerState", Table: "tBgpPeerNgTable", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "tBgpPeerNgOperTable", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "tBgpPeerNgOperTable", As: "sent"},
				},
			},
		},
		{
			name:        "Arista switch overrides generic contract with Arista peer tables",
			sysObjectID: "1.3.6.1.4.1.30065.1.3011.7010.427.48",
			profileFile: "arista-switch.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "aristaBgp4V2PeerAdminStatus", Table: "aristaBgp4V2PeerTable", As: "admin_enabled", Dim: "running"},
					{Metric: "aristaBgp4V2PeerState", Table: "aristaBgp4V2PeerTable", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "aristaBgp4V2PeerCountersTable", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "aristaBgp4V2PeerCountersTable", As: "sent"},
				},
			},
		},
		{
			name:        "Dell OS10 exposes common contract from Dell peer tables",
			sysObjectID: "1.3.6.1.4.1.674.11000.5000.100.2.1.1",
			profileFile: "dell-os10.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "dell.os10bgp4V2PeerAdminStatus", Table: "dell.os10bgp4V2PeerTable", As: "admin_enabled", Dim: "running"},
					{Metric: "dell.os10bgp4V2PeerState", Table: "dell.os10bgp4V2PeerTable", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "dell.os10bgp4V2PeerCountersTable", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "dell.os10bgp4V2PeerCountersTable", As: "sent"},
				},
			},
		},
		{
			name:        "Huawei routers expose common alert surface through virtual aliases",
			sysObjectID: "1.3.6.1.4.1.2011.2.224.279",
			profileFile: "huawei-routers.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "huawei.hwBgpPeerAdminStatus", Table: "hwBgpPeerTable", As: "admin_enabled", Dim: "start"},
					{Metric: "huawei.hwBgpPeerState", Table: "hwBgpPeerTable", As: "established", Dim: "established"},
				},
				"bgpPeerFsmEstablishedTransitions": {
					{Metric: "huawei.hwBgpPeerFsmEstablishedCounter", Table: "hwBgpPeerTable"},
				},
				"bgpPeerUpdates": {
					{Metric: "huawei.hwBgpPeerInUpdateMsgCounter", Table: "hwBgpPeerMessageTable", As: "received"},
					{Metric: "huawei.hwBgpPeerOutUpdateMsgCounter", Table: "hwBgpPeerMessageTable", As: "sent"},
				},
			},
		},
		{
			name:        "Cumulus switches inherit the generic BGP4-MIB contract",
			sysObjectID: "1.3.6.1.4.1.40310",
			profileFile: "nvidia-cumulus-linux-switch.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "bgpPeerAdminStatus", Table: "bgpPeerTable", As: "admin_enabled", Dim: "start"},
					{Metric: "bgpPeerState", Table: "bgpPeerTable", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "bgpPeerTable", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "bgpPeerTable", As: "sent"},
				},
			},
		},
		{
			name:        "Alcatel dual-stack keeps IPv4 generic and adds IPv6 aliases",
			sysObjectID: "1.3.6.1.4.1.6486.801.1.1.2.1.1",
			profileFile: "alcatel-lucent-ent.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
				"bgpPeerAvailability": {
					{Metric: "bgpPeerAdminStatus", Table: "bgpPeerTable", As: "admin_enabled", Dim: "start"},
					{Metric: "bgpPeerState", Table: "bgpPeerTable", As: "established", Dim: "established"},
				},
				"bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "bgpPeerTable", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "bgpPeerTable", As: "sent"},
				},
				"alcatel.bgpPeerAvailability": {
					{Metric: "bgpPeerAdminStatus", Table: "alaBgpPeer6Table", As: "admin_enabled", Dim: "start"},
					{Metric: "bgpPeerState", Table: "alaBgpPeer6Table", As: "established", Dim: "established"},
				},
				"alcatel.bgpPeerUpdates": {
					{Metric: "bgpPeerInUpdates", Table: "alaBgpPeer6Table", As: "received"},
					{Metric: "bgpPeerOutUpdates", Table: "alaBgpPeer6Table", As: "sent"},
				},
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			matched := FindProfiles(tc.sysObjectID, "", nil)
			index := slices.IndexFunc(matched, func(p *Profile) bool {
				return strings.HasSuffix(p.SourceFile, tc.profileFile)
			})
			require.NotEqual(t, -1, index, "expected %s profile to match", tc.profileFile)

			profile := matched[index]
			for name, wantSources := range tc.virtuals {
				vmIndex := slices.IndexFunc(profile.Definition.VirtualMetrics, func(vm ddprofiledefinition.VirtualMetricConfig) bool {
					return vm.Name == name
				})
				require.NotEqual(t, -1, vmIndex, "expected virtual metric %s", name)

				vm := profile.Definition.VirtualMetrics[vmIndex]
				gotSources := vm.Sources
				if len(gotSources) == 0 && len(vm.Alternatives) > 0 {
					gotSources = vm.Alternatives[0].Sources
				}
				assert.Equal(t, wantSources, gotSources)
			}
		})
	}
}

func Test_BGPVirtualMetricContracts_HuaweiUpdatesHasStatisticsFallback(t *testing.T) {
	matched := FindProfiles("1.3.6.1.4.1.2011.2.224.279", "", nil)
	index := slices.IndexFunc(matched, func(p *Profile) bool {
		return strings.HasSuffix(p.SourceFile, "huawei-routers.yaml")
	})
	require.NotEqual(t, -1, index, "expected huawei-routers.yaml profile to match")

	profile := matched[index]
	vmIndex := slices.IndexFunc(profile.Definition.VirtualMetrics, func(vm ddprofiledefinition.VirtualMetricConfig) bool {
		return vm.Name == "bgpPeerUpdates"
	})
	require.NotEqual(t, -1, vmIndex, "expected virtual metric bgpPeerUpdates")

	vm := profile.Definition.VirtualMetrics[vmIndex]
	require.Len(t, vm.Alternatives, 2)

	assert.Equal(t, []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "huawei.hwBgpPeerInUpdateMsgCounter", Table: "hwBgpPeerMessageTable", As: "received"},
		{Metric: "huawei.hwBgpPeerOutUpdateMsgCounter", Table: "hwBgpPeerMessageTable", As: "sent"},
	}, vm.Alternatives[0].Sources)

	assert.Equal(t, []ddprofiledefinition.VirtualMetricSourceConfig{
		{Metric: "huawei.hwBgpPeerInUpdateMsgs", Table: "hwBgpPeerStatisticTable", As: "received"},
		{Metric: "huawei.hwBgpPeerOutUpdateMsgs", Table: "hwBgpPeerStatisticTable", As: "sent"},
	}, vm.Alternatives[1].Sources)
}
