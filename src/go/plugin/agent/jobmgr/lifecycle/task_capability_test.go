// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestTaskSupervisorCommitsPreparedCapabilityWithoutReadyResource(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	ref := dispatchCapabilityTask(t, supervisor, plan)
	if completion := <-supervisor.CompletionCh(); completion.Ref != ref || completion.Kind != TaskOutcomePreparedCapability || completion.Err != nil {
		t.Fatalf("prepared capability completion differs: %+v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionCommitCapability, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if ack.Err != nil || ack.CapabilityDisposition != CapabilityApplied {
		t.Fatalf("capability commit acknowledgement differs: %+v", ack)
	}
	if err := capability.releaseCarrier(); err != nil {
		t.Fatal(err)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil || ack.Kind != TaskActionTerminate {
		t.Fatalf("capability termination differs: %+v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorRetainsPreparedCapabilityAfterCommitPanic(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	ref := dispatchCapabilityTask(t, supervisor, plan)
	<-supervisor.CompletionCh()
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionCommitCapability, ExpectedGeneration: 1}); err != nil {
		t.Fatal(err)
	}
	ack := <-supervisor.AcknowledgementCh()
	if !errors.Is(ack.Err, ErrTaskPanic) || ack.CapabilityDisposition != CapabilityRetained {
		t.Fatalf("capability panic acknowledgement differs: %+v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil || ack.Kind != TaskActionDispose {
		t.Fatalf("capability disposal differs: %+v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 4, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatalf("capability termination differs: %+v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
		t.Fatal(err)
	}
}

func TestTaskSupervisorDisposesPreparedCapabilityWithWrongPermitOwner(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
	permitOwner := ResourceIdentity{ID: "secret", Generation: 1}
	wrongOwner := ResourceIdentity{ID: "other-secret", Generation: 1}
	plan, err := NewPreparedCapabilityPermitTaskPlan(SourceJobManager, time.Time{}, TransactionTaskPhases, admission, admissionRef, permitOwner, permitPlan,
		func(_ context.Context, permit LongLivedPermit) (PreparedCapability, error) {
			if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
				return nil, err
			}
			return &testPreparedCapability{identity: wrongOwner, permit: permit}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	ref := dispatchCapabilityTask(t, supervisor, plan)
	completion := <-supervisor.CompletionCh()
	if completion.Ref != ref || completion.Kind != TaskOutcomePreparedCapability || completion.Err == nil || !strings.Contains(completion.Err.Error(), "prepared capability identity differs from permit owner") {
		t.Fatalf("wrong-owner completion differs: %+v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil || ack.Kind != TaskActionDispose {
		t.Fatalf("wrong-owner disposal differs: %+v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatalf("wrong-owner termination differs: %+v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
		t.Fatal(err)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("wrong-owner capability retained permit: %+v", census)
	}
}

func TestTaskSupervisorDisposesPreparedCapabilityAfterIdentityPanic(t *testing.T) {
	admission := NewAdmissionLedger()
	admissionRef := grantLongLivedTestAdmission(t, admission, 100)
	supervisor := newLongLivedTestSupervisor(t)
	permitPlan, err := NewSecretStoreLongLivedPlan(40)
	if err != nil {
		t.Fatal(err)
	}
	permitOwner := ResourceIdentity{ID: "secret", Generation: 1}
	plan, err := NewPreparedCapabilityPermitTaskPlan(SourceJobManager, time.Time{}, TransactionTaskPhases, admission, admissionRef, permitOwner, permitPlan,
		func(_ context.Context, permit LongLivedPermit) (PreparedCapability, error) {
			if err := permit.ActivateExternal(LongLivedESecretStore); err != nil {
				return nil, err
			}
			return &testPreparedCapability{identity: permitOwner, permit: permit, panicIdentity: true}, nil
		})
	if err != nil {
		t.Fatal(err)
	}
	ref := dispatchCapabilityTask(t, supervisor, plan)
	completion := <-supervisor.CompletionCh()
	if completion.Ref != ref || completion.Kind != TaskOutcomePreparedCapability || completion.Err == nil || !strings.Contains(completion.Err.Error(), "prepared capability identity panic") {
		t.Fatalf("identity-panic completion differs: %+v", completion)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 2, Kind: TaskActionDispose}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil || ack.Kind != TaskActionDispose {
		t.Fatalf("identity-panic disposal differs: %+v", ack)
	}
	if err := supervisor.SendAction(TaskAction{Ref: ref, Sequence: 3, Kind: TaskActionTerminate}); err != nil {
		t.Fatal(err)
	}
	if ack := <-supervisor.AcknowledgementCh(); ack.Err != nil {
		t.Fatalf("identity-panic termination differs: %+v", ack)
	}
	if err := supervisor.Release(ref); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
		t.Fatal(err)
	}
	if census := supervisor.LongLivedCensus(); census != (LongLivedCensus{}) {
		t.Fatalf("identity-panic capability retained permit: %+v", census)
	}
}

func dispatchCapabilityTask(t *testing.T, supervisor *TaskSupervisor, plan TaskPlan) TaskRef {
	t.Helper()
	request, err := supervisor.Enqueue(TaskClassFrameworkControl, plan)
	if err != nil {
		t.Fatal(err)
	}
	var starts [TaskStartServiceQuantum]TaskStart
	count, _, err := supervisor.Dispatch(context.Background(), 1, &starts)
	if err != nil || count != 1 || starts[0].Request != request {
		t.Fatalf("capability dispatch differs: count=%d start=%+v err=%v", count, starts[0], err)
	}
	return starts[0].Task
}

type testPreparedCapability struct {
	identity      ResourceIdentity
	permit        LongLivedPermit
	panicIdentity bool
	panicCommit   bool
}

func (tpc *testPreparedCapability) Identity() ResourceIdentity {
	if tpc.panicIdentity {
		panic("identity panic")
	}
	return tpc.identity
}

func (tpc *testPreparedCapability) Commit(context.Context, uint64) (CapabilityDisposition, error) {
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
