// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _       = os.ReadFile("testdata/config.json")
	dataConfigYAML, _       = os.ReadFile("testdata/config.yaml")
	dataLegacyBGPPeers, _   = os.ReadFile("testdata/legacy_bgp_peers.xml")
	dataAdvancedBGPPeers, _ = os.ReadFile("testdata/advanced_bgp_peers.xml")
	dataSystemInfo, _       = os.ReadFile("testdata/system_info.xml")
	dataHAState, _          = os.ReadFile("testdata/ha_state.xml")
	dataEnvironment, _      = os.ReadFile("testdata/environment.xml")
	dataLicenses, _         = os.ReadFile("testdata/licenses.xml")
	dataIPSecSA, _          = os.ReadFile("testdata/ipsec_sa.xml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":       dataConfigJSON,
		"dataConfigYAML":       dataConfigYAML,
		"dataLegacyBGPPeers":   dataLegacyBGPPeers,
		"dataAdvancedBGPPeers": dataAdvancedBGPPeers,
		"dataSystemInfo":       dataSystemInfo,
		"dataHAState":          dataHAState,
		"dataEnvironment":      dataEnvironment,
		"dataLicenses":         dataLicenses,
		"dataIPSecSA":          dataIPSecSA,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	collecttest.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   func(*Collector)
		wantFail bool
	}{
		"success with API key": {
			config: func(c *Collector) {
				c.APIKey = "key"
			},
		},
		"success with username and password": {
			config: func(c *Collector) {
				c.Username = "user"
				c.Password = "pass"
			},
		},
		"fail when URL not set": {
			config: func(c *Collector) {
				c.URL = ""
				c.APIKey = "key"
			},
			wantFail: true,
		},
		"fail when auth not set": {
			config:   func(*Collector) {},
			wantFail: true,
		},
		"fail when force_http2 is set": {
			config: func(c *Collector) {
				c.APIKey = "key"
				c.ForceHTTP2 = true
			},
			wantFail: true,
		},
		"fail when unsupported request body is set": {
			config: func(c *Collector) {
				c.APIKey = "key"
				c.Body = "body"
			},
			wantFail: true,
		},
		"fail when all metricsets are disabled": {
			config: func(c *Collector) {
				c.APIKey = "key"
				c.CollectBGP = false
				c.CollectSystem = false
				c.CollectHA = false
				c.CollectEnvironment = false
				c.CollectLicenses = false
				c.CollectIPSec = false
			},
			wantFail: true,
		},
		"fail when cardinality cap is invalid": {
			config: func(c *Collector) {
				c.APIKey = "key"
				c.MaxIPSecTunnels = 0
			},
			wantFail: true,
		},
		"fail when selector is invalid": {
			config: func(c *Collector) {
				c.APIKey = "key"
				c.BGPPeers = matcher.SimpleExpr{Includes: []string{"~ (ab"}}
			},
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			test.config(collr)
			collr.newAPIClient = func(Config) (panosAPIClient, error) {
				return &mockAPIClient{}, nil
			}

			if test.wantFail {
				assert.Error(t, collr.Init(context.Background()))
			} else {
				assert.NoError(t, collr.Init(context.Background()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		client   panosAPIClient
		wantFail bool
	}{
		"success with BGP peers": {
			client: &mockAPIClient{
				responses: map[string][]byte{legacyBGPPeerCommand: dataLegacyBGPPeers},
			},
		},
		"fail when no BGP peers": {
			client: &mockAPIClient{
				responses: map[string][]byte{legacyBGPPeerCommand: []byte(`<response status="success"><result></result></response>`)},
			},
			wantFail: true,
		},
		"fail on API error": {
			client: &mockAPIClient{
				errors: map[string]error{legacyBGPPeerCommand: errors.New("api error")},
			},
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			enableOnlyBGP(collr)
			collr.apiClient = test.client

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	collr := New()

	collecttest.AssertChartTemplateSchema(t, collr.ChartTemplateYAML())
	spec, err := charttpl.DecodeYAML([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestCollector_CollectV2WritesMetricStore(t *testing.T) {
	collr := New()
	enableOnlyBGP(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{legacyBGPPeerCommand: dataLegacyBGPPeers},
	}

	reader := collectV2Reader(t, collr)

	value, ok := reader.Value("bgp_peer_state_established", metrix.Labels{
		"vr":            "default",
		"peer_address":  "192.0.2.1",
		"local_address": "192.0.2.254",
		"remote_as":     "65001",
		"peer_group":    "edge",
	})
	require.True(t, ok)
	assert.Equal(t, float64(1), value)

	value, ok = reader.Value("bgp_peer_prefixes_received_total", metrix.Labels{
		"vr":            "default",
		"peer_address":  "192.0.2.1",
		"local_address": "192.0.2.254",
		"remote_as":     "65001",
		"peer_group":    "edge",
		"afi":           "ipv4",
		"safi":          "unicast",
	})
	require.True(t, ok)
	assert.Equal(t, float64(40), value)
}

func TestCollector_CollectV2WritesIPSecTunnelIdentity(t *testing.T) {
	collr := New()
	enableOnlyIPSec(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{ipsecSACommand: dataIPSecSA},
	}

	reader := collectV2Reader(t, collr)

	value, ok := reader.Value("ipsec_tunnel_sa_lifetime_time_until_expiration", metrix.Labels{
		"tunnel":     "branch-a",
		"gateway":    "gw-branch-a",
		"remote":     "198.51.100.10",
		"tunnel_id":  "66",
		"protocol":   "ESP",
		"encryption": "G256",
	})
	require.True(t, ok)
	assert.Equal(t, float64(1727), value)
}

func TestCollector_Collect_LegacyBGP(t *testing.T) {
	collr := New()
	enableOnlyBGP(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{legacyBGPPeerCommand: dataLegacyBGPPeers},
	}

	mx := collectMap(t, collr)

	require.NotNil(t, mx)
	assert.Equal(t, routingEngineLegacy, collr.routingEngine)
	assert.Equal(t, int64(1), mx["bgp_peer_default_192_0_2_1_state_established"])
	assert.Equal(t, int64(3600), mx["bgp_peer_default_192_0_2_1_uptime"])
	assert.Equal(t, int64(100), mx["bgp_peer_default_192_0_2_1_messages_in"])
	assert.Equal(t, int64(120), mx["bgp_peer_default_192_0_2_1_messages_out"])
	assert.Equal(t, int64(40), mx["bgp_peer_default_192_0_2_1_ipv4_unicast_prefixes_received_total"])
	assert.Equal(t, int64(8), mx["bgp_peer_default_192_0_2_1_ipv4_unicast_prefixes_advertised"])
	assert.Equal(t, int64(1), mx["bgp_peer_blue_198_51_100_1_state_active"])
	assert.Equal(t, int64(330), mx["bgp_peer_blue_198_51_100_1_uptime"])
	assert.Equal(t, int64(1), mx["bgp_vr_default_peers_configured"])
	assert.Equal(t, int64(1), mx["bgp_vr_default_peers_established"])
	assert.Equal(t, int64(1), mx["bgp_vr_blue_peers_configured"])
	assert.Equal(t, int64(0), mx["bgp_vr_blue_peers_established"])
	assert.Equal(t, int64(2), mx["bgp_peers_collection_discovered"])
	assert.Equal(t, int64(2), mx["bgp_peers_collection_monitored"])
	assert.Equal(t, int64(2), mx["bgp_prefix_families_collection_discovered"])
	assert.Equal(t, int64(2), mx["bgp_prefix_families_collection_monitored"])
	assert.Equal(t, int64(2), mx["bgp_virtual_routers_collection_discovered"])
	assert.Equal(t, int64(2), mx["bgp_virtual_routers_collection_monitored"])

	assert.Len(t, *collr.Charts(), 23)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_AdvancedBGPFallback(t *testing.T) {
	collr := New()
	enableOnlyBGP(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			legacyBGPPeerCommand:       []byte(`<response status="success"><result></result></response>`),
			advancedBGPPeerCommands[0]: dataAdvancedBGPPeers,
			advancedBGPPeerCommands[1]: []byte(`<response status="success"><result></result></response>`),
			advancedBGPPeerCommands[2]: []byte(`<response status="success"><result></result></response>`),
		},
	}

	mx := collectMap(t, collr)

	require.NotNil(t, mx)
	assert.Equal(t, routingEngineAdvanced, collr.routingEngine)
	assert.Equal(t, advancedBGPPeerCommands[0], collr.bgpCommand)
	assert.Equal(t, int64(1), mx["bgp_peer_lr_a_203_0_113_1_state_openconfirm"])
	assert.Equal(t, int64(93784), mx["bgp_peer_lr_a_203_0_113_1_uptime"])
	assert.Equal(t, int64(100), mx["bgp_peer_lr_a_203_0_113_1_ipv4_unicast_prefixes_received_total"])
	assert.Equal(t, int64(1), mx["bgp_vr_lr_a_peers_configured"])

	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_AppliesBGPCardinalityControls(t *testing.T) {
	collr := New()
	enableOnlyBGP(collr)
	collr.MaxBGPPrefixFamiliesPerPeer = 1
	collr.MaxBGPVirtualRouters = 1
	collr.BGPPeers = matcher.SimpleExpr{Includes: []string{"* default/*"}}
	require.NoError(t, collr.initEntitySelectors())
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{legacyBGPPeerCommand: dataLegacyBGPPeers},
	}

	mx := collectMap(t, collr)

	require.NotNil(t, mx)
	assert.Equal(t, int64(2), mx["bgp_peers_collection_discovered"])
	assert.Equal(t, int64(1), mx["bgp_peers_collection_monitored"])
	assert.Equal(t, int64(1), mx["bgp_peers_collection_omitted_by_selector"])
	assert.Equal(t, int64(0), mx["bgp_peers_collection_omitted_by_limit"])
	assert.Equal(t, int64(2), mx["bgp_prefix_families_collection_discovered"])
	assert.Equal(t, int64(1), mx["bgp_prefix_families_collection_monitored"])
	assert.Equal(t, int64(0), mx["bgp_prefix_families_collection_omitted_by_selector"])
	assert.Equal(t, int64(1), mx["bgp_prefix_families_collection_omitted_by_limit"])
	assert.Equal(t, int64(2), mx["bgp_virtual_routers_collection_discovered"])
	assert.Equal(t, int64(1), mx["bgp_virtual_routers_collection_monitored"])
	assert.Equal(t, int64(0), mx["bgp_virtual_routers_collection_omitted_by_selector"])
	assert.Equal(t, int64(1), mx["bgp_virtual_routers_collection_omitted_by_limit"])
	assert.Equal(t, int64(1), mx["bgp_peer_default_192_0_2_1_state_established"])
	assert.NotContains(t, mx, "bgp_peer_blue_198_51_100_1_state_active")
	assert.Equal(t, int64(40), mx["bgp_peer_default_192_0_2_1_ipv4_unicast_prefixes_received_total"])
	assert.NotContains(t, mx, "bgp_peer_default_192_0_2_1_ipv6_unicast_prefixes_received_total")
	assert.Contains(t, mx, "bgp_vr_blue_peers_configured")
	assert.NotContains(t, mx, "bgp_vr_default_peers_configured")
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_NoBGPStateIsCached(t *testing.T) {
	now := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
	api := &mockAPIClient{}
	collr := New()
	enableOnlyBGP(collr)
	collr.apiClient = api
	collr.now = func() time.Time { return now }

	mx, err := collr.collect()
	assert.Nil(t, mx)
	assert.Error(t, err)
	assert.Equal(t, routingEngineNone, collr.routingEngine)
	assert.Equal(t, 4, len(api.commands))

	api.commands = nil
	mx, err = collr.collect()
	assert.Nil(t, mx)
	assert.Error(t, err)
	assert.Empty(t, api.commands)

	now = now.Add(noBGPReprobeInterval)
	mx, err = collr.collect()
	assert.Nil(t, mx)
	assert.Error(t, err)
	assert.Equal(t, 4, len(api.commands))
}

func TestCollector_Collect_RemovesChartsAfterGraceMisses(t *testing.T) {
	collr := New()
	enableOnlyBGP(collr)
	api := &mockAPIClient{
		responses: map[string][]byte{legacyBGPPeerCommand: dataLegacyBGPPeers},
	}
	collr.apiClient = api

	require.NotNil(t, collectMap(t, collr))
	assert.True(t, collr.activePeerCharts["default_192_0_2_1"])

	api.responses[legacyBGPPeerCommand] = []byte(`<response status="success"><result></result></response>`)
	for i := 1; i < staleChartMaxMisses; i++ {
		mx, err := collr.collect()
		assert.Nil(t, mx)
		assert.NoError(t, err)
		assert.True(t, collr.activePeerCharts["default_192_0_2_1"])
	}

	mx, err := collr.collect()
	assert.Nil(t, mx)
	assert.NoError(t, err)
	assert.False(t, collr.activePeerCharts["default_192_0_2_1"])
}

func TestCollector_Collect_RemovesDynamicChartsAfterGraceMisses(t *testing.T) {
	collr := New()
	collr.CollectBGP = false
	collr.CollectSystem = false
	collr.CollectHA = false
	collr.CollectLicenses = false
	collr.CollectIPSec = false
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{environmentCommand: dataEnvironment},
	}

	require.NotNil(t, collectMap(t, collr))
	assert.True(t, collr.activeDynamicCharts["env_fan_fan_1_fan_1_rpm_speed"])

	collr.EnvironmentSensors = matcher.SimpleExpr{Includes: []string{"* temperature/*"}}
	require.NoError(t, collr.initEntitySelectors())

	for i := 1; i < staleChartMaxMisses; i++ {
		require.NotNil(t, collectMap(t, collr))
		assert.True(t, collr.activeDynamicCharts["env_fan_fan_1_fan_1_rpm_speed"])
	}

	require.NotNil(t, collectMap(t, collr))
	assert.False(t, collr.activeDynamicCharts["env_fan_fan_1_fan_1_rpm_speed"])
}

func TestCollector_Collect_ReadOnlyTelemetry(t *testing.T) {
	now := time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC)
	collr := New()
	collr.now = func() time.Time { return now }
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			systemInfoCommand:    dataSystemInfo,
			haStateCommand:       dataHAState,
			environmentCommand:   dataEnvironment,
			licenseInfoCommand:   dataLicenses,
			ipsecSACommand:       dataIPSecSA,
			legacyBGPPeerCommand: dataLegacyBGPPeers,
		},
	}

	mx := collectMap(t, collr)

	require.NotNil(t, mx)
	assert.Equal(t, int64(183845), mx["system_uptime"])
	assert.Equal(t, int64(1), mx["system_device_certificate_status_valid"])
	assert.Equal(t, int64(1), mx["system_operational_mode_normal"])

	assert.Equal(t, int64(1), mx["ha_enabled"])
	assert.Equal(t, int64(1), mx["ha_local_state_active"])
	assert.Equal(t, int64(1), mx["ha_peer_state_passive"])
	assert.Equal(t, int64(1), mx["ha_peer_connection_status_up"])
	assert.Equal(t, int64(1), mx["ha_state_sync_synchronized"])
	assert.Equal(t, int64(0), mx["ha_links_status_ha1_backup"])
	assert.Equal(t, int64(100), mx["ha_priority_local"])
	assert.Equal(t, int64(110), mx["ha_priority_peer"])

	assert.Equal(t, int64(40900), mx["env_temperature_temperature_1_temperature_inlet"])
	assert.Equal(t, int64(9157), mx["env_fan_fan_1_fan_1_rpm_speed"])
	assert.Equal(t, int64(3332), mx["env_voltage_voltage_1_3_3v_power_rail"])
	assert.Equal(t, int64(1), mx["env_sensor_voltage_1_3_3v_power_rail_alarm"])
	assert.Equal(t, int64(1), mx["env_psu_power_supply_1_power_supply_1_inserted"])

	assert.Equal(t, int64(3), mx["license_count_total"])
	assert.Equal(t, int64(1), mx["license_count_expired"])
	assert.Equal(t, int64(30), mx["license_threat_prevention_days_until_expiration"])
	assert.Equal(t, int64(1), mx["license_premium_support_status_expired"])
	assert.Equal(t, licenseNeverExpires, mx["license_globalprotect_portal_days_until_expiration"])

	assert.Equal(t, int64(2), mx["ipsec_tunnels_active"])
	assert.Equal(t, int64(1727), mx["ipsec_tunnel_branch_a_gw_branch_a_198_51_100_10_66_sa_lifetime"])
	assert.Equal(t, int64(99), mx["ipsec_tunnel_branch_b_gw_branch_b_203_0_113_20_67_sa_lifetime"])

	assert.Equal(t, int64(1), mx["bgp_peer_default_192_0_2_1_state_established"])
	assert.Equal(t, int64(4), mx["env_sensors_collection_discovered"])
	assert.Equal(t, int64(4), mx["env_sensors_collection_monitored"])
	assert.Equal(t, int64(3), mx["license_collection_discovered"])
	assert.Equal(t, int64(3), mx["license_collection_monitored"])
	assert.Equal(t, int64(2), mx["ipsec_tunnels_collection_discovered"])
	assert.Equal(t, int64(2), mx["ipsec_tunnels_collection_monitored"])
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_AppliesTelemetryCardinalityControls(t *testing.T) {
	collr := New()
	collr.CollectBGP = false
	collr.CollectSystem = false
	collr.CollectHA = false
	collr.MaxEnvironmentSensors = 1
	collr.MaxLicenses = 1
	collr.MaxIPSecTunnels = 1
	collr.EnvironmentSensors = matcher.SimpleExpr{Includes: []string{"* temperature/*"}}
	collr.Licenses = matcher.SimpleExpr{Includes: []string{"* Threat Prevention"}}
	require.NoError(t, collr.initEntitySelectors())
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			environmentCommand: dataEnvironment,
			licenseInfoCommand: dataLicenses,
			ipsecSACommand:     dataIPSecSA,
		},
	}

	mx := collectMap(t, collr)

	require.NotNil(t, mx)
	assert.Equal(t, int64(4), mx["env_sensors_collection_discovered"])
	assert.Equal(t, int64(1), mx["env_sensors_collection_monitored"])
	assert.Equal(t, int64(3), mx["env_sensors_collection_omitted_by_selector"])
	assert.Equal(t, int64(0), mx["env_sensors_collection_omitted_by_limit"])
	assert.Equal(t, int64(40900), mx["env_temperature_temperature_1_temperature_inlet"])
	assert.NotContains(t, mx, "env_fan_fan_1_fan_1_rpm_speed")
	assert.Equal(t, int64(3), mx["license_count_total"])
	assert.Equal(t, int64(1), mx["license_count_expired"])
	assert.Equal(t, int64(3), mx["license_collection_discovered"])
	assert.Equal(t, int64(1), mx["license_collection_monitored"])
	assert.Equal(t, int64(2), mx["license_collection_omitted_by_selector"])
	assert.Equal(t, int64(0), mx["license_collection_omitted_by_limit"])
	assert.Equal(t, int64(30), mx["license_threat_prevention_days_until_expiration"])
	assert.NotContains(t, mx, "license_premium_support_status_expired")
	assert.Equal(t, int64(2), mx["ipsec_tunnels_active"])
	assert.Equal(t, int64(2), mx["ipsec_tunnels_collection_discovered"])
	assert.Equal(t, int64(1), mx["ipsec_tunnels_collection_monitored"])
	assert.Equal(t, int64(0), mx["ipsec_tunnels_collection_omitted_by_selector"])
	assert.Equal(t, int64(1), mx["ipsec_tunnels_collection_omitted_by_limit"])
	assert.Contains(t, mx, "ipsec_tunnel_branch_a_gw_branch_a_198_51_100_10_66_sa_lifetime")
	assert.NotContains(t, mx, "ipsec_tunnel_branch_b_gw_branch_b_203_0_113_20_67_sa_lifetime")
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_ReportsUnrecognizedSystemResponse(t *testing.T) {
	collr := New()
	enableOnlySystem(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			systemInfoCommand: []byte(`<response status="success"><result></result></response>`),
		},
	}

	mx, err := collr.collect()

	require.Error(t, err)
	assert.Nil(t, mx)
	assert.Contains(t, err.Error(), "system metricset")
	assert.Contains(t, err.Error(), "expected <system>")
}

func TestCollector_Collect_ReportsMalformedTelemetryValues(t *testing.T) {
	collr := New()
	enableOnlySystem(collr)
	collr.CollectEnvironment = true
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			systemInfoCommand:  dataSystemInfo,
			environmentCommand: []byte(`<response status="success"><result><thermal><entry><slot>1</slot><description>Temperature Inlet</description><DegreesC>not-a-number</DegreesC><alarm>True</alarm></entry></thermal></result></response>`),
		},
	}

	mx, err := collr.collect()

	require.Error(t, err)
	require.NotNil(t, mx)
	assert.Equal(t, int64(183845), mx["system_uptime"])
	assert.Equal(t, int64(1), mx["env_sensor_temperature_1_temperature_inlet_alarm"])
	assert.NotContains(t, mx, "env_temperature_temperature_1_temperature_inlet")
	assert.Contains(t, err.Error(), `environment temperature Temperature Inlet: invalid decimal "not-a-number"`)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_ReportsMalformedLicenseExpirationWithoutFakeNeverValue(t *testing.T) {
	collr := New()
	enableOnlyLicenses(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			licenseInfoCommand: []byte(`<response status="success"><result><licenses><entry><feature>Threat Prevention</feature><description>Threat prevention updates</description><expires>tomorrow-ish</expires><expired>no</expired></entry></licenses></result></response>`),
		},
	}

	mx, err := collr.collect()

	require.Error(t, err)
	require.NotNil(t, mx)
	assert.Equal(t, int64(1), mx["license_count_total"])
	assert.Equal(t, int64(1), mx["license_threat_prevention_status_valid"])
	assert.NotContains(t, mx, "license_threat_prevention_days_until_expiration")
	assert.Contains(t, err.Error(), `license Threat Prevention expiration: invalid expiration date "tomorrow-ish"`)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_ReportsMalformedLicenseStatusWithoutFakeValidValue(t *testing.T) {
	collr := New()
	enableOnlyLicenses(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			licenseInfoCommand: []byte(`<response status="success"><result><licenses><entry><feature>Threat Prevention</feature><description>Threat prevention updates</description><expires>June 01, 2026</expires><expired>maybe</expired></entry></licenses></result></response>`),
		},
	}

	mx, err := collr.collect()

	require.Error(t, err)
	require.NotNil(t, mx)
	assert.Equal(t, int64(1), mx["license_count_total"])
	assert.NotContains(t, mx, "license_threat_prevention_status_valid")
	assert.NotContains(t, mx, "license_threat_prevention_status_expired")
	assert.Equal(t, int64(30), mx["license_threat_prevention_days_until_expiration"])
	assert.Contains(t, err.Error(), `license Threat Prevention expired status: invalid status "maybe"`)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_ReportsMalformedIPSecLifetime(t *testing.T) {
	collr := New()
	enableOnlyIPSec(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			ipsecSACommand: []byte(`<response status="success"><result><ntun>1</ntun><entries><entry><name>branch-a</name><gateway>gw-branch-a</gateway><remote>198.51.100.10</remote><remain>soon</remain><tid>66</tid></entry></entries></result></response>`),
		},
	}

	mx, err := collr.collect()

	require.Error(t, err)
	require.NotNil(t, mx)
	assert.Equal(t, int64(1), mx["ipsec_tunnels_active"])
	assert.NotContains(t, mx, "ipsec_tunnel_branch_a_gw_branch_a_198_51_100_10_66_sa_lifetime")
	assert.Contains(t, err.Error(), `IPsec tunnel branch-a remain: invalid integer "soon"`)
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestCollector_Collect_UsesIPSecNTunWhenEntriesAreAbsent(t *testing.T) {
	collr := New()
	enableOnlyIPSec(collr)
	collr.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			ipsecSACommand: []byte(`<response status="success"><result><ntun>2</ntun></result></response>`),
		},
	}

	mx, err := collr.collect()

	require.Error(t, err)
	require.NotNil(t, mx)
	assert.Equal(t, int64(2), mx["ipsec_tunnels_active"])
	assert.Equal(t, int64(0), mx["ipsec_tunnels_collection_discovered"])
	assert.Equal(t, int64(0), mx["ipsec_tunnels_collection_monitored"])
	assert.Contains(t, err.Error(), "IPsec active tunnel count mismatch: ntun=2 entries=0")
	collecttest.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
}

func TestPangoAPIClient_OpRefreshesAPIKeyOnceOnUnauthorized(t *testing.T) {
	operator := &mockPangoOperator{
		responses: []mockPangoResponse{
			{err: errors.New("code 16: Unauthorized")},
			{body: []byte("<response status=\"success\"/>")},
		},
	}
	client := &pangoAPIClient{client: operator, canRefresh: true}

	body, err := client.op("cmd")

	require.NoError(t, err)
	assert.Equal(t, []byte("<response status=\"success\"/>"), body)
	assert.Equal(t, 2, operator.opCalls)
	assert.Equal(t, 1, operator.refreshCalls)
}

func TestPangoAPIClient_OpRefreshesAPIKeyWhenInitializeFindsExpiredKey(t *testing.T) {
	operator := &mockPangoOperator{
		initializeErrs: []error{errors.New("code 16: Unauthorized"), nil},
		responses: []mockPangoResponse{
			{body: []byte("<response status=\"success\"/>")},
		},
	}
	client := &pangoAPIClient{client: operator, canRefresh: true}

	body, err := client.op("cmd")

	require.NoError(t, err)
	assert.Equal(t, []byte("<response status=\"success\"/>"), body)
	assert.Equal(t, 2, operator.initializeCalls)
	assert.Equal(t, 1, operator.opCalls)
	assert.Equal(t, 1, operator.refreshCalls)
}

func TestPangoAPIClient_OpResetsInitializationWhenRefreshFails(t *testing.T) {
	operator := &mockPangoOperator{
		responses: []mockPangoResponse{
			{err: errors.New("code 16: Unauthorized")},
		},
		refreshErr: errors.New("refresh failed with key=secret"),
	}
	client := &pangoAPIClient{client: operator, canRefresh: true, initialized: true}

	body, err := client.op("cmd")

	require.Error(t, err)
	assert.Nil(t, body)
	assert.False(t, client.initialized)
	assert.NotContains(t, err.Error(), "secret")
	assert.Contains(t, err.Error(), "key=<redacted>")
}

func TestPangoAPIClient_OpSanitizesPasswordInErrors(t *testing.T) {
	err := sanitizePANOSAPIError(errors.New("https://fw.example.invalid/api/?type=keygen&user=netdata&password=secret"))

	require.Error(t, err)
	assert.NotContains(t, err.Error(), "secret")
	assert.Contains(t, err.Error(), "password=<redacted>")
}

func TestParseBGPPeers(t *testing.T) {
	tests := map[string]struct {
		data     []byte
		wantLen  int
		validate func(*testing.T, []bgpPeer)
	}{
		"legacy": {
			data:    dataLegacyBGPPeers,
			wantLen: 2,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "default", peers[0].VR)
				assert.Equal(t, "192.0.2.1", peers[0].PeerAddress)
				assert.Equal(t, "192.0.2.254", peers[0].LocalAddress)
				assert.Equal(t, "edge", peers[0].PeerGroup)
				assert.Equal(t, "65001", peers[0].RemoteAS)
				assert.Equal(t, "established", peers[0].State)
				assert.Equal(t, "ipv4", peers[0].PrefixCounters[0].AFI)
				assert.Equal(t, "unicast", peers[0].PrefixCounters[0].SAFI)
				assert.Equal(t, "198.51.100.1", peers[1].PeerAddress)
				assert.Equal(t, "active", peers[1].State)
			},
		},
		"advanced": {
			data:    dataAdvancedBGPPeers,
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "lr-a", peers[0].VR)
				assert.Equal(t, "203.0.113.1", peers[0].PeerAddress)
				assert.Equal(t, "openconfirm", peers[0].State)
				assert.Equal(t, int64(93784), peers[0].Uptime)
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			peers, err := parseBGPPeers(test.data)
			require.NoError(t, err)
			require.Len(t, peers, test.wantLen)
			test.validate(t, peers)
		})
	}
}

func TestParseBGPPeers_ErrorResponseWithNestedLines(t *testing.T) {
	data := []byte(`<response status="error" code="16"><msg><line>Unauthorized</line><line>Invalid API key</line></msg></response>`)

	peers, err := parseBGPPeers(data)

	require.Error(t, err)
	assert.Nil(t, peers)
	assert.Contains(t, err.Error(), "code 16")
	assert.Contains(t, err.Error(), "Unauthorized; Invalid API key")
}

func TestParseBGPPeers_ErrorResponseWithResultMessage(t *testing.T) {
	data := []byte(`<response status="error" code="400"><result><msg>Parameter &quot;format&quot; is required while exporting certificate</msg></result></response>`)

	peers, err := parseBGPPeers(data)

	require.Error(t, err)
	assert.Nil(t, peers)
	assert.Contains(t, err.Error(), "code 400")
	assert.Contains(t, err.Error(), "Bad request")
	assert.Contains(t, err.Error(), `Parameter "format" is required while exporting certificate`)
}

func TestParseBGPPeers_MalformedNumericFieldFails(t *testing.T) {
	data := []byte(`<response status="success"><result><entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>abc</msg-total-in><msg-total-out>1</msg-total-out><msg-update-in>1</msg-update-in><msg-update-out>1</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry></result></response>`)

	peers, err := parseBGPPeers(data)

	require.Error(t, err)
	assert.Nil(t, peers)
	assert.Contains(t, err.Error(), `BGP peer 192.0.2.1 msg-total-in: invalid integer "abc"`)
}

func TestParseBGPPeers_MissingNumericFieldFails(t *testing.T) {
	data := []byte(`<response status="success"><result><entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>1</msg-total-in><msg-total-out>1</msg-total-out><msg-update-in>1</msg-update-in><msg-update-out>1</msg-update-out><status-flap-counts>0</status-flap-counts></entry></result></response>`)

	peers, err := parseBGPPeers(data)

	require.Error(t, err)
	assert.Nil(t, peers)
	assert.Contains(t, err.Error(), "BGP peer 192.0.2.1 established-counts: missing integer")
}

func TestParseReadOnlyTelemetry(t *testing.T) {
	system, err := parseSystemInfo(dataSystemInfo)
	require.NoError(t, err)
	assert.Equal(t, "edge-fw-a", system.Hostname)
	assert.Equal(t, "11.1.2", system.SWVersion)

	ha, err := parseHAState(dataHAState)
	require.NoError(t, err)
	assert.Equal(t, "yes", ha.Enabled)
	assert.Equal(t, "active", normalizeHAState(ha.Group.LocalInfo.State))
	assert.Equal(t, "passive", normalizeHAState(ha.Group.PeerInfo.State))

	env, err := parseEnvironment(dataEnvironment)
	require.NoError(t, err)
	require.Len(t, env.ThermalEntries, 1)
	require.Len(t, env.FanEntries, 1)
	require.Len(t, env.VoltageEntries, 1)
	require.Len(t, env.PowerSupplyEntries, 1)
	assert.Equal(t, "Temperature Inlet", env.ThermalEntries[0].Description)
	assert.Equal(t, "3.332", env.VoltageEntries[0].Volts)

	licenses, found, err := parseLicenses(dataLicenses)
	require.NoError(t, err)
	assert.True(t, found)
	require.Len(t, licenses, 3)
	assert.Equal(t, "Threat Prevention", licenses[0].Feature)

	tunnels, activeCount, found, err := parseIPSecTunnels(dataIPSecSA)
	require.NoError(t, err)
	assert.True(t, found)
	assert.Equal(t, int64(2), activeCount)
	require.Len(t, tunnels, 2)
	assert.Equal(t, "branch-a", tunnels[0].Name)
}

func TestParserHelpers(t *testing.T) {
	t.Run("normalize BGP state", func(t *testing.T) {
		tests := map[string]string{
			"Established":   "established",
			"OpenConfirm":   "openconfirm",
			"Open-Sent":     "opensent",
			"Active":        "active",
			"Connect":       "connect",
			"Idle":          "idle",
			"unknown-state": "",
		}
		for in, want := range tests {
			assert.Equal(t, want, normalizeBGPState(in), in)
		}
	})

	t.Run("parse PAN-OS duration", func(t *testing.T) {
		tests := map[string]int64{
			"3600":              3600,
			"01:00:00":          3600,
			"1 days 02:03:04":   93784,
			"30m":               1800,
			"2 mins":            120,
			"5 secs":            5,
			"2 hours 5 seconds": 7205,
			"":                  0,
		}
		for in, want := range tests {
			got, err := parsePANOSDurationField("duration", in)
			require.NoError(t, err, in)
			assert.Equal(t, want, got, in)
		}
	})

	t.Run("strict parsers report malformed values", func(t *testing.T) {
		_, err := parsePANOSIntField("test integer", "not-an-int")
		assert.EqualError(t, err, `test integer: invalid integer "not-an-int"`)

		_, err = parseRequiredPANOSIntField("test integer", "")
		assert.EqualError(t, err, "test integer: missing integer")

		_, err = parsePANOSDecimalField("test decimal", "not-a-decimal", 1000)
		assert.EqualError(t, err, `test decimal: invalid decimal "not-a-decimal"`)

		_, err = parseRequiredPANOSDecimalField("test decimal", "", 1000)
		assert.EqualError(t, err, "test decimal: missing decimal")

		_, err = parsePANOSDurationField("test duration", "since reboot")
		assert.EqualError(t, err, `test duration: invalid duration "since reboot"`)

		_, err = parseRequiredPANOSDurationField("test duration", "")
		assert.EqualError(t, err, "test duration: missing duration")

		_, err = parsePANOSDurationField("test duration", "01:99:00")
		assert.EqualError(t, err, `test duration: invalid duration "01:99:00"`)
	})

	t.Run("normalize address", func(t *testing.T) {
		tests := map[string]string{
			"192.0.2.1:179":       "192.0.2.1",
			"192.0.2.1":           "192.0.2.1",
			"[2001:db8::1]:179":   "2001:db8::1",
			"2001:db8::1":         "2001:db8::1",
			"example.invalid:179": "example.invalid",
		}
		for in, want := range tests {
			assert.Equal(t, want, normalizeAddress(in), in)
		}
	})

	t.Run("normalize AFI SAFI", func(t *testing.T) {
		afi, safi := normalizeAFISAFI("bgpAfiIpv4-unicast")
		assert.Equal(t, "ipv4", afi)
		assert.Equal(t, "unicast", safi)

		afi, safi = normalizeAFISAFI("ipv6-unicast")
		assert.Equal(t, "ipv6", afi)
		assert.Equal(t, "unicast", safi)
	})
}

func TestParseAPIURL(t *testing.T) {
	tests := map[string]struct {
		raw      string
		want     panosAPIURL
		wantFail bool
	}{
		"https host": {
			raw: "https://192.0.2.1",
			want: panosAPIURL{
				protocol: "https",
				hostname: "192.0.2.1",
			},
		},
		"http port api path": {
			raw: "http://fw.example.invalid:8443/api",
			want: panosAPIURL{
				protocol: "http",
				hostname: "fw.example.invalid",
				port:     8443,
			},
		},
		"ipv6": {
			raw: "https://[2001:db8::1]/",
			want: panosAPIURL{
				protocol: "https",
				hostname: "[2001:db8::1]",
			},
		},
		"bad scheme": {
			raw:      "ftp://192.0.2.1",
			wantFail: true,
		},
		"embedded credentials": {
			raw:      "https://user:pass@192.0.2.1",
			wantFail: true,
		},
		"bad path": {
			raw:      "https://192.0.2.1/other",
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			got, err := parseAPIURL(test.raw)
			if test.wantFail {
				assert.Error(t, err)
			} else {
				require.NoError(t, err)
				assert.Equal(t, test.want, got)
			}
		})
	}
}

func collectMap(t *testing.T, collr *Collector) map[string]int64 {
	t.Helper()

	mx, err := collr.collect()
	require.NoError(t, err)
	require.NotNil(t, mx)
	return mx
}

func collectV2Reader(t *testing.T, collr *Collector) metrix.Reader {
	t.Helper()

	managed, ok := metrix.AsCycleManagedStore(collr.MetricStore())
	require.True(t, ok)

	cc := managed.CycleController()
	committed := false
	cc.BeginCycle()
	defer func() {
		if !committed {
			cc.AbortCycle()
		}
	}()

	require.NoError(t, collr.Collect(context.Background()))
	cc.CommitCycleSuccess()
	committed = true

	return collr.MetricStore().Read(metrix.ReadRaw())
}

type mockAPIClient struct {
	responses map[string][]byte
	errors    map[string]error
	commands  []string
	info      map[string]string
}

func (m *mockAPIClient) op(cmd string) ([]byte, error) {
	m.commands = append(m.commands, cmd)
	if err := m.errors[cmd]; err != nil {
		return nil, err
	}
	if resp := m.responses[cmd]; resp != nil {
		return resp, nil
	}
	return []byte(`<response status="success"><result></result></response>`), nil
}

func (m *mockAPIClient) closeIdleConnections() {}

func (m *mockAPIClient) systemInfo() map[string]string { return m.info }

type mockPangoResponse struct {
	body []byte
	err  error
}

type mockPangoOperator struct {
	initializeErr   error
	initializeErrs  []error
	refreshErr      error
	initializeCalls int
	responses       []mockPangoResponse
	info            map[string]string
	opCalls         int
	refreshCalls    int
}

func (m *mockPangoOperator) Initialize() error {
	if len(m.initializeErrs) > 0 {
		err := m.initializeErrs[min(m.initializeCalls, len(m.initializeErrs)-1)]
		m.initializeCalls++
		return err
	}
	m.initializeCalls++
	return m.initializeErr
}

func (m *mockPangoOperator) Op(any, string, any, any) ([]byte, error) {
	resp := m.responses[m.opCalls]
	m.opCalls++
	return resp.body, resp.err
}

func (m *mockPangoOperator) RetrieveApiKey() error {
	m.refreshCalls++
	return m.refreshErr
}

func (m *mockPangoOperator) SystemInfo() map[string]string { return m.info }

func enableOnlyBGP(c *Collector) {
	c.CollectSystem = false
	c.CollectHA = false
	c.CollectEnvironment = false
	c.CollectLicenses = false
	c.CollectIPSec = false
}

func enableOnlySystem(c *Collector) {
	c.CollectBGP = false
	c.CollectSystem = true
	c.CollectHA = false
	c.CollectEnvironment = false
	c.CollectLicenses = false
	c.CollectIPSec = false
}

func enableOnlyLicenses(c *Collector) {
	c.CollectBGP = false
	c.CollectSystem = false
	c.CollectHA = false
	c.CollectEnvironment = false
	c.CollectLicenses = true
	c.CollectIPSec = false
}

func enableOnlyIPSec(c *Collector) {
	c.CollectBGP = false
	c.CollectSystem = false
	c.CollectHA = false
	c.CollectEnvironment = false
	c.CollectLicenses = false
	c.CollectIPSec = true
}
