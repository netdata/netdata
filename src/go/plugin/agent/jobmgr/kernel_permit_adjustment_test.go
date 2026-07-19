// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type sizedKernelTransaction struct {
	scope         lifecycle.ResourceTransactionScope
	permit        lifecycle.LongLivedPermit
	resourceBytes int64
	applied       chan int64
	disposed      chan struct{}
	disposeOnce   sync.Once
}

func (transaction *sizedKernelTransaction) Scope() lifecycle.ResourceTransactionScope {
	return transaction.scope
}

func (transaction *sizedKernelTransaction) LongLivedResourceBytes() (
	int64,
	error,
) {
	return transaction.resourceBytes, nil
}

func (transaction *sizedKernelTransaction) Apply(
	context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	transaction.applied <- transaction.permit.CapacityBytes()
	if err := transaction.permit.AbortUnused(); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return compositeTestApplied(transaction.scope)
}

func (transaction *sizedKernelTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	transaction.disposeOnce.Do(func() {
		close(transaction.disposed)
	})
	return nil, transaction.permit.AbortUnused()
}

func sizedTransactionPlan(
	t *testing.T,
	id string,
	initialResourceBytes,
	actualResourceBytes int64,
	applied chan int64,
	disposed chan struct{},
) WorkPlan {
	t.Helper()
	permit, err := lifecycle.NewSecretStoreLongLivedPlan(
		initialResourceBytes,
	)
	if err != nil {
		t.Fatal(err)
	}
	return WorkPlan{
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID:                id,
			AllocateSuccessor: true,
			Permit:            permit,
			Prepare: func(
				_ context.Context,
				_ lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return &sizedKernelTransaction{
					scope:         scope,
					permit:        permit,
					resourceBytes: actualResourceBytes,
					applied:       applied,
					disposed:      disposed,
				}, nil
			},
		},
	}
}

func TestKernelAdjustsPreparedTransactionPermitBeforeApply(
	t *testing.T,
) {
	tests := map[string]struct {
		initial int64
		actual  int64
	}{
		"grow": {
			initial: 512,
			actual:  2_048,
		},
		"shrink": {
			initial: 2_048,
			actual:  512,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel, run, _, _, tasks :=
				newKernelWithPlanner(
					t,
					stoppedKernelPlanner{},
				)
			startKernelLoop(t, kernel)
			if err := run.OpenAdmission(); err != nil {
				t.Fatal(err)
			}
			applied := make(chan int64, 1)
			disposed := make(chan struct{})
			if err := kernel.SubmitPreparedAndWait(
				t.Context(),
				Request{
					UID:     "permit-adjust-" + name,
					LaneKey: "permit-adjust-" + name,
					Source:  lifecycle.SourceJobManager,
					Route:   "internal/test/permit-adjust",
				},
				sizedTransactionPlan(
					t,
					"permit-adjust-"+name,
					test.initial,
					test.actual,
					applied,
					disposed,
				),
			); err != nil {
				t.Fatal(err)
			}
			select {
			case got := <-applied:
				want := test.actual +
					lifecycle.TaskChildExecutionBytes
				if got != want {
					t.Fatalf(
						"applied permit bytes=%d want=%d",
						got,
						want,
					)
				}
			default:
				t.Fatal("adjusted transaction did not apply")
			}
			if census := tasks.LongLivedCensus(); census !=
				(lifecycle.LongLivedCensus{}) {
				t.Fatalf(
					"adjusted transaction retained permit: %+v",
					census,
				)
			}
			kernel.Stop()
			waitCtx, cancel := context.WithTimeout(
				context.Background(),
				time.Second,
			)
			defer cancel()
			if err := kernel.Wait(waitCtx); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestKernelCancelsPreparedTransactionWaitingForPermitGrowth(
	t *testing.T,
) {
	kernel, run, admission, _, tasks := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	startKernelLoop(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	blockerEntered := make(chan struct{})
	blockerRelease := make(chan struct{})
	blockerDone := make(chan error, 1)
	go func() {
		blockerDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "permit-growth-blocker",
				LaneKey: "permit-growth-blocker",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/permit-growth-blocker",
			},
			WorkPlan{
				OwnedBytes: 100 * 1024 * 1024,
				Work: lifecycle.FrameTaskWork(
					func(context.Context) (
						lifecycle.SealedResult,
						error,
					) {
						close(blockerEntered)
						<-blockerRelease
						return lifecycle.NewSealedResult(
							204,
							"text/plain",
							nil,
						)
					},
				),
			},
		)
	}()
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("admission blocker did not start")
	}

	applied := make(chan int64, 1)
	disposed := make(chan struct{})
	deadline := time.Now().Add(100 * time.Millisecond)
	transactionDone := make(chan error, 1)
	go func() {
		transactionDone <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:      "permit-growth-waiter",
				LaneKey:  "permit-growth-waiter",
				Source:   lifecycle.SourceJobManager,
				Route:    "internal/test/permit-growth-waiter",
				Deadline: deadline,
			},
			sizedTransactionPlan(
				t,
				"permit-growth-waiter",
				512,
				64*1024*1024,
				applied,
				disposed,
			),
		)
	}()
	select {
	case <-disposed:
	case <-time.After(time.Second):
		t.Fatal(
			"deadline did not dispose transaction waiting for permit growth",
		)
	}
	select {
	case <-applied:
		t.Fatal("capacity-rejected transaction applied")
	default:
	}
	select {
	case <-transactionDone:
	case <-time.After(time.Second):
		t.Fatal("disposed growth waiter did not reach terminal")
	}
	if census := tasks.LongLivedCensus(); census !=
		(lifecycle.LongLivedCensus{}) {
		t.Fatalf(
			"disposed growth waiter retained permit: %+v",
			census,
		)
	}

	close(blockerRelease)
	select {
	case err := <-blockerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("admission blocker did not finish")
	}
	if census := admission.Census(); census.LongLivedRecords != 0 ||
		census.LongLivedBytes != 0 {
		t.Fatalf(
			"cancelled permit growth retained admission: %+v",
			census,
		)
	}
	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()
	if err := kernel.Wait(waitCtx); err != nil {
		t.Fatal(err)
	}
}
