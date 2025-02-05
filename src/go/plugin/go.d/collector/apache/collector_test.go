// SPDX-License-Identifier: GPL-3.0-or-later

package apache

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

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataSimpleStatusMPMEvent, _     = os.ReadFile("testdata/simple-status-mpm-event.txt")
	dataExtendedStatusMPMEvent, _   = os.ReadFile("testdata/extended-status-mpm-event.txt")
	dataExtendedStatusMPMPrefork, _ = os.ReadFile("testdata/extended-status-mpm-prefork.txt")
	dataLighttpdStatus, _           = os.ReadFile("testdata/lighttpd-status.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":               dataConfigJSON,
		"dataConfigYAML":               dataConfigYAML,
		"dataSimpleStatusMPMEvent":     dataSimpleStatusMPMEvent,
		"dataExtendedStatusMPMEvent":   dataExtendedStatusMPMEvent,
		"dataExtendedStatusMPMPrefork": dataExtendedStatusMPMPrefork,
		"dataLighttpdStatus":           dataLighttpdStatus,
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
		"success when URL has no wantMetrics suffix": {
			wantFail: false,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:38001"},
				},
			},
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
		"success on simple status MPM Event": {
			wantFail: false,
			prepare:  caseMPMEventSimpleStatus,
		},
		"success on extended status MPM Event": {
			wantFail: false,
			prepare:  caseMPMEventExtendedStatus,
		},
		"success on extended status MPM Prefork": {
			wantFail: false,
			prepare:  caseMPMPreforkExtendedStatus,
		},
		"fail on Lighttpd response": {
			wantFail: true,
			prepare:  caseLighttpdResponse,
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
		"success on simple status MPM Event": {
			prepare:         caseMPMEventSimpleStatus,
			wantNumOfCharts: len(baseCharts),
			wantMetrics: map[string]int64{
				"busy_workers":            1,
				"conns_async_closing":     0,
				"conns_async_keep_alive":  0,
				"conns_async_writing":     0,
				"conns_total":             0,
				"idle_workers":            74,
				"scoreboard_closing":      0,
				"scoreboard_dns_lookup":   0,
				"scoreboard_finishing":    0,
				"scoreboard_idle_cleanup": 0,
				"scoreboard_keepalive":    0,
				"scoreboard_logging":      0,
				"scoreboard_open":         325,
				"scoreboard_reading":      0,
				"scoreboard_sending":      1,
				"scoreboard_starting":     0,
				"scoreboard_waiting":      74,
			},
		},
		"success on extended status MPM Event": {
			prepare:         caseMPMEventExtendedStatus,
			wantNumOfCharts: len(baseCharts) + len(extendedCharts),
			wantMetrics: map[string]int64{
				"busy_workers":            1,
				"bytes_per_req":           136533000,
				"bytes_per_sec":           4800000,
				"conns_async_closing":     0,
				"conns_async_keep_alive":  0,
				"conns_async_writing":     0,
				"conns_total":             0,
				"idle_workers":            99,
				"req_per_sec":             3515,
				"scoreboard_closing":      0,
				"scoreboard_dns_lookup":   0,
				"scoreboard_finishing":    0,
				"scoreboard_idle_cleanup": 0,
				"scoreboard_keepalive":    0,
				"scoreboard_logging":      0,
				"scoreboard_open":         300,
				"scoreboard_reading":      0,
				"scoreboard_sending":      1,
				"scoreboard_starting":     0,
				"scoreboard_waiting":      99,
				"total_accesses":          9,
				"total_kBytes":            12,
				"uptime":                  256,
			},
		},
		"success on extended status MPM Prefork": {
			prepare:         caseMPMPreforkExtendedStatus,
			wantNumOfCharts: len(baseCharts) + len(extendedCharts) - 2,
			wantMetrics: map[string]int64{
				"busy_workers":            70,
				"bytes_per_req":           3617880000,
				"bytes_per_sec":           614250000000,
				"idle_workers":            1037,
				"req_per_sec":             16978100,
				"scoreboard_closing":      0,
				"scoreboard_dns_lookup":   0,
				"scoreboard_finishing":    0,
				"scoreboard_idle_cleanup": 0,
				"scoreboard_keepalive":    0,
				"scoreboard_logging":      0,
				"scoreboard_open":         3,
				"scoreboard_reading":      0,
				"scoreboard_sending":      0,
				"scoreboard_starting":     0,
				"scoreboard_waiting":      3,
				"total_accesses":          120358784,
				"total_kBytes":            4252382776,
				"uptime":                  708904,
			},
		},
		"fail on Lighttpd response": {
			prepare:         caseLighttpdResponse,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on invalid data response": {
			prepare:         caseInvalidDataResponse,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on connection refused": {
			prepare:         caseConnectionRefused,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
		"fail on 404 response": {
			prepare:         case404,
			wantNumOfCharts: 0,
			wantMetrics:     nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			_ = collr.Check(context.Background())

			collected := collr.Collect(context.Background())

			require.Equal(t, test.wantMetrics, collected)
			assert.Equal(t, test.wantNumOfCharts, len(*collr.Charts()))
		})
	}
}

func caseMPMEventSimpleStatus(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataSimpleStatusMPMEvent)
		}))
	collr := New()
	collr.URL = srv.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseMPMEventExtendedStatus(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataExtendedStatusMPMEvent)
		}))
	collr := New()
	collr.URL = srv.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseMPMPreforkExtendedStatus(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataExtendedStatusMPMPrefork)
		}))
	collr := New()
	collr.URL = srv.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseLighttpdResponse(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataLighttpdStatus)
		}))
	collr := New()
	collr.URL = srv.URL + "/server-status?auto"
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
	collr.URL = srv.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:65001/server-status?auto"
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
	collr.URL = srv.URL + "/server-status?auto"
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}
