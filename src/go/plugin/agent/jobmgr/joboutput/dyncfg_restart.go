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

// Observation follows accepted intent across reconstruction. Physical attempts
// still fence execution; installed runtimes settle through their exact identity.
type restartObservers struct {
	mu          sync.Mutex
	sequence    uint64
	closed      bool
	entries     map[lifecycle.ResourceIdentity]*restartReceipt
	activations map[restartContinuation]*restartReceipt
}

type restartContinuation struct {
	run       uint64
	intent    uint64
	candidate candidateKey
}

func observedContinuation(token candidateWorkToken) restartContinuation {
	return restartContinuation{run: token.Run, intent: token.Intent, candidate: token.Candidate}
}

// These effects belong to the adapter, not the pure candidate policy. They run
// after its delta commits and before retirement or retry can release effects.
type activationRestartSettlement struct {
	work   candidateWorkToken
	result lifecycle.SealedResult
}

type restartPause struct {
	work     candidateWorkToken
	identity lifecycle.ResourceIdentity
}

type restartObservationTransition struct {
	settlement *activationRestartSettlement
	pause      *restartPause
}

type restartReceipt struct {
	result     chan lifecycle.SealedResult
	identity   lifecycle.ResourceIdentity // protected by restartObservers.mu
	activation restartContinuation        // protected by restartObservers.mu
	detached   bool                       // protected by restartObservers.mu
}

func (observers *restartObservers) next() uint64 {
	observers.mu.Lock()
	defer observers.mu.Unlock()
	observers.sequence++
	return observers.sequence
}

func (observers *restartObservers) register(identity lifecycle.ResourceIdentity, receipt *restartReceipt) error {
	observers.mu.Lock()
	defer observers.mu.Unlock()
	if receipt.detached {
		return nil
	}
	if observers.closed {
		return jobmgr.ErrProcessAttemptRetired
	}
	if !identity.Valid() || receipt.identity.Valid() || observers.entries[identity] != nil {
		return errors.New("job output: invalid restart observer registration")
	}
	if observers.entries == nil {
		observers.entries = make(map[lifecycle.ResourceIdentity]*restartReceipt)
	}
	receipt.identity = identity
	observers.entries[identity] = receipt
	return nil
}

func (observers *restartObservers) detach(receipt *restartReceipt) {
	observers.mu.Lock()
	defer observers.mu.Unlock()
	receipt.detached = true
	if observers.entries[receipt.identity] == receipt {
		delete(observers.entries, receipt.identity)
	}
	if observers.activations[receipt.activation] == receipt {
		delete(observers.activations, receipt.activation)
	}
}

// A replacement may have to wait for the retired runtime's physical release.
// Bridge that one accepted attempt to its eventual installed generation.
func (dcjc *DynCfgJobController) deferRestartToActivation(identity lifecycle.ResourceIdentity, token acceptedActivationToken) {
	observers := &dcjc.restartObservers
	observers.mu.Lock()
	defer observers.mu.Unlock()
	if receipt := observers.entries[identity]; receipt != nil {
		delete(observers.entries, identity)
		if observers.activations == nil {
			observers.activations = make(map[restartContinuation]*restartReceipt)
		}
		receipt.identity = lifecycle.ResourceIdentity{}
		receipt.activation = observedContinuation(token.work)
		observers.activations[receipt.activation] = receipt
	}
}

func (dcjc *DynCfgJobController) installActivationRestart(token acceptedActivationToken, identity lifecycle.ResourceIdentity) {
	observers := &dcjc.restartObservers
	observers.mu.Lock()
	defer observers.mu.Unlock()
	key := observedContinuation(token.work)
	if receipt := observers.activations[key]; receipt != nil {
		delete(observers.activations, key)
		receipt.activation = restartContinuation{}
		receipt.identity = identity
		observers.entries[identity] = receipt
	}
}

func restartRetiredResult() lifecycle.SealedResult {
	return rejectedResult(jobFailure{
		class:   failureUnavailable,
		message: "the restarted job was stopped or replaced before becoming ready.",
	})
}

func (observers *restartObservers) completeLocked(receipt *restartReceipt, result lifecycle.SealedResult) {
	if observers.entries[receipt.identity] == receipt {
		delete(observers.entries, receipt.identity)
	}
	if observers.activations[receipt.activation] == receipt {
		delete(observers.activations, receipt.activation)
	}
	receipt.identity = lifecycle.ResourceIdentity{}
	receipt.activation = restartContinuation{}
	receipt.result <- result
}

// Work slots provide semantic authority even while no executor is registered.
// Capture only the bounded slot identities; delta.apply consumes its predecessor.
func (dcjc *DynCfgJobController) prepareRestartObservationCommit(delta *candidateDelta, transition restartObservationTransition) func() {
	var previous, surviving [2]candidateWorkToken
	for index, record := range []*candidateRecord{delta.before, delta.after} {
		if record == nil {
			continue
		}
		for role, work := range []*candidateWork{record.Active, record.Proposal} {
			if work != nil && delta.owner.validRecord(record, work.Token) {
				if index == 0 {
					previous[role] = work.Token
				} else {
					surviving[role] = work.Token
				}
			}
		}
	}
	return func() {
		observers := &dcjc.restartObservers
		observers.mu.Lock()
		defer observers.mu.Unlock()
		if pause := transition.pause; pause != nil && surviving[0].Run != 0 {
			receipt := observers.activations[observedContinuation(pause.work)]
			if receipt == nil && pause.identity.Valid() {
				receipt = observers.entries[pause.identity]
			}
			if receipt != nil {
				delete(observers.entries, receipt.identity)
				delete(observers.activations, receipt.activation)
				if observers.activations == nil {
					observers.activations = make(map[restartContinuation]*restartReceipt)
				}
				receipt.identity = lifecycle.ResourceIdentity{}
				receipt.activation = observedContinuation(surviving[0])
				observers.activations[receipt.activation] = receipt
			}
		}
		if settlement := transition.settlement; settlement != nil &&
			(settlement.work == previous[0] || settlement.work == previous[1]) {
			if receipt := observers.activations[observedContinuation(settlement.work)]; receipt != nil {
				observers.completeLocked(receipt, settlement.result)
			}
		}
		for _, token := range previous {
			key := observedContinuation(token)
			if token.Run == 0 || key == observedContinuation(surviving[0]) || key == observedContinuation(surviving[1]) {
				continue
			}
			if receipt := observers.activations[key]; receipt != nil {
				observers.completeLocked(receipt, restartRetiredResult())
			}
		}
	}
}

func (observers *restartObservers) retire() {
	observers.mu.Lock()
	defer observers.mu.Unlock()
	observers.closed = true
	for _, receipt := range observers.activations {
		observers.completeLocked(receipt, restartRetiredResult())
	}
	for _, receipt := range observers.entries {
		observers.completeLocked(receipt, restartRetiredResult())
	}
}

// completeRestart is called only after the exact generation's graph settlement
// commits. A later retry or another generation cannot answer an older RESTART.
func (dcjc *DynCfgJobController) completeRestart(identity lifecycle.ResourceIdentity, result lifecycle.SealedResult) {
	observers := &dcjc.restartObservers
	observers.mu.Lock()
	defer observers.mu.Unlock()
	if receipt := observers.entries[identity]; receipt != nil {
		observers.completeLocked(receipt, result)
	}
}

// Specific failure settlement runs before this generic retirement fallback.
func (dcjc *DynCfgJobController) retireRestart(identity lifecycle.ResourceIdentity) {
	dcjc.completeRestart(identity, restartRetiredResult())
}

func (dcjc *DynCfgJobController) restart(ctx context.Context, request DynCfgJobRequest, target dynCfgTarget) (lifecycle.SealedResult, error) {
	if dcjc.commands == nil {
		return lifecycle.SealedResult{}, errors.New("job output: restart command port is unavailable")
	}
	sequence := dcjc.restartObservers.next()
	if sequence == 0 {
		return lifecycle.SealedResult{}, errors.New("job output: restart observer identity exhausted")
	}
	receipt := &restartReceipt{result: make(chan lifecycle.SealedResult, 1)}
	defer dcjc.restartObservers.detach(receipt)
	deadline, _ := ctx.Deadline()
	internal := jobmgr.Request{
		UID:      fmt.Sprintf("jobmgr-restart-%d-%d", dcjc.generation, sequence),
		Source:   lifecycle.SourceJobManager,
		LaneKey:  target.resourceID,
		Route:    "internal/jobs/restart",
		Deadline: deadline,
	}
	plan := jobmgr.WorkPlan{
		Claims:              []string{DynCfgJobGraphClaim},
		NoResponse:          true,
		YieldClaimOnPrepare: DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                target.resourceID,
			AllocateSuccessor: true,
			Permit:            lifecycle.NewJobLongLivedPlan(),
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				prepared, err := dcjc.Prepare(ctx, request, current, scope, permit)
				if err != nil {
					return prepared, err
				}
				if err := dcjc.restartObservers.register(scope.Successor, receipt); err != nil {
					return prepared, err
				}
				return &restartTransaction{PreparedResourceTransaction: prepared, controller: dcjc, scope: scope}, nil
			},
		},
	}
	// Terminal completion releases the inner resource lane before this observer
	// waits. Cancellation after acceptance only ends observation.
	if err := dcjc.commands.SubmitPreparedAndWait(ctx, internal, plan); err != nil {
		return lifecycle.SealedResult{}, err
	}
	select {
	case result := <-receipt.result:
		return result, nil
	case <-ctx.Done():
		return lifecycle.SealedResult{}, context.Cause(ctx)
	}
}

type restartTransaction struct {
	lifecycle.PreparedResourceTransaction
	controller *DynCfgJobController
	scope      lifecycle.ResourceTransactionScope
}

func (transaction *restartTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	before, existed := transaction.controller.graph.Lookup(transaction.scope.ID)
	applied, err := transaction.PreparedResourceTransaction.Apply(ctx)
	if err != nil {
		return applied, err
	}
	_, disposition, current := applied.Ownership()
	installed := (disposition == lifecycle.ResourceTransactionInstalled || disposition == lifecycle.ResourceTransactionReplaced) &&
		current != nil && current.Identity() == transaction.scope.Successor
	if !installed {
		if after, exists := transaction.controller.graph.Lookup(transaction.scope.ID); existed && exists &&
			(before.Status == dyncfg.StatusRunning.String() || before.Status == dyncfg.StatusFailed.String()) &&
			after.Status == dyncfg.StatusAccepted.String() {
			if token, ok := transaction.controller.scheduler.accepted.currentToken(transaction.scope.ID); ok {
				transaction.controller.deferRestartToActivation(transaction.scope.Successor, token)
				return applied, nil
			}
		}
		transaction.controller.completeRestart(transaction.scope.Successor, applied.Result())
	}
	return applied, nil
}
