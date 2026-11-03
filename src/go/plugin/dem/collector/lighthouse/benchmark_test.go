// SPDX-License-Identifier: GPL-3.0-or-later
package lighthouse_test

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/lighthouse"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
)

type benchmarkExecutor struct{ result model.Execution }

func (e benchmarkExecutor) Check(context.Context, model.Kind) error { return nil }
func (e benchmarkExecutor) Execute(_ context.Context, _ model.Request, state func(string)) model.Execution {
	state("running")
	return e.result
}

// One terminal observation and a fixed metric surface isolate collector-cycle
// overhead. Cost is O(result events + artifacts), independent of other jobs or
// history. Timing is a local trend; allocations are the regression signal.
func BenchmarkCollect(b *testing.B) {
	duration := 1250.5
	zero := 0.0
	e := benchmarkExecutor{
		result: model.Execution{
			Drained: true,
			Run: model.Run{
				Kind:       model.Lighthouse,
				Outcome:    model.Success,
				DurationMS: &duration,
				Metrics: &model.LabMetrics{
					Performance: &zero,
					FCPMS:       &duration,
					LCPMS:       &duration,
					TBTMS:       &zero,
					SIMS:        &duration,
					CLS:         &zero,
				},
			},
		},
	}
	c := lighthouse.New(lighthouse.Dependencies{
		Executor: e,
		Registry: syntheticregistry.New(),
	})
	c.Name = "benchmark"
	c.URL = "https://example.org/"
	if err := c.Init(context.Background()); err != nil {
		b.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	ready := make(chan struct{})
	done := make(chan error, 1)
	go func() { done <- c.Run(ctx, func() { close(ready) }) }()
	select {
	case <-ready:
	case err := <-done:
		cancel()
		b.Fatal(err)
	}
	defer func() {
		cancel()
		if err := <-done; err != nil {
			b.Error(err)
		}
	}()
	managed, ok := metrix.AsCycleManagedStore(c.MetricStore())
	if !ok {
		b.Fatal("store is not cycle-managed")
	}
	controller := managed.CycleController()
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		controller.BeginCycle()
		if err := c.Collect(context.Background()); err != nil {
			b.Fatal(err)
		}
		if err := controller.CommitCycleSuccess(); err != nil {
			b.Fatal(err)
		}
	}
	b.StopTimer()
}
