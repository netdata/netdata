// SPDX-License-Identifier: GPL-3.0-or-later

package typesense

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const testApiKey = "XYZ"

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer27HealthOk, _  = os.ReadFile("testdata/v27.0/health_ok.json")
	dataVer27HealthNok, _ = os.ReadFile("testdata/v27.0/health_nok.json")
	dataVer27Stats, _     = os.ReadFile("testdata/v27.0/stats.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataVer27HealthOk":  dataVer27HealthOk,
		"dataVer27HealthNok": dataVer27HealthNok,
		"dataVer27Stats":     dataVer27Stats,
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
		"success with valid API key": {
			wantFail: false,
			prepare:  caseOk,
		},
		"success without API key": {
			wantFail: false,
			prepare:  caseOkNoApiKey,
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
		prepare         func(t *testing.T) (collr *Collector, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success with valid API key": {
			prepare:         caseOk,
			wantNumOfCharts: len(baseCharts) + len(statsCharts),
			wantMetrics: map[string]int64{
				"delete_latency_ms":              1,
				"delete_requests_per_second":     1100,
				"health_status_ok":               1,
				"health_status_out_of_disk":      0,
				"health_status_out_of_memory":    0,
				"import_latency_ms":              1,
				"import_requests_per_second":     1100,
				"overloaded_requests_per_second": 1100,
				"pending_write_batches":          1,
				"search_latency_ms":              1,
				"search_requests_per_second":     1100,
				"total_requests_per_second":      1100,
				"write_latency_ms":               1,
				"write_requests_per_second":      1100,
			},
		},
		"success without API key": {
			prepare:         caseOkNoApiKey,
			wantNumOfCharts: len(baseCharts),
			wantMetrics: map[string]int64{
				"health_status_ok":            0,
				"health_status_out_of_disk":   1,
				"health_status_out_of_memory": 0,
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
			case urlPathHealth:
				_, _ = w.Write(dataVer27HealthOk)
			case urlPathStats:
				if r.Header.Get("X-TYPESENSE-API-KEY") != testApiKey {
					msg := "{\"message\": \"Forbidden - a valid `x-typesense-api-key` header must be sent.\"}"
					_, _ = w.Write([]byte(msg))
					w.WriteHeader(http.StatusUnauthorized)
				} else {
					_, _ = w.Write(dataVer27Stats)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	collr := New()
	collr.URL = srv.URL
	collr.APIKey = testApiKey
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseOkNoApiKey(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathHealth:
				_, _ = w.Write(dataVer27HealthNok)
			case urlPathStats:
				if r.Header.Get("X-TYPESENSE-API-KEY") != testApiKey {
					msg := "{\"message\": \"Forbidden - a valid `x-typesense-api-key` header must be sent.\"}"
					_, _ = w.Write([]byte(msg))
					w.WriteHeader(http.StatusUnauthorized)
				} else {
					_, _ = w.Write(dataVer27Stats)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	collr := New()
	collr.URL = srv.URL
	collr.APIKey = ""
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
