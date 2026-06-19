// SPDX-License-Identifier: GPL-3.0-or-later

package snmptopology

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp"
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
