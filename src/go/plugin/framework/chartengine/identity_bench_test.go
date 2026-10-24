// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"fmt"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

func BenchmarkResolveSeriesRoutesOptionalIdentityCold(b *testing.B) {
	shapes := []struct {
		identityKeys int
		sourceLabels int
	}{
		{identityKeys: 1, sourceLabels: 8},
		{identityKeys: 1, sourceLabels: 128},
		{identityKeys: 8, sourceLabels: 8},
		{identityKeys: 8, sourceLabels: 128},
		{identityKeys: 32, sourceLabels: 32},
		{identityKeys: 32, sourceLabels: 128},
	}

	for _, shape := range shapes {
		for _, present := range []bool{false, true} {
			name := fmt.Sprintf("keys_%d/labels_%d/present_%t", shape.identityKeys, shape.sourceLabels, present)
			b.Run(name, func(b *testing.B) {
				benchmarkResolveSeriesRoutesOptionalIdentityCold(b, shape.identityKeys, shape.sourceLabels, present)
			})
		}
	}
}

func benchmarkResolveSeriesRoutesOptionalIdentityCold(b *testing.B, identityKeyCount, sourceLabelCount int, present bool) {
	b.Helper()

	var template strings.Builder
	template.WriteString(`version: v1
groups:
  - family: Bench
    metrics: [bench_metric]
    charts:
      - id: bench
        title: Bench metric
        context: bench.metric
        units: units
        instances:
          optional_by_labels:
`)
	for i := range identityKeyCount {
		fmt.Fprintf(&template, "            - identity_%03d\n", i)
	}
	template.WriteString(`        dimensions:
          - selector: bench_metric
            name: value
`)

	engine, err := New(WithRuntimeStore(nil))
	if err != nil {
		b.Fatalf("new engine: %v", err)
	}
	if err := engine.LoadYAML([]byte(template.String()), 1); err != nil {
		b.Fatalf("load template: %v", err)
	}

	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("benchmark store is not cycle-managed")
	}
	cc := managed.CycleController()
	gauge := store.Write().SnapshotMeter("").Gauge("bench_metric")
	cc.BeginCycle()
	gauge.Observe(1)
	if err := cc.CommitCycleSuccess(); err != nil {
		b.Fatalf("commit benchmark metric: %v", err)
	}
	reader := store.Read(metrix.ReadFlatten())

	labels := make([]metrix.Label, 0, sourceLabelCount)
	if present {
		for i := range identityKeyCount {
			labels = append(labels, metrix.Label{Key: fmt.Sprintf("identity_%03d", i), Value: fmt.Sprintf("value_%03d", i)})
		}
	}
	for i := len(labels); i < sourceLabelCount; i++ {
		labels = append(labels, metrix.Label{Key: fmt.Sprintf("unrelated_%03d", i), Value: fmt.Sprintf("value_%03d", i)})
	}
	view := labelSliceView{items: labels}
	meta := metrix.SeriesMeta{Kind: metrix.MetricKindGauge}
	index := engine.state.matchIndex

	const identityBatchSize = 1024
	identities := make([]metrix.SeriesIdentity, identityBatchSize)
	for i := range identities {
		identities[i] = metrix.SeriesIdentity{
			ID:     metrix.SeriesID(fmt.Sprintf("bench-series-%04d", i)),
			Hash64: uint64(i) + 1,
		}
	}
	cache := newRouteCache()

	b.ReportAllocs()
	b.ReportMetric(float64(identityKeyCount), "identity_keys/op")
	b.ReportMetric(float64(sourceLabelCount), "source_labels/op")
	b.ResetTimer()
	for i := range b.N {
		if i > 0 && i%identityBatchSize == 0 {
			b.StopTimer()
			cache = newRouteCache()
			b.StartTimer()
		}
		buildSeq := uint64(i/identityBatchSize) + 1
		routes, cached, err := engine.resolveSeriesRoutes(
			cache,
			true,
			nil,
			identities[i%identityBatchSize],
			"bench_metric",
			view,
			meta,
			reader,
			index,
			1,
			buildSeq,
		)
		if err != nil {
			b.Fatalf("resolve cold route: %v", err)
		}
		if cached {
			b.Fatal("route unexpectedly resolved from cache")
		}
		if len(routes) != 1 {
			b.Fatalf("routes = %d, want 1", len(routes))
		}
	}
}
