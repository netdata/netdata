// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
)

// SecretDependentStop records whether the acknowledged command quiesced an
// enabled job. Only stopped jobs are resumed after the Store mutation.
type SecretDependentStop struct {
	mu      sync.Mutex
	stopped bool
}

func (sds *SecretDependentStop) Stopped() (bool, error) {
	if sds == nil {
		return false, errors.New("job output: nil dependent stop")
	}
	sds.mu.Lock()
	defer sds.mu.Unlock()
	return sds.stopped, nil
}

func (sds *SecretDependentStop) markStopped() {
	sds.mu.Lock()
	sds.stopped = true
	sds.mu.Unlock()
}

// SecretDependentStart resumes the exact paused activation generation. Runtime
// outcomes are published later by the accepted activation owner.
type SecretDependentStart struct {
	resume func()
	once   sync.Once
}

func (sds *SecretDependentStart) Err() error {
	if sds == nil {
		return errors.New("job output: nil dependent start")
	}
	return nil
}

// RetainPending releases the same paused intent if a child deadline prevented
// the short resume transaction from applying. Revoked generations stay revoked.
func (sds *SecretDependentStart) RetainPending() {
	if sds != nil && sds.resume != nil {
		sds.once.Do(sds.resume)
	}
}

func (dcjc *DynCfgJobController) PlanSecretDependentStop(id string) (jobmgr.WorkPlan, *SecretDependentStop, error) {
	if dcjc == nil || id == "" {
		return jobmgr.WorkPlan{}, nil, errors.New("job output: invalid dependent stop")
	}
	state := &SecretDependentStop{}
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: id,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				if permit.Valid() || scope.ID != id {
					return nil, errors.New("job output: invalid dependent stop scope")
				}
				record, exists := dcjc.graph.Lookup(id)
				if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
					return nil, err
				}
				if !exists || record.Status != dyncfg.StatusRunning.String() &&
					(record.Status != dyncfg.StatusAccepted.String() || current == nil && !dcjc.ActivationEnabled(id)) {
					return dcjc.noop(scope, current, permit, noResponseResult())
				}
				config, err := graphRecordConfig(record)
				if err != nil {
					return nil, err
				}
				delta, err := dcjc.candidates.prepareAdoptExact(config, true, dyncfg.StatusAccepted, true, 0, dcjc.scheduler.retries.now())
				if err != nil {
					return nil, err
				}
				postimage := graphConfig(record, dyncfg.StatusAccepted)
				reply := internalReply()
				reply.candidate = delta
				reply.retainedCandidate = true // Store mutation owns permission to release this work.
				pause := &restartPause{identity: scope.Current}
				if before := delta.before; before != nil && before.Active != nil && before.Binding != nil &&
					before.Active.Token.Candidate == before.Binding.Key {
					pause.work = before.Active.Token
				}
				reply.restart.pause = pause
				return dcjc.prepareMutationAfterApply(
					ctx, scope, current, nil, permit, resourceRemovalDisposition(current), &postimage,
					reply, dcjc.configStatusCleanup(id, dyncfg.StatusAccepted), state.markStopped, jobConfigIntentPreserve,
				)
			},
		},
	}, state, nil
}

func (dcjc *DynCfgJobController) PlanSecretDependentStart(id string) (jobmgr.WorkPlan, *SecretDependentStart, error) {
	if dcjc == nil || id == "" {
		return jobmgr.WorkPlan{}, nil, errors.New("job output: invalid dependent start")
	}
	state := &SecretDependentStart{}
	var token candidateWorkToken
	if record := dcjc.candidates.view(id); record != nil && record.Active != nil {
		token = record.Active.Token
	}
	state.resume = func() { dcjc.retainCandidateResume(id, token) }
	// The enclosing Store mutation retains the graph claim across stop, commit
	// and resume. No collector construction or startup runs in this child.
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: id,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				if permit.Valid() || scope.ID != id {
					return nil, errors.New("job output: invalid dependent start scope")
				}
				record, exists := dcjc.graph.Lookup(id)
				if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
					return nil, err
				}
				if !exists || record.Status != dyncfg.StatusAccepted.String() || current != nil ||
					!dcjc.ActivationEnabled(id) {
					return dcjc.noop(scope, current, permit, noResponseResult())
				}
				delta, err := dcjc.candidates.prepareResume(id, token, dcjc.scheduler.retries.now())
				if err != nil {
					return nil, err
				}
				return dcjc.prepareCandidateDecision(ctx, scope, nil, permit, delta)
			},
		},
	}, state, nil
}
