// SPDX-License-Identifier: GPL-3.0-or-later

package rspamd

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

	dataV34Stat, _ = os.ReadFile("testdata/v3.4-stat.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataV34Stat":    dataV34Stat,
	} {
		require.NotNil(t, data, name)
	}
}

func TestRspamd_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Rspamd{}, dataConfigJSON, dataConfigYAML)
}

func TestRspamd_Init(t *testing.T) {
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
			rsp := New()
			rsp.Config = test.config

			if test.wantFail {
				assert.Error(t, rsp.Init())
			} else {
				assert.NoError(t, rsp.Init())
			}
		})
	}
}

func TestRspamd_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestRspamd_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Rspamd, func())
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
			rsp, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, rsp.Check())
			} else {
				assert.NoError(t, rsp.Check())
			}
		})
	}
}

func TestRspamd_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func(t *testing.T) (*Rspamd, func())
		wantMetrics map[string]int64
	}{
		"success on valid response": {
			prepare: prepareCaseOk,
			wantMetrics: map[string]int64{
				"actions_add_header":         1,
				"actions_custom":             0,
				"actions_discard":            0,
				"actions_greylist":           1,
				"actions_invalid_max_action": 0,
				"actions_no_action":          1,
				"actions_quarantine":         0,
				"actions_reject":             1,
				"actions_rewrite_subject":    1,
				"actions_soft_reject":        1,
				"actions_unknown_action":     0,
				"connections":                1,
				"control_connections":        117,
				"ham_count":                  1,
				"learned":                    1,
				"scanned":                    1,
				"spam_count":                 1,
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
			rsp, cleanup := test.prepare(t)
			defer cleanup()

			mx := rsp.Collect()

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, rsp.Charts(), mx)
			}
		})
	}
}

func prepareCaseOk(t *testing.T) (*Rspamd, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/stat":
				_, _ = w.Write(dataV34Stat)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))

	rsp := New()
	rsp.URL = srv.URL
	require.NoError(t, rsp.Init())

	return rsp, srv.Close
}

func prepareCaseUnexpectedJsonResponse(t *testing.T) (*Rspamd, func()) {
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

	rsp := New()
	rsp.URL = srv.URL
	require.NoError(t, rsp.Init())

	return rsp, srv.Close
}

func prepareCaseInvalidFormatResponse(t *testing.T) (*Rspamd, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	rsp := New()
	rsp.URL = srv.URL
	require.NoError(t, rsp.Init())

	return rsp, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*Rspamd, func()) {
	t.Helper()
	rsp := New()
	rsp.URL = "http://127.0.0.1:65001/stat"
	require.NoError(t, rsp.Init())

	return rsp, func() {}
}
