// SPDX-License-Identifier: GPL-3.0-or-later

// Package syntheticjobtest provides the browser executor fake for synthetic collector tests.
package syntheticjobtest

import (
	"context"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/dem/collector/internal/syntheticjob"
	"github.com/netdata/netdata/go/plugins/plugin/dem/synthetic"
)

// Executor fakes the process browser capability. Nil hooks accept preparation, keep scheduled attempts queued until
// cancellation and panic on rehearsal.
type Executor struct {
	CheckFunc func(context.Context) error
	// ExecuteFunc runs an admitted attempt; ExecuteScheduled completes the run identity and timing it leaves empty.
	ExecuteFunc  func(context.Context, synthetic.Request, func(synthetic.JobState)) synthetic.Execution
	RehearseFunc func(context.Context, synthetic.Request) error
}

var _ syntheticjob.Executor = Executor{}

func (e Executor) Check(ctx context.Context) error {
	if e.CheckFunc != nil {
		return e.CheckFunc(ctx)
	}
	return nil
}

func (e Executor) Rehearse(ctx context.Context, request synthetic.Request) error {
	if e.RehearseFunc == nil {
		panic("unexpected rehearsal")
	}
	return e.RehearseFunc(ctx, request)
}

func (e Executor) ExecuteScheduled(
	ctx context.Context,
	r synthetic.Request,
	state func(synthetic.JobState),
	admitted func(time.Time) synthetic.Schedule,
) synthetic.Execution {
	state(synthetic.JobWaiting)
	// Without ExecuteFunc the attempt is genuinely queued work, not an admitted run.
	if e.ExecuteFunc == nil {
		<-ctx.Done()
		return synthetic.Execution{
			Drained: true,
			Run: synthetic.Run{
				Outcome:     synthetic.Cancelled,
				CompletedUS: time.Now().UnixMicro(),
			},
		}
	}
	at := time.Now()
	schedule := admitted(at)
	result := e.ExecuteFunc(ctx, r, state)
	run := &result.Run
	if run.ID == "" {
		run.ID = fmt.Sprint(at.UnixNano())
	}
	if run.JobID == "" {
		run.JobID = r.JobID()
	}
	if run.Name == "" {
		run.Name = r.Name
	}
	if run.Kind == "" {
		run.Kind = r.Kind
	}
	run.VnodeGUID = r.VnodeGUID
	if run.StartedUS == 0 {
		run.StartedUS = at.UnixMicro()
	}
	if result.Drained && run.CompletedUS < run.StartedUS {
		run.CompletedUS = time.Now().UnixMicro()
	}
	run.ScheduledUS = schedule.DueUS
	run.QueuedUS = schedule.QueuedUS
	return result
}
