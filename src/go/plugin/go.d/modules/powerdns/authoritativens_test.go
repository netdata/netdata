// SPDX-License-Identifier: GPL-3.0-or-later

package powerdns

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
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

func TestAuthoritativeNS_ConfigurationSerialize(t *testing.T) {
	module.TestConfigurationSerialize(t, &AuthoritativeNS{}, dataConfigJSON, dataConfigYAML)
}

func TestAuthoritativeNS_Init(t *testing.T) {
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
			ns := New()
			ns.Config = test.config

			if test.wantFail {
				assert.Error(t, ns.Init())
			} else {
				assert.NoError(t, ns.Init())
			}
		})
	}
}

func TestAuthoritativeNS_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (ns *AuthoritativeNS, cleanup func())
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
			ns, cleanup := test.prepare()
			defer cleanup()
			require.NoError(t, ns.Init())

			if test.wantFail {
				assert.Error(t, ns.Check())
			} else {
				assert.NoError(t, ns.Check())
			}
		})
	}
}

func TestAuthoritativeNS_Charts(t *testing.T) {
	ns := New()
	require.NoError(t, ns.Init())
	assert.NotNil(t, ns.Charts())
}

func TestAuthoritativeNS_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func TestAuthoritativeNS_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (p *AuthoritativeNS, cleanup func())
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
			ns, cleanup := test.prepare()
			defer cleanup()
			require.NoError(t, ns.Init())

			collected := ns.Collect()

			assert.Equal(t, test.wantCollected, collected)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, ns, collected)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, ns *AuthoritativeNS, collected map[string]int64) {
	for _, chart := range *ns.Charts() {
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

func preparePowerDNSAuthoritativeNSV430() (*AuthoritativeNS, func()) {
	srv := preparePowerDNSAuthoritativeNSEndpoint()
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSAuthoritativeNSRecursorData() (*AuthoritativeNS, func()) {
	srv := preparePowerDNSRecursorEndpoint()
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSAuthoritativeNSInvalidData() (*AuthoritativeNS, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSAuthoritativeNS404() (*AuthoritativeNS, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSAuthoritativeNSConnectionRefused() (*AuthoritativeNS, func()) {
	ns := New()
	ns.URL = "http://127.0.0.1:38001"

	return ns, func() {}
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
