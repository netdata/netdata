// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

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
		for _, topo := range prof.Definition.Topology {
			metricNames[string(topo.Kind)] = struct{}{}
			if topo.Symbol.Name != "" {
				metricNames[topo.Symbol.Name] = struct{}{}
			}
			for _, sym := range topo.Symbols {
				metricNames[sym.Name] = struct{}{}
			}
		}
		for _, res := range prof.Definition.Metadata {
			for field := range res.Fields {
				metadataFields[field] = struct{}{}
			}
		}
	}

	for name, tc := range map[string]struct {
		metric string
	}{
		"lldp": {metric: "lldp_rem"},
		"cdp":  {metric: "cdp_cache"},
		"fdb":  {metric: "fdb_entry"},
		"stp":  {metric: "stp_port"},
		"vtp":  {metric: "vtp_vlan"},
	} {
		t.Run("metric/"+name, func(t *testing.T) {
			assert.Contains(t, metricNames, tc.metric)
		})
	}
	for name, tc := range map[string]struct {
		field string
	}{
		"lldp-system-name": {field: "lldp_loc_sys_name"},
		"vtp-version":      {field: "vtp_version"},
	} {
		t.Run("metadata/"+name, func(t *testing.T) {
			assert.Contains(t, metadataFields, tc.field)
		})
	}
}

func TestFindTopologyProfiles_PrunesBGPToTopologyFields(t *testing.T) {
	tests := map[string]struct {
		manualProfile string
		validate      func(t *testing.T, row ddprofiledefinition.BGPConfig)
	}{
		"standard_peer_keeps_topology_fields": {
			manualProfile: "f5-big-ip",
			validate: func(t *testing.T, row ddprofiledefinition.BGPConfig) {
				t.Helper()

				assert.NotEqual(t, ddprofiledefinition.BGPAdminConfig{}, row.Admin)
				assert.NotEqual(t, ddprofiledefinition.BGPStateConfig{}, row.State)
				assert.NotEqual(t, ddprofiledefinition.BGPConnectionConfig{}, row.Connection)
				assert.Equal(t, ddprofiledefinition.BGPTrafficConfig{}, row.Traffic)
				assert.Equal(t, ddprofiledefinition.BGPTransitionsConfig{}, row.Transitions)
				assert.Equal(t, ddprofiledefinition.BGPTimersConfig{}, row.Timers)
				assert.Equal(t, ddprofiledefinition.BGPLastErrorConfig{}, row.LastError)
				assert.Empty(t, row.MetricTags)
			},
		},
		"huawei_peer_keeps_single_anchor_category": {
			manualProfile: "huawei-routers",
			validate: func(t *testing.T, row ddprofiledefinition.BGPConfig) {
				t.Helper()

				assert.Equal(t, ddprofiledefinition.BGPAdminConfig{}, row.Admin)
				assert.Equal(t, ddprofiledefinition.BGPStateConfig{}, row.State)
				assert.Equal(t, ddprofiledefinition.BGPConnectionConfig{}, row.Connection)
				assert.NotEqual(t, ddprofiledefinition.BGPTransitionsConfig{}, row.Transitions)
				assert.Equal(t, ddprofiledefinition.BGPTrafficConfig{}, row.Traffic)
				assert.Empty(t, row.MetricTags)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			profiles := (&Collector{}).findTopologyProfiles(ddsnmp.DeviceConnectionInfo{
				ManualProfiles: []string{tc.manualProfile},
			})

			var rows []ddprofiledefinition.BGPConfig
			for _, profile := range profiles {
				require.NotNil(t, profile.Definition)
				for _, row := range profile.Definition.BGP {
					rows = append(rows, row)
					assert.Equal(t, ddprofiledefinition.BGPRowKindPeer, row.Kind)
					assert.Equal(t, ddprofiledefinition.BGPRoutesConfig{}, row.Routes)
					assert.Equal(t, ddprofiledefinition.BGPRouteLimitsConfig{}, row.RouteLimits)
					assert.Equal(t, ddprofiledefinition.BGPDeviceCountsConfig{}, row.Device)
					assert.Empty(t, row.StaticTags)
				}
			}

			require.Len(t, rows, 1)
			tc.validate(t, rows[0])
		})
	}
}
