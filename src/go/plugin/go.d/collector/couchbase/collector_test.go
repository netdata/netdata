// SPDX-License-Identifier: GPL-3.0-or-later

package couchbase

import (
	"context"
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

	dataVer660BucketsBasicStats, _ = os.ReadFile("testdata/6.6.0/buckets_basic_stats.json")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON":              dataConfigJSON,
		"dataConfigYAML":              dataConfigYAML,
		"dataVer660BucketsBasicStats": dataVer660BucketsBasicStats,
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
		"fails on unset 'URL'": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL: "",
					},
				},
			},
		},
		"fails on invalid URL": {
			wantFail: true,
			config: Config{
				HTTPConfig: web.HTTPConfig{
					RequestConfig: web.RequestConfig{
						URL: "127.0.0.1:9090",
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
		prepare  func(*testing.T) (collr *Collector, cleanup func())
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

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare       func(t *testing.T) (collr *Collector, cleanup func())
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
			collr, cleanup := test.prepare(t)
			defer cleanup()

			mx := collr.Collect(context.Background())

			assert.Equal(t, test.wantCollected, mx)
			module.TestMetricsHasAllChartsDims(t, collr.Charts(), mx)
		})
	}
}

func prepareCouchbaseV660(t *testing.T) (collr *Collector, cleanup func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataVer660BucketsBasicStats)
		}))

	collr = New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCouchbaseInvalidData(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("hello and\n goodbye"))
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCouchbase404(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
		}))
	collr := New()
	collr.URL = srv.URL
	require.NoError(t, collr.Init(context.Background()))

	return collr, srv.Close
}

func prepareCouchbaseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:38001"
	require.NoError(t, collr.Init(context.Background()))

	return collr, func() {}
}
