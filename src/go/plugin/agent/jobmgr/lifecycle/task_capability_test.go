// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestTaskSupervisorCommitsPreparedCapabilityWithoutReadyResource(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	require.NoError(t, err)
	identity := ResourceIdentity{ID: "secret", Generation: 1}
	var capability *testPreparedCapability
	plan, err := NewPreparedCapabilityPermitTaskPlan(SourceJobManager, time.Time{}, TransactionTaskPhases, admission, admissionRef, identity, permitPlan,
		func(_ context.Context, permit LongLivedPermit) (PreparedCapability, error) {
			if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
				return nil, err
			}
			capability = &testPreparedCapability{identity: identity, permit: permit}
			return capability, nil
		})
	require.NoError(t, err)
	ref := dispatchCapabilityTask(t, supervisor, plan)

	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != ref || completion.Kind != TaskOutcomePreparedCapability || completion.Err != nil)
	require.NoError(t, supervisor.CancelWithCause(
		ref,
		&StoppingRejection{Generation: 7},
	))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionCommitCapability, ExpectedGeneration: 1}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Err != nil || ack.CapabilityDisposition != CapabilityApplied)
	require.NoError(t, capability.commitContextErr)

	require.NoError(t, capability.releaseCarrier())

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	acknowledgementCh := <-supervisor.AcknowledgementCh()
	require.False(t, acknowledgementCh.Err != nil || acknowledgementCh.Kind != TaskActionTerminate)

	require.NoError(t, supervisor.Release(ref))

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, releaseOrdinaryErr)

}

func TestTaskSupervisorRetainsPreparedCapabilityAfterCommitPanic(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	require.NoError(t, err)
	identity := ResourceIdentity{ID: "secret", Generation: 1}
	var capability *testPreparedCapability
	plan, err := NewPreparedCapabilityPermitTaskPlan(SourceJobManager, time.Time{}, TransactionTaskPhases, admission, admissionRef, identity, permitPlan,
		func(_ context.Context, permit LongLivedPermit) (PreparedCapability, error) {
			if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
				return nil, err
			}
			capability = &testPreparedCapability{identity: identity, permit: permit, panicCommit: true}
			return capability, nil
		})
	require.NoError(t, err)
	ref := dispatchCapabilityTask(t, supervisor, plan)
	<-supervisor.CompletionCh()

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionCommitCapability, ExpectedGeneration: 1}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, !errors.Is(ack.Err, ErrTaskPanic) || ack.CapabilityDisposition != CapabilityRetained)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionDispose}))

	acknowledgementCh := <-supervisor.AcknowledgementCh()
	require.False(t, acknowledgementCh.Err != nil || acknowledgementCh.Kind != TaskActionDispose)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, releaseOrdinaryErr)

}

func TestTaskSupervisorDisposesPreparedCapabilityWithWrongPermitOwner(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	require.NoError(t, err)
	permitOwner := ResourceIdentity{ID: "secret", Generation: 1}
	wrongOwner := ResourceIdentity{ID: "other-secret", Generation: 1}
	plan, err := NewPreparedCapabilityPermitTaskPlan(SourceJobManager, time.Time{}, TransactionTaskPhases, admission, admissionRef, permitOwner, permitPlan,
		func(_ context.Context, permit LongLivedPermit) (PreparedCapability, error) {
			if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
				return nil, err
			}
			return &testPreparedCapability{identity: wrongOwner, permit: permit}, nil
		})
	require.NoError(t, err)
	ref := dispatchCapabilityTask(t, supervisor, plan)
	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != ref || completion.Kind != TaskOutcomePreparedCapability || completion.Err == nil || !strings.Contains(completion.Err.Error(), "prepared capability identity differs from permit owner"))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Err != nil || ack.Kind != TaskActionDispose)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, releaseOrdinaryErr)

	census := supervisor.LongLivedCensus()
	require.EqualValues(t, LongLivedCensus{}, census)
}

func TestTaskSupervisorDisposesPreparedCapabilityAfterIdentityPanic(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	require.NoError(t, err)
	permitOwner := ResourceIdentity{ID: "secret", Generation: 1}
	plan, err := NewPreparedCapabilityPermitTaskPlan(SourceJobManager, time.Time{}, TransactionTaskPhases, admission, admissionRef, permitOwner, permitPlan,
		func(_ context.Context, permit LongLivedPermit) (PreparedCapability, error) {
			if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
				return nil, err
			}
			return &testPreparedCapability{identity: permitOwner, permit: permit, panicIdentity: true}, nil
		})
	require.NoError(t, err)
	ref := dispatchCapabilityTask(t, supervisor, plan)
	completion := <-supervisor.CompletionCh()
	require.False(t, completion.Ref != ref || completion.Kind != TaskOutcomePreparedCapability || completion.Err == nil || !strings.Contains(completion.Err.Error(), "prepared capability identity panic"))

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}))

	ack := <-supervisor.AcknowledgementCh()
	require.False(t, ack.Err != nil || ack.Kind != TaskActionDispose)

	require.NoError(t, supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}))

	require.Nil(t, (<-supervisor.AcknowledgementCh()).Err)

	require.NoError(t, supervisor.Release(ref))

	_, releaseOrdinaryErr := admission.ReleaseOrdinary(admissionRef)
	require.NoError(t, releaseOrdinaryErr)

	census := supervisor.LongLivedCensus()
	require.EqualValues(t, LongLivedCensus{}, census)
}

func dispatchCapabilityTask(t *testing.T, supervisor *TaskSupervisor, plan TaskPlan) TaskRef {
	t.Helper()
	request, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
	require.NoError(t, err)
	var starts [TaskStartServiceQuantum]TaskStart
	count, _, err := supervisor.Dispatch(context.Background(), 1, &starts)
	require.False(t, err != nil || count != 1 || starts[0].Request != request)
	return starts[0].Task
}

type testPreparedCapability struct {
	identity         ResourceIdentity
	permit           LongLivedPermit
	panicIdentity    bool
	panicCommit      bool
	commitContextErr error
}

func (tpc *testPreparedCapability) Identity() ResourceIdentity {
	if tpc.panicIdentity {
		panic("identity panic")
	}
	return tpc.identity
}

func (tpc *testPreparedCapability) Commit(ctx context.Context, _ uint64) (CapabilityDisposition, error) {
	tpc.commitContextErr = ctx.Err()
	if tpc.panicCommit {
		panic("commit panic")
	}
	return CapabilityApplied, nil
}

func (tpc *testPreparedCapability) Dispose(context.Context) error {
	return tpc.releaseCarrier()
}

func (tpc *testPreparedCapability) releaseCarrier() error {
	if err := tpc.permit.ReleaseExternal(LongLivedESecretStore); err != nil {
		return err
	}
	if err := tpc.permit.ReleaseBytes(); err != nil {
		return err
	}
	return tpc.permit.Return()
}
