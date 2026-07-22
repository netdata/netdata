// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"container/heap"
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func (ck *CommandKernel) cancelOperation(uid string) {
	ck.cancelOperationWithCause(uid, context.Canceled)
}

func (ck *CommandKernel) cancelOperationWithCause(uid string, cause error) {
	operation := ck.operations[uid]
	if operation != nil && operation.activeChild != nil && !operation.activeChild.compositeRollback {
		ck.cancelOperationWithCause(operation.activeChild.UID, cause)
	}
	if operation == nil || operation.Response == lifecycle.ResponseCommitted ||
		operation.Response == lifecycle.ResponsePoisoned {
		return
	}
	if operation.TimedOut() {
		return
	}
	if !operation.cancelled {
		ck.recordCompositeChildTerminalCause(operation, cause)
	}
	operation.cancelled = true
	event := DiagnosticEvent{
		Name:       "operation cancelled",
		UID:        operation.UID,
		Route:      operation.request.Route,
		Lane:       operation.LaneKey,
		Generation: ck.run.Generation(),
		Operation:  operation.ID,
		Task:       operation.Task,
		Source:     operation.Source,
		Err:        cause,
		Rollback:   operation.compositeRollback,
		Composite:  operation.parent != nil || operation.composite != nil,
	}
	if operation.plan.Transaction != nil {
		event.Resource = operation.plan.Transaction.ID
	}
	ck.trace(event)
	if operation.Child == lifecycle.ChildExecuting {
		_ = ck.tasks.CancelWithCause(operation.Task, cause)
		if operation.Response != lifecycle.ResponseNotRequired && !operation.plan.CooperativeCancel {
			ck.enqueueControl(operation, lifecycle.ControlCancelled)
		}
		return
	}
	if operation.Child == lifecycle.ChildDeadlineStartPending {
		return
	}
	if operation.Child == lifecycle.ChildNotStarted {
		ck.unlinkQueued(operation)
		if operation.Response != lifecycle.ResponseNotRequired {
			ck.enqueueControl(operation, lifecycle.ControlCancelled)
		} else {
			ck.tryDispose(operation)
		}
		return
	}
}

func (ck *CommandKernel) serviceDeadlines(now time.Time, quantum int) bool {
	for quantum > 0 && ck.deadlines.Len() > 0 {
		entry := ck.deadlines[0]
		if entry.when.After(now) {
			return false
		}
		heap.Pop(&ck.deadlines)
		quantum--
		operation := entry.operation
		if operation.State >= lifecycle.OperationDisposing ||
			(operation.Response != lifecycle.ResponseOpen && operation.Response != lifecycle.ResponseNotRequired) {
			continue
		}
		ck.markOperationTimedOut(operation)
		if operation.activeChild != nil && !operation.activeChild.compositeRollback {
			ck.cancelOperationWithCause(operation.activeChild.UID, context.DeadlineExceeded)
		}
		deferControl := defersDeadlineControl(operation)
		if operation.Child == lifecycle.ChildExecuting {
			_ = ck.tasks.CancelWithCause(operation.Task, context.DeadlineExceeded)
			if operation.Response == lifecycle.ResponseNotRequired {
				if err := ck.markRetainedTimeout(operation, true); err != nil {
					ck.run.Dirty(err)
				}
			}
		} else if operation.Child == lifecycle.ChildNotStarted {
			if requiresCooperativeDeadlineStart(operation) {
				if err := operation.RequireDeadlineStart(); err != nil {
					ck.run.Dirty(err)
					return false
				}
				if operation.taskRequest.Valid() {
					if err := ck.tasks.SetPendingCancellation(operation.taskRequest, context.DeadlineExceeded); err != nil {
						ck.run.Dirty(err)
						return false
					}
				}
			} else {
				ck.unlinkQueued(operation)
				if operation.Response == lifecycle.ResponseNotRequired {
					ck.tryDispose(operation)
				}
			}
		}
		if operation.Response != lifecycle.ResponseNotRequired && !deferControl {
			ck.enqueueControl(operation, lifecycle.ControlDeadline)
		}
	}
	return ck.deadlines.Len() > 0 && !ck.deadlines[0].when.After(now)
}

func requiresCooperativeDeadlineStart(operation *commandOperation) bool {
	return operation != nil && operation.plan.Work != nil && operation.plan.CooperativeDeadline
}

func defersDeadlineControl(operation *commandOperation) bool {
	if operation == nil {
		return false
	}
	if operation.plan.Transaction != nil && operation.Child == lifecycle.ChildActionPending {
		return true
	}
	if !operation.plan.CooperativeDeadline {
		return false
	}
	return operation.Child == lifecycle.ChildExecuting ||
		operation.Child == lifecycle.ChildNotStarted && requiresCooperativeDeadlineStart(operation)
}

func (ck *CommandKernel) expireReadyOperationDeadline(operation *commandOperation) bool {
	if operation == nil || operation.TimedOut() || operation.request.Deadline.IsZero() ||
		operation.request.Deadline.After(ck.clock.Now()) {
		return false
	}
	ck.markOperationTimedOut(operation)
	if requiresCooperativeDeadlineStart(operation) {
		if err := operation.RequireDeadlineStart(); err != nil {
			ck.run.Dirty(err)
			return true
		}
		return false
	}
	ck.unlinkQueued(operation)
	if operation.Response == lifecycle.ResponseOpen {
		ck.enqueueControl(operation, lifecycle.ControlDeadline)
	} else {
		ck.tryDispose(operation)
	}
	return true
}

func (ck *CommandKernel) markOperationDeadlineIfDue(operation *commandOperation) {
	if operation == nil || operation.request.Deadline.IsZero() ||
		(operation.Response != lifecycle.ResponseOpen && operation.Response != lifecycle.ResponseNotRequired) {
		return
	}
	if operation.TimedOut() {
		return
	}
	if operation.request.Deadline.After(ck.clock.Now()) {
		return
	}
	ck.markOperationTimedOut(operation)
}

func (ck *CommandKernel) markOperationTimedOut(operation *commandOperation) {
	if operation == nil || operation.TimedOut() {
		return
	}
	operation.MarkTimedOut()
	ck.traceOperation("operation deadline reached", operation)
	ck.recordCompositeChildTerminalCause(operation, context.DeadlineExceeded)
	if ck.runtimeObserver != nil {
		ck.runtimeObserver.AddRuntimeCounter(lifecycle.RuntimeCounterOperationTimeouts, 1)
	}
}

func (ck *CommandKernel) recordCompositeChildTerminalCause(operation *commandOperation, cause error) {
	if operation == nil || operation.parent == nil || operation.ownershipChain ||
		operation.Response != lifecycle.ResponseNotRequired || cause == nil ||
		errors.Is(operation.terminalErr, cause) {
		return
	}
	operation.terminalErr = errors.Join(operation.terminalErr, cause)
}

func (ck *CommandKernel) enqueueControl(operation *commandOperation, status lifecycle.ControlStatus) {
	if operation == nil || operation.Response != lifecycle.ResponseOpen || operation.controlQueued {
		return
	}
	operation.control = status
	operation.controlQueued = true
	ck.controls.push(operation)
	event := DiagnosticEvent{
		Name:       "operation control queued",
		UID:        operation.UID,
		Route:      operation.request.Route,
		Lane:       operation.LaneKey,
		Generation: ck.run.Generation(),
		Operation:  operation.ID,
		Task:       operation.Task,
		Source:     operation.Source,
		Control:    status,
	}
	if operation.plan.Transaction != nil {
		event.Resource = operation.plan.Transaction.ID
	}
	ck.trace(event)
}

func (ck *CommandKernel) serviceControls(quantum int) bool {
	for quantum > 0 && ck.controls.count > 0 {
		operation := ck.controls.front()
		if operation.Response == lifecycle.ResponseOpen {
			_ = operation.MarkResponsePending()
		}
		err := ck.frames.TryCommitControl(
			lifecycle.ControlFramePlan{
				UID:    operation.UID,
				Status: operation.control,
				Expiry: lifecycle.ExpiryAt(ck.clock.Now()),
			},
		)
		if errors.Is(err, lifecycle.ErrFrameOwnerBusy) {
			return false
		}
		ck.controls.pop()
		operation.controlQueued = false
		quantum--
		if err != nil {
			ck.trace(DiagnosticEvent{
				Name:       "operation control frame failed",
				UID:        operation.UID,
				Route:      operation.request.Route,
				Lane:       operation.LaneKey,
				Generation: ck.run.Generation(),
				Operation:  operation.ID,
				Source:     operation.Source,
				Control:    operation.control,
				Err:        err,
			})
			operation.PoisonResponse()
			ck.run.Dirty(err)
			ck.tryDispose(operation)
			continue
		}
		if err := operation.CommitResponse(); err != nil {
			ck.run.Dirty(err)
			continue
		}
		ck.trace(DiagnosticEvent{
			Name:       "operation control frame committed",
			UID:        operation.UID,
			Route:      operation.request.Route,
			Lane:       operation.LaneKey,
			Generation: ck.run.Generation(),
			Operation:  operation.ID,
			Source:     operation.Source,
			Control:    operation.control,
		})
		if err := ck.completeOperationUID(operation, true); err != nil {
			ck.run.Dirty(err)
			return false
		}
		if operation.TimedOut() && operation.Child == lifecycle.ChildExecuting {
			if err := ck.markRetainedTimeout(operation, false); err != nil {
				ck.run.Dirty(err)
			}
		}
		ck.tryDispose(operation)
	}
	return ck.controls.count > 0
}

func (ck *CommandKernel) markRetainedTimeout(operation *commandOperation, background bool) error {
	if operation == nil || !operation.TimedOut() || operation.Child != lifecycle.ChildExecuting ||
		!operation.Task.Valid() {
		return errors.New("jobmgr kernel: invalid retained-timeout mark")
	}
	saturated, err := ck.tasks.MarkRetainedTimeout(operation.Task)
	if err != nil {
		return err
	}
	if saturated && background {
		return errors.New("jobmgr kernel: fourth background timeout reached the retained-timeout fail-stop threshold")
	}
	if saturated {
		return errors.New("jobmgr kernel: fourth retained timeout reached the fail-stop threshold")
	}
	return nil
}

func (ck *CommandKernel) tryDispose(operation *commandOperation) {
	if operation == nil || operation.activeChild != nil || operation.deferredCompletion != nil {
		return
	}
	if !operation.CanDisposeTerminal() {
		return
	}
	if operation.taskRequest.Valid() {
		ck.run.Dirty(errors.New("jobmgr kernel: terminal operation retained a pending task request"))
		return
	}
	if operation.State < lifecycle.OperationDisposing {
		_ = operation.Advance(lifecycle.OperationDisposing)
	}
	if err := ck.endCompositeFence(operation); err != nil {
		ck.run.Dirty(err)
		return
	}
	if !operation.uidCompleted {
		if err := ck.completeOperationUID(operation, false); err != nil {
			ck.run.Dirty(err)
			return
		}
	}
	if operation.functionInvocation.Valid() {
		ref := operation.functionInvocation
		operation.functionInvocation = FunctionInvocationRef{}
		if err := ck.releaseFunctionInvocation(ref); err != nil {
			ck.run.Dirty(err)
		}
	}
	if operation.deadline.index >= 0 {
		heap.Remove(&ck.deadlines, operation.deadline.index)
	}
	if operation.claimsHeld {
		ck.traceOperation("operation claims released", operation)
	}
	for _, granted := range ck.releaseClaims(operation) {
		ck.markReady(granted.lane)
	}
	lane := operation.lane
	if lane.active == operation {
		lane.active = nil
	}
	if lane.head == operation {
		ck.removeHead(lane)
	} else if operation.previous != nil || operation.next != nil {
		ck.unlink(operation)
	}
	if operation.plan.Transaction != nil {
		if lane.transactionPlanned <= 0 {
			ck.run.Dirty(errors.New("jobmgr kernel: transaction plan marker cleared twice"))
			return
		}
		lane.transactionPlanned--
	}
	lane.owners--
	if lane.owners < 0 {
		ck.run.Dirty(errors.New("jobmgr kernel: negative lane ownership"))
		return
	}
	if ck.shutdownPhase != commandShutdownRunning && ck.shutdownBarrierDone &&
		lane.shutdownVisited && lane.owners == 0 {
		if err := ck.enqueueShutdownStop(lane); err != nil {
			ck.run.Dirty(err)
			return
		}
	}
	_ = operation.Advance(lifecycle.OperationDisposedTerminal)
	ck.traceOperation("operation disposed", operation)
	delete(ck.operations, operation.UID)
	if operation.Source == lifecycle.SourceFunction {
		ck.functionOperations--
		if ck.functionOperations < 0 {
			ck.run.Dirty(errors.New("jobmgr kernel: negative Function operation count"))
			return
		}
	}
	ck.unlinkRuntimeOperation(operation)
	ck.observeRuntimeOperations()
	if ck.shutdownPhase == commandShutdownRunning || operation.shutdownVisited || operation.shutdownChild {
		ck.unlinkOperation(operation)
	}
	ck.completeCompositeChild(operation)
	if operation.ownershipChain {
		operation.ownershipChain = false
		ck.ownershipChains--
		if ck.ownershipChains < 0 {
			ck.run.Dirty(errors.New("jobmgr kernel: negative ownership continuation count"))
			return
		}
	}
	if operation.terminalResult != nil {
		operation.terminalResult <- operation.terminalErr
		operation.terminalResult = nil
	}
	if lane.active == nil && lane.head != nil {
		ck.markReady(lane)
	}
	ck.releaseUnusedLane(lane)
}

func (ck *CommandKernel) appendOperation(operation *commandOperation) {
	if operation == nil || operation.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid operation-list append"))
		return
	}
	operation.allPrevious = ck.operationTail
	if ck.operationTail != nil {
		ck.operationTail.allNext = operation
	} else {
		ck.operationHead = operation
	}
	ck.operationTail = operation
	operation.allListed = true
}

func (ck *CommandKernel) appendRuntimeOperation(operation *commandOperation) {
	if operation == nil || operation.runtimeListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid runtime operation append"))
		return
	}
	operation.runtimePrevious = ck.runtimeTail
	if ck.runtimeTail != nil {
		ck.runtimeTail.runtimeNext = operation
	} else {
		ck.runtimeHead = operation
	}
	ck.runtimeTail = operation
	operation.runtimeListed = true
}

func (ck *CommandKernel) unlinkRuntimeOperation(operation *commandOperation) {
	if operation == nil || !operation.runtimeListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid runtime operation removal"))
		return
	}
	if operation.runtimePrevious != nil {
		operation.runtimePrevious.runtimeNext = operation.runtimeNext
	} else {
		ck.runtimeHead = operation.runtimeNext
	}
	if operation.runtimeNext != nil {
		operation.runtimeNext.runtimePrevious = operation.runtimePrevious
	} else {
		ck.runtimeTail = operation.runtimePrevious
	}
	operation.runtimePrevious = nil
	operation.runtimeNext = nil
	operation.runtimeListed = false
}

func (ck *CommandKernel) observeRuntimeOperations() {
	if ck == nil || ck.runtimeObserver == nil {
		return
	}
	ck.runtimeObserver.SetRuntimeGauge(lifecycle.RuntimeGaugeOperationsActive, len(ck.operations))
	ck.runtimeObserver.SetRuntimeGauge(lifecycle.RuntimeGaugeFunctionInvocationsActive, ck.functionOperations)
	var oldest time.Time
	if ck.runtimeHead != nil {
		oldest = ck.runtimeHead.runtimeStarted
	}
	ck.runtimeObserver.SetRuntimeTimestamp(lifecycle.RuntimeTimestampOldestOperation, oldest)
}

func (ck *CommandKernel) unlinkOperation(operation *commandOperation) {
	if operation == nil || !operation.allListed {
		ck.run.Dirty(errors.New("jobmgr kernel: invalid operation-list removal"))
		return
	}
	if operation.allPrevious != nil {
		operation.allPrevious.allNext = operation.allNext
	} else {
		ck.operationHead = operation.allNext
	}
	if operation.allNext != nil {
		operation.allNext.allPrevious = operation.allPrevious
	} else {
		ck.operationTail = operation.allPrevious
	}
	operation.allPrevious = nil
	operation.allNext = nil
	operation.allListed = false
}

func (ck *CommandKernel) completeOperationUID(operation *commandOperation, tombstone bool) error {
	if operation.uidCompleted {
		return errors.New("jobmgr kernel: operation UID completed twice")
	}
	if err := ck.uids.Complete(operation.UID, tombstone, ck.clock.Now()); err != nil {
		return err
	}
	operation.uidCompleted = true
	return nil
}

func (ck *CommandKernel) unlinkQueued(operation *commandOperation) {
	if operation.Child == lifecycle.ChildDeadlineStartPending {
		ck.run.Dirty(errors.New("jobmgr kernel: required deadline start was unlinked without abandonment"))
		return
	}
	lane := operation.lane
	if lane.ready {
		ck.ready[sourceIndex(lane.source)].remove(lane)
	}
	if operation.fenceBlocked {
		if err := ck.removeCompositeFenceBlocked(operation); err != nil {
			ck.run.Dirty(err)
		}
	}
	if operation.taskRequest.Valid() {
		var err error
		if operation.plan.Transaction != nil {
			var outcome lifecycle.TaskOutcome
			outcome, err = ck.tasks.CancelPendingOutcome(operation.taskRequest)
			if err == nil {
				err = ck.restoreTransactionOutcome(operation, outcome)
			}
		} else {
			err = ck.tasks.CancelPending(operation.taskRequest)
		}
		if err != nil {
			ck.run.Dirty(err)
		} else {
			delete(ck.tasksByRequest, operation.taskRequest)
			operation.taskRequest = lifecycle.TaskRequestRef{}
		}
	}
	if operation.claimsInherited {
		operation.claimsHeld = false
	} else if operation.claimsHeld {
		for _, granted := range ck.releaseClaims(operation) {
			ck.markReady(granted.lane)
		}
	} else if ck.claims.waiting(operation) {
		granted, err := ck.claims.cancel(operation)
		if err != nil {
			ck.run.Dirty(err)
		}
		for _, grantedOperation := range granted {
			ck.markReady(grantedOperation.lane)
		}
	} else if operation.claimRegistered {
		if err := ck.claims.abandon(operation); err != nil {
			ck.run.Dirty(err)
		}
	}
	if lane.head == operation {
		ck.removeHead(lane)
	} else {
		ck.unlink(operation)
	}
	if operation.State < lifecycle.OperationDisposing {
		_ = operation.Advance(lifecycle.OperationDisposing)
	}
	if lane.active == nil && lane.head != nil {
		ck.markReady(lane)
	}
}

func (ck *CommandKernel) releaseClaims(operation *commandOperation) []*commandOperation {
	if operation.claimsInherited {
		operation.claimsHeld = false
		return nil
	}
	if !operation.claimsHeld {
		return nil
	}
	granted, err := ck.claims.release(operation)
	if err != nil {
		ck.run.Dirty(err)
		return nil
	}
	return granted
}

func (ck *CommandKernel) removeHead(lane *commandLane) {
	operation := lane.head
	if operation == nil {
		return
	}
	ck.removingLaneOperation(lane, operation)
	lane.head = operation.next
	if lane.head != nil {
		lane.head.previous = nil
	} else {
		lane.tail = nil
	}
	operation.previous = nil
	operation.next = nil
}

func (ck *CommandKernel) unlink(operation *commandOperation) {
	ck.removingLaneOperation(operation.lane, operation)
	if operation.previous != nil {
		operation.previous.next = operation.next
	}
	if operation.next != nil {
		operation.next.previous = operation.previous
	}
	if operation.lane.tail == operation {
		operation.lane.tail = operation.previous
	}
	operation.previous = nil
	operation.next = nil
}

func (ck *CommandKernel) removingLaneOperation(lane *commandLane, operation *commandOperation) {
	if lane == nil || operation == nil || lane.continuationTail != operation {
		return
	}
	previous := operation.previous
	if previous != nil && previous.parent != nil {
		lane.continuationTail = previous
	} else {
		lane.continuationTail = nil
	}
}

func (ck *CommandKernel) markReady(lane *commandLane) {
	if lane == nil || lane.active != nil || lane.head == nil || lane.head.fenceBlocked || ck.claims.waiting(lane.head) {
		return
	}
	lane.source = lane.head.Source
	index := sourceIndex(lane.source)
	ck.ready[index].push(lane)
}

func (ck *CommandKernel) nextReadyLane() *commandLane {
	first := sourceIndex(ck.nextSource)
	second := 1 - first
	if lane := ck.ready[first].pop(); lane != nil {
		ck.nextSource = otherSource(ck.nextSource)
		return lane
	}
	if lane := ck.ready[second].pop(); lane != nil {
		ck.nextSource = otherSource(lane.source)
		return lane
	}
	return nil
}

func (ck *CommandKernel) nextDeadline() time.Time {
	if ck.deadlines.Len() == 0 {
		return time.Time{}
	}
	return ck.deadlines[0].when
}
