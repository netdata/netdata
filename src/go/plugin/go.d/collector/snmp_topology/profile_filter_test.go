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

	assert.Contains(t, metricNames, "lldp_rem")
	assert.Contains(t, metricNames, "cdp_cache")
	assert.Contains(t, metricNames, "fdb_entry")
	assert.Contains(t, metricNames, "stp_port")
	assert.Contains(t, metricNames, "vtp_vlan")
	assert.Contains(t, metadataFields, "lldp_loc_sys_name")
	assert.Contains(t, metadataFields, "vtp_version")
}
