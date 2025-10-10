// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

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

	dataVer431statistics, _        = os.ReadFile("testdata/v4.3.1/statistics.json")
	dataAuthoritativeStatistics, _ = os.ReadFile("testdata/authoritative/statistics.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":              dataConfigJSON,
		"dataConfigYAML":              dataConfigYAML,
		"dataVer431statistics":        dataVer431statistics,
		"dataAuthoritativeStatistics": dataAuthoritativeStatistics,
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
		"success on valid response v4.3.1": {
			prepare: preparePowerDNSRecursorV431,
		},
		"fails on response from PowerDNS Authoritative Server": {
			wantFail: true,
			prepare:  preparePowerDNSRecursorAuthoritativeData,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  preparePowerDNSRecursor404,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  preparePowerDNSRecursorConnectionRefused,
		},
		"fails on response with invalid data": {
			wantFail: true,
			prepare:  preparePowerDNSRecursorInvalidData,
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
		"success on valid response v4.3.1": {
			prepare: preparePowerDNSRecursorV431,
			wantCollected: map[string]int64{
				"all-outqueries":                41,
				"answers-slow":                  1,
				"answers0-1":                    1,
				"answers1-10":                   1,
				"answers10-100":                 1,
				"answers100-1000":               1,
				"auth-zone-queries":             1,
				"auth4-answers-slow":            1,
				"auth4-answers0-1":              1,
				"auth4-answers1-10":             5,
				"auth4-answers10-100":           35,
				"auth4-answers100-1000":         1,
				"auth6-answers-slow":            1,
				"auth6-answers0-1":              1,
				"auth6-answers1-10":             1,
				"auth6-answers10-100":           1,
				"auth6-answers100-1000":         1,
				"cache-entries":                 171,
				"cache-hits":                    1,
				"cache-misses":                  1,
				"case-mismatches":               1,
				"chain-resends":                 1,
				"client-parse-errors":           1,
				"concurrent-queries":            1,
				"cpu-msec-thread-0":             439,
				"cpu-msec-thread-1":             445,
				"cpu-msec-thread-2":             466,
				"dlg-only-drops":                1,
				"dnssec-authentic-data-queries": 1,
				"dnssec-check-disabled-queries": 1,
				"dnssec-queries":                1,
				"dnssec-result-bogus":           1,
				"dnssec-result-indeterminate":   1,
				"dnssec-result-insecure":        1,
				"dnssec-result-nta":             1,
				"dnssec-result-secure":          5,
				"dnssec-validations":            5,
				"dont-outqueries":               1,
				"ecs-queries":                   1,
				"ecs-responses":                 1,
				"edns-ping-matches":             1,
				"edns-ping-mismatches":          1,
				"empty-queries":                 1,
				"failed-host-entries":           1,
				"fd-usage":                      32,
				"ignored-packets":               1,
				"ipv6-outqueries":               1,
				"ipv6-questions":                1,
				"malloc-bytes":                  1,
				"max-cache-entries":             1000000,
				"max-mthread-stack":             1,
				"max-packetcache-entries":       500000,
				"negcache-entries":              1,
				"no-packet-error":               1,
				"noedns-outqueries":             1,
				"noerror-answers":               1,
				"noping-outqueries":             1,
				"nsset-invalidations":           1,
				"nsspeeds-entries":              78,
				"nxdomain-answers":              1,
				"outgoing-timeouts":             1,
				"outgoing4-timeouts":            1,
				"outgoing6-timeouts":            1,
				"over-capacity-drops":           1,
				"packetcache-entries":           1,
				"packetcache-hits":              1,
				"packetcache-misses":            1,
				"policy-drops":                  1,
				"policy-result-custom":          1,
				"policy-result-drop":            1,
				"policy-result-noaction":        1,
				"policy-result-nodata":          1,
				"policy-result-nxdomain":        1,
				"policy-result-truncate":        1,
				"qa-latency":                    1,
				"qname-min-fallback-success":    1,
				"query-pipe-full-drops":         1,
				"questions":                     1,
				"real-memory-usage":             44773376,
				"rebalanced-queries":            1,
				"resource-limits":               1,
				"security-status":               3,
				"server-parse-errors":           1,
				"servfail-answers":              1,
				"spoof-prevents":                1,
				"sys-msec":                      1520,
				"tcp-client-overflow":           1,
				"tcp-clients":                   1,
				"tcp-outqueries":                1,
				"tcp-questions":                 1,
				"throttle-entries":              1,
				"throttled-out":                 1,
				"throttled-outqueries":          1,
				"too-old-drops":                 1,
				"truncated-drops":               1,
				"udp-in-errors":                 1,
				"udp-noport-errors":             1,
				"udp-recvbuf-errors":            1,
				"udp-sndbuf-errors":             1,
				"unauthorized-tcp":              1,
				"unauthorized-udp":              1,
				"unexpected-packets":            1,
				"unreachables":                  1,
				"uptime":                        1624,
				"user-msec":                     465,
				"variable-responses":            1,
				"x-our-latency":                 1,
				"x-ourtime-slow":                1,
				"x-ourtime0-1":                  1,
				"x-ourtime1-2":                  1,
				"x-ourtime16-32":                1,
				"x-ourtime2-4":                  1,
				"x-ourtime4-8":                  1,
				"x-ourtime8-16":                 1,
			},
		},
		"fails on response from PowerDNS Authoritative Server": {
			prepare: preparePowerDNSRecursorAuthoritativeData,
		},
		"fails on 404 response": {
			prepare: preparePowerDNSRecursor404,
		},
		"fails on connection refused": {
			prepare: preparePowerDNSRecursorConnectionRefused,
		},
		"fails on response with invalid data": {
			prepare: preparePowerDNSRecursorInvalidData,
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

func preparePowerDNSRecursorV431() (*Collector, func()) {
	srv := preparePowerDNSRecursorEndpoint()
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSRecursorAuthoritativeData() (*Collector, func()) {
	srv := preparePowerDNSAuthoritativeEndpoint()
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSRecursorInvalidData() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSRecursor404() (*Collector, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func preparePowerDNSRecursorConnectionRefused() (*Collector, func()) {
	collr := New()
	collr.URL = "http://127.0.0.1:38001"

	return collr, func() {}
}

func preparePowerDNSRecursorEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathLocalStatistics:
				_, _ = w.Write(dataVer431statistics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}

func preparePowerDNSAuthoritativeEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathLocalStatistics:
				_, _ = w.Write(dataAuthoritativeStatistics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}
