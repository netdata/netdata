// SPDX-License-Identifier: GPL-3.0-or-later

package panos

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/charttpl"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/collecttest"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestCollector_IPSecState(t *testing.T) {
	tests := map[string]collectScenario{
		"IPsec tunnel and gateway state with every tunnel established": {
			steps: []collectStep{
				{
					name: "all tunnels active and all gateways up",
					wantMetrics: mergeMetrics(
						ipsecTunnelStateMetrics("branch-a", "1", "active"),
						ipsecTunnelStateMetrics("branch-a-2", "2", "active"),
						ipsecTunnelStateMetrics("branch-b", "5", "active"),
						ipsecTunnelStateMetrics("branch-b-2", "6", "active"),
						ipsecTunnelStateMetrics("branch-d", "3", "active"),
						ipsecTunnelStateMetrics("branch-c", "4", "active"),
						ipsecGatewayStateMetrics("gw-branch-a", "ikev1", "up"),
						ipsecGatewayStateMetrics("gw-branch-b", "ikev1", "up"),
						ipsecGatewayStateMetrics("gw-branch-c", "ikev2", "up"),
						ipsecGatewayStateMetrics("gw-branch-d", "ikev2", "up"),
					),
					check: func(t *testing.T, c *Collector, _ *mockAPIClient, _ map[string]metrix.SampleValue) {
						collecttest.AssertChartCoverage(t, c, collecttest.ChartCoverageExpectation{})
					},
				},
			},
		},
		"IPsec standby firewall with tunnels that have no SA": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecSACommand:      dataIPSecSAPartial,
					ipsecFlowCommand:    dataIPSecFlowPartial,
					ipsecIKESACommand:   dataIPSecIKESAPartial,
					ipsecGatewayCommand: dataIPSecGateway,
				}
			},
			steps: []collectStep{
				{
					name: "configured tunnels without an SA are inactive and their gateways are down",
					wantMetrics: mergeMetrics(
						map[string]metrix.SampleValue{
							metricKey("ipsec_tunnels_active", nil): 2,
							metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-d", "gw-branch-d", "198.51.100.40", "3", "ESP", "G256")): 5145,
						},
						ipsecTunnelStateMetrics("branch-a", "1", "inactive"),
						ipsecTunnelStateMetrics("branch-a-2", "2", "inactive"),
						ipsecTunnelStateMetrics("branch-b", "5", "inactive"),
						ipsecTunnelStateMetrics("branch-b-2", "6", "inactive"),
						ipsecTunnelStateMetrics("branch-d", "3", "active"),
						ipsecTunnelStateMetrics("branch-c", "4", "active"),
						ipsecGatewayStateMetrics("gw-branch-a", "ikev1", "down"),
						ipsecGatewayStateMetrics("gw-branch-b", "ikev1", "down"),
						ipsecGatewayStateMetrics("gw-branch-c", "ikev2", "up"),
						ipsecGatewayStateMetrics("gw-branch-d", "ikev2", "up"),
					),
					wantMissing: []string{
						metricKey("ipsec_tunnel_sa_lifetime", ipsecLabels("branch-a", "gw-branch-a", "198.51.100.10", "1", "ESP", "A256")),
					},
				},
			},
		},
		"IPsec empty IKE SA list marks every gateway down": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				// Captured from a gateway-filtered show vpn ike-sa for a gateway with no SA.
				api.responses = map[string][]byte{ipsecIKESACommand: dataIPSecIKESAEmpty}
			},
			steps: []collectStep{
				{
					name: "no SAs is a valid response, not a gap",
					wantMetrics: mergeMetrics(
						ipsecGatewayStateMetrics("gw-branch-a", "ikev1", "down"),
						ipsecGatewayStateMetrics("gw-branch-b", "ikev1", "down"),
						ipsecGatewayStateMetrics("gw-branch-c", "ikev2", "down"),
						ipsecGatewayStateMetrics("gw-branch-d", "ikev2", "down"),
					),
				},
			},
		},
		"IPsec unexpected IKE SA payload leaves gateway state as a gap": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecIKESACommand: []byte(`<response status="success"><result><error>IKE SA query failed.</error></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "gateways are not reported down when the SA list cannot be read",
					wantMetrics: mergeMetrics(
						ipsecTunnelStateMetrics("branch-a", "1", "active"),
					),
					wantMissing: []string{
						stateMetricKey("ipsec_gateway_state", "down", ipsecGatewayLabels("gw-branch-a", "ikev1")),
						stateMetricKey("ipsec_gateway_state", "up", ipsecGatewayLabels("gw-branch-a", "ikev1")),
					},
					wantLog: []string{"unexpected element <error>"},
				},
			},
		},
		"IPsec IKE SA query failure leaves gateway state as a gap": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.errors = map[string]error{ipsecIKESACommand: errors.New("transport failed")}
			},
			steps: []collectStep{
				{
					name: "gateways are not reported down when the SA list is unavailable",
					wantMetrics: mergeMetrics(
						ipsecTunnelStateMetrics("branch-a", "1", "active"),
					),
					wantMissing: []string{
						stateMetricKey("ipsec_gateway_state", "down", ipsecGatewayLabels("gw-branch-a", "ikev1")),
						stateMetricKey("ipsec_gateway_state", "up", ipsecGatewayLabels("gw-branch-a", "ikev1")),
					},
					wantLog: []string{"ipsec gateway metricset: IPsec IKE SA query API call: transport failed"},
				},
			},
		},
		"IPsec gateway query failure does not affect tunnel state": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.errors = map[string]error{ipsecGatewayCommand: errors.New("transport failed")}
			},
			steps: []collectStep{
				{
					name:        "tunnel state commits without gateway state",
					wantMetrics: ipsecTunnelStateMetrics("branch-a", "1", "active"),
					wantMissing: []string{
						stateMetricKey("ipsec_gateway_state", "up", ipsecGatewayLabels("gw-branch-a", "ikev1")),
					},
					wantLog:    []string{"ipsec gateway metricset: IPsec gateway query API call: transport failed"},
					notWantLog: []string{"IKE SA query"},
				},
			},
		},
		"IPsec flow entries with unrecognized or missing fields": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecFlowCommand: []byte(
						`<response status="success"><result><IPSec>` +
							`<entry><name>branch-a</name><id>1</id><state>future-state</state></entry>` +
							`<entry><name>branch-b</name><id>2</id></entry>` +
							`<entry><id>3</id><state>active</state></entry>` +
							`<entry><name>branch-d</name><state>Active</state></entry>` +
							`</IPSec></result></response>`,
					),
				}
			},
			steps: []collectStep{
				{
					name: "unrecognized state is unknown and incomplete entries are gaps",
					wantMetrics: mergeMetrics(
						ipsecTunnelStateMetrics("branch-a", "1", "unknown"),
						ipsecTunnelStateMetrics("branch-d", "unknown", "active"),
					),
					wantMissing: []string{
						stateMetricKey("ipsec_tunnel_state", "active", ipsecTunnelStateLabels("branch-b", "2")),
						stateMetricKey("ipsec_tunnel_state", "inactive", ipsecTunnelStateLabels("branch-b", "2")),
						stateMetricKey("ipsec_tunnel_state", "active", ipsecTunnelStateLabels("unknown", "3")),
					},
					wantLog: []string{
						"IPsec tunnel branch-b state: missing status",
						"IPsec flow entry: missing tunnel name",
					},
				},
			},
		},
		"IPsec gateway matching and IKE version": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecGatewayCommand: []byte(
						`<response status="success"><result><entries>` +
							`<entry><id> 1 </id><name> gw-branch-a </name><v1/></entry>` +
							`<entry><name>gw-branch-b</name><v2/></entry>` +
							`<entry><id>3</id><name>gw-branch-c</name><v1/><v2/></entry>` +
							`<entry><id>4</id><name>gw-branch-d</name></entry>` +
							`<entry><id>5</id></entry>` +
							`</entries></result></response>`,
					),
					ipsecIKESACommand: []byte(
						`<response status="success"><result>` +
							`<entry><gwid> 1 </gwid><name> renamed </name></entry>` +
							`<entry><gwid>1</gwid><name>renamed</name></entry>` +
							`<entry><name>gw-branch-b</name></entry>` +
							`</result></response>`,
					),
				}
			},
			steps: []collectStep{
				{
					name: "match by ID or by name, and only a single configured IKE version is reported",
					wantMetrics: mergeMetrics(
						ipsecGatewayStateMetrics("gw-branch-a", "ikev1", "up"),
						ipsecGatewayStateMetrics("gw-branch-b", "ikev2", "up"),
						ipsecGatewayStateMetrics("gw-branch-c", "unknown", "down"),
						ipsecGatewayStateMetrics("gw-branch-d", "unknown", "down"),
					),
					wantLog: []string{"IPsec gateway entry: missing gateway name"},
				},
			},
		},
		"missing IPsec flow and gateway payloads are partial success": {
			prepare: func(_ *Collector, api *mockAPIClient) {
				api.responses = map[string][]byte{
					ipsecFlowCommand:    []byte(`<response status="success"><result></result></response>`),
					ipsecGatewayCommand: []byte(`<response status="success"><result></result></response>`),
				}
			},
			steps: []collectStep{
				{
					name: "system commits and IPsec state metrics are absent",
					wantMetrics: map[string]metrix.SampleValue{
						metricKey("system_uptime", systemLabels()): 183845,
					},
					wantMissing: []string{
						stateMetricKey("ipsec_tunnel_state", "active", ipsecTunnelStateLabels("branch-a", "1")),
						stateMetricKey("ipsec_gateway_state", "up", ipsecGatewayLabels("gw-branch-a", "ikev1")),
					},
					wantLog: []string{
						"ipsec tunnel state metricset",
						"expected <IPSec>",
						"ipsec gateway metricset",
						"expected <entries>",
					},
				},
			},
		},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) { runCollectScenario(t, tc) })
	}
}
func TestCollector_IPSecInvalidIKESAIsGap(t *testing.T) {
	for name, body := range map[string]string{
		"missing result":    `<response status="success"/>`,
		"text result":       `<response status="success"><result>unexpected telemetry</result></response>`,
		"unexpected child":  `<response status="success"><result><error>query failed</error></result></response>`,
		"unidentified SA":   `<response status="success"><result><entry><role>Init</role></entry></result></response>`,
		"blank SA identity": `<response status="success"><result><entry><gwid> </gwid><name> </name></entry></result></response>`,
		"partially readable list": `<response status="success"><result>` +
			`<entry><gwid>1</gwid><name>gw-branch-a</name></entry><entry/>` +
			`</result></response>`,
	} {
		t.Run(name, func(t *testing.T) {
			c := New()
			var logs bytes.Buffer
			c.Logger = logger.NewWithWriter(&logs)
			api := &mockAPIClient{}
			c.apiClient = api
			// Establish a previous observation so a parse error also proves that stale values do not leak.
			_, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			api.responses = map[string][]byte{ipsecIKESACommand: []byte(body)}
			mx, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			assert.Empty(t, ipsecStateSeries(mx, "ipsec_gateway_state"))
			assert.NotEmpty(t, ipsecStateSeries(mx, "ipsec_tunnel_state"))
			assert.Contains(t, mx, metricKey("system_uptime", systemLabels()))
			assert.Contains(t, logs.String(), "ipsec gateway metricset")
		})
	}
}

func TestCollector_IPSecTunnelInit(t *testing.T) {
	c := New()
	c.apiClient = &mockAPIClient{
		responses: map[string][]byte{
			ipsecFlowCommand: []byte(`<response status="success"><result><IPSec>` +
				`<entry><name>branch-a</name><id>1</id><state>init</state></entry>` +
				`</IPSec></result></response>`),
		},
	}
	mx, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
	require.NoError(t, err)
	assert.Equal(t, map[string]metrix.SampleValue{
		stateMetricKey("ipsec_tunnel_state", "active", ipsecTunnelStateLabels("branch-a", "1")):   0,
		stateMetricKey("ipsec_tunnel_state", "inactive", ipsecTunnelStateLabels("branch-a", "1")): 0,
		stateMetricKey("ipsec_tunnel_state", "init", ipsecTunnelStateLabels("branch-a", "1")):     1,
		stateMetricKey("ipsec_tunnel_state", "unknown", ipsecTunnelStateLabels("branch-a", "1")):  0,
	}, ipsecStateSeries(mx, "ipsec_tunnel_state"))
}

func TestCollector_IPSecGatewayLifecycle(t *testing.T) {
	c := New()
	api := &mockAPIClient{}
	c.apiClient = api
	for _, step := range []struct {
		name      string
		responses map[string][]byte
		errors    map[string]error
		want      string
	}{
		{name: "initial observation", want: "up"},
		{name: "query failure", errors: map[string]error{ipsecIKESACommand: errors.New("transport failed")}},
		{name: "valid empty list", responses: map[string][]byte{ipsecIKESACommand: dataIPSecIKESAEmpty}, want: "down"},
		{name: "recovery", want: "up"},
		{name: "removed configuration", responses: map[string][]byte{ipsecGatewayCommand: []byte(`<response status="success"><result><entries/></result></response>`)}},
		{name: "configuration restored", want: "up"},
	} {
		t.Run(step.name, func(t *testing.T) {
			api.responses, api.errors, api.commands = step.responses, step.errors, nil
			mx, err := collecttest.CollectScalarSeries(c, metrix.ReadFlatten())
			require.NoError(t, err)
			states := ipsecStateSeries(mx, "ipsec_gateway_state")
			if step.want == "" {
				assert.Empty(t, states)
			} else {
				assert.Equal(t, mergeMetrics(
					ipsecGatewayStateMetrics("gw-branch-a", "ikev1", step.want),
					ipsecGatewayStateMetrics("gw-branch-b", "ikev1", step.want),
					ipsecGatewayStateMetrics("gw-branch-c", "ikev2", step.want),
					ipsecGatewayStateMetrics("gw-branch-d", "ikev2", step.want),
				), states)
			}
			assert.NotEmpty(t, ipsecStateSeries(mx, "ipsec_tunnel_state"))
			if step.name == "removed configuration" {
				assert.NotContains(t, api.commands, ipsecIKESACommand)
			}
		})
	}
}

func TestIPSecStateMetadata(t *testing.T) {
	body, err := os.ReadFile("metadata.yaml")
	require.NoError(t, err)
	module, err := collecttest.DecodeMetadataModule(body, "")
	require.NoError(t, err)
	charts, err := collecttest.ChartTemplateCharts(New().ChartTemplateYAML())
	require.NoError(t, err)
	stateCharts := make(map[string]charttpl.Chart)
	for _, name := range []string{"panos.ipsec.tunnel.state", "panos.ipsec.gateway.state"} {
		require.Contains(t, charts, name)
		stateCharts[name] = charts[name]
	}
	var metrics []collecttest.MetadataMetric
	for _, metric := range module.Metrics {
		if _, ok := stateCharts[metric.Name]; ok {
			metrics = append(metrics, metric)
		}
	}
	require.NoError(t, collecttest.CheckMetadataMetricsMatchCharts(metrics, stateCharts, map[string][]string{
		"ipsec_tunnel_state":  ipsecTunnelStates,
		"ipsec_gateway_state": ipsecGatewayStates,
	}))
}

func ipsecStateSeries(mx map[string]metrix.SampleValue, metric string) map[string]metrix.SampleValue {
	out := make(map[string]metrix.SampleValue)
	for key, value := range mx {
		if strings.HasPrefix(key, metric+"{") {
			out[key] = value
		}
	}
	return out
}

// Synthetic inventories measure the complete decode/join/write path as gateway count grows.
func BenchmarkCollectIPSecGatewayMetrics(b *testing.B) {
	for _, count := range []int{4, 256, 4096} {
		b.Run(fmt.Sprintf("gateways_%d", count), func(b *testing.B) {
			var gateways, sas strings.Builder
			gateways.WriteString(`<response status="success"><result><entries>`)
			sas.WriteString(`<response status="success"><result>`)
			for i := range count {
				fmt.Fprintf(&gateways, `<entry><id>%d</id><name>gateway-%d</name><v2/></entry>`, i+1, i+1)
				fmt.Fprintf(&sas, `<entry><gwid>%d</gwid><name>gateway-%d</name><created>Jan.01 00:00:00</created><expires>Jan.02 00:00:00</expires></entry>`, i+1, i+1)
			}
			gateways.WriteString(`</entries></result></response>`)
			sas.WriteString(`</result></response>`)
			c := New()
			api := &mockAPIClient{
				responses: map[string][]byte{
					ipsecGatewayCommand: []byte(gateways.String()),
					ipsecIKESACommand:   []byte(sas.String()),
				},
			}
			c.apiClient = api
			managed, ok := metrix.AsCycleManagedStore(c.store)
			require.True(b, ok)
			cycles := managed.CycleController()
			b.ReportAllocs()
			for b.Loop() {
				api.commands = api.commands[:0]
				cycles.BeginCycle()
				_, err := c.collectIPSecGatewayMetrics(context.Background())
				if err != nil {
					b.Fatal(err)
				}
				if err := cycles.CommitCycleSuccess(); err != nil {
					b.Fatal(err)
				}
			}
		})
	}
}
