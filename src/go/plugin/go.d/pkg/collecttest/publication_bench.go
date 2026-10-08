// SPDX-License-Identifier: GPL-3.0-or-later

package collecttest

import (
	"bytes"
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/netdataapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartemit"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

// BenchmarkPublication measures the framework publication stages the V2 job
// runtime runs after a successful Collect: template capture, metric commit,
// per-host-scope raw+flattened reads, planning, emission, plan commit and the
// job runtime-metrics flush. Engine options mirror JobV2: no engine-owned
// runtime store and one runtime-sample aggregator flushed per cycle.
//
// collect runs inside each cycle with the benchmark timer stopped, so the
// result covers publication only. Warmup cycles require every warm cycle to
// emit the same non-zero number of chart updates; it is reported as
// charts/cycle.
func BenchmarkPublication(
	b *testing.B,
	collector interface {
		MetricStore() metrix.CollectorStore
	},
	collect func(context.Context) error,
) {
	b.Helper()
	p := newPublicationBench(b, collector)
	charts := p.warmup(b, collect)
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		p.cycle(b, collect, false)
	}
	b.ReportMetric(float64(charts), "charts/cycle")
}

type publicationBench struct {
	store      metrix.CollectorStore
	controller metrix.CycleController
	source     *collectorapi.ChartTemplateSource
	engines    map[string]*chartengine.Engine
	aggregator *chartengine.RuntimeAggregator
	buf        bytes.Buffer
	api        *netdataapi.API
}

func newPublicationBench(b *testing.B, collector interface {
	MetricStore() metrix.CollectorStore
}) *publicationBench {
	b.Helper()
	store := collector.MetricStore()
	managed, ok := metrix.AsCycleManagedStore(store)
	if !ok {
		b.Fatal("collecttest: metric store is not cycle-managed")
	}
	source, err := collectorapi.NewChartTemplateSource(collector)
	if err != nil {
		b.Fatalf("collecttest: chart provider: %v", err)
	}
	p := &publicationBench{
		store:      store,
		controller: managed.CycleController(),
		source:     source,
		engines:    make(map[string]*chartengine.Engine),
		aggregator: chartengine.NewRuntimeAggregator(metrix.NewRuntimeStore()),
	}
	p.api = netdataapi.New(&p.buf)
	return p
}

func (p *publicationBench) warmup(b *testing.B, collect func(context.Context) error) int {
	want := 0
	for i := range 4 {
		updates := p.cycle(b, collect, true)
		switch {
		case i == 0:
		case updates == 0:
			b.Fatal("collecttest: warm publication emitted no chart updates")
		case want != 0 && updates != want:
			b.Fatalf("collecttest: unstable warm publication: %d chart updates, previous %d", updates, want)
		}
		if i > 0 {
			want = updates
		}
	}
	return want
}

// cycle runs one collection cycle and publishes it; with count set it returns
// the number of emitted chart updates.
func (p *publicationBench) cycle(b *testing.B, collect func(context.Context) error, count bool) int {
	b.StopTimer()
	p.controller.BeginCycle()
	if err := collect(context.Background()); err != nil {
		p.controller.AbortCycle()
		b.Fatalf("collecttest: collect: %v", err)
	}
	b.StartTimer()

	set, err := p.source.Capture()
	if err != nil {
		b.Fatalf("collecttest: chart template: %v", err)
	}
	if err := p.controller.CommitCycleSuccess(); err != nil {
		b.Fatalf("collecttest: commit: %v", err)
	}
	updates := 0
	reader := p.store.Read(metrix.ReadFlatten()).(metrix.FreshVisibleHostScopesReader)
	for _, scope := range reader.FreshVisibleHostScopes() {
		engine := p.engines[scope.ScopeKey]
		if engine == nil {
			engine, err = chartengine.New(
				chartengine.WithRuntimeStore(nil),
				chartengine.WithRuntimeSampleObserver(p.aggregator.Observe),
			)
			if err != nil {
				b.Fatalf("collecttest: chart engine: %v", err)
			}
			p.engines[scope.ScopeKey] = engine
		}
		attempt, err := engine.PreparePlanWithOptions(
			p.store.Read(metrix.ReadRaw(), metrix.ReadFlatten(), metrix.ReadHostScope(scope.ScopeKey)),
			chartengine.PlanOptions{
				TemplateSet: set,
			},
		)
		if err != nil {
			b.Fatalf("collecttest: plan host scope %q: %v", scope.ScopeKey, err)
		}
		env := chartemit.EmitEnv{
			TypeID:      "bench.job",
			UpdateEvery: 1,
		}
		if !scope.IsDefault() {
			env.HostScope = &chartemit.HostScope{
				GUID: scope.GUID,
			}
		}
		p.buf.Reset()
		if err := chartemit.ApplyPlan(p.api, attempt.Plan(), env); err != nil {
			b.Fatalf("collecttest: emit host scope %q: %v", scope.ScopeKey, err)
		}
		if count {
			updates += bytes.Count(p.buf.Bytes(), []byte("BEGIN "))
		}
		if err := attempt.Commit(); err != nil {
			b.Fatalf("collecttest: commit plan host scope %q: %v", scope.ScopeKey, err)
		}
	}
	p.aggregator.Flush()
	return updates
}
