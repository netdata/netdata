// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

type ResourceTransactionSpec struct {
	Scope                         lifecycle.ResourceTransactionScope       // identity of current + successor slots this txn spans
	Disposition                   lifecycle.ResourceTransactionDisposition // Unchanged / Installed / Removed / Replaced
	Current                       lifecycle.ReadyResource                  // running resource to stop/finalize (Removed/Replaced)
	Successor                     lifecycle.PreparedResource               // prepared job to start/publish (Installed/Replaced)
	UnusedPermit                  lifecycle.LongLivedPermit                // permit to abort when a successor slot goes unused
	Graph                         *dyncfg.Graph                            // dyncfg graph whose postimage is committed on apply
	Mutation                      dyncfg.GraphMutation                     // prepared graph mutation (valid when MutationPrepared)
	MutationPrepared              bool                                     // Mutation is owned and must be committed or aborted
	AfterGraphCommit              func()                                   // fires after the primary graph commit
	AfterApply                    func()                                   // fires after the whole transaction applies
	ActivationBusyFallback        *ResourceActivationFallback              // postimage after incumbent removal + busy promotion
	ActivationQuarantinedFallback *ResourceActivationFallback              // postimage after incumbent removal + quarantine
	Result                        lifecycle.SealedResult                   // sealed dyncfg response for the caller
	Cleanup                       lifecycle.TaskCleanup                    // protocol-frame cleanup emitted on success
}

// ResourceActivationFallback is the graph/resource outcome used when a
// successor cannot acquire its installed-runtime identity after the incumbent
// has been logically removed.
type ResourceActivationFallback struct {
	Change           dyncfg.GraphChange // failed or removed graph postimage
	AfterGraphCommit func()             // commits source/dependency projections
	AfterApply       func()             // records pending/retry state
	Result           lifecycle.SealedResult
	Cleanup          lifecycle.TaskCleanup
}

func resourceRemovalDisposition(current lifecycle.ReadyResource) lifecycle.ResourceTransactionDisposition {
	if current == nil {
		return lifecycle.ResourceTransactionUnchanged
	}
	return lifecycle.ResourceTransactionRemoved
}

func resourceInstallationDisposition(current lifecycle.ReadyResource) lifecycle.ResourceTransactionDisposition {
	if current == nil {
		return lifecycle.ResourceTransactionInstalled
	}
	return lifecycle.ResourceTransactionReplaced
}

// PreparedResourceTransaction owns one unpublished graph postimage and the
// optional prepared successor job until Apply or Dispose consumes both.
type PreparedResourceTransaction struct {
	mu sync.Mutex

	consumed bool
	spec     ResourceTransactionSpec
}

type PreparedNoopResourceTransaction struct {
	mu sync.Mutex // guards consumed

	consumed   bool                               // the transaction has been applied or disposed
	scope      lifecycle.ResourceTransactionScope // the (no-op) transaction scope
	current    lifecycle.ReadyResource            // unchanged current resource, if any
	permit     lifecycle.LongLivedPermit          // permit to abort on apply (usually empty)
	result     lifecycle.SealedResult             // sealed dyncfg response to deliver
	cleanup    lifecycle.TaskCleanup              // protocol-frame cleanup on success
	afterApply func()                             // side effect after apply
}

func PrepareNoopResourceTransaction(
	scope lifecycle.ResourceTransactionScope,
	current lifecycle.ReadyResource,
	permit lifecycle.LongLivedPermit,
	result lifecycle.SealedResult,
	cleanup lifecycle.TaskCleanup,
	afterApply func(),
) (*PreparedNoopResourceTransaction, error) {
	if !scope.Valid() ||
		(current == nil) == scope.Current.Valid() ||
		current != nil && current.Identity() != scope.Current ||
		cleanup == nil {
		return nil, errors.New("job output: invalid no-op transaction")
	}
	if scope.Successor.Valid() {
		if !permit.Valid() || permit.Owner() != scope.Successor || permit.Class() != lifecycle.LongLivedJob {
			return nil, errors.New("job output: no-op transaction has invalid successor permit")
		}
		if err := permit.ValidateLive(); err != nil {
			return nil, err
		}
	} else if permit.Valid() {
		return nil, errors.New("job output: no-op transaction has an unexpected permit")
	}
	return &PreparedNoopResourceTransaction{
		scope:      scope,
		current:    current,
		permit:     permit,
		result:     result,
		cleanup:    cleanup,
		afterApply: afterApply,
	}, nil
}

func (pnrt *PreparedNoopResourceTransaction) Scope() lifecycle.ResourceTransactionScope {
	if pnrt == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	pnrt.mu.Lock()
	defer pnrt.mu.Unlock()
	if pnrt.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return pnrt.scope
}

func (pnrt *PreparedNoopResourceTransaction) Apply(context.Context) (lifecycle.AppliedResourceTransaction, error) {
	scope, current, permit, result, cleanup, afterApply, err := pnrt.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if permit.Valid() {
		if err := permit.AbortUnused(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	applied, err := lifecycle.NewAppliedResourceTransaction(
		scope,
		lifecycle.ResourceTransactionUnchanged,
		current,
		result,
		cleanup,
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if afterApply != nil {
		afterApply()
	}
	return applied, nil
}

func (pnrt *PreparedNoopResourceTransaction) Dispose(context.Context) (lifecycle.ReadyResource, error) {
	_, current, permit, _, _, _, err := pnrt.take()
	if err != nil {
		return nil, err
	}
	if permit.Valid() {
		err = permit.AbortUnused()
	}
	return current, err
}

func (pnrt *PreparedNoopResourceTransaction) take() (
	lifecycle.ResourceTransactionScope,
	lifecycle.ReadyResource,
	lifecycle.LongLivedPermit,
	lifecycle.SealedResult,
	lifecycle.TaskCleanup,
	func(),
	error,
) {
	if pnrt == nil {
		return lifecycle.ResourceTransactionScope{},
			nil,
			lifecycle.LongLivedPermit{},
			lifecycle.SealedResult{},
			nil,
			nil,
			errors.New("job output: nil no-op transaction")
	}
	pnrt.mu.Lock()
	defer pnrt.mu.Unlock()
	if pnrt.consumed {
		return lifecycle.ResourceTransactionScope{},
			nil,
			lifecycle.LongLivedPermit{},
			lifecycle.SealedResult{},
			nil,
			nil,
			errors.New("job output: no-op transaction consumed")
	}
	pnrt.consumed = true
	return pnrt.scope, pnrt.current, pnrt.permit, pnrt.result, pnrt.cleanup, pnrt.afterApply, nil
}

func PrepareResourceTransaction(spec ResourceTransactionSpec) (*PreparedResourceTransaction, error) {
	if err := validateResourceTransactionSpec(spec); err != nil {
		return nil, err
	}
	return &PreparedResourceTransaction{
		spec: spec,
	}, nil
}

func (prt *PreparedResourceTransaction) Scope() lifecycle.ResourceTransactionScope {
	if prt == nil {
		return lifecycle.ResourceTransactionScope{}
	}
	prt.mu.Lock()
	defer prt.mu.Unlock()
	if prt.consumed {
		return lifecycle.ResourceTransactionScope{}
	}
	return prt.spec.Scope
}

func (prt *PreparedResourceTransaction) Apply(ctx context.Context) (
	applied lifecycle.AppliedResourceTransaction,
	resultErr error,
) {
	spec, err := prt.take()
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	mutationOwned := spec.Graph != nil && spec.MutationPrepared
	graphCommitted := false
	ownershipDisposition := lifecycle.ResourceTransactionUnchanged
	ownershipCurrent := spec.Current
	var pendingInstallation *JobGeneration
	appliedSealed := false
	defer func() {
		expectedRetirement := jobmgr.ContainsOnlyErrorLeaves(
			resultErr,
			jobmgr.ErrProcessAttemptRetired,
			jobmgr.ErrProcessAttemptStopped,
		)
		settlementProven := true
		if recovered := recover(); recovered != nil {
			settlementProven = false
			resultErr = errors.Join(
				resultErr,
				fmt.Errorf("%w in prepared resource transaction apply: %v", lifecycle.ErrTaskPanic, recovered),
			)
		}
		if resultErr != nil && pendingInstallation != nil {
			remaining, settleErr := pendingInstallation.settleFailedInstallation(
				context.WithoutCancel(ctx),
			)
			pendingInstallation = nil
			if remaining != nil {
				ownershipCurrent = remaining
				ownershipDisposition = spec.Disposition
			}
			if settleErr != nil {
				settlementProven = false
			}
			resultErr = errors.Join(
				resultErr,
				settleErr,
			)
		}
		if mutationOwned {
			abortErr := spec.Graph.Abort(spec.Mutation)
			if abortErr != nil {
				settlementProven = false
			}
			resultErr = errors.Join(resultErr, abortErr)
		}
		if resultErr != nil && !appliedSealed {
			failed, ownershipErr := lifecycle.NewAppliedResourceTransaction(
				spec.Scope,
				ownershipDisposition,
				ownershipCurrent,
				spec.Result,
				spec.Cleanup,
			)
			if ownershipErr != nil {
				settlementProven = false
			}
			resultErr = errors.Join(resultErr, ownershipErr)
			if ownershipErr == nil {
				applied = failed
			}
		}
		if expectedRetirement && settlementProven && !appliedSealed && !graphCommitted {
			// Only pre-commit retirement proves a complete rollback. Once the
			// graph commits, acknowledgement failure must remain fail-closed.
			resultErr = nil
		}
	}()
	if ctx == nil {
		return lifecycle.AppliedResourceTransaction{}, errors.New("job output: nil transaction apply context")
	}

	switch spec.Disposition {
	case lifecycle.ResourceTransactionUnchanged,
		lifecycle.ResourceTransactionInstalled:
	case lifecycle.ResourceTransactionRemoved:
		if spec.UnusedPermit.Valid() {
			if err := spec.UnusedPermit.AbortUnused(); err != nil {
				return lifecycle.AppliedResourceTransaction{}, err
			}
		}
		if err := spec.Current.Stop(ctx); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		if err := spec.Current.Finalize(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		ownershipCurrent = nil
		ownershipDisposition = lifecycle.ResourceTransactionRemoved
	case lifecycle.ResourceTransactionReplaced:
		if err := spec.Current.Stop(ctx); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		if err := spec.Current.Finalize(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		ownershipCurrent = nil
		ownershipDisposition = lifecycle.ResourceTransactionRemoved
	default:
		return lifecycle.AppliedResourceTransaction{}, errors.New("job output: invalid transaction disposition")
	}

	var current lifecycle.ReadyResource
	disposition := spec.Disposition
	switch spec.Disposition {
	case lifecycle.ResourceTransactionUnchanged:
		if spec.UnusedPermit.Valid() {
			if err := spec.UnusedPermit.AbortUnused(); err != nil {
				return lifecycle.AppliedResourceTransaction{}, err
			}
		}
		current = spec.Current
	case lifecycle.ResourceTransactionInstalled,
		lifecycle.ResourceTransactionReplaced:
		current, err = spec.Successor.AcceptStart(ctx, spec.Scope.Successor.Generation)
		if current != nil {
			generation, isJobGeneration := current.(*JobGeneration)
			if !isJobGeneration || !generation.installationPending() {
				ownershipCurrent = current
				ownershipDisposition = spec.Disposition
			} else {
				pendingInstallation = generation
			}
		}
		if err != nil {
			if fallback := activationFallback(spec, err); fallback != nil && current == nil {
				spec.Result = fallback.Result
				spec.Cleanup = fallback.Cleanup
				if mutationOwned {
					if abortErr := spec.Graph.Abort(spec.Mutation); abortErr != nil {
						return lifecycle.AppliedResourceTransaction{}, abortErr
					}
					mutationOwned = false
				}
				fallbackMutation, mutationErr := spec.Graph.PrepareMutation(
					[]dyncfg.GraphChange{fallback.Change},
				)
				switch {
				case mutationErr == nil:
					if commitErr := commitGraphMutation(spec.Graph, fallbackMutation); commitErr != nil {
						return lifecycle.AppliedResourceTransaction{}, commitErr
					}
					if fallback.AfterGraphCommit != nil {
						fallback.AfterGraphCommit()
					}
				case errors.Is(mutationErr, dyncfg.ErrGraphNoChange):
				// The graph already represents the fallback outcome.
				default:
					return lifecycle.AppliedResourceTransaction{}, mutationErr
				}
				applied, applyErr := lifecycle.NewAppliedResourceTransaction(
					spec.Scope,
					ownershipDisposition,
					nil,
					fallback.Result,
					fallback.Cleanup,
				)
				if applyErr != nil {
					return lifecycle.AppliedResourceTransaction{}, applyErr
				}
				appliedSealed = true
				if fallback.AfterApply != nil {
					fallback.AfterApply()
				}
				return applied, nil
			}
			return lifecycle.AppliedResourceTransaction{}, err
		}
		if current == nil {
			return lifecycle.AppliedResourceTransaction{},
				errors.New("job output: accepted transaction successor is nil")
		}
		if err := current.Publish(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		if pendingInstallation != nil {
			if err := pendingInstallation.reserveInstallation(); err != nil {
				return lifecycle.AppliedResourceTransaction{}, err
			}
		}
	case lifecycle.ResourceTransactionRemoved:
	default:
		return lifecycle.AppliedResourceTransaction{}, errors.New("job output: invalid transaction disposition")
	}
	if spec.Graph != nil && spec.MutationPrepared {
		mutationOwned = false
		if err := commitGraphMutation(spec.Graph, spec.Mutation); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
		graphCommitted = true
		if spec.AfterGraphCommit != nil {
			spec.AfterGraphCommit()
		}
	}
	applied, err = lifecycle.NewAppliedResourceTransaction(
		spec.Scope,
		disposition,
		current,
		spec.Result,
		spec.Cleanup,
	)
	if err != nil {
		return lifecycle.AppliedResourceTransaction{}, err
	}
	if installed, ok := current.(interface{ acknowledgeInstallation() error }); ok {
		if err := installed.acknowledgeInstallation(); err != nil {
			return lifecycle.AppliedResourceTransaction{}, err
		}
	}
	pendingInstallation = nil
	appliedSealed = true
	if spec.AfterApply != nil {
		spec.AfterApply()
	}
	return applied, nil
}

func activationFallback(spec ResourceTransactionSpec, err error) *ResourceActivationFallback {
	switch {
	case errors.Is(err, jobmgr.ErrProcessAttemptQuarantined):
		return spec.ActivationQuarantinedFallback
	case errors.Is(err, jobmgr.ErrProcessAttemptBusy):
		return spec.ActivationBusyFallback
	default:
		return nil
	}
}

func commitGraphMutation(graph *dyncfg.Graph, mutation dyncfg.GraphMutation) (resultErr error) {
	mutationOwned := true
	defer func() {
		if mutationOwned {
			resultErr = errors.Join(resultErr, graph.Abort(mutation))
		}
	}()
	if err := graph.Commit(mutation); err != nil {
		return err
	}
	mutationOwned = false
	return nil
}

func composeAfterApply(first, second func()) func() {
	if first == nil {
		return second
	}
	if second == nil {
		return first
	}
	return func() {
		first()
		second()
	}
}

func (prt *PreparedResourceTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	spec, err := prt.take()
	if err != nil {
		return nil, err
	}
	if ctx == nil {
		return nil, errors.New("job output: nil transaction dispose context")
	}
	var abortErr error
	if spec.Graph != nil && spec.MutationPrepared {
		abortErr = spec.Graph.Abort(spec.Mutation)
	}
	var successorErr error
	if spec.Successor != nil {
		successorErr = spec.Successor.Dispose(ctx)
	} else if spec.UnusedPermit.Valid() {
		successorErr = spec.UnusedPermit.AbortUnused()
	}
	return spec.Current, errors.Join(abortErr, successorErr)
}

func (prt *PreparedResourceTransaction) take() (ResourceTransactionSpec, error) {
	if prt == nil {
		return ResourceTransactionSpec{}, errors.New("job output: nil prepared resource transaction")
	}
	prt.mu.Lock()
	defer prt.mu.Unlock()
	if prt.consumed {
		return ResourceTransactionSpec{}, errors.New("job output: prepared resource transaction consumed")
	}
	prt.consumed = true
	spec := prt.spec
	prt.spec = ResourceTransactionSpec{}
	return spec, nil
}

func validateResourceTransactionSpec(spec ResourceTransactionSpec) error {
	if !spec.Scope.Valid() || spec.Cleanup == nil {
		return errors.New("job output: invalid resource transaction")
	}
	if spec.Current != nil && spec.Current.Identity() != spec.Scope.Current {
		return errors.New("job output: transaction current differs from scope")
	}
	if spec.Successor != nil && spec.Successor.Identity() != spec.Scope.Successor {
		return errors.New("job output: transaction successor differs from scope")
	}
	if successor, ok := spec.Successor.(interface {
		validateLivePermit() error
	}); ok {
		if err := successor.validateLivePermit(); err != nil {
			return err
		}
	}
	if spec.UnusedPermit.Valid() &&
		(spec.UnusedPermit.Owner() != spec.Scope.Successor ||
			spec.UnusedPermit.Class() != lifecycle.LongLivedJob) {
		return errors.New("job output: transaction unused permit differs from scope")
	}
	if spec.UnusedPermit.Valid() {
		if err := spec.UnusedPermit.ValidateLive(); err != nil {
			return err
		}
	}
	if spec.Successor != nil && spec.UnusedPermit.Valid() {
		return errors.New("job output: transaction owns both successor and unused permit")
	}
	for _, fallback := range []*ResourceActivationFallback{
		spec.ActivationBusyFallback,
		spec.ActivationQuarantinedFallback,
	} {
		if fallback == nil {
			continue
		}
		if spec.Successor == nil ||
			spec.Graph == nil ||
			fallback.Change.ID != spec.Scope.ID ||
			fallback.Cleanup == nil {
			return errors.New("job output: invalid activation fallback")
		}
		if fallback.Change.Config != nil && fallback.Change.Config.ID != spec.Scope.ID {
			return errors.New("job output: activation fallback config differs from scope")
		}
	}
	switch spec.Disposition {
	case lifecycle.ResourceTransactionUnchanged:
		if spec.Successor != nil || (spec.Current == nil) == spec.Scope.Current.Valid() {
			return errors.New("job output: invalid unchanged transaction")
		}
		if spec.Scope.Successor.Valid() != spec.UnusedPermit.Valid() {
			return errors.New("job output: unchanged transaction lost successor permit")
		}
	case lifecycle.ResourceTransactionRemoved:
		if spec.Current == nil ||
			!spec.Scope.Current.Valid() ||
			spec.Successor != nil ||
			spec.UnusedPermit.Valid() != spec.Scope.Successor.Valid() {
			return errors.New("job output: invalid remove transaction")
		}
	case lifecycle.ResourceTransactionInstalled:
		if spec.Current != nil ||
			spec.Scope.Current.Valid() ||
			spec.Successor == nil ||
			spec.UnusedPermit.Valid() ||
			!spec.Scope.Successor.Valid() {
			return errors.New("job output: invalid install transaction")
		}
	case lifecycle.ResourceTransactionReplaced:
		if spec.Current == nil ||
			!spec.Scope.Current.Valid() ||
			spec.Successor == nil ||
			spec.UnusedPermit.Valid() ||
			!spec.Scope.Successor.Valid() {
			return errors.New("job output: invalid replace transaction")
		}
	default:
		return errors.New("job output: unknown resource transaction disposition")
	}
	return nil
}
