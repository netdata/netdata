// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestSubmitPreparedAndWaitJoinsAcceptedCancellation(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	loop, err := NewKernelLoop(kernel)
	require.NoError(t, err)

	require.NoError(t, loop.Start(t.Context()))

	require.NoError(t, run.OpenAdmission())

	started := make(chan struct{})
	release := make(chan struct{})
	result, err := lifecycle.NewSealedResult(200, "application/json", []byte(`{}`))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	returned := make(chan error, 1)
	go func() {
		returned <- kernel.SubmitPreparedAndWait(
			ctx,
			Request{
				UID:     "prepared-join-cancel",
				LaneKey: "prepared-join-cancel",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			WorkPlan{
				Work: lifecycle.FrameTaskWork(
					func(context.Context) (
						lifecycle.SealedResult,
						error,
					) {
						close(started)
						<-release
						return result, nil
					},
				),
			},
		)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "prepared command did not start")
	}
	cancel()
	select {
	case err := <-returned:
		require.FailNowf(t, "test failed", "accepted prepared command returned before terminal disposal: %v", err)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-returned:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "prepared command was not joined after terminal disposal")
	}
	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(context.Background(), time.Second)
	defer waitCancel()

	require.NoError(t, kernel.Wait(waitCtx))
}
