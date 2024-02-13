// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns_recursor

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/tlscfg"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	v431statistics, _          = os.ReadFile("testdata/v4.3.1/statistics.json")
	authoritativeStatistics, _ = os.ReadFile("testdata/authoritative/statistics.json")
)

func Test_testDataIsCorrectlyReadAndValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"v431statistics":          v431statistics,
		"authoritativeStatistics": authoritativeStatistics,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*Recursor)(nil), New())
}

func TestRecursor_Init(t *testing.T) {
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
				HTTP: web.HTTP{
					Request: web.Request{URL: ""},
				},
			},
		},
		"fails on invalid TLSCA": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{
						URL: "http://127.0.0.1:38001",
					},
					Client: web.Client{
						TLSConfig: tlscfg.TLSConfig{TLSCA: "testdata/tls"},
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			recursor := New()
			recursor.Config = test.config

			if test.wantFail {
				assert.False(t, recursor.Init())
			} else {
				assert.True(t, recursor.Init())
			}
		})
	}
}

func TestRecursor_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (r *Recursor, cleanup func())
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
			recursor, cleanup := test.prepare()
			defer cleanup()
			require.True(t, recursor.Init())

			if test.wantFail {
				assert.False(t, recursor.Check())
			} else {
				assert.True(t, recursor.Check())
			}
		})
	}
}

func TestRecursor_Charts(t *testing.T) {
	recursor := New()
	require.True(t, recursor.Init())
	assert.NotNil(t, recursor.Charts())
}

func TestRecursor_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestRecursor_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (r *Recursor, cleanup func())
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
			recursor, cleanup := test.prepare()
			defer cleanup()
			require.True(t, recursor.Init())

			collected := recursor.Collect()

			assert.Equal(t, test.wantCollected, collected)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, recursor, collected)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, rec *Recursor, collected map[string]int64) {
	for _, chart := range *rec.Charts() {
		if chart.Obsolete {
			continue
		}
		for _, dim := range chart.Dims {
			_, ok := collected[dim.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", dim.ID, chart.ID)
		}
		for _, v := range chart.Vars {
			_, ok := collected[v.ID]
			assert.Truef(t, ok, "chart '%s' dim '%s': no dim in collected", v.ID, chart.ID)
		}
	}
}

func preparePowerDNSRecursorV431() (*Recursor, func()) {
	srv := preparePowerDNSRecursorEndpoint()
	recursor := New()
	recursor.URL = srv.URL

	return recursor, srv.Close
}

func preparePowerDNSRecursorAuthoritativeData() (*Recursor, func()) {
	srv := preparePowerDNSAuthoritativeEndpoint()
	recursor := New()
	recursor.URL = srv.URL

	return recursor, srv.Close
}

func preparePowerDNSRecursorInvalidData() (*Recursor, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	recursor := New()
	recursor.URL = srv.URL

	return recursor, srv.Close
}

func preparePowerDNSRecursor404() (*Recursor, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	recursor := New()
	recursor.URL = srv.URL

	return recursor, srv.Close
}

func preparePowerDNSRecursorConnectionRefused() (*Recursor, func()) {
	recursor := New()
	recursor.URL = "http://127.0.0.1:38001"

	return recursor, func() {}
}

func preparePowerDNSRecursorEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case urlPathLocalStatistics:
				_, _ = w.Write(v431statistics)
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
				_, _ = w.Write(authoritativeStatistics)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}
