// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
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
	loop, err := NewKernelLoop(kernel)
	require.NoError(t, err)
	require.NoError(t, loop.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	sentinel := errors.New("transaction preparation failed")
	err = kernel.SubmitPreparedAndWait(
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

func TestSubmitPreparedAndWaitDirtiesRunForRetainedTransactionPreparation(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	loop, err := NewKernelLoop(kernel)
	require.NoError(t, err)
	require.NoError(t, loop.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())

	sentinel := errors.New("transaction preparation retained ownership")
	err = kernel.SubmitPreparedAndWait(
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
		trigger      func(*testing.T, *CommandKernel)
		stopAfterRun bool
	}{
		"user cancellation": {
			trigger: func(t *testing.T, kernel *CommandKernel) {
				t.Helper()
				require.NoError(t, kernel.Cancel(context.Background(), "atomic-action"))
			},
			stopAfterRun: true,
		},
		"shutdown": {
			trigger: func(_ *testing.T, kernel *CommandKernel) {
				kernel.Stop()
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
			loop, err := NewKernelLoop(kernel)
			require.NoError(t, err)
			require.NoError(t, loop.Start(t.Context()))
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

func TestKernelDisposesResourcePreparedAfterCancellation(t *testing.T) {
	kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
	loop, err := NewKernelLoop(kernel)
	require.NoError(t, err)
	require.NoError(t, loop.Start(t.Context()))
	require.NoError(t, run.OpenAdmission())
	defer func() {
		kernel.Stop()
		require.NoError(t, kernel.Wait(context.Background()))
	}()

	permitPlan, err := lifecycle.NewJobLongLivedPlan(40)
	require.NoError(t, err)
	preparing := make(chan struct{})
	release := make(chan struct{})
	accepted := make(chan struct{})
	disposed := make(chan struct{})
	resource := newKernelTestReadyResource(
		"cancelled-install",
		nil,
		nil,
	)
	returned := make(chan error, 1)
	go func() {
		returned <- kernel.SubmitPreparedAndWait(
			context.Background(),
			Request{
				UID:     "cancelled-install",
				LaneKey: "cancelled-install",
				Source:  lifecycle.SourceJobManager,
				Route:   "internal/test",
			},
			WorkPlan{
				NoResponse: true,
				Resource: &ResourcePlan{
					Action: ResourceInstall,
					ID:     "cancelled-install",
					Permit: permitPlan,
					Prepare: func(
						_ context.Context,
						generation uint64,
						permit lifecycle.LongLivedPermit,
					) (lifecycle.PreparedResource, error) {
						close(preparing)
						<-release
						return &cancellationObservedPreparedResource{
							identity: lifecycle.ResourceIdentity{
								ID:         "cancelled-install",
								Generation: generation,
							},
							permit:   permit,
							ready:    resource,
							accepted: accepted,
							disposed: disposed,
						}, nil
					},
				},
			},
		)
	}()
	<-preparing
	require.NoError(t, kernel.Cancel(
		context.Background(),
		"cancelled-install",
	))
	close(release)

	select {
	case <-disposed:
	case <-accepted:
		require.FailNow(
			t,
			"test failed",
			"cancelled resource installation was accepted",
		)
	case <-time.After(time.Second):
		require.FailNow(
			t,
			"test failed",
			"cancelled resource installation was not settled",
		)
	}
	require.NoError(t, <-returned)
}

type cancellationObservedPreparedResource struct {
	identity lifecycle.ResourceIdentity
	permit   lifecycle.LongLivedPermit
	ready    *kernelTestReadyResource
	accepted chan<- struct{}
	disposed chan<- struct{}
}

func (copr *cancellationObservedPreparedResource) Identity() lifecycle.ResourceIdentity {
	return copr.identity
}

func (copr *cancellationObservedPreparedResource) AcceptStart(
	_ context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	close(copr.accepted)
	if expected != copr.identity.Generation {
		return nil, errors.New(
			"cancelled resource generation differs",
		)
	}
	if err := copr.permit.ActivateExternal(
		lifecycle.LongLivedEJobResources,
	); err != nil {
		return nil, err
	}
	copr.ready.identity = copr.identity
	copr.ready.permit = copr.permit
	return copr.ready, nil
}

func (copr *cancellationObservedPreparedResource) Dispose(
	context.Context,
) error {
	close(copr.disposed)
	return copr.permit.AbortUnused()
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
	loop, err := NewKernelLoop(kernel)
	require.NoError(t, err)
	require.NoError(t, loop.Start(t.Context()))
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

func TestShutdownPreparationCancellationDoesNotDirtyResourceOrCapability(t *testing.T) {
	tests := map[string]struct {
		plan func(*testing.T, chan<- struct{}) WorkPlan
	}{
		"resource": {
			plan: func(t *testing.T, started chan<- struct{}) WorkPlan {
				t.Helper()
				permit, err := lifecycle.NewJobLongLivedPlan(40)
				require.NoError(t, err)
				return WorkPlan{
					NoResponse: true,
					Resource: &ResourcePlan{
						Action: ResourceInstall,
						ID:     "stopping-resource",
						Permit: permit,
						Prepare: func(
							ctx context.Context,
							_ uint64,
							_ lifecycle.LongLivedPermit,
						) (lifecycle.PreparedResource, error) {
							close(started)
							<-ctx.Done()
							return nil, ctx.Err()
						},
					},
				}
			},
		},
		"capability": {
			plan: func(t *testing.T, started chan<- struct{}) WorkPlan {
				t.Helper()
				permit, err := lifecycle.NewSecretStoreLongLivedPlan(40)
				require.NoError(t, err)
				return WorkPlan{
					NoResponse: true,
					Capability: &CapabilityPlan{
						ID:     "stopping-capability",
						Permit: permit,
						Prepare: func(
							ctx context.Context,
							_ uint64,
							_ lifecycle.LongLivedPermit,
						) (lifecycle.PreparedCapability, error) {
							close(started)
							<-ctx.Done()
							return nil, ctx.Err()
						},
					},
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			kernel, run, _, _, _ := newKernelWithPlanner(t, stoppedKernelPlanner{})
			loop, err := NewKernelLoop(kernel)
			require.NoError(t, err)
			require.NoError(t, loop.Start(t.Context()))
			require.NoError(t, run.OpenAdmission())
			started := make(chan struct{})
			returned := make(chan error, 1)
			plan := test.plan(t, started)
			go func() {
				returned <- kernel.SubmitPreparedAndWait(
					context.Background(),
					Request{
						UID:     "stopping-" + name,
						LaneKey: "stopping-" + name,
						Source:  lifecycle.SourceJobManager,
						Route:   "internal/test",
					},
					plan,
				)
			}()
			<-started

			kernel.Stop()
			stopping, ok := errors.AsType[*lifecycle.StoppingRejection](<-returned)
			require.True(t, ok)
			require.EqualValues(t, run.Generation(), stopping.Generation)
			require.NoError(t, kernel.Wait(context.Background()))
			require.NoError(t, run.DirtyCause())
		})
	}
}

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
