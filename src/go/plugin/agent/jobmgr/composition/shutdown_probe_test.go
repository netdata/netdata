// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type compositionShutdownProbe struct {
	cancelled <-chan struct{}
	settled   <-chan error
}

func startCompositionShutdownProbe(
	ctx context.Context,
	kernel *jobmgr.CommandKernel,
	uid string,
) (compositionShutdownProbe, error) {
	if ctx == nil || kernel == nil || uid == "" {
		return compositionShutdownProbe{}, errors.New("invalid shutdown probe")
	}
	started := make(chan struct{})
	cancelled := make(chan struct{})
	settled := make(chan error, 1)
	go func() {
		settled <- kernel.SubmitPreparedAndWait(
			context.Background(),
			jobmgr.Request{
				UID:     uid,
				LaneKey: uid,
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/shutdown-probe",
			},
			jobmgr.WorkPlan{
				Work: func(ctx context.Context) (lifecycle.TaskOutcome, error) {
					close(started)
					<-ctx.Done()
					close(cancelled)
					result, err := lifecycle.NewControlResult(lifecycle.ControlInternal)
					if err != nil {
						return lifecycle.TaskOutcome{}, err
					}
					return lifecycle.NewFrameOutcome(result)
				},
			},
		)
	}()
	select {
	case <-started:
		return compositionShutdownProbe{
			cancelled: cancelled,
			settled:   settled,
		}, nil
	case err := <-settled:
		return compositionShutdownProbe{}, errors.Join(errors.New("shutdown probe settled before starting"), err)
	case <-ctx.Done():
		return compositionShutdownProbe{}, ctx.Err()
	}
}

func (probe compositionShutdownProbe) waitCancellation(ctx context.Context) error {
	select {
	case <-probe.cancelled:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (probe compositionShutdownProbe) waitSettlement(ctx context.Context) error {
	select {
	case err := <-probe.settled:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}
