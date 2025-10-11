// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer2310Metrics, _ = os.ReadFile("testdata/v2.3.10/metrics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataVer2310Metrics": dataVer2310Metrics,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'url'": {
			wantFail: true,
			config: Config{HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{},
			}},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					ClientConfig: web.ClientConfig{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				}},
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (h *Collector, cleanup func())
	}{
		"success on valid response v2.3.1": {
			wantFail: false,
			prepare:  prepareCaseHaproxyV231Metrics,
		},
		"fails on response with unexpected metrics (not HAProxy)": {
			wantFail: true,
			prepare:  prepareCaseNotHaproxyMetrics,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  prepareCase404Response,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareCaseConnectionRefused,
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
		prepare       func(t *testing.T) (c *Collector, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid response v2.3.1": {
			prepare: prepareCaseHaproxyV231Metrics,
			wantCollected: map[string]int64{
				"haproxy_backend_bytes_in_proxy_proxy1":              21057046294,
				"haproxy_backend_bytes_in_proxy_proxy2":              2493759083896,
				"haproxy_backend_bytes_out_proxy_proxy1":             41352782609,
				"haproxy_backend_bytes_out_proxy_proxy2":             5131407558,
				"haproxy_backend_current_queue_proxy_proxy1":         1,
				"haproxy_backend_current_queue_proxy_proxy2":         1,
				"haproxy_backend_current_sessions_proxy_proxy1":      1,
				"haproxy_backend_current_sessions_proxy_proxy2":      1322,
				"haproxy_backend_http_responses_1xx_proxy_proxy1":    1,
				"haproxy_backend_http_responses_1xx_proxy_proxy2":    4130401,
				"haproxy_backend_http_responses_2xx_proxy_proxy1":    21338013,
				"haproxy_backend_http_responses_2xx_proxy_proxy2":    1,
				"haproxy_backend_http_responses_3xx_proxy_proxy1":    10004,
				"haproxy_backend_http_responses_3xx_proxy_proxy2":    1,
				"haproxy_backend_http_responses_4xx_proxy_proxy1":    10170758,
				"haproxy_backend_http_responses_4xx_proxy_proxy2":    1,
				"haproxy_backend_http_responses_5xx_proxy_proxy1":    3075,
				"haproxy_backend_http_responses_5xx_proxy_proxy2":    1,
				"haproxy_backend_http_responses_other_proxy_proxy1":  5657,
				"haproxy_backend_http_responses_other_proxy_proxy2":  1,
				"haproxy_backend_queue_time_average_proxy_proxy1":    0,
				"haproxy_backend_queue_time_average_proxy_proxy2":    0,
				"haproxy_backend_response_time_average_proxy_proxy1": 52,
				"haproxy_backend_response_time_average_proxy_proxy2": 1,
				"haproxy_backend_sessions_proxy_proxy1":              31527507,
				"haproxy_backend_sessions_proxy_proxy2":              4131723,
			},
		},
		"fails on response with unexpected metrics (not HAProxy)": {
			prepare: prepareCaseNotHaproxyMetrics,
		},
		"fails on 404 response": {
			prepare: prepareCase404Response,
		},
		"fails on connection refused": {
			prepare: prepareCaseConnectionRefused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareCaseHaproxyV231Metrics(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer2310Metrics)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCaseNotHaproxyMetrics(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`
# HELP haproxy_backend_http_responses_total Total number of HTTP responses.
# TYPE haproxy_backend_http_responses_total counter
application_backend_http_responses_total{proxy="infra-traefik-web",code="1xx"} 0
application_backend_http_responses_total{proxy="infra-vernemq-ws",code="1xx"} 4130401
application_backend_http_responses_total{proxy="infra-traefik-web",code="2xx"} 21338013
application_backend_http_responses_total{proxy="infra-vernemq-ws",code="2xx"} 0
application_backend_http_responses_total{proxy="infra-traefik-web",code="3xx"} 10004
application_backend_http_responses_total{proxy="infra-vernemq-ws",code="3xx"} 0
application_backend_http_responses_total{proxy="infra-traefik-web",code="4xx"} 10170758
application_backend_http_responses_total{proxy="infra-vernemq-ws",code="4xx"} 0
application_backend_http_responses_total{proxy="infra-traefik-web",code="5xx"} 3075
application_backend_http_responses_total{proxy="infra-vernemq-ws",code="5xx"} 0
application_backend_http_responses_total{proxy="infra-traefik-web",code="other"} 5657
application_backend_http_responses_total{proxy="infra-vernemq-ws",code="other"} 0
`))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCase404Response(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:38001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}
