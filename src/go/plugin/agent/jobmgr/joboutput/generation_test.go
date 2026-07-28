// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"bytes"
	"testing"

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
