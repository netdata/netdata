// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestSubmitPreparedAndWaitJoinsAcceptedCancellation(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	loop, err := NewKernelLoop(kernel)
	if err != nil {
		t.Fatal(err)
	}
	if err := loop.Start(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}
	started := make(chan struct{})
	release := make(chan struct{})
	result, err := lifecycle.NewSealedResult(
		200,
		"application/json",
		[]byte(`{}`),
	)
	if err != nil {
		t.Fatal(err)
	}
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
		t.Fatal("prepared command did not start")
	}
	cancel()
	select {
	case err := <-returned:
		t.Fatalf(
			"accepted prepared command returned before terminal disposal: %v",
			err,
		)
	case <-time.After(25 * time.Millisecond):
	}
	close(release)
	select {
	case <-returned:
	case <-time.After(time.Second):
		t.Fatal("prepared command was not joined after terminal disposal")
	}
	kernel.Stop()
	waitCtx, waitCancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer waitCancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
}
