// SPDX-License-Identifier: GPL-3.0-or-later

package syntheticjob_test

import (
	"context"
	"maps"
	"slices"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjobtest"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

// A completed background observation isolates publication-cycle cost. Snapshots
// retain only scalars, so event payload and history size must not affect Collect.
// Timing is a local trend; allocations are the regression signal.
func BenchmarkCollect(b *testing.B) { benchmarkCollect(b, 0) }

func BenchmarkCollectWith500Events(b *testing.B) { benchmarkCollect(b, 500) }

func benchmarkCollect(b *testing.B, events int) {
	for _, module := range slices.Sorted(maps.Keys(kinds)) {
		b.Run(module, func(b *testing.B) { benchmarkKindCollect(b, module, kinds[module], events) })
	}
}

func benchmarkKindCollect(b *testing.B, module string, k kind, events int) {
	run := fixture()
	run.Outcome = synthetic.Success
	run.Events = make([]synthetic.Event, events)
	for i := range run.Events {
		run.Events[i] = synthetic.Event{
			Kind:    "stdout",
			Message: "fixed synthetic event",
		}
	}
	e := syntheticjobtest.Executor{
		ExecuteFunc: func(context.Context, synthetic.Request, func(synthetic.JobState)) synthetic.Execution {
			return synthetic.Execution{
				Drained: true,
				Run:     run,
			}
		},
	}
	hub := registry.New()
	c := k.new(e, hub, settings{
		name: "benchmark",
	})
	c.SetConfiguredVnode(vnodes.VirtualNode{
		GUID:     siteGUID,
		Hostname: "site",
	})
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
	deadline := time.Now().Add(3 * time.Second)
	for {
		jobs := hub.Snapshot(time.Now())
		if len(jobs) == 1 && jobs[0].HasObservation {
			break
		}
		if time.Now().After(deadline) {
			b.Fatalf("%s background observation did not complete", module)
		}
		time.Sleep(time.Millisecond)
	}
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
