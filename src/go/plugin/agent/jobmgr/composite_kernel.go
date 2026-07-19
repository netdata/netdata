// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

type kernelCompositeScope struct {
	kernel *CommandKernel
	parent *commandOperation
	closed atomic.Bool
	fenced bool

	rollbackMu     sync.Mutex
	rollbackCtx    context.Context
	rollbackCancel context.CancelFunc
}

func newKernelCompositeScope(
	kernel *CommandKernel,
	parent *commandOperation,
) *kernelCompositeScope {
	return &kernelCompositeScope{kernel: kernel, parent: parent}
}

func (scope *kernelCompositeScope) SubmitPreparedAndWait(
	ctx context.Context,
	request Request,
	plan WorkPlan,
) error {
	return scope.submitAndWait(ctx, request, plan, false)
}

func (scope *kernelCompositeScope) SubmitRollbackAndWait(
	request Request,
	plan WorkPlan,
) error {
	ctx, err := scope.RollbackContext()
	if err != nil {
		return err
	}
	return scope.submitAndWait(ctx, request, plan, true)
}

func (scope *kernelCompositeScope) RollbackContext() (
	context.Context,
	error,
) {
	if scope == nil ||
		scope.kernel == nil ||
		scope.parent == nil ||
		scope.closed.Load() {
		return nil, errors.New(
			"jobmgr composite: rollback outside active parent",
		)
	}
	scope.rollbackMu.Lock()
	defer scope.rollbackMu.Unlock()
	if scope.closed.Load() {
		return nil, errors.New(
			"jobmgr composite: rollback outside active parent",
		)
	}
	if scope.rollbackCtx != nil {
		return scope.rollbackCtx, nil
	}
	ctx, cancel, err := scope.kernel.run.NewRollbackContext()
	if err != nil {
		return nil, err
	}
	scope.rollbackCtx = ctx
	scope.rollbackCancel = cancel
	return ctx, nil
}

func (scope *kernelCompositeScope) close() {
	if scope == nil || !scope.closed.CompareAndSwap(false, true) {
		return
	}
	scope.rollbackMu.Lock()
	if scope.rollbackCancel != nil {
		scope.rollbackCancel()
	}
	scope.rollbackCtx = nil
	scope.rollbackCancel = nil
	scope.rollbackMu.Unlock()
}

func (scope *kernelCompositeScope) submitAndWait(
	ctx context.Context,
	request Request,
	plan WorkPlan,
	rollback bool,
) error {
	if scope == nil ||
		scope.kernel == nil ||
		scope.parent == nil ||
		scope.closed.Load() ||
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
			scope.parent.request.Deadline,
		)
	}
	if err := request.Validate(); err != nil {
		return errors.Join(
			err,
			scope.kernel.abortRequestInputBody(request),
		)
	}
	request.Args = append([]string(nil), request.Args...)
	owned, err := prepareOwnedJobPlan(request, plan)
	if err != nil {
		return errors.Join(
			err,
			scope.kernel.abortRequestInputBody(request),
		)
	}
	result := make(chan error, 1)
	terminal := make(chan error, 1)
	if err := scope.kernel.enqueueSubmission(
		ctx,
		request.Source,
		submission{
			request:   request,
			plan:      owned,
			context:   ctx,
			composite: scope,
			rollback:  rollback,
			result:    result,
			terminal:  terminal,
		},
	); err != nil {
		return errors.Join(
			err,
			scope.kernel.abortRequestInputBody(request),
		)
	}

	accepted, err := scope.waitChildAdmission(
		ctx,
		request.UID,
		result,
	)
	if !accepted {
		return err
	}
	return errors.Join(
		err,
		scope.waitChildTerminal(
			ctx,
			request.UID,
			terminal,
			rollback,
		),
	)
}

func (scope *kernelCompositeScope) waitChildAdmission(
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
		case scope.kernel.cancel <- uid:
		case err := <-result:
			return err == nil, errors.Join(cause, err)
		case <-scope.kernel.done:
			return false, errors.Join(cause, ErrStopped)
		}
		select {
		case err := <-result:
			return err == nil, errors.Join(cause, err)
		case <-scope.kernel.done:
			return false, errors.Join(cause, ErrStopped)
		}
	case <-scope.kernel.done:
		return false, ErrStopped
	}
}

func (scope *kernelCompositeScope) waitChildTerminal(
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
				_ = scope.kernel.run.Dirty(
					errors.Join(
						errors.New(
							"jobmgr composite: rollback deadline exceeded",
						),
						cancellation,
					),
				)
				scope.kernel.NotifyControlReady()
			}
			select {
			case scope.kernel.cancel <- uid:
			case err := <-terminal:
				return errors.Join(cancellation, err)
			case <-scope.kernel.done:
				return errors.Join(cancellation, ErrStopped)
			}
		case <-scope.kernel.done:
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

type compositeFenceClaimUse struct {
	readers int
	writers int
}

func (bridge *preparedCompositeBridge) Scope() (
	scope lifecycle.ResourceTransactionScope,
) {
	if bridge == nil || bridge.transaction == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	return bridge.transaction.Scope()
}

func (bridge *preparedCompositeBridge) Apply(
	ctx context.Context,
) (lifecycle.AppliedResourceTransaction, error) {
	if bridge == nil ||
		bridge.transaction == nil ||
		bridge.scope == nil {
		return lifecycle.AppliedResourceTransaction{},
			errors.New("jobmgr composite: invalid prepared bridge")
	}
	defer bridge.scope.close()
	return bridge.transaction.ApplyComposite(ctx, bridge.scope)
}

func (bridge *preparedCompositeBridge) Dispose(
	ctx context.Context,
) (lifecycle.ReadyResource, error) {
	if bridge == nil ||
		bridge.transaction == nil ||
		bridge.scope == nil {
		return nil, errors.New(
			"jobmgr composite: invalid prepared bridge",
		)
	}
	defer bridge.scope.close()
	return bridge.transaction.Dispose(ctx)
}

func (kernel *CommandKernel) validateCompositeAdmission(
	scope *kernelCompositeScope,
	plan WorkPlan,
	rollback bool,
) (*commandOperation, error) {
	if scope == nil ||
		scope.kernel != kernel ||
		scope.parent == nil ||
		scope.closed.Load() {
		return nil, errors.New(
			"jobmgr composite: stale parent scope",
		)
	}
	parent := scope.parent
	if kernel.operations[parent.UID] != parent ||
		parent.composite != scope ||
		parent.activeChild != nil ||
		!parent.claimsHeld ||
		parent.plan.Transaction == nil ||
		parent.plan.Transaction.PrepareComposite == nil {
		return nil, errors.New(
			"jobmgr composite: parent is not accepting a child",
		)
	}
	if !rollback && (parent.cancelled || parent.TimedOut()) {
		return nil, errors.New(
			"jobmgr composite: cancelled parent rejected normal child",
		)
	}
	childClaims, err := normalizeAuthorityClaimModes(
		plan.Claims,
		plan.ReadClaims,
	)
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
	child []authorityClaim,
) bool {
	parentIndex := 0
	for _, wanted := range child {
		for parentIndex < len(parent) &&
			parent[parentIndex].key < wanted.key {
			parentIndex++
		}
		if parentIndex >= len(parent) ||
			parent[parentIndex].key != wanted.key {
			return false
		}
		held := parent[parentIndex]
		if wanted.mode == authorityClaimWrite &&
			held.mode != authorityClaimWrite {
			return false
		}
	}
	return true
}

func (kernel *CommandKernel) insertCompositeOperation(
	lane *commandLane,
	operation *commandOperation,
) {
	if lane == nil || operation == nil || operation.parent == nil {
		kernel.run.Dirty(
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

func (kernel *CommandKernel) beginCompositeFence(
	parent *commandOperation,
) error {
	if kernel == nil || parent == nil || parent.composite == nil ||
		parent.composite.parent != parent {
		return errors.New("jobmgr composite: invalid admission fence parent")
	}
	if parent.composite.fenced {
		return nil
	}
	for _, claim := range parent.claims {
		use := kernel.compositeFenceClaims[claim.key]
		if claim.mode == authorityClaimRead {
			use.readers++
		} else {
			use.writers++
		}
		kernel.compositeFenceClaims[claim.key] = use
	}
	parent.composite.fenced = true
	return nil
}

func (kernel *CommandKernel) endCompositeFence(
	parent *commandOperation,
) error {
	if kernel == nil || parent == nil || parent.composite == nil ||
		!parent.composite.fenced {
		return nil
	}
	for _, claim := range parent.claims {
		use := kernel.compositeFenceClaims[claim.key]
		if claim.mode == authorityClaimRead {
			use.readers--
		} else {
			use.writers--
		}
		if use.readers < 0 || use.writers < 0 {
			return errors.New(
				"jobmgr composite: negative admission fence ownership",
			)
		}
		if use.readers == 0 && use.writers == 0 {
			delete(kernel.compositeFenceClaims, claim.key)
		} else {
			kernel.compositeFenceClaims[claim.key] = use
		}
	}
	parent.composite.fenced = false
	kernel.compositeFenceGeneration++
	if kernel.compositeFenceGeneration == 0 {
		return errors.New(
			"jobmgr composite: admission fence generation wrapped",
		)
	}
	kernel.compositeFenceRecheck = kernel.compositeFenceHead != nil
	return nil
}

func (kernel *CommandKernel) compositeFenceConflicts(
	claims []authorityClaim,
) bool {
	for _, claim := range claims {
		use := kernel.compositeFenceClaims[claim.key]
		if claim.mode == authorityClaimRead {
			if use.writers != 0 {
				return true
			}
		} else if use.readers != 0 || use.writers != 0 {
			return true
		}
	}
	return false
}

func (kernel *CommandKernel) blockOnCompositeFence(
	operation *commandOperation,
) error {
	if operation == nil || operation.parent != nil ||
		operation.admitted || !operation.admission.Valid() ||
		operation.fenceBlocked ||
		!kernel.compositeFenceConflicts(operation.claims) {
		return errors.New(
			"jobmgr composite: invalid admission fence block",
		)
	}
	operation.fencePrevious = kernel.compositeFenceTail
	operation.fenceNext = nil
	operation.fenceBlocked = true
	operation.fenceChecked = kernel.compositeFenceGeneration
	if kernel.compositeFenceTail != nil {
		kernel.compositeFenceTail.fenceNext = operation
	} else {
		kernel.compositeFenceHead = operation
	}
	kernel.compositeFenceTail = operation
	kernel.compositeFenceCount++
	return nil
}

func (kernel *CommandKernel) removeCompositeFenceBlocked(
	operation *commandOperation,
) error {
	if operation == nil || !operation.fenceBlocked ||
		kernel.compositeFenceCount <= 0 {
		return errors.New(
			"jobmgr composite: invalid admission fence removal",
		)
	}
	if operation.fencePrevious != nil {
		operation.fencePrevious.fenceNext = operation.fenceNext
	} else {
		kernel.compositeFenceHead = operation.fenceNext
	}
	if operation.fenceNext != nil {
		operation.fenceNext.fencePrevious = operation.fencePrevious
	} else {
		kernel.compositeFenceTail = operation.fencePrevious
	}
	operation.fencePrevious = nil
	operation.fenceNext = nil
	operation.fenceBlocked = false
	kernel.compositeFenceCount--
	if kernel.compositeFenceCount == 0 {
		kernel.compositeFenceHead = nil
		kernel.compositeFenceTail = nil
		kernel.compositeFenceRecheck = false
	}
	return nil
}

func (kernel *CommandKernel) serviceCompositeFenceBlocked(
	quantum int,
) bool {
	if !kernel.compositeFenceRecheck || quantum <= 0 {
		return false
	}
	for serviced := 0; serviced < quantum; serviced++ {
		operation := kernel.compositeFenceHead
		if operation == nil {
			kernel.compositeFenceRecheck = false
			return false
		}
		if operation.fenceChecked ==
			kernel.compositeFenceGeneration {
			kernel.compositeFenceRecheck = false
			return false
		}
		if err := kernel.removeCompositeFenceBlocked(
			operation,
		); err != nil {
			kernel.run.Dirty(err)
			return false
		}
		operation.fenceChecked = kernel.compositeFenceGeneration
		if kernel.compositeFenceConflicts(operation.claims) {
			if err := kernel.blockOnCompositeFence(
				operation,
			); err != nil {
				kernel.run.Dirty(err)
				return false
			}
			continue
		}
		if err := kernel.admission.ResumeOrdinary(
			operation.admission,
		); err != nil {
			kernel.run.Dirty(err)
			return false
		}
	}
	return kernel.compositeFenceRecheck
}

func (kernel *CommandKernel) completeCompositeChild(
	child *commandOperation,
) {
	if child == nil || child.parent == nil {
		return
	}
	parent := child.parent
	child.parent = nil
	if parent.activeChild != child {
		kernel.run.Dirty(
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
		kernel.completeTask(completion)
	}
	kernel.tryDispose(parent)
}
