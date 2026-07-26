// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"maps"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestFlattenSnapshotPreservesCanonicalSeriesIdentity(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	meter := store.Write().SnapshotMeter("svc")
	labelNames := []string{"a", "m", "z"}
	labelValues := []string{"first", "middle", "last"}

	histogram, err := meter.Vec(labelNames...).Histogram("latency", WithHistogramBounds(1, 2)).GetWithLabelValues(labelValues...)
	require.NoError(t, err)
	summary, err := meter.Vec(labelNames...).Summary("duration", WithSummaryQuantiles(0.5, 0.9)).GetWithLabelValues(labelValues...)
	require.NoError(t, err)
	stateSet, err := meter.Vec(labelNames...).StateSet(
		"mode",
		WithStateSetStates("maintenance", "operational"),
		WithStateSetMode(ModeEnum),
	).GetWithLabelValues(labelValues...)
	require.NoError(t, err)
	measureSet, err := meter.Vec(labelNames...).MeasureSetGauge(
		"payload",
		WithMeasureSetFields(
			MeasureFieldSpec{Name: "bytes"},
			MeasureFieldSpec{Name: "rows"},
		),
	).GetWithLabelValues(labelValues...)
	require.NoError(t, err)

	cycle.BeginCycle()
	observeStructuredFlattenTestPoints(histogram, summary, stateSet)
	measureSet.ObservePoint(MeasureSetPoint{Values: []SampleValue{128, 4}})
	require.NoError(t, cycle.CommitCycleSuccess())

	snapshot := store.(*storeView).core.snapshot.Load()
	sourceLabels := make(map[string][]Label, len(snapshot.series))
	for key, series := range snapshot.series {
		sourceLabels[key] = append([]Label(nil), series.labels...)
	}

	flat := flattenSnapshot(snapshot)
	baseLabels := map[string]string{"a": "first", "m": "middle", "z": "last"}

	requireFlattenedSeriesIdentity(t, flat, "svc.latency_count", baseLabels)
	requireFlattenedSeriesIdentity(t, flat, "svc.latency_sum", baseLabels)
	requireFlattenedSeriesIdentity(t, flat, "svc.latency_bucket", labelsWith(baseLabels, HistogramBucketLabel, "1"))
	requireFlattenedSeriesIdentity(t, flat, "svc.latency_bucket", labelsWith(baseLabels, HistogramBucketLabel, "+Inf"))
	requireFlattenedSeriesIdentity(t, flat, "svc.duration_count", baseLabels)
	requireFlattenedSeriesIdentity(t, flat, "svc.duration_sum", baseLabels)
	requireFlattenedSeriesIdentity(t, flat, "svc.duration", labelsWith(baseLabels, SummaryQuantileLabel, "0.5"))
	requireFlattenedSeriesIdentity(t, flat, "svc.mode", labelsWith(baseLabels, "svc.mode", "operational"))
	requireFlattenedSeriesIdentity(t, flat, "svc.payload_bytes", labelsWith(baseLabels, MeasureSetFieldLabel, "bytes"))

	histogramSource := requireSnapshotSeriesByName(t, snapshot, "svc.latency")
	require.Same(t, &histogramSource.labels[0], &requireFlattenedSeries(t, flat, "svc.latency_count", baseLabels).labels[0])
	require.Same(t, &histogramSource.labels[0], &requireFlattenedSeries(t, flat, "svc.latency_sum", baseLabels).labels[0])
	summarySource := requireSnapshotSeriesByName(t, snapshot, "svc.duration")
	require.Same(t, &summarySource.labels[0], &requireFlattenedSeries(t, flat, "svc.duration_count", baseLabels).labels[0])
	require.Same(t, &summarySource.labels[0], &requireFlattenedSeries(t, flat, "svc.duration_sum", baseLabels).labels[0])

	for key, want := range sourceLabels {
		require.Equal(t, want, snapshot.series[key].labels, "flattening mutated source labels for %q", key)
	}
}

func TestFlattenSnapshotReusesRoleDescriptorsWithinSourceSeries(t *testing.T) {
	store := NewCollectorStore()
	cycle := cycleController(t, store)
	meter := store.Write().SnapshotMeter("svc")

	histogram := meter.Histogram("latency", WithHistogramBounds(1, 2))
	summary := meter.Summary("duration", WithSummaryQuantiles(0.5, 0.9))
	stateSet := meter.StateSet(
		"mode",
		WithStateSetStates("maintenance", "operational"),
		WithStateSetMode(ModeEnum),
	)

	cycle.BeginCycle()
	observeStructuredFlattenTestPoints(histogram, summary, stateSet)
	require.NoError(t, cycle.CommitCycleSuccess())

	flat := flattenSnapshot(store.(*storeView).core.snapshot.Load())
	bucketOne := requireFlattenedSeries(t, flat, "svc.latency_bucket", map[string]string{HistogramBucketLabel: "1"})
	bucketTwo := requireFlattenedSeries(t, flat, "svc.latency_bucket", map[string]string{HistogramBucketLabel: "2"})
	bucketInf := requireFlattenedSeries(t, flat, "svc.latency_bucket", map[string]string{HistogramBucketLabel: "+Inf"})
	require.Same(t, bucketOne.desc, bucketTwo.desc)
	require.Same(t, bucketOne.desc, bucketInf.desc)
	require.NotSame(t, bucketOne.desc, requireFlattenedSeries(t, flat, "svc.latency_count", nil).desc)
	require.NotSame(t, bucketOne.desc, requireFlattenedSeries(t, flat, "svc.latency_sum", nil).desc)

	quantileHalf := requireFlattenedSeries(t, flat, "svc.duration", map[string]string{SummaryQuantileLabel: "0.5"})
	quantileNinety := requireFlattenedSeries(t, flat, "svc.duration", map[string]string{SummaryQuantileLabel: "0.9"})
	require.Same(t, quantileHalf.desc, quantileNinety.desc)
	require.NotSame(t, quantileHalf.desc, requireFlattenedSeries(t, flat, "svc.duration_count", nil).desc)
	require.NotSame(t, quantileHalf.desc, requireFlattenedSeries(t, flat, "svc.duration_sum", nil).desc)

	stateMaintenance := requireFlattenedSeries(t, flat, "svc.mode", map[string]string{"svc.mode": "maintenance"})
	stateOperational := requireFlattenedSeries(t, flat, "svc.mode", map[string]string{"svc.mode": "operational"})
	require.Same(t, stateMaintenance.desc, stateOperational.desc)
}

func TestFlattenSnapshotAllocationEnvelope(t *testing.T) {
	const (
		lowCardinality  = 32
		highCardinality = 512
		cardinalityRate = highCardinality / lowCardinality
		growthSlack     = 1.10
	)

	measure := func(t *testing.T, store CollectorStore, wantSeries int) float64 {
		t.Helper()
		snapshot := store.(*storeView).core.snapshot.Load()
		requireFlattenProjectionSeries(t, snapshot, wantSeries)
		return testing.AllocsPerRun(3, func() {
			benchmarkReaderCountSink = len(flattenSnapshot(snapshot).series)
		})
	}

	t.Run("scalar projection allocation count stays bounded", func(t *testing.T) {
		low := measure(t, benchmarkCommittedScalarStore(t, lowCardinality), lowCardinality)
		high := measure(t, benchmarkCommittedScalarStore(t, highCardinality), highCardinality)

		// Scalar projection shares committed series. Map and index growth may add
		// a few allocation events, but must not add per-series object allocations.
		require.LessOrEqualf(t, low, float64(20), "low-cardinality scalar allocations = %.0f", low)
		require.LessOrEqualf(t, high, float64(25), "high-cardinality scalar allocations = %.0f", high)
		require.LessOrEqualf(t, high, low*2, "%dx cardinality grew scalar allocations from %.0f to %.0f", cardinalityRate, low, high)
	})

	tests := map[string]struct {
		store               func(testing.TB, int) CollectorStore
		wantSeriesPerSource int
		maxAllocsPerSource  float64
	}{
		"mixed structured": {
			store: func(tb testing.TB, totalSeries int) CollectorStore {
				return benchmarkCommittedMixedStore(tb, totalSeries)
			},
			wantSeriesPerSource: 15,
			maxAllocsPerSource:  100,
		},
		"histogram with eight labels": {
			store: func(tb testing.TB, totalSeries int) CollectorStore {
				return benchmarkCommittedHistogramStore(tb, totalSeries, 8)
			},
			wantSeriesPerSource: 15,
			maxAllocsPerSource:  140,
		},
		"summary with eight labels and quantiles": {
			store: func(tb testing.TB, totalSeries int) CollectorStore {
				return benchmarkCommittedSummaryStore(tb, totalSeries, 8, 8)
			},
			wantSeriesPerSource: 10,
			maxAllocsPerSource:  105,
		},
		"stateset with eight labels and states": {
			store: func(tb testing.TB, totalSeries int) CollectorStore {
				return benchmarkCommittedStateSetStore(tb, totalSeries, 8, 8)
			},
			wantSeriesPerSource: 8,
			maxAllocsPerSource:  85,
		},
		"measureset gauge with eight labels and fields": {
			store: func(tb testing.TB, totalSeries int) CollectorStore {
				return benchmarkCommittedMeasureSetStore(tb, totalSeries, 8, 8, MeasureSetSemanticsGauge)
			},
			wantSeriesPerSource: 8,
			maxAllocsPerSource:  105,
		},
		"measureset counter with eight labels and fields": {
			store: func(tb testing.TB, totalSeries int) CollectorStore {
				return benchmarkCommittedMeasureSetStore(tb, totalSeries, 8, 8, MeasureSetSemanticsCounter)
			},
			wantSeriesPerSource: 8,
			maxAllocsPerSource:  105,
		},
	}

	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			low := measure(
				t,
				tc.store(t, lowCardinality),
				tc.wantSeriesPerSource*lowCardinality,
			)
			high := measure(
				t,
				tc.store(t, highCardinality),
				tc.wantSeriesPerSource*highCardinality,
			)

			lowLimit := tc.maxAllocsPerSource * lowCardinality
			highLimit := tc.maxAllocsPerSource * highCardinality
			require.LessOrEqualf(t, low, lowLimit, "low-cardinality allocations %.0f exceed limit %.0f", low, lowLimit)
			require.LessOrEqualf(t, high, highLimit, "high-cardinality allocations %.0f exceed limit %.0f", high, highLimit)
			require.LessOrEqualf(
				t,
				high,
				low*cardinalityRate*growthSlack,
				"%dx cardinality grew allocations from %.0f to %.0f",
				cardinalityRate,
				low,
				high,
			)
		})
	}
}

func observeStructuredFlattenTestPoints(histogram SnapshotHistogram, summary SnapshotSummary, stateSet StateSetInstrument) {
	histogram.ObservePoint(HistogramPoint{
		Count: 3,
		Sum:   4,
		Buckets: []BucketPoint{
			{UpperBound: 1, CumulativeCount: 1},
			{UpperBound: 2, CumulativeCount: 2},
		},
	})
	summary.ObservePoint(SummaryPoint{
		Count: 2,
		Sum:   3,
		Quantiles: []QuantilePoint{
			{Quantile: 0.5, Value: 1},
			{Quantile: 0.9, Value: 2},
		},
	})
	stateSet.ObserveStateSet(StateSetPoint{
		States: map[string]bool{"operational": true},
	})
}

func requireFlattenedSeriesIdentity(t *testing.T, snapshot *readSnapshot, name string, labels map[string]string) {
	t.Helper()

	series := requireFlattenedSeries(t, snapshot, name, labels)
	items, labelsKey, err := canonicalizeLabels(labels)
	require.NoError(t, err)
	key := makeSeriesKey("", name, labelsKey)
	require.Equal(t, items, series.labels)
	require.Equal(t, labelsKey, series.labelsKey)
	require.Equal(t, key, series.key)
	require.Equal(t, SeriesID(key), series.id)
	require.Equal(t, seriesIDHash(SeriesID(key)), series.hash64)
}

func requireFlattenedSeries(t *testing.T, snapshot *readSnapshot, name string, labels map[string]string) *committedSeries {
	t.Helper()

	_, labelsKey, err := canonicalizeLabels(labels)
	require.NoError(t, err)
	key := makeSeriesKey("", name, labelsKey)
	series := snapshot.series[key]
	require.NotNil(t, series, "missing flattened series %q", key)
	return series
}

func requireSnapshotSeriesByName(t *testing.T, snapshot *readSnapshot, name string) *committedSeries {
	t.Helper()

	for _, series := range snapshot.series {
		if series.name == name {
			return series
		}
	}
	require.FailNow(t, "missing source series", name)
	return nil
}

func labelsWith(base map[string]string, key, value string) map[string]string {
	labels := make(map[string]string, len(base)+1)
	maps.Copy(labels, base)
	labels[key] = value
	return labels
}
