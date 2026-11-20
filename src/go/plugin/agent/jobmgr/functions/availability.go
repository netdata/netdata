// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
)

// active spans admission through reconciliation. The controller lock protects it.
// Selectors only share the shorter critical section that installs observations.
type ownerAvailability struct {
	active    bool
	selectors sync.RWMutex
}

type availabilityPollMember struct {
	jobName    string // empty for the agent or process bundle
	bundle     *functionBundle
	poll       functionAvailabilityPoll
	result     functionAvailabilityResult
	diagnostic string
}

func (c *Controller) ownerAvailabilityLocked(owner functionOwner) *ownerAvailability {
	state := c.availability[owner]
	if state == nil {
		state = &ownerAvailability{}
		c.availability[owner] = state
	}
	return state
}

func (c *Controller) reconcileOwner(ctx context.Context, owner functionOwner) error {
	if c == nil || ctx == nil || owner.name == "" {
		return errors.New("jobmgr Function controller: invalid module reconcile")
	}
	state, members, err := c.beginAvailabilityPass(owner)
	if err != nil || len(members) == 0 {
		return err
	}

	// Admit all workers before awaiting any of them. Preserve synchronous
	// admission errors and finish every attempt already started.
	admitted := members[:0]
	var startErr error
	for _, member := range members {
		poll, err := member.bundle.startAvailabilityPoll()
		if errors.Is(err, jobmgr.ErrProcessAttemptBusy) || errors.Is(err, jobmgr.ErrProcessAttemptStopped) {
			continue
		}
		if err != nil {
			startErr = errors.Join(startErr, err)
			continue
		}
		if poll.attempt != nil {
			member.poll = poll
			admitted = append(admitted, member)
		}
	}
	if len(admitted) == 0 {
		c.mu.Lock()
		state.active = false
		c.mu.Unlock()
	} else {
		go c.finishAvailabilityPass(owner, state, admitted)
	}
	return startErr
}

func (c *Controller) beginAvailabilityPass(owner functionOwner) (*ownerAvailability, []availabilityPollMember, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.usableLocked(); err != nil {
		return nil, nil, err
	}
	plan, ok := c.plans[owner]
	if !ok {
		return nil, nil, errors.New("jobmgr Function controller: module is not registered")
	}
	if state := c.availability[owner]; state != nil && state.active {
		return nil, nil, nil
	}
	var members []availabilityPollMember
	if bundle := plan.agentBundle; bundle != nil && bundle.pollable {
		members = append(members, availabilityPollMember{bundle: bundle})
	}
	for name, job := range c.ownerJobs(owner) {
		if job.bundle.pollable {
			members = append(members, availabilityPollMember{jobName: name, bundle: job.bundle})
		}
	}
	if len(members) == 0 {
		return nil, nil, nil
	}
	state := c.ownerAvailabilityLocked(owner)
	state.active = true
	return state, members, nil
}

func (poll functionAvailabilityPoll) await() (functionAvailabilityResult, string) {
	if err := poll.attempt.Await(context.Background()); err != nil {
		if jobmgr.ContainsOnlyErrorLeaves(err,
			context.Canceled,
			jobmgr.ErrProcessAttemptRetired,
			jobmgr.ErrProcessAttemptStopped,
			jobmgr.ErrProcessAttemptSuperseded,
		) {
			return functionAvailabilityResult{}, ""
		}
		return functionAvailabilityResult{err: err}, DiagnosticAvailabilityAttemptFailed
	}
	result := <-poll.workerResult
	if result.err != nil {
		return result, DiagnosticAvailabilityCallbackFailed
	}
	return result, ""
}

func (c *Controller) finishAvailabilityPass(owner functionOwner, state *ownerAvailability, members []availabilityPollMember) {
	for i := range members {
		members[i].result, members[i].diagnostic = members[i].poll.await()
	}
	c.mu.Lock()
	var reconcileErr error
	if c.usableLocked() == nil {
		state.selectors.Lock()
		changed := false
		for _, m := range members {
			if m.result.availability == nil {
				continue
			}
			if m.jobName != "" && c.ownerJobs(owner)[m.jobName].bundle != m.bundle {
				continue // detached or replaced while the pass was settling
			}
			if m.bundle.commitAvailability(m.result.availability) {
				changed = true
			}
		}
		state.selectors.Unlock()
		if changed {
			reconcileErr = c.reconcileOwnerLocked(context.Background(), owner)
		}
	}
	state.active = false
	c.mu.Unlock()

	for _, m := range members {
		c.observeAvailabilityFailure(m.diagnostic, m.bundle, m.result.err)
	}
	c.observeAvailabilityFailure(DiagnosticAvailabilityReconciliationFailed, c.plans[owner].agentBundle, reconcileErr)
}
