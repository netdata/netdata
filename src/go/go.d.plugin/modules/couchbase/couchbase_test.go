// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	v660BucketsBasicStats, _ = os.ReadFile("testdata/6.6.0/buckets_basic_stats.json")
)

func TestNew(t *testing.T) {
	assert.Implements(t, (*module.Module)(nil), New())
}

func Test_testDataIsCorrectlyReadAndValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"v660BucketsBasicStats": v660BucketsBasicStats,
	} {
		require.NotNilf(t, data, name)
	}
}

func TestCouchbase_Init(t *testing.T) {
	tests := map[string]struct {
		config   Config
		wantFail bool
	}{
		"success on default config": {
			config: New().Config,
		},
		"fails on unset 'URL'": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{
						URL: "",
					},
				},
			},
		},
		"fails on invalid URL": {
			wantFail: true,
			config: Config{
				HTTP: web.HTTP{
					Request: web.Request{
						URL: "127.0.0.1:9090",
					},
				},
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cb := New()
			cb.Config = test.config

			if test.wantFail {
				assert.False(t, cb.Init())
			} else {
				assert.True(t, cb.Init())
			}
		})
	}
}

func TestCouchbase_Check(t *testing.T) {
	tests := map[string]struct {
		prepare  func(*testing.T) (cb *Couchbase, cleanup func())
		wantFail bool
	}{
		"success on valid response v6.6.0": {
			prepare: prepareCouchbaseV660,
		},
		"fails on response with invalid data": {
			wantFail: true,
			prepare:  prepareCouchbaseInvalidData,
		},
		"fails on 404 response": {
			wantFail: true,
			prepare:  prepareCouchbase404,
		},
		"fails on connection refused": {
			wantFail: true,
			prepare:  prepareCouchbaseConnectionRefused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cb, cleanup := test.prepare(t)
			defer cleanup()

			if test.wantFail {
				assert.False(t, cb.Check())
			} else {
				assert.True(t, cb.Check())
			}
		})
	}
}

func TestCouchbase_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) (cb *Couchbase, cleanup func())
		wantCollected map[string]int64
	}{
		"success on valid response v6.6.0": {
			prepare: prepareCouchbaseV660,
			wantCollected: map[string]int64{
				"bucket_beer-sample_data_used":                     13990431,
				"bucket_beer-sample_disk_fetches":                  1,
				"bucket_beer-sample_disk_used":                     27690472,
				"bucket_beer-sample_item_count":                    7303,
				"bucket_beer-sample_mem_used":                      34294872,
				"bucket_beer-sample_ops_per_sec":                   1100,
				"bucket_beer-sample_quota_percent_used":            32706,
				"bucket_beer-sample_vb_active_num_non_resident":    1,
				"bucket_gamesim-sample_data_used":                  5371804,
				"bucket_gamesim-sample_disk_fetches":               1,
				"bucket_gamesim-sample_disk_used":                  13821793,
				"bucket_gamesim-sample_item_count":                 586,
				"bucket_gamesim-sample_mem_used":                   29586696,
				"bucket_gamesim-sample_ops_per_sec":                1100,
				"bucket_gamesim-sample_quota_percent_used":         28216,
				"bucket_gamesim-sample_vb_active_num_non_resident": 1,
				"bucket_travel-sample_data_used":                   53865472,
				"bucket_travel-sample_disk_fetches":                1,
				"bucket_travel-sample_disk_used":                   62244260,
				"bucket_travel-sample_item_count":                  31591,
				"bucket_travel-sample_mem_used":                    54318184,
				"bucket_travel-sample_ops_per_sec":                 1100,
				"bucket_travel-sample_quota_percent_used":          51801,
				"bucket_travel-sample_vb_active_num_non_resident":  1,
			},
		},
		"fails on response with invalid data": {
			prepare: prepareCouchbaseInvalidData,
		},
		"fails on 404 response": {
			prepare: prepareCouchbase404,
		},
		"fails on connection refused": {
			prepare: prepareCouchbaseConnectionRefused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			cb, cleanup := test.prepare(t)
			defer cleanup()

			collected := cb.Collect()

			assert.Equal(t, test.wantCollected, collected)
			ensureCollectedHasAllChartsDimsVarsIDs(t, cb, collected)
		})
	}
}

func prepareCouchbaseV660(t *testing.T) (cb *Couchbase, cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(v660BucketsBasicStats)
		}))

	cb = New()
	cb.URL = srv.URL
	require.True(t, cb.Init())

	return cb, srv.Close
}

func prepareCouchbaseInvalidData(t *testing.T) (*Couchbase, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	cb := New()
	cb.URL = srv.URL
	require.True(t, cb.Init())

	return cb, srv.Close
}

func prepareCouchbase404(t *testing.T) (*Couchbase, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	cb := New()
	cb.URL = srv.URL
	require.True(t, cb.Init())

	return cb, srv.Close
}

func prepareCouchbaseConnectionRefused(t *testing.T) (*Couchbase, func()) {
	t.Helper()
	cb := New()
	cb.URL = "http://127.0.0.1:38001"
	require.True(t, cb.Init())

	return cb, func() {}
}

func ensureCollectedHasAllChartsDimsVarsIDs(t *testing.T, cb *Couchbase, collected map[string]int64) {
	for _, chart := range *cb.Charts() {
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
