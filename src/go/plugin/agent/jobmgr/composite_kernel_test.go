// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"reflect"
	"strings"
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

func compositeTestPlan(
	id string,
	claims []string,
	onApply func(),
) WorkPlan {
	return WorkPlan{
		Claims:     claims,
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: id,
			Prepare: func(
				context.Context,
				lifecycle.ReadyResource,
				lifecycle.ResourceTransactionScope,
				lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return &simpleCompositeChildTransaction{
					scope: lifecycle.ResourceTransactionScope{ID: id},
					apply: onApply,
				}, nil
			},
		},
	}
}

func compositeParentTestPlan(
	id string,
	claims []string,
	apply func(context.Context, CompositeCommandScope) error,
) WorkPlan {
	return WorkPlan{
		Claims:     claims,
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: id,
			PrepareComposite: func(
				_ context.Context,
				_ lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				_ lifecycle.LongLivedPermit,
			) (PreparedCompositeResourceTransaction, error) {
				return &compositeTestTransaction{
					scope: scope,
					apply: apply,
				}, nil
			},
		},
	}
}

func stopCompositeTestKernel(
	t *testing.T,
	kernel *CommandKernel,
) {
	t.Helper()
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

func waitForCompositeRecords(
	t *testing.T,
	admission *lifecycle.AdmissionLedger,
	count int,
) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if census := admission.Census(); census.ActiveRecords >= count {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf(
				"active admission records did not reach %d: %+v",
				count,
				admission.Census(),
			)
		}
		time.Sleep(time.Millisecond)
	}
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

func TestCompositeChildRejectsActiveParentLane(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	startKernelLoop(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	childErr := make(chan error, 1)
	parentPlan := compositeParentTestPlan(
		"parent",
		[]string{"graph"},
		func(
			ctx context.Context,
			commands CompositeCommandScope,
		) error {
			childErr <- commands.SubmitPreparedAndWait(
				ctx,
				Request{
					UID:     "same-lane-child",
					LaneKey: "parent",
					Source:  lifecycle.SourceJobManager,
					Route:   "internal/test/same-lane-child",
				},
				compositeTestPlan(
					"parent",
					[]string{"graph"},
					nil,
				),
			)
			return nil
		},
	)
	parentDone := make(chan error, 1)
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "same-lane-parent",
				LaneKey: "parent",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/same-lane-parent",
			},
			parentPlan,
		)
	}()

	select {
	case err := <-childErr:
		if err == nil ||
			!strings.Contains(err.Error(), "active parent lane") {
			t.Fatalf("same-lane child error=%v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("same-lane child was not rejected promptly")
	}
	select {
	case err := <-parentDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("parent did not complete after same-lane rejection")
	}
	if err := run.DirtyCause(); err != nil {
		t.Fatalf("same-lane rejection dirtied run: %v", err)
	}
	stopCompositeTestKernel(t, kernel)
}

func TestCompositeChildContinuationPrecedesRunnableTargetLaneWork(
	t *testing.T,
) {
	kernel, run, admission, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	startKernelLoop(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	var mu sync.Mutex
	var applied []string
	blockerEntered := make(chan struct{})
	blockerRelease := make(chan struct{})
	blockerDone := make(chan error, 1)
	go func() {
		blockerDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "fifo-blocker",
				LaneKey: "job",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/fifo-blocker",
			},
			compositeTestPlan("job", nil, func() {
				close(blockerEntered)
				<-blockerRelease
			}),
		)
	}()
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("target-lane blocker did not start")
	}

	parentEntered := make(chan struct{})
	submitChild := make(chan struct{})
	parentDone := make(chan error, 1)
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "fifo-parent",
				LaneKey: "parent",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/fifo-parent",
			},
			compositeParentTestPlan(
				"parent",
				[]string{"graph"},
				func(
					ctx context.Context,
					commands CompositeCommandScope,
				) error {
					close(parentEntered)
					<-submitChild
					return commands.SubmitPreparedAndWait(
						ctx,
						Request{
							UID:     "fifo-child",
							LaneKey: "job",
							Source:  lifecycle.SourceJobManager,
							Route:   "internal/test/fifo-child",
						},
						compositeTestPlan(
							"job",
							[]string{"graph"},
							func() {
								mu.Lock()
								applied = append(
									applied,
									"child",
								)
								mu.Unlock()
							},
						),
					)
				},
			),
		)
	}()
	select {
	case <-parentEntered:
	case <-time.After(time.Second):
		t.Fatal("FIFO parent did not enter apply")
	}

	if err := kernel.SubmitPrepared(
		t.Context(),
		Request{
			UID:     "fifo-normal",
			LaneKey: "job",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test/fifo-normal",
		},
		compositeTestPlan("job", nil, func() {
			mu.Lock()
			applied = append(applied, "normal")
			mu.Unlock()
		}),
	); err != nil {
		t.Fatal(err)
	}
	close(submitChild)
	waitForCompositeRecords(t, admission, 4)
	close(blockerRelease)

	select {
	case err := <-blockerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("target-lane blocker did not finish")
	}
	select {
	case err := <-parentDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("FIFO parent did not finish")
	}
	mu.Lock()
	got := append([]string(nil), applied...)
	mu.Unlock()
	if want := []string{"child", "normal"}; !reflect.DeepEqual(
		got,
		want,
	) {
		t.Fatalf("target-lane apply order=%v want=%v", got, want)
	}
	if err := run.DirtyCause(); err != nil {
		t.Fatalf("FIFO run dirtied: %v", err)
	}
	stopCompositeTestKernel(t, kernel)
}

func TestCompositeFenceDefersConflictingAdmissionButNotUnrelatedWork(
	t *testing.T,
) {
	kernel, run, admission, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	startKernelLoop(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	childEntered := make(chan struct{})
	childRelease := make(chan struct{})
	parentDone := make(chan error, 1)
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "fence-parent",
				LaneKey: "fence-parent",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/fence-parent",
			},
			compositeParentTestPlan(
				"fence-parent",
				[]string{"graph"},
				func(
					ctx context.Context,
					commands CompositeCommandScope,
				) error {
					return commands.SubmitPreparedAndWait(
						ctx,
						Request{
							UID:     "fence-child",
							LaneKey: "fence-child",
							Source:  lifecycle.SourceJobManager,
							Route:   "internal/test/fence-child",
						},
						compositeTestPlan(
							"fence-child",
							[]string{"graph"},
							func() {
								close(childEntered)
								<-childRelease
							},
						),
					)
				},
			),
		)
	}()
	select {
	case <-childEntered:
	case <-time.After(time.Second):
		t.Fatal("composite child did not start")
	}

	beforeConflict := admission.Census()
	capacity := lifecycle.OrdinaryBudgetBytes -
		beforeConflict.ProcessBytes
	available := capacity - beforeConflict.OrdinaryBytes
	conflictingRequest := Request{
		UID:     "fence-conflicting",
		LaneKey: "fence-conflicting",
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/test/fence-conflicting",
	}
	conflictingApplied := make(chan struct{})
	conflictingPlan := compositeTestPlan(
		"fence-conflicting",
		[]string{"graph"},
		func() { close(conflictingApplied) },
	)
	base, err := operationAdmissionBytes(
		conflictingRequest,
		conflictingPlan,
	)
	if err != nil {
		t.Fatal(err)
	}
	conflictingPlan.OwnedBytes = available - base
	if conflictingPlan.OwnedBytes <= 0 {
		t.Fatalf(
			"invalid conflicting byte calculation: capacity=%d census=%+v base=%d",
			capacity,
			beforeConflict,
			base,
		)
	}
	conflictingPlan, err = prepareOwnedJobPlan(
		conflictingRequest,
		conflictingPlan,
	)
	if err != nil {
		t.Fatal(err)
	}
	conflictingAdmitted := make(chan error, 1)
	conflictingDone := make(chan error, 1)
	if err := kernel.enqueueSubmission(
		t.Context(),
		conflictingRequest.Source,
		submission{
			request:  conflictingRequest,
			plan:     conflictingPlan,
			context:  t.Context(),
			result:   conflictingAdmitted,
			terminal: conflictingDone,
		},
	); err != nil {
		t.Fatal(err)
	}
	select {
	case <-kernel.submissionSpace[sourceIndex(
		conflictingRequest.Source,
	)]:
	case <-time.After(time.Second):
		t.Fatal("conflicting submission was not dequeued")
	}

	unrelatedApplied := make(chan struct{})
	unrelatedDone := make(chan error, 1)
	go func() {
		unrelatedDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "fence-unrelated",
				LaneKey: "fence-unrelated",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/fence-unrelated",
			},
			compositeTestPlan(
				"fence-unrelated",
				nil,
				func() { close(unrelatedApplied) },
			),
		)
	}()
	select {
	case <-unrelatedApplied:
	case <-time.After(time.Second):
		t.Fatalf(
			"unrelated work was starved by conflicting admission: %+v",
			admission.Census(),
		)
	}
	select {
	case err := <-conflictingAdmitted:
		t.Fatalf(
			"conflicting operation admitted before parent terminal: %v",
			err,
		)
	default:
	}
	select {
	case err := <-unrelatedDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("unrelated operation did not finish")
	}

	close(childRelease)
	select {
	case err := <-parentDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("composite parent did not finish")
	}
	select {
	case err := <-conflictingAdmitted:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("conflicting operation was not admitted after parent terminal")
	}
	select {
	case <-conflictingApplied:
	case <-time.After(time.Second):
		t.Fatal("conflicting operation did not run after parent terminal")
	}
	select {
	case err := <-conflictingDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("conflicting operation did not reach terminal")
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.OrdinaryBytes != 0 {
		t.Fatalf("composite fence admission did not converge: %+v", census)
	}
	if err := run.DirtyCause(); err != nil {
		t.Fatalf("composite fence run dirtied: %v", err)
	}
	stopCompositeTestKernel(t, kernel)
}

func TestCompositeChildGetsParentLinkedProgressAdmission(
	t *testing.T,
) {
	kernel, run, admission, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	startKernelLoop(t, kernel)
	if err := run.OpenAdmission(); err != nil {
		t.Fatal(err)
	}

	parentEntered := make(chan struct{})
	submitChild := make(chan struct{})
	childEntered := make(chan struct{})
	childRelease := make(chan struct{})
	parentDone := make(chan error, 1)
	go func() {
		parentDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			Request{
				UID:     "progress-parent",
				LaneKey: "parent",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/progress-parent",
			},
			compositeParentTestPlan(
				"parent",
				[]string{"graph"},
				func(
					ctx context.Context,
					commands CompositeCommandScope,
				) error {
					close(parentEntered)
					<-submitChild
					return commands.SubmitPreparedAndWait(
						ctx,
						Request{
							UID:     "progress-child",
							LaneKey: "child",
							Source:  lifecycle.SourceJobManager,
							Route:   "internal/test/progress-child",
						},
						compositeTestPlan(
							"child",
							[]string{"graph"},
							func() {
								close(childEntered)
								<-childRelease
							},
						),
					)
				},
			),
		)
	}()
	select {
	case <-parentEntered:
	case <-time.After(time.Second):
		t.Fatal("progress parent did not enter apply")
	}

	beforeBlocker := admission.Census()
	capacity := lifecycle.OrdinaryBudgetBytes -
		beforeBlocker.ProcessBytes
	blockerRequest := Request{
		UID:     "progress-blocker",
		LaneKey: "blocker",
		Source:  lifecycle.SourceJobManager,
		Route:   "internal/test/progress-blocker",
	}
	blockerEntered := make(chan struct{})
	blockerRelease := make(chan struct{})
	blockerPlan := compositeTestPlan(
		"blocker",
		nil,
		func() {
			close(blockerEntered)
			<-blockerRelease
		},
	)
	base, err := operationAdmissionBytes(
		blockerRequest,
		blockerPlan,
	)
	if err != nil {
		t.Fatal(err)
	}
	blockerPlan.OwnedBytes =
		capacity - beforeBlocker.OrdinaryBytes - base
	if blockerPlan.OwnedBytes <= 0 {
		t.Fatalf(
			"invalid blocker byte calculation: capacity=%d census=%+v base=%d",
			capacity,
			beforeBlocker,
			base,
		)
	}
	blockerDone := make(chan error, 1)
	go func() {
		blockerDone <- kernel.SubmitPreparedAndWait(
			t.Context(),
			blockerRequest,
			blockerPlan,
		)
	}()
	select {
	case <-blockerEntered:
	case <-time.After(time.Second):
		t.Fatal("ordinary-budget blocker did not start")
	}
	if census := admission.Census(); census.OrdinaryBytes != capacity {
		t.Fatalf(
			"ordinary budget not fully held: capacity=%d census=%+v",
			capacity,
			census,
		)
	}

	close(submitChild)
	select {
	case <-childEntered:
	case <-time.After(time.Second):
		t.Fatal(
			"composite child did not receive parent-linked progress admission",
		)
	}
	if census := admission.Census(); census.OrdinaryBytes <= capacity {
		t.Fatalf(
			"composite progress was not represented as bounded overcommit: %+v",
			census,
		)
	}
	close(childRelease)
	select {
	case err := <-parentDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("progress parent did not finish")
	}
	close(blockerRelease)
	select {
	case err := <-blockerDone:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("ordinary-budget blocker did not finish")
	}
	if census := admission.Census(); census.ActiveRecords != 0 ||
		census.OrdinaryBytes != 0 {
		t.Fatalf("composite progress admission leaked: %+v", census)
	}
	if err := run.DirtyCause(); err != nil {
		t.Fatalf("progress run dirtied: %v", err)
	}
	stopCompositeTestKernel(t, kernel)
}
