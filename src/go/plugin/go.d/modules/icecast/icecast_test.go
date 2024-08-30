// SPDX-License-Identifier: GPL-3.0-or-later

package icecast

import (
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

	dataServerStatsMultiSource, _  = os.ReadFile("testdata/stats_multi_source.json")
	dataServerStatsSingleSource, _ = os.ReadFile("testdata/stats_single_source.json")
	dataServerStatsNoSources, _    = os.ReadFile("testdata/stats_no_sources.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":              dataConfigJSON,
		"dataConfigYAML":              dataConfigYAML,
		"dataServerStats":             dataServerStatsMultiSource,
		"dataServerStatsSingleSource": dataServerStatsSingleSource,
		"dataServerStatsNoSources":    dataServerStatsNoSources,
	} {
		require.NotNil(t, data, name)
	}
}

func TestIcecast_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Icecast{}, dataConfigJSON, dataConfigYAML)
}

func TestIcecast_Init(t *testing.T) {
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
				HTTP: web.HTTP{
					Request: web.Request{URL: ""},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			icecast := New()
			icecast.Config = test.config

			if test.wantFail {
				assert.Error(t, icecast.Init())
			} else {
				assert.NoError(t, icecast.Init())
			}
		})
	}
}

func TestIcecast_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestIcecast_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Icecast, func())
	}{
		"success multiple sources": {
			wantFail: false,
			prepare:  prepareCaseMultipleSources,
		},
		"success single source": {
			wantFail: false,
			prepare:  prepareCaseMultipleSources,
		},
		"fails on no sources": {
			wantFail: true,
			prepare:  prepareCaseNoSources,
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
			icecast, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, icecast.Check())
			} else {
				assert.NoError(t, icecast.Check())
			}
		})
	}
}

func TestIcecast_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func(t *testing.T) (*Icecast, func())
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success multiple sources": {
			prepare:    prepareCaseMultipleSources,
			wantCharts: len(sourceChartsTmpl) * 2,
			wantMetrics: map[string]int64{
				"source_abc_listeners": 1,
				"source_efg_listeners": 10,
			},
		},
		"success single source": {
			prepare:    prepareCaseSingleSource,
			wantCharts: len(sourceChartsTmpl) * 1,
			wantMetrics: map[string]int64{
				"source_abc_listeners": 1,
			},
		},
		"fails on no sources": {
			prepare: prepareCaseNoSources,
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
			icecast, cleanup := test.prepare(t)
			defer cleanup()

			mx := icecast.Collect()

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*icecast.Charts()))
				module.TestMetricsHasAllChartsDims(t, icecast.Charts(), mx)
			}
		})
	}
}

func prepareCaseMultipleSources(t *testing.T) (*Icecast, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathServerStats:
				_, _ = w.Write(dataServerStatsMultiSource)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	icecast := New()
	icecast.URL = srv.URL
	require.NoError(t, icecast.Init())

	return icecast, srv.Close
}

func prepareCaseSingleSource(t *testing.T) (*Icecast, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathServerStats:
				_, _ = w.Write(dataServerStatsSingleSource)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	icecast := New()
	icecast.URL = srv.URL
	require.NoError(t, icecast.Init())

	return icecast, srv.Close
}

func prepareCaseNoSources(t *testing.T) (*Icecast, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathServerStats:
				_, _ = w.Write(dataServerStatsNoSources)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	icecast := New()
	icecast.URL = srv.URL
	require.NoError(t, icecast.Init())

	return icecast, srv.Close
}

func prepareCaseUnexpectedJsonResponse(t *testing.T) (*Icecast, func()) {
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

	icecast := New()
	icecast.URL = srv.URL
	require.NoError(t, icecast.Init())

	return icecast, srv.Close
}

func prepareCaseInvalidFormatResponse(t *testing.T) (*Icecast, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	icecast := New()
	icecast.URL = srv.URL
	require.NoError(t, icecast.Init())

	return icecast, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*Icecast, func()) {
	t.Helper()
	icecast := New()
	icecast.URL = "http://127.0.0.1:65001"
	require.NoError(t, icecast.Init())

	return icecast, func() {}
}
