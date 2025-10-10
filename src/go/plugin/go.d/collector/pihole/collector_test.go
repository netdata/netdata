// SPDX-License-Identifier: GPL-3.0-or-later

package pihole

import (
	"context"
	"encoding/json"
	"io"
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

	dataStatsSummary, _ = os.ReadFile("testdata/v6.0.5/stats_summary.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":   dataConfigJSON,
		"dataConfigYAML":   dataConfigYAML,
		"dataStatsSummary": dataStatsSummary,
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
		"fails with default": {
			wantFail: true,
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
		"fail when password not set": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1", Password: ""},
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
		"case success": {
			wantFail: false,
			prepare:  caseSuccess,
		},
		"case wrong password": {
			wantFail: true,
			prepare:  caseWrongPassword,
		},
		"case error on stats summary": {
			wantFail: true,
			prepare:  caseErrOnStatsSummary,
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
		"case success": {
			prepare:       caseSuccess,
			wantNumCharts: len(summaryCharts),
			wantMetrics: map[string]int64{
				"clients_active":                        2,
				"clients_total":                         2,
				"gravity_domains_being_blocked":         131270,
				"gravity_last_update":                   1741494842,
				"gravity_last_update_seconds_ago":       107202,
				"queries_blocked":                       1,
				"queries_cached":                        204,
				"queries_forwarded":                     45,
				"queries_frequency":                     0,
				"queries_percent_blocked":               1100,
				"queries_replies_BLOB":                  1,
				"queries_replies_CNAME":                 1,
				"queries_replies_DNSSEC":                1,
				"queries_replies_DOMAIN":                72,
				"queries_replies_IP":                    124,
				"queries_replies_NODATA":                49,
				"queries_replies_NONE":                  1,
				"queries_replies_NOTIMP":                1,
				"queries_replies_NXDOMAIN":              4,
				"queries_replies_OTHER":                 1,
				"queries_replies_REFUSED":               1,
				"queries_replies_RRNAME":                1,
				"queries_replies_SERVFAIL":              1,
				"queries_replies_UNKNOWN":               1,
				"queries_status_CACHE":                  121,
				"queries_status_CACHE_STALE":            83,
				"queries_status_DBBUSY":                 1,
				"queries_status_DENYLIST":               1,
				"queries_status_DENYLIST_CNAME":         1,
				"queries_status_EXTERNAL_BLOCKED_EDE15": 1,
				"queries_status_EXTERNAL_BLOCKED_IP":    1,
				"queries_status_EXTERNAL_BLOCKED_NULL":  1,
				"queries_status_EXTERNAL_BLOCKED_NXRA":  1,
				"queries_status_FORWARDED":              45,
				"queries_status_GRAVITY":                1,
				"queries_status_GRAVITY_CNAME":          1,
				"queries_status_IN_PROGRESS":            1,
				"queries_status_REGEX":                  1,
				"queries_status_REGEX_CNAME":            1,
				"queries_status_RETRIED":                1,
				"queries_status_RETRIED_DNSSEC":         1,
				"queries_status_SPECIAL_DOMAIN":         1,
				"queries_status_UNKNOWN":                1,
				"queries_total":                         249,
				"queries_types_A":                       84,
				"queries_types_AAAA":                    84,
				"queries_types_ANY":                     1,
				"queries_types_DNSKEY":                  1,
				"queries_types_DS":                      1,
				"queries_types_HTTPS":                   1,
				"queries_types_MX":                      1,
				"queries_types_NAPTR":                   1,
				"queries_types_NS":                      1,
				"queries_types_OTHER":                   1,
				"queries_types_PTR":                     73,
				"queries_types_RRSIG":                   1,
				"queries_types_SOA":                     1,
				"queries_types_SRV":                     8,
				"queries_types_SVCB":                    1,
				"queries_types_TXT":                     1,
				"queries_unique_domains":                29,
			},
		},
		"case wrong password": {
			prepare:       caseWrongPassword,
			wantNumCharts: len(summaryCharts),
		},
		"case error on stats summary": {
			prepare:       caseErrOnStatsSummary,
			wantNumCharts: len(summaryCharts),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			copyBlockListLastUpdate(mx, test.wantMetrics)

			require.Equal(t, test.wantMetrics, mx)

			assert.Len(t, *collr.Charts(), test.wantNumCharts)
			if len(test.wantMetrics) > 0 {
				module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
			}
		})
	}
}

func caseSuccess(t *testing.T) (collr *Collector, cleanup func()) {
	collr, mock := New(), mockPiholeServer{password: "secret"}
	srv := mock.newPiholeHTTPServer()
	collr.URL = srv.URL
	collr.Password = mock.password

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseWrongPassword(t *testing.T) (collr *Collector, cleanup func()) {
	collr, mock := New(), mockPiholeServer{password: "secret"}
	srv := mock.newPiholeHTTPServer()
	collr.URL = srv.URL
	collr.Password = mock.password + "!"

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func caseErrOnStatsSummary(t *testing.T) (collr *Collector, cleanup func()) {
	collr, mock := New(), mockPiholeServer{password: "secret", errOnStatsSummary: true}
	srv := mock.newPiholeHTTPServer()
	collr.URL = srv.URL
	collr.Password = mock.password

	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

type mockPiholeServer struct {
	password          string
	errOnStatsSummary bool
}

func (m mockPiholeServer) newPiholeHTTPServer() *httptest.Server {
	const (
		ftlSid  = "ftl-sid"
		ftlCsrf = "ftl-csrf"
	)

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case urlPathAPIAuth:
			switch r.Method {
			case http.MethodGet:
				if r.Header.Get("X-FTL-SID") != ftlSid || r.Header.Get("X-FTL-CSRF") != ftlCsrf {
					var resp ftlErrorResponse
					resp.Error.Key = "unauthorized"
					resp.Error.Message = "Unauthorized"
					w.WriteHeader(http.StatusUnauthorized)
					bs, _ := json.Marshal(resp)
					_, _ = w.Write(bs)
					return
				}

				var resp ftlAPIAuthResponse
				resp.Session.Valid = true
				resp.Session.Sid = ftlSid
				resp.Session.Csrf = ftlCsrf
				bs, _ := json.Marshal(resp)
				_, _ = w.Write(bs)
			case http.MethodPost:
				bs, err := io.ReadAll(r.Body)
				if err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				var pass struct {
					Password string `json:"password"`
				}
				if err := json.Unmarshal(bs, &pass); err != nil {
					w.WriteHeader(http.StatusBadRequest)
					return
				}

				if pass.Password != m.password {
					var resp ftlAPIAuthResponse
					w.WriteHeader(http.StatusUnauthorized)
					bs, _ := json.Marshal(resp)
					_, _ = w.Write(bs)
					return
				}

				var resp ftlAPIAuthResponse
				resp.Session.Valid = true
				resp.Session.Sid = ftlSid
				resp.Session.Csrf = ftlCsrf
				bs, _ = json.Marshal(resp)
				_, _ = w.Write(bs)
			}
		case urlPathAPIStatsSummary:
			if m.errOnStatsSummary {
				w.WriteHeader(http.StatusBadRequest)
				return
			}
			if r.Header.Get("X-FTL-SID") != ftlSid || r.Header.Get("X-FTL-CSRF") != ftlCsrf {
				var resp ftlErrorResponse
				resp.Error.Key = "unauthorized"
				resp.Error.Message = "Unauthorized"
				w.WriteHeader(http.StatusUnauthorized)
				bs, _ := json.Marshal(resp)
				_, _ = w.Write(bs)
				return
			}
			_, _ = w.Write(dataStatsSummary)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
}

func copyBlockListLastUpdate(dst, src map[string]int64) {
	k := "gravity_last_update_seconds_ago"
	if v, ok := src[k]; ok {
		if _, ok := dst[k]; ok {
			dst[k] = v
		}
	}
}
