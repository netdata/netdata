// SPDX-License-Identifier: GPL-3.0-or-later

package puppet

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

	serviceStatusResponse, _ = os.ReadFile("testdata/serviceStatusResponse.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":        dataConfigJSON,
		"dataConfigYAML":        dataConfigYAML,
		"serviceStatusResponse": serviceStatusResponse,
	} {
		require.NotNil(t, data, name)
	}
}

func TestPuppet_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Puppet{}, dataConfigJSON, dataConfigYAML)
}

func TestPuppet_Init(t *testing.T) {
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
			puppet := New()
			puppet.Config = test.config

			if test.wantFail {
				assert.Error(t, puppet.Init())
			} else {
				assert.NoError(t, puppet.Init())
			}
		})
	}
}

func TestPuppet_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestPuppet_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Puppet, func())
	}{
		"success default config": {
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
			puppet, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, puppet.Check())
			} else {
				assert.NoError(t, puppet.Check())
			}
		})
	}
}

func TestPuppet_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func(t *testing.T) (*Puppet, func())
		wantMetrics map[string]int64
	}{
		"success default config": {
			prepare: prepareCaseOkDefault,
			wantMetrics: map[string]int64{
				"cpu_usage":             49,
				"fd_max":                524288,
				"fd_used":               234,
				"gc_cpu_usage":          0,
				"jvm_heap_committed":    1073741824,
				"jvm_heap_init":         1073741824,
				"jvm_heap_max":          1073741824,
				"jvm_heap_used":         550502400,
				"jvm_nonheap_committed": 334102528,
				"jvm_nonheap_init":      7667712,
				"jvm_nonheap_max":       -1,
				"jvm_nonheap_used":      291591160,
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
			puppet, cleanup := test.prepare(t)
			defer cleanup()

			mx := puppet.Collect()

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				testMetricsHasAllChartsDims(t, puppet, mx)
			}
		})
	}
}

func testMetricsHasAllChartsDims(t *testing.T, puppet *Puppet, mx map[string]int64) {
	for _, chart := range *puppet.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := mx[dim.ID]
			assert.Truef(t, ok, "collected metrics has no data for dim '%s' chart '%s'", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := mx[v.ID]
			assert.Truef(t, ok, "collected metrics has no data for var '%s' chart '%s'", v.ID, chart.ID)
		}
	}
}

func prepareCaseOkDefault(t *testing.T) (*Puppet, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/status/v1/services":
				if r.URL.RawQuery != urlQueryStatusService {
					w.WriteHeader(http.StatusNotFound)
				} else {
					_, _ = w.Write(serviceStatusResponse)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	puppet := New()
	puppet.URL = srv.URL
	require.NoError(t, puppet.Init())

	return puppet, srv.Close
}

func prepareCaseUnexpectedJsonResponse(t *testing.T) (*Puppet, func()) {
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

	puppet := New()
	puppet.URL = srv.URL
	require.NoError(t, puppet.Init())

	return puppet, srv.Close
}

func prepareCaseInvalidFormatResponse(t *testing.T) (*Puppet, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	puppet := New()
	puppet.URL = srv.URL
	require.NoError(t, puppet.Init())

	return puppet, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*Puppet, func()) {
	t.Helper()
	puppet := New()
	puppet.URL = "http://127.0.0.1:65001"
	require.NoError(t, puppet.Init())

	return puppet, func() {}
}
