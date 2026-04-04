// SPDX-License-Identifier: GPL-3.0-or-later

package chartengine

import (
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
)

var (
	ErrOutstandingPlanAttempt = errors.New("chartengine: plan attempt already outstanding")
	ErrStalePlanAttempt       = errors.New("chartengine: stale plan attempt")
	ErrFinishedPlanAttempt    = errors.New("chartengine: plan attempt already finished")
)

type PlanAttempt struct {
	state *planAttemptState
}

type planAttemptState struct {
	mu sync.Mutex

	engine       *Engine
	plan         Plan
	materialized materializedState
	epoch        uint64
	commitSeq    uint64
	attemptID    uint64
	reserved     bool
	finished     bool
}

func (a PlanAttempt) Plan() Plan {
	if a.state == nil {
		return Plan{}
	}
	return a.state.plan
}

func (a PlanAttempt) Commit() error {
	if a.state == nil {
		return nil
	}

	a.state.mu.Lock()
	if a.state.finished {
		a.state.mu.Unlock()
		return ErrFinishedPlanAttempt
	}
	a.state.finished = true
	reserved := a.state.reserved
	engine := a.state.engine
	materialized := a.state.materialized
	epoch := a.state.epoch
	commitSeq := a.state.commitSeq
	attemptID := a.state.attemptID
	a.state.mu.Unlock()

	if !reserved {
		return nil
	}
	if engine == nil {
		return fmt.Errorf("chartengine: nil engine on commit")
	}
	return engine.commitAttempt(materialized, epoch, commitSeq, attemptID)
}

func (a PlanAttempt) Abort() {
	if a.state == nil {
		return
	}

	a.state.mu.Lock()
	if a.state.finished {
		a.state.mu.Unlock()
		return
	}
	a.state.finished = true
	reserved := a.state.reserved
	engine := a.state.engine
	attemptID := a.state.attemptID
	a.state.mu.Unlock()

	if !reserved || engine == nil {
		return
	}
	engine.abortAttempt(attemptID)
}

func newPreparedAttempt(
	engine *Engine,
	plan Plan,
	materialized materializedState,
	epoch uint64,
	commitSeq uint64,
	attemptID uint64,
) PlanAttempt {
	return PlanAttempt{
		state: &planAttemptState{
			engine:       engine,
			plan:         plan,
			materialized: materialized,
			epoch:        epoch,
			commitSeq:    commitSeq,
			attemptID:    attemptID,
			reserved:     true,
		},
	}
}

func newNoopAttempt(plan Plan) PlanAttempt {
	return PlanAttempt{
		state: &planAttemptState{
			plan: plan,
		},
	}
}

func (e *Engine) PreparePlan(reader metrix.Reader) (PlanAttempt, error) {
	plan, materialized, epoch, commitSeq, attemptID, reserved, err := e.preparePlan(reader)
	if err != nil {
		return PlanAttempt{}, err
	}
	if !reserved {
		return newNoopAttempt(plan), nil
	}
	return newPreparedAttempt(e, plan, materialized, epoch, commitSeq, attemptID), nil
}

func (e *Engine) nextAttemptIDLocked() uint64 {
	e.state.nextAttempt++
	if e.state.nextAttempt == 0 {
		e.state.nextAttempt++
	}
	return e.state.nextAttempt
}

func (e *Engine) commitAttempt(materialized materializedState, epoch, commitSeq, attemptID uint64) error {
	if e == nil {
		return fmt.Errorf("chartengine: nil engine")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.state.outstanding != attemptID || e.state.outstanding == 0 {
		return ErrStalePlanAttempt
	}
	if e.state.engineEpoch != epoch || e.state.commitSeq != commitSeq {
		e.state.outstanding = 0
		return ErrStalePlanAttempt
	}

	e.state.materialized = materialized
	e.state.commitSeq++
	e.state.outstanding = 0
	return nil
}

func (e *Engine) abortAttempt(attemptID uint64) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.state.outstanding == attemptID {
		e.state.outstanding = 0
	}
	e.mu.Unlock()
}

func prepareAndCommitPlan(engine *Engine, reader metrix.Reader) (Plan, error) {
	if engine == nil {
		return Plan{}, fmt.Errorf("chartengine: nil engine")
	}
	attempt, err := engine.PreparePlan(reader)
	if err != nil {
		return Plan{}, err
	}

	plan := attempt.Plan()
	if err := attempt.Commit(); err != nil {
		return Plan{}, err
	}
	return plan, nil
}
