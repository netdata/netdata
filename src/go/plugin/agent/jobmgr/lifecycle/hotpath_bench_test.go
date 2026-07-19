package lifecycle

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func BenchmarkBAdmission(b *testing.B) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	var grants [4]AdmissionGrant
	b.ReportAllocs()
	for b.Loop() {
		request := ledger.RequestOrdinary(1, lane, 1)
		if request.Rejected != nil {
			require.FailNow(b, "benchmark failed", request.Rejected)
		}
		count, _, err := ledger.TakeGrants(1, &grants)
		if err != nil || count != 1 {
			require.FailNowf(b, "benchmark failed", "grant count=%d err=%v", count, err)
		}
		if _, err := ledger.ReleaseOrdinary(request.Ref); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBOperationTransition(b *testing.B) {
	b.ReportAllocs()
	var id OperationID
	for b.Loop() {
		id++
		operation, err := NewOperation(
			id,
			"uid",
			SourceFunction,
			"lane",
			true,
		)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		for _, state := range []OperationState{
			OperationQueued,
			OperationReady,
			OperationRunning,
			OperationDisposing,
		} {
			if err := operation.Advance(state); err != nil {
				require.FailNow(b, "benchmark failed", err)
			}
		}
		if err := operation.MarkResponsePending(); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if err := operation.CommitResponse(); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if !operation.CanDisposeTerminal() {
			require.FailNow(b, "benchmark failed", "terminal disposition was not ready")
		}
		if err := operation.Advance(OperationDisposedTerminal); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBTaskSupervisorEnqueueCancel(b *testing.B) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	plan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(
			func(context.Context) (SealedResult, error) {
				return NewSealedResult(200, "application/json", []byte(`{}`))
			},
		),
	}
	b.ReportAllocs()
	for b.Loop() {
		ref, err := supervisor.Enqueue(TaskClassGenericFunction, plan)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if err := supervisor.CancelPending(ref); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBTaskSupervisorDispatch(b *testing.B) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	plan := TaskPlan{
		Source: SourceFunction,
		Work: func(context.Context) (TaskOutcome, error) {
			return TaskOutcome{}, nil
		},
	}
	var started [TaskStartServiceQuantum]TaskStart
	pending, err := supervisor.Enqueue(TaskClassGenericFunction, plan)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		count, more, err := supervisor.Dispatch(context.Background(), 1, &started)
		b.StopTimer()
		if err != nil || count != 1 || more {
			require.FailNowf(b, "benchmark failed",
				"dispatch count=%d more=%t err=%v",
				count,
				more,
				err,
			)
		}
		if started[0].Request != pending {
			require.FailNowf(b, "benchmark failed",
				"started request=%+v, want %+v",
				started[0].Request,
				pending,
			)
		}
		completion := <-supervisor.CompletionCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2,
			Kind: TaskActionDispose,
		}); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		ack := <-supervisor.AcknowledgementCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: ack.Ref, Sequence: 3,
			Kind: TaskActionTerminate,
		}); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		ack = <-supervisor.AcknowledgementCh()
		if err := supervisor.Release(ack.Ref); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		started[0] = TaskStart{}
		pending, err = supervisor.Enqueue(TaskClassGenericFunction, plan)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		b.StartTimer()
	}
	b.StopTimer()
	if err := supervisor.CancelPending(pending); err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
}

func BenchmarkBTaskChildLaunchCompletion(b *testing.B) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	plan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(
			func(context.Context) (SealedResult, error) {
				return NewSealedResult(200, "application/json", []byte(`{}`))
			},
		),
	}
	var started [TaskStartServiceQuantum]TaskStart
	b.ReportAllocs()
	for b.Loop() {
		if _, err := supervisor.Enqueue(TaskClassGenericFunction, plan); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		count, _, err := supervisor.Dispatch(context.Background(), 1, &started)
		if err != nil || count != 1 {
			require.FailNowf(b, "benchmark failed", "dispatch count=%d err=%v", count, err)
		}
		completion := <-supervisor.CompletionCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2,
			Kind: TaskActionDispose,
		}); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		ack := <-supervisor.AcknowledgementCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: ack.Ref, Sequence: 3,
			Kind: TaskActionTerminate,
		}); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		ack = <-supervisor.AcknowledgementCh()
		if err := supervisor.Release(ack.Ref); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBFrameCommit(b *testing.B) {
	owner, err := NewFrameOwner(io.Discard)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	result, err := NewSealedResult(200, "application/json", []byte(`{"status":"ok"}`))
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		frame, err := PrepareFrame("uid", result, 1)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if err := owner.Commit(frame); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBRunAck(b *testing.B) {
	supervisor, err := NewRunSupervisor(
		1,
		RealClock{},
		time.Second,
	)
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	if err := supervisor.OpenAdmission(); err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if !supervisor.Admitting() {
			require.FailNow(b, "benchmark failed", "run unexpectedly stopped admitting")
		}
	}
}

func BenchmarkBUIDAdmission(b *testing.B) {
	ledger := NewUIDLedger()
	now := time.Unix(1_000, 0)
	var uids [256]string
	for index := range uids {
		uids[index] = fmt.Sprintf("uid-%03d", index)
	}
	var sequence uint64
	b.ReportAllocs()
	for b.Loop() {
		sequence++
		uid := uids[sequence%uint64(len(uids))]
		if err := ledger.Admit(uid, now); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if err := ledger.Complete(uid, false, now); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}
