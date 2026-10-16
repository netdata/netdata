// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

// interruptExecution suspends only physical work. The controller retains the
// predecessor's logical intent until acceptance or acknowledged resumption.
func (aai *acceptedActivationIndex) interruptExecution(id string) (candidateWorkToken, bool) {
	aai.mu.Lock()
	entry := aai.entries[id]
	if entry == nil || entry.spec.work == nil {
		aai.mu.Unlock()
		return candidateWorkToken{}, false
	}
	token, owned := entry.spec.work.Token, entry.stage
	stage := aai.detachLocked(id)
	aai.mu.Unlock()
	if owned != nil {
		owned.Cancel(jobmgr.ErrProcessAttemptSuperseded)
	}
	releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
	return token, true
}

// The delivery owner outlives the request context. It never waits in the
// originating command lane and retains completion responsibility until ack.
func (dcjc *DynCfgJobController) retainCandidateResume(id string, token candidateWorkToken) {
	if !dcjc.candidates.valid(id, token) {
		return
	}
	aai := dcjc.scheduler.accepted
	aai.mu.Lock()
	if !aai.bound || aai.closed || aai.failed {
		aai.mu.Unlock()
		return
	}
	aai.generation++
	sequence := aai.generation
	if sequence == 0 {
		aai.mu.Unlock()
		aai.fail(errors.New("job output: continuation delivery sequence wrapped"))
		return
	}
	aai.wg.Add(1)
	aai.mu.Unlock()
	go func() {
		defer aai.wg.Done()
		err := dcjc.commands.SubmitPreparedAndWait(context.Background(), jobmgr.Request{
			UID:     fmt.Sprintf("jobmgr-candidate-resume-%d-%d", token.Run, sequence),
			LaneKey: id, Source: lifecycle.SourceJobManager, Route: "internal/jobs/candidate-resume",
		}, dcjc.planCandidateResume(id, token))
		if err != nil && !lifecycle.ContainsOnlyCurrentStoppingRejections(err, token.Run) &&
			!jobmgr.ContainsOnlyErrorLeaves(err, jobmgr.ErrProcessAttemptRetired, jobmgr.ErrProcessAttemptStopped) {
			dcjc.candidateFailure(err)
		}
	}()
}

func (dcjc *DynCfgJobController) planCandidateResume(id string, token candidateWorkToken) jobmgr.WorkPlan {
	return jobmgr.WorkPlan{Claims: []string{DynCfgJobGraphClaim}, NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{ID: id,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				if current != nil {
					return dcjc.noop(scope, current, permit, noResponseResult())
				}
				delta, err := dcjc.candidates.prepareResume(id, token, dcjc.scheduler.retries.now())
				if err != nil {
					return nil, err
				}
				return dcjc.prepareCandidateDecision(ctx, scope, current, permit, delta)
			},
		},
	}
}

type candidatePreflightTransaction struct {
	lifecycle.PreparedResourceTransaction
	resume func()
	once   sync.Once
}

func (t *candidatePreflightTransaction) Apply(ctx context.Context) (lifecycle.AppliedResourceTransaction, error) {
	defer t.once.Do(t.resume)
	return t.PreparedResourceTransaction.Apply(ctx)
}

func (t *candidatePreflightTransaction) Dispose(ctx context.Context) (lifecycle.ReadyResource, error) {
	defer t.once.Do(t.resume)
	return t.PreparedResourceTransaction.Dispose(ctx)
}
