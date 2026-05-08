// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestCatalogResolve_ManualProfilePolicies(t *testing.T) {
	catalog := &Catalog{profiles: []*Profile{
		{
			SourceFile: "auto.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				Selector: ddprofiledefinition.SelectorSpec{
					{SysObjectID: ddprofiledefinition.SelectorIncludeExclude{Include: []string{"1.3.6.1.*"}}},
				},
				Metrics: []ddprofiledefinition.MetricsConfig{
					{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"}},
				},
			},
		},
		{
			SourceFile: "manual.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				Metrics: []ddprofiledefinition.MetricsConfig{
					{Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.1.0", Name: "sysDescr"}},
				},
			},
		},
	}}

	tests := map[string]struct {
		policy   ManualProfilePolicy
		expected []string
	}{
		"fallback_keeps_auto_match": {policy: ManualProfileFallback, expected: []string{"auto.yaml"}},
		"augment_appends_manual":    {policy: ManualProfileAugment, expected: []string{"auto.yaml", "manual.yaml"}},
		"override_uses_manual_only": {policy: ManualProfileOverride, expected: []string{"manual.yaml"}},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profiles := catalog.Resolve(ResolveRequest{
				SysObjectID:    "1.3.6.1.4",
				ManualProfiles: []string{"manual"},
				ManualPolicy:   tc.policy,
			}).Profiles()

			require.Len(t, profiles, len(tc.expected))
			for i, expected := range tc.expected {
				assert.Equal(t, expected, profiles[i].SourceFile)
			}
		})
	}
}

func TestResolvedProfileSetProject_SeparatesMetricsAndTopology(t *testing.T) {
	tests := map[string]struct {
		consumer      ProfileConsumer
		metrics       int
		topology      int
		licensing     int
		bgp           int
		virtual       int
		metadataField string
		metricTag     string
		sysobjectID   string
		firstKind     ddprofiledefinition.TopologyKind
	}{
		"metrics_projection": {
			consumer:      ConsumerMetrics,
			metrics:       2,
			virtual:       1,
			metadataField: "vendor",
			metricTag:     "model",
			sysobjectID:   "sysobjectid_vendor",
		},
		"topology_projection": {
			consumer:      ConsumerTopology,
			topology:      2,
			metadataField: "lldp_loc_sys_name",
			metricTag:     "lldp_loc_chassis_id",
			sysobjectID:   "sysobjectid_topology_vendor",
			firstKind:     ddprofiledefinition.KindLldpRem,
		},
		"licensing_projection": {
			consumer:      ConsumerLicensing,
			licensing:     1,
			metadataField: "license_vendor",
			metricTag:     "license_model",
			sysobjectID:   "sysobjectid_license_vendor",
		},
		"bgp_projection": {
			consumer:      ConsumerBGP,
			bgp:           1,
			metadataField: "bgp_vendor",
			metricTag:     "bgp_model",
			sysobjectID:   "sysobjectid_bgp_vendor",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			resolved := &ResolvedProfileSet{profiles: []*Profile{projectionTestProfile()}}

			profiles := resolved.Project(tc.consumer).Profiles()

			require.Len(t, profiles, 1)
			def := profiles[0].Definition
			require.Len(t, def.Metrics, tc.metrics)
			require.Len(t, def.Topology, tc.topology)
			require.Len(t, def.Licensing, tc.licensing)
			require.Len(t, def.BGP, tc.bgp)
			require.Len(t, def.VirtualMetrics, tc.virtual)
			require.Len(t, def.Metadata["device"].Fields, 1)
			assert.Contains(t, def.Metadata["device"].Fields, tc.metadataField)
			require.Len(t, def.MetricTags, 1)
			assert.Equal(t, tc.metricTag, def.MetricTags[0].Tag)
			require.Len(t, def.SysobjectIDMetadata, 1)
			assert.Contains(t, def.SysobjectIDMetadata[0].Metadata, tc.sysobjectID)
			if tc.firstKind != "" {
				assert.Equal(t, tc.firstKind, def.Topology[0].Kind)
			}
		})
	}
}

func TestResolvedProfileSetProject_MetricsAndLicensing(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{projectionTestProfile()}}

	profiles := resolved.Project(ConsumerMetrics, ConsumerLicensing).Profiles()

	require.Len(t, profiles, 1)
	def := profiles[0].Definition
	require.Len(t, def.Metrics, 2)
	require.Len(t, def.Licensing, 1)
	require.Len(t, def.VirtualMetrics, 1)
	require.Empty(t, def.Topology)
	assert.Contains(t, def.Metadata["device"].Fields, "vendor")
	assert.Contains(t, def.Metadata["device"].Fields, "license_vendor")
	assert.NotContains(t, def.Metadata["device"].Fields, "lldp_loc_sys_name")
	require.Len(t, def.MetricTags, 2)
	assert.Equal(t, "model", def.MetricTags[0].Tag)
	assert.Equal(t, "license_model", def.MetricTags[1].Tag)
	require.Len(t, def.SysobjectIDMetadata, 1)
	assert.Contains(t, def.SysobjectIDMetadata[0].Metadata, "sysobjectid_vendor")
	assert.Contains(t, def.SysobjectIDMetadata[0].Metadata, "sysobjectid_license_vendor")
	assert.NotContains(t, def.SysobjectIDMetadata[0].Metadata, "sysobjectid_topology_vendor")
}

func TestResolvedProfileSetProject_MetricsAndBGP(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{projectionTestProfile()}}

	profiles := resolved.Project(ConsumerMetrics, ConsumerBGP).Profiles()

	require.Len(t, profiles, 1)
	def := profiles[0].Definition
	require.Len(t, def.Metrics, 2)
	require.Len(t, def.BGP, 1)
	require.Len(t, def.VirtualMetrics, 1)
	require.Empty(t, def.Topology)
	require.Empty(t, def.Licensing)
	assert.Contains(t, def.Metadata["device"].Fields, "vendor")
	assert.Contains(t, def.Metadata["device"].Fields, "bgp_vendor")
	assert.NotContains(t, def.Metadata["device"].Fields, "lldp_loc_sys_name")
	require.Len(t, def.MetricTags, 2)
	assert.Equal(t, "model", def.MetricTags[0].Tag)
	assert.Equal(t, "bgp_model", def.MetricTags[1].Tag)
	require.Len(t, def.SysobjectIDMetadata, 1)
	assert.Contains(t, def.SysobjectIDMetadata[0].Metadata, "sysobjectid_vendor")
	assert.Contains(t, def.SysobjectIDMetadata[0].Metadata, "sysobjectid_bgp_vendor")
	assert.NotContains(t, def.SysobjectIDMetadata[0].Metadata, "sysobjectid_topology_vendor")
}

func TestResolvedProfileSetProject_UnscopedMetricTagsPropagateToLicensing(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{
		{
			SourceFile: "licensing.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{
					{MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "device_model"}},
				},
				Licensing: []ddprofiledefinition.LicensingConfig{
					{
						ID:              "license",
						OriginProfileID: "licensing.yaml",
						Identity: ddprofiledefinition.LicenseIdentityConfig{
							ID: ddprofiledefinition.LicenseValueConfig{Value: "license"},
						},
					},
				},
			},
		},
	}}

	profiles := resolved.Project(ConsumerLicensing).Profiles()

	require.Len(t, profiles, 1)
	require.Len(t, profiles[0].Definition.MetricTags, 1)
	assert.Equal(t, "device_model", profiles[0].Definition.MetricTags[0].Tag)
}

func TestResolvedProfileSetProject_UnscopedMetricTagsPropagateToBGP(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{
		{
			SourceFile: "bgp.yaml",
			Definition: &ddprofiledefinition.ProfileDefinition{
				MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{
					{MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "device_model"}},
				},
				BGP: []ddprofiledefinition.BGPConfig{
					{
						ID:   "peer",
						Kind: ddprofiledefinition.BGPRowKindPeer,
						Identity: ddprofiledefinition.BGPIdentityConfig{
							Neighbor: ddprofiledefinition.BGPValueConfig{Value: "192.0.2.1"},
							RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"},
						},
						State: ddprofiledefinition.BGPStateConfig{
							BGPValueConfig: ddprofiledefinition.BGPValueConfig{Value: "established"},
						},
					},
				},
			},
		},
	}}

	profiles := resolved.Project(ConsumerBGP).Profiles()

	require.Len(t, profiles, 1)
	require.Len(t, profiles[0].Definition.MetricTags, 1)
	assert.Equal(t, "device_model", profiles[0].Definition.MetricTags[0].Tag)
}

func TestResolvedProfileSetProject_DoesNotShareMutableProjectionState(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{projectionTestProfile()}}

	view1 := resolved.Project(ConsumerTopology).Profiles()
	view2 := resolved.Project(ConsumerTopology).Profiles()

	require.Len(t, view1, 1)
	require.Len(t, view2, 1)

	view1[0].Definition.Topology[0].MetricTags[0].Tag = "mutated"
	view1[0].Definition.Metadata["device"].Fields["lldp_loc_sys_name"] = ddprofiledefinition.MetadataField{Value: "mutated"}

	assert.Equal(t, "lldp_rem_index", view2[0].Definition.Topology[0].MetricTags[0].Tag)
	assert.NotEqual(t, "mutated", view2[0].Definition.Metadata["device"].Fields["lldp_loc_sys_name"].Value)

	fresh := resolved.Project(ConsumerTopology).Profiles()
	assert.Equal(t, "lldp_rem_index", fresh[0].Definition.Topology[0].MetricTags[0].Tag)
	assert.NotEqual(t, "mutated", fresh[0].Definition.Metadata["device"].Fields["lldp_loc_sys_name"].Value)
}

func TestProjectedViewFilterByKind(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{projectionTestProfile()}}

	view := resolved.Project(ConsumerTopology).FilterByKind(map[ddprofiledefinition.TopologyKind]bool{
		ddprofiledefinition.KindStpPort: true,
	}).Profiles()

	require.Len(t, view, 1)
	require.Len(t, view[0].Definition.Topology, 1)
	assert.Equal(t, ddprofiledefinition.KindStpPort, view[0].Definition.Topology[0].Kind)
	assert.Empty(t, view[0].Definition.Metrics)

	unfiltered := resolved.Project(ConsumerTopology).Profiles()
	require.Len(t, unfiltered[0].Definition.Topology, 2)
	assert.Empty(t, unfiltered[0].Definition.Metrics)
}

func projectionTestProfile() *Profile {
	return &Profile{
		SourceFile: "projection.yaml",
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metadata: ddprofiledefinition.MetadataConfig{
				"device": {
					Fields: map[string]ddprofiledefinition.MetadataField{
						"vendor": {
							Value:     "Cisco",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerMetrics},
						},
						"lldp_loc_sys_name": {
							Symbol:    ddprofiledefinition.SymbolConfig{Name: "lldpLocSysName"},
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerTopology},
						},
						"license_vendor": {
							Value:     "Cisco licensing",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerLicensing},
						},
						"bgp_vendor": {
							Value:     "Cisco BGP",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerBGP},
						},
					},
				},
			},
			SysobjectIDMetadata: []ddprofiledefinition.SysobjectIDMetadataEntryConfig{
				{
					SysobjectID: "1.3.6.1.4.1.9",
					Metadata: map[string]ddprofiledefinition.MetadataField{
						"sysobjectid_vendor": {
							Value:     "Cisco",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerMetrics},
						},
						"sysobjectid_topology_vendor": {
							Value:     "Cisco topology",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerTopology},
						},
						"sysobjectid_license_vendor": {
							Value:     "Cisco licensing",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerLicensing},
						},
						"sysobjectid_bgp_vendor": {
							Value:     "Cisco BGP",
							Consumers: ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerBGP},
						},
					},
				},
			},
			MetricTags: []ddprofiledefinition.GlobalMetricTagConfig{
				{
					MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "model"},
					Consumers:       ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerMetrics},
				},
				{
					MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "lldp_loc_chassis_id"},
					Consumers:       ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerTopology},
				},
				{
					MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "license_model"},
					Consumers:       ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerLicensing},
				},
				{
					MetricTagConfig: ddprofiledefinition.MetricTagConfig{Tag: "bgp_model"},
					Consumers:       ddprofiledefinition.ConsumerSet{ddprofiledefinition.ConsumerBGP},
				},
			},
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.3.0", Name: "systemUptime"},
				},
				{
					Symbol: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.1.5.0", Name: "sysName"},
				},
			},
			Topology: []ddprofiledefinition.TopologyConfig{
				{
					Kind: ddprofiledefinition.KindLldpRem,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{OID: "1.0.8802.1.1.2.1.4.1", Name: "lldpRemTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.0.8802.1.1.2.1.4.1.1.6", Name: "lldp_rem"},
						},
						MetricTags: ddprofiledefinition.MetricTagConfigList{
							{Tag: "lldp_rem_index", Index: 3},
						},
					},
				},
				{
					Kind: ddprofiledefinition.KindStpPort,
					MetricsConfig: ddprofiledefinition.MetricsConfig{
						Table: ddprofiledefinition.SymbolConfig{OID: "1.3.6.1.2.1.17.2.15.1", Name: "dot1dStpPortTable"},
						Symbols: []ddprofiledefinition.SymbolConfig{
							{OID: "1.3.6.1.2.1.17.2.15.1.3", Name: "stp_port"},
						},
						MetricTags: ddprofiledefinition.MetricTagConfigList{
							{Tag: "stp_port", Index: 1},
						},
					},
				},
			},
			Licensing: []ddprofiledefinition.LicensingConfig{
				{
					ID:              "smart",
					OriginProfileID: "_cisco-licensing-smart.yaml",
					Identity: ddprofiledefinition.LicenseIdentityConfig{
						ID: ddprofiledefinition.LicenseValueConfig{Value: "smart"},
					},
				},
			},
			BGP: []ddprofiledefinition.BGPConfig{
				{
					ID:   "peer",
					Kind: ddprofiledefinition.BGPRowKindPeer,
					Identity: ddprofiledefinition.BGPIdentityConfig{
						Neighbor: ddprofiledefinition.BGPValueConfig{Value: "192.0.2.1"},
						RemoteAS: ddprofiledefinition.BGPValueConfig{Value: "65001"},
					},
				},
			},
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{Name: "sysNameTotal", Sources: []ddprofiledefinition.VirtualMetricSourceConfig{{Metric: "sysName"}}},
			},
		},
	}
}
