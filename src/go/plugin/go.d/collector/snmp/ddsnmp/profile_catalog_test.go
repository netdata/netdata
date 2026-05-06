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

	fallback := catalog.Resolve(ResolveRequest{
		SysObjectID:    "1.3.6.1.4",
		ManualProfiles: []string{"manual"},
		ManualPolicy:   ManualProfileFallback,
	}).Profiles()
	require.Len(t, fallback, 1)
	assert.Equal(t, "auto.yaml", fallback[0].SourceFile)

	augment := catalog.Resolve(ResolveRequest{
		SysObjectID:    "1.3.6.1.4",
		ManualProfiles: []string{"manual"},
		ManualPolicy:   ManualProfileAugment,
	}).Profiles()
	require.Len(t, augment, 2)
	assert.Equal(t, "auto.yaml", augment[0].SourceFile)
	assert.Equal(t, "manual.yaml", augment[1].SourceFile)

	override := catalog.Resolve(ResolveRequest{
		SysObjectID:    "1.3.6.1.4",
		ManualProfiles: []string{"manual"},
		ManualPolicy:   ManualProfileOverride,
	}).Profiles()
	require.Len(t, override, 1)
	assert.Equal(t, "manual.yaml", override[0].SourceFile)
}

func TestResolvedProfileSetProject_SeparatesMetricsAndTopology(t *testing.T) {
	resolved := &ResolvedProfileSet{profiles: []*Profile{projectionTestProfile()}}

	metricsProfiles := resolved.Project(ConsumerMetrics).Profiles()
	topologyProfiles := resolved.Project(ConsumerTopology).Profiles()

	require.Len(t, metricsProfiles, 1)
	metricsDef := metricsProfiles[0].Definition
	require.Len(t, metricsDef.Metrics, 2)
	assert.Empty(t, metricsDef.Topology)
	require.Len(t, metricsDef.VirtualMetrics, 1)
	require.Len(t, metricsDef.Metadata["device"].Fields, 1)
	assert.Contains(t, metricsDef.Metadata["device"].Fields, "vendor")
	require.Len(t, metricsDef.MetricTags, 1)
	assert.Equal(t, "model", metricsDef.MetricTags[0].Tag)

	require.Len(t, topologyProfiles, 1)
	topologyDef := topologyProfiles[0].Definition
	require.Len(t, topologyDef.Topology, 2)
	assert.Equal(t, ddprofiledefinition.KindLldpRem, topologyDef.Topology[0].Kind)
	assert.Empty(t, topologyDef.Metrics)
	assert.Empty(t, topologyDef.VirtualMetrics)
	require.Len(t, topologyDef.Metadata["device"].Fields, 1)
	assert.Contains(t, topologyDef.Metadata["device"].Fields, "lldp_loc_sys_name")
	require.Len(t, topologyDef.MetricTags, 1)
	assert.Equal(t, "lldp_loc_chassis_id", topologyDef.MetricTags[0].Tag)
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
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{Name: "sysNameTotal", Sources: []ddprofiledefinition.VirtualMetricSourceConfig{{Metric: "sysName"}}},
			},
		},
	}
}
