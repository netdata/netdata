// SPDX-License-Identifier: GPL-3.0-or-later

package nats

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer210HealthzOk, _ = os.ReadFile("testdata/v2.10.24/healthz-ok.json")
	dataVer210Varz, _      = os.ReadFile("testdata/v2.10.24/varz.json")
	dataVer210Accstatz, _  = os.ReadFile("testdata/v2.10.24/accstatz.json")
	dataVer210Routez, _    = os.ReadFile("testdata/v2.10.24/routez.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"dataVer210HealthzOk": dataVer210HealthzOk,
		"dataVer210Varz":      dataVer210Varz,
		"dataVer210Accstatz":  dataVer210Accstatz,
		"dataVer210Routez":    dataVer210Routez,
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
		prepare  func(t *testing.T) (nu *Collector, cleanup func())
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  caseOk,
		},
		"fail on unexpected JSON response": {
			wantFail: true,
			prepare:  caseUnexpectedJsonResponse,
		},
		"fail on invalid data response": {
			wantFail: true,
			prepare:  caseInvalidDataResponse,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
		"fail on 404 response": {
			wantFail: true,
			prepare:  case404,
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (nu *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success on valid response": {
			prepare: caseOk,
			wantNumOfCharts: len(serverCharts) +
				len(accountChartsTmpl)*3 +
				len(routeChartsTmpl)*1,
			wantMetrics: map[string]int64{
				"accstatz_acc_$G_conns":               0,
				"accstatz_acc_$G_leaf_nodes":          0,
				"accstatz_acc_$G_num_subs":            5,
				"accstatz_acc_$G_received_bytes":      0,
				"accstatz_acc_$G_received_msgs":       0,
				"accstatz_acc_$G_sent_bytes":          0,
				"accstatz_acc_$G_sent_msgs":           0,
				"accstatz_acc_$G_slow_consumers":      0,
				"accstatz_acc_$G_total_conns":         0,
				"accstatz_acc_$SYS_conns":             0,
				"accstatz_acc_$SYS_leaf_nodes":        0,
				"accstatz_acc_$SYS_num_subs":          220,
				"accstatz_acc_$SYS_received_bytes":    0,
				"accstatz_acc_$SYS_received_msgs":     0,
				"accstatz_acc_$SYS_sent_bytes":        0,
				"accstatz_acc_$SYS_sent_msgs":         0,
				"accstatz_acc_$SYS_slow_consumers":    0,
				"accstatz_acc_$SYS_total_conns":       0,
				"accstatz_acc_default_conns":          44,
				"accstatz_acc_default_leaf_nodes":     0,
				"accstatz_acc_default_num_subs":       1133,
				"accstatz_acc_default_received_bytes": 62023455,
				"accstatz_acc_default_received_msgs":  916392,
				"accstatz_acc_default_sent_bytes":     529749990,
				"accstatz_acc_default_sent_msgs":      2546732,
				"accstatz_acc_default_slow_consumers": 1,
				"accstatz_acc_default_total_conns":    44,
				"routez_route_id_1_in_bytes":          4,
				"routez_route_id_1_in_msgs":           1,
				"routez_route_id_1_num_subs":          1,
				"routez_route_id_1_out_bytes":         4,
				"routez_route_id_1_out_msgs":          1,
				"varz_http_endpoint_/_req":            5710,
				"varz_http_endpoint_/accountz_req":    2201,
				"varz_http_endpoint_/accstatz_req":    6,
				"varz_http_endpoint_/connz_req":       3649,
				"varz_http_endpoint_/gatewayz_req":    2204,
				"varz_http_endpoint_/healthz_req":     3430,
				"varz_http_endpoint_/ipqueuesz_req":   0,
				"varz_http_endpoint_/jsz_req":         2958,
				"varz_http_endpoint_/leafz_req":       9,
				"varz_http_endpoint_/raftz_req":       0,
				"varz_http_endpoint_/routez_req":      2202,
				"varz_http_endpoint_/stacksz_req":     0,
				"varz_http_endpoint_/subsz_req":       4412,
				"varz_http_endpoint_/varz_req":        7114,
				"varz_srv_connections":                44,
				"varz_srv_cpu":                        10,
				"varz_srv_healthz_status_error":       0,
				"varz_srv_healthz_status_ok":          1,
				"varz_srv_in_bytes":                   62024985,
				"varz_srv_in_msgs":                    916475,
				"varz_srv_mem":                        95731712,
				"varz_srv_out_bytes":                  529775656,
				"varz_srv_out_msgs":                   2546840,
				"varz_srv_remotes":                    0,
				"varz_srv_routes":                     0,
				"varz_srv_slow_consumers":             1,
				"varz_srv_subscriptions":              1358,
				"varz_srv_total_connections":          74932,
				"varz_srv_uptime":                     339394,
			},
		},
		"fail on unexpected JSON response": {
			prepare:     caseUnexpectedJsonResponse,
			wantMetrics: nil,
		},
		"fail on invalid data response": {
			prepare:     caseInvalidDataResponse,
			wantMetrics: nil,
		},
		"fail on connection refused": {
			prepare:     caseConnectionRefused,
			wantMetrics: nil,
		},
		"fail on 404 response": {
			prepare:     case404,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			_ = collr.Check(context.Background())

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()), "want charts")

				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func caseOk(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathHealthz:
				_, _ = w.Write(dataVer210HealthzOk)
			case urlPathVarz:
				_, _ = w.Write(dataVer210Varz)
			case urlPathAccstatz:
				if r.URL.RawQuery != urlQueryAccstatz {
					w.WriteHeader(http.StatusNotFound)
					return
				}
				_, _ = w.Write(dataVer210Accstatz)
			case urlPathRoutez:
				_, _ = w.Write(dataVer210Routez)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseUnexpectedJsonResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	resp := `
{
    "elephant": {
        "burn": false,
        "mountain": true,
        "fog": false,
        "skin": -1561907625,
        "burst": "anyway",
        "shadow": 1558616893
    },
    "start": "ever",
    "base": 2093056027,
    "mission": -2007590351,
    "victory": 999053756,
    "die": false
}
`
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(resp))
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

func case404(t *testing.T) (*Collector, func()) {
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
