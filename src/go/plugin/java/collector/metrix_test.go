// SPDX-License-Identifier: GPL-3.0-or-later

package collector

import (
	"fmt"
	"os"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/java/ingest"
	"github.com/stretchr/testify/require"
	collectorv1 "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	metricsv1 "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"
)

func BenchmarkJavaInstrumentWrite(b *testing.B) {
	for _, churn := range []bool{false, true} {
		b.Run(fmt.Sprintf("churn=%t", churn), func(b *testing.B) {
			store := metrix.NewCollectorStore()
			managed, _ := metrix.AsCycleManagedStore(store)
			cycle := managed.CycleController()
			instruments := newInstruments(store)
			app := ingest.Application{Application: "orders", Instance: "instance-0", Samples: []ingest.Sample{
				{Name: "jvm_memory_used_bytes", Value: 1024, Labels: map[string]string{"memory_type": "heap", "memory_pool": "Old Gen"}},
				{Name: "hikari_connections", Value: 2, Labels: map[string]string{"pool_name": "main", "pool_id": "1", "state": "idle"}},
				{Name: "hikari_pending_requests", Labels: map[string]string{"pool_name": "main", "pool_id": "1"}},
				{Name: "hikari_limit", Value: 10, Labels: map[string]string{"pool_name": "main", "pool_id": "1"}},
			}}
			b.ReportAllocs()
			for i := 0; i < b.N; i++ {
				if churn {
					app.Instance = fmt.Sprint("instance-", i)
				}
				cycle.BeginCycle()
				instruments.write(app, "Orders")
				_ = cycle.CommitCycleSuccess()
			}
		})
	}
}

func TestDynamicInstrumentLabelsRemainIsolated(t *testing.T) {
	store := metrix.NewCollectorStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	cycle := managed.CycleController()
	instruments := newInstruments(store)
	for i := 0; i < 30; i++ {
		instance := fmt.Sprint("instance-", i)
		pool := fmt.Sprint("pool-", i)
		app := ingest.Application{Application: "orders", Instance: instance, Samples: []ingest.Sample{
			{Name: "hikari_pending_requests", Value: float64(i), Labels: map[string]string{"pool_name": "main", "pool_id": pool}},
		}}
		cycle.BeginCycle()
		instruments.write(app, "Orders")
		require.NoError(t, cycle.CommitCycleSuccess())
		labels := metrix.Labels{"application": "orders", "instance": instance, "application_name": "Orders", "pool_name": "main", "pool_id": pool}
		value, exists := store.Read().Value("hikari_pending_requests", labels)
		require.True(t, exists)
		require.Equal(t, float64(i), value)
	}
	for range 12 {
		cycle.BeginCycle()
		require.NoError(t, cycle.CommitCycleSuccess())
	}
	count := 0
	store.Read(metrix.ReadRaw()).ForEachSeries(func(_ string, _ metrix.LabelView, _ metrix.SampleValue) { count++ })
	require.Zero(t, count, "retired dynamic series should leave the metric store")
}

func TestHistogramIdentitySurvivesReceiverRestartAndSourceSilence(t *testing.T) {
	data, err := os.ReadFile("../ingest/testdata/java-2.32.0.json")
	require.NoError(t, err)
	var req collectorv1.ExportMetricsServiceRequest
	require.NoError(t, protojson.Unmarshal(data, &req))
	observed := time.Unix(1791540100, 0)
	for _, r := range req.ResourceMetrics {
		for _, scope := range r.ScopeMetrics {
			var kept []*metricsv1.Metric
			for _, metric := range scope.Metrics {
				if metric.GetHistogram() != nil {
					for _, p := range metric.GetHistogram().DataPoints {
						p.TimeUnixNano = uint64(observed.UnixNano())
					}
					kept = append(kept, metric)
				}
			}
			scope.Metrics = kept
		}
	}
	ingress := ingest.New()
	ingress.Admit("fixture-instance", "orders")
	store := metrix.NewCollectorStore()
	instruments := newInstruments(store)
	managed, ok := metrix.AsCycleManagedStore(store)
	require.True(t, ok)
	cycle := managed.CycleController()
	engine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, engine.LoadYAML([]byte(charts), 1))
	collect := func(at time.Time) []string {
		cycle.BeginCycle()
		for _, app := range ingress.Snapshot(at, time.Second) {
			instruments.write(app, "Orders")
		}
		require.NoError(t, cycle.CommitCycleSuccess())
		plan, err := engine.PreparePlan(store.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
		require.NoError(t, err)
		var created []string
		for _, action := range plan.Plan().Actions {
			if action, ok := action.(chartengine.CreateChartAction); ok {
				created = append(created, action.ChartID)
			}
		}
		require.NoError(t, plan.Commit())
		return created
	}
	result, err := ingress.Ingest(&req, observed)
	require.NoError(t, err)
	require.Positive(t, result.Accepted)
	first := collect(observed)
	require.NotEmpty(t, first)
	// A replacement job sees later observations of the same producer. Even if
	// it collects an empty snapshot first, its charts retain their identities.
	restartedIngress := ingest.New()
	restartedIngress.Admit("fixture-instance", "orders")
	require.Empty(t, restartedIngress.Snapshot(observed.Add(time.Second), time.Second))
	for _, r := range req.ResourceMetrics {
		for _, scope := range r.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, p := range metric.GetHistogram().GetDataPoints() {
					p.TimeUnixNano = uint64(observed.Add(time.Second).UnixNano())
				}
			}
		}
	}
	_, err = restartedIngress.Ingest(&req, observed.Add(time.Second))
	require.NoError(t, err)
	restartedStore := metrix.NewCollectorStore()
	restartedInstruments := newInstruments(restartedStore)
	restartedManaged, ok := metrix.AsCycleManagedStore(restartedStore)
	require.True(t, ok)
	restartedManaged.CycleController().BeginCycle()
	for _, app := range restartedIngress.Snapshot(observed.Add(time.Second), time.Second) {
		restartedInstruments.write(app, "Renamed Orders")
	}
	require.NoError(t, restartedManaged.CycleController().CommitCycleSuccess())
	restartedEngine, err := chartengine.New()
	require.NoError(t, err)
	require.NoError(t, restartedEngine.LoadYAML([]byte(charts), 1))
	restartedPlan, err := restartedEngine.PreparePlan(restartedStore.Read(metrix.ReadRaw(), metrix.ReadFlatten()))
	require.NoError(t, err)
	var restartedIDs []string
	for _, action := range restartedPlan.Plan().Actions {
		if action, ok := action.(chartengine.CreateChartAction); ok {
			restartedIDs = append(restartedIDs, action.ChartID)
		}
	}
	require.ElementsMatch(t, first, restartedIDs, "job restart and display rename preserve an uninterrupted producer identity")
	require.NoError(t, restartedPlan.Commit())

	require.Empty(t, collect(observed.Add(2*time.Second)))
	// The same cumulative series resumes after a gap. Keeping its watermark
	// allows an advancing observation without changing its publication identity.
	for _, r := range req.ResourceMetrics {
		for _, scope := range r.ScopeMetrics {
			for _, metric := range scope.Metrics {
				for _, p := range metric.GetHistogram().GetDataPoints() {
					p.TimeUnixNano = uint64(observed.Add(3 * time.Second).UnixNano())
				}
			}
		}
	}
	_, err = ingress.Ingest(&req, observed.Add(3*time.Second))
	require.NoError(t, err)
	require.Empty(t, collect(observed.Add(3*time.Second)), "resuming a valid cumulative stream does not create new chart identities")
}
