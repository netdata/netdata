// SPDX-License-Identifier: GPL-3.0-or-later

package ingest

import (
	"fmt"
	"math"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	commonv1 "go.opentelemetry.io/proto/otlp/common/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcev1 "go.opentelemetry.io/proto/otlp/resource/v1"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"
)

var now = time.Unix(1700000000, 0)

func attrs(values ...string) []*commonv1.KeyValue {
	var out []*commonv1.KeyValue
	for i := 0; i < len(values); i += 2 {
		out = append(out, &commonv1.KeyValue{Key: values[i], Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_StringValue{StringValue: values[i+1]}}})
	}
	return out
}
func request(instance string, metrics ...*metricsv1.Metric) *collectorv1.ExportMetricsServiceRequest {
	return &collectorv1.ExportMetricsServiceRequest{ResourceMetrics: []*metricsv1.ResourceMetrics{{Resource: &resourcev1.Resource{Attributes: attrs("service.instance.id", instance, "service.name", "untrusted display")}, ScopeMetrics: []*metricsv1.ScopeMetrics{{Scope: &commonv1.InstrumentationScope{Name: "java-test"}, Metrics: metrics}}}}}
}

// These shapes match the pinned Java 2.32.0 recording: memory is a
// non-monotonic cumulative sum, pools gauges, HTTP an explicit histogram.
func memory() *metricsv1.Metric {
	return &metricsv1.Metric{Name: "jvm.memory.used", Unit: "By", Data: &metricsv1.Metric_Sum{Sum: &metricsv1.Sum{AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricsv1.NumberDataPoint{{Attributes: attrs("jvm.memory.type", "heap", "jvm.memory.pool.name", "Old Gen"), TimeUnixNano: uint64(now.UnixNano()), StartTimeUnixNano: uint64(now.Add(-time.Hour).UnixNano()), Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 1024}}}}}}
}
func httpMetric() *metricsv1.Metric {
	sum := 0.8
	return &metricsv1.Metric{Name: "http.server.request.duration", Unit: "s", Data: &metricsv1.Metric_Histogram{Histogram: &metricsv1.Histogram{AggregationTemporality: metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, DataPoints: []*metricsv1.HistogramDataPoint{{Attributes: append(attrs("http.request.method", "GET", "http.route", "/work"), &commonv1.KeyValue{Key: "http.response.status_code", Value: &commonv1.AnyValue{Value: &commonv1.AnyValue_IntValue{IntValue: 200}}}), TimeUnixNano: uint64(now.UnixNano()), StartTimeUnixNano: uint64(now.Add(-time.Hour).UnixNano()), Count: 4, Sum: &sum, ExplicitBounds: []float64{0.1, 0.5}, BucketCounts: []uint64{1, 2, 1}}}}}}
}
func TestAdmissionFreshnessAndCopies(t *testing.T) {
	s := New()
	r, err := s.Ingest(request("a", memory()), now)
	require.Error(t, err)
	assert.Equal(t, Result{Rejected: 1}, r)
	s.Admit("a", "orders")
	s.Admit("b", "billing")
	for _, id := range []string{"a", "b"} {
		r, err = s.Ingest(request(id, memory()), now)
		require.NoError(t, err)
		assert.Equal(t, 1, r.Accepted)
	}
	got := s.Snapshot(now, 5*time.Second)
	require.Len(t, got, 2)
	assert.Equal(t, "orders", got[0].Application)
	assert.Equal(t, now, got[0].LastSeen)
	assert.Equal(t, float64(1024), got[0].Samples[0].Value)
	got[0].Samples[0].Labels["memory_type"] = "mutated"
	assert.Equal(t, "heap", s.Snapshot(now, 5*time.Second)[0].Samples[0].Labels["memory_type"])
	s.Admit("a", "orders")
	require.Len(t, s.Snapshot(now, 5*time.Second), 2)
	_, err = s.Ingest(request("a", memory()), now.Add(4*time.Second))
	require.Error(t, err)
	assert.Empty(t, s.Snapshot(now.Add(6*time.Second), 5*time.Second))
	older := memory()
	older.GetSum().DataPoints[0].TimeUnixNano -= uint64(time.Second)
	_, err = s.Ingest(request("a", older), now.Add(7*time.Second))
	require.Error(t, err)
	s.Remove("a")
	s.Admit("new-a", "orders")
	_, err = s.Ingest(request("a", memory()), now)
	require.Error(t, err)
	assert.Empty(t, s.Snapshot(now.Add(6*time.Second), 5*time.Second))
}
func TestHistogramConversionAndEpoch(t *testing.T) {
	s := New()
	s.Admit("a", "orders")
	m := httpMetric()
	_, err := s.Ingest(request("a", m), now)
	require.NoError(t, err)
	got := s.Snapshot(now, time.Second)[0].Samples[0]
	require.NotNil(t, got.Histogram)
	assert.Equal(t, float64(4), got.Histogram.Count)
	assert.Equal(t, 0.8, got.Histogram.Sum)
	require.Len(t, got.Histogram.Buckets, 2)
	assert.Equal(t, float64(1), got.Histogram.Buckets[0].CumulativeCount)
	assert.Equal(t, float64(3), got.Histogram.Buckets[1].CumulativeCount)
	got.Histogram.Buckets[0].CumulativeCount = 999
	assert.Equal(t, float64(1), s.Snapshot(now, time.Second)[0].Samples[0].Histogram.Buckets[0].CumulativeCount)
	reset := proto.Clone(m).(*metricsv1.Metric)
	p := reset.GetHistogram().DataPoints[0]
	p.TimeUnixNano += uint64(time.Second)
	p.StartTimeUnixNano = uint64(now.UnixNano())
	p.Count = 1
	p.BucketCounts = []uint64{0, 1, 0}
	v := 0.2
	p.Sum = &v
	_, err = s.Ingest(request("a", reset), now.Add(time.Second))
	require.NoError(t, err)
	updated := s.Snapshot(now.Add(time.Second), 5*time.Second)[0].Samples[0]
	assert.NotEqual(t, got.Labels["source_epoch"], updated.Labels["source_epoch"])
	assert.Equal(t, float64(1), updated.Histogram.Count)
	m.GetHistogram().DataPoints[0].TimeUnixNano = uint64(now.Add(2 * time.Second).UnixNano())
	_, err = s.Ingest(request("a", m), now.Add(2*time.Second))
	require.Error(t, err)
	assert.Equal(t, updated.SourceTime, s.Snapshot(now.Add(2*time.Second), 5*time.Second)[0].Samples[0].SourceTime)
}
func TestRejectMalformedKnownFamilies(t *testing.T) {
	tests := map[string]func(*metricsv1.Metric){
		"unit": func(m *metricsv1.Metric) { m.Unit = "ms" },
		"delta": func(m *metricsv1.Metric) {
			m.GetHistogram().AggregationTemporality = metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
		},
		"exponential": func(m *metricsv1.Metric) {
			m.Data = &metricsv1.Metric_ExponentialHistogram{ExponentialHistogram: &metricsv1.ExponentialHistogram{}}
		},
		"bucket shape": func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].BucketCounts = []uint64{4} },
		"bucket count": func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].Count = 5 },
		"bucket overflow": func(m *metricsv1.Metric) {
			m.GetHistogram().DataPoints[0].BucketCounts = []uint64{math.MaxUint64, 2, 3}
		},
		"bounds order":      func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].ExplicitBounds = []float64{0.5, 0.1} },
		"bounds finite":     func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].ExplicitBounds[0] = math.NaN() },
		"missing sum":       func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].Sum = nil },
		"nonfinite sum":     func(m *metricsv1.Metric) { v := math.Inf(1); m.GetHistogram().DataPoints[0].Sum = &v },
		"missing epoch":     func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].StartTimeUnixNano = 0 },
		"missing timestamp": func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].TimeUnixNano = 0 },
		"future timestamp":  func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].TimeUnixNano += uint64(time.Second) },
		"epoch after timestamp": func(m *metricsv1.Metric) {
			m.GetHistogram().DataPoints[0].StartTimeUnixNano = uint64(now.Add(time.Second).UnixNano())
		},
		"no recorded value": func(m *metricsv1.Metric) { m.GetHistogram().DataPoints[0].Flags = 1 },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			s := New()
			s.Admit("a", "orders")
			m := httpMetric()
			mutate(m)
			r, err := s.Ingest(request("a", m, memory()), now)
			require.Error(t, err)
			assert.Equal(t, Result{Accepted: 1, Rejected: 1}, r)
			require.Len(t, s.Snapshot(now, time.Second)[0].Samples, 1)
		})
	}
}
func TestAmbiguityAndBounds(t *testing.T) {
	t.Run("duplicate projected identity", func(t *testing.T) {
		s := New()
		s.Admit("a", "orders")
		a, b := httpMetric(), httpMetric()
		b.GetHistogram().DataPoints[0].Attributes = append(b.GetHistogram().DataPoints[0].Attributes, attrs("url.scheme", "https")...)
		r, err := s.Ingest(request("a", a, b), now)
		require.Error(t, err)
		assert.Equal(t, 2, r.Rejected)
		assert.Empty(t, s.Snapshot(now, time.Second))
	})
	t.Run("origin changed", func(t *testing.T) {
		s := New()
		s.Admit("a", "orders")
		_, err := s.Ingest(request("a", httpMetric()), now)
		require.NoError(t, err)
		m := httpMetric()
		m.GetHistogram().DataPoints[0].TimeUnixNano += uint64(time.Second)
		m.GetHistogram().DataPoints[0].Attributes = append(m.GetHistogram().DataPoints[0].Attributes, attrs("url.scheme", "https")...)
		_, err = s.Ingest(request("a", m), now.Add(time.Second))
		require.Error(t, err)
	})
	t.Run("global bounds pinned after removal", func(t *testing.T) {
		s := New()
		s.Admit("a", "orders")
		_, err := s.Ingest(request("a", httpMetric()), now)
		require.NoError(t, err)
		s.Remove("a")
		s.Admit("b", "billing")
		m := httpMetric()
		m.GetHistogram().DataPoints[0].ExplicitBounds[0] = 0.2
		_, err = s.Ingest(request("b", m), now)
		require.Error(t, err)
		assert.Empty(t, s.Snapshot(now, time.Second))
	})
	t.Run("same epoch regression", func(t *testing.T) {
		s := New()
		s.Admit("a", "orders")
		_, err := s.Ingest(request("a", httpMetric()), now)
		require.NoError(t, err)
		m := httpMetric()
		p := m.GetHistogram().DataPoints[0]
		p.TimeUnixNano += uint64(time.Second)
		p.Count = 3
		p.BucketCounts = []uint64{0, 2, 1}
		_, err = s.Ingest(request("a", m), now.Add(time.Second))
		require.Error(t, err)
	})
}
func TestHistogramRangeRegressionCannotHideBehindEarlierGrowth(t *testing.T) {
	s := New()
	s.Admit("a", "orders")
	_, err := s.Ingest(request("a", httpMetric()), now)
	require.NoError(t, err)
	m := httpMetric()
	p := m.GetHistogram().DataPoints[0]
	p.TimeUnixNano += uint64(time.Second)
	p.Count = 5
	p.BucketCounts = []uint64{3, 1, 1} // middle range regresses although every prefix grows
	_, err = s.Ingest(request("a", m), now.Add(time.Second))
	require.Error(t, err)
}

func TestMemoryTypesMissingAndNonfinite(t *testing.T) {
	for _, kind := range []string{"monotonic", "delta", "missing", "nan", "flags", "duplicate attributes"} {
		t.Run(kind, func(t *testing.T) {
			s := New()
			s.Admit("a", "orders")
			m := memory()
			p := m.GetSum().DataPoints[0]
			switch kind {
			case "monotonic":
				m.GetSum().IsMonotonic = true
			case "delta":
				m.GetSum().AggregationTemporality = metricsv1.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
			case "missing":
				p.Value = nil
			case "nan":
				p.Value = &metricsv1.NumberDataPoint_AsDouble{AsDouble: math.NaN()}
			case "flags":
				p.Flags = 1
			case "duplicate attributes":
				p.Attributes = append(p.Attributes, p.Attributes[0])
			}
			_, err := s.Ingest(request("a", m), now)
			require.Error(t, err)
			assert.Empty(t, s.Snapshot(now, time.Second))
		})
	}
}
func TestPoolsAndIndependentStaleness(t *testing.T) {
	s := New()
	s.Admit("a", "orders")
	m := memory()
	_, err := s.Ingest(request("a", m), now)
	require.NoError(t, err)
	var ms []*metricsv1.Metric
	for _, name := range []string{"connections", "pending_requests", "limit"} {
		unit := "{connection}"
		labels := attrs("pool.name", "workers", "pool.id", "1")
		if name == "pending_requests" {
			unit = "{request}"
		}
		if name == "connections" {
			labels = append(labels, attrs("state", "idle")...)
		}
		ms = append(ms, &metricsv1.Metric{Name: "netdata.spike.hikari." + name, Unit: unit, Data: &metricsv1.Metric_Gauge{Gauge: &metricsv1.Gauge{DataPoints: []*metricsv1.NumberDataPoint{{Attributes: labels, TimeUnixNano: uint64(now.Add(4 * time.Second).UnixNano()), Value: &metricsv1.NumberDataPoint_AsInt{AsInt: 0}}}}}})
	}
	r, err := s.Ingest(request("a", ms...), now.Add(4*time.Second))
	require.NoError(t, err)
	assert.Equal(t, 3, r.Accepted)
	got := s.Snapshot(now.Add(6*time.Second), 5*time.Second)
	require.Len(t, got, 1)
	require.Len(t, got[0].Samples, 3)
	for _, p := range got[0].Samples {
		assert.Equal(t, float64(0), p.Value)
		assert.Equal(t, "workers", p.Labels["pool_name"])
	}
}
func TestConcurrentLifecycle(t *testing.T) {
	s := New()
	var wg sync.WaitGroup
	for range 8 {
		wg.Go(func() {
			for range 30 {
				s.Admit("a", "orders")
				_, _ = s.Ingest(request("a", memory(), httpMetric()), now)
				s.Snapshot(now, time.Second)
				s.Remove("a")
			}
		})
	}
	wg.Wait()
}

// The fixture preserves selected raw metric shapes from the pinned Java agent
// recording; resource identities and command arguments are synthetic.
func TestPinnedJavaFixture(t *testing.T) {
	data, err := os.ReadFile("testdata/java-2.32.0.json")
	require.NoError(t, err)
	var req collectorv1.ExportMetricsServiceRequest
	require.NoError(t, protojson.Unmarshal(data, &req))
	s := New()
	s.Admit("fixture-instance", "display override")
	observed := time.Unix(1791540100, 0)
	result, err := s.Ingest(&req, observed)
	require.NoError(t, err)
	assert.Equal(t, 14, result.Accepted)
	got := s.Snapshot(observed, time.Hour)
	require.Len(t, got, 1)
	assert.Equal(t, "display override", got[0].Application)
	assert.Equal(t, "21.0.12+1", got[0].Runtime)
	families := make(map[string]bool)
	for _, sample := range got[0].Samples {
		families[sample.Name] = true
	}
	assert.Len(t, families, 5)
}

func TestSnapshotRetiresChurnAndRejectsExpiredReplay(t *testing.T) {
	s := New()
	s.Admit("a", "orders")
	for i := 0; i < 40; i++ {
		observed := now.Add(time.Duration(i) * 2 * time.Second)
		m := memory()
		p := m.GetSum().DataPoints[0]
		p.TimeUnixNano = uint64(observed.UnixNano())
		p.Attributes = attrs("jvm.memory.type", "heap", "jvm.memory.pool.name", fmt.Sprintf("pool-%d", i))
		_, err := s.Ingest(request("a", m), observed)
		require.NoError(t, err)
		require.Len(t, s.Snapshot(observed, time.Second), 1)
		assert.Empty(t, s.Snapshot(observed.Add(2*time.Second), time.Second))
		require.Empty(t, s.apps["a"].points, "source silence must release retired label state")
		result, err := s.Ingest(request("a", m), observed.Add(3*time.Second))
		require.Error(t, err, "expired exports must not restore retired state")
		assert.Equal(t, Result{Rejected: 1}, result)
	}
	// Wall-clock rollback must not reopen the retired source-time interval.
	assert.Empty(t, s.Snapshot(now, time.Second))
	_, err := s.Ingest(request("a", memory()), now)
	require.Error(t, err)
}

func TestHistogramWatermarkSurvivesSourceSilence(t *testing.T) {
	s := New()
	s.Admit("a", "orders")
	_, err := s.Ingest(request("a", httpMetric()), now)
	require.NoError(t, err)
	first := s.Snapshot(now, time.Second)[0].Samples[0]
	assert.Empty(t, s.Snapshot(now.Add(3*time.Second), time.Second))
	require.Len(t, s.apps["a"].points, 1, "keep cumulative validation state while the process remains admitted")
	resumed := httpMetric()
	p := resumed.GetHistogram().DataPoints[0]
	p.TimeUnixNano = uint64(now.Add(4 * time.Second).UnixNano())
	p.Count = 1
	p.BucketCounts = []uint64{0, 1, 0}
	sum := 0.2
	p.Sum = &sum
	_, err = s.Ingest(request("a", resumed), now.Add(4*time.Second))
	require.Error(t, err, "source silence must not allow a same-epoch cumulative regression")
	resumed = httpMetric()
	p = resumed.GetHistogram().DataPoints[0]
	p.TimeUnixNano = uint64(now.Add(4 * time.Second).UnixNano())
	_, err = s.Ingest(request("a", resumed), now.Add(4*time.Second))
	require.NoError(t, err)
	after := s.Snapshot(now.Add(4*time.Second), time.Second)[0].Samples[0]
	assert.Equal(t, first.Labels["source_epoch"], after.Labels["source_epoch"])
	assert.Equal(t, first.Histogram.Count, after.Histogram.Count)
	_, err = s.Ingest(request("a", resumed), now.Add(5*time.Second))
	require.Error(t, err, "duplicate exports remain rejected after recovery")
	s.Remove("a")
	require.Empty(t, s.apps, "process retirement releases its cumulative watermarks")
}
