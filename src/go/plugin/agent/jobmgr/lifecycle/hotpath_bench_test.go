package lifecycle

import (
	"context"
	"fmt"
	"io"
	"testing"
	"time"
)

func BenchmarkBAdmission(b *testing.B) {
	ledger := NewAdmissionLedger()
	lane := AdmissionLaneRef{Slot: 1, Generation: 1}
	var grants [4]AdmissionGrant
	b.ReportAllocs()
	for b.Loop() {
		request := ledger.RequestOrdinary(1, lane, 1)
		if request.Rejected != nil {
			b.Fatal(request.Rejected)
		}
		count, _, err := ledger.TakeGrants(1, &grants)
		if err != nil || count != 1 {
			b.Fatalf("grant count=%d err=%v", count, err)
		}
		if _, err := ledger.ReleaseOrdinary(request.Ref); err != nil {
			b.Fatal(err)
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
			b.Fatal(err)
		}
		for _, state := range []OperationState{
			OperationQueued,
			OperationReady,
			OperationRunning,
			OperationDisposing,
		} {
			if err := operation.Advance(state); err != nil {
				b.Fatal(err)
			}
		}
		if err := operation.MarkResponsePending(); err != nil {
			b.Fatal(err)
		}
		if err := operation.CommitResponse(); err != nil {
			b.Fatal(err)
		}
		if !operation.CanDisposeTerminal() {
			b.Fatal("terminal disposition was not ready")
		}
		if err := operation.Advance(
			OperationDisposedTerminal,
		); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBTaskSupervisorDispatch(b *testing.B) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		b.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		b.Fatal(err)
	}
	plan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(
			func(context.Context) (SealedResult, error) {
				return NewSealedResult(
					200,
					"application/json",
					[]byte(`{}`),
				)
			},
		),
	}
	b.ReportAllocs()
	for b.Loop() {
		ref, err := supervisor.Enqueue(TaskClassGenericFunction, plan)
		if err != nil {
			b.Fatal(err)
		}
		if err := supervisor.CancelPending(ref); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBTaskChildLaunchCompletion(b *testing.B) {
	frame, err := NewFrameOwner(io.Discard)
	if err != nil {
		b.Fatal(err)
	}
	supervisor, err := NewTaskSupervisor(frame)
	if err != nil {
		b.Fatal(err)
	}
	plan := TaskPlan{
		Source: SourceFunction,
		Work: FrameTaskWork(
			func(context.Context) (SealedResult, error) {
				return NewSealedResult(
					200,
					"application/json",
					[]byte(`{}`),
				)
			},
		),
	}
	var started [TransientTaskSlots]TaskStart
	b.ReportAllocs()
	for b.Loop() {
		if _, err := supervisor.Enqueue(
			TaskClassGenericFunction,
			plan,
		); err != nil {
			b.Fatal(err)
		}
		count, _, err := supervisor.Dispatch(
			context.Background(),
			1,
			&started,
		)
		if err != nil || count != 1 {
			b.Fatalf("dispatch count=%d err=%v", count, err)
		}
		completion := <-supervisor.CompletionCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: completion.Ref, Sequence: 2,
			Kind: TaskActionDispose,
		}); err != nil {
			b.Fatal(err)
		}
		ack := <-supervisor.AcknowledgementCh()
		if err := supervisor.SendAction(TaskAction{
			Ref: ack.Ref, Sequence: 3,
			Kind: TaskActionTerminate,
		}); err != nil {
			b.Fatal(err)
		}
		ack = <-supervisor.AcknowledgementCh()
		if err := supervisor.Release(ack.Ref); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBFrameCommit(b *testing.B) {
	owner, err := NewFrameOwner(io.Discard)
	if err != nil {
		b.Fatal(err)
	}
	result, err := NewSealedResult(
		200,
		"application/json",
		[]byte(`{"status":"ok"}`),
	)
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		frame, err := PrepareFrame("uid", result, 1)
		if err != nil {
			b.Fatal(err)
		}
		if err := owner.Commit(frame); err != nil {
			b.Fatal(err)
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
		b.Fatal(err)
	}
	if err := supervisor.OpenAdmission(); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if !supervisor.Admitting() {
			b.Fatal("run unexpectedly stopped admitting")
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
			b.Fatal(err)
		}
		if err := ledger.Complete(uid, false, now); err != nil {
			b.Fatal(err)
		}
	}
}
