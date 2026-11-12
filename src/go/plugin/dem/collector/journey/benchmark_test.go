// SPDX-License-Identifier: GPL-3.0-or-later
package journey_test

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/journey"
	model "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
	syntheticregistry "github.com/netdata/netdata/go/plugins/plugin/dem/synthetic/registry"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
)

type benchmarkExecutor struct{ result model.Execution }

func (e benchmarkExecutor) Check(context.Context, model.Kind) error { return nil }
func (e benchmarkExecutor) Rehearse(context.Context, model.Request) error {
	panic("unexpected rehearsal")
}

func (e benchmarkExecutor) ExecuteScheduled(
	_ context.Context,
	r model.Request,
	state func(string),
	admitted func(time.Time) model.Schedule,
) model.Execution {
	at := time.Now()
	schedule := admitted(at)
	state("running")
	result := e.result
	result.Run.ID = "benchmark-run"
	result.Run.JobID = r.JobID()
	result.Run.StartedUS = at.UnixMicro()
	result.Run.CompletedUS = time.Now().UnixMicro()
	result.Run.ScheduledUS = schedule.DueUS
	result.Run.QueuedUS = schedule.QueuedUS
	return result
}

// A completed background observation isolates publication-cycle cost. Snapshots
// retain only scalars, so event payload and history size must not affect Collect.
// Timing is a local trend; allocations are the regression signal.
func BenchmarkCollect(b *testing.B) { benchmarkCollect(b, 0) }

func BenchmarkCollectWith500Events(b *testing.B) { benchmarkCollect(b, 500) }

func benchmarkCollect(b *testing.B, events int) {
	duration := 1250.5

	e := benchmarkExecutor{
		result: model.Execution{
			Drained: true,
			Run: model.Run{
				VnodeGUID:  "11111111-1111-4111-8111-111111111111",
				Kind:       model.Journey,
				Outcome:    model.Success,
				DurationMS: &duration,
				Tests: &model.TestCounts{
					Declared: 2,
					Passed:   2,
				},
			},
		},
	}
	e.result.Run.Events = make([]model.Event, events)
	for i := range e.result.Run.Events {
		e.result.Run.Events[i] = model.Event{
			Kind:    "stdout",
			Message: "fixed synthetic event",
		}
	}
	hub := syntheticregistry.New()
	c := journey.New(journey.Dependencies{
		Executor: e,
		Registry: hub,
	})
	c.SetConfiguredVnode(vnodes.VirtualNode{GUID: "11111111-1111-4111-8111-111111111111", Hostname: "site"})
	c.Name = "benchmark"
	c.Script = "export {};"
	c.Secrets = []journey.Secret{{Name: "DEM_SECRET_TOKEN", Value: "fixture-value"}}
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
		if len(jobs) == 1 && jobs[0].Latest != nil {
			break
		}
		if time.Now().After(deadline) {
			b.Fatal("background observation did not complete")
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
