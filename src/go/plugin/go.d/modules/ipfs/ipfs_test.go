// SPDX-License-Identifier: GPL-3.0-or-later

package ipfs

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

	ipfstestdata, _ = os.ReadFile("testdata/stats_bw.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"ipfstestdata":   ipfstestdata,
	} {
		require.NotNil(t, data, name)
	}
}

func TestIPFS_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &IPFS{}, dataConfigJSON, dataConfigYAML)
}

func TestIPFS_Init(t *testing.T) {
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
			ipfs := New()
			ipfs.Config = test.config

			if test.wantFail {
				assert.Error(t, ipfs.Init())
			} else {
				assert.NoError(t, ipfs.Init())
			}
		})
	}
}

func TestIPFS_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestIPFS_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*IPFS, func())
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  prepareCaseOk,
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
			ipfs, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, ipfs.Check())
			} else {
				assert.NoError(t, ipfs.Check())
			}
		})
	}
}

func TestIPFS_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func(t *testing.T) (*IPFS, func())
		wantMetrics map[string]int64
	}{
		"success on valid response": {
			prepare: prepareCaseOk,
			wantMetrics: map[string]int64{
				"TotalIn":  20113594,
				"TotalOut": 3113852,
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
			ipfs, cleanup := test.prepare(t)
			defer cleanup()

			mx := ipfs.Collect()

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				testMetricsHasAllChartsDims(t, ipfs, mx)
			}
		})
	}
}

func testMetricsHasAllChartsDims(t *testing.T, ipfs *IPFS, mx map[string]int64) {
	for _, chart := range *ipfs.Charts() {
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

func prepareCaseOk(t *testing.T) (*IPFS, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/api/v0/stats/bw":
				_, _ = w.Write(ipfstestdata)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	ipfs := New()
	ipfs.URL = srv.URL
	require.NoError(t, ipfs.Init())

	return ipfs, srv.Close
}

func prepareCaseUnexpectedJsonResponse(t *testing.T) (*IPFS, func()) {
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
			switch r.URL.Path {
			case "/stat":
				_, _ = w.Write([]byte(resp))
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	ipfs := New()
	ipfs.URL = srv.URL
	require.NoError(t, ipfs.Init())

	return ipfs, srv.Close
}

func prepareCaseInvalidFormatResponse(t *testing.T) (*IPFS, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	ipfs := New()
	ipfs.URL = srv.URL
	require.NoError(t, ipfs.Init())

	return ipfs, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*IPFS, func()) {
	t.Helper()
	ipfs := New()
	ipfs.URL = "http://127.0.0.1:5002"
	require.NoError(t, ipfs.Init())

	return ipfs, func() {}
}
