// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"container/heap"
	"context"
	"errors"
	"fmt"
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func prepareOwnedJobPlan(request Request, plan WorkPlan) (WorkPlan, error) {
	plan.Claims = slices.Clone(plan.Claims)
	if plan.Transaction != nil {
		transaction := *plan.Transaction
		plan.Transaction = &transaction
	}
	if err := plan.validate(); err != nil {
		return WorkPlan{}, err
	}
	if plan.Transaction != nil && plan.Transaction.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: transaction identity differs from lane")
	}
	return plan, nil
}

func (ck *CommandKernel) admitSubmission(
	request Request,
	plan WorkPlan,
	submissionContext context.Context,
	submissionResult,
	terminalResult chan error,
	composite *kernelCompositeScope,
	rollback bool,
) error {
	var parent *commandOperation
	if composite != nil {
		var err error
		parent, err = ck.validateCompositeAdmission(
			composite,
			plan,
			rollback,
		)
		if err != nil {
			return ck.abortRequestInputBodyWith(request, err)
		}
	} else if rollback {
		return ck.abortRequestInputBodyWith(
			request,
			errors.New("jobmgr kernel: rollback child has no parent"),
		)
	} else if !ck.run.Admitting() {
		return ck.rejectClosedAdmission(request)
	}
	now := ck.clock.Now()
	if err := ck.uids.Admit(request.UID, now); err != nil {
		if ck.runtimeObserver != nil {
			ck.runtimeObserver.AddRuntimeCounter(
				lifecycle.RuntimeCounterDuplicateUIDRejected,
				1,
			)
		}
		return ck.abortRequestInputBodyWith(request, err)
	}
	var functionInvocation FunctionInvocationRef
	var functionResourceID string
	var laneID commandLaneKey
	if request.Source == lifecycle.SourceFunction {
		decision, err := ck.functionCatalog.ResolveAndAcquire(FunctionLookup{
			UID: request.UID, Route: request.Route, Args: request.Args,
			Payload: request.Payload, ContentType: request.ContentType,
			Permissions: request.Permissions, CallerSource: request.CallerSource,
			Timeout: request.Timeout, HasPayload: request.HasPayload,
		})
		if err != nil {
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return err
		}
		if err := decision.validate(); err != nil {
			if decision.Lease.Valid() {
				err = errors.Join(err, ck.releaseFunctionInvocation(decision.Lease))
			}
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return err
		}
		if decision.Rejected != 0 {
			if err := errors.Join(
				ck.uids.Complete(request.UID, false, now),
				ck.abortRequestInputBody(request),
			); err != nil {
				return err
			}
			return preAdmissionControl{
				status: decision.Rejected,
				cause: fmt.Errorf(
					"jobmgr kernel: Function catalog rejected route %q",
					request.Route,
				),
			}
		}
		plan = decision.Plan
		plan.Claims = slices.Clone(plan.Claims)
		functionInvocation = decision.Lease
		functionResourceID = decision.ResourceID
		request.LaneKey = request.Route
		if functionResourceID != "" {
			request.LaneKey = functionResourceID
		}
	}
	releaseFunctionInvocation := functionInvocation.Valid()
	defer func() {
		if releaseFunctionInvocation {
			if err := ck.releaseFunctionInvocation(functionInvocation); err != nil {
				ck.run.Dirty(err)
			}
		}
	}()
	claims, err := normalizeAuthorityClaims(plan.Claims)
	if err != nil {
		_ = ck.uids.Complete(request.UID, false, now)
		_ = ck.abortRequestInputBody(request)
		return err
	}
	if request.Source == lifecycle.SourceJobManager {
		laneID = commandLaneKey{source: request.Source, key: request.LaneKey}
	} else if functionResourceID != "" {
		laneID = resourceCommandLaneKey(functionResourceID)
	} else {
		laneID = commandLaneKey{
			source:             request.Source,
			functionInvocation: functionInvocation,
		}
	}
	if plan.Transaction != nil {
		laneID = resourceCommandLaneKey(request.LaneKey)
	}
	if parent != nil &&
		parent.lane != nil &&
		parent.lane.mapKey == laneID {
		return ck.abortRequestInputBodyWith(
			request,
			errors.New(
				"jobmgr composite: child cannot use its active parent lane",
			),
			ck.uids.Complete(request.UID, false, now),
		)
	}
	lane := ck.lanes[laneID]
	if plan.Transaction != nil && lane != nil &&
		(lane.currentStopping || lane.retiringIdentity.Valid()) {
		_ = ck.uids.Complete(request.UID, false, now)
		_ = ck.abortRequestInputBody(request)
		return errors.New(
			"jobmgr kernel: resource transaction overlaps retiring resource authority",
		)
	}
	if lane == nil {
		lane, err = ck.allocateLane(laneID, request)
		if err != nil {
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return err
		}
	}
	ck.nextID++
	operationGeneration, err := lifecycle.NewOperation(ck.nextID, request.UID, request.Source, request.LaneKey, !plan.NoResponse)
	if err != nil {
		_ = ck.uids.Complete(request.UID, false, now)
		ck.releaseUnusedLane(lane)
		_ = ck.abortRequestInputBody(request)
		return err
	}
	if parent != nil {
		if err := ck.beginCompositeFence(parent); err != nil {
			ck.releaseUnusedLane(lane)
			return ck.abortRequestInputBodyWith(
				request,
				err,
				ck.uids.Complete(request.UID, false, now),
			)
		}
	}
	operation := &commandOperation{
		OperationGeneration: operationGeneration, request: request, plan: plan, claims: claims,
		functionInvocation: functionInvocation,
		deadline:           deadlineEntry{index: -1},
		submissionContext:  submissionContext, submissionResult: submissionResult, terminalResult: terminalResult,
		parent: parent, claimsInherited: parent != nil,
		compositeRollback: rollback,
		shutdownChild: parent != nil &&
			ck.shutdownPhase == commandShutdownActionDrain,
		runtimeStarted: now,
	}
	if parent == nil {
		prepareClaimEdges(operation, claims)
		if err := ck.claims.register(operation); err != nil {
			_ = ck.uids.Complete(request.UID, false, now)
			ck.releaseUnusedLane(lane)
			return err
		}
	}
	operation.lane = lane
	lane.owners++
	if parent != nil {
		ck.insertCompositeOperation(lane, operation)
		parent.activeChild = operation
	} else if lane.tail != nil {
		lane.tail.next = operation
		operation.previous = lane.tail
		lane.tail = operation
	} else {
		lane.head = operation
		lane.tail = operation
	}
	_ = operation.Advance(lifecycle.OperationQueued)
	ck.operations[request.UID] = operation
	ck.appendOperation(operation)
	ck.appendRuntimeOperation(operation)
	if request.Source == lifecycle.SourceFunction {
		ck.functionOperations++
	}
	if ck.runtimeObserver != nil {
		ck.runtimeObserver.AddRuntimeCounter(
			lifecycle.RuntimeCounterOperationsAdmitted,
			1,
		)
	}
	ck.observeRuntimeOperations()
	releaseFunctionInvocation = false
	if plan.Transaction != nil {
		lane.transactionPlanned++
	}
	if plan.Transaction != nil &&
		plan.Transaction.PrepareComposite != nil {
		operation.composite = newKernelCompositeScope(ck, operation)
	}
	if !request.Deadline.IsZero() {
		operation.deadline = deadlineEntry{when: request.Deadline, operation: operation, index: -1}
		heap.Push(&ck.deadlines, &operation.deadline)
	}
	if parent == nil && ck.compositeFenceConflicts(operation.claims) {
		if err := ck.blockOnCompositeFence(operation); err != nil {
			ck.run.Dirty(err)
			ck.settleSubmission(operation, err)
			ck.cancelOperation(operation.UID)
			return nil
		}
	}
	ck.settleSubmission(operation, nil)
	if !operation.fenceBlocked && lane.active == nil && lane.head == operation {
		ck.markReady(lane)
	}
	return nil
}

func (ck *CommandKernel) rejectClosedAdmission(request Request) error {
	if ck.runtimeObserver != nil {
		ck.runtimeObserver.AddRuntimeCounter(
			lifecycle.RuntimeCounterShutdownRejected,
			1,
		)
	}
	closedErr := errors.New("jobmgr kernel: admission closed")
	if err := ck.abortRequestInputBody(request); err != nil {
		ck.run.Dirty(err)
		return errors.Join(ck.stoppingError(), err)
	}
	if ck.run.IsStopping() {
		return ck.run.StoppingCause()
	}
	return closedErr
}

func (ck *CommandKernel) settleSubmission(operation *commandOperation, err error) {
	if operation == nil || operation.submissionResult == nil {
		return
	}
	operation.submissionResult <- err
	operation.submissionResult = nil
	operation.submissionContext = nil
}
