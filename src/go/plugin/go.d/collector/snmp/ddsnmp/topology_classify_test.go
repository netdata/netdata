// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmp

import (
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
	"github.com/stretchr/testify/assert"
)

func TestIsTopologyMetric(t *testing.T) {
	for _, name := range []string{
		"lldpLocPortEntry", "lldpLocManAddrEntry", "lldpRemEntry",
		"lldpRemManAddrEntry", "lldpRemManAddrCompatEntry",
		"cdpCacheEntry",
		"topologyIfNameEntry", "topologyIfStatusEntry", "topologyIfDuplexEntry", "topologyIpIfIndexEntry",
		"dot1dBasePortIfIndexEntry", "dot1dTpFdbEntry", "dot1qTpFdbEntry", "dot1qVlanCurrentEntry",
		"dot1dStpPortEntry", "vtpVlanEntry",
		"ipNetToPhysicalEntry", "ipNetToMediaEntry",
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

	for _, name := range []string{"ifTraffic", "lldpRemEntry", "", "uptime"} {
		assert.False(t, IsTopologySysUptimeMetric(name), "expected NOT uptime: %s", name)
	}
}

func TestLooksLikeTopologyIdentifier(t *testing.T) {
	for _, value := range []string{
		"lldpLocChassisId", "cdpDeviceId", "topology_if_name",
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
		Symbol: ddprofiledefinition.SymbolConfig{Name: "lldpLocPortEntry"},
	}))

	assert.True(t, MetricConfigContainsTopologyData(&ddprofiledefinition.MetricsConfig{
		Symbol: ddprofiledefinition.SymbolConfig{Name: "systemUptime"},
	}))

	assert.True(t, MetricConfigContainsTopologyData(&ddprofiledefinition.MetricsConfig{
		Symbols: []ddprofiledefinition.SymbolConfig{{Name: "dot1dTpFdbEntry"}},
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
				{Symbol: ddprofiledefinition.SymbolConfig{Name: "lldpRemEntry"}},
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
