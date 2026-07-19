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
	anchor := lane.active
	cursor := lane.head
	if anchor != nil {
		cursor = anchor.next
	}
	for ; cursor != nil; cursor = cursor.next {
		if cursor.parent != nil ||
			!authorityClaimsConflict(
				operation.parent.claims,
				cursor.claims,
			) {
			anchor = cursor
			continue
		}
		break
	}
	if anchor == nil {
		operation.next = lane.head
		if lane.head != nil {
			lane.head.previous = operation
		} else {
			lane.tail = operation
		}
		lane.head = operation
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
}

func authorityClaimsConflict(left, right []authorityClaim) bool {
	leftIndex := 0
	rightIndex := 0
	for leftIndex < len(left) && rightIndex < len(right) {
		leftClaim := left[leftIndex]
		rightClaim := right[rightIndex]
		switch {
		case leftClaim.key < rightClaim.key:
			leftIndex++
		case rightClaim.key < leftClaim.key:
			rightIndex++
		default:
			if leftClaim.mode != authorityClaimRead ||
				rightClaim.mode != authorityClaimRead {
				return true
			}
			leftIndex++
			rightIndex++
		}
	}
	return false
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
