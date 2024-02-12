// SPDX-License-Identifier: GPL-3.0-or-later

package dnsdist

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
	v151JSONStat, _ = os.ReadFile("testdata/v1.5.1/jsonstat.json")
)

func Test_testDataIsCorrectlyReadAndValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"v151JSONStat": v151JSONStat,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestNew(t *testing.T) {
	assert.IsType(t, (*DNSdist)(nil), New())
}

func Test_Init(t *testing.T) {
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
			ns := New()
			ns.Config = test.config

			if test.wantFail {
				assert.False(t, ns.Init())
			} else {
				assert.True(t, ns.Init())
			}
		})
	}
}

func Test_Charts(t *testing.T) {
	dist := New()
	require.True(t, dist.Init())
	assert.NotNil(t, dist.Charts())
}

func Test_Cleanup(t *testing.T) {
	assert.NotPanics(t, New().Cleanup)
}

func Test_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (dist *DNSdist, cleanup func())
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
			dist, cleanup := test.prepare()
			defer cleanup()
			require.True(t, dist.Init())

			if test.wantFail {
				assert.False(t, dist.Check())
			} else {
				assert.True(t, dist.Check())
			}
		})
	}
}

func Test_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func() (dist *DNSdist, cleanup func())
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
			dist, cleanup := test.prepare()
			defer cleanup()
			require.True(t, dist.Init())

			collected := dist.Collect()

			assert.Equal(t, test.wantCollected, collected)
			if len(test.wantCollected) > 0 {
				ensureCollectedHasAllChartsDimsVarsIDs(t, dist, collected)
			}
		})
	}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, dist *DNSdist, collected map[string]int64) {
	for _, chart := range *dist.Charts() {
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

func preparePowerDNSdistV151() (*DNSdist, func()) {
	srv := preparePowerDNSDistEndpoint()
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSdist404() (*DNSdist, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSdistConnectionRefused() (*DNSdist, func()) {
	ns := New()
	ns.URL = "http://127.0.0.1:38001"

	return ns, func() {}
}

func preparePowerDNSdistInvalidData() (*DNSdist, func()) {
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	ns := New()
	ns.URL = srv.URL

	return ns, srv.Close
}

func preparePowerDNSDistEndpoint() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.String() {
			case "/jsonstat?command=stats":
				_, _ = w.Write(v151JSONStat)
			default:
				w.WriteHeader(http.StatusNotFound)
			}
		}))
}
