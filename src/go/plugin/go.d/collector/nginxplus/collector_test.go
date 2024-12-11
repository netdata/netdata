// SPDX-License-Identifier: GPL-3.0-or-later

package nginxplus

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataAPI8APIVersions, _       = os.ReadFile("testdata/api-8/api_versions.json")
	dataAPI8Connections, _       = os.ReadFile("testdata/api-8/connections.json")
	dataAPI8EndpointsHTTP, _     = os.ReadFile("testdata/api-8/endpoints_http.json")
	dataAPI8EndpointsRoot, _     = os.ReadFile("testdata/api-8/endpoints_root.json")
	dataAPI8EndpointsStream, _   = os.ReadFile("testdata/api-8/endpoints_stream.json")
	dataAPI8HTTPCaches, _        = os.ReadFile("testdata/api-8/http_caches.json")
	dataAPI8HTTPLocationZones, _ = os.ReadFile("testdata/api-8/http_location_zones.json")
	dataAPI8HTTPRequests, _      = os.ReadFile("testdata/api-8/http_requests.json")
	dataAPI8HTTPServerZones, _   = os.ReadFile("testdata/api-8/http_server_zones.json")
	dataAPI8HTTPUpstreams, _     = os.ReadFile("testdata/api-8/http_upstreams.json")
	dataAPI8SSL, _               = os.ReadFile("testdata/api-8/ssl.json")
	dataAPI8StreamServerZones, _ = os.ReadFile("testdata/api-8/stream_server_zones.json")
	dataAPI8StreamUpstreams, _   = os.ReadFile("testdata/api-8/stream_upstreams.json")
	dataAPI8Resolvers, _         = os.ReadFile("testdata/api-8/resolvers.json")
	data404, _                   = os.ReadFile("testdata/404.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":            dataConfigJSON,
		"dataConfigYAML":            dataConfigYAML,
		"dataAPI8APIVersions":       dataAPI8APIVersions,
		"dataAPI8Connections":       dataAPI8Connections,
		"dataAPI8EndpointsHTTP":     dataAPI8EndpointsHTTP,
		"dataAPI8EndpointsRoot":     dataAPI8EndpointsRoot,
		"dataAPI8EndpointsStream":   dataAPI8EndpointsStream,
		"dataAPI8HTTPCaches":        dataAPI8HTTPCaches,
		"dataAPI8HTTPLocationZones": dataAPI8HTTPLocationZones,
		"dataAPI8HTTPRequests":      dataAPI8HTTPRequests,
		"dataAPI8HTTPServerZones":   dataAPI8HTTPServerZones,
		"dataAPI8HTTPUpstreams":     dataAPI8HTTPUpstreams,
		"dataAPI8SSL":               dataAPI8SSL,
		"dataAPI8StreamServerZones": dataAPI8StreamServerZones,
		"dataAPI8StreamUpstreams":   dataAPI8StreamUpstreams,
		"dataAPI8Resolvers":         dataAPI8Resolvers,
		"data404":                   data404,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		config   Config
	}{
		"success with default": {
			wantFail: false,
			config:   New().Config,
		},
		"fail when URL not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

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
		wantFail bool
		prepare  func(t *testing.T) (collr *Collector, cleanup func())
	}{
		"success when all requests OK": {
			wantFail: false,
			prepare:  caseAPI8AllRequestsOK,
		},
		"success when all requests except stream OK": {
			wantFail: false,
			prepare:  caseAPI8AllRequestsExceptStreamOK,
		},
		"fail on invalid data response": {
			wantFail: true,
			prepare:  caseInvalidDataResponse,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, collr.Check(context.Background()))
			} else {
				assert.NoError(t, collr.Check(context.Background()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (collr *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success when all requests OK": {
			prepare: caseAPI8AllRequestsOK,
			wantNumOfCharts: len(baseCharts) +
				len(httpCacheChartsTmpl) +
				len(httpServerZoneChartsTmpl) +
				len(httpLocationZoneChartsTmpl)*2 +
				len(httpUpstreamChartsTmpl) +
				len(httpUpstreamServerChartsTmpl)*2 +
				len(streamServerZoneChartsTmpl) +
				len(streamUpstreamChartsTmpl) +
				len(streamUpstreamServerChartsTmpl)*2 +
				len(resolverZoneChartsTmpl)*2,
			wantMetrics: map[string]int64{
				"connections_accepted":                                                                   6079,
				"connections_active":                                                                     1,
				"connections_dropped":                                                                    0,
				"connections_idle":                                                                       8,
				"http_cache_cache_backend_bypassed_bytes":                                                67035,
				"http_cache_cache_backend_bypassed_responses":                                            109,
				"http_cache_cache_backend_served_bytes":                                                  0,
				"http_cache_cache_backend_served_responses":                                              0,
				"http_cache_cache_backend_size":                                                          0,
				"http_cache_cache_backend_state_cold":                                                    0,
				"http_cache_cache_backend_state_warm":                                                    1,
				"http_cache_cache_backend_written_bytes":                                                 0,
				"http_cache_cache_backend_written_responses":                                             0,
				"http_location_zone_server_api_bytes_received":                                           1854427,
				"http_location_zone_server_api_bytes_sent":                                               4668778,
				"http_location_zone_server_api_requests":                                                 9188,
				"http_location_zone_server_api_requests_discarded":                                       0,
				"http_location_zone_server_api_responses":                                                9188,
				"http_location_zone_server_api_responses_1xx":                                            0,
				"http_location_zone_server_api_responses_2xx":                                            9187,
				"http_location_zone_server_api_responses_3xx":                                            0,
				"http_location_zone_server_api_responses_4xx":                                            1,
				"http_location_zone_server_api_responses_5xx":                                            0,
				"http_location_zone_server_dashboard_bytes_received":                                     0,
				"http_location_zone_server_dashboard_bytes_sent":                                         0,
				"http_location_zone_server_dashboard_requests":                                           0,
				"http_location_zone_server_dashboard_requests_discarded":                                 0,
				"http_location_zone_server_dashboard_responses":                                          0,
				"http_location_zone_server_dashboard_responses_1xx":                                      0,
				"http_location_zone_server_dashboard_responses_2xx":                                      0,
				"http_location_zone_server_dashboard_responses_3xx":                                      0,
				"http_location_zone_server_dashboard_responses_4xx":                                      0,
				"http_location_zone_server_dashboard_responses_5xx":                                      0,
				"http_requests_current":                                                                  1,
				"http_requests_total":                                                                    8363,
				"http_server_zone_server_backend_bytes_received":                                         1773834,
				"http_server_zone_server_backend_bytes_sent":                                             4585734,
				"http_server_zone_server_backend_requests":                                               8962,
				"http_server_zone_server_backend_requests_discarded":                                     0,
				"http_server_zone_server_backend_requests_processing":                                    1,
				"http_server_zone_server_backend_responses":                                              8961,
				"http_server_zone_server_backend_responses_1xx":                                          0,
				"http_server_zone_server_backend_responses_2xx":                                          8960,
				"http_server_zone_server_backend_responses_3xx":                                          0,
				"http_server_zone_server_backend_responses_4xx":                                          1,
				"http_server_zone_server_backend_responses_5xx":                                          0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_active":                     0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_bytes_received":             0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_bytes_sent":                 0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_downtime":                   1020,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_header_time":                0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_requests":                   26,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_response_time":              0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses":                  0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_1xx":              0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_2xx":              0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_3xx":              0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_4xx":              0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_5xx":              0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_checking":             0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_down":                 0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_draining":             0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_unavail":              1,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_unhealthy":            0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_up":                   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_active":                     0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_bytes_received":             86496,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_bytes_sent":                 9180,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_downtime":                   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_header_time":                1,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_requests":                   102,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_response_time":              1,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses":                  102,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_1xx":              0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_2xx":              102,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_3xx":              0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_4xx":              0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_5xx":              0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_checking":             0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_down":                 0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_draining":             0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_unavail":              0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_unhealthy":            0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_up":                   1,
				"http_upstream_backend_zone_http_backend_keepalive":                                      0,
				"http_upstream_backend_zone_http_backend_peers":                                          2,
				"http_upstream_backend_zone_http_backend_zombies":                                        0,
				"resolver_zone_resolver-http_requests_addr":                                              0,
				"resolver_zone_resolver-http_requests_name":                                              0,
				"resolver_zone_resolver-http_requests_srv":                                               2939408,
				"resolver_zone_resolver-http_responses_formerr":                                          0,
				"resolver_zone_resolver-http_responses_noerror":                                          0,
				"resolver_zone_resolver-http_responses_notimp":                                           0,
				"resolver_zone_resolver-http_responses_nxdomain":                                         2939404,
				"resolver_zone_resolver-http_responses_refused":                                          0,
				"resolver_zone_resolver-http_responses_servfail":                                         0,
				"resolver_zone_resolver-http_responses_timedout":                                         4,
				"resolver_zone_resolver-http_responses_unknown":                                          0,
				"resolver_zone_resolver-stream_requests_addr":                                            0,
				"resolver_zone_resolver-stream_requests_name":                                            638797,
				"resolver_zone_resolver-stream_requests_srv":                                             0,
				"resolver_zone_resolver-stream_responses_formerr":                                        0,
				"resolver_zone_resolver-stream_responses_noerror":                                        433136,
				"resolver_zone_resolver-stream_responses_notimp":                                         0,
				"resolver_zone_resolver-stream_responses_nxdomain":                                       40022,
				"resolver_zone_resolver-stream_responses_refused":                                        165639,
				"resolver_zone_resolver-stream_responses_servfail":                                       0,
				"resolver_zone_resolver-stream_responses_timedout":                                       0,
				"resolver_zone_resolver-stream_responses_unknown":                                        0,
				"ssl_handshake_timeout":                                                                  4,
				"ssl_handshakes":                                                                         15804607,
				"ssl_handshakes_failed":                                                                  37862,
				"ssl_no_common_cipher":                                                                   24,
				"ssl_no_common_protocol":                                                                 16648,
				"ssl_peer_rejected_cert":                                                                 0,
				"ssl_session_reuses":                                                                     13096060,
				"ssl_verify_failures_expired_cert":                                                       0,
				"ssl_verify_failures_hostname_mismatch":                                                  0,
				"ssl_verify_failures_other":                                                              0,
				"ssl_verify_failures_no_cert":                                                            0,
				"ssl_verify_failures_revoked_cert":                                                       0,
				"stream_server_zone_tcp_server_bytes_received":                                           0,
				"stream_server_zone_tcp_server_bytes_sent":                                               0,
				"stream_server_zone_tcp_server_connections":                                              0,
				"stream_server_zone_tcp_server_connections_discarded":                                    0,
				"stream_server_zone_tcp_server_connections_processing":                                   0,
				"stream_server_zone_tcp_server_sessions":                                                 0,
				"stream_server_zone_tcp_server_sessions_2xx":                                             0,
				"stream_server_zone_tcp_server_sessions_4xx":                                             0,
				"stream_server_zone_tcp_server_sessions_5xx":                                             0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_active":          0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_bytes_received":  0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_bytes_sent":      0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_connections":     0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_downtime":        0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_state_checking":  0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_state_down":      0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_state_unavail":   0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_state_unhealthy": 0,
				"stream_upstream_stream_backend_server_127.0.0.1:12346_zone_tcp_servers_state_up":        1,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_active":          0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_bytes_received":  0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_bytes_sent":      0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_connections":     0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_downtime":        0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_state_checking":  0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_state_down":      0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_state_unavail":   0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_state_unhealthy": 0,
				"stream_upstream_stream_backend_server_127.0.0.1:12347_zone_tcp_servers_state_up":        1,
				"stream_upstream_stream_backend_zone_tcp_servers_peers":                                  2,
				"stream_upstream_stream_backend_zone_tcp_servers_zombies":                                0,
			},
		},
		"success when all requests except stream OK": {
			prepare: caseAPI8AllRequestsExceptStreamOK,
			wantNumOfCharts: len(baseCharts) +
				len(httpCacheChartsTmpl) +
				len(httpServerZoneChartsTmpl) +
				len(httpLocationZoneChartsTmpl)*2 +
				len(httpUpstreamChartsTmpl) +
				len(httpUpstreamServerChartsTmpl)*2 +
				len(resolverZoneChartsTmpl)*2,
			wantMetrics: map[string]int64{
				"connections_accepted":                                                        6079,
				"connections_active":                                                          1,
				"connections_dropped":                                                         0,
				"connections_idle":                                                            8,
				"http_cache_cache_backend_bypassed_bytes":                                     67035,
				"http_cache_cache_backend_bypassed_responses":                                 109,
				"http_cache_cache_backend_served_bytes":                                       0,
				"http_cache_cache_backend_served_responses":                                   0,
				"http_cache_cache_backend_size":                                               0,
				"http_cache_cache_backend_state_cold":                                         0,
				"http_cache_cache_backend_state_warm":                                         1,
				"http_cache_cache_backend_written_bytes":                                      0,
				"http_cache_cache_backend_written_responses":                                  0,
				"http_location_zone_server_api_bytes_received":                                1854427,
				"http_location_zone_server_api_bytes_sent":                                    4668778,
				"http_location_zone_server_api_requests":                                      9188,
				"http_location_zone_server_api_requests_discarded":                            0,
				"http_location_zone_server_api_responses":                                     9188,
				"http_location_zone_server_api_responses_1xx":                                 0,
				"http_location_zone_server_api_responses_2xx":                                 9187,
				"http_location_zone_server_api_responses_3xx":                                 0,
				"http_location_zone_server_api_responses_4xx":                                 1,
				"http_location_zone_server_api_responses_5xx":                                 0,
				"http_location_zone_server_dashboard_bytes_received":                          0,
				"http_location_zone_server_dashboard_bytes_sent":                              0,
				"http_location_zone_server_dashboard_requests":                                0,
				"http_location_zone_server_dashboard_requests_discarded":                      0,
				"http_location_zone_server_dashboard_responses":                               0,
				"http_location_zone_server_dashboard_responses_1xx":                           0,
				"http_location_zone_server_dashboard_responses_2xx":                           0,
				"http_location_zone_server_dashboard_responses_3xx":                           0,
				"http_location_zone_server_dashboard_responses_4xx":                           0,
				"http_location_zone_server_dashboard_responses_5xx":                           0,
				"http_requests_current":                                                       1,
				"http_requests_total":                                                         8363,
				"http_server_zone_server_backend_bytes_received":                              1773834,
				"http_server_zone_server_backend_bytes_sent":                                  4585734,
				"http_server_zone_server_backend_requests":                                    8962,
				"http_server_zone_server_backend_requests_discarded":                          0,
				"http_server_zone_server_backend_requests_processing":                         1,
				"http_server_zone_server_backend_responses":                                   8961,
				"http_server_zone_server_backend_responses_1xx":                               0,
				"http_server_zone_server_backend_responses_2xx":                               8960,
				"http_server_zone_server_backend_responses_3xx":                               0,
				"http_server_zone_server_backend_responses_4xx":                               1,
				"http_server_zone_server_backend_responses_5xx":                               0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_active":          0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_bytes_received":  0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_bytes_sent":      0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_downtime":        1020,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_header_time":     0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_requests":        26,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_response_time":   0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses":       0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_1xx":   0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_2xx":   0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_3xx":   0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_4xx":   0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_responses_5xx":   0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_checking":  0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_down":      0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_draining":  0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_unavail":   1,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_unhealthy": 0,
				"http_upstream_backend_server_127.0.0.1:81_zone_http_backend_state_up":        0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_active":          0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_bytes_received":  86496,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_bytes_sent":      9180,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_downtime":        0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_header_time":     1,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_requests":        102,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_response_time":   1,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses":       102,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_1xx":   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_2xx":   102,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_3xx":   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_4xx":   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_responses_5xx":   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_checking":  0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_down":      0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_draining":  0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_unavail":   0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_unhealthy": 0,
				"http_upstream_backend_server_127.0.0.1:82_zone_http_backend_state_up":        1,
				"http_upstream_backend_zone_http_backend_keepalive":                           0,
				"http_upstream_backend_zone_http_backend_peers":                               2,
				"http_upstream_backend_zone_http_backend_zombies":                             0,
				"resolver_zone_resolver-http_requests_addr":                                   0,
				"resolver_zone_resolver-http_requests_name":                                   0,
				"resolver_zone_resolver-http_requests_srv":                                    2939408,
				"resolver_zone_resolver-http_responses_formerr":                               0,
				"resolver_zone_resolver-http_responses_noerror":                               0,
				"resolver_zone_resolver-http_responses_notimp":                                0,
				"resolver_zone_resolver-http_responses_nxdomain":                              2939404,
				"resolver_zone_resolver-http_responses_refused":                               0,
				"resolver_zone_resolver-http_responses_servfail":                              0,
				"resolver_zone_resolver-http_responses_timedout":                              4,
				"resolver_zone_resolver-http_responses_unknown":                               0,
				"resolver_zone_resolver-stream_requests_addr":                                 0,
				"resolver_zone_resolver-stream_requests_name":                                 638797,
				"resolver_zone_resolver-stream_requests_srv":                                  0,
				"resolver_zone_resolver-stream_responses_formerr":                             0,
				"resolver_zone_resolver-stream_responses_noerror":                             433136,
				"resolver_zone_resolver-stream_responses_notimp":                              0,
				"resolver_zone_resolver-stream_responses_nxdomain":                            40022,
				"resolver_zone_resolver-stream_responses_refused":                             165639,
				"resolver_zone_resolver-stream_responses_servfail":                            0,
				"resolver_zone_resolver-stream_responses_timedout":                            0,
				"resolver_zone_resolver-stream_responses_unknown":                             0,
				"ssl_handshake_timeout":                                                       4,
				"ssl_handshakes":                                                              15804607,
				"ssl_handshakes_failed":                                                       37862,
				"ssl_no_common_cipher":                                                        24,
				"ssl_no_common_protocol":                                                      16648,
				"ssl_peer_rejected_cert":                                                      0,
				"ssl_session_reuses":                                                          13096060,
				"ssl_verify_failures_expired_cert":                                            0,
				"ssl_verify_failures_hostname_mismatch":                                       0,
				"ssl_verify_failures_other":                                                   0,
				"ssl_verify_failures_no_cert":                                                 0,
				"ssl_verify_failures_revoked_cert":                                            0,
			},
		},
		"fail on invalid data response": {
			prepare:         caseInvalidDataResponse,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on connection refused": {
			prepare:         caseConnectionRefused,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equalf(t, test.wantNumOfCharts, len(*collr.Charts()), "number of charts")
				module.TestMetricsHasAllChartsDimsSkip(t, collr.Charts(), mx, func(chart *module.Chart, _ *module.Dim) bool {
					return chart.ID == uptimeChart.ID
				})
			}
		})
	}
}

func caseAPI8AllRequestsOK(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathAPIVersions:
				_, _ = w.Write(dataAPI8APIVersions)
			case fmt.Sprintf(urlPathAPIEndpointsRoot, 8):
				_, _ = w.Write(dataAPI8EndpointsRoot)
			case fmt.Sprintf(urlPathAPIEndpointsHTTP, 8):
				_, _ = w.Write(dataAPI8EndpointsHTTP)
			case fmt.Sprintf(urlPathAPIEndpointsStream, 8):
				_, _ = w.Write(dataAPI8EndpointsStream)
			case fmt.Sprintf(urlPathAPIConnections, 8):
				_, _ = w.Write(dataAPI8Connections)
			case fmt.Sprintf(urlPathAPISSL, 8):
				_, _ = w.Write(dataAPI8SSL)
			case fmt.Sprintf(urlPathAPIHTTPRequests, 8):
				_, _ = w.Write(dataAPI8HTTPRequests)
			case fmt.Sprintf(urlPathAPIHTTPServerZones, 8):
				_, _ = w.Write(dataAPI8HTTPServerZones)
			case fmt.Sprintf(urlPathAPIHTTPLocationZones, 8):
				_, _ = w.Write(dataAPI8HTTPLocationZones)
			case fmt.Sprintf(urlPathAPIHTTPUpstreams, 8):
				_, _ = w.Write(dataAPI8HTTPUpstreams)
			case fmt.Sprintf(urlPathAPIHTTPCaches, 8):
				_, _ = w.Write(dataAPI8HTTPCaches)
			case fmt.Sprintf(urlPathAPIStreamServerZones, 8):
				_, _ = w.Write(dataAPI8StreamServerZones)
			case fmt.Sprintf(urlPathAPIStreamUpstreams, 8):
				_, _ = w.Write(dataAPI8StreamUpstreams)
			case fmt.Sprintf(urlPathAPIResolvers, 8):
				_, _ = w.Write(dataAPI8Resolvers)
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(data404)

			}
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseAPI8AllRequestsExceptStreamOK(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathAPIVersions:
				_, _ = w.Write(dataAPI8APIVersions)
			case fmt.Sprintf(urlPathAPIEndpointsRoot, 8):
				_, _ = w.Write(dataAPI8EndpointsRoot)
			case fmt.Sprintf(urlPathAPIEndpointsHTTP, 8):
				_, _ = w.Write(dataAPI8EndpointsHTTP)
			case fmt.Sprintf(urlPathAPIEndpointsStream, 8):
				_, _ = w.Write(dataAPI8EndpointsStream)
			case fmt.Sprintf(urlPathAPIConnections, 8):
				_, _ = w.Write(dataAPI8Connections)
			case fmt.Sprintf(urlPathAPISSL, 8):
				_, _ = w.Write(dataAPI8SSL)
			case fmt.Sprintf(urlPathAPIHTTPRequests, 8):
				_, _ = w.Write(dataAPI8HTTPRequests)
			case fmt.Sprintf(urlPathAPIHTTPServerZones, 8):
				_, _ = w.Write(dataAPI8HTTPServerZones)
			case fmt.Sprintf(urlPathAPIHTTPLocationZones, 8):
				_, _ = w.Write(dataAPI8HTTPLocationZones)
			case fmt.Sprintf(urlPathAPIHTTPUpstreams, 8):
				_, _ = w.Write(dataAPI8HTTPUpstreams)
			case fmt.Sprintf(urlPathAPIHTTPCaches, 8):
				_, _ = w.Write(dataAPI8HTTPCaches)
			case fmt.Sprintf(urlPathAPIResolvers, 8):
				_, _ = w.Write(dataAPI8Resolvers)
			default:
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write(data404)

			}
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseInvalidDataResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}
