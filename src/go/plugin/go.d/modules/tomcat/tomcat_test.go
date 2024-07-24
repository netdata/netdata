// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

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

	dataServerStatus, _ = os.ReadFile("testdata/stats.xml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":   dataConfigJSON,
		"dataConfigYAML":   dataConfigYAML,
		"dataServerStatus": dataServerStatus,
	} {
		require.NotNil(t, data, name)
	}
}

func TestTomcat_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Tomcat{}, dataConfigJSON, dataConfigYAML)
}

func TestTomcat_Init(t *testing.T) {
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
		"fail when URL has no wantMetrics suffix": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{URL: "http://127.0.0.1:38001"},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			tomcat := New()
			tomcat.Config = test.config

			if test.wantFail {
				assert.Error(t, tomcat.Init())
			} else {
				assert.NoError(t, tomcat.Init())
			}
		})
	}
}

func TestTomcat_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestTomcat_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Tomcat, func())
	}{
		"success case": {
			wantFail: false,
			prepare:  caseSuccess,
		},
		"fails on unexpected xml response": {
			wantFail: true,
			prepare:  prepareCaseUnexpectedXMLResponse,
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
			tomcat, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.Error(t, tomcat.Check())
			} else {
				assert.NoError(t, tomcat.Check())
			}
		})
	}
}

func TestTomcat_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare                 func(t *testing.T) (tomcat *Tomcat, cleanup func())
		wantMetrics             map[string]int64
		disconnectBeforeCleanup bool
		disconnectAfterCleanup  bool
		wantCharts              int
	}{
		"success case": {
			prepare:                 caseSuccess,
			disconnectBeforeCleanup: false,
			disconnectAfterCleanup:  true,
			wantCharts:              len(charts),
			wantMetrics: map[string]int64{
				"busy_thread_count":    1,
				"bytes_received":       0,
				"bytes_sent":           12174519,
				"code_cache_committed": 13172736,
				"code_cache_max":       122908672,
				"code_cache_used":      13132032,
				"compressed_committed": 1900544,
				"compressed_max":       1073741824,
				"compressed_used":      1712872,
				"current_thread_count": 10,
				"eden_committed":       108003328,
				"eden_max":             -1,
				"eden_used":            23068672,
				"error_count":          24,
				"jvm.free":             144529816,
				"jvm.max":              1914699776,
				"jvm.total":            179306496,
				"metaspace_committed":  18939904,
				"metaspace_max":        -1,
				"metaspace_used":       18537336,
				"processing_time":      28326,
				"request_count":        4838,
				"survivor_committed":   5242880,
				"survivor_max":         -1,
				"survivor_used":        5040192,
				"tenured_committed":    66060288,
				"tenured_max":          1914699776,
				"tenured_used":         6175120,
			},
		},
		"fails on unexpected xml response": {
			prepare: prepareCaseUnexpectedXMLResponse,
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
			tomcat, cleanup := test.prepare(t)
			defer cleanup()

			mx := tomcat.Collect()

			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*tomcat.Charts()))
				module.TestMetricsHasAllChartsDims(t, tomcat.Charts(), mx)
			}
		})
	}
}

func caseSuccess(t *testing.T) (*Tomcat, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataServerStatus)
		}))
	tomcat := New()
	tomcat.URL = srv.URL + "/status?XML=true"
	require.NoError(t, tomcat.Init())

	return tomcat, srv.Close
}

func prepareCaseConnectionRefused(t *testing.T) (*Tomcat, func()) {
	t.Helper()
	tomcat := New()
	tomcat.URL = "http://127.0.0.1:65001" + urlPathServerStats
	require.NoError(t, tomcat.Init())

	return tomcat, func() {}
}

func prepareCaseUnexpectedXMLResponse(t *testing.T) (*Tomcat, func()) {
	t.Helper()
	resp := `
<?xml version="1.0" encoding="UTF-8" ?>
 <root>
     <elephant>
         <burn>false</burn>
         <mountain>true</mountain>
         <fog>false</fog>
         <skin>-1561907625</skin>
         <burst>anyway</burst>
         <shadow>1558616893</shadow>
     </elephant>
     <start>ever</start>
     <base>2093056027</base>
     <mission>-2007590351</mission>
     <victory>999053756</victory>
     <die>false</die>
 </root>

`
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(resp))
		}))

	tomcat := New()
	tomcat.URL = srv.URL + urlPathServerStats
	require.NoError(t, tomcat.Init())

	return tomcat, srv.Close
}

func prepareCaseInvalidFormatResponse(t *testing.T) (*Tomcat, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))

	tomcat := New()
	tomcat.URL = srv.URL + urlPathServerStats
	require.NoError(t, tomcat.Init())

	return tomcat, srv.Close
}
