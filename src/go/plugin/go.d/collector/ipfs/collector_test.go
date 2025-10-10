// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	apiv0PinLsData, _      = os.ReadFile("testdata/api_v0_pin_ls.json")
	apiv0StatsBwData, _    = os.ReadFile("testdata/api_v0_stats_bw.json")
	apiv0StatsRepoData, _  = os.ReadFile("testdata/api_v0_stats_repo.json")
	apiv0SwarmPeersData, _ = os.ReadFile("testdata/api_v0_swarm_peers.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":      dataConfigJSON,
		"dataConfigYAML":      dataConfigYAML,
		"apiv0PinLsData":      apiv0PinLsData,
		"apiv0StatsBwData":    apiv0StatsBwData,
		"apiv0StatsRepoData":  apiv0StatsRepoData,
		"apiv0SwarmPeersData": apiv0SwarmPeersData,
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

func TestCollector_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Collector, func())
	}{
		"success default config": {
			wantFail: false,
			prepare:  prepareCaseOkDefault,
		},
		"success all queries enabled": {
			wantFail: false,
			prepare:  prepareCaseOkDefault,
		},
		"fails on unexpected json response": {
			wantFail: true,
			prepare:  prepareCaseUnexpectedJsonResponse,
		},
		"fails on invalid format response": {
			wantFail: true,
			prepare:  prepareCaseInvalidFormatResponse,
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
		prepare     func(t *testing.T) (*Collector, func())
		wantMetrics map[string]int64
	}{
		"success default config": {
			prepare: prepareCaseOkDefault,
			wantMetrics: map[string]int64{
				"in":    20113594,
				"out":   3113852,
				"peers": 6,
			},
		},
		"success all queries enabled": {
			prepare: prepareCaseOkAllQueriesEnabled,
			wantMetrics: map[string]int64{
				"in":             20113594,
				"objects":        1,
				"out":            3113852,
				"peers":          6,
				"pinned":         1,
				"recursive_pins": 1,
				"size":           25495,
				"used_percent":   0,
			},
		},
		"fails on unexpected json response": {
			prepare: prepareCaseUnexpectedJsonResponse,
		},
		"fails on invalid format response": {
			prepare: prepareCaseInvalidFormatResponse,
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

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareCaseOkDefault(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathStatsBandwidth:
				_, _ = w.Write(apiv0StatsBwData)
			case urlPathStatsRepo:
				_, _ = w.Write(apiv0StatsRepoData)
			case urlPathSwarmPeers:
				_, _ = w.Write(apiv0SwarmPeersData)
			case urlPathPinLs:
				_, _ = w.Write(apiv0PinLsData)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCaseOkAllQueriesEnabled(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr, cleanup := prepareCaseOkDefault(t)

	collr.QueryRepoApi = true
	collr.QueryPinApi = true

	return collr, cleanup
}

func prepareCaseUnexpectedJsonResponse(t *testing.T) (*Collector, func()) {
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

func prepareCaseInvalidFormatResponse(t *testing.T) (*Collector, func()) {
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

func prepareCaseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}
