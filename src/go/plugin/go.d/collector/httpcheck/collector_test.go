// SPDX-License-Identifier: GPL-3.0-or-later

package httpcheck

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
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
		"success if url set": {
			wantFail: false,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:38001"},
				},
			},
		},
		"fail with default": {
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
		"fail if wrong response regex": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{URL: "http://127.0.0.1:38001"},
				},
				ResponseMatch: "(?:qwe))",
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
	tests := map[string]struct {
		prepare    func(t *testing.T) *Collector
		wantCharts bool
	}{
		"no charts if not inited": {
			wantCharts: false,
			prepare: func(t *testing.T) *Collector {
				return New()
			},
		},
		"charts if inited": {
			wantCharts: true,
			prepare: func(t *testing.T) *Collector {
				collr := New()
				collr.URL = "http://127.0.0.1:38001"
				require.NoError(t, collr.Init(context.Background()))

				return collr
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := test.prepare(t)

			if test.wantCharts {
				assert.NotNil(t, collr.Charts())
			} else {
				assert.Nil(t, collr.Charts())
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })

	collr.URL = "http://127.0.0.1:38001"
	require.NoError(t, collr.Init(context.Background()))
	assert.NotPanics(t, func() { collr.Cleanup(context.Background()) })
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func() (collr *Collector, cleanup func())
		wantFail bool
	}{
		"success case":       {wantFail: false, prepare: prepareSuccessCase},
		"timeout case":       {wantFail: false, prepare: prepareTimeoutCase},
		"redirect success":   {wantFail: false, prepare: prepareRedirectSuccessCase},
		"redirect fail":      {wantFail: false, prepare: prepareRedirectFailCase},
		"bad status case":    {wantFail: false, prepare: prepareBadStatusCase},
		"bad content case":   {wantFail: false, prepare: prepareBadContentCase},
		"no connection case": {wantFail: false, prepare: prepareNoConnectionCase},
		"cookie auth case":   {wantFail: false, prepare: prepareCookieAuthCase},
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
		prepare     func() (collr *Collector, cleanup func())
		update      func(check *Collector)
		wantMetrics map[string]int64
	}{
		"success case": {
			prepare: prepareSuccessCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
		"timeout case": {
			prepare: prepareTimeoutCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        0,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       1,
			},
		},
		"redirect success case": {
			prepare: prepareRedirectSuccessCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        0,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
		"redirect fail case": {
			prepare: prepareRedirectFailCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        0,
				"no_connection": 0,
				"redirect":      1,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"bad status case": {
			prepare: prepareBadStatusCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    1,
				"in_state":      2,
				"length":        0,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"bad content case": {
			prepare: prepareBadContentCase,
			wantMetrics: map[string]int64{
				"bad_content":   1,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        17,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"no connection case": {
			prepare: prepareNoConnectionCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        0,
				"no_connection": 1,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match include no value success case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Key: "header-key2"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match include with value success case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Key: "header-key2", Value: "= header-value"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match include no value bad headers case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Key: "header-key99"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    1,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match include with value bad headers case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Key: "header-key2", Value: "= header-value99"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    1,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match exclude no value success case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Exclude: true, Key: "header-key99"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match exclude with value success case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Exclude: true, Key: "header-key2", Value: "= header-value99"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match exclude no value bad headers case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Exclude: true, Key: "header-key2"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    1,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"header match exclude with value bad headers case": {
			prepare: prepareSuccessCase,
			update: func(collr *Collector) {
				collr.HeaderMatch = []headerMatchConfig{
					{Exclude: true, Key: "header-key2", Value: "= header-value"},
				}
			},
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    1,
				"bad_status":    0,
				"in_state":      2,
				"length":        5,
				"no_connection": 0,
				"redirect":      0,
				"success":       0,
				"time":          0,
				"timeout":       0,
			},
		},
		"cookie auth case": {
			prepare: prepareCookieAuthCase,
			wantMetrics: map[string]int64{
				"bad_content":   0,
				"bad_header":    0,
				"bad_status":    0,
				"in_state":      2,
				"length":        0,
				"no_connection": 0,
				"redirect":      0,
				"success":       1,
				"time":          0,
				"timeout":       0,
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare()
			defer cleanup()

			if test.update != nil {
				test.update(collr)
			}

			require.NoError(t, collr.Init(context.Background()))

			var mx map[string]int64

			for i := 0; i < 2; i++ {
				mx = collr.Collect(context.Background())
				time.Sleep(time.Duration(collr.UpdateEvery) * time.Second)
			}

			copyResponseTime(test.wantMetrics, mx)

			require.Equal(t, test.wantMetrics, mx)
		})
	}
}

func prepareSuccessCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.ResponseMatch = "match"

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("header-key1", "header-value")
			w.Header().Set("header-key2", "header-value")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("match"))
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareTimeoutCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.Timeout = confopt.Duration(time.Millisecond * 100)

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(collr.Timeout.Duration() + time.Millisecond*100)
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareRedirectSuccessCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.NotFollowRedirect = true
	collr.AcceptedStatuses = []int{301}

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com", http.StatusMovedPermanently)
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareRedirectFailCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.NotFollowRedirect = true

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			http.Redirect(w, r, "https://example.com", http.StatusMovedPermanently)
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareBadStatusCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusBadGateway)
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareBadContentCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.ResponseMatch = "no match"

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello and goodbye"))
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func prepareNoConnectionCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.URL = "http://127.0.0.1:38001"

	return collr, func() {}
}

func prepareCookieAuthCase() (*Collector, func()) {
	collr := New()
	collr.UpdateEvery = 1
	collr.CookieFile = "testdata/cookie.txt"

	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if _, err := r.Cookie("JSESSIONID"); err != nil {
				w.WriteHeader(http.StatusUnauthorized)
			} else {
				w.WriteHeader(http.StatusOK)
			}
		}))

	collr.URL = srv.URL

	return collr, srv.Close
}

func copyResponseTime(dst, src map[string]int64) {
	if v, ok := src["time"]; ok {
		if _, ok := dst["time"]; ok {
			dst["time"] = v
		}
	}
}
