// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	"context"
	"math"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var (
	dataConfigJSON, _ = os.ReadFile("testdata/config.json")
	dataConfigYAML, _ = os.ReadFile("testdata/config.yaml")
	dataMetrics, _    = os.ReadFile("testdata/metrics.txt")
)

func Test_testDataIsValid(t *testing.T) {
	for name, data := range map[string][]byte{
		"dataConfigJSON": dataConfigJSON,
		"dataConfigYAML": dataConfigYAML,
		"dataMetrics":    dataMetrics,
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
		"success with default config": {
			wantFail: false,
			config:   New().Config,
		},
		"fail when URL is empty": {
			wantFail: true,
			config: func() Config {
				cfg := New().Config
				cfg.URL = ""
				return cfg
			}(),
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr := New()
			collr.Config = test.config

			if test.wantFail {
				assert.Error(t, collr.Init(context.TODO()))
			} else {
				assert.NoError(t, collr.Init(context.TODO()))
			}
		})
	}
}

func TestCollector_Check(t *testing.T) {
	tests := map[string]struct {
		wantFail bool
		prepare  func(t *testing.T) (*Collector, func())
	}{
		"success on valid response": {
			wantFail: false,
			prepare:  caseValidMetrics,
		},
		"fail on invalid response": {
			wantFail: true,
			prepare:  caseInvalidMetrics,
		},
		"fail on connection refused": {
			wantFail: true,
			prepare:  caseConnectionRefused,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			require.NoError(t, collr.Init(context.TODO()))

			if test.wantFail {
				assert.Error(t, collr.Check(context.TODO()))
			} else {
				assert.NoError(t, collr.Check(context.TODO()))
			}
		})
	}
}

func TestCollector_Collect(t *testing.T) {
	tests := map[string]struct {
		prepare     func(t *testing.T) (*Collector, func())
		wantMetrics map[string]int64
	}{
		"success on valid response": {
			prepare:     caseValidMetrics,
			wantMetrics: map[string]int64{
				// We expect some metrics to be collected
				// Exact values depend on testdata/metrics.txt content
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			collr, cleanup := test.prepare(t)
			defer cleanup()

			require.NoError(t, collr.Init(context.TODO()))
			require.NoError(t, collr.Check(context.TODO()))

			mx := collr.Collect(context.TODO())
			assert.NotNil(t, mx)
			// Basic sanity check - we should have collected some metrics
			assert.Greater(t, len(mx), 0)

			// Verify key metrics are present
			expectedMetrics := []string{
				"request_total",
				"inflight_mutating",
				"inflight_readonly",
				"process_goroutines",
				"process_threads",
				"process_resident_memory_bytes",
				"audit_events_total",
			}

			for _, m := range expectedMetrics {
				_, ok := mx[m]
				assert.True(t, ok, "Expected metric %s not found", m)
			}

			// Verify we have dynamic dimensions (verbs/codes)
			verbCount := 0
			codeCount := 0
			for k := range mx {
				if len(k) > 15 && k[:15] == "request_by_verb" {
					verbCount++
				}
				if len(k) > 15 && k[:15] == "request_by_code" {
					codeCount++
				}
			}
			assert.Greater(t, verbCount, 0, "Expected some verb dimensions")
			assert.Greater(t, codeCount, 0, "Expected some code dimensions")

			t.Logf("Collected %d metrics, %d verbs, %d codes", len(mx), verbCount, codeCount)
		})
	}
}

func TestHistogramPercentile(t *testing.T) {
	tests := map[string]struct {
		buckets    []histogramBucket
		percentile float64
		want       float64
		wantNaN    bool
	}{
		"empty buckets returns NaN": {
			buckets:    []histogramBucket{},
			percentile: 0.5,
			wantNaN:    true,
		},
		"zero count returns NaN": {
			buckets: []histogramBucket{
				{le: 0.1, count: 0},
				{le: 0.5, count: 0},
			},
			percentile: 0.5,
			wantNaN:    true,
		},
		"p50 single bucket": {
			buckets: []histogramBucket{
				{le: 0.1, count: 100},
			},
			percentile: 0.5,
			want:       0.05, // Half of first bucket
		},
		"p50 two buckets equal distribution": {
			buckets: []histogramBucket{
				{le: 0.1, count: 50},
				{le: 0.2, count: 100},
			},
			percentile: 0.5,
			want:       0.1, // Should be at boundary
		},
		"p90 realistic distribution": {
			buckets: []histogramBucket{
				{le: 0.005, count: 80},
				{le: 0.01, count: 90},
				{le: 0.025, count: 95},
				{le: 0.05, count: 98},
				{le: 0.1, count: 100},
			},
			percentile: 0.9,
			want:       0.01, // 90% of 100 = 90, in second bucket
		},
		"p99 realistic distribution": {
			buckets: []histogramBucket{
				{le: 0.005, count: 80},
				{le: 0.01, count: 90},
				{le: 0.025, count: 95},
				{le: 0.05, count: 98},
				{le: 0.1, count: 100},
			},
			percentile: 0.99,
			want:       0.075, // 99% of 100 = 99, in last bucket
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			result := histogramPercentile(test.buckets, test.percentile)
			if test.wantNaN {
				assert.True(t, math.IsNaN(result), "Expected NaN but got %f", result)
			} else {
				assert.InDelta(t, test.want, result, 0.01, "Expected %f but got %f", test.want, result)
			}
		})
	}
}

func TestCollector_Cleanup(t *testing.T) {
	collr := New()
	require.NoError(t, collr.Init(context.TODO()))
	collr.Cleanup(context.TODO())
}

// Test case helpers

func caseValidMetrics(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write(dataMetrics)
		}))

	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func caseInvalidMetrics(t *testing.T) (*Collector, func()) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte("not prometheus metrics"))
		}))

	collr := New()
	collr.URL = srv.URL

	return collr, srv.Close
}

func caseConnectionRefused(t *testing.T) (*Collector, func()) {
	t.Helper()
	collr := New()
	collr.URL = "http://127.0.0.1:38001/metrics"

	return collr, func() {}
}
