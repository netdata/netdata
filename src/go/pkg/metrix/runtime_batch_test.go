// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// runtimeBatchWorkload writes counters, gauges, a summary and a histogram, including
// repeated writes of one series, through a fresh runtime store.
func runtimeBatchWorkload(store RuntimeStore) {
	meter := store.Write().StatefulMeter("rt")
	requests := meter.Counter("requests_total")
	depth := meter.Gauge("queue_depth")
	latency := meter.Summary("latency_seconds", WithSummaryQuantiles(0.5, 0.9))
	sizes := meter.Histogram("size_bytes", WithHistogramBounds(1, 10))
	for i := range 3 {
		requests.Add(2)
		depth.Set(SampleValue(10 + i))
		latency.Observe(SampleValue(i) / 10)
		sizes.Observe(SampleValue(i * 4))
	}
}

func runtimeSeriesState(t *testing.T, store RuntimeStore) map[string]any {
	t.Helper()
	reader := store.Read(ReadRaw())
	delta, ok := reader.Delta("rt.requests_total", nil)
	require.True(t, ok)
	summary, ok := reader.Summary("rt.latency_seconds", nil)
	require.True(t, ok)
	sizes, ok := reader.Histogram("rt.size_bytes", nil)
	require.True(t, ok)
	flatReader := store.Read(ReadRaw(), ReadFlatten())
	flat := map[string]SampleValue{}
	flatDeltas := map[string]SampleValue{}
	flatReader.ForEachSeries(func(name string, labels LabelView, v SampleValue) {
		key := name
		if q, ok := labels.Get(SummaryQuantileLabel); ok {
			key += "{" + q + "}"
		}
		if le, ok := labels.Get(HistogramBucketLabel); ok {
			key += "{" + le + "}"
		}
		flat[key] = v
		if d, ok := flatReader.Delta(name, labels.CloneMap()); ok {
			flatDeltas[key] = d
		}
	})
	return map[string]any{
		"requests_delta":   delta,
		"summary":          summary,
		"histogram":        sizes,
		"flattened":        flat,
		"flattened_deltas": flatDeltas,
	}
}

func TestRuntimeStoreWriteBatchPublishesSameStateOnce(t *testing.T) {
	immediate := NewRuntimeStore()
	runtimeBatchWorkload(immediate)

	batched := NewRuntimeStore()
	batcher, ok := batched.(RuntimeBatchWriter)
	require.True(t, ok)
	before := batched.Read(ReadRaw()).CollectMeta()
	batcher.WriteBatch(func() {
		runtimeBatchWorkload(batched)
		_, visible := batched.Read(ReadRaw()).Value("rt.queue_depth", nil)
		assert.False(t, visible, "a batched write became visible before the batch ended")
		assert.Equal(t, before, batched.Read(ReadRaw()).CollectMeta())
	})

	assert.Equal(t, runtimeSeriesState(t, immediate), runtimeSeriesState(t, batched))
	assert.Equal(t, immediate.Read().CollectMeta(), batched.Read().CollectMeta())
}

func TestRuntimeStoreWriteBatchNestsAndSurvivesPanic(t *testing.T) {
	store := NewRuntimeStore()
	batcher := store.(RuntimeBatchWriter)
	gauge := store.Write().StatefulMeter("rt").Gauge("value")

	batcher.WriteBatch(func() {
		gauge.Set(1)
		batcher.WriteBatch(func() { gauge.Set(2) })
		_, visible := store.Read(ReadRaw()).Value("rt.value", nil)
		assert.False(t, visible, "a nested batch published before the outer batch ended")
	})
	value, ok := store.Read(ReadRaw()).Value("rt.value", nil)
	require.True(t, ok)
	assert.Equal(t, SampleValue(2), value)

	assert.Panics(t, func() {
		batcher.WriteBatch(func() {
			gauge.Set(3)
			panic("collector bug")
		})
	})
	value, ok = store.Read(ReadRaw()).Value("rt.value", nil)
	require.True(t, ok)
	assert.Equal(t, SampleValue(3), value, "writes before a panic must still publish")

	// The store keeps accepting immediate writes after a batch.
	gauge.Set(4)
	value, _ = store.Read(ReadRaw()).Value("rt.value", nil)
	assert.Equal(t, SampleValue(4), value)
}
