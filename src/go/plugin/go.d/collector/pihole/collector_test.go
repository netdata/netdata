// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	pathSetupVarsOK    = "testdata/setupVars.conf"
	pathSetupVarsWrong = "testdata/wrong.conf"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")

	dataEmptyResp                     = []byte("[]")
	dataSummaryRawResp, _             = os.ReadFile("testdata/summaryRaw.json")
	dataGetQueryTypesResp, _          = os.ReadFile("testdata/getQueryTypes.json")
	dataGetForwardDestinationsResp, _ = os.ReadFile("testdata/getForwardDestinations.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":                 dataConfigJSON,
		"dataConfigYAML":                 dataConfigYAML,
		"dataEmptyResp":                  dataEmptyResp,
		"dataSummaryRawResp":             dataSummaryRawResp,
		"dataGetQueryTypesResp":          dataGetQueryTypesResp,
		"dataGetForwardDestinationsResp": dataGetForwardDestinationsResp,
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

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (collr *Collector, cleanup func())
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
		prepare       func(t *testing.T) (collr *Collector, cleanup func())
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
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			copyBlockListLastUpdate(mx, test.wantMetrics)
			require.Equal(t, test.wantMetrics, mx)
			if len(test.wantMetrics) > 0 {
				assert.Len(t, *collr.Charts(), test.wantNumCharts)
			}
		})
	}
}

func caseSuccessWithWebPassword(t *testing.T) (*Collector, func()) {
	collr, srv := New(), mockPiholeServer{}.newPiholeHTTPServer()

	collr.SetupVarsPath = pathSetupVarsOK
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseFailNoWebPassword(t *testing.T) (*Collector, func()) {
	collr, srv := New(), mockPiholeServer{}.newPiholeHTTPServer()

	collr.SetupVarsPath = pathSetupVarsWrong
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseFailUnsupportedVersion(t *testing.T) (*Collector, func()) {
	collr, srv := New(), mockPiholeServer{unsupportedVersion: true}.newPiholeHTTPServer()

	collr.SetupVarsPath = pathSetupVarsOK
	collr.URL = srv.URL

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

type mockPiholeServer struct {
	unsupportedVersion bool
	errOnAPIVersion    bool
	errOnSummary       bool
	errOnQueryTypes    bool
	errOnGetForwardDst bool
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
