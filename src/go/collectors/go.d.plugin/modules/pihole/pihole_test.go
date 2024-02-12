// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pathSetupVarsOK    = "testdata/setupVars.conf"
	pathSetupVarsWrong = "testdata/wrong.conf"
)

var (
	dataEmptyResp                     = []byte("[]")
	dataSummaryRawResp, _             = os.ReadFile("testdata/summaryRaw.json")
	dataGetQueryTypesResp, _          = os.ReadFile("testdata/getQueryTypes.json")
	dataGetForwardDestinationsResp, _ = os.ReadFile("testdata/getForwardDestinations.json")
)

func TestPihole_Init(t *testing.T) {
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
			p := New()
			p.Config = test.config

			if test.wantFail {
				assert.False(t, p.Init())
			} else {
				assert.True(t, p.Init())
			}
		})
	}
}

func TestPihole_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (p *Pihole, cleanup func())
	}{
		"success with web password": {
			wantFail: false,
			prepare:  caseSuccessWithWebPassword,
		},
		"fail without web password": {
			wantFail: true,
			prepare:  caseFailNoWebPassword,
		},
		"fail on unsupported version": {
			wantFail: true,
			prepare:  caseFailUnsupportedVersion,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.False(t, p.Check())
			} else {
				assert.True(t, p.Check())
			}
		})
	}
}

func TestPihole_Charts(t *testing.T) {
	assert.NotNil(t, New().Charts())
}

func TestPihole_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) (p *Pihole, cleanup func())
		wantMetrics   map[string]int64
		wantNumCharts int
	}{
		"success with web password": {
			prepare:       caseSuccessWithWebPassword,
			wantNumCharts: len(baseCharts) + 2,
			wantMetrics: map[string]int64{
				"A":                        1229,
				"AAAA":                     1229,
				"ANY":                      100,
				"PTR":                      7143,
				"SOA":                      100,
				"SRV":                      100,
				"TXT":                      100,
				"ads_blocked_today":        1,
				"ads_blocked_today_perc":   33333,
				"ads_percentage_today":     100,
				"blocking_status_disabled": 0,
				"blocking_status_enabled":  1,
				"blocklist_last_update":    106273651,
				"destination_blocked":      220,
				"destination_cached":       8840,
				"destination_other":        940,
				"dns_queries_today":        1,
				"domains_being_blocked":    1,
				"queries_cached":           1,
				"queries_cached_perc":      33333,
				"queries_forwarded":        1,
				"queries_forwarded_perc":   33333,
				"unique_clients":           1,
			},
		},
		"fail without web password": {
			prepare:     caseFailNoWebPassword,
			wantMetrics: nil,
		},
		"fail on unsupported version": {
			prepare:     caseFailUnsupportedVersion,
			wantMetrics: nil,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			p, cleanup := test.prepare(t)
			defer cleanup()

			mx := p.Collect()

			copyBlockListLastUpdate(mx, test.wantMetrics)
			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *p.Charts(), test.wantNumCharts)
			}
		})
	}
}

func caseSuccessWithWebPassword(t *testing.T) (*Pihole, func()) {
	p, srv := New(), mockPiholeServer{}.newPiholeHTTPServer()

	p.SetupVarsPath = pathSetupVarsOK
	p.URL = srv.URL

	require.True(t, p.Init())

	return p, srv.Close
}

func caseFailNoWebPassword(t *testing.T) (*Pihole, func()) {
	p, srv := New(), mockPiholeServer{}.newPiholeHTTPServer()

	p.SetupVarsPath = pathSetupVarsWrong
	p.URL = srv.URL

	require.True(t, p.Init())

	return p, srv.Close
}

func caseFailUnsupportedVersion(t *testing.T) (*Pihole, func()) {
	p, srv := New(), mockPiholeServer{unsupportedVersion: true}.newPiholeHTTPServer()

	p.SetupVarsPath = pathSetupVarsOK
	p.URL = srv.URL

	require.True(t, p.Init())

	return p, srv.Close
}

type mockPiholeServer struct {
	unsupportedVersion bool
	errOnAPIVersion    bool
	errOnSummary       bool
	errOnQueryTypes    bool
	errOnGetForwardDst bool
	errOnTopClients    bool
	errOnTopItems      bool
}

func (m mockPiholeServer) newPiholeHTTPServer() *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != urlPathAPI || len(r.URL.Query()) == 0 {
			w.WriteHeader(http.StatusBadRequest)
		}

		if r.URL.Query().Get(urlQueryKeyAuth) == "" {
			_, _ = w.Write(dataEmptyResp)
			return
		}

		if r.URL.Query().Has(urlQueryKeyAPIVersion) {
			if m.errOnAPIVersion {
				w.WriteHeader(http.StatusNotFound)
			} else if m.unsupportedVersion {
				_, _ = w.Write([]byte(fmt.Sprintf(`{"version": %d}`, wantAPIVersion+1)))
			} else {
				_, _ = w.Write([]byte(fmt.Sprintf(`{"version": %d}`, wantAPIVersion)))
			}
			return
		}

		if r.URL.Query().Has(urlQueryKeySummaryRaw) {
			if m.errOnSummary {
				w.WriteHeader(http.StatusNotFound)
			} else {
				_, _ = w.Write(dataSummaryRawResp)
			}
			return
		}

		data := dataEmptyResp
		isErr := false
		switch {
		case r.URL.Query().Has(urlQueryKeyGetQueryTypes):
			data, isErr = dataGetQueryTypesResp, m.errOnQueryTypes
		case r.URL.Query().Has(urlQueryKeyGetForwardDestinations):
			data, isErr = dataGetForwardDestinationsResp, m.errOnGetForwardDst
		}

		if isErr {
			w.WriteHeader(http.StatusNotFound)
		} else {
			_, _ = w.Write(data)
		}
	}))
}

func copyBlockListLastUpdate(dst, src map[string]int64) {
	k := "blocklist_last_update"
	if v, ok := src[k]; ok {
		if _, ok := dst[k]; ok {
			dst[k] = v
		}
	}
}
