// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"slices"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type kernelCompositeScope struct {
	kernel *CommandKernel    // owning kernel
	parent *commandOperation // parent composite operation
	closed atomic.Bool       // scope closed to new child submissions
	fenced bool              // the composite fence is installed

	rollbackMu     sync.Mutex         // guards the lazily-created rollback context
	rollbackCtx    context.Context    // run-owned bounded rollback context
	rollbackCancel context.CancelFunc // cancels rollbackCtx
}

func newKernelCompositeScope(
	kernel *CommandKernel,
	parent *commandOperation,
) *kernelCompositeScope {
	return &kernelCompositeScope{kernel: kernel, parent: parent}
}

func (kcs *kernelCompositeScope) SubmitPreparedAndWait(
	ctx context.Context,
	request Request,
	plan WorkPlan,
) error {
	return kcs.submitAndWait(ctx, request, plan, false)
}

func (kcs *kernelCompositeScope) SubmitRollbackAndWait(
	request Request,
	plan WorkPlan,
) error {
	ctx, err := kcs.RollbackContext()
	if err != nil {
		return err
	}
	return kcs.submitAndWait(ctx, request, plan, true)
}

func (kcs *kernelCompositeScope) RollbackContext() (
	context.Context,
	error,
) {
	if kcs == nil ||
		kcs.kernel == nil ||
		kcs.parent == nil ||
		kcs.closed.Load() {
		return nil, errors.New(
			"jobmgr composite: rollback outside active parent",
		)
	}
	kcs.rollbackMu.Lock()
	defer kcs.rollbackMu.Unlock()
	if kcs.closed.Load() {
		return nil, errors.New(
			"jobmgr composite: rollback outside active parent",
		)
	}
	if kcs.rollbackCtx != nil {
		return kcs.rollbackCtx, nil
	}
	ctx, cancel, err := kcs.kernel.run.NewRollbackContext()
	if err != nil {
		return nil, err
	}
	kcs.rollbackCtx = ctx
	kcs.rollbackCancel = cancel
	return ctx, nil
}

func (kcs *kernelCompositeScope) close() {
	if kcs == nil || !kcs.closed.CompareAndSwap(false, true) {
		return
	}
	kcs.rollbackMu.Lock()
	if kcs.rollbackCancel != nil {
		kcs.rollbackCancel()
	}
	kcs.rollbackCtx = nil
	kcs.rollbackCancel = nil
	kcs.rollbackMu.Unlock()
}

func (kcs *kernelCompositeScope) submitAndWait(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	rollback bool,
) error {
	if kcs == nil ||
		kcs.kernel == nil ||
		kcs.parent == nil ||
		kcs.closed.Load() ||
		ctx == nil {
		return errors.New("jobmgr composite: invalid child submission")
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if request.Source != lifecycle.SourceJobManager ||
		!plan.NoResponse {
		return errors.New(
			"jobmgr composite: child must be a response-free Job Manager command",
		)
	}
	request.Deadline = earliestDeadline(
		request.Deadline,
		contextDeadline(ctx),
	)
	if !rollback {
		request.Deadline = earliestDeadline(
			request.Deadline,
			kcs.parent.request.Deadline,
		)
	}
	if err := request.Validate(); err != nil {
		return errors.Join(
			err,
			kcs.kernel.abortRequestInputBody(request),
		)
	}
	request.Args = slices.Clone(request.Args)
	owned, err := prepareOwnedJobPlan(request, plan)
	if err != nil {
		return errors.Join(
			err,
			kcs.kernel.abortRequestInputBody(request),
		)
	}
	result := make(chan error, 1)
	terminal := make(chan error, 1)
	if err := kcs.kernel.enqueueSubmission(
		ctx,
		request.Source,
		submission{
			request:   request,
			plan:      owned,
			context:   ctx,
			composite: kcs,
			rollback:  rollback,
			result:    result,
			terminal:  terminal,
		},
	); err != nil {
		return errors.Join(
			err,
			kcs.kernel.abortRequestInputBody(request),
		)
	}

	accepted, err := kcs.waitChildAdmission(
		ctx,
		request.UID,
		result,
	)
	if !accepted {
		return err
	}
	return errors.Join(
		err,
		kcs.waitChildTerminal(
			ctx,
			request.UID,
			terminal,
			rollback,
		),
	)
}

func (kcs *kernelCompositeScope) waitChildAdmission(
	ctx context.Context,
	uid string,
	result <-chan error,
) (bool, error) {
	select {
	case err := <-result:
		return err == nil, err
	case <-ctx.Done():
		cause := context.Cause(ctx)
		select {
		case kcs.kernel.cancel <- uid:
		case err := <-result:
			return err == nil, errors.Join(cause, err)
		case <-kcs.kernel.done:
			return false, errors.Join(cause, ErrStopped)
		}
		select {
		case err := <-result:
			return err == nil, errors.Join(cause, err)
		case <-kcs.kernel.done:
			return false, errors.Join(cause, ErrStopped)
		}
	case <-kcs.kernel.done:
		return false, ErrStopped
	}
}

func (kcs *kernelCompositeScope) waitChildTerminal(
	ctx context.Context,
	uid string,
	terminal <-chan error,
	rollback bool,
) error {
	var cancellation error
	done := ctx.Done()
	for {
		select {
		case err := <-terminal:
			return errors.Join(cancellation, err)
		case <-done:
			cancellation = context.Cause(ctx)
			done = nil
			if rollback {
				kcs.kernel.run.Dirty(
					errors.Join(
						errors.New(
							"jobmgr composite: rollback deadline exceeded",
						),
						cancellation,
					),
				)
				kcs.kernel.NotifyControlReady()
			}
			select {
			case kcs.kernel.cancel <- uid:
			case err := <-terminal:
				return errors.Join(cancellation, err)
			case <-kcs.kernel.done:
				return errors.Join(cancellation, ErrStopped)
			}
		case <-kcs.kernel.done:
			return errors.Join(cancellation, ErrStopped)
		}
	}
}

func earliestDeadline(left, right time.Time) time.Time {
	switch {
	case left.IsZero():
		return right
	case right.IsZero() || left.Before(right):
		return left
	default:
		return right
	}
}

func contextDeadline(ctx context.Context) time.Time {
	if ctx == nil {
		return time.Time{}
	}
	deadline, _ := ctx.Deadline()
	return deadline
}

type preparedCompositeBridge struct {
	transaction PreparedCompositeResourceTransaction
	scope       *kernelCompositeScope
}

func (pcb *preparedCompositeBridge) Scope() (
	scope lifecycle.ResourceTransactionScope,
) {
	if pcb == nil || pcb.transaction == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	return pcb.transaction.Scope()
}

func (pcb *preparedCompositeBridge) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	if pcb == nil ||
		pcb.transaction == nil ||
		pcb.scope == nil {
		return lifecycle.AppliedResourceTransaction{},
			errors.New("jobmgr composite: invalid prepared bridge")
	}
	defer pcb.scope.close()
	return pcb.transaction.ApplyComposite(ctx, pcb.scope)
}

func (pcb *preparedCompositeBridge) Dispose(
	ctx context.Context,
) (lifecycle.ReadyResource, error) {
	if pcb == nil ||
		pcb.transaction == nil ||
		pcb.scope == nil {
		return nil, errors.New(
			"jobmgr composite: invalid prepared bridge",
		)
	}
	defer pcb.scope.close()
	return pcb.transaction.Dispose(ctx)
}

func (ck *CommandKernel) validateCompositeAdmission(
	scope *kernelCompositeScope,
	plan WorkPlan,
	rollback bool,
) (*commandOperation, error) {
	if scope == nil ||
		scope.kernel != ck ||
		scope.parent == nil ||
		scope.closed.Load() {
		return nil, errors.New(
			"jobmgr composite: stale parent scope",
		)
	}
	parent := scope.parent
	if ck.operations[parent.UID] != parent ||
		parent.composite != scope ||
		parent.activeChild != nil ||
		!parent.claimsHeld ||
		parent.plan.Transaction == nil ||
		parent.plan.Transaction.PrepareComposite == nil {
		return nil, errors.New(
			"jobmgr composite: parent is not accepting a child",
		)
	}
	if ck.shutdownPhase == commandShutdownCleanupDrain ||
		(ck.shutdownPhase == commandShutdownActionDrain &&
			!parent.ownershipChain) {
		return nil, errors.New(
			"jobmgr composite: continuation authority is closed",
		)
	}
	if !rollback &&
		(parent.cancelled || parent.TimedOut()) &&
		!parent.ownershipChain {
		return nil, errors.New(
			"jobmgr composite: cancelled parent rejected normal child",
		)
	}
	childClaims, err := normalizeAuthorityClaims(plan.Claims)
	if err != nil {
		return nil, err
	}
	if !claimsCoveredByParent(parent.claims, childClaims) {
		return nil, errors.New(
			"jobmgr composite: child claim is outside parent scope",
		)
	}
	return parent, nil
}

func claimsCoveredByParent(
	parent,
	child []string,
) bool {
	parentIndex := 0
	for _, wanted := range child {
		for parentIndex < len(parent) &&
			parent[parentIndex] < wanted {
			parentIndex++
		}
		if parentIndex >= len(parent) ||
			parent[parentIndex] != wanted {
			return false
		}
	}
	return true
}

func (ck *CommandKernel) insertCompositeOperation(
	lane *commandLane,
	operation *commandOperation,
) {
	if lane == nil || operation == nil || operation.parent == nil {
		ck.run.Dirty(
			errors.New("jobmgr composite: invalid lane insertion"),
		)
		return
	}
	anchor := lane.continuationTail
	if anchor == nil {
		anchor = lane.active
	}
	if anchor == nil {
		operation.next = lane.head
		if lane.head != nil {
			lane.head.previous = operation
		} else {
			lane.tail = operation
		}
		lane.head = operation
		lane.continuationTail = operation
		return
	}
	operation.previous = anchor
	operation.next = anchor.next
	if anchor.next != nil {
		anchor.next.previous = operation
	} else {
		lane.tail = operation
	}
	anchor.next = operation
	lane.continuationTail = operation
}

func (ck *CommandKernel) beginCompositeFence(
	parent *commandOperation,
) error {
	if ck == nil || parent == nil || parent.composite == nil ||
		parent.composite.parent != parent {
		return errors.New("jobmgr composite: invalid admission fence parent")
	}
	if parent.composite.fenced {
		return nil
	}
	for _, claim := range parent.claims {
		ck.compositeFenceClaims[claim]++
	}
	parent.composite.fenced = true
	return nil
}

func (ck *CommandKernel) endCompositeFence(
	parent *commandOperation,
) error {
	if ck == nil || parent == nil || parent.composite == nil ||
		!parent.composite.fenced {
		return nil
	}
	for _, claim := range parent.claims {
		use := ck.compositeFenceClaims[claim] - 1
		if use < 0 {
			return errors.New(
				"jobmgr composite: negative admission fence ownership",
			)
		}
		if use == 0 {
			delete(ck.compositeFenceClaims, claim)
		} else {
			ck.compositeFenceClaims[claim] = use
		}
	}
	parent.composite.fenced = false
	ck.compositeFenceGeneration++
	if ck.compositeFenceGeneration == 0 {
		return errors.New(
			"jobmgr composite: admission fence generation wrapped",
		)
	}
	ck.compositeFenceRecheck = ck.compositeFenceHead != nil
	return nil
}

func (ck *CommandKernel) compositeFenceConflicts(
	claims []string,
) bool {
	for _, claim := range claims {
		if ck.compositeFenceClaims[claim] != 0 {
			return true
		}
	}
	return false
}

func (ck *CommandKernel) blockOnCompositeFence(
	operation *commandOperation,
) error {
	if operation == nil || operation.parent != nil ||
		operation.admitted || !operation.admission.Valid() ||
		operation.fenceBlocked ||
		!ck.compositeFenceConflicts(operation.claims) {
		return errors.New(
			"jobmgr composite: invalid admission fence block",
		)
	}
	operation.fencePrevious = ck.compositeFenceTail
	operation.fenceNext = nil
	operation.fenceBlocked = true
	operation.fenceChecked = ck.compositeFenceGeneration
	if ck.compositeFenceTail != nil {
		ck.compositeFenceTail.fenceNext = operation
	} else {
		ck.compositeFenceHead = operation
	}
	ck.compositeFenceTail = operation
	ck.compositeFenceCount++
	return nil
}

func (ck *CommandKernel) removeCompositeFenceBlocked(
	operation *commandOperation,
) error {
	if operation == nil || !operation.fenceBlocked ||
		ck.compositeFenceCount <= 0 {
		return errors.New(
			"jobmgr composite: invalid admission fence removal",
		)
	}
	if operation.fencePrevious != nil {
		operation.fencePrevious.fenceNext = operation.fenceNext
	} else {
		ck.compositeFenceHead = operation.fenceNext
	}
	if operation.fenceNext != nil {
		operation.fenceNext.fencePrevious = operation.fencePrevious
	} else {
		ck.compositeFenceTail = operation.fencePrevious
	}
	operation.fencePrevious = nil
	operation.fenceNext = nil
	operation.fenceBlocked = false
	ck.compositeFenceCount--
	if ck.compositeFenceCount == 0 {
		ck.compositeFenceHead = nil
		ck.compositeFenceTail = nil
		ck.compositeFenceRecheck = false
	}
	return nil
}

func (ck *CommandKernel) serviceCompositeFenceBlocked(
	quantum int,
) bool {
	if !ck.compositeFenceRecheck || quantum <= 0 {
		return false
	}
	for range quantum {
		operation := ck.compositeFenceHead
		if operation == nil {
			ck.compositeFenceRecheck = false
			return false
		}
		if operation.fenceChecked ==
			ck.compositeFenceGeneration {
			ck.compositeFenceRecheck = false
			return false
		}
		if err := ck.removeCompositeFenceBlocked(
			operation,
		); err != nil {
			ck.run.Dirty(err)
			return false
		}
		operation.fenceChecked = ck.compositeFenceGeneration
		if ck.compositeFenceConflicts(operation.claims) {
			if err := ck.blockOnCompositeFence(
				operation,
			); err != nil {
				ck.run.Dirty(err)
				return false
			}
			continue
		}
		if err := ck.admission.ResumeOrdinary(
			operation.admission,
		); err != nil {
			ck.run.Dirty(err)
			return false
		}
	}
	return ck.compositeFenceRecheck
}

func (ck *CommandKernel) completeCompositeChild(
	child *commandOperation,
) {
	if child == nil || child.parent == nil {
		return
	}
	parent := child.parent
	child.parent = nil
	if parent.activeChild != child {
		ck.run.Dirty(
			errors.New(
				"jobmgr composite: terminal child differs from parent ownership",
			),
		)
		return
	}
	parent.activeChild = nil
	if parent.deferredCompletion != nil {
		completion := *parent.deferredCompletion
		parent.deferredCompletion = nil
		ck.completeTask(completion)
	}
	ck.tryDispose(parent)
}
