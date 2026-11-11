// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestSubmissionPreservesAdmissionResultAfterShutdown(t *testing.T) {
	tests := map[string]struct {
		reject bool
		cancel bool
	}{
		"accepted":                       {},
		"rejected":                       {reject: true},
		"accepted with cancelled caller": {cancel: true},
		"rejected with cancelled caller": {reject: true, cancel: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			// Both the buffered result and kernel completion are ready before
			// observation. Repeat to exercise the competing select branches.
			for range 32 {
				planned := make(chan struct{})
				rejection := errors.New("route rejected")
				kernel, run, uids, _ := newKernelWithPlanner(t, plannerFunc(func(context.Context, string, []string) (WorkPlan, error) {
					close(planned)
					if test.reject {
						return WorkPlan{}, rejection
					}
					return stoppedKernelPlanner{}.Plan(Request{})
				}))
				require.NoError(t, run.OpenAdmission())
				ctx, cancel := context.WithCancel(t.Context())
				observer := &submissionObserverContext{
					Context: ctx,
					entered: make(chan struct{}),
					release: make(chan struct{}),
				}
				var releaseOnce sync.Once
				release := func() { releaseOnce.Do(func() { close(observer.release) }) }
				t.Cleanup(release)
				t.Cleanup(cancel)
				submitted := make(chan error, 1)
				go func() {
					submitted <- kernel.Submit(observer, Request{
						UID:    "delayed-observer",
						Source: lifecycle.SourceFunction,
						Route:  "route",
					})
				}()
				select {
				case <-observer.entered:
				case <-time.After(time.Second):
					t.Fatal("submission observer did not reach its gate")
				}
				require.NoError(t, kernel.Start(t.Context()))
				select {
				case <-planned:
				case <-time.After(time.Second):
					t.Fatal("submission was not planned")
				}
				kernel.Stop()
				waitCtx, cancelWait := context.WithTimeout(t.Context(), time.Second)
				require.NoError(t, kernel.Wait(waitCtx))
				cancelWait()
				if test.cancel {
					cancel()
				}
				release()
				select {
				case err := <-submitted:
					if test.reject {
						require.ErrorIs(t, err, rejection)
					} else {
						require.NoError(t, err)
					}
				case <-time.After(time.Second):
					t.Fatal("submission observer did not finish")
				}
				cancel()
				closeUIDLedger(t, uids)
			}
		})
	}
}

// Pause only the submission observer; the kernel can admit and settle its work.
type submissionObserverContext struct {
	context.Context
	once    sync.Once
	entered chan struct{}
	release chan struct{}
}

func (ctx *submissionObserverContext) Done() <-chan struct{} {
	ctx.once.Do(func() {
		close(ctx.entered)
		<-ctx.release
	})
	return ctx.Context.Done()
}
