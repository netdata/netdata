// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

func TestSelectTopologyRefreshProfiles_KeepsTopologyMetadataOnly(t *testing.T) {
	profiles := []*ddsnmp.Profile{{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metadata: ddprofiledefinition.MetadataConfig{
				"device": {
					Fields: map[string]ddprofiledefinition.MetadataField{
						"lldp_loc_sys_name": {
							Symbol: ddprofiledefinition.SymbolConfig{Name: "lldpLocSysName"},
						},
						"vendor": {
							Value: "Juniper",
						},
					},
				},
			},
			MetricTags: []ddprofiledefinition.MetricTagConfig{
				{
					Tag: "lldp_loc_chassis_id",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						Name: "lldpLocChassisId",
					},
				},
				{
					Tag: "ups_model",
					Symbol: ddprofiledefinition.SymbolConfigCompat{
						Name: "upsModel",
					},
				},
			},
			Metrics: []ddprofiledefinition.MetricsConfig{
				{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.0.8802.1.1.2.1.3.7.1.2",
						Name: "_topology_lldp_loc_port_entry",
					},
				},
				{
					Symbol: ddprofiledefinition.SymbolConfig{
						OID:  "1.3.6.1.2.1.33.1.2.1.0",
						Name: "upsBatteryStatus",
					},
				},
			},
			VirtualMetrics: []ddprofiledefinition.VirtualMetricConfig{
				{
					Name: "lldpLocalPortRows",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{Metric: "_topology_lldp_loc_port_entry", Table: "lldpLocPortTable"},
					},
				},
				{
					Name: "upsBatteryStatusTotal",
					Sources: []ddprofiledefinition.VirtualMetricSourceConfig{
						{Metric: "upsBatteryStatus", Table: "upsBatteryTable"},
					},
				},
			},
		},
	}}

	selected := selectTopologyRefreshProfiles(profiles)
	require.Len(t, selected, 1)

	prof := selected[0]
	require.NotNil(t, prof.Definition)
	require.Len(t, prof.Definition.Metrics, 1)
	assert.Equal(t, "_topology_lldp_loc_port_entry", prof.Definition.Metrics[0].Symbol.Name)
	require.Len(t, prof.Definition.VirtualMetrics, 1)
	assert.Equal(t, "lldpLocalPortRows", prof.Definition.VirtualMetrics[0].Name)
	require.Len(t, prof.Definition.MetricTags, 1)
	assert.Equal(t, "lldp_loc_chassis_id", prof.Definition.MetricTags[0].Tag)
	require.Len(t, prof.Definition.Metadata, 1)
	assert.Contains(t, prof.Definition.Metadata["device"].Fields, "lldp_loc_sys_name")
	assert.NotContains(t, prof.Definition.Metadata["device"].Fields, "vendor")
}

func TestFindTopologyProfiles_UsesDeclarativeProfileExtensions(t *testing.T) {
	profiles := (&Collector{}).findTopologyProfiles(ddsnmp.DeviceConnectionInfo{
		SysObjectID: "1.3.6.1.4.1.9.1.1",
	})
	require.NotEmpty(t, profiles)

	metricNames := make(map[string]struct{})
	metadataFields := make(map[string]struct{})

	for _, prof := range profiles {
		require.NotNil(t, prof.Definition)
		for _, metric := range prof.Definition.Metrics {
			if metric.Symbol.Name != "" {
				metricNames[metric.Symbol.Name] = struct{}{}
			}
			for _, sym := range metric.Symbols {
				metricNames[sym.Name] = struct{}{}
			}
		}
		for _, res := range prof.Definition.Metadata {
			for field := range res.Fields {
				metadataFields[field] = struct{}{}
			}
		}
	}

	assert.Contains(t, metricNames, "_topology_lldp_rem_entry")
	assert.Contains(t, metricNames, "_topology_cdp_cache_entry")
	assert.Contains(t, metricNames, "_topology_fdb_entry")
	assert.Contains(t, metricNames, "_topology_stp_port_entry")
	assert.Contains(t, metricNames, "_topology_vtp_vlan_entry")
	assert.Contains(t, metadataFields, "lldp_loc_sys_name")
	assert.Contains(t, metadataFields, "vtp_version")
}
