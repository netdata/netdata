// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
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
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { rejected++; return nil },
		finalCleanup:     func(context.Context) error { final++; return nil },
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
	) (ConstructedJob, error) {
		require.NoError(t, owner.BindAttachment())
		return candidate, nil
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

func TestPreparedJobRetirementBeforeResourceAcceptanceLeavesPermitUnused(t *testing.T) {
	attempts, err := containment.NewAuthority(nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		attempts.BeginShutdown()
		require.NoError(t, attempts.Shutdown(context.Background()))
	})
	identity := lifecycle.ResourceIdentity{ID: "module_job", Generation: 1}
	candidate := ConstructedJob{
		Variant:          JobVariantV1,
		CollectorCleanup: func(context.Context) error { return nil },
		finalCleanup:     func(context.Context) error { return nil },
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
	) (ConstructedJob, error) {
		require.NoError(t, owner.BindAttachment())
		candidate.activateAttachment = func() error {
			owner.Retire()
			return nil
		}
		return candidate, nil
	}
	permit, tasks := issueTestJobPermit(t, identity.ID, identity.Generation)
	prepared := PreparedJob{state: &preparedJobState{
		id:          identity.ID,
		generation:  identity.Generation,
		constructed: candidate,
		permit:      permit,
		owner:       owner,
	}}

	_, err = prepared.Accept(context.Background(), identity.Generation)
	require.Error(t, err)

	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
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
