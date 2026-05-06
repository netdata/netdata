// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
)

func TestIsTopologyMetric(t *testing.T) {
	for _, name := range []string{
		"_topology_lldp_loc_port_entry", "_topology_lldp_loc_man_addr_entry", "_topology_lldp_rem_entry",
		"_topology_lldp_rem_man_addr_entry", "_topology_lldp_rem_man_addr_compat_entry",
		"_topology_cdp_cache_entry",
		"_topology_if_name_entry", "_topology_if_status_entry", "_topology_if_duplex_entry", "_topology_ip_if_index_entry",
		"_topology_bridge_port_if_index_entry", "_topology_fdb_entry", "_topology_qbridge_fdb_entry", "_topology_qbridge_vlan_entry",
		"_topology_stp_port_entry", "_topology_vtp_vlan_entry",
		"_topology_arp_entry", "_topology_arp_legacy_entry",
	} {
		assert.True(t, IsTopologyMetric(name), "expected topology: %s", name)
	}

	for _, name := range []string{
		"ifTraffic", "ifErrors", "sysUptime", "upsBatteryStatus", "", "cpu.usage",
	} {
		assert.False(t, IsTopologyMetric(name), "expected NOT topology: %s", name)
	}
}

func TestIsTopologySysUptimeMetric(t *testing.T) {
	for _, name := range []string{"sysUptime", "systemUptime", "SYSUPTIME", "SystemUptime", " sysUptime "} {
		assert.True(t, IsTopologySysUptimeMetric(name), "expected uptime: %s", name)
	}

	for _, name := range []string{"ifTraffic", "_topology_lldp_rem_entry", "", "uptime"} {
		assert.False(t, IsTopologySysUptimeMetric(name), "expected NOT uptime: %s", name)
	}
}

func TestLooksLikeTopologyIdentifier(t *testing.T) {
	for _, value := range []string{
		"lldpLocChassisId", "cdpDeviceId", "topology_if_name", "_topology_lldp_rem_entry",
		"dot1dBasePort", "dot1qVlanId", "stpPortState",
		"vtpVlanName", "fdbMac", "bridgeIfIndex", "arpIp",
		"LLDP_CAPS", "CDP_PORT",
	} {
		assert.True(t, LooksLikeTopologyIdentifier(value), "expected topology identifier: %s", value)
	}

	for _, value := range []string{
		"ifTraffic", "sysName", "cpu_usage", "", "snmp_host", "upsModel",
	} {
		assert.False(t, LooksLikeTopologyIdentifier(value), "expected NOT topology identifier: %s", value)
	}
}

func TestMetricConfigContainsTopologyData(t *testing.T) {
	assert.True(t, MetricConfigContainsTopologyData(&ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{Name: "_topology_lldp_loc_port_entry"},
	}))

	assert.True(t, MetricConfigContainsTopologyData(&ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{Name: "systemUptime"},
	}))

	assert.True(t, MetricConfigContainsTopologyData(&ddprofiledefinition.MetricsConfig{
		Symbols: []ddprofiledefinition.SymbolConfig{{Name: "_topology_fdb_entry"}},
	}))

	assert.False(t, MetricConfigContainsTopologyData(&ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{Name: "ifTraffic"},
	}))

	assert.False(t, MetricConfigContainsTopologyData(nil))
}

func TestProfileContainsTopologyData(t *testing.T) {
	assert.True(t, ProfileContainsTopologyData(&Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{Symbol: ddprofiledefinition.SymbolConfig{Name: "_topology_lldp_rem_entry"}},
			},
		},
	}))

	assert.True(t, ProfileContainsTopologyData(&Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metadata: ddprofiledefinition.MetadataConfig{
				"device": {
					Fields: map[string]ddprofiledefinition.MetadataField{
						"lldp_loc_sys_name": {
							Symbol: ddprofiledefinition.SymbolConfig{Name: "lldpLocSysName"},
						},
					},
				},
			},
		},
	}))

	assert.False(t, ProfileContainsTopologyData(&Profile{
		Definition: &ddprofiledefinition.ProfileDefinition{
			Metrics: []ddprofiledefinition.MetricsConfig{
				{Symbol: ddprofiledefinition.SymbolConfig{Name: "ifTraffic"}},
			},
		},
	}))

	assert.False(t, ProfileContainsTopologyData(nil))
}
