// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type compositeTestTransaction struct {
	scope lifecycle.ResourceTransactionScope
	apply func(
		context.Context,
		CompositeCommandScope,
	) error
}

func (transaction *compositeTestTransaction) Scope() lifecycle.ResourceTransactionScope {
	return transaction.scope
}

func (transaction *compositeTestTransaction) ApplyComposite(
	ctx context.Context,
	commands CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	if transaction.apply != nil {
		if err := transaction.apply(ctx, commands); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	return compositeTestApplied(transaction.scope)
}

func (transaction *compositeTestTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	return nil, nil
}

type simpleCompositeChildTransaction struct {
	scope lifecycle.ResourceTransactionScope
	apply func()
}

func (transaction *simpleCompositeChildTransaction) Scope() lifecycle.ResourceTransactionScope {
	return transaction.scope
}

func (transaction *simpleCompositeChildTransaction) Apply(
	context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	if transaction.apply != nil {
		transaction.apply()
	}
	return compositeTestApplied(transaction.scope)
}

func (transaction *simpleCompositeChildTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	return nil, nil
}

func compositeTestApplied(
	scope lifecycle.ResourceTransactionScope,
) (lifecycle.AppliedResourceTransaction, error) {
	result, err := lifecycle.NewSealedResult(
		204,
		"text/plain",
		nil,
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		scope,
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		func() error { return nil },
	)
}

func TestCompositeChildBypassesParentClaimWaiterOnTargetLane(
	t *testing.T,
) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	startKernelLoop(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var applied []string
	parentEntered := make(chan struct{})
	submitChild := make(chan struct{})
	normalApplied := make(chan struct{})
	parentDone := make(chan error, 1)

	transactionPlan := func(
		id string,
		onApply func(),
	) *ResourceTransactionPlan {
		return &ResourceTransactionPlan{
			ID: id,
			Prepare: func(
				context.Context,
				lifecycle.ReadyResource,
				lifecycle.ResourceTransactionScope,
				lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return &simpleCompositeChildTransaction{
					scope: lifecycle.ResourceTransactionScope{
						ID: id,
					},
					apply: onApply,
				}, nil
			},
		}
	}
	childPlan := WorkPlan{
		Claims:     []string{"graph"},
		NoResponse: true,
		Transaction: transactionPlan("job", func() {
			mu.Lock()
			applied = append(applied, "child")
			mu.Unlock()
		}),
	}
	parentPlan := WorkPlan{
		Claims:     []string{"graph"},
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: "parent",
			PrepareComposite: func(
				_ context.Context,
				_ lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				_ lifecycle.LongLivedPermit,
			) (PreparedCompositeResourceTransaction, error) {
				return &compositeTestTransaction{
					scope: scope,
					apply: func(
						ctx context.Context,
						commands CompositeCommandScope,
					) error {
						close(parentEntered)
						select {
						case <-submitChild:
						case <-ctx.Done():
							return context.Cause(ctx)
						}
						return commands.SubmitPreparedAndWait(
							ctx,
							Request{
								UID:     "composite-child",
								LaneKey: "job",
								Source:  lifecycle.SourceJobManager,
								Route:   "internal/test/child",
							},
							childPlan,
						)
					},
				}, nil
			},
		},
	}
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "composite-parent",
				LaneKey: "parent",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/parent",
			},
			parentPlan,
		)
	}()
	select {
	case <-parentEntered:
	case <-time.After(time.Second):
		t.Fatal("composite parent did not enter apply")
	}

	normalPlan := WorkPlan{
		Claims:     []string{"graph"},
		NoResponse: true,
		Transaction: transactionPlan("job", func() {
			mu.Lock()
			applied = append(applied, "normal")
			mu.Unlock()
			close(normalApplied)
		}),
	}
	if err := kernel.SubmitPrepared(
		t.Context(),
		Request{
			UID:     "normal-claim-waiter",
			LaneKey: "job",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test/normal",
		},
		normalPlan,
	); err != nil {
		t.Fatal(err)
	}
	close(submitChild)

	select {
	case err := <-parentDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal(
			"composite child deadlocked behind the parent's claim waiter",
		)
	}
	select {
	case <-normalApplied:
	case <-time.After(time.Second):
		t.Fatalf(
			"normal claim waiter did not run after parent release: dirty=%v",
			run.DirtyCause(),
		)
	}
	mu.Lock()
	got := append([]string(nil), applied...)
	mu.Unlock()
	if want := []string{"child", "normal"}; !reflect.DeepEqual(
		got,
		want,
	) {
		t.Fatalf("apply order=%v want=%v", got, want)
	}

	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()
	if err := kernel.Wait(waitCtx); err != nil &&
		!errors.Is(err, ErrStopped) {
		t.Fatal(err)
	}
}
