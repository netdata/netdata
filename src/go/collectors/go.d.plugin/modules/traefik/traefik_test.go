// SPDX-License-Identifier: GPL-3.0-or-later

package traefik

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	v221Metrics, _ = os.ReadFile("testdata/v2.2.1/metrics.txt")
)

func Test_Testdata(t *testing.T) {
	for name, data := range map[string][]byte{
		"v2.2.1_Metrics": v221Metrics,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Traefik)(nil), New())
}

func TestTraefik_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'url'": {
			wantFail: true,
			config: Config{HTTP: web.HTTP{
				Request: web.Request{},
			}},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Client: web.Client{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				}},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			rdb := New()
			rdb.Config = test.config

			if test.wantFail {
				assert.False(t, rdb.Init())
			} else {
				assert.True(t, rdb.Init())
			}
		})
	}
}

func TestTraefik_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestTraefik_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestTraefik_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (tk *Traefik, cleanup func())
	}{
		"success on valid response v2.3.1": {
			wantFail: false,
			prepare:  prepareCaseTraefikV221Metrics,
		},
		"fails on response with unexpected metrics (not HAProxy)": {
			wantFail: true,
			prepare:  prepareCaseNotTraefikMetrics,
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
			tk, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.False(t, tk.Check())
			} else {
				assert.True(t, tk.Check())
			}
		})
	}
}

func TestTraefik_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) (tk *Traefik, cleanup func())
		wantCollected []map[string]int64
	}{
		"success on valid response v2.2.1": {
			prepare: prepareCaseTraefikV221Metrics,
			wantCollected: []map[string]int64{
				{
					"entrypoint_open_connections_traefik_http_GET":          1,
					"entrypoint_open_connections_web_http_DELETE":           0,
					"entrypoint_open_connections_web_http_GET":              0,
					"entrypoint_open_connections_web_http_HEAD":             0,
					"entrypoint_open_connections_web_http_OPTIONS":          0,
					"entrypoint_open_connections_web_http_PATCH":            0,
					"entrypoint_open_connections_web_http_POST":             4,
					"entrypoint_open_connections_web_http_PUT":              0,
					"entrypoint_open_connections_web_websocket_GET":         0,
					"entrypoint_request_duration_average_traefik_http_1xx":  0,
					"entrypoint_request_duration_average_traefik_http_2xx":  0,
					"entrypoint_request_duration_average_traefik_http_3xx":  0,
					"entrypoint_request_duration_average_traefik_http_4xx":  0,
					"entrypoint_request_duration_average_traefik_http_5xx":  0,
					"entrypoint_request_duration_average_web_http_1xx":      0,
					"entrypoint_request_duration_average_web_http_2xx":      0,
					"entrypoint_request_duration_average_web_http_3xx":      0,
					"entrypoint_request_duration_average_web_http_4xx":      0,
					"entrypoint_request_duration_average_web_http_5xx":      0,
					"entrypoint_request_duration_average_web_websocket_1xx": 0,
					"entrypoint_request_duration_average_web_websocket_2xx": 0,
					"entrypoint_request_duration_average_web_websocket_3xx": 0,
					"entrypoint_request_duration_average_web_websocket_4xx": 0,
					"entrypoint_request_duration_average_web_websocket_5xx": 0,
					"entrypoint_requests_traefik_http_1xx":                  0,
					"entrypoint_requests_traefik_http_2xx":                  2840814,
					"entrypoint_requests_traefik_http_3xx":                  0,
					"entrypoint_requests_traefik_http_4xx":                  8,
					"entrypoint_requests_traefik_http_5xx":                  0,
					"entrypoint_requests_web_http_1xx":                      0,
					"entrypoint_requests_web_http_2xx":                      1036208982,
					"entrypoint_requests_web_http_3xx":                      416262,
					"entrypoint_requests_web_http_4xx":                      267591379,
					"entrypoint_requests_web_http_5xx":                      223136,
					"entrypoint_requests_web_websocket_1xx":                 0,
					"entrypoint_requests_web_websocket_2xx":                 0,
					"entrypoint_requests_web_websocket_3xx":                 0,
					"entrypoint_requests_web_websocket_4xx":                 79137,
					"entrypoint_requests_web_websocket_5xx":                 0,
				},
			},
		},
		"properly calculating entrypoint request duration delta": {
			prepare: prepareCaseTraefikEntrypointRequestDuration,
			wantCollected: []map[string]int64{
				{
					"entrypoint_request_duration_average_traefik_http_1xx":  0,
					"entrypoint_request_duration_average_traefik_http_2xx":  0,
					"entrypoint_request_duration_average_traefik_http_3xx":  0,
					"entrypoint_request_duration_average_traefik_http_4xx":  0,
					"entrypoint_request_duration_average_traefik_http_5xx":  0,
					"entrypoint_request_duration_average_web_websocket_1xx": 0,
					"entrypoint_request_duration_average_web_websocket_2xx": 0,
					"entrypoint_request_duration_average_web_websocket_3xx": 0,
					"entrypoint_request_duration_average_web_websocket_4xx": 0,
					"entrypoint_request_duration_average_web_websocket_5xx": 0,
				},
				{
					"entrypoint_request_duration_average_traefik_http_1xx":  0,
					"entrypoint_request_duration_average_traefik_http_2xx":  500,
					"entrypoint_request_duration_average_traefik_http_3xx":  0,
					"entrypoint_request_duration_average_traefik_http_4xx":  0,
					"entrypoint_request_duration_average_traefik_http_5xx":  0,
					"entrypoint_request_duration_average_web_websocket_1xx": 0,
					"entrypoint_request_duration_average_web_websocket_2xx": 0,
					"entrypoint_request_duration_average_web_websocket_3xx": 250,
					"entrypoint_request_duration_average_web_websocket_4xx": 0,
					"entrypoint_request_duration_average_web_websocket_5xx": 0,
				},
				{
					"entrypoint_request_duration_average_traefik_http_1xx":  0,
					"entrypoint_request_duration_average_traefik_http_2xx":  1000,
					"entrypoint_request_duration_average_traefik_http_3xx":  0,
					"entrypoint_request_duration_average_traefik_http_4xx":  0,
					"entrypoint_request_duration_average_traefik_http_5xx":  0,
					"entrypoint_request_duration_average_web_websocket_1xx": 0,
					"entrypoint_request_duration_average_web_websocket_2xx": 0,
					"entrypoint_request_duration_average_web_websocket_3xx": 500,
					"entrypoint_request_duration_average_web_websocket_4xx": 0,
					"entrypoint_request_duration_average_web_websocket_5xx": 0,
				},
				{
					"entrypoint_request_duration_average_traefik_http_1xx":  0,
					"entrypoint_request_duration_average_traefik_http_2xx":  0,
					"entrypoint_request_duration_average_traefik_http_3xx":  0,
					"entrypoint_request_duration_average_traefik_http_4xx":  0,
					"entrypoint_request_duration_average_traefik_http_5xx":  0,
					"entrypoint_request_duration_average_web_websocket_1xx": 0,
					"entrypoint_request_duration_average_web_websocket_2xx": 0,
					"entrypoint_request_duration_average_web_websocket_3xx": 0,
					"entrypoint_request_duration_average_web_websocket_4xx": 0,
					"entrypoint_request_duration_average_web_websocket_5xx": 0,
				},
			},
		},
		"fails on response with unexpected metrics (not Traefik)": {
			prepare: prepareCaseNotTraefikMetrics,
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
			tk, cleanup := test.prepare(t)
			defer cleanup()

			var ms map[string]int64
			for _, want := range test.wantCollected {
				ms = tk.Collect()
				assert.Equal(t, want, ms)
			}
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, tk, ms)
			}
		})
	}
}

func prepareCaseTraefikV221Metrics(t *testing.T) (*Traefik, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(v221Metrics)
		}))
	h := New()
	h.URL = srv.URL
	require.True(t, h.Init())

	return h, srv.Close
}

func prepareCaseTraefikEntrypointRequestDuration(t *testing.T) (*Traefik, func()) {
	t.Helper()
	var num int
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			num++
			switch num {
			case 1:
				_, _ = w.Write([]byte(`
traefik_entrypoint_request_duration_seconds_sum{code="200",entrypoint="traefik",method="GET",protocol="http"} 10.1
traefik_entrypoint_request_duration_seconds_sum{code="300",entrypoint="web",method="GET",protocol="websocket"} 20.1
traefik_entrypoint_request_duration_seconds_count{code="200",entrypoint="traefik",method="PUT",protocol="http"} 30
traefik_entrypoint_request_duration_seconds_count{code="300",entrypoint="web",method="PUT",protocol="websocket"} 40
`))
			case 2:
				_, _ = w.Write([]byte(`
traefik_entrypoint_request_duration_seconds_sum{code="200",entrypoint="traefik",method="GET",protocol="http"} 15.1
traefik_entrypoint_request_duration_seconds_sum{code="300",entrypoint="web",method="GET",protocol="websocket"} 25.1
traefik_entrypoint_request_duration_seconds_count{code="200",entrypoint="traefik",method="PUT",protocol="http"} 40
traefik_entrypoint_request_duration_seconds_count{code="300",entrypoint="web",method="PUT",protocol="websocket"} 60
`))
			default:
				_, _ = w.Write([]byte(`
traefik_entrypoint_request_duration_seconds_sum{code="200",entrypoint="traefik",method="GET",protocol="http"} 25.1
traefik_entrypoint_request_duration_seconds_sum{code="300",entrypoint="web",method="GET",protocol="websocket"} 35.1
traefik_entrypoint_request_duration_seconds_count{code="200",entrypoint="traefik",method="PUT",protocol="http"} 50
traefik_entrypoint_request_duration_seconds_count{code="300",entrypoint="web",method="PUT",protocol="websocket"} 80
`))
			}
		}))
	h := New()
	h.URL = srv.URL
	require.True(t, h.Init())

	return h, srv.Close
}

func prepareCaseNotTraefikMetrics(t *testing.T) (*Traefik, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`
# HELP application_backend_http_responses_total Total number of HTTP responses.
# TYPE application_backend_http_responses_total counter
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
	h := New()
	h.URL = srv.URL
	require.True(t, h.Init())

	return h, srv.Close
}

func prepareCase404Response(t *testing.T) (*Traefik, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	h := New()
	h.URL = srv.URL
	require.True(t, h.Init())

	return h, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*Traefik, func()) {
	t.Helper()
	h := New()
	h.URL = "http://127.0.0.1:38001"
	require.True(t, h.Init())

	return h, func() {}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, tk *Traefik, ms map[string]int64) {
	for _, chart := range *tk.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := ms[dim.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := ms[v.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", v.ID, chart.ID)
		}
	}
}
