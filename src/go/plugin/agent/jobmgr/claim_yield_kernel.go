// SPDX-License-Identifier: GPL-3.0-or-later

package jobmgr

import (
	"context"
	"errors"
)

type claimYieldAction uint8

const (
	claimYieldRelease claimYieldAction = iota + 1
	claimYieldReacquire
)

type claimYieldRequest struct {
	operation *commandOperation
	action    claimYieldAction
	result    chan error
}

type kernelClaimYieldLease struct {
	kernel    *CommandKernel
	operation *commandOperation
}

func (lease kernelClaimYieldLease) release(ctx context.Context) error {
	return lease.request(ctx, claimYieldRelease)
}

func (lease kernelClaimYieldLease) reacquire(ctx context.Context) error {
	return lease.request(ctx, claimYieldReacquire)
}

func (lease kernelClaimYieldLease) request(ctx context.Context, action claimYieldAction) error {
	if lease.kernel == nil || lease.operation == nil || ctx == nil {
		return errors.New("jobmgr claims: invalid yield lease")
	}
	result := make(chan error, 1)
	request := claimYieldRequest{
		operation: lease.operation,
		action:    action,
		result:    result,
	}
	select {
	case lease.kernel.claimYields <- request:
	case <-ctx.Done():
		return context.Cause(ctx)
	case <-lease.kernel.done:
		return ErrStopped
	}
	select {
	case err := <-result:
		return err
	case <-lease.kernel.done:
		return ErrStopped
	}
}

func (ck *CommandKernel) serviceClaimYields(quantum int) bool {
	if ck == nil || quantum <= 0 {
		return false
	}
	for range quantum {
		select {
		case request := <-ck.claimYields:
			ck.serviceClaimYield(request)
		default:
			return false
		}
	}
	return len(ck.claimYields) != 0
}

func (ck *CommandKernel) serviceClaimYield(request claimYieldRequest) {
	operation := request.operation
	if operation == nil || request.result == nil ||
		ck.operations[operation.UID] != operation ||
		operation.lane == nil || operation.lane.active != operation ||
		operation.plan.YieldClaimOnPrepare == "" {
		request.result <- errors.New("jobmgr claims: invalid yield request")
		return
	}
	owner := operation
	if operation.claimsInherited {
		owner = operation.parent
	}
	if owner == nil ||
		ck.operations[owner.UID] != owner ||
		owner.lane == nil || owner.lane.active != owner ||
		(operation.claimsInherited && owner.activeChild != operation) {
		request.result <- errors.New("jobmgr claims: invalid yield owner")
		return
	}
	switch request.action {
	case claimYieldRelease:
		if owner.claimsYielded || owner.claimYieldBorrower != nil ||
			owner.claimYieldResult != nil || !owner.claimsHeld {
			request.result <- errors.New("jobmgr claims: invalid release request")
			return
		}
		fenceSuspended := false
		if owner.composite != nil && owner.composite.fenced {
			if err := ck.suspendCompositeFenceClaim(owner, operation.plan.YieldClaimOnPrepare); err != nil {
				request.result <- err
				return
			}
			fenceSuspended = true
		}
		granted, err := ck.claims.yield(
			owner,
			operation.plan.YieldClaimOnPrepare,
			operation.request.LaneKey,
		)
		if err != nil {
			if fenceSuspended {
				err = errors.Join(err, ck.restoreCompositeFenceClaim(owner, operation.plan.YieldClaimOnPrepare))
			}
			request.result <- err
			return
		}
		owner.claimsYielded = true
		owner.claimYieldBorrower = operation
		for _, grantedOperation := range granted {
			ck.completeClaimGrant(grantedOperation)
		}
		request.result <- nil
	case claimYieldReacquire:
		if !owner.claimsYielded || owner.claimYieldBorrower != operation ||
			owner.claimYieldResult != nil || owner.claimsHeld || ck.claims.waiting(owner) {
			request.result <- errors.New("jobmgr claims: invalid reacquire request")
			return
		}
		granted, err := ck.claims.acquire(owner)
		if err != nil {
			request.result <- err
			return
		}
		if granted {
			if owner.claimYieldFence {
				if err := ck.restoreCompositeFenceClaim(owner, operation.plan.YieldClaimOnPrepare); err != nil {
					request.result <- err
					return
				}
			}
			owner.claimsYielded = false
			owner.claimYieldBorrower = nil
			request.result <- nil
			return
		}
		owner.claimYieldResult = request.result
	default:
		request.result <- errors.New("jobmgr claims: unknown yield action")
	}
}

func (ck *CommandKernel) completeClaimGrant(operation *commandOperation) {
	if operation == nil {
		ck.run.Dirty(errors.New("jobmgr claims: nil granted operation"))
		return
	}
	if operation.claimYieldResult == nil {
		ck.markReady(operation.lane)
		return
	}
	if !operation.claimsYielded || operation.claimYieldBorrower == nil ||
		!operation.claimsHeld || ck.claims.waiting(operation) {
		ck.run.Dirty(errors.New("jobmgr claims: invalid yielded claim grant"))
		return
	}
	result := operation.claimYieldResult
	borrower := operation.claimYieldBorrower
	if operation.claimYieldFence {
		if err := ck.restoreCompositeFenceClaim(operation, borrower.plan.YieldClaimOnPrepare); err != nil {
			ck.run.Dirty(err)
			result <- err
			return
		}
	}
	operation.claimYieldResult = nil
	operation.claimsYielded = false
	operation.claimYieldBorrower = nil
	result <- nil
}
