// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"io"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

type compositeTestTransaction struct {
	scope lifecycle.ResourceTransactionScope
	apply func(context.Context, CompositeCommandScope) error
}

func compositeTestPlan(id string, claims []string, onApply func()) WorkPlan {
	return WorkPlan{
		Claims:     claims,
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: id,
			Prepare: func(
				_ context.Context,
				_ lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return &simpleCompositeChildTransaction{
					scope:  scope,
					apply:  onApply,
					permit: permit,
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

func stopCompositeTestKernel(t *testing.T, kernel *testCommandKernel) {
	t.Helper()
	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := kernel.Wait(waitCtx)
	require.False(t, err != nil && !errors.Is(err, ErrStopped))
}

func waitForAdmittedOperations(t *testing.T, observer *kernelRuntimeObserver, count int) {
	t.Helper()
	deadline := time.Now().Add(time.Second)
	for {
		if observer.operationsAdmitted.Load() >= uint64(count) {
			return
		}
		require.False(t, time.Now().After(deadline))
		time.Sleep(time.Millisecond)
	}
}

func (ctt *compositeTestTransaction) Scope() lifecycle.ResourceTransactionScope {
	return ctt.scope
}

func (ctt *compositeTestTransaction) ApplyComposite(
	ctx context.Context,
	commands CompositeCommandScope,
) (lifecycle.AppliedResourceTransaction, error) {
	if ctt.apply != nil {
		if err := ctt.apply(ctx, commands); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	return compositeTestApplied(ctt.scope)
}

func (ctt *compositeTestTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	return nil, nil
}

type simpleCompositeChildTransaction struct {
	scope  lifecycle.ResourceTransactionScope
	apply  func()
	permit lifecycle.LongLivedPermit
}

func (scct *simpleCompositeChildTransaction) Scope() lifecycle.ResourceTransactionScope {
	return scct.scope
}

func (scct *simpleCompositeChildTransaction) Apply(context.Context) (lifecycle.AppliedResourceTransaction, error) {
	if scct.apply != nil {
		scct.apply()
	}
	if scct.permit.Valid() {
		if err := scct.permit.AbortUnused(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		scct.permit = lifecycle.LongLivedPermit{}
	}
	return compositeTestApplied(scct.scope)
}

func (scct *simpleCompositeChildTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	if !scct.permit.Valid() {
		return nil, nil
	}
	err := scct.permit.AbortUnused()
	scct.permit = lifecycle.LongLivedPermit{}
	return nil, err
}

func compositeTestApplied(scope lifecycle.ResourceTransactionScope) (lifecycle.AppliedResourceTransaction, error) {
	result, err := lifecycle.NewSealedResult(204, "text/plain", nil)
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

func TestCompositeChildBypassesParentClaimWaiterOnTargetLane(t *testing.T) {
	kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	startKernelLoop(t, kernel)

	require.NoError(t, run.OpenAdmission())

	var mu sync.Mutex
	var applied []string
	parentEntered := make(chan struct{})
	submitChild := make(chan struct{})
	normalApplied := make(chan struct{})
	parentDone := make(chan error, 1)

	transactionPlan := func(id string, onApply func()) *ResourceTransactionPlan {
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
					apply: func(ctx context.Context, commands CompositeCommandScope) error {
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
		require.FailNow(t, "test failed", "composite parent did not enter apply")
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

	require.NoError(t, kernel.SubmitPrepared(
		t.Context(),
		Request{
			UID:     "normal-claim-waiter",
			LaneKey: "job",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test/normal",
		},
		normalPlan,
	),
	)

	close(submitChild)

	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "composite child deadlocked behind the parent's claim waiter")
	}
	select {
	case <-normalApplied:
	case <-time.After(time.Second):
		require.FailNowf(
			t,
			"test failed",
			"normal claim waiter did not run after parent release: dirty=%v",
			run.DirtyCause(),
		)
	}
	mu.Lock()
	got := append([]string(nil), applied...)
	mu.Unlock()

	want := []string{"child", "normal"}
	require.Equal(t, want, got)

	kernel.Stop()
	waitCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	err := kernel.Wait(waitCtx)
	require.False(t, err != nil && !errors.Is(err, ErrStopped))
}

func TestCompositeChildCancelledBeforeApplyReportsTerminalCause(t *testing.T) {
	tests := map[string]struct {
		deadline bool
		want     error
	}{
		"deadline": {
			deadline: true,
			want:     context.DeadlineExceeded,
		},
		"explicit cancellation": {
			want: context.Canceled,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			clock := newKernelFinalizerClock()
			kernel, run, uids, tasks := newKernelWithClockFinalizerAndTimeout(
				t,
				stoppedKernelPlanner{},
				io.Discard,
				clock,
				newNoopRunFinalizer(),
				time.Second,
			)
			require.NoError(t, run.OpenAdmission())
			startKernelLoop(t, kernel)

			childPrepareEntered := make(chan struct{})
			childPlan := WorkPlan{
				NoResponse: true,
				Transaction: &ResourceTransactionPlan{
					ID: "cancelled-composite-child",
					Prepare: func(
						ctx context.Context,
						_ lifecycle.ReadyResource,
						scope lifecycle.ResourceTransactionScope,
						_ lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResourceTransaction, error) {
						close(childPrepareEntered)
						<-ctx.Done()
						return &simpleCompositeChildTransaction{
							scope: scope,
						}, nil
					},
				},
			}
			childResult := make(chan error, 1)
			parentPlan := compositeParentTestPlan(
				"cancelled-composite-parent",
				nil,
				func(ctx context.Context, commands CompositeCommandScope) error {
					childResult <- commands.SubmitPreparedAndWait(ctx, Request{
						UID:     "cancelled-composite-child",
						LaneKey: "cancelled-composite-child",
						Source:  lifecycle.SourceJobManager,
						Route:   "internal/test/cancelled-composite-child",
					}, childPlan)
					return nil
				},
			)
			request := Request{
				UID:     "cancelled-composite-parent",
				LaneKey: "cancelled-composite-parent",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test/cancelled-composite-parent",
			}
			if test.deadline {
				request.Deadline = clock.Now().Add(time.Second)
			}
			parentResult := make(chan error, 1)
			go func() {
				parentResult <- kernel.SubmitPreparedAndWait(context.Background(), request, parentPlan)
			}()
			select {
			case <-childPrepareEntered:
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "composite child preparation did not start")
			}
			if test.deadline {
				clock.advance(time.Second + time.Nanosecond)
				kernel.NotifyControlReady()
			} else {
				require.NoError(t, kernel.Cancel(context.Background(), request.UID))
			}
			select {
			case err := <-childResult:
				require.ErrorIs(t, err, test.want)
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "cancelled composite child did not reach terminal state")
			}
			select {
			case err := <-parentResult:
				require.NoError(t, err)
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "cancelled composite parent did not reach terminal state")
			}

			kernel.Stop()
			require.NoError(t, kernel.Wait(context.Background()))
			require.NoError(t, run.DirtyCause())
			require.False(t, tasks.Active() != 0 || tasks.Pending() != 0)
			closeUIDLedger(t, uids)
		})
	}
}

func TestCompositeActionSubmitsChildAfterShutdownCut(t *testing.T) {
	tests := map[string]struct {
		suffix string
		submit func(context.Context, CompositeCommandScope, Request, WorkPlan) error
	}{
		"normal child": {
			suffix: "normal",
			submit: func(ctx context.Context, commands CompositeCommandScope, request Request, plan WorkPlan) error {
				return commands.SubmitPreparedAndWait(ctx, request, plan)
			},
		},
		"rollback child": {
			suffix: "rollback",
			submit: func(_ context.Context, commands CompositeCommandScope, request Request, plan WorkPlan) error {
				return commands.SubmitRollbackAndWait(request, plan)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
			diagnostics := &recordingDiagnosticObserver{
				trace: true,
			}
			require.NoError(t, kernel.BindDiagnosticObserver(diagnostics))
			startKernelLoop(t, kernel)
			require.NoError(t, run.OpenAdmission())
			probeCtx, cancelProbe := context.WithTimeout(context.Background(), time.Second)
			defer cancelProbe()
			probe, err := startShutdownProbe(probeCtx, kernel.CommandKernel, "composite-shutdown-probe-"+test.suffix)
			require.NoError(t, err)

			parentEntered := make(chan struct{})
			submitChild := make(chan struct{})
			childApplied := make(chan struct{})
			parentDone := make(chan error, 1)
			parentID := "shutdown-composite-parent-" + test.suffix
			childID := "shutdown-composite-child-" + test.suffix
			go func() {
				parentDone <- kernel.SubmitPreparedAndWait(
					context.Background(),
					Request{
						UID:     parentID,
						LaneKey: parentID,
						Source:  lifecycle.SourceJobManager,
						Route:   "internal/test/" + parentID,
					},
					compositeParentTestPlan(
						parentID,
						[]string{"graph"},
						func(ctx context.Context, commands CompositeCommandScope) error {
							close(parentEntered)
							<-submitChild
							return test.submit(
								ctx,
								commands,
								Request{
									UID:     childID,
									LaneKey: childID,
									Source:  lifecycle.SourceJobManager,
									Route:   "internal/test/" + childID,
								},
								compositeTestPlan(
									childID,
									[]string{"graph"},
									func() {
										close(childApplied)
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
				require.FailNow(t, "test failed", "composite parent did not enter apply")
			}

			kernel.Stop()
			shutdownCtx, cancelShutdown := context.WithTimeout(context.Background(), time.Second)
			defer cancelShutdown()
			require.NoError(t, probe.waitCancellation(shutdownCtx))
			close(submitChild)

			select {
			case <-childApplied:
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "post-cut composite child did not apply")
			}
			select {
			case err := <-parentDone:
				require.NoError(t, err)
			case <-time.After(time.Second):
				require.FailNow(t, "test failed", "post-cut composite parent did not settle")
			}
			waitCtx, cancelWait := context.WithTimeout(context.Background(), time.Second)
			defer cancelWait()
			require.NoError(t, kernel.Wait(waitCtx))
			require.NoError(t, probe.waitSettlement(waitCtx))
			require.NoError(t, run.DirtyCause())
			if test.suffix == "rollback" {
				require.True(t, slices.ContainsFunc(diagnostics.snapshot(), func(event DiagnosticEvent) bool {
					return event.Name == "operation admitted" &&
						event.UID == childID &&
						event.Rollback &&
						event.Composite
				}))
			}
		})
	}
}

func TestCompositeChildRejectsActiveParentLane(t *testing.T) {
	kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	startKernelLoop(t, kernel)

	require.NoError(t, run.OpenAdmission())

	childErr := make(chan error, 1)
	parentPlan := compositeParentTestPlan(
		"parent",
		[]string{"graph"},
		func(ctx context.Context, commands CompositeCommandScope) error {
			childErr <- commands.SubmitPreparedAndWait(
				ctx,
				Request{
					UID:     "same-lane-child",
					LaneKey: "parent",
					Source:  lifecycle.SourceJobManager,
					Route:   "internal/test/same-lane-child",
				},
				compositeTestPlan("parent", []string{"graph"}, nil),
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
		require.False(t, err == nil || !strings.Contains(err.Error(), "active parent lane"))
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "same-lane child was not rejected promptly")
	}
	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "parent did not complete after same-lane rejection")
	}

	require.NoError(t, run.DirtyCause())

	stopCompositeTestKernel(t, kernel)
}

func TestCompositeChildContinuationPrecedesRunnableTargetLaneWork(t *testing.T) {
	kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	observer := &kernelRuntimeObserver{}
	require.NoError(t, kernel.BindRuntimeObserver(observer))
	startKernelLoop(t, kernel)

	require.NoError(t, run.OpenAdmission())

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
		require.FailNow(t, "test failed", "target-lane blocker did not start")
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
				func(ctx context.Context, commands CompositeCommandScope) error {
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
								applied = append(applied, "child")
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
		require.FailNow(t, "test failed", "FIFO parent did not enter apply")
	}

	normalDone := make(chan error, 1)
	go func() {
		normalDone <- kernel.SubmitPreparedAndWait(
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
		)
	}()
	close(submitChild)
	waitForAdmittedOperations(t, observer, 4)
	close(blockerRelease)

	select {
	case err := <-blockerDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "target-lane blocker did not finish")
	}
	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "FIFO parent did not finish")
	}
	select {
	case err := <-normalDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "ordinary target-lane operation did not finish")
	}
	mu.Lock()
	got := append([]string(nil), applied...)
	mu.Unlock()

	want := []string{"child", "normal"}
	require.Equal(t, want, got)

	require.NoError(t, run.DirtyCause())

	stopCompositeTestKernel(t, kernel)
}

func TestCompositeFenceAcceptsButDefersConflictingWorkWithoutBlockingUnrelatedWork(t *testing.T) {
	kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	startKernelLoop(t, kernel)

	require.NoError(t, run.OpenAdmission())

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
				func(ctx context.Context, commands CompositeCommandScope) error {
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
		require.FailNow(t, "test failed", "composite child did not start")
	}

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
	permit := lifecycle.NewSecretStoreLongLivedPlan()
	conflictingPlan.Transaction.AllocateSuccessor = true
	conflictingPlan.Transaction.Permit = permit
	conflictingPlan, err := prepareOwnedJobPlan(conflictingRequest, conflictingPlan)
	require.NoError(t, err)
	conflictingAdmitted := make(chan error, 1)
	conflictingDone := make(chan error, 1)

	require.NoError(t, kernel.enqueueSubmission(
		t.Context(),
		conflictingRequest.Source,
		submission{
			request:  conflictingRequest,
			plan:     conflictingPlan,
			context:  t.Context(),
			result:   conflictingAdmitted,
			terminal: conflictingDone,
		},
	),
	)

	select {
	case <-kernel.submissionSpace[sourceIndex(conflictingRequest.Source)]:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "conflicting submission was not dequeued")
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
		require.FailNow(t, "test failed", "unrelated work was starved by the conflicting operation")
	}
	select {
	case err := <-conflictingAdmitted:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "conflicting operation was not accepted")
	}
	select {
	case <-conflictingApplied:
		require.FailNow(t, "test failed", "conflicting operation ran before parent terminal")
	default:
	}
	select {
	case err := <-unrelatedDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "unrelated operation did not finish")
	}

	close(childRelease)
	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "composite parent did not finish")
	}
	select {
	case <-conflictingApplied:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "conflicting operation did not run after parent terminal")
	}
	select {
	case err := <-conflictingDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "conflicting operation did not reach terminal")
	}

	require.NoError(t, run.DirtyCause())

	stopCompositeTestKernel(t, kernel)
}

func TestCompositeChildRunsWhileParentOwnsFence(t *testing.T) {
	kernel, run, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	startKernelLoop(t, kernel)

	require.NoError(t, run.OpenAdmission())

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
				func(ctx context.Context, commands CompositeCommandScope) error {
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
		require.FailNow(t, "test failed", "progress parent did not enter apply")
	}

	close(submitChild)
	select {
	case <-childEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "composite child did not start while its parent owned the fence")
	}

	close(childRelease)
	select {
	case err := <-parentDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "progress parent did not finish")
	}
	require.NoError(t, run.DirtyCause())

	stopCompositeTestKernel(t, kernel)
}
