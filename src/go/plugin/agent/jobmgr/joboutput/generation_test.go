// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/runtimecomp"
	"github.com/stretchr/testify/require"
)

var _ lifecycle.PreparedResource = PreparedJob{}
var _ lifecycle.ReadyResource = (*JobGeneration)(nil)

func TestPreparedTransactionRejectsStaleUnusedPermitAtConstruction(t *testing.T) {
	permit, _ := issueTestJobPermit(t, "job", 1)
	require.NoError(t, permit.AbortUnused())
	result, err := lifecycle.NewSealedResult(204, "application/json", nil)
	require.NoError(t, err)

	_, err = PrepareNoopResourceTransaction(
		lifecycle.ResourceTransactionScope{
			ID: "job",
			Successor: lifecycle.ResourceIdentity{
				ID:         "job",
				Generation: 1,
			},
		},
		nil,
		permit,
		result,
		func() error { return nil },
		nil,
	)
	require.Error(t, err)
}

func TestJobLifecyclePanicsPreserveTaskClassification(t *testing.T) {
	err := callJobLifecycle("test", func() error {
		panic("lifecycle")
	})
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
}

func TestPreparedJobAcceptanceTransfersFinalCleanupToProcessOwner(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	var rejected, final int
	identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 1}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { rejected++; return nil },
		finalCleanup:     func(context.Context) error { final++; return nil },
		outputGate:       gate,
	}
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       identity.ID,
			Resource:  identity.ID,
		},
	)
	candidate.attach = func(
		lifecycle.ResourceIdentity,
		*stagedJobOwner,
	) (constructedJobAttachment, error) {
		require.NoError(t, owner.BindAttachment())
		return constructedJobAttachment{resources: candidate, transferred: true}, nil
	}
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	prepared := PreparedJob{state: &preparedJobState{
		id:          identity.ID,
		generation:  identity.Generation,
		constructed: candidate,
		permit:      permit,
		owner:       owner,
	}}

	generation, err := prepared.Accept(context.Background(), identity.Generation)
	require.NoError(t, err)
	require.NotNil(t, generation)
	owner.Reject()
	select {
	case <-owner.done:
	case <-time.After(time.Second):
		t.Fatal("accepted process owner did not finalize")
	}
	require.NoError(t, permit.ReleaseExternal())
	require.NoError(t, permit.Return())

	require.Zero(t, rejected)
	require.EqualValues(t, 1, final)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestStagedJobRetirementBeforeResourceAcceptanceLeavesPermitUnused(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 1}
	var output bytes.Buffer
	frames, err := lifecycle.NewFrameOwner(&output)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	var cleanups int
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { cleanups++; return nil },
		finalCleanup:     func(context.Context) error { return nil },
		outputGate:       gate,
	}
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       identity.ID,
			Resource:  identity.ID,
		},
	)
	require.NoError(t, owner.Promote(context.Background()))
	require.NoError(t, owner.BindAttachment())
	require.NoError(t, owner.AdoptAttachment(candidate))
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	require.EqualValues(t, 1, attempts.CutTarget(1))
	requireTestSignal(t, owner.retire, "runtime owner did not record retirement")

	_, err = owner.AcceptResources(permit)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptRetired)
	require.True(t, jobmgr.ContainsOnlyErrorLeaves(
		err,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	))
	require.NoError(t, permit.AbortUnused())
	owner.Reject()
	requireTestSignal(t, owner.done, "retired runtime owner did not finalize")
	_, writeErr := gate.Write([]byte("late\n"))
	require.ErrorIs(t, writeErr, errGenerationOutputFenced)
	require.Empty(t, output.Bytes())
	require.EqualValues(t, 1, cleanups)

	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestStagedJobAcceptanceRejectsInvalidOutputGate(t *testing.T) {
	tests := map[string]struct {
		gate func(*testing.T) *generationOutputGate
		want string
	}{
		"missing": {
			gate: func(*testing.T) *generationOutputGate { return nil },
			want: "job output: nil generation output gate",
		},
		"already active": {
			gate: func(t *testing.T) *generationOutputGate {
				frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
				require.NoError(t, err)
				gate, err := newGenerationOutputGate(frames)
				require.NoError(t, err)
				require.NoError(t, gate.Activate())
				return gate
			},
			want: "job output: invalid generation output activation",
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			candidate := ConstructedJob{
				Variant:          JobVariantV1,
				CollectorCleanup: func(context.Context) error { return nil },
				outputGate:       test.gate(t),
			}
			owner := newStagedJobOwner(
				context.Background(),
				candidate,
				attempts,
				1,
				jobmgr.ProcessAttemptIdentity{
					Namespace: jobmgr.ProcessAttemptJobRuntime,
					Key:       "module_job",
					Resource:  "module_job",
				},
			)
			require.NoError(t, owner.Promote(context.Background()))
			require.NoError(t, owner.BindAttachment())
			require.NoError(t, owner.AdoptAttachment(candidate))
			permit, tasks := issueTestJobPermit(t, "module_job", 1)

			_, err = owner.AcceptResources(permit)

			require.EqualError(t, err, test.want)
			require.False(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			require.NoError(t, permit.AbortUnused())
			owner.Reject()
			requireTestSignal(t, owner.done, "rejected runtime owner did not finalize")
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
			attempts.BeginShutdown()
			require.NoError(t, attempts.Shutdown(context.Background()))
		})
	}
}

func TestRuntimeContainmentRevokesOutputWithoutWaitingForDrain(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	writeEntered := make(chan struct{})
	releaseWrite := make(chan struct{})
	var releaseOnce sync.Once
	release := func() {
		releaseOnce.Do(func() { close(releaseWrite) })
	}
	defer release()
	frames, err := lifecycle.NewFrameOwner(frameWriteFunc(func(payload []byte) (int, error) {
		close(writeEntered)
		<-releaseWrite
		return len(payload), nil
	}))
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	var rejected, final int
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { rejected++; return nil },
		finalCleanup:     func(context.Context) error { final++; return nil },
		outputGate:       gate,
	}
	identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 1}
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       identity.ID,
			Resource:  identity.ID,
		},
	)
	require.NoError(t, owner.Promote(context.Background()))
	require.NoError(t, owner.BindAttachment())
	require.NoError(t, owner.AdoptAttachment(candidate))
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	_, err = owner.AcceptResources(permit)
	require.NoError(t, err)

	writeDone := make(chan error, 1)
	go func() {
		_, err := gate.Write([]byte("admitted\n"))
		writeDone <- err
	}()
	requireTestSignal(t, writeEntered, "initial output did not acquire its write lease")
	cutDone := make(chan int, 1)
	go func() {
		cutDone <- attempts.CutTarget(1)
	}()
	select {
	case count := <-cutDone:
		require.EqualValues(t, 1, count)
	case <-time.After(time.Second):
		release()
		t.Fatal("containment waited for the admitted output lease to drain")
	}
	lateWriteDone := make(chan error, 1)
	go func() {
		_, err := gate.Write([]byte("late\n"))
		lateWriteDone <- err
	}()
	select {
	case err = <-lateWriteDone:
		require.ErrorIs(t, err, errGenerationOutputFenced)
	case <-time.After(time.Second):
		release()
		t.Fatal("late output waited for the admitted output lease to drain")
	}

	release()
	require.NoError(t, <-writeDone)
	owner.Reject()
	requireTestSignal(t, owner.done, "contained runtime owner did not finalize after output drain")
	require.NoError(t, permit.ReleaseExternal())
	require.NoError(t, permit.Return())
	require.Zero(t, rejected)
	require.EqualValues(t, 1, final)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	attempts.BeginShutdown()
	require.NoError(t, attempts.Shutdown(context.Background()))
	require.EqualValues(t, containment.Census{}, attempts.Census())
}

func TestStagedJobRetirementPreservesFirstCause(t *testing.T) {
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	owner := newStagedJobOwner(
		context.Background(),
		ConstructedJob{outputGate: gate},
		nil,
		1,
		jobmgr.ProcessAttemptIdentity{},
	)
	structural := errors.New("runtime failed before readiness")
	owner.beginRetirement(structural)
	owner.containRuntimeAttempt(jobmgr.ErrProcessAttemptRetired)

	err = owner.retirementError("fallback")
	require.ErrorIs(t, err, structural)
	require.NotErrorIs(t, err, jobmgr.ErrProcessAttemptRetired)
	require.False(t, jobmgr.ContainsOnlyErrorLeaves(
		err,
		jobmgr.ErrProcessAttemptRetired,
		jobmgr.ErrProcessAttemptStopped,
	))
}

func TestPreparedJobPreActivationRetirementPreservesCause(t *testing.T) {
	tests := map[string]struct {
		variant JobVariant
		cause   error
		preBind bool
	}{
		"V1 retired before bind": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptRetired,
			preBind: true,
		},
		"V1 stopped before bind": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptStopped,
			preBind: true,
		},
		"V2 retired before bind": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptRetired,
			preBind: true,
		},
		"V2 stopped before bind": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptStopped,
			preBind: true,
		},
		"V1 retired after transfer": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptRetired,
		},
		"V1 stopped after transfer": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptStopped,
		},
		"V2 retired after transfer": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptRetired,
		},
		"V2 stopped after transfer": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptStopped,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newPreActivationRetirementFixture(t, test.variant, test.cause, test.preBind)

			_, err := fixture.prepared.Accept(context.Background(), fixture.identity.Generation)

			require.ErrorIs(t, err, test.cause)
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			fixture.requireSettled(t)
		})
	}
}

func TestPreparedJobRetirementAfterAttachmentAdoptionPreservesCause(t *testing.T) {
	tests := map[string]error{
		"retired": jobmgr.ErrProcessAttemptRetired,
		"stopped": jobmgr.ErrProcessAttemptStopped,
	}
	for name, cause := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newPreActivationRetirementFixtureForWindow(
				t,
				JobVariantV2,
				cause,
				retirementAfterAdoption,
				nil,
			)

			_, err := fixture.prepared.Accept(context.Background(), fixture.identity.Generation)

			require.ErrorIs(t, err, cause)
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			fixture.requireSettled(t)
		})
	}
}

func TestPreparedJobRejectsTargetCutBeforeWorkerObservesCancellation(t *testing.T) {
	tests := map[string]error{
		"retired": jobmgr.ErrProcessAttemptRetired,
		"stopped": jobmgr.ErrProcessAttemptStopped,
	}
	for name, cause := range tests {
		t.Run(name, func(t *testing.T) {
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			delayed := &delayedRuntimeCancellationAuthority{
				ProcessAttemptAuthority: attempts,
				cutObserved:             make(chan struct{}),
				release:                 make(chan struct{}),
			}
			defer delayed.releaseCancellation()
			fixture := newPreparedAttachmentFixture(
				t,
				JobVariantV2,
				delayed,
				true,
				jobHandlerAttacherFunc(func(
					_ lifecycle.ResourceIdentity,
					staged StagedHandlerLifecycle,
				) (ProcessHandlerLifecycle, error) {
					cutTestAuthority(t, attempts, cause)
					requireTestSignal(t, delayed.cutObserved, "authority cut did not cancel the runtime attempt")
					return staged.(ProcessHandlerLifecycle), nil
				}),
			)

			generation, err := fixture.prepared.Accept(context.Background(), fixture.identity.Generation)
			if generation != nil {
				require.NoError(t, generation.abortProcessOwned(context.Background()))
			}
			delayed.releaseCancellation()

			require.ErrorIs(t, err, cause)
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			requireTestSignal(t, fixture.owner.done, "retired runtime owner did not finalize")
			require.EqualValues(t, 1, fixture.state.rejected)
			require.Zero(t, fixture.state.final)
			require.EqualValues(t, 1, fixture.state.handlerFinalized)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, fixture.tasks.LongLivedCensus())
			require.NoError(t, attempts.Shutdown(context.Background()))
			require.EqualValues(t, containment.Census{}, attempts.Census())
		})
	}
}

func TestProcessOwnedJobDoesNotStartAfterContainment(t *testing.T) {
	tests := map[string]struct {
		variant JobVariant
		cause   error
	}{
		"V1 retired": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptRetired,
		},
		"V1 stopped": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptStopped,
		},
		"V2 retired": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptRetired,
		},
		"V2 stopped": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptStopped,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			fixture := newPreparedAttachmentFixture(t, test.variant, attempts, false, nil)
			generation, err := fixture.prepared.Accept(context.Background(), fixture.identity.Generation)
			require.NoError(t, err)
			require.NotNil(t, generation)

			cutTestAuthority(t, attempts, test.cause)
			_, writeErr := fixture.gate.Write([]byte("late\n"))
			require.ErrorIs(t, writeErr, errGenerationOutputFenced)
			err = generation.Start(context.Background())

			require.ErrorIs(t, err, test.cause)
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			select {
			case <-fixture.job.started:
				t.Fatal("managed job started after containment")
			default:
			}
			requireTestSignal(t, fixture.owner.done, "contained runtime owner did not finalize")
			require.Zero(t, fixture.state.rejected)
			require.EqualValues(t, 1, fixture.state.final)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, fixture.tasks.LongLivedCensus())
			require.NoError(t, attempts.Shutdown(context.Background()))
			require.EqualValues(t, containment.Census{}, attempts.Census())
		})
	}
}

func TestProcessOwnedJobRejectsContainmentBetweenStartReservationAndWorkerClaim(t *testing.T) {
	tests := map[string]error{
		"retired": jobmgr.ErrProcessAttemptRetired,
		"stopped": jobmgr.ErrProcessAttemptStopped,
	}
	for name, cause := range tests {
		t.Run(name, func(t *testing.T) {
			attempts, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			fixture := newPreparedAttachmentFixture(t, JobVariantV2, attempts, false, nil)
			generation, err := fixture.prepared.Accept(context.Background(), fixture.identity.Generation)
			require.NoError(t, err)
			require.NotNil(t, generation)
			require.NoError(t, fixture.owner.reserveStart())

			cutTestAuthority(t, attempts, cause)
			_, retiring, err := fixture.owner.beginManagedStart()

			require.True(t, retiring)
			require.ErrorIs(t, err, cause)
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			select {
			case <-fixture.job.started:
				t.Fatal("managed job started after containment won the reserved start")
			default:
			}
			require.NoError(t, generation.abortProcessOwned(context.Background()))
			requireTestSignal(t, fixture.owner.done, "contained runtime owner did not finalize")
			require.Zero(t, fixture.state.rejected)
			require.EqualValues(t, 1, fixture.state.final)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, fixture.tasks.LongLivedCensus())
			require.NoError(t, attempts.Shutdown(context.Background()))
			require.EqualValues(t, containment.Census{}, attempts.Census())
		})
	}
}

func TestPreparedJobAdoptsPartialHandlerTransferBeforeAttachmentFailure(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attachErr := errors.New("partial handler attachment failed")
	fixture := newPreparedAttachmentFixture(
		t,
		JobVariantV1,
		attempts,
		true,
		jobHandlerAttacherFunc(func(
			_ lifecycle.ResourceIdentity,
			staged StagedHandlerLifecycle,
		) (ProcessHandlerLifecycle, error) {
			return staged.(ProcessHandlerLifecycle), attachErr
		}),
	)

	generation, err := fixture.prepared.Accept(context.Background(), fixture.identity.Generation)

	require.Nil(t, generation)
	require.ErrorIs(t, err, attachErr)
	requireTestSignal(t, fixture.owner.done, "partially attached runtime owner did not finalize")
	require.EqualValues(t, 1, fixture.state.rejected)
	require.Zero(t, fixture.state.final)
	require.EqualValues(t, 1, fixture.state.handlerFinalized)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, fixture.tasks.LongLivedCensus())
	attempts.BeginShutdown()
	require.NoError(t, attempts.Shutdown(context.Background()))
	require.EqualValues(t, containment.Census{}, attempts.Census())
}

func TestPreparedJobDoesNotHideProjectionFailureDuringRetirement(t *testing.T) {
	projectionErr := errors.New("projection failed")
	tests := map[string]error{
		"retired": jobmgr.ErrProcessAttemptRetired,
		"stopped": jobmgr.ErrProcessAttemptStopped,
	}
	for name, cause := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newPreActivationRetirementFixtureWithRuntimeError(
				t,
				JobVariantV2,
				cause,
				false,
				errors.Join(cause, projectionErr),
			)
			result, err := lifecycle.NewSealedResult(500, "application/json", nil)
			require.NoError(t, err)
			transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
				Scope: lifecycle.ResourceTransactionScope{
					ID:        fixture.identity.ID,
					Successor: fixture.identity,
				},
				Disposition: lifecycle.ResourceTransactionInstalled,
				Successor:   fixture.prepared,
				Result:      result,
				Cleanup:     func() error { return nil },
			})
			require.NoError(t, err)

			applied, err := transaction.Apply(context.Background())

			require.ErrorIs(t, err, cause)
			require.ErrorIs(t, err, projectionErr)
			require.False(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			_, disposition, current := applied.Ownership()
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Nil(t, current)
			fixture.requireSettled(t)
		})
	}
}

func TestPreparedResourceTransactionSettlesPreActivationRetirement(t *testing.T) {
	tests := map[string]struct {
		variant JobVariant
		cause   error
		preBind bool
	}{
		"V1 retired before bind": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptRetired,
			preBind: true,
		},
		"V2 stopped before bind": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptStopped,
			preBind: true,
		},
		"V1 stopped after transfer": {
			variant: JobVariantV1,
			cause:   jobmgr.ErrProcessAttemptStopped,
		},
		"V2 retired after transfer": {
			variant: JobVariantV2,
			cause:   jobmgr.ErrProcessAttemptRetired,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			fixture := newPreActivationRetirementFixture(t, test.variant, test.cause, test.preBind)
			graph, err := dyncfg.NewGraph(nil)
			require.NoError(t, err)
			postimage := dyncfg.GraphConfig{
				ID:     fixture.identity.ID,
				Module: "module",
				Name:   "job",
			}
			mutation, err := graph.PrepareMutation([]dyncfg.GraphChange{{
				ID:     fixture.identity.ID,
				Config: &postimage,
			}})
			require.NoError(t, err)
			result, err := lifecycle.NewSealedResult(200, "application/json", nil)
			require.NoError(t, err)
			scope := lifecycle.ResourceTransactionScope{
				ID:        fixture.identity.ID,
				Successor: fixture.identity,
			}
			transaction, err := PrepareResourceTransaction(ResourceTransactionSpec{
				Scope:            scope,
				Disposition:      lifecycle.ResourceTransactionInstalled,
				Successor:        fixture.prepared,
				Graph:            graph,
				Mutation:         mutation,
				MutationPrepared: true,
				Result:           result,
				Cleanup:          func() error { return nil },
			})
			require.NoError(t, err)

			applied, err := transaction.Apply(context.Background())

			require.NoError(t, err)
			gotScope, disposition, current := applied.Ownership()
			require.Equal(t, scope, gotScope)
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Nil(t, current)
			_, exists := graph.Lookup(fixture.identity.ID)
			require.False(t, exists)
			fixture.requireSettled(t)
		})
	}
}

func TestCandidateCancellationBeforePromotionPreservesRetirementCause(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	outputGate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	var cleanups int
	ctx, cancel := context.WithCancelCause(context.Background())
	owner := newStagedJobOwner(
		ctx,
		ConstructedJob{
			Variant:          JobVariantV1,
			CollectorCleanup: func(context.Context) error { cleanups++; return nil },
			outputGate:       outputGate,
		},
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       "module_job",
			Resource:  "module_job",
		},
	)
	finished := make(chan error, 1)
	go func() {
		finished <- owner.finishCandidate(ctx)
	}()

	cancel(jobmgr.ErrProcessAttemptStopped)
	select {
	case <-owner.rejectCandidate:
	case <-time.After(time.Second):
		t.Fatal("candidate cancellation did not reject its ownership")
	}
	require.ErrorIs(t, owner.Promote(context.Background()), jobmgr.ErrProcessAttemptStopped)
	require.NoError(t, <-finished)
	require.EqualValues(t, 1, cleanups)

	attempts.BeginShutdown()
	require.NoError(t, attempts.Shutdown(context.Background()))
}

func TestCandidateCancellationDoesNotRejectPromotionInProgress(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := &promotionGateAuthority{
		ProcessAttemptAuthority: delegate,
		entered:                 make(chan struct{}),
		release:                 make(chan struct{}),
	}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	outputGate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	var cleanups int
	candidateCtx, cancelCandidate := context.WithCancelCause(context.Background())
	owner := newStagedJobOwner(
		candidateCtx,
		ConstructedJob{
			Variant:          JobVariantV1,
			CollectorCleanup: func(context.Context) error { cleanups++; return nil },
			outputGate:       outputGate,
		},
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       "module_job",
			Resource:  "module_job",
		},
	)
	finished := make(chan error, 1)
	go func() {
		finished <- owner.finishCandidate(candidateCtx)
	}()
	promoted := make(chan error, 1)
	go func() {
		promoted <- owner.Promote(context.Background())
	}()
	select {
	case <-attempts.entered:
	case <-time.After(time.Second):
		t.Fatal("promotion did not reach runtime-attempt admission")
	}

	cancelCandidate(jobmgr.ErrProcessAttemptStopped)
	close(attempts.release)
	require.NoError(t, <-promoted)
	require.NoError(t, <-finished)
	owner.mu.Lock()
	ownership, retiring, decided := owner.ownership, owner.retiring, owner.decided
	owner.mu.Unlock()
	require.Equal(t, stagedJobOwnedByRuntime, ownership)
	require.False(t, retiring)
	require.False(t, decided)

	owner.Reject()
	select {
	case <-owner.done:
	case <-time.After(time.Second):
		t.Fatal("promoted runtime did not retire")
	}
	require.EqualValues(t, 1, cleanups)
	delegate.BeginShutdown()
	require.NoError(t, delegate.Shutdown(context.Background()))
}

func TestCandidateCancellationControlsFailedPromotion(t *testing.T) {
	delegate, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	attempts := &busyPromotionGateAuthority{
		ProcessAttemptAuthority: delegate,
		entered:                 make(chan struct{}),
		release:                 make(chan struct{}),
	}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	outputGate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	var cleanups int
	candidateCtx, cancelCandidate := context.WithCancelCause(context.Background())
	owner := newStagedJobOwner(
		candidateCtx,
		ConstructedJob{
			Variant:          JobVariantV1,
			CollectorCleanup: func(context.Context) error { cleanups++; return nil },
			outputGate:       outputGate,
		},
		attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       "module_job",
			Resource:  "module_job",
		},
	)
	finished := make(chan error, 1)
	go func() {
		finished <- owner.finishCandidate(candidateCtx)
	}()
	promoted := make(chan error, 1)
	go func() {
		promoted <- owner.Promote(context.Background())
	}()
	select {
	case <-attempts.entered:
	case <-time.After(time.Second):
		t.Fatal("promotion did not reach busy runtime supersession")
	}

	cancelCandidate(jobmgr.ErrProcessAttemptStopped)
	timer := time.NewTimer(time.Second)
	defer timer.Stop()
	select {
	case err := <-promoted:
		require.ErrorIs(t, err, jobmgr.ErrProcessAttemptStopped)
	case <-timer.C:
		close(attempts.release)
		<-promoted
		t.Fatal("candidate cancellation did not interrupt busy runtime supersession")
	}
	require.NoError(t, <-finished)
	require.EqualValues(t, 1, cleanups)

	delegate.BeginShutdown()
	require.NoError(t, delegate.Shutdown(context.Background()))
}

func TestRuntimeContainmentSettlesPromotionOwnership(t *testing.T) {
	tests := map[string]struct {
		cutAfterAdmission bool
		cause             error
	}{
		"retired before admission": {
			cause: jobmgr.ErrProcessAttemptRetired,
		},
		"stopped before admission": {
			cause: jobmgr.ErrProcessAttemptStopped,
		},
		"retired after admission": {
			cutAfterAdmission: true,
			cause:             jobmgr.ErrProcessAttemptRetired,
		},
		"stopped after admission": {
			cutAfterAdmission: true,
			cause:             jobmgr.ErrProcessAttemptStopped,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			delegate, err := containment.NewAuthority(nil)
			require.NoError(t, err)
			attempts := &promotionAdmissionGateAuthority{
				ProcessAttemptAuthority: delegate,
				entered:                 make(chan struct{}),
				release:                 make(chan struct{}),
				cutAfterAdmission:       test.cutAfterAdmission,
			}
			frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
			require.NoError(t, err)
			gate, err := newGenerationOutputGate(frames)
			require.NoError(t, err)
			var cleanups atomic.Int32
			owner := newStagedJobOwner(
				context.Background(),
				ConstructedJob{
					Variant:          JobVariantV1,
					CollectorCleanup: func(context.Context) error { cleanups.Add(1); return nil },
					outputGate:       gate,
				},
				attempts,
				1,
				jobmgr.ProcessAttemptIdentity{
					Namespace: jobmgr.ProcessAttemptJobRuntime,
					Key:       "module_job",
					Resource:  "module_job",
				},
			)
			candidateFinished := make(chan error, 1)
			go func() {
				candidateFinished <- owner.finishCandidate(context.Background())
			}()
			promoted := make(chan error, 1)
			go func() {
				promoted <- owner.Promote(context.Background())
			}()
			requireTestSignal(t, attempts.entered, "promotion did not reach the admission boundary")

			cutTestAuthority(t, delegate, test.cause)
			close(attempts.release)
			err = <-promoted
			owner.Reject()

			require.ErrorIs(t, err, test.cause)
			require.True(t, jobmgr.ContainsOnlyErrorLeaves(
				err,
				jobmgr.ErrProcessAttemptRetired,
				jobmgr.ErrProcessAttemptStopped,
			))
			require.NoError(t, <-candidateFinished)
			require.NoError(t, delegate.Shutdown(context.Background()))
			require.EqualValues(t, 1, cleanups.Load())
			require.EqualValues(t, containment.Census{}, delegate.Census())
		})
	}
}

type promotionGateAuthority struct {
	jobmgr.ProcessAttemptAuthority
	entered chan struct{}
	release chan struct{}
}

func (a *promotionGateAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	close(a.entered)
	<-a.release
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}

type busyPromotionGateAuthority struct {
	jobmgr.ProcessAttemptAuthority
	entered chan struct{}
	release chan struct{}
}

type promotionAdmissionGateAuthority struct {
	jobmgr.ProcessAttemptAuthority
	entered           chan struct{}
	release           chan struct{}
	cutAfterAdmission bool
}

func (a *promotionAdmissionGateAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	work := plan.Work
	plan.Work = func(
		ctx context.Context,
		admission jobmgr.ProcessAttemptAdmission,
	) error {
		return work(ctx, promotionAdmissionGate{
			ProcessAttemptAdmission: admission,
			entered:                 a.entered,
			release:                 a.release,
			cutAfterAdmission:       a.cutAfterAdmission,
		})
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}

type promotionAdmissionGate struct {
	jobmgr.ProcessAttemptAdmission
	entered           chan struct{}
	release           chan struct{}
	cutAfterAdmission bool
}

func (gate promotionAdmissionGate) Admit() error {
	if gate.cutAfterAdmission {
		err := gate.ProcessAttemptAdmission.Admit()
		close(gate.entered)
		<-gate.release
		return err
	}
	close(gate.entered)
	<-gate.release
	return gate.ProcessAttemptAdmission.Admit()
}

type delayedRuntimeCancellationAuthority struct {
	jobmgr.ProcessAttemptAuthority
	cutObserved chan struct{}
	release     chan struct{}
	releaseOnce sync.Once
}

func (a *delayedRuntimeCancellationAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace != jobmgr.ProcessAttemptJobRuntime {
		return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
	}
	work := plan.Work
	plan.Work = func(
		attemptCtx context.Context,
		admission jobmgr.ProcessAttemptAdmission,
	) error {
		delayedCtx, cancel := context.WithCancelCause(context.Background())
		go func() {
			<-attemptCtx.Done()
			close(a.cutObserved)
			<-a.release
			cancel(context.Cause(attemptCtx))
		}()
		return work(delayedCtx, admission)
	}
	return a.ProcessAttemptAuthority.StartProcessAttempt(ctx, plan)
}

func (a *delayedRuntimeCancellationAuthority) releaseCancellation() {
	a.releaseOnce.Do(func() {
		close(a.release)
	})
}

func (*busyPromotionGateAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return nil, jobmgr.ErrProcessAttemptBusy
}

func (a *busyPromotionGateAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	_ jobmgr.ProcessAttemptIdentity,
) error {
	close(a.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-a.release:
		return jobmgr.ErrProcessAttemptBusy
	}
}

type preActivationRetirementWindow uint8

const (
	retirementBeforeBind preActivationRetirementWindow = iota + 1
	retirementAfterTransfer
	retirementAfterAdoption
)

type preparedAttachmentFixture struct {
	prepared PreparedJob
	identity lifecycle.ResourceIdentity
	owner    *stagedJobOwner
	gate     *generationOutputGate
	output   *bytes.Buffer
	job      *blockingStopManagedJob
	state    *preActivationRetirementState
	tasks    *lifecycle.TaskSupervisor
}

type preparedAttachmentFixtureOptions struct {
	variant      JobVariant
	attempts     jobmgr.ProcessAttemptAuthority
	withHandler  bool
	runtimeStage *stagedRuntimeService
	newRuntime   func(*stagedJobOwner) runtimecomp.Service
	newAttacher  func(*stagedJobOwner) JobHandlerAttacher
	beforeAttach func(*stagedJobOwner)
}

func newPreparedAttachmentFixture(
	t *testing.T,
	variant JobVariant,
	attempts jobmgr.ProcessAttemptAuthority,
	withHandler bool,
	attacher JobHandlerAttacher,
) *preparedAttachmentFixture {
	t.Helper()
	return newPreparedAttachmentFixtureWithOptions(t, preparedAttachmentFixtureOptions{
		variant:     variant,
		attempts:    attempts,
		withHandler: withHandler,
		newAttacher: func(*stagedJobOwner) JobHandlerAttacher {
			return attacher
		},
	})
}

func newPreparedAttachmentFixtureWithOptions(
	t *testing.T,
	options preparedAttachmentFixtureOptions,
) *preparedAttachmentFixture {
	t.Helper()
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	job := newBlockingStopManagedJob()
	state := &preActivationRetirementState{}
	candidate := ConstructedJob{
		Variant:          options.variant,
		CollectorCleanup: func(context.Context) error { state.rejected++; return nil },
		finalCleanup:     func(context.Context) error { state.final++; return nil },
		candidateJob:     job,
		runtimeStage:     options.runtimeStage,
		outputGate:       gate,
	}
	if options.withHandler {
		candidate.StagedHandlers = &preActivationRetirementHandler{state: state}
	}
	identity := lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}
	owner := newStagedJobOwner(
		context.Background(),
		candidate,
		options.attempts,
		1,
		jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJobRuntime,
			Key:       identity.ID,
			Resource:  identity.ID,
		},
	)
	var attacher JobHandlerAttacher
	if options.newAttacher != nil {
		attacher = options.newAttacher(owner)
	}
	var runtimeService runtimecomp.Service
	if options.newRuntime != nil {
		runtimeService = options.newRuntime(owner)
	}
	attachment := factoryAttachment{
		runtime:         runtimeService,
		handlerAttacher: attacher,
		scheduler:       newTestScheduler(t),
	}
	staged := candidate
	candidate.attach = func(
		identity lifecycle.ResourceIdentity,
		owner *stagedJobOwner,
	) (constructedJobAttachment, error) {
		if options.beforeAttach != nil {
			options.beforeAttach(owner)
		}
		return attachment.attach(staged, identity, owner)
	}
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	return &preparedAttachmentFixture{
		prepared: PreparedJob{state: &preparedJobState{
			id:          identity.ID,
			generation:  identity.Generation,
			constructed: candidate,
			permit:      permit,
			owner:       owner,
		}},
		identity: identity,
		owner:    owner,
		gate:     gate,
		output:   output,
		job:      job,
		state:    state,
		tasks:    tasks,
	}
}

type preActivationRetirementFixture struct {
	*preparedAttachmentFixture
	attempts     *containment.Authority
	wantHandler  bool
	shutdownOnce sync.Once
	shutdownErr  error
}

func newPreActivationRetirementFixture(
	t *testing.T,
	variant JobVariant,
	cause error,
	preBind bool,
) *preActivationRetirementFixture {
	window := retirementAfterTransfer
	if preBind {
		window = retirementBeforeBind
	}
	return newPreActivationRetirementFixtureForWindow(t, variant, cause, window, nil)
}

func newPreActivationRetirementFixtureWithRuntimeError(
	t *testing.T,
	variant JobVariant,
	cause error,
	preBind bool,
	runtimeErr error,
) *preActivationRetirementFixture {
	window := retirementAfterTransfer
	if preBind {
		window = retirementBeforeBind
	}
	return newPreActivationRetirementFixtureForWindow(t, variant, cause, window, runtimeErr)
}

func newPreActivationRetirementFixtureForWindow(
	t *testing.T,
	variant JobVariant,
	cause error,
	window preActivationRetirementWindow,
	runtimeErr error,
) *preActivationRetirementFixture {
	t.Helper()
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	var runtimeStage *stagedRuntimeService
	if runtimeErr != nil || window == retirementAfterAdoption {
		runtimeStage = newStagedRuntimeService()
		require.NoError(t, runtimeStage.RegisterComponent(runtimecomp.ComponentConfig{Name: "test"}))
	}
	cut := func() {
		cutTestAuthority(t, attempts, cause)
	}
	var newAttacher func(*stagedJobOwner) JobHandlerAttacher
	if window == retirementAfterTransfer {
		newAttacher = func(owner *stagedJobOwner) JobHandlerAttacher {
			return jobHandlerAttacherFunc(func(
				_ lifecycle.ResourceIdentity,
				staged StagedHandlerLifecycle,
			) (ProcessHandlerLifecycle, error) {
				cut()
				requireTestSignal(t, owner.retire, "runtime owner did not record retirement")
				return staged.(ProcessHandlerLifecycle), nil
			})
		}
	}
	var newRuntime func(*stagedJobOwner) runtimecomp.Service
	switch {
	case runtimeErr != nil:
		newRuntime = func(*stagedJobOwner) runtimecomp.Service {
			return projectionFailureRuntimeService{err: runtimeErr}
		}
	case window == retirementAfterAdoption:
		newRuntime = func(owner *stagedJobOwner) runtimecomp.Service {
			return projectionHookRuntimeService{register: func() error {
				cut()
				requireTestSignal(t, owner.retire, "runtime owner did not record retirement")
				return nil
			}}
		}
	}
	var beforeAttach func(*stagedJobOwner)
	if window == retirementBeforeBind {
		beforeAttach = func(owner *stagedJobOwner) {
			cut()
			requireTestSignal(t, owner.done, "retired candidate did not finalize before binding")
		}
	}
	prepared := newPreparedAttachmentFixtureWithOptions(t, preparedAttachmentFixtureOptions{
		variant:      variant,
		attempts:     attempts,
		withHandler:  window == retirementAfterTransfer,
		runtimeStage: runtimeStage,
		newRuntime:   newRuntime,
		newAttacher:  newAttacher,
		beforeAttach: beforeAttach,
	})
	fixture := &preActivationRetirementFixture{
		preparedAttachmentFixture: prepared,
		attempts:                  attempts,
		wantHandler:               window == retirementAfterTransfer,
	}
	t.Cleanup(func() {
		fixture.owner.Reject()
		require.NoError(t, fixture.shutdown())
	})
	return fixture
}

func (fixture *preActivationRetirementFixture) requireSettled(t *testing.T) {
	t.Helper()
	requireTestSignal(t, fixture.owner.done, "retired runtime owner did not finalize")
	_, writeErr := fixture.gate.Write([]byte("late\n"))
	require.ErrorIs(t, writeErr, errGenerationOutputFenced)
	require.Empty(t, fixture.output.Bytes())
	require.EqualValues(t, 1, fixture.state.rejected)
	require.Zero(t, fixture.state.final)
	if fixture.wantHandler {
		require.EqualValues(t, 1, fixture.state.handlerFinalized)
	} else {
		require.Zero(t, fixture.state.handlerFinalized)
	}
	require.EqualValues(t, lifecycle.LongLivedCensus{}, fixture.tasks.LongLivedCensus())
	require.NoError(t, fixture.shutdown())
	require.EqualValues(t, containment.Census{}, fixture.attempts.Census())
}

func (fixture *preActivationRetirementFixture) shutdown() error {
	fixture.shutdownOnce.Do(func() {
		fixture.attempts.BeginShutdown()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		fixture.shutdownErr = fixture.attempts.Shutdown(ctx)
	})
	return fixture.shutdownErr
}

type preActivationRetirementState struct {
	rejected         int
	final            int
	handlerFinalized int
}

type preActivationRetirementHandler struct {
	state *preActivationRetirementState
}

func (*preActivationRetirementHandler) Publish() error { return nil }

func (*preActivationRetirementHandler) CloseAndDrain(context.Context) error { return nil }

func (*preActivationRetirementHandler) Detach(context.Context) error { return nil }

func (handler *preActivationRetirementHandler) Finalize(context.Context) error {
	handler.state.handlerFinalized++
	return nil
}

type projectionFailureRuntimeService struct {
	err error
}

func (service projectionFailureRuntimeService) RegisterComponent(runtimecomp.ComponentConfig) error {
	return service.err
}

func (projectionFailureRuntimeService) UnregisterComponent(string) {}

func (projectionFailureRuntimeService) QuarantineComponent(string) {}

func (projectionFailureRuntimeService) FinalizeComponent(string) {}

func (service projectionFailureRuntimeService) RegisterProducer(string, func() error) error {
	return service.err
}

func (projectionFailureRuntimeService) UnregisterProducer(string) {}

type projectionHookRuntimeService struct {
	register func() error
}

func (service projectionHookRuntimeService) RegisterComponent(runtimecomp.ComponentConfig) error {
	return service.register()
}

func (projectionHookRuntimeService) UnregisterComponent(string) {}

func (projectionHookRuntimeService) QuarantineComponent(string) {}

func (projectionHookRuntimeService) FinalizeComponent(string) {}

func (service projectionHookRuntimeService) RegisterProducer(string, func() error) error {
	return service.register()
}

func (projectionHookRuntimeService) UnregisterProducer(string) {}

type jobHandlerAttacherFunc func(
	lifecycle.ResourceIdentity,
	StagedHandlerLifecycle,
) (ProcessHandlerLifecycle, error)

func (attach jobHandlerAttacherFunc) Attach(
	identity lifecycle.ResourceIdentity,
	staged StagedHandlerLifecycle,
) (ProcessHandlerLifecycle, error) {
	return attach(identity, staged)
}

func requireTestSignal(t *testing.T, signal <-chan struct{}, message string) {
	t.Helper()
	select {
	case <-signal:
	case <-time.After(time.Second):
		t.Fatal(message)
	}
}

func cutTestAuthority(t *testing.T, attempts *containment.Authority, cause error) {
	t.Helper()
	switch cause {
	case jobmgr.ErrProcessAttemptRetired:
		require.EqualValues(t, 1, attempts.CutTarget(1))
	case jobmgr.ErrProcessAttemptStopped:
		attempts.BeginShutdown()
	default:
		require.FailNow(t, "invalid retirement test cause")
	}
}

type testingHelper interface {
	require.TestingT
	Helper()
}

func issueTestJobPermit(
	t testingHelper,
	id string,
	generation uint64,
) (
	lifecycle.LongLivedPermit,
	*lifecycle.TaskSupervisor,
) {
	t.Helper()
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	plan := lifecycle.NewJobLongLivedPlan()
	permit, err := tasks.IssueLongLivedPermit(lifecycle.ResourceIdentity{
		ID:         id,
		Generation: generation,
	}, plan)
	require.NoError(t, err)
	return permit, tasks
}
