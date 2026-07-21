// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"io"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/require"
)

func TestSubmitPreparedAndWaitReturnsCleanTransactionPreparationError(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	sentinel := errors.New("transaction preparation failed")
	err := kernel.SubmitPreparedAndWait(
		context.Background(),
		Request{
			UID:     "prepared-transaction-failure",
			LaneKey: "prepared-transaction-failure",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test",
		},
		WorkPlan{
			NoResponse: true,
			Transaction: &ResourceTransactionPlan{
				ID: "prepared-transaction-failure",
				Prepare: func(
					context.Context,
					lifecycle.ReadyResource,
					lifecycle.ResourceTransactionScope,
					lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return nil, sentinel
				},
			},
		},
	)
	require.ErrorIs(t, err, sentinel)

	kernel.Stop()
	require.NoError(t, kernel.Wait(context.Background()))
}

func TestSubmitPreparedPreservesStoppingRejectionWithoutInputBodyCleanup(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	plan, err := stoppedKernelPlanner{}.Plan(Request{})
	require.NoError(t, err)

	kernel.Stop()
	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancelShutdown()
	require.NoError(t, kernel.WaitShutdownStarted(shutdownCtx))

	err = kernel.SubmitPrepared(
		context.Background(),
		Request{
			UID:     "stopping-boundary",
			LaneKey: "stopping-boundary",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test",
		},
		plan,
	)
	require.Same(t, run.StoppingCause(), err)
	require.NoError(t, kernel.Wait(context.Background()))
}

func TestSubmitPreparedAndWaitDirtiesRunForRetainedTransactionPreparation(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	sentinel := errors.New("transaction preparation retained ownership")
	err := kernel.SubmitPreparedAndWait(
		context.Background(),
		Request{
			UID:     "retained-transaction-failure",
			LaneKey: "retained-transaction-failure",
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/test",
		},
		WorkPlan{
			NoResponse: true,
			Transaction: &ResourceTransactionPlan{
				ID: "retained-transaction-failure",
				Prepare: func(
					context.Context,
					lifecycle.ReadyResource,
					lifecycle.ResourceTransactionScope,
					lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return nil, lifecycle.RetainOwnership(sentinel)
				},
			},
		},
	)
	require.ErrorIs(t, err, sentinel)
	require.ErrorIs(t, run.DirtyCause(), sentinel)

	kernel.Stop()
	require.ErrorIs(t, kernel.Wait(context.Background()), sentinel)
}

func TestKernelDoesNotCancelStartedTransactionAction(t *testing.T) {
	tests := map[string]struct {
		trigger      func(*testing.T, *testCommandKernel)
		stopAfterRun bool
	}{
		"user cancellation": {
			trigger: func(t *testing.T, kernel *testCommandKernel) {
				t.Helper()
				require.NoError(t, kernel.Cancel(context.Background(), "atomic-action"))
			},
			stopAfterRun: true,
		},
		"shutdown": {
			trigger: func(_ *testing.T, kernel *testCommandKernel) {
				kernel.Stop()
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
			require.NoError(t, kernel.Start(t.Context()))
			require.NoError(t, run.OpenAdmission())
			entered := make(chan struct{})
			release := make(chan struct{})
			actionDone := make(chan error, 1)
			submitted := make(chan error, 1)
			go func() {
				submitted <- kernel.SubmitPreparedAndWait(
					context.Background(),
					Request{
						UID:     "atomic-action",
						LaneKey: "atomic-action",
						Source:  lifecycle.SourceJobManager,
						Route:   "internal/test",
					},
					atomicTransactionPlan(entered, release, actionDone),
				)
			}()
			<-entered

			test.trigger(t, kernel)
			select {
			case err := <-actionDone:
				require.FailNowf(t, "started action was cancelled", "error: %v", err)
			case <-time.After(50 * time.Millisecond):
			}
			close(release)
			require.NoError(t, <-actionDone)
			if test.stopAfterRun {
				require.NoError(t, <-submitted)
				kernel.Stop()
			}
			require.NoError(t, kernel.Wait(context.Background()))
			require.NoError(t, run.DirtyCause())
		})
	}
}

func TestKernelShutdownBudgetBoundsProtectedOwnershipAction(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlannerAndTimeout(
		t,
		stoppedKernelPlanner{},
		20*time.Millisecond,
	)
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	entered := make(chan struct{})
	release := make(chan struct{})
	actionDone := make(chan error, 1)
	submitted := make(chan error, 1)
	go func() {
		submitted <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "atomic-action",
				LaneKey: "atomic-action",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			atomicTransactionPlan(entered, release, actionDone),
		)
	}()
	<-entered

	kernel.Stop()
	waitErr := kernel.Wait(context.Background())
	require.ErrorContains(t, waitErr, "shutdown deadline exceeded")
	terminal := run.TerminalState()
	require.True(t, terminal.Reached)
	require.False(t, terminal.Quiescent)
	require.Error(t, terminal.Dirty)

	close(release)
	select {
	case err := <-actionDone:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"blocked ownership action did not exit after release",
		)
	}
	select {
	case err := <-submitted:
		require.ErrorContains(t, err, "shutdown deadline exceeded")
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"blocked ownership submission did not observe shutdown expiry",
		)
	}
}

func TestKernelShutdownAllowsProtectedFunctionMutationHandoff(t *testing.T) {
	catalog := &shutdownActionMutationCatalog{}
	kernel, run, admission, uids, _ :=
		newKernelWithClockFinalizerCatalogAndTimeout(
			t,
			stoppedKernelPlanner{},
			catalog,
			io.Discard,
			lifecycle.RealClock{},
			newNoopRunFinalizer(),
			3*time.Second,
		)
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	actionEntered := make(chan struct{})
	actionRelease := make(chan struct{})
	submitted := make(chan error, 1)
	go func() {
		submitted <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "shutdown-action-mutation",
				LaneKey: "shutdown-action-mutation",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			WorkPlan{
				NoResponse: true,
				Transaction: &ResourceTransactionPlan{
					ID: "shutdown-action-mutation",
					Prepare: func(
						context.Context,
						lifecycle.ReadyResource,
						lifecycle.ResourceTransactionScope,
						lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResourceTransaction, error) {
						return &shutdownActionMutationTransaction{
							kernel:  kernel,
							entered: actionEntered,
							release: actionRelease,
							scope: lifecycle.ResourceTransactionScope{
								ID: "shutdown-action-mutation",
							},
						}, nil
					},
				},
			},
		)
	}()
	select {
	case <-actionEntered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "transaction action did not enter")
	}

	kernel.Stop()
	shutdownCtx, cancelShutdown := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancelShutdown()
	require.NoError(t, kernel.WaitShutdownStarted(shutdownCtx))
	close(actionRelease)

	select {
	case err := <-submitted:
		require.NoError(t, err)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "transaction mutation did not settle")
	}
	waitCtx, cancelWait := context.WithTimeout(
		context.Background(),
		5*time.Second,
	)
	defer cancelWait()
	waitErr := kernel.Wait(waitCtx)
	require.NoErrorf(
		t,
		waitErr,
		"phase=%d continuations=%d cancellationDone=%v inputReady=%v mutationActive=%v mutationPaused=%v catalog active=%v closed=%v",
		kernel.shutdownPhase,
		kernel.ownershipChains,
		kernel.shutdownCancelDone,
		kernel.shutdownInputBodyReady,
		kernel.functionMutationActive,
		kernel.functionMutationPaused,
		catalog.active.Load(),
		catalog.closed.Load(),
	)
	require.NoError(t, run.DirtyCause())
	require.EqualValues(t, 1, catalog.beginCalls.Load())
	require.EqualValues(t, 1, catalog.commitCalls.Load())
	require.True(t, catalog.closed.Load())
	require.NoError(t, admission.CloseDrained(run.Generation()))
	closeUIDLedger(t, uids)
}

type shutdownActionMutation struct{}

func (shutdownActionMutation) FunctionCatalogMutation() {}

type shutdownActionMutationTransaction struct {
	kernel  *testCommandKernel
	entered chan<- struct{}
	release <-chan struct{}
	scope   lifecycle.ResourceTransactionScope
}

func (samt *shutdownActionMutationTransaction) Scope() lifecycle.ResourceTransactionScope {
	return samt.scope
}

func (samt *shutdownActionMutationTransaction) Apply(
	context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	close(samt.entered)
	<-samt.release
	mutation := shutdownActionMutation{}
	if err := samt.kernel.QuiesceFunctions(
		context.Background(),
		mutation,
	); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if _, err := samt.kernel.CommitFunctions(
		context.Background(),
		mutation,
	); err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	result, err := lifecycle.NewSealedResult(
		204,
		"text/plain",
		nil,
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		samt.scope,
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		func() error { return nil },
	)
}

func (*shutdownActionMutationTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	return nil, nil
}

type shutdownActionMutationCatalog struct {
	active      atomic.Bool
	closed      atomic.Bool
	beginCalls  atomic.Int32
	commitCalls atomic.Int32
}

func (*shutdownActionMutationCatalog) ResolveAndAcquire(
	FunctionLookup,
) (FunctionCatalogDecision, error) {
	return FunctionCatalogDecision{}, nil
}

func (*shutdownActionMutationCatalog) ReleaseInvocation(
	FunctionInvocationRef,
) (FunctionCleanupPlan, error) {
	return FunctionCleanupPlan{}, nil
}

func (*shutdownActionMutationCatalog) CompleteCleanup(
	FunctionCleanupRef,
) error {
	return nil
}

func (samc *shutdownActionMutationCatalog) BeginMutation(
	FunctionCatalogMutation,
) error {
	if !samc.active.CompareAndSwap(false, true) {
		return errors.New("test Function mutation began twice")
	}
	samc.beginCalls.Add(1)
	return nil
}

func (samc *shutdownActionMutationCatalog) AdvanceMutationQuiesce(
	int,
) (FunctionCatalogMutationProgress, error) {
	if !samc.active.Load() {
		return FunctionCatalogMutationProgress{},
			errors.New("test Function mutation is inactive")
	}
	return FunctionCatalogMutationProgress{
		Version:  1,
		Quiesced: true,
	}, nil
}

func (samc *shutdownActionMutationCatalog) ResumeMutation(
	FunctionCatalogMutation,
) error {
	if !samc.active.Load() {
		return errors.New("test Function mutation is inactive")
	}
	return nil
}

func (samc *shutdownActionMutationCatalog) AdvanceMutation(
	int,
	*[MaximumFunctionCleanupBatch]FunctionCleanupPlan,
) (FunctionCatalogMutationProgress, int, error) {
	if !samc.active.CompareAndSwap(true, false) {
		return FunctionCatalogMutationProgress{}, 0,
			errors.New("test Function mutation is inactive")
	}
	samc.commitCalls.Add(1)
	return FunctionCatalogMutationProgress{
		Version: 2,
		Done:    true,
	}, 0, nil
}

func (samc *shutdownActionMutationCatalog) AbortMutation(
	*[MaximumFunctionCleanupBatch]FunctionCleanupPlan,
) (int, error) {
	samc.active.Store(false)
	return 0, nil
}

func (*shutdownActionMutationCatalog) BeginClose() error {
	return nil
}

func (samc *shutdownActionMutationCatalog) CloseStep(
	int,
	*[MaximumFunctionCleanupBatch]FunctionCleanupPlan,
) (int, bool, error) {
	samc.closed.Store(true)
	return 0, false, nil
}

func (samc *shutdownActionMutationCatalog) LifecycleCensus() FunctionCatalogCensus {
	return FunctionCatalogCensus{
		Closed:         samc.closed.Load(),
		MutationActive: samc.active.Load(),
	}
}

func atomicTransactionPlan(
	entered chan<- struct{},
	release <-chan struct{},
	actionDone chan<- error,
) WorkPlan {
	return WorkPlan{
		NoResponse: true,
		Transaction: &ResourceTransactionPlan{
			ID: "atomic-action",
			Prepare: func(
				context.Context,
				lifecycle.ReadyResource,
				lifecycle.ResourceTransactionScope,
				lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return &atomicPreparedTransaction{
					scope: lifecycle.ResourceTransactionScope{
						ID: "atomic-action",
					},
					entered: entered,
					release: release,
					done:    actionDone,
				}, nil
			},
		},
	}
}

type atomicPreparedTransaction struct {
	scope   lifecycle.ResourceTransactionScope
	entered chan<- struct{}
	release <-chan struct{}
	done    chan<- error
}

func (apt *atomicPreparedTransaction) Scope() lifecycle.ResourceTransactionScope {
	return apt.scope
}

func (apt *atomicPreparedTransaction) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	close(apt.entered)
	select {
	case <-ctx.Done():
		apt.done <- ctx.Err()
		return lifecycle.AppliedResourceTransaction{}, ctx.Err()
	case <-apt.release:
	}
	apt.done <- nil
	result, err := lifecycle.NewSealedResult(
		200,
		"application/json",
		[]byte(`{"accepted":true}`),
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	return lifecycle.NewAppliedResourceTransaction(
		apt.scope,
		lifecycle.ResourceTransactionUnchanged,
		nil,
		result,
		func() error { return nil },
	)
}

func (*atomicPreparedTransaction) Dispose(
	context.Context,
) (lifecycle.ReadyResource, error) {
	return nil, nil
}

func TestShutdownCancellationPreservesGenerationStoppingCause(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	started := make(chan struct{})
	returned := make(chan error, 1)
	go func() {
		returned <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "shutdown-preparation",
				LaneKey: "shutdown-preparation",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			WorkPlan{
				NoResponse: true,
				Transaction: &ResourceTransactionPlan{
					ID: "shutdown-preparation",
					Prepare: func(
						ctx context.Context,
						_ lifecycle.ReadyResource,
						_ lifecycle.ResourceTransactionScope,
						_ lifecycle.LongLivedPermit,
					) (
						lifecycle.PreparedResourceTransaction,
						error,
					) {
						close(started)
						<-ctx.Done()
						return nil, ctx.Err()
					},
				},
			},
		)
	}()
	select {
	case <-started:
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"prepared transaction did not start",
		)
	}

	kernel.Stop()
	select {
	case err := <-returned:
		stopping, ok :=
			errors.AsType[*lifecycle.StoppingRejection](err)
		require.True(t, ok)
		require.EqualValues(
			t,
			run.Generation(),
			stopping.Generation,
		)
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"shutdown cancellation did not complete",
		)
	}
	require.NoError(t, kernel.Wait(context.Background()))
	state := run.TerminalState()
	require.True(t, state.Reached)
	require.True(t, state.Quiescent)
	require.NoError(t, state.Dirty)
}

func TestShutdownPreparationCancellationDoesNotDirtyResource(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	require.NoError(t, kernel.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())
	started := make(chan struct{})
	observed := make(chan error, 1)
	returned := make(chan error, 1)
	go func() {
		returned <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "stopping-transaction",
				LaneKey: "stopping-transaction",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			WorkPlan{
				NoResponse: true,
				Transaction: &ResourceTransactionPlan{
					ID: "stopping-transaction",
					Prepare: func(
						ctx context.Context,
						_ lifecycle.ReadyResource,
						_ lifecycle.ResourceTransactionScope,
						_ lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResourceTransaction, error) {
						close(started)
						<-ctx.Done()
						observed <- context.Cause(ctx)
						return nil, ctx.Err()
					},
				},
			},
		)
	}()
	<-started

	kernel.Stop()
	cause := <-observed
	returnedErr := <-returned
	require.Same(t, run.StoppingCause(), cause)
	stopping, ok := errors.AsType[*lifecycle.StoppingRejection](returnedErr)
	require.Truef(t, ok, "terminal error is not a stopping rejection: %v", returnedErr)
	require.EqualValues(t, run.Generation(), stopping.Generation)
	require.NoError(t, kernel.Wait(context.Background()))
	require.NoError(t, run.DirtyCause())
}

func TestSubmitPreparedAndWaitJoinsAcceptedCancellation(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(
		t,
		stoppedKernelPlanner{},
	)
	require.NoError(t, kernel.Start(t.Context()))

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
