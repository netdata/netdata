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

func (ck *CommandKernel) prepareJobPlan(request Request) (WorkPlan, error) {
	plan, err := ck.jobPlanner.Plan(request)
	if err != nil {
		return WorkPlan{}, err
	}
	return prepareOwnedJobPlan(request, plan)
}

func prepareOwnedJobPlan(request Request, plan WorkPlan) (WorkPlan, error) {
	plan.Claims = slices.Clone(plan.Claims)
	plan.ReadClaims = slices.Clone(plan.ReadClaims)
	if plan.Resource != nil {
		resource := *plan.Resource
		plan.Resource = &resource
	}
	if plan.Transaction != nil {
		transaction := *plan.Transaction
		plan.Transaction = &transaction
	}
	if plan.Capability != nil {
		capability := *plan.Capability
		plan.Capability = &capability
	}
	if err := plan.validate(); err != nil {
		return WorkPlan{}, err
	}
	if plan.Resource != nil && plan.Resource.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: resource identity differs from lane")
	}
	if plan.Transaction != nil && plan.Transaction.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: transaction identity differs from lane")
	}
	if plan.Capability != nil && plan.Capability.ID != request.LaneKey {
		return WorkPlan{}, errors.New("jobmgr kernel: capability identity differs from lane")
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
		if request.InputBodyToken != 0 {
			return ck.abortRequestInputBodyWith(
				request,
				errors.New(
					"jobmgr composite: child cannot own parser input",
				),
			)
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
		plan.ReadClaims = slices.Clone(plan.ReadClaims)
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
	claims, err := normalizeAuthorityClaimModes(plan.Claims, plan.ReadClaims)
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
	if plan.Resource != nil || plan.Transaction != nil {
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
	if resource := plan.Resource; resource != nil {
		if lane != nil && lane.transactionPlanned != 0 {
			_ = ck.uids.Complete(request.UID, false, now)
			_ = ck.abortRequestInputBody(request)
			return errors.New(
				"jobmgr kernel: internal resource plan overlaps a public resource transaction",
			)
		}
		switch resource.Action {
		case ResourceInstall:
			if lane != nil && (lane.installPlanned || ((lane.current != nil || lane.currentIdentity.Valid()) && !lane.stopPlanned && !lane.currentStopping && !lane.retiringIdentity.Valid())) {
				_ = ck.uids.Complete(request.UID, false, now)
				_ = ck.abortRequestInputBody(request)
				return errors.New("jobmgr kernel: install is not sequenced after an exact stop")
			}
		case ResourceStop:
			if lane == nil || lane.stopPlanned || lane.currentStopping || lane.retiringIdentity.Valid() ||
				(lane.current == nil && !lane.currentIdentity.Valid() && !lane.installPlanned) {
				err := errors.Join(ck.uids.Complete(request.UID, false, now), ck.abortRequestInputBody(request))
				if err == nil {
					if submissionResult != nil {
						submissionResult <- nil
					}
					if terminalResult != nil {
						terminalResult <- nil
					}
				}
				return err
			}
		}
	}
	if plan.Transaction != nil && lane != nil &&
		(lane.installPlanned ||
			lane.stopPlanned ||
			lane.currentStopping ||
			lane.retiringIdentity.Valid()) {
		_ = ck.uids.Complete(request.UID, false, now)
		_ = ck.abortRequestInputBody(request)
		return errors.New(
			"jobmgr kernel: resource transaction overlaps internal resource authority",
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
	charge, err := operationAdmissionBytes(request, plan)
	if err != nil {
		_ = ck.uids.Complete(request.UID, false, now)
		ck.releaseUnusedLane(lane)
		_ = ck.abortRequestInputBody(request)
		return err
	}
	admissionLane := lifecycle.AdmissionLaneRef{Slot: lane.slot, Generation: lane.generation}
	requested := lifecycle.AdmissionRequestResult{}
	if request.InputBodyToken != 0 {
		requested = ck.admission.TransferInputBody(ck.run.Generation(), request.InputBodyToken, admissionLane, charge, request.PayloadCapacity)
	} else if parent != nil {
		if err := ck.beginCompositeFence(parent); err != nil {
			ck.releaseUnusedLane(lane)
			return ck.abortRequestInputBodyWith(
				request,
				err,
				ck.uids.Complete(request.UID, false, now),
			)
		}
		requested = ck.admission.GrantCompositeProgress(
			ck.run.Generation(),
			parent.admission,
			admissionLane,
			charge,
		)
	} else {
		requested = ck.admission.RequestOrdinary(ck.run.Generation(), admissionLane, charge)
	}
	if requested.Rejected != nil {
		ck.releaseUnusedLane(lane)
		return ck.abortRequestInputBodyWith(
			request,
			requested.Rejected,
			ck.uids.Complete(request.UID, false, now),
		)
	}
	request.InputBodyToken = 0
	operation := &commandOperation{
		OperationGeneration: operationGeneration, request: request, plan: plan, claims: claims,
		functionInvocation: functionInvocation,
		admission:          requested.Ref, admissionBase: charge, deadline: deadlineEntry{index: -1},
		submissionContext: submissionContext, submissionResult: submissionResult, terminalResult: terminalResult,
		parent: parent, claimsInherited: parent != nil,
		compositeRollback: rollback,
		shutdownChild: parent != nil &&
			ck.shutdownPhase == commandShutdownActionDrain,
		runtimeStarted: now,
	}
	if parent == nil {
		prepareClaimEdges(operation, claims)
		if err := ck.claims.register(operation); err != nil {
			_ = ck.admission.CancelWaiting(requested.Ref)
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
	ck.byAdmission[requested.Ref] = operation
	releaseFunctionInvocation = false
	if resource := plan.Resource; resource != nil {
		switch resource.Action {
		case ResourceInstall:
			lane.installPlanned = true
		case ResourceStop:
			lane.stopPlanned = true
		}
	}
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
	if parent != nil {
		operation.admitted = true
		ck.settleSubmission(operation, nil)
	}
	if operation.admitted && lane.active == nil && lane.head == operation {
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

func (ck *CommandKernel) serviceAdmissions(quantum int) bool {
	if ck.admissionServiceGate != nil {
		select {
		case <-ck.admissionServiceGate:
		default:
			return false
		}
	}
	var grants [4]lifecycle.AdmissionGrant
	count, more, err := ck.admission.TakeGrants(quantum, &grants)
	if err != nil {
		ck.run.Dirty(err)
		return false
	}
	for _, grant := range grants[:count] {
		if grant.Kind == lifecycle.ReservationInputBodyGrowth {
			select {
			case ck.inputBodyGrants <- grant:
			default:
				ck.run.Dirty(errors.New("jobmgr kernel: input body grant gate is full"))
				return false
			}
			continue
		}
		operation := ck.byAdmission[grant.Ref]
		if operation == nil || operation.admission != grant.Ref {
			ck.run.Dirty(errors.New("jobmgr kernel: invalid admission grant"))
			return false
		}
		switch grant.Kind {
		case lifecycle.ReservationOrdinary:
			if operation.admitted {
				ck.run.Dirty(errors.New("jobmgr kernel: duplicate initial admission grant"))
				return false
			}
			if ck.compositeFenceConflicts(operation.claims) {
				wake, suspendErr :=
					ck.admission.SuspendOrdinary(
						grant.Ref,
						operation.request.PayloadCapacity,
					)
				if suspendErr != nil {
					ck.run.Dirty(suspendErr)
					return false
				}
				if err := ck.blockOnCompositeFence(
					operation,
				); err != nil {
					ck.run.Dirty(err)
					return false
				}
				more = more || wake
				continue
			}
			operation.admitted = true
			ck.settleSubmission(operation, nil)
			if operation.lane.active == nil && operation.lane.head == operation {
				ck.markReady(operation.lane)
			}
		case lifecycle.ReservationOrdinaryGrowth:
			if !operation.admitted ||
				operation.Child !=
					lifecycle.ChildResultReady {
				ck.run.Dirty(errors.New("jobmgr kernel: invalid result growth grant"))
				return false
			}
			if !operation.resultGrowthWaiting {
				ck.run.Dirty(
					errors.New(
						"jobmgr kernel: unowned ordinary growth grant",
					),
				)
				return false
			}
			operation.resultGrowthWaiting = false
			if err := ck.sendEncodeAction(operation); err != nil {
				ck.run.Dirty(err)
				return false
			}
		default:
			ck.run.Dirty(errors.New("jobmgr kernel: unexpected admission grant kind"))
			return false
		}
	}
	return more
}

func (ck *CommandKernel) settleSubmission(operation *commandOperation, err error) {
	if operation == nil || operation.submissionResult == nil {
		return
	}
	operation.submissionResult <- err
	operation.submissionResult = nil
	operation.submissionContext = nil
}
