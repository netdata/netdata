// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"bytes"
	"context"
	"errors"
	"maps"
	"os"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
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
		setup       func(*Collector)
		keepFactory bool
		wantErr     string
		check       func(*testing.T, *Collector)
	}{
		"success with API key": {
			setup: func(c *Collector) {
				c.APIKey = "key"
			},
			check: func(t *testing.T, c *Collector) {
				assert.NotNil(t, c.apiClient)
			},
		},
		"success with username and password": {
			setup: func(c *Collector) {
				c.Username = "user"
				c.Password = "pass"
			},
			check: func(t *testing.T, c *Collector) {
				assert.NotNil(t, c.apiClient)
			},
		},
		"api client factory error": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.newAPIClient = func(Config) (panosAPIClient, error) {
					return nil, errors.New("factory failed")
				}
			},
			keepFactory: true,
			wantErr:     "init PAN-OS API client: factory failed",
		},
		"URL not set": {
			setup: func(c *Collector) {
				c.URL = ""
				c.APIKey = "key"
			},
			wantErr: "url not configured",
		},
		"auth not set": {
			wantErr: "api_key or username/password",
		},
		"force_http2 is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.ForceHTTP2 = true
			},
			wantErr: "force_http2",
		},
		"request body is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.Body = "body"
			},
			wantErr: "body",
		},
		"bearer token file is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.BearerTokenFile = "/tmp/token"
			},
			wantErr: "bearer_token_file",
		},
		"request method is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.Method = "POST"
			},
			wantErr: "method",
		},
		"not following redirects is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.NotFollowRedirect = true
			},
			wantErr: "not_follow_redirects",
		},
		"proxy username is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.ProxyUsername = "proxy-user"
			},
			wantErr: "proxy_username/proxy_password",
		},
		"proxy password is not supported": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.ProxyPassword = "proxy-pass"
			},
			wantErr: "proxy_username/proxy_password",
		},
		"tls cert without key is rejected": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.TLSCert = "/tmp/client.pem"
			},
			wantErr: "tls_cert and tls_key",
		},
		"tls key without cert is rejected": {
			setup: func(c *Collector) {
				c.APIKey = "key"
				c.TLSKey = "/tmp/client-key.pem"
			},
			wantErr: "tls_cert and tls_key",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			if tc.setup != nil {
				tc.setup(collr)
			}
			if !tc.keepFactory {
				collr.newAPIClient = func(Config) (panosAPIClient, error) {
					return &mockAPIClient{}, nil
				}
			}

			err := collr.Init(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				return
			}
			require.NoError(t, err)
			if tc.check != nil {
				tc.check(t, collr)
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		client       panosAPIClient
		wantErr      string
		wantCommands []string
	}{
		"success probes system info only": {
			client: &mockAPIClient{
				responses: map[string][]byte{
					systemInfoCommand:    dataSystemInfo,
					haStateCommand:       []byte(`<response status="success"><result></result></response>`),
					environmentCommand:   []byte(`<response status="success"><result></result></response>`),
					licenseInfoCommand:   []byte(`<response status="success"><result></result></response>`),
					ipsecSACommand:       []byte(`<response status="success"><result></result></response>`),
					legacyBGPPeerCommand: dataLegacyBGPPeers,
				},
			},
			wantCommands: []string{systemInfoCommand},
		},
		"malformed optional metricsets do not fail check": {
			client: &mockAPIClient{
				responses: map[string][]byte{
					systemInfoCommand:  dataSystemInfo,
					haStateCommand:     []byte(`<response status="success"><result></result></response>`),
					environmentCommand: []byte(`<response status="success"><result></result></response>`),
					licenseInfoCommand: []byte(`<response status="success"><result></result></response>`),
					ipsecSACommand:     []byte(`<response status="success"><result></result></response>`),
				},
			},
			wantCommands: []string{systemInfoCommand},
		},
		"fails when system info API call fails": {
			client: &mockAPIClient{
				errors: map[string]error{systemInfoCommand: errors.New("api error")},
			},
			wantErr:      "api error",
			wantCommands: []string{systemInfoCommand},
		},
		"fails when system info payload is missing": {
			client: &mockAPIClient{
				responses: map[string][]byte{systemInfoCommand: []byte(`<response status="success"><result></result></response>`)},
			},
			wantErr:      "expected <system>",
			wantCommands: []string{systemInfoCommand},
		},
		"fails when API client is not initialized": {
			wantErr: "API client not initialized",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.apiClient = tc.client
			api, _ := tc.client.(*mockAPIClient)

			err := collr.Check(context.Background())
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
				if tc.wantCommands != nil {
					require.NotNil(t, api)
					assert.Equal(t, tc.wantCommands, api.commands)
				}
				return
			}
			require.NoError(t, err)
			require.NotNil(t, api)
			assert.Equal(t, tc.wantCommands, api.commands)
		})
	}
}

func TestCollector_CheckStopsOnCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	api := &mockAPIClient{}
	collr := New()
	collr.apiClient = api

	err := collr.Check(ctx)
	require.ErrorIs(t, err, context.Canceled)
	assert.Empty(t, api.commands)
}

func TestCollector_Cleanup(t *testing.T) {
	tests := map[string]struct {
		client *mockAPIClient
		want   int
	}{
		"client not initialized": {},
		"client initialized": {
			client: &mockAPIClient{},
			want:   1,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			if tc.client != nil {
				collr.apiClient = tc.client
			}

			assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
			if tc.client != nil {
				assert.Equal(t, tc.want, tc.client.closeCalls)
			}
		})
	}
}

func TestCollector_CollectStopsOnCanceledContext(t *testing.T) {
	tests := map[string]struct {
		cancelBeforeCollect bool
		cancelAfterCommand  string
		wantCommands        []string
	}{
		"canceled before first API call": {
			cancelBeforeCollect: true,
		},
		"canceled after system metricset": {
			cancelAfterCommand: systemInfoCommand,
			wantCommands:       []string{systemInfoCommand},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			if tc.cancelBeforeCollect {
				cancel()
			}

			api := &mockAPIClient{}
			api.onOp = func(_ context.Context, cmd string) {
				if cmd == tc.cancelAfterCommand {
					cancel()
				}
			}
			collr := New()
			collr.apiClient = api

			err := collectOnceWithContext(t, collr, ctx)
			require.ErrorIs(t, err, context.Canceled)
			assert.Equal(t, tc.wantCommands, api.commands)
		})
	}
}

func TestCollector_MetricStore(t *testing.T) {
	assert.NotNil(t, New().MetricStore())
}

func TestCollector_ChartTemplateYAML(t *testing.T) {
	collr := New()

	collecttest.AssertChartTemplateSchema(t, collr.ChartTemplateYAML())
	spec, err := charttpl.DecodeYAML([]byte(collr.ChartTemplateYAML()))
	require.NoError(t, err)
	_, err = chartengine.Compile(spec, 1)
	require.NoError(t, err)
}

func TestCollector_Collect(t *testing.T) {
	type collectStep struct {
		name        string
		setup       func(*Collector, *mockAPIClient)
		wantErr     string
		wantMetrics map[string]metrix.SampleValue
		wantMissing []string
		wantLog     []string
		notWantLog  []string
		check       func(*testing.T, *Collector, *mockAPIClient, map[string]metrix.SampleValue)
	}
	tests := map[string]struct {
		prepare func(*Collector, *mockAPIClient)
		steps   []collectStep
	}{
		"read-only telemetry and legacy BGP": {
			prepare: func(c *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					systemInfoCommand:    dataSystemInfo,
					haStateCommand:       dataHAState,
					environmentCommand:   dataEnvironment,
					licenseInfoCommand:   dataLicenses,
					ipsecSACommand:       dataIPSecSA,
					legacyBGPPeerCommand: dataLegacyBGPPeers,
				}
				c.now = func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) }
			},
			steps: []collectStep{
				{
					name: "collects all read-only metricsets",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()):                                                                              183845,
						stateMetricKey("system_device_certificate_status", "valid", systemLabels()):                                             1,
						stateMetricKey("system_operational_mode", "normal", systemLabels()):                                                     1,
						stateMetricKey("ha_status", "enabled", nil):                                                                             1,
						stateMetricKey("ha_status", "disabled", nil):                                                                            0,
						stateMetricKey("ha_local_state", "active", nil):                                                                         1,
						stateMetricKey("ha_peer_state", "passive", nil):                                                                         1,
						stateMetricKey("ha_peer_connection_status", "up", nil):                                                                  1,
						stateMetricKey("ha_peer_connection_status", "down", nil):                                                                0,
						stateMetricKey("ha_peer_connection_status", "unknown", nil):                                                             0,
						stateMetricKey("ha_state_sync_status", "synchronized", nil):                                                             1,
						stateMetricKey("ha_state_sync_status", "not_synchronized", nil):                                                         0,
						stateMetricKey("ha_state_sync_status", "unknown", nil):                                                                  0,
						stateMetricKey("ha_link_status", "up", haLinkLabels("ha1")):                                                             1,
						stateMetricKey("ha_link_status", "down", haLinkLabels("ha1")):                                                           0,
						stateMetricKey("ha_link_status", "unknown", haLinkLabels("ha1")):                                                        0,
						stateMetricKey("ha_link_status", "up", haLinkLabels("ha1_backup")):                                                      0,
						stateMetricKey("ha_link_status", "down", haLinkLabels("ha1_backup")):                                                    1,
						stateMetricKey("ha_link_status", "unknown", haLinkLabels("ha1_backup")):                                                 0,
						metricKey("environment_temperature", envLabels("temperature", "1", "Temperature Inlet")):                                40900,
						metricKey("environment_fan_speed", envLabels("fan", "1", "Fan 1 RPM")):                                                  9157,
						metricKey("environment_voltage", envLabels("voltage", "1", "3.3V Power Rail")):                                          3332,
						stateMetricKey("environment_sensor_alarm_status", "alarm", envLabels("voltage", "1", "3.3V Power Rail")):                1,
						stateMetricKey("environment_sensor_alarm_status", "clear", envLabels("voltage", "1", "3.3V Power Rail")):                0,
						stateMetricKey("environment_power_supply_presence_status", "present", envLabels("power_supply", "1", "Power Supply 1")): 1,
						stateMetricKey("environment_power_supply_presence_status", "absent", envLabels("power_supply", "1", "Power Supply 1")):  0,
						stateMetricKey("environment_power_supply_alarm_status", "clear", envLabels("power_supply", "1", "Power Supply 1")):      1,
						stateMetricKey("environment_power_supply_alarm_status", "alarm", envLabels("power_supply", "1", "Power Supply 1")):      0,
						metricKey("license_count_total", nil):                                                                                   3,
						metricKey("license_count_expired", nil):                                                                                 1,
						metricKey("license_time_until_expiration", licenseLabels("Threat Prevention", "Threat prevention updates")):             30,
						stateMetricKey("license_status", "expired", licenseLabels("Premium Support", "Support entitlement")):                    1,
						metricKey("license_time_until_expiration", licenseLabels("GlobalProtect Portal", "Portal entitlement")):                 metrix.SampleValue(licenseNeverExpires),
						metricKey("ipsec_tunnels_active", nil):                                                                                  2,
						metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-a", "gw-branch-a", "198.51.100.10", "66", "ESP", "G256")):     1727,
						metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-b", "gw-branch-b", "203.0.113.20", "67", "ESP", "AES128")):    99,
						stateMetricKey("bgp_peer_state", "established", legacyPeerLabels()):                                                     1,
					},
					wantMissing: []string{
						metricKey("license_time_until_expiration", licenseLabels("Premium Support", "Support entitlement")),
						"env_sensors_collection_discovered",
						"license_collection_discovered",
						"ipsec_tunnels_collection_discovered",
					},
					check: func(t *testing.T, c *Collector, _ *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineLegacy, c.routingEngine)
						collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
					},
				},
			},
		},
		"advanced BGP fallback": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand:       []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[0]: dataAdvancedBGPPeers,
					advancedBGPPeerCommands[1]: []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[2]: []byte(`<response status="success"><result></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "collects ARE peers after legacy empty success",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "openconfirm", advancedPeerLabels()):                  1,
						metricKey("bgp_peer_uptime", advancedPeerLabels()):                                     93784,
						metricKey("bgp_peer_prefixes_received_total", advancedPrefixLabels("ipv4", "unicast")): 100,
						metricKey("bgp_vr_peers_total_configured", metrix.Labels{"vr": "lr-a"}):                1,
					},
					check: func(t *testing.T, c *Collector, _ *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineAdvanced, c.routingEngine)
						assert.Equal(t, advancedBGPPeerCommands[0], c.bgpCommand)
					},
				},
			},
		},
		"no BGP state is cached": {
			prepare: func(c *Collector, _ *mockAPIClient) {
				now := time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC)
				c.now = func() time.Time { return now }
			},
			steps: []collectStep{
				{
					name: "initial full BGP probe",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					check: func(t *testing.T, c *Collector, api *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineNone, c.routingEngine)
						assert.Len(t, api.commands, 9)
					},
				},
				{
					name: "cached no-BGP skips BGP commands",
					setup: func(_ *Collector, api *mockAPIClient) {
						api.commands = nil
					},
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					check: func(t *testing.T, _ *Collector, api *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Len(t, api.commands, 5)
					},
				},
				{
					name: "reprobes after no-BGP interval",
					setup: func(c *Collector, api *mockAPIClient) {
						api.commands = nil
						c.now = func() time.Time {
							return time.Date(2026, 5, 2, 0, 0, 0, 0, time.UTC).Add(noBGPReprobeInterval)
						}
					},
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					check: func(t *testing.T, _ *Collector, api *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Len(t, api.commands, 9)
					},
				},
			},
		},
		"BGP probe errors with empty success do not cache no-BGP": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand:       []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[1]: []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[2]: []byte(`<response status="success"><result></result></response>`),
				}
				api.errors = map[string]error{
					advancedBGPPeerCommands[0]: errors.New("advanced routing query failed"),
				}
			},
			steps: []collectStep{
				{
					name: "first partial BGP probe failure",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					wantLog: []string{"advanced routing query failed"},
					check:   assertBGPProbeErrorNotCached,
				},
				{
					name: "second cycle probes again",
					setup: func(_ *Collector, api *mockAPIClient) {
						api.commands = nil
					},
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					notWantLog: []string{"advanced routing query failed"},
					check:      assertBGPProbeErrorNotCached,
				},
			},
		},
		"stale cached BGP command reprobes": {
			prepare: func(c *Collector, api *mockAPIClient) {
				c.routingEngine = routingEngineLegacy
				c.bgpCommand = legacyBGPPeerCommand
				api.responses = map[string][]byte{
					legacyBGPPeerCommand:       []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[0]: dataAdvancedBGPPeers,
				}
			},
			steps: []collectStep{
				{
					name: "empty cached legacy command tries ARE commands",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "openconfirm", advancedPeerLabels()): 1,
					},
					check: func(t *testing.T, c *Collector, api *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineAdvanced, c.routingEngine)
						assert.Equal(t, advancedBGPPeerCommands[0], c.bgpCommand)
						assert.Equal(t, []string{
							systemInfoCommand,
							haStateCommand,
							environmentCommand,
							licenseInfoCommand,
							ipsecSACommand,
							legacyBGPPeerCommand,
							advancedBGPPeerCommands[0],
						}, api.commands)
					},
				},
			},
		},
		"stale BGP labels are dropped between cycles": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{legacyBGPPeerCommand: dataLegacyBGPPeers}
			},
			steps: []collectStep{
				{
					name: "old remote AS",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "established", legacyPeerLabels()): 1,
					},
				},
				{
					name: "new remote AS replaces old label set",
					setup: func(_ *Collector, api *mockAPIClient) {
						api.responses[legacyBGPPeerCommand] = []byte(strings.Replace(string(dataLegacyBGPPeers), "<remote-as>65001</remote-as>", "<remote-as>65111</remote-as>", 1))
					},
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "established", legacyPeerLabelsWithRemoteAS("65111")): 1,
					},
					wantMissing: []string{
						stateMetricKey("bgp_peer_state", "established", legacyPeerLabels()),
					},
				},
			},
		},
		"malformed BGP peer preserves valid peers": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand: []byte(`<response status="success"><result>
						<entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>abc</msg-total-in><msg-total-out>1</msg-total-out><msg-update-in>1</msg-update-in><msg-update-out>1</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
						<entry><peer-address>192.0.2.2</peer-address><status>Established</status><status-duration>120</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
					</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "valid peer still emitted",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "established", fallbackPeerLabels("192.0.2.2")): 1,
					},
					wantMissing: []string{
						stateMetricKey("bgp_peer_state", "established", fallbackPeerLabels("192.0.2.1")),
					},
					wantLog: []string{`BGP peer entry 192.0.2.1: BGP peer 192.0.2.1 msg-total-in: invalid integer`},
				},
			},
		},
		"malformed BGP prefix preserves peer": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand: []byte(`<response status="success"><result>
						<entry>
							<peer-address>192.0.2.1</peer-address>
							<status>Established</status>
							<status-duration>60</status-duration>
							<msg-total-in>10</msg-total-in>
							<msg-total-out>20</msg-total-out>
							<msg-update-in>3</msg-update-in>
							<msg-update-out>4</msg-update-out>
							<status-flap-counts>0</status-flap-counts>
							<established-counts>1</established-counts>
							<prefix-counter>
								<entry name="ipv4-unicast"><incoming-total>abc</incoming-total><incoming-accepted>1</incoming-accepted><incoming-rejected>0</incoming-rejected><outgoing-advertised>2</outgoing-advertised></entry>
							</prefix-counter>
						</entry>
					</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "peer metrics survive malformed prefix counter",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "established", fallbackPeerLabels("192.0.2.1")): 1,
						metricKey("bgp_vr_peers_total_configured", metrix.Labels{"vr": "default"}):       1,
					},
					wantMissing: []string{
						metricKey("bgp_peer_prefixes_received_total", fallbackPrefixLabels("192.0.2.1", "ipv4", "unicast")),
					},
					wantLog: []string{`BGP peer entry 192.0.2.1: BGP peer 192.0.2.1 ipv4-unicast incoming-total: invalid integer`},
				},
			},
		},
		"advanced BGP second command fallback": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand:       []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[0]: []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[1]: dataAdvancedBGPPeers,
				}
			},
			steps: []collectStep{
				{
					name: "collects ARE peers from second supported command",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "openconfirm", advancedPeerLabels()): 1,
					},
					check: func(t *testing.T, c *Collector, _ *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineAdvanced, c.routingEngine)
						assert.Equal(t, advancedBGPPeerCommands[1], c.bgpCommand)
					},
				},
			},
		},
		"advanced BGP third command fallback": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand:       []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[0]: []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[1]: []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[2]: dataAdvancedBGPPeers,
				}
			},
			steps: []collectStep{
				{
					name: "collects ARE peers from third supported command",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "openconfirm", advancedPeerLabels()): 1,
					},
					check: func(t *testing.T, c *Collector, _ *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineAdvanced, c.routingEngine)
						assert.Equal(t, advancedBGPPeerCommands[2], c.bgpCommand)
					},
				},
			},
		},
		"cached BGP command failure reprobes alternate commands": {
			prepare: func(c *Collector, api *mockAPIClient) {
				c.routingEngine = routingEngineLegacy
				c.bgpCommand = legacyBGPPeerCommand
				api.errors = map[string]error{legacyBGPPeerCommand: errors.New("legacy BGP query failed")}
				api.responses = map[string][]byte{
					advancedBGPPeerCommands[0]: []byte(`<response status="success"><result></result></response>`),
					advancedBGPPeerCommands[1]: dataAdvancedBGPPeers,
				}
			},
			steps: []collectStep{
				{
					name: "cached command error does not prevent ARE fallback",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "openconfirm", advancedPeerLabels()): 1,
					},
					wantLog: []string{"legacy BGP query failed"},
					check: func(t *testing.T, c *Collector, api *mockAPIClient, _ map[string]metrix.SampleValue) {
						assert.Equal(t, routingEngineAdvanced, c.routingEngine)
						assert.Equal(t, advancedBGPPeerCommands[1], c.bgpCommand)
						assert.Equal(t, []string{
							systemInfoCommand,
							haStateCommand,
							environmentCommand,
							licenseInfoCommand,
							ipsecSACommand,
							legacyBGPPeerCommand,
							legacyBGPPeerCommand,
							advancedBGPPeerCommands[0],
							advancedBGPPeerCommands[1],
						}, api.commands)
					},
				},
			},
		},
		"unknown BGP state": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					legacyBGPPeerCommand: []byte(`<response status="success"><result>
						<entry><peer-address>192.0.2.1</peer-address><status>Clearing</status><status-duration>60</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
					</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "unrecognized non-empty state maps to unknown",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("bgp_peer_state", "unknown", fallbackPeerLabels("192.0.2.1")):     1,
						stateMetricKey("bgp_peer_state", "established", fallbackPeerLabels("192.0.2.1")): 0,
						metricKey("bgp_vr_peers_by_state_unknown", metrix.Labels{"vr": "default"}):       1,
						metricKey("bgp_vr_peers_total_configured", metrix.Labels{"vr": "default"}):       1,
						metricKey("bgp_vr_peers_total_established", metrix.Labels{"vr": "default"}):      0,
					},
				},
			},
		},
		"missing optional label values use fallbacks": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				systemInfo := strings.Replace(string(dataSystemInfo), "      <sw-version>11.1.2</sw-version>\n", "", 1)
				bgpPeers := strings.Replace(string(dataLegacyBGPPeers), "      <peer-group>edge</peer-group>\n", "", 1)
				bgpPeers = strings.Replace(bgpPeers, "      <remote-as>65001</remote-as>\n", "", 1)
				api.responses = map[string][]byte{
					systemInfoCommand:    []byte(systemInfo),
					licenseInfoCommand:   []byte(`<response status="success"><result><licenses><entry><feature>Threat Prevention</feature><expires>June 01, 2026</expires><expired>no</expired></entry></licenses></result></response>`),
					legacyBGPPeerCommand: []byte(bgpPeers),
				}
			},
			steps: []collectStep{
				{
					name: "fallback label values are explicit",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", metrix.Labels{"hostname": "edge-fw-a", "model": "PA-850", "serial": "0123456789", "sw_version": "unknown"}):                                                                 183845,
						stateMetricKey("license_status", "valid", licenseLabels("Threat Prevention", "unknown")):                                                                                                               1,
						stateMetricKey("bgp_peer_state", "established", metrix.Labels{"vr": "default", "peer_address": "192.0.2.1", "local_address": "192.0.2.254", "remote_as": "unknown_as", "peer_group": "unknown_group"}): 1,
					},
				},
			},
		},
		"system abnormal status states": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				systemInfo := strings.Replace(string(dataSystemInfo), "<device-certificate-status>Valid</device-certificate-status>", "<device-certificate-status>invalid</device-certificate-status>", 1)
				systemInfo = strings.Replace(systemInfo, "<operational-mode>normal</operational-mode>", "<operational-mode>maintenance</operational-mode>", 1)
				api.responses = map[string][]byte{systemInfoCommand: []byte(systemInfo)}
			},
			steps: []collectStep{
				{
					name: "invalid certificate and non-normal mode are explicit states",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("system_device_certificate_status", "valid", systemLabels()):   0,
						stateMetricKey("system_device_certificate_status", "invalid", systemLabels()): 1,
						stateMetricKey("system_operational_mode", "normal", systemLabels()):           0,
						stateMetricKey("system_operational_mode", "other", systemLabels()):            1,
					},
				},
			},
		},
		"malformed system uptime is partial failure": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					systemInfoCommand: []byte(strings.Replace(string(dataSystemInfo), "<uptime>2 days, 03:04:05</uptime>", "<uptime>soon</uptime>", 1)),
				}
			},
			steps: []collectStep{
				{
					name: "other metricsets commit and system metrics are omitted",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("ha_status", "enabled", nil): 1,
					},
					wantMissing: []string{metricKey("system_uptime", systemLabels())},
					wantLog:     []string{`system uptime: invalid duration`},
				},
			},
		},
		"malformed environment value preserves other metrics": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					environmentCommand: []byte(`<response status="success"><result><thermal><entry><slot>1</slot><description>Temperature Inlet</description><DegreesC>not-a-number</DegreesC><alarm>True</alarm></entry></thermal></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "sensor alarm survives bad temperature",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
						stateMetricKey("environment_sensor_alarm_status", "alarm", envLabels("temperature", "1", "Temperature Inlet")): 1,
						stateMetricKey("environment_sensor_alarm_status", "clear", envLabels("temperature", "1", "Temperature Inlet")): 0,
					},
					wantMissing: []string{
						metricKey("environment_temperature", envLabels("temperature", "1", "Temperature Inlet")),
					},
					wantLog: []string{`environment temperature Temperature Inlet: invalid decimal`},
				},
			},
		},
		"environment fan and fans sections are both collected": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					environmentCommand: []byte(`<response status="success"><result>
						<fan>
							<entry><slot>1</slot><description>Fan 1 RPM</description><RPMs>9000</RPMs><alarm>False</alarm></entry>
						</fan>
						<fans>
							<entry><slot>1</slot><description>Fan 1 RPM</description><RPMs>9100</RPMs><alarm>False</alarm></entry>
							<entry><slot>2</slot><description>Fan 2 RPM</description><RPMs>9200</RPMs><alarm>True</alarm></entry>
						</fans>
					</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "first duplicate fan wins and second fan is collected",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("environment_fan_speed", envLabels("fan", "1", "Fan 1 RPM")):                         9000,
						stateMetricKey("environment_sensor_alarm_status", "clear", envLabels("fan", "1", "Fan 1 RPM")): 1,
						stateMetricKey("environment_sensor_alarm_status", "alarm", envLabels("fan", "1", "Fan 1 RPM")): 0,
						metricKey("environment_fan_speed", envLabels("fan", "2", "Fan 2 RPM")):                         9200,
						stateMetricKey("environment_sensor_alarm_status", "clear", envLabels("fan", "2", "Fan 2 RPM")): 0,
						stateMetricKey("environment_sensor_alarm_status", "alarm", envLabels("fan", "2", "Fan 2 RPM")): 1,
					},
				},
			},
		},
		"malformed power supply alarm preserves presence": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					environmentCommand: []byte(`<response status="success"><result>
						<power-supply>
							<entry><slot>1</slot><description>Power Supply 1</description><Inserted>False</Inserted><alarm>maybe</alarm></entry>
						</power-supply>
					</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "presence commits and alarm is omitted",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("environment_power_supply_presence_status", "present", envLabels("power_supply", "1", "Power Supply 1")): 0,
						stateMetricKey("environment_power_supply_presence_status", "absent", envLabels("power_supply", "1", "Power Supply 1")):  1,
					},
					wantMissing: []string{
						stateMetricKey("environment_power_supply_alarm_status", "clear", envLabels("power_supply", "1", "Power Supply 1")),
						stateMetricKey("environment_power_supply_alarm_status", "alarm", envLabels("power_supply", "1", "Power Supply 1")),
					},
					wantLog: []string{`environment power supply Power Supply 1 alarm: invalid status`},
				},
			},
		},
		"empty environment payload is partial success": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					environmentCommand: []byte(`<response status="success"><result></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "system metrics commit and environment metrics are absent",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					wantMissing: []string{
						metricKey("environment_fan_speed", envLabels("fan", "1", "Fan 1 RPM")),
					},
					wantLog: []string{
						"environment metricset",
						"expected <thermal>, <fan>, <fans>, <power>, or <power-supply>",
					},
				},
			},
		},
		"HA priority fields are ignored": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					haStateCommand: []byte(strings.Replace(string(dataHAState), "<priority>100</priority>", "<priority>high</priority>", 1)),
				}
			},
			steps: []collectStep{
				{
					name: "malformed priority does not affect HA state collection",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("ha_status", "enabled", nil):                 1,
						stateMetricKey("ha_status", "disabled", nil):                0,
						stateMetricKey("ha_local_state", "active", nil):             1,
						stateMetricKey("ha_peer_state", "passive", nil):             1,
						stateMetricKey("ha_state_sync_status", "synchronized", nil): 1,
						stateMetricKey("ha_state_sync_status", "unknown", nil):      0,
					},
					notWantLog: []string{"PAN-OS partial collection error"},
				},
			},
		},
		"HA disabled emits disabled status": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					haStateCommand: []byte(`<response status="success"><result><enabled>no</enabled></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "disabled status commits without HA detail samples",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("ha_status", "enabled", nil):  0,
						stateMetricKey("ha_status", "disabled", nil): 1,
					},
					wantMissing: []string{
						stateMetricKey("ha_local_state", "unknown", nil),
						stateMetricKey("ha_peer_state", "unknown", nil),
					},
				},
			},
		},
		"missing HA binary status fields are omitted": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					haStateCommand: []byte(`<response status="success"><result>
						<enabled>yes</enabled>
						<group>
							<mode>Active-Passive</mode>
							<local-info>
								<state>active</state>
								<priority>100</priority>
							</local-info>
							<peer-info>
								<state>passive</state>
								<priority>110</priority>
								<conn-ha1-backup>
									<conn-status>down</conn-status>
								</conn-ha1-backup>
								<conn-ha2>
									<conn-status>probing</conn-status>
								</conn-ha2>
							</peer-info>
						</group>
					</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "missing peer/sync/link fields produce gaps, explicit down and unknown remain state sets",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("ha_status", "enabled", nil):                             1,
						stateMetricKey("ha_status", "disabled", nil):                            0,
						stateMetricKey("ha_local_state", "active", nil):                         1,
						stateMetricKey("ha_peer_state", "passive", nil):                         1,
						stateMetricKey("ha_link_status", "up", haLinkLabels("ha1_backup")):      0,
						stateMetricKey("ha_link_status", "down", haLinkLabels("ha1_backup")):    1,
						stateMetricKey("ha_link_status", "unknown", haLinkLabels("ha1_backup")): 0,
						stateMetricKey("ha_link_status", "up", haLinkLabels("ha2")):             0,
						stateMetricKey("ha_link_status", "down", haLinkLabels("ha2")):           0,
						stateMetricKey("ha_link_status", "unknown", haLinkLabels("ha2")):        1,
					},
					wantMissing: []string{
						stateMetricKey("ha_peer_connection_status", "up", nil),
						stateMetricKey("ha_peer_connection_status", "down", nil),
						stateMetricKey("ha_peer_connection_status", "unknown", nil),
						stateMetricKey("ha_state_sync_status", "synchronized", nil),
						stateMetricKey("ha_state_sync_status", "not_synchronized", nil),
						stateMetricKey("ha_state_sync_status", "unknown", nil),
						stateMetricKey("ha_link_status", "up", haLinkLabels("ha1")),
						stateMetricKey("ha_link_status", "down", haLinkLabels("ha1")),
						stateMetricKey("ha_link_status", "unknown", haLinkLabels("ha1")),
						stateMetricKey("ha_link_status", "up", haLinkLabels("ha2_backup")),
						stateMetricKey("ha_link_status", "down", haLinkLabels("ha2_backup")),
						stateMetricKey("ha_link_status", "unknown", haLinkLabels("ha2_backup")),
					},
				},
			},
		},
		"HA non-happy states are normalized": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					haStateCommand: []byte(`<response status="success"><result>
							<enabled>yes</enabled>
							<group>
								<running-sync>incomplete</running-sync>
								<local-info><state>suspended</state></local-info>
								<peer-info>
									<state>non-functional</state>
									<conn-status>probing</conn-status>
								</peer-info>
							</group>
						</result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "suspended non-functional and unknown connection states are explicit",
					wantMetrics: map[string]metrix.SampleValue{
						stateMetricKey("ha_local_state", "suspended", nil):              1,
						stateMetricKey("ha_peer_state", "non_functional", nil):          1,
						stateMetricKey("ha_peer_connection_status", "unknown", nil):     1,
						stateMetricKey("ha_state_sync_status", "not_synchronized", nil): 1,
						stateMetricKey("ha_state_sync_status", "synchronized", nil):     0,
						stateMetricKey("ha_state_sync_status", "unknown", nil):          0,
						stateMetricKey("ha_peer_connection_status", "up", nil):          0,
						stateMetricKey("ha_peer_connection_status", "down", nil):        0,
						stateMetricKey("ha_peer_state", "active", nil):                  0,
						stateMetricKey("ha_peer_state", "passive", nil):                 0,
						stateMetricKey("ha_peer_state", "suspended", nil):               0,
						stateMetricKey("ha_peer_state", "unknown", nil):                 0,
						stateMetricKey("ha_local_state", "active", nil):                 0,
						stateMetricKey("ha_local_state", "passive", nil):                0,
						stateMetricKey("ha_local_state", "non_functional", nil):         0,
						stateMetricKey("ha_local_state", "unknown", nil):                0,
					},
				},
			},
		},
		"malformed license expiration does not emit fake never value": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					licenseInfoCommand: []byte(`<response status="success"><result><licenses><entry><feature>Threat Prevention</feature><description>Threat prevention updates</description><expires>tomorrow-ish</expires><expired>no</expired></entry></licenses></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "status commits and expiration is omitted",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("license_count_total", nil): 1,
						stateMetricKey("license_status", "valid", licenseLabels("Threat Prevention", "Threat prevention updates")): 1,
					},
					wantMissing: []string{metricKey("license_time_until_expiration", licenseLabels("Threat Prevention", "Threat prevention updates"))},
					wantLog:     []string{`license Threat Prevention expiration: invalid expiration date`},
				},
			},
		},
		"license expiration edge cases": {
			prepare: func(c *Collector, api *mockAPIClient) {
				c.now = func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) }
				api.responses = map[string][]byte{
					licenseInfoCommand: []byte(`<response status="success"><result><licenses>
						<entry><feature>Expires Today</feature><description>today</description><expires>May 02, 2026</expires><expired>no</expired></entry>
						<entry><feature>Future</feature><description>future</description><expires>June 01, 2026</expires><expired>no</expired></entry>
						<entry><feature>Never</feature><description>never</description><expires>Never</expires><expired>no</expired></entry>
						<entry><feature>Explicitly Expired</feature><description>explicit expired</description><expires>April 01, 2026</expires><expired>yes</expired></entry>
						<entry><feature>Date Expired</feature><description>date expired</description><expires>April 01, 2026</expires></entry>
					</licenses></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "expired licenses trigger status only",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("license_count_total", nil):                                                                5,
						metricKey("license_count_expired", nil):                                                              2,
						stateMetricKey("license_status", "valid", licenseLabels("Expires Today", "today")):                   1,
						metricKey("license_time_until_expiration", licenseLabels("Expires Today", "today")):                  0,
						metricKey("license_time_until_expiration", licenseLabels("Future", "future")):                        30,
						metricKey("license_time_until_expiration", licenseLabels("Never", "never")):                          metrix.SampleValue(licenseNeverExpires),
						stateMetricKey("license_status", "expired", licenseLabels("Explicitly Expired", "explicit expired")): 1,
						stateMetricKey("license_status", "expired", licenseLabels("Date Expired", "date expired")):           1,
					},
					wantMissing: []string{
						metricKey("license_time_until_expiration", licenseLabels("Explicitly Expired", "explicit expired")),
						metricKey("license_time_until_expiration", licenseLabels("Date Expired", "date expired")),
					},
				},
			},
		},
		"missing licenses payload is partial success": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					licenseInfoCommand: []byte(`<response status="success"><result></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "system commits and license metrics are absent",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					wantMissing: []string{metricKey("license_count_total", nil)},
					wantLog: []string{
						"licenses metricset",
						"expected <licenses>",
					},
				},
			},
		},
		"malformed license status omits status dimensions": {
			prepare: func(c *Collector, api *mockAPIClient) {
				c.now = func() time.Time { return time.Date(2026, 5, 2, 12, 0, 0, 0, time.UTC) }
				api.responses = map[string][]byte{
					licenseInfoCommand: []byte(`<response status="success"><result><licenses><entry><feature>Threat Prevention</feature><description>Threat prevention updates</description><expires>June 01, 2026</expires><expired>maybe</expired></entry></licenses></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "expiration commits and status is omitted",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("license_count_total", nil): 1,
						metricKey("license_time_until_expiration", licenseLabels("Threat Prevention", "Threat prevention updates")): 30,
					},
					wantMissing: []string{
						stateMetricKey("license_status", "valid", licenseLabels("Threat Prevention", "Threat prevention updates")),
						stateMetricKey("license_status", "expired", licenseLabels("Threat Prevention", "Threat prevention updates")),
					},
					wantLog: []string{`license Threat Prevention expired status: invalid status`},
				},
			},
		},
		"malformed IPsec lifetime preserves active tunnel count": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand: []byte(`<response status="success"><result><ntun>1</ntun><entries><entry><name>branch-a</name><gateway>gw-branch-a</gateway><remote>198.51.100.10</remote><remain>soon</remain><tid>66</tid></entry></entries></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name:        "bad tunnel lifetime is omitted",
					wantMetrics: map[string]metrix.SampleValue{metricKey("ipsec_tunnels_active", nil): 1},
					wantMissing: []string{metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-a", "gw-branch-a", "198.51.100.10", "66", "unknown", "unknown"))},
					wantLog:     []string{`IPsec tunnel branch-a remain: invalid integer`},
				},
			},
		},
		"IPsec summary-only response uses ntun": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand: []byte(`<response status="success"><result><ntun>2</ntun></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name:        "active count commits without tunnel instances",
					wantMetrics: map[string]metrix.SampleValue{metricKey("ipsec_tunnels_active", nil): 2},
					wantMissing: []string{metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("unknown", "unknown", "unknown", "unknown", "unknown", "unknown"))},
				},
			},
		},
		"IPsec count mismatch is partial success": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand: []byte(`<response status="success"><result><ntun>2</ntun><entries><entry><name>branch-a</name><gateway>gw-branch-a</gateway><remote>198.51.100.10</remote><remain>60</remain><tid>66</tid></entry></entries></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "count and tunnel metrics commit",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("ipsec_tunnels_active", nil): 2,
						metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-a", "gw-branch-a", "198.51.100.10", "66", "unknown", "unknown")): 60,
					},
					wantLog: []string{"IPsec active tunnel count mismatch: ntun=2 entries=1"},
				},
			},
		},
		"IPsec entries-only response infers active count": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand: []byte(`<response status="success"><result><entries><entry><name>branch-a</name><gateway>gw-branch-a</gateway><remote>198.51.100.10</remote><remain>60</remain><tid>66</tid></entry></entries></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "entries length becomes active tunnel count",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("ipsec_tunnels_active", nil): 1,
						metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-a", "gw-branch-a", "198.51.100.10", "66", "unknown", "unknown")): 60,
					},
				},
			},
		},
		"malformed IPsec active count is partial failure": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand: []byte(`<response status="success"><result><ntun>two</ntun></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "system commits and IPsec active count is omitted",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					wantMissing: []string{metricKey("ipsec_tunnels_active", nil)},
					wantLog:     []string{`IPsec active tunnel count: invalid integer`},
				},
			},
		},
		"missing IPsec payload is partial success": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand: []byte(`<response status="success"><result></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "system commits and IPsec metrics are absent",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					wantMissing: []string{metricKey("ipsec_tunnels_active", nil)},
					wantLog: []string{
						"ipsec metricset",
						"expected <ntun> or <entries>",
					},
				},
			},
		},
		"all metricsets fail": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.errors = allCommandErrors(errors.New("api error"))
			},
			steps: []collectStep{
				{
					name:    "public Collect returns an error",
					wantErr: "api error",
					notWantLog: []string{
						"api error",
						"PAN-OS partial collection error",
					},
				},
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			var logBuf bytes.Buffer
			collr.Logger = logger.NewWithWriter(&logBuf)
			api := &mockAPIClient{}
			collr.apiClient = api
			if tc.prepare != nil {
				tc.prepare(collr, api)
			}

			for _, step := range tc.steps {
				t.Run(step.name, func(t *testing.T) {
					if step.setup != nil {
						step.setup(collr, api)
					}

					logBuf.Reset()
					mx, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
					logOutput := logBuf.String()
					if step.wantErr != "" {
						require.ErrorContains(t, err, step.wantErr)
						assertExpectedLogs(t, logOutput, step.wantLog, step.notWantLog)
						return
					}
					require.NoError(t, err)
					assertExpectedMetrics(t, mx, step.wantMetrics)
					assertMissingMetrics(t, mx, step.wantMissing)
					assertExpectedLogs(t, logOutput, step.wantLog, step.notWantLog)
					if step.check != nil {
						step.check(t, collr, api, mx)
					}
				})
			}
		})
	}
}

func TestCollector_Collect_ReturnsMetricsetAPIErrors(t *testing.T) {
	tests := map[string]struct {
		command     string
		wantMetric  string
		wantMissing string
		wantLog     string
	}{
		"system": {
			command:     systemInfoCommand,
			wantMetric:  stateMetricKey("ha_status", "enabled", nil),
			wantMissing: metricKey("system_uptime", systemLabels()),
			wantLog:     "system metricset: system info query API call: transport failed",
		},
		"ha": {
			command:     haStateCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: stateMetricKey("ha_status", "enabled", nil),
			wantLog:     "ha metricset: HA state query API call: transport failed",
		},
		"environment": {
			command:     environmentCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: metricKey("environment_temperature", envLabels("temperature", "1", "Temperature Inlet")),
			wantLog:     "environment metricset: environmentals query API call: transport failed",
		},
		"licenses": {
			command:     licenseInfoCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: metricKey("license_count_total", nil),
			wantLog:     "licenses metricset: license info query API call: transport failed",
		},
		"ipsec": {
			command:     ipsecSACommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: metricKey("ipsec_tunnels_active", nil),
			wantLog:     "ipsec metricset: IPsec SA query API call: transport failed",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logBuf bytes.Buffer
			collr := New()
			collr.Logger = logger.NewWithWriter(&logBuf)
			collr.apiClient = &mockAPIClient{
				errors: map[string]error{tc.command: errors.New("transport failed")},
			}

			mx, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assertMetricPresent(t, mx, tc.wantMetric)
			assertMissingMetrics(t, mx, []string{tc.wantMissing})
			assert.Contains(t, logBuf.String(), tc.wantLog)
		})
	}
}

func TestCollector_Collect_ReportsMalformedXMLResponse(t *testing.T) {
	tests := map[string]struct {
		command     string
		wantMetric  string
		wantMissing string
		wantLog     string
	}{
		"system": {
			command:     systemInfoCommand,
			wantMetric:  stateMetricKey("ha_status", "enabled", nil),
			wantMissing: metricKey("system_uptime", systemLabels()),
			wantLog:     "parse PAN-OS system info response",
		},
		"ha": {
			command:     haStateCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: stateMetricKey("ha_status", "enabled", nil),
			wantLog:     "parse PAN-OS HA response",
		},
		"environment": {
			command:     environmentCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: metricKey("environment_temperature", envLabels("temperature", "1", "Temperature Inlet")),
			wantLog:     "parse PAN-OS environment response",
		},
		"licenses": {
			command:     licenseInfoCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: metricKey("license_count_total", nil),
			wantLog:     "parse PAN-OS licenses response",
		},
		"ipsec": {
			command:     ipsecSACommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: metricKey("ipsec_tunnels_active", nil),
			wantLog:     "parse PAN-OS IPsec response",
		},
		"bgp": {
			command:     legacyBGPPeerCommand,
			wantMetric:  metricKey("system_uptime", systemLabels()),
			wantMissing: stateMetricKey("bgp_peer_state", "established", legacyPeerLabels()),
			wantLog:     "parse PAN-OS BGP response",
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			var logBuf bytes.Buffer
			collr := New()
			collr.Logger = logger.NewWithWriter(&logBuf)
			collr.apiClient = &mockAPIClient{
				responses: map[string][]byte{tc.command: []byte(`<response status="success"><result><broken></result></response>`)},
			}

			mx, err := collecttest.CollectScalarSeries(collr, metrix.ReadFlatten())
			require.NoError(t, err)
			assertMetricPresent(t, mx, tc.wantMetric)
			assertMissingMetrics(t, mx, []string{tc.wantMissing})
			assert.Contains(t, logBuf.String(), tc.wantLog)
		})
	}
}

func TestPangoAPIClient_Op(t *testing.T) {
	tests := map[string]struct {
		client *pangoAPIClient
		check  func(*testing.T, *pangoAPIClient, *mockPangoOperator, []byte, error)
	}{
		"refreshes API key once on unauthorized operation": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					responses: []mockPangoResponse{
						{err: errors.New("code 16: Unauthorized")},
						{body: []byte("<response status=\"success\"/>")},
					},
				},
				canRefresh: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.NoError(t, err)
				assert.Equal(t, []byte("<response status=\"success\"/>"), body)
				assert.Equal(t, 2, operator.opCalls)
				assert.Equal(t, 1, operator.refreshCalls)
			},
		},
		"passes vsys to pango operation": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					responses: []mockPangoResponse{
						{body: []byte("<response status=\"success\"/>")},
					},
				},
				vsys:        "vsys2",
				initialized: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.NoError(t, err)
				assert.Equal(t, []byte("<response status=\"success\"/>"), body)
				assert.Equal(t, []string{"vsys2"}, operator.vsys)
			},
		},
		"initialize non unauthorized error is not refreshed": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					initializeErr: errors.New("dial tcp failed"),
				},
				canRefresh: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.ErrorContains(t, err, "dial tcp failed")
				assert.Nil(t, body)
				assert.Equal(t, 1, operator.initializeCalls)
				assert.Equal(t, 0, operator.refreshCalls)
				assert.Equal(t, 0, operator.opCalls)
			},
		},
		"refreshes API key when initialize finds expired key": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					initializeErrs: []error{errors.New("code 16: Unauthorized"), nil},
					responses: []mockPangoResponse{
						{body: []byte("<response status=\"success\"/>")},
					},
				},
				canRefresh: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.NoError(t, err)
				assert.Equal(t, []byte("<response status=\"success\"/>"), body)
				assert.Equal(t, 2, operator.initializeCalls)
				assert.Equal(t, 1, operator.opCalls)
				assert.Equal(t, 1, operator.refreshCalls)
			},
		},
		"refresh failure after unauthorized initialize is returned": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					initializeErrs: []error{errors.New("code 16: Unauthorized")},
					refreshErr:     errors.New("refresh failed"),
				},
				canRefresh: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.ErrorContains(t, err, "refresh PAN-OS API key after unauthorized initialization")
				require.ErrorContains(t, err, "refresh failed")
				assert.Nil(t, body)
				assert.Equal(t, 1, operator.initializeCalls)
				assert.Equal(t, 1, operator.refreshCalls)
				assert.Equal(t, 0, operator.opCalls)
			},
		},
		"reinitialize failure after refresh is returned": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					initializeErrs: []error{errors.New("code 16: Unauthorized"), errors.New("still unauthorized")},
				},
				canRefresh: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.ErrorContains(t, err, "re-initialize PAN-OS API client after key refresh")
				require.ErrorContains(t, err, "still unauthorized")
				assert.Nil(t, body)
				assert.Equal(t, 2, operator.initializeCalls)
				assert.Equal(t, 1, operator.refreshCalls)
				assert.Equal(t, 0, operator.opCalls)
			},
		},
		"does not refresh API key on unrelated response code": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					responses: []mockPangoResponse{
						{err: errors.New("code 160: operation failed")},
						{body: []byte("<response status=\"success\"/>")},
					},
				},
				canRefresh: true,
			},
			check: func(t *testing.T, _ *pangoAPIClient, operator *mockPangoOperator, body []byte, err error) {
				require.ErrorContains(t, err, "code 160")
				assert.Nil(t, body)
				assert.Equal(t, 1, operator.opCalls)
				assert.Equal(t, 0, operator.refreshCalls)
			},
		},
		"resets initialization when refresh fails": {
			client: &pangoAPIClient{
				client: &mockPangoOperator{
					responses: []mockPangoResponse{
						{err: errors.New("code 16: Unauthorized")},
					},
					refreshErr: errors.New("refresh failed with key=secret"),
				},
				canRefresh:  true,
				initialized: true,
			},
			check: func(t *testing.T, client *pangoAPIClient, _ *mockPangoOperator, body []byte, err error) {
				require.Error(t, err)
				assert.Nil(t, body)
				assert.False(t, client.initialized)
				assert.NotContains(t, err.Error(), "secret")
				assert.Contains(t, err.Error(), "key=<redacted>")
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			operator := tc.client.client.(*mockPangoOperator)
			body, err := tc.client.op(context.Background(), "cmd")
			tc.check(t, tc.client, operator, body, err)
		})
	}
}

func TestIsUnauthorizedError(t *testing.T) {
	tests := map[string]struct {
		err  error
		want bool
	}{
		"nil": {
			err:  nil,
			want: false,
		},
		"unauthorized": {
			err:  errors.New("Unauthorized"),
			want: true,
		},
		"code 16": {
			err:  errors.New("code 16: Unauthorized"),
			want: true,
		},
		"code colon 16": {
			err:  errors.New("code: 16"),
			want: true,
		},
		"code 22": {
			err:  errors.New("code 22: session timed out"),
			want: true,
		},
		"code 403": {
			err:  errors.New("code 403: forbidden"),
			want: true,
		},
		"forbidden": {
			err:  errors.New("forbidden"),
			want: true,
		},
		"session timed out": {
			err:  errors.New("session timed out"),
			want: true,
		},
		"code 160": {
			err:  errors.New("code 160: operation failed"),
			want: false,
		},
		"code 162": {
			err:  errors.New("code 162: operation failed"),
			want: false,
		},
		"connection refused": {
			err:  errors.New("dial tcp 192.0.2.1:443: connect: connection refused"),
			want: false,
		},
		"tls error": {
			err:  errors.New("tls: failed to verify certificate"),
			want: false,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, isUnauthorizedError(tc.err))
		})
	}
}

func TestSanitizePANOSAPIError(t *testing.T) {
	tests := map[string]struct {
		err     error
		notWant []string
		want    []string
	}{
		"password query parameter": {
			err:     errors.New("https://fw.example.invalid/api/?type=keygen&user=netdata&password=secret"),
			notWant: []string{"netdata", "secret"},
			want:    []string{"type=keygen", "user=<redacted>", "password=<redacted>"},
		},
		"username query parameter": {
			err:     errors.New("https://fw.example.invalid/api/?username=netdata&api_key=secret"),
			notWant: []string{"netdata", "secret"},
			want:    []string{"username=<redacted>", "api_key=<redacted>"},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			err := sanitizePANOSAPIError(tc.err)
			require.Error(t, err)
			for _, s := range tc.notWant {
				assert.NotContains(t, err.Error(), s)
			}
			for _, s := range tc.want {
				assert.Contains(t, err.Error(), s)
			}
		})
	}
}

func TestParseBGPPeers(t *testing.T) {
	tests := map[string]struct {
		data     []byte
		wantLen  int
		wantErr  string
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
		"error response with nested lines": {
			data:    []byte(`<response status="error" code="16"><msg><line>Unauthorized</line><line>Invalid API key</line></msg></response>`),
			wantErr: "Unauthorized; Invalid API key",
		},
		"error response with result message": {
			data:    []byte(`<response status="error" code="400"><result><msg>Parameter &quot;format&quot; is required while exporting certificate</msg></result></response>`),
			wantErr: `Parameter "format" is required while exporting certificate`,
		},
		"malformed numeric field fails peer": {
			data:    []byte(`<response status="success"><result><entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>abc</msg-total-in><msg-total-out>1</msg-total-out><msg-update-in>1</msg-update-in><msg-update-out>1</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry></result></response>`),
			wantErr: `BGP peer 192.0.2.1 msg-total-in: invalid integer "abc"`,
		},
		"missing numeric field fails peer": {
			data:    []byte(`<response status="success"><result><entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>1</msg-total-in><msg-total-out>1</msg-total-out><msg-update-in>1</msg-update-in><msg-update-out>1</msg-update-out><status-flap-counts>0</status-flap-counts></entry></result></response>`),
			wantErr: "BGP peer 192.0.2.1 established-counts: missing integer",
		},
		"malformed peer is skipped when another peer is valid": {
			data: []byte(`<response status="success"><result>
				<entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>abc</msg-total-in><msg-total-out>1</msg-total-out><msg-update-in>1</msg-update-in><msg-update-out>1</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
				<entry><peer-address>192.0.2.2</peer-address><status>Established</status><status-duration>120</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
			</result></response>`),
			wantLen: 1,
			wantErr: `BGP peer entry 192.0.2.1: BGP peer 192.0.2.1 msg-total-in: invalid integer "abc"`,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "192.0.2.2", peers[0].PeerAddress)
				assert.Equal(t, int64(120), peers[0].Uptime)
			},
		},
		"malformed prefix counter preserves peer": {
			data: []byte(`<response status="success"><result>
					<entry>
						<peer-address>192.0.2.1</peer-address>
					<status>Established</status>
					<status-duration>60</status-duration>
					<msg-total-in>10</msg-total-in>
					<msg-total-out>20</msg-total-out>
					<msg-update-in>3</msg-update-in>
					<msg-update-out>4</msg-update-out>
					<status-flap-counts>0</status-flap-counts>
					<established-counts>1</established-counts>
					<prefix-counter>
						<entry name="ipv4-unicast"><incoming-total>bad</incoming-total><incoming-accepted>1</incoming-accepted><incoming-rejected>0</incoming-rejected><outgoing-advertised>2</outgoing-advertised></entry>
						<entry name="ipv6-unicast"><incoming-total>7</incoming-total><incoming-accepted>6</incoming-accepted><incoming-rejected>1</incoming-rejected><outgoing-advertised>3</outgoing-advertised></entry>
					</prefix-counter>
				</entry>
			</result></response>`),
			wantLen: 1,
			wantErr: `BGP peer entry 192.0.2.1: BGP peer 192.0.2.1 ipv4-unicast incoming-total: invalid integer "bad"`,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "192.0.2.1", peers[0].PeerAddress)
				require.Len(t, peers[0].PrefixCounters, 1)
				assert.Equal(t, "ipv6", peers[0].PrefixCounters[0].AFI)
				assert.Equal(t, "unicast", peers[0].PrefixCounters[0].SAFI)
				assert.Equal(t, int64(7), peers[0].PrefixCounters[0].IncomingTotal)
			},
		},
		"deduplicates same vr and peer": {
			data: []byte(`<response status="success"><result>
					<entry><vr>default</vr><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
					<entry><vr>default</vr><peer-address>192.0.2.1</peer-address><status>Active</status><status-duration>120</status-duration><msg-total-in>11</msg-total-in><msg-total-out>21</msg-total-out><msg-update-in>4</msg-update-in><msg-update-out>5</msg-update-out><status-flap-counts>1</status-flap-counts><established-counts>2</established-counts></entry>
				</result></response>`),
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "192.0.2.1", peers[0].PeerAddress)
				assert.Equal(t, "established", peers[0].State)
				assert.Equal(t, int64(10), peers[0].MessagesIn)
			},
		},
		"uses peer name when peer address is missing": {
			data: []byte(`<response status="success"><result>
					<entry name="peer-a"><state>Established</state><uptime>60</uptime><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><flap-count>0</flap-count><established-counts>1</established-counts></entry>
				</result></response>`),
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "peer-a", peers[0].PeerAddress)
				assert.Equal(t, "established", peers[0].State)
			},
		},
		"attribute-only peer fields": {
			data: []byte(`<response status="success"><result>
					<entry peer-address="192.0.2.4" vr="vr-a" peer-group="edge" remote-as="65010"><status>Established</status><status-duration>60</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
				</result></response>`),
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "vr-a", peers[0].VR)
				assert.Equal(t, "192.0.2.4", peers[0].PeerAddress)
				assert.Equal(t, "edge", peers[0].PeerGroup)
				assert.Equal(t, "65010", peers[0].RemoteAS)
			},
		},
		"prefix counters without afi safi use unknown family": {
			data: []byte(`<response status="success"><result>
					<entry>
						<peer-address>192.0.2.1</peer-address>
						<status>Established</status>
						<status-duration>60</status-duration>
						<msg-total-in>10</msg-total-in>
						<msg-total-out>20</msg-total-out>
						<msg-update-in>3</msg-update-in>
						<msg-update-out>4</msg-update-out>
						<status-flap-counts>0</status-flap-counts>
						<established-counts>1</established-counts>
						<prefix-counter>
							<entry><incoming-total>7</incoming-total><incoming-accepted>6</incoming-accepted><incoming-rejected>1</incoming-rejected><outgoing-advertised>3</outgoing-advertised></entry>
						</prefix-counter>
					</entry>
				</result></response>`),
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				require.Len(t, peers[0].PrefixCounters, 1)
				assert.Equal(t, "unknown", peers[0].PrefixCounters[0].AFI)
				assert.Equal(t, "unknown", peers[0].PrefixCounters[0].SAFI)
				assert.Equal(t, int64(7), peers[0].PrefixCounters[0].IncomingTotal)
			},
		},
		"container entries without peer data are skipped": {
			data: []byte(`<response status="success"><result>
					<entry name="default">
						<entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
					</entry>
				</result></response>`),
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, "default", peers[0].VR)
				assert.Equal(t, "192.0.2.1", peers[0].PeerAddress)
			},
		},
		"deep nesting beyond limit is truncated": {
			data: deepNestedBGPPeerXML(maxBGPPeerEntryDepth + 1),
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Empty(t, peers)
			},
		},
		"placeholder uptime fails peer": {
			data: []byte(`<response status="success"><result>
					<entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>n/a</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
				</result></response>`),
			wantErr: `BGP peer 192.0.2.1 uptime: invalid duration "n/a"`,
		},
		"zero uptime is accepted": {
			data: []byte(`<response status="success"><result>
					<entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>0</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>
				</result></response>`),
			wantLen: 1,
			validate: func(t *testing.T, peers []bgpPeer) {
				assert.Equal(t, int64(0), peers[0].Uptime)
			},
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			peers, err := parseBGPPeers(tc.data)
			if tc.wantErr != "" {
				require.ErrorContains(t, err, tc.wantErr)
			} else {
				require.NoError(t, err)
			}
			if tc.wantLen > 0 {
				require.Len(t, peers, tc.wantLen)
			}
			if tc.validate != nil {
				tc.validate(t, peers)
			}
		})
	}
}

func TestParseReadOnlyTelemetry(t *testing.T) {
	tests := map[string]func(*testing.T){
		"system": func(t *testing.T) {
			system, err := parseSystemInfo(dataSystemInfo)
			require.NoError(t, err)
			assert.Equal(t, "edge-fw-a", system.Hostname)
			assert.Equal(t, "11.1.2", system.SWVersion)
		},
		"ha": func(t *testing.T) {
			ha, err := parseHAState(dataHAState)
			require.NoError(t, err)
			assert.Equal(t, "yes", ha.Enabled)
			assert.Equal(t, "active", normalizeHAState(ha.Group.LocalInfo.State))
			assert.Equal(t, "passive", normalizeHAState(ha.Group.PeerInfo.State))
		},
		"environment": func(t *testing.T) {
			env, err := parseEnvironment(dataEnvironment)
			require.NoError(t, err)
			require.Len(t, env.ThermalEntries, 1)
			require.Len(t, env.FanEntries, 1)
			require.Len(t, env.VoltageEntries, 1)
			require.Len(t, env.PowerSupplyEntries, 1)
			assert.Equal(t, "Temperature Inlet", env.ThermalEntries[0].Description)
			assert.Equal(t, "3.332", env.VoltageEntries[0].Volts)
		},
		"environment fan and fans": func(t *testing.T) {
			env, err := parseEnvironment([]byte(`<response status="success"><result>
				<fan>
					<entry><slot>1</slot><description>Fan 1 RPM</description><RPMs>9000</RPMs><alarm>False</alarm></entry>
				</fan>
				<fans>
					<entry><slot>1</slot><description>Fan 1 RPM</description><RPMs>9100</RPMs><alarm>False</alarm></entry>
					<entry><slot>2</slot><description>Fan 2 RPM</description><RPMs>9200</RPMs><alarm>True</alarm></entry>
				</fans>
			</result></response>`))
			require.NoError(t, err)
			require.Len(t, env.FanEntries, 2)
			assert.Equal(t, "Fan 1 RPM", env.FanEntries[0].Description)
			assert.Equal(t, "9000", env.FanEntries[0].RPMs)
			assert.Equal(t, "Fan 2 RPM", env.FanEntries[1].Description)
			assert.Equal(t, "9200", env.FanEntries[1].RPMs)
		},
		"licenses": func(t *testing.T) {
			licenses, found, err := parseLicenses(dataLicenses)
			require.NoError(t, err)
			assert.True(t, found)
			require.Len(t, licenses, 3)
			assert.Equal(t, "Threat Prevention", licenses[0].Feature)
		},
		"ipsec": func(t *testing.T) {
			ipsecPayload, err := parseIPSecTunnels(dataIPSecSA)
			require.NoError(t, err)
			assert.True(t, ipsecPayload.found)
			assert.True(t, ipsecPayload.entriesFound)
			assert.Equal(t, int64(2), ipsecPayload.activeCount)
			require.Len(t, ipsecPayload.tunnels, 2)
			assert.Equal(t, "branch-a", ipsecPayload.tunnels[0].Name)
		},
	}

	for name, run := range tests {
		t.Run(name, run)
	}
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
			"unknown-state": "unknown",
			"":              "",
		}
		for in, want := range tests {
			assert.Equal(t, want, normalizeBGPState(in), in)
		}
	})

	t.Run("parse PAN-OS duration", func(t *testing.T) {
		tests := map[string]int64{
			"3600":              3600,
			"0":                 0,
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
		tests := map[string]struct {
			parse   func() error
			wantErr string
		}{
			"invalid integer": {
				parse: func() error {
					_, err := parsePANOSIntField("test integer", "not-an-int")
					return err
				},
				wantErr: `test integer: invalid integer "not-an-int"`,
			},
			"missing integer": {
				parse: func() error {
					_, err := parseRequiredPANOSIntField("test integer", "")
					return err
				},
				wantErr: "test integer: missing integer",
			},
			"invalid decimal": {
				parse: func() error {
					_, err := parsePANOSDecimalField("test decimal", "not-a-decimal", 1000)
					return err
				},
				wantErr: `test decimal: invalid decimal "not-a-decimal"`,
			},
			"missing decimal": {
				parse: func() error {
					_, err := parseRequiredPANOSDecimalField("test decimal", "", 1000)
					return err
				},
				wantErr: "test decimal: missing decimal",
			},
			"invalid duration": {
				parse: func() error {
					_, err := parsePANOSDurationField("test duration", "since reboot")
					return err
				},
				wantErr: `test duration: invalid duration "since reboot"`,
			},
			"missing duration": {
				parse: func() error {
					_, err := parseRequiredPANOSDurationField("test duration", "")
					return err
				},
				wantErr: "test duration: missing duration",
			},
			"invalid clock duration": {
				parse: func() error {
					_, err := parsePANOSDurationField("test duration", "01:99:00")
					return err
				},
				wantErr: `test duration: invalid duration "01:99:00"`,
			},
			"placeholder duration never": {
				parse: func() error {
					_, err := parsePANOSDurationField("test duration", "never")
					return err
				},
				wantErr: `test duration: invalid duration "never"`,
			},
			"placeholder duration dash": {
				parse: func() error {
					_, err := parsePANOSDurationField("test duration", "-")
					return err
				},
				wantErr: `test duration: invalid duration "-"`,
			},
			"placeholder duration n/a": {
				parse: func() error {
					_, err := parsePANOSDurationField("test duration", "n/a")
					return err
				},
				wantErr: `test duration: invalid duration "n/a"`,
			},
		}
		for name, tc := range tests {
			t.Run(name, func(t *testing.T) {
				assert.EqualError(t, tc.parse(), tc.wantErr)
			})
		}
	})

	t.Run("normalize address", func(t *testing.T) {
		tests := map[string]string{
			"192.0.2.1:179":       "192.0.2.1",
			"192.0.2.1":           "192.0.2.1",
			"[2001:db8::1]:179":   "2001:db8::1",
			"2001:db8::1":         "2001:db8::1",
			"[2001:db8::1]":       "2001:db8::1",
			"fw.example.invalid":  "fw.example.invalid",
			"example.invalid:179": "example.invalid",
			"example.invalid:bgp": "example.invalid",
		}
		for in, want := range tests {
			assert.Equal(t, want, normalizeAddress(in), in)
		}
	})

	t.Run("normalize AFI SAFI", func(t *testing.T) {
		tests := map[string]struct {
			wantAFI  string
			wantSAFI string
		}{
			"bgpAfiIpv4-unicast": {wantAFI: "ipv4", wantSAFI: "unicast"},
			"ipv6-unicast":       {wantAFI: "ipv6", wantSAFI: "unicast"},
		}
		for in, want := range tests {
			afi, safi := normalizeAFISAFI(in)
			assert.Equal(t, want.wantAFI, afi, in)
			assert.Equal(t, want.wantSAFI, safi, in)
		}
	})

	t.Run("PAN-OS response code names", func(t *testing.T) {
		tests := map[string]string{
			"1":       "Unknown command",
			"6":       "Bad XPath",
			"16":      "Unauthorized",
			"22":      "Session timed out",
			"400":     "Bad request",
			" 403 ":   "Forbidden",
			"unknown": "",
		}
		for code, want := range tests {
			assert.Equal(t, want, panosResponseCodeName(code), code)
		}
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
		"ipv4 port api path": {
			raw: "https://192.0.2.1:8443/api",
			want: panosAPIURL{
				protocol: "https",
				hostname: "192.0.2.1",
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
		"ipv6 port": {
			raw: "https://[2001:db8::1]:8443/api",
			want: panosAPIURL{
				protocol: "https",
				hostname: "[2001:db8::1]",
				port:     8443,
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
		"query": {
			raw:      "https://192.0.2.1/api?type=keygen",
			wantFail: true,
		},
		"fragment": {
			raw:      "https://192.0.2.1/api#fragment",
			wantFail: true,
		},
		"port zero": {
			raw:      "https://192.0.2.1:0/api",
			wantFail: true,
		},
		"port greater than max": {
			raw:      "https://192.0.2.1:65536/api",
			wantFail: true,
		},
		"non numeric port": {
			raw:      "https://192.0.2.1:not-a-port/api",
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

func assertBGPProbeErrorNotCached(t *testing.T, c *Collector, api *mockAPIClient, _ map[string]metrix.SampleValue) {
	t.Helper()
	assert.Equal(t, routingEngineUnknown, c.routingEngine)
	assert.True(t, c.noBGPProbedAt.IsZero())
	assert.Len(t, api.commands, 9)
}

func collectOnceWithContext(t *testing.T, c *Collector, ctx context.Context) error {
	t.Helper()

	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	require.True(t, ok)

	cycle := managed.CycleController()
	committed := false
	cycle.BeginCycle()
	defer func() {
		if !committed {
			cycle.AbortCycle()
		}
	}()

	if err := c.Collect(ctx); err != nil {
		return err
	}
	require.NoError(t, cycle.CommitCycleSuccess())
	committed = true
	return nil
}

func allCommandErrors(err error) map[string]error {
	return map[string]error{
		systemInfoCommand:          err,
		haStateCommand:             err,
		environmentCommand:         err,
		licenseInfoCommand:         err,
		ipsecSACommand:             err,
		legacyBGPPeerCommand:       err,
		advancedBGPPeerCommands[0]: err,
		advancedBGPPeerCommands[1]: err,
		advancedBGPPeerCommands[2]: err,
	}
}

func assertExpectedMetrics(t *testing.T, got map[string]metrix.SampleValue, want map[string]metrix.SampleValue) {
	t.Helper()
	for key, wantValue := range want {
		gotValue, ok := got[key]
		require.True(t, ok, "metric %s", key)
		assert.Equal(t, wantValue, gotValue, key)
	}
}

func assertMetricPresent(t *testing.T, got map[string]metrix.SampleValue, key string) {
	t.Helper()
	_, ok := got[key]
	require.True(t, ok, "metric %s", key)
}

func assertMissingMetrics(t *testing.T, got map[string]metrix.SampleValue, missing []string) {
	t.Helper()
	for _, key := range missing {
		_, ok := got[key]
		assert.False(t, ok, "metric %s", key)
	}
}

func assertExpectedLogs(t *testing.T, got string, want, notWant []string) {
	t.Helper()
	for _, text := range want {
		assert.Contains(t, got, text)
	}
	for _, text := range notWant {
		assert.NotContains(t, got, text)
	}
}

func metricKey(name string, labels metrix.Labels) string {
	if len(labels) == 0 {
		return name
	}

	keys := make([]string, 0, len(labels))
	for key := range labels {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	var b strings.Builder
	b.WriteString(name)
	b.WriteByte('{')
	for i, key := range keys {
		if i > 0 {
			b.WriteByte(',')
		}
		b.WriteString(key)
		b.WriteByte('=')
		b.WriteString(strconv.Quote(labels[key]))
	}
	b.WriteByte('}')
	return b.String()
}

func stateMetricKey(name, state string, labels metrix.Labels) string {
	return metricKey(name, stateLabels(name, state, labels))
}

func stateLabels(name, state string, labels metrix.Labels) metrix.Labels {
	out := make(metrix.Labels, len(labels)+1)
	maps.Copy(out, labels)
	out[name] = state
	return out
}

func systemLabels() metrix.Labels {
	return metrix.Labels{"hostname": "edge-fw-a", "model": "PA-850", "serial": "0123456789", "sw_version": "11.1.2"}
}

func envLabels(sensorType, slot, sensor string) metrix.Labels {
	return metrix.Labels{"sensor_type": sensorType, "slot": slot, "sensor": sensor}
}

func haLinkLabels(link string) metrix.Labels {
	return metrix.Labels{"link": link}
}

func licenseLabels(feature, description string) metrix.Labels {
	return metrix.Labels{"feature": feature, "description": description}
}

func ipsecLabels(tunnel, gateway, remote, tunnelID, protocol, encryption string) metrix.Labels {
	return metrix.Labels{
		"tunnel":     tunnel,
		"gateway":    gateway,
		"remote":     remote,
		"tunnel_id":  tunnelID,
		"protocol":   protocol,
		"encryption": encryption,
	}
}

func legacyPeerLabels() metrix.Labels {
	return legacyPeerLabelsWithRemoteAS("65001")
}

func legacyPeerLabelsWithRemoteAS(remoteAS string) metrix.Labels {
	return metrix.Labels{
		"vr":            "default",
		"peer_address":  "192.0.2.1",
		"local_address": "192.0.2.254",
		"remote_as":     remoteAS,
		"peer_group":    "edge",
	}
}

func advancedPeerLabels() metrix.Labels {
	return metrix.Labels{
		"vr":            "lr-a",
		"peer_address":  "203.0.113.1",
		"local_address": "203.0.113.254",
		"remote_as":     "65100",
		"peer_group":    "core",
	}
}

func advancedPrefixLabels(afi, safi string) metrix.Labels {
	labels := advancedPeerLabels()
	labels["afi"] = afi
	labels["safi"] = safi
	return labels
}

func fallbackPeerLabels(peerAddress string) metrix.Labels {
	return metrix.Labels{
		"vr":            "default",
		"peer_address":  peerAddress,
		"local_address": "unknown",
		"remote_as":     "unknown_as",
		"peer_group":    "unknown_group",
	}
}

func fallbackPrefixLabels(peerAddress, afi, safi string) metrix.Labels {
	labels := fallbackPeerLabels(peerAddress)
	labels["afi"] = afi
	labels["safi"] = safi
	return labels
}

func deepNestedBGPPeerXML(depth int) []byte {
	var b strings.Builder
	b.WriteString(`<response status="success"><result>`)
	for i := range depth {
		b.WriteString(`<entry name="container`)
		b.WriteString(strconv.Itoa(i))
		b.WriteString(`">`)
	}
	b.WriteString(`<entry><peer-address>192.0.2.1</peer-address><status>Established</status><status-duration>60</status-duration><msg-total-in>10</msg-total-in><msg-total-out>20</msg-total-out><msg-update-in>3</msg-update-in><msg-update-out>4</msg-update-out><status-flap-counts>0</status-flap-counts><established-counts>1</established-counts></entry>`)
	for range depth {
		b.WriteString(`</entry>`)
	}
	b.WriteString(`</result></response>`)
	return []byte(b.String())
}

type mockAPIClient struct {
	responses  map[string][]byte
	errors     map[string]error
	commands   []string
	info       map[string]string
	closeCalls int
	onOp       func(context.Context, string)
}

func (m *mockAPIClient) op(ctx context.Context, cmd string) ([]byte, error) {
	m.commands = append(m.commands, cmd)
	if m.onOp != nil {
		m.onOp(ctx, cmd)
	}
	if err := m.errors[cmd]; err != nil {
		return nil, err
	}
	if resp := m.responses[cmd]; resp != nil {
		return resp, nil
	}
	switch cmd {
	case systemInfoCommand:
		return dataSystemInfo, nil
	case haStateCommand:
		return dataHAState, nil
	case environmentCommand:
		return dataEnvironment, nil
	case licenseInfoCommand:
		return dataLicenses, nil
	case ipsecSACommand:
		return dataIPSecSA, nil
	case legacyBGPPeerCommand, advancedBGPPeerCommands[0], advancedBGPPeerCommands[1], advancedBGPPeerCommands[2]:
		return []byte(`<response status="success"><result></result></response>`), nil
	default:
		return []byte(`<response status="success"><result></result></response>`), nil
	}
}

func (m *mockAPIClient) closeIdleConnections() { m.closeCalls++ }

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
	vsys            []string
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

func (m *mockPangoOperator) Op(_ any, vsys string, _, _ any) ([]byte, error) {
	m.vsys = append(m.vsys, vsys)
	resp := m.responses[m.opCalls]
	m.opCalls++
	return resp.body, resp.err
}

func (m *mockPangoOperator) RetrieveApiKey() error {
	m.refreshCalls++
	return m.refreshErr
}

func (m *mockPangoOperator) SystemInfo() map[string]string { return m.info }
