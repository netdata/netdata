// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func taskClassForOperation(
	operation *commandOperation,
	lane *commandLane,
) lifecycle.TaskClass {
	if operation.Source == lifecycle.SourceJobManager || lane.mapKey.resource {
		return lifecycle.TaskClassFrameworkControl
	}
	return lifecycle.TaskClassGenericFunction
}

func (ck *CommandKernel) scheduleTasks(quantum int) bool {
	for quantum > 0 {
		lane := ck.nextReadyLane()
		if lane == nil {
			return false
		}
		quantum--
		operation := lane.head
		if operation == nil || !operation.admitted || lane.active != nil {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid ready lane"))
			return false
		}
		if ck.shutdownPhase != commandShutdownRunning &&
			(ck.shutdownPhase != commandShutdownActionDrain ||
				operation.parent == nil ||
				!operation.parent.ownershipChain) {
			ck.run.Dirty(
				errors.New(
					"jobmgr kernel: non-continuation task scheduled after shutdown cut",
				),
			)
			return false
		}
		if !operation.claimsHeld {
			if operation.claimsInherited {
				operation.claimsHeld = true
			} else {
				if operation.State < lifecycle.OperationAcquiringClaims {
					_ = operation.Advance(lifecycle.OperationAcquiringClaims)
				}
				granted, err := ck.claims.acquire(operation)
				if err != nil {
					ck.run.Dirty(err)
					return false
				}
				if !granted {
					continue
				}
			}
		}
		if operation.State < lifecycle.OperationReady {
			_ = operation.Advance(lifecycle.OperationReady)
		}
		phaseLimit := uint8(lifecycle.TransactionTaskPhases)
		if operation.Source == lifecycle.SourceFunction {
			phaseLimit = lifecycle.FunctionTaskPhases
		}
		taskPlan := lifecycle.TaskPlan{
			Source: operation.Source, Deadline: operation.request.Deadline,
			MaxPhaseTransitions: phaseLimit, Work: operation.plan.Work, Cleanup: operation.plan.Cleanup,
		}
		if operation.Child == lifecycle.ChildDeadlineStartPending {
			taskPlan.InitialCancellation = context.DeadlineExceeded
		}
		if transaction := operation.plan.Transaction; transaction != nil {
			if lane.currentStopping ||
				lane.retiringIdentity.Valid() ||
				(lane.current == nil) != !lane.currentIdentity.Valid() {
				ck.run.Dirty(
					errors.New(
						"jobmgr kernel: transaction encountered an invalid current resource slot",
					),
				)
				return false
			}
			scope := lifecycle.ResourceTransactionScope{
				ID:      transaction.ID,
				Current: lane.currentIdentity,
			}
			if transaction.AllocateSuccessor {
				ck.nextResourceGeneration++
				generation := ck.nextResourceGeneration
				if generation == 0 {
					ck.run.Dirty(
						errors.New(
							"jobmgr kernel: transaction successor generation wrapped",
						),
					)
					return false
				}
				scope.Successor = lifecycle.ResourceIdentity{
					ID:         transaction.ID,
					Generation: generation,
				}
				operation.resourceGeneration = generation
			}
			operation.transactionScope = scope
			transactionWork := transaction.Prepare
			if transaction.PrepareComposite != nil {
				prepare := transaction.PrepareComposite
				composite := operation.composite
				transactionWork = func(
					ctx context.Context,
					current lifecycle.ReadyResource,
					scope lifecycle.ResourceTransactionScope,
					permit lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					prepared, prepareErr := prepare(
						ctx,
						current,
						scope,
						permit,
					)
					if prepared == nil {
						return nil, prepareErr
					}
					return &preparedCompositeBridge{
						transaction: prepared,
						scope:       composite,
					}, prepareErr
				}
			}
			var err error
			if scope.Successor.Valid() {
				taskPlan, err = lifecycle.NewResourceTransactionPermitTaskPlan(
					operation.Source,
					operation.request.Deadline,
					lifecycle.TransactionTaskPhases,
					ck.admission,
					operation.admission,
					lane.current,
					scope,
					transaction.Permit,
					transactionWork,
				)
			} else {
				taskPlan, err = lifecycle.NewResourceTransactionTaskPlan(
					operation.Source,
					operation.request.Deadline,
					lifecycle.TransactionTaskPhases,
					lane.current,
					scope,
					transactionWork,
				)
			}
			if err != nil {
				ck.run.Dirty(err)
				return false
			}
		}
		requestRef, err := ck.tasks.Enqueue(
			taskClassForOperation(operation, lane),
			taskPlan,
		)
		if err != nil {
			for _, grantedOperation := range ck.releaseClaims(operation) {
				ck.markReady(grantedOperation.lane)
			}
			ck.markReady(lane)
			return false
		}
		if operation.plan.Transaction != nil && operation.transactionScope.Current.Valid() {
			lane.current = nil
			lane.currentStopping = true
		}
		operation.taskRequest = requestRef
		lane.active = operation
		ck.tasksByRequest[requestRef] = operation
	}
	return ck.ready[0].len != 0 || ck.ready[1].len != 0
}

func (ck *CommandKernel) serviceTaskStarts(quantum int) bool {
	var started [lifecycle.TaskStartServiceQuantum]lifecycle.TaskStart
	count, more, dispatchErr := ck.tasks.Dispatch(
		context.Background(),
		quantum,
		&started,
	)
	for _, start := range started[:count] {
		if start.Err != nil {
			ck.rejectTaskStart(start)
			continue
		}
		if cleanupRef, ok := ck.functionCleanupRequests[start.Request]; ok {
			if _, exists := ck.functionCleanupTasks[start.Task]; exists {
				ck.run.Dirty(errors.New("jobmgr kernel: duplicate Function cleanup task slot"))
				return more
			}
			delete(ck.functionCleanupRequests, start.Request)
			ck.functionCleanupTasks[start.Task] = functionCleanupTask{ref: cleanupRef}
			continue
		}
		if ck.shutdownBarrierRequest.Valid() &&
			start.Request == ck.shutdownBarrierRequest {
			if ck.shutdownBarrierTask.Valid() ||
				ck.shutdownBarrierDone ||
				ck.shutdownBarrierFailed {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown barrier start acknowledgement"))
				return more
			}
			ck.shutdownBarrierRequest = lifecycle.TaskRequestRef{}
			ck.shutdownBarrierTask = start.Task
			continue
		}
		if ck.finalizerRequest.Valid() && start.Request == ck.finalizerRequest {
			if ck.finalizerTask.Valid() || ck.finalizerDone || ck.finalizerFailed {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid run finalizer start acknowledgement"))
				return more
			}
			ck.finalizerRequest = lifecycle.TaskRequestRef{}
			ck.finalizerTask = start.Task
			continue
		}
		operation := ck.tasksByRequest[start.Request]
		if operation == nil {
			lane := ck.shutdownRequests[start.Request]
			if lane == nil || lane.shutdownRequest != start.Request ||
				lane.shutdownTask.Valid() || ck.shutdownTasks[start.Task] != nil {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid shutdown task start acknowledgement"))
				return more
			}
			delete(ck.shutdownRequests, start.Request)
			lane.shutdownRequest = lifecycle.TaskRequestRef{}
			lane.shutdownTask = start.Task
			ck.shutdownTasks[start.Task] = lane
			continue
		}
		if operation == nil || operation.taskRequest != start.Request ||
			(operation.Child != lifecycle.ChildNotStarted && operation.Child != lifecycle.ChildDeadlineStartPending) {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid task start acknowledgement"))
			return more
		}
		delete(ck.tasksByRequest, start.Request)
		operation.taskRequest = lifecycle.TaskRequestRef{}
		if err := operation.Advance(lifecycle.OperationRunning); err != nil {
			ck.run.Dirty(err)
			return more
		}
		if err := operation.StartChild(start.Task); err != nil {
			ck.run.Dirty(err)
			return more
		}
		ck.tasksByRef[start.Task] = operation
	}
	if dispatchErr != nil {
		ck.run.Dirty(dispatchErr)
	}
	return more
}

func (ck *CommandKernel) rejectTaskStart(start lifecycle.TaskStart) {
	if cleanupRef, ok := ck.functionCleanupRequests[start.Request]; ok {
		delete(ck.functionCleanupRequests, start.Request)
		completeErr := errors.Join(errors.New("jobmgr kernel: Function cleanup task start rejected"), start.Err)
		ck.run.Dirty(errors.Join(completeErr, ck.functionCatalog.CompleteCleanup(cleanupRef)))
		return
	}
	if ck.shutdownBarrierRequest.Valid() &&
		start.Request == ck.shutdownBarrierRequest {
		ck.shutdownBarrierRequest = lifecycle.TaskRequestRef{}
		ck.shutdownBarrierFailed = true
		ck.run.Dirty(errors.Join(
			errors.New("jobmgr kernel: shutdown barrier task start rejected"),
			start.Err,
		))
		return
	}
	operation := ck.tasksByRequest[start.Request]
	if !errors.Is(start.Err, lifecycle.ErrLongLivedRecordCapacity) || operation == nil ||
		operation.taskRequest != start.Request || operation.Child != lifecycle.ChildNotStarted {
		ck.run.Dirty(errors.Join(errors.New("jobmgr kernel: invalid task start rejection"), start.Err))
		return
	}
	if operation.plan.Transaction != nil {
		if err := ck.restoreTransactionOutcome(operation, start.Outcome); err != nil {
			ck.run.Dirty(errors.Join(
				errors.New("jobmgr kernel: rejected transaction start lost current resource"),
				err,
			))
			return
		}
	} else if start.Outcome.Kind() != lifecycle.TaskOutcomeNone {
		ck.run.Dirty(
			errors.New("jobmgr kernel: rejected non-transaction start returned an outcome"),
		)
		return
	}
	delete(ck.tasksByRequest, start.Request)
	operation.taskRequest = lifecycle.TaskRequestRef{}
	operation.terminalErr = errors.Join(operation.terminalErr, start.Err)
	ck.unlinkQueued(operation, start.Err)
	if operation.Response != lifecycle.ResponseNotRequired {
		ck.enqueueControl(operation, lifecycle.ControlUnavailable)
	}
	ck.tryDispose(operation)
}
