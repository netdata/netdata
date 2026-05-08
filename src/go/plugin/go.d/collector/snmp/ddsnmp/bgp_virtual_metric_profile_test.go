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

	tests := map[string]struct {
		sysObjectID string
		profileFile string
		virtuals    map[string][]ddprofiledefinition.VirtualMetricSourceConfig
	}{
		"Nokia SR OS overrides generic contract with TiMOS peer tables": {
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
		"Arista switch overrides generic contract with Arista peer tables": {
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
		"Dell OS10 exposes common contract from Dell peer tables": {
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
		"Huawei routers expose common alert surface through virtual aliases": {
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
		"Alcatel dual-stack keeps IPv6 virtual aliases after standard BGP migration": {
			sysObjectID: "1.3.6.1.4.1.6486.801.1.1.2.1.1",
			profileFile: "alcatel-lucent-ent.yaml",
			virtuals: map[string][]ddprofiledefinition.VirtualMetricSourceConfig{
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

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
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
