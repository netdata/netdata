// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataVer151JSONStat, _ = os.ReadFile("testdata/v1.5.1/jsonstat.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":     dataConfigJSON,
		"dataConfigYAML":     dataConfigYAML,
		"dataVer151JSONStat": dataVer151JSONStat,
	} {
		require.NotNil(t, data, name)
	}
}

func TestCollector_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &Collector{}, dataConfigJSON, dataConfigYAML)
}

func TestCollector_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset URL": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: ""},
				},
			},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL: "http://127.0.0.1:38001",
					},
					ClientConfig: web.ClientConfig{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
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
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (collr *Collector, cleanup func())
		wantFail bool
	}{
		"success on valid response v1.5.1": {
			prepare:  preparePowerDNSdistV151,
			wantFail: false,
		},
		"fails on 404 response": {
			prepare:  preparePowerDNSdist404,
			wantFail: true,
		},
		"fails on connection refused": {
			prepare:  preparePowerDNSdistConnectionRefused,
			wantFail: true,
		},
		"fails with invalid data": {
			prepare:  preparePowerDNSdistInvalidData,
			wantFail: true,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()
			require.NoError(t, collr.Init(context.Background()))

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
		prepare       func() (collr *Collector, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid response v1.5.1": {
			prepare: preparePowerDNSdistV151,
			wantCollected: map[string]int64{
				"acl-drops":              1,
				"cache-hits":             1,
				"cache-misses":           1,
				"cpu-sys-msec":           411,
				"cpu-user-msec":          939,
				"downstream-send-errors": 1,
				"downstream-timeouts":    1,
				"dyn-blocked":            1,
				"empty-queries":          1,
				"latency-avg100":         14237,
				"latency-avg1000":        9728,
				"latency-avg10000":       1514,
				"latency-avg1000000":     15,
				"latency-slow":           1,
				"latency0-1":             1,
				"latency1-10":            3,
				"latency10-50":           996,
				"latency100-1000":        4,
				"latency50-100":          1,
				"no-policy":              1,
				"noncompliant-queries":   1,
				"noncompliant-responses": 1,
				"queries":                1003,
				"rdqueries":              1003,
				"real-memory-usage":      202125312,
				"responses":              1003,
				"rule-drop":              1,
				"rule-nxdomain":          1,
				"rule-refused":           1,
				"self-answered":          1,
				"servfail-responses":     1,
				"trunc-failures":         1,
			},
		},
		"fails on 404 response": {
			prepare: preparePowerDNSdist404,
		},
		"fails on connection refused": {
			prepare: preparePowerDNSdistConnectionRefused,
		},
		"fails with invalid data": {
			prepare: preparePowerDNSdistInvalidData,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()
			require.NoError(t, collr.Init(context.Background()))

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			if len(test.wantCollected) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func preparePowerDNSdistV151() (*Collector, func()) {
	srv := preparePowerDNSDistEndpoint()
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSdist404() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSdistConnectionRefused() (*Collector, func()) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001"

	return collr, func() {}
}

func preparePowerDNSdistInvalidData() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSDistEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.String() {
			case "/jsonstat?command=stats":
				_, _ = w.Write(dataVer151JSONStat)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}
