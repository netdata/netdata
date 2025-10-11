// SPDX-License-Identifier: GPL-3.0-or-later

package tomcat

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

	dataServerStatus, _ = os.ReadFile("testdata/server_status.xml")
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
		"success case": {
			wantFail: false,
			prepare:  prepareCaseSuccess,
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
		prepare     func(t *testing.T) (collr *Collector, cleanup func())
		wantMetrics map[string]int64
		wantCharts  int
	}{
		"success case": {
			prepare:    prepareCaseSuccess,
			wantCharts: len(defaultCharts) + len(jvmMemoryPoolChartsTmpl)*8 + len(connectorChartsTmpl)*2,
			wantMetrics: map[string]int64{
				"connector_http-nio-8080_request_info_bytes_received":  0,
				"connector_http-nio-8080_request_info_bytes_sent":      12174519,
				"connector_http-nio-8080_request_info_error_count":     24,
				"connector_http-nio-8080_request_info_processing_time": 28326,
				"connector_http-nio-8080_request_info_request_count":   4838,
				"connector_http-nio-8080_thread_info_busy":             1,
				"connector_http-nio-8080_thread_info_count":            10,
				"connector_http-nio-8080_thread_info_idle":             9,
				"connector_http-nio-8081_request_info_bytes_received":  0,
				"connector_http-nio-8081_request_info_bytes_sent":      12174519,
				"connector_http-nio-8081_request_info_error_count":     24,
				"connector_http-nio-8081_request_info_processing_time": 28326,
				"connector_http-nio-8081_request_info_request_count":   4838,
				"connector_http-nio-8081_thread_info_busy":             1,
				"connector_http-nio-8081_thread_info_count":            10,
				"connector_http-nio-8081_thread_info_idle":             9,
				"jvm_memory_free":  144529816,
				"jvm_memory_total": 179306496,
				"jvm_memory_used":  34776680,
				"jvm_memorypool_codeheap_non-nmethods_commited":          2555904,
				"jvm_memorypool_codeheap_non-nmethods_max":               5840896,
				"jvm_memorypool_codeheap_non-nmethods_used":              1477888,
				"jvm_memorypool_codeheap_non-profiled_nmethods_commited": 4587520,
				"jvm_memorypool_codeheap_non-profiled_nmethods_max":      122908672,
				"jvm_memorypool_codeheap_non-profiled_nmethods_used":     4536704,
				"jvm_memorypool_codeheap_profiled_nmethods_commited":     13172736,
				"jvm_memorypool_codeheap_profiled_nmethods_max":          122908672,
				"jvm_memorypool_codeheap_profiled_nmethods_used":         13132032,
				"jvm_memorypool_compressed_class_space_commited":         1900544,
				"jvm_memorypool_compressed_class_space_max":              1073741824,
				"jvm_memorypool_compressed_class_space_used":             1712872,
				"jvm_memorypool_g1_eden_space_commited":                  108003328,
				"jvm_memorypool_g1_eden_space_max":                       -1,
				"jvm_memorypool_g1_eden_space_used":                      23068672,
				"jvm_memorypool_g1_old_gen_commited":                     66060288,
				"jvm_memorypool_g1_old_gen_max":                          1914699776,
				"jvm_memorypool_g1_old_gen_used":                         6175120,
				"jvm_memorypool_g1_survivor_space_commited":              5242880,
				"jvm_memorypool_g1_survivor_space_max":                   -1,
				"jvm_memorypool_g1_survivor_space_used":                  5040192,
				"jvm_memorypool_metaspace_commited":                      18939904,
				"jvm_memorypool_metaspace_max":                           -1,
				"jvm_memorypool_metaspace_used":                          18537336,
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
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, mx)

			if len(test.wantMetrics) > 0 {
				assert.Equal(t, test.wantCharts, len(*collr.Charts()))
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func prepareCaseSuccess(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathServerStatus:
				if r.URL.RawQuery != urlQueryServerStatus {
					w.WriteHeader(http.StatusNotFound)
				} else {
					_, _ = w.Write(dataServerStatus)
				}
			default:
				w.WriteHeader(http.StatusNotFound)
			}
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

func prepareCaseUnexpectedXMLResponse(t *testing.T) (*Collector, func()) {
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
