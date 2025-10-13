// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

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

	dataVer430statistics, _   = os.ReadFile("testdata/v4.3.0/statistics.json")
	dataRecursorStatistics, _ = os.ReadFile("testdata/recursor/statistics.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":         dataConfigJSON,
		"dataConfigYAML":         dataConfigYAML,
		"dataVer430statistics":   dataVer430statistics,
		"dataRecursorStatistics": dataRecursorStatistics,
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (collr *Collector, cleanup func())
		wantFail bool
	}{
		"success on valid response v4.3.0": {
			prepare: preparePowerDNSAuthoritativeNSV430,
		},
		"fails on response from PowerDNS Recursor": {
			wantFail: true,
			prepare:  preparePowerDNSAuthoritativeNSRecursorData,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  preparePowerDNSAuthoritativeNS404,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  preparePowerDNSAuthoritativeNSConnectionRefused,
		},
		"fails on response with invalid data": {
			wantFail: true,
			prepare:  preparePowerDNSAuthoritativeNSInvalidData,
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

func TestCollector_Charts(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.Background()))
	assert.NotNil(t, collr.Charts())
}

func TestCollector_Cleanup(t *testing.T) {
	assert.NotPanics(t, func() { New().Cleanup(context.Background()) })
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (collr *Collector, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid response v4.3.0": {
			prepare: preparePowerDNSAuthoritativeNSV430,
			wantCollected: map[string]int64{
				"corrupt-packets":                1,
				"cpu-iowait":                     513,
				"cpu-steal":                      1,
				"deferred-cache-inserts":         1,
				"deferred-cache-lookup":          1,
				"deferred-packetcache-inserts":   1,
				"deferred-packetcache-lookup":    1,
				"dnsupdate-answers":              1,
				"dnsupdate-changes":              1,
				"dnsupdate-queries":              1,
				"dnsupdate-refused":              1,
				"fd-usage":                       23,
				"incoming-notifications":         1,
				"key-cache-size":                 1,
				"latency":                        1,
				"meta-cache-size":                1,
				"open-tcp-connections":           1,
				"overload-drops":                 1,
				"packetcache-hit":                1,
				"packetcache-miss":               1,
				"packetcache-size":               1,
				"qsize-q":                        1,
				"query-cache-hit":                1,
				"query-cache-miss":               1,
				"query-cache-size":               1,
				"rd-queries":                     1,
				"real-memory-usage":              164507648,
				"recursing-answers":              1,
				"recursing-questions":            1,
				"recursion-unanswered":           1,
				"ring-logmessages-capacity":      10000,
				"ring-logmessages-size":          10,
				"ring-noerror-queries-capacity":  10000,
				"ring-noerror-queries-size":      1,
				"ring-nxdomain-queries-capacity": 10000,
				"ring-nxdomain-queries-size":     1,
				"ring-queries-capacity":          10000,
				"ring-queries-size":              1,
				"ring-remotes-capacity":          10000,
				"ring-remotes-corrupt-capacity":  10000,
				"ring-remotes-corrupt-size":      1,
				"ring-remotes-size":              1,
				"ring-remotes-unauth-capacity":   10000,
				"ring-remotes-unauth-size":       1,
				"ring-servfail-queries-capacity": 10000,
				"ring-servfail-queries-size":     1,
				"ring-unauth-queries-capacity":   10000,
				"ring-unauth-queries-size":       1,
				"security-status":                1,
				"servfail-packets":               1,
				"signature-cache-size":           1,
				"signatures":                     1,
				"sys-msec":                       128,
				"tcp-answers":                    1,
				"tcp-answers-bytes":              1,
				"tcp-queries":                    1,
				"tcp4-answers":                   1,
				"tcp4-answers-bytes":             1,
				"tcp4-queries":                   1,
				"tcp6-answers":                   1,
				"tcp6-answers-bytes":             1,
				"tcp6-queries":                   1,
				"timedout-packets":               1,
				"udp-answers":                    1,
				"udp-answers-bytes":              1,
				"udp-do-queries":                 1,
				"udp-in-errors":                  1,
				"udp-noport-errors":              1,
				"udp-queries":                    1,
				"udp-recvbuf-errors":             1,
				"udp-sndbuf-errors":              1,
				"udp4-answers":                   1,
				"udp4-answers-bytes":             1,
				"udp4-queries":                   1,
				"udp6-answers":                   1,
				"udp6-answers-bytes":             1,
				"udp6-queries":                   1,
				"uptime":                         207,
				"user-msec":                      56,
			},
		},
		"fails on response from PowerDNS Recursor": {
			prepare: preparePowerDNSAuthoritativeNSRecursorData,
		},
		"fails on 404 response": {
			prepare: preparePowerDNSAuthoritativeNS404,
		},
		"fails on connection refused": {
			prepare: preparePowerDNSAuthoritativeNSConnectionRefused,
		},
		"fails on response with invalid data": {
			prepare: preparePowerDNSAuthoritativeNSInvalidData,
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

func preparePowerDNSAuthoritativeNSV430() (*Collector, func()) {
	srv := preparePowerDNSAuthoritativeNSEndpoint()
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSAuthoritativeNSRecursorData() (*Collector, func()) {
	srv := preparePowerDNSRecursorEndpoint()
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSAuthoritativeNSInvalidData() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSAuthoritativeNS404() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSAuthoritativeNSConnectionRefused() (*Collector, func()) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001"

	return collr, func() {}
}

func preparePowerDNSAuthoritativeNSEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathLocalStatistics:
				_, _ = w.Write(dataVer430statistics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}

func preparePowerDNSRecursorEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathLocalStatistics:
				_, _ = w.Write(dataRecursorStatistics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}
