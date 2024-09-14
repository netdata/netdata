// SPDX-License-Identifier: GPL-3.0-or-later

package nginxunit

import (
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

	dataVer1291Status, _ = os.ReadFile("testdata/v1.29.1/status.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":    dataConfigJSON,
		"dataConfigYAML":    dataConfigYAML,
		"dataVer1291Status": dataVer1291Status,
	} {
		require.NotNil(t, data, name)

	}
}

func TestNginxUnit_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &NginxUnit{}, dataConfigJSON, dataConfigYAML)
}

func TestNginxUnit_Init(t *testing.T) {
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
			nu := New()
			nu.Config = test.config

			if test.wantFail {
				assert.Error(t, nu.Init())
			} else {
				assert.NoError(t, nu.Init())
			}
		})
	}
}

func TestNginxUnit_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (nu *NginxUnit, cleanup func())
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
			nu, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, nu.Check())
			} else {
				assert.NoError(t, nu.Check())
			}
		})
	}
}

func TestNginxUnit_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestNginxUnit_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare         func(t *testing.T) (nu *NginxUnit, cleanup func())
		wantNumOfCharts int
		wantMetrics     map[string]int64
	}{
		"success on valid response": {
			prepare:         caseOk,
			wantNumOfCharts: len(charts),
			wantMetrics: map[string]int64{
				"connections_accepted": 1,
				"connections_active":   1,
				"connections_closed":   1,
				"connections_idle":     1,
				"requests_total":       1,
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
			nu, cleanup := test.prepare(t)
			defer cleanup()

			_ = nu.Check()

			mx := nu.Collect()

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantNumOfCharts, len(*nu.Charts()), "want charnu")

				module.TestMetricsHasAllChartsDims(t, nu.Charts(), mx)
			}
		})
	}
}

func caseOk(t *testing.T) (*NginxUnit, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathStatus:
				_, _ = w.Write(dataVer1291Status)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
	nu := New()
	nu.URL = srv.URL
	require.NoError(t, nu.Init())

	return nu, srv.Close
}

func caseUnexpectedJsonResponse(t *testing.T) (*NginxUnit, func()) {
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
	nu := New()
	nu.URL = srv.URL
	require.NoError(t, nu.Init())

	return nu, srv.Close
}

func caseInvalidDataResponse(t *testing.T) (*NginxUnit, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	nu := New()
	nu.URL = srv.URL
	require.NoError(t, nu.Init())

	return nu, srv.Close
}

func caseConnectionRefused(t *testing.T) (*NginxUnit, func()) {
	t.Helper()
	nu := New()
	nu.URL = "http://127.0.0.1:65001"
	require.NoError(t, nu.Init())

	return nu, func() {}
}

func case404(t *testing.T) (*NginxUnit, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	nu := New()
	nu.URL = srv.URL
	require.NoError(t, nu.Init())

	return nu, srv.Close
}
