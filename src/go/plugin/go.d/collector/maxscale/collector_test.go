// SPDX-License-Identifier: GPL-3.0-or-later

package maxscale

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

	dataVer24MaxScale, _        = os.ReadFile("testdata/v24.02.3/maxscale.json")
	dataVer24MaxScaleThreads, _ = os.ReadFile("testdata/v24.02.3/maxscale_threads.json")
	dataVer24Servers, _         = os.ReadFile("testdata/v24.02.3/servers.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":           dataConfigJSON,
		"dataConfigYAML":           dataConfigYAML,
		"dataVer24MaxScale":        dataVer24MaxScale,
		"dataVer24MaxScaleThreads": dataVer24MaxScaleThreads,
		"dataVer24Servers":         dataVer24Servers,
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
			prepare:         caseOk,
			wantNumOfCharts: len(charts) + len(serverChartsTmpl)*1,
			wantMetrics: map[string]int64{
				"server_kek_connections":        0,
				"server_kek_state_Binlog Relay": 0,
				"server_kek_state_Down":         1,
				"server_kek_state_Drained":      0,
				"server_kek_state_Draining":     0,
				"server_kek_state_Maintenance":  0,
				"server_kek_state_Master":       0,
				"server_kek_state_Relay Master": 0,
				"server_kek_state_Running":      0,
				"server_kek_state_Slave":        0,
				"server_kek_state_Synced":       0,
				"threads_accepts":               0,
				"threads_current_fds":           3,
				"threads_errors":                0,
				"threads_hangups":               0,
				"threads_qc_cache_evictions":    0,
				"threads_qc_cache_hits":         0,
				"threads_qc_cache_inserts":      0,
				"threads_qc_cache_misses":       0,
				"threads_reads":                 68359,
				"threads_sessions":              0,
				"threads_state_Active":          1,
				"threads_state_Dormant":         0,
				"threads_state_Draining":        0,
				"threads_total_fds":             3,
				"threads_writes":                0,
				"threads_zombies":               0,
				"uptime":                        61298,
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
			case urlPathMaxscale:
				_, _ = w.Write(dataVer24MaxScale)
			case urlPathMaxscaleThreads:
				_, _ = w.Write(dataVer24MaxScaleThreads)
			case urlPathServers:
				_, _ = w.Write(dataVer24Servers)
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
