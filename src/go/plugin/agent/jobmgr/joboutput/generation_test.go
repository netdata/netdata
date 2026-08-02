// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"errors"
	"sync"
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

type preActivationRetirementFixture struct {
	prepared     PreparedJob
	identity     lifecycle.ResourceIdentity
	owner        *stagedJobOwner
	gate         *generationOutputGate
	output       *bytes.Buffer
	state        *preActivationRetirementState
	tasks        *lifecycle.TaskSupervisor
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
	output := &bytes.Buffer{}
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	gate, err := newGenerationOutputGate(frames)
	require.NoError(t, err)
	job := newBlockingStopManagedJob()
	state := &preActivationRetirementState{}
	handler := &preActivationRetirementHandler{state: state}
	candidate := ConstructedJob{
		Variant:          variant,
		CollectorCleanup: func(context.Context) error { state.rejected++; return nil },
		finalCleanup:     func(context.Context) error { state.final++; return nil },
		candidateJob:     job,
		outputGate:       gate,
	}
	if runtimeErr != nil || window == retirementAfterAdoption {
		candidate.runtimeStage = newStagedRuntimeService()
		require.NoError(t, candidate.runtimeStage.RegisterComponent(runtimecomp.ComponentConfig{Name: "test"}))
	}
	if window == retirementAfterTransfer {
		candidate.StagedHandlers = handler
	}
	identity := lifecycle.ResourceIdentity{ID: job.FullName(), Generation: 1}
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
	cut := func() {
		switch cause {
		case jobmgr.ErrProcessAttemptRetired:
			require.EqualValues(t, 1, attempts.CutTarget(1))
		case jobmgr.ErrProcessAttemptStopped:
			attempts.BeginShutdown()
		default:
			require.FailNow(t, "invalid retirement test cause")
		}
	}
	var attacher JobHandlerAttacher
	if window == retirementAfterTransfer {
		attacher = jobHandlerAttacherFunc(func(
			_ lifecycle.ResourceIdentity,
			staged StagedHandlerLifecycle,
		) (ProcessHandlerLifecycle, error) {
			cut()
			requireTestSignal(t, owner.retire, "runtime owner did not record retirement")
			return staged.(ProcessHandlerLifecycle), nil
		})
	}
	var runtimeService runtimecomp.Service
	switch {
	case runtimeErr != nil:
		runtimeService = projectionFailureRuntimeService{err: runtimeErr}
	case window == retirementAfterAdoption:
		runtimeService = projectionHookRuntimeService{register: func() error {
			cut()
			requireTestSignal(t, owner.retire, "runtime owner did not record retirement")
			return nil
		}}
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
		if window == retirementBeforeBind {
			cut()
			requireTestSignal(t, owner.done, "retired candidate did not finalize before binding")
		}
		return attachment.attach(staged, identity, owner)
	}
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	fixture := &preActivationRetirementFixture{
		prepared: PreparedJob{state: &preparedJobState{
			id:          identity.ID,
			generation:  identity.Generation,
			constructed: candidate,
			permit:      permit,
			owner:       owner,
		}},
		identity:    identity,
		owner:       owner,
		gate:        gate,
		output:      output,
		state:       state,
		tasks:       tasks,
		attempts:    attempts,
		wantHandler: window == retirementAfterTransfer,
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
