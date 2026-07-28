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
