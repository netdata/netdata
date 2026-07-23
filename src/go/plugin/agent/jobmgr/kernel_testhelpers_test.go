// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func frameTaskWork(work func(context.Context) (lifecycle.SealedResult, error)) lifecycle.TaskWork {
	return func(ctx context.Context) (lifecycle.TaskOutcome, error) {
		result, err := work(ctx)
		if err != nil {
			return lifecycle.TaskOutcome{}, err
		}
		return lifecycle.NewFrameOutcome(result)
	}
}

type shutdownProbe struct {
	cancelled <-chan struct{}
	settled   <-chan error
}

func startShutdownProbe(ctx context.Context, ck *CommandKernel, uid string) (shutdownProbe, error) {
	if ctx == nil || ck == nil || uid == "" {
		return shutdownProbe{}, errors.New("invalid shutdown probe")
	}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	settled := make(chan error, 1)
	go func() {
		settled <- ck.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     uid,
				LaneKey: uid,
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/shutdown-probe",
			},
			WorkPlan{
				Work: frameTaskWork(func(ctx context.Context) (lifecycle.SealedResult, error) {
					close(started)
					<-ctx.Done()
					close(cancelled)
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				}),
			},
		)
	}()
	select {
	case <-started:
		return shutdownProbe{
			cancelled: cancelled,
			settled:   settled,
		}, nil
	case err := <-settled:
		return shutdownProbe{}, errors.Join(errors.New("shutdown probe settled before starting"), err)
	case <-ctx.Done():
		return shutdownProbe{}, ctx.Err()
	}
}

func (probe shutdownProbe) waitCancellation(ctx context.Context) error {
	select {
	case <-probe.cancelled:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (probe shutdownProbe) waitSettlement(ctx context.Context) error {
	select {
	case err := <-probe.settled:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (ck *CommandKernel) submitAndWait(ctx context.Context, request Request) error {
	terminal := make(chan error, 1)
	if err := ck.submit(ctx, request, terminal); err != nil {
		return err
	}
	select {
	case err := <-terminal:
		return err
	case <-ctx.Done():
		return ctx.Err()
	case <-ck.done:
		return ck.Wait(context.Background())
	}
}

type runShutdownBarrierFunc func(context.Context, uint64) error

func (fn runShutdownBarrierFunc) BeforeFunctionCatalogClose(ctx context.Context, generation uint64) error {
	return fn(ctx, generation)
}

type runFinalizerFunc func(context.Context, uint64) error

func (fn runFinalizerFunc) FinalizeRun(ctx context.Context, generation uint64) error {
	return fn(ctx, generation)
}

// admit is a test-only convenience that submits an already-prepared command
// through the kernel admission path.
func (ck *CommandKernel) admit(request Request, plan WorkPlan) error {
	return ck.admitSubmission(request, plan, nil, nil, nil, false)
}

func newNoopRunFinalizer() RunFinalizer {
	return runFinalizerFunc(func(context.Context, uint64) error { return nil })
}

func newNoopRunShutdownBarrier() RunShutdownBarrier {
	return runShutdownBarrierFunc(func(context.Context, uint64) error { return nil })
}
