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
	transition   *templateTransition
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
	transition := a.state.transition
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
	return engine.commitAttempt(materialized, transition, epoch, commitSeq, attemptID)
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

// PlanOptions selects a complete desired snapshot and an optional host reset.
// Nil TemplateSet keeps the current program. ResetMaterialized starts a new host's
// presentation; it never emits retirements for the previous host.
type PlanOptions struct {
	TemplateSet       *TemplateSet
	ResetMaterialized bool
}

func (e *Engine) PreparePlan(reader metrix.Reader) (PlanAttempt, error) {
	return e.PreparePlanWithOptions(reader, PlanOptions{})
}

// PreparePlanWithOptions stages template and lifecycle changes together. Neither
// becomes visible to subsequent plans until the returned attempt is committed.
func (e *Engine) PreparePlanWithOptions(reader metrix.Reader, opts PlanOptions) (PlanAttempt, error) {
	if e == nil {
		return PlanAttempt{}, fmt.Errorf("chartengine: nil engine")
	}
	if reader == nil {
		return PlanAttempt{}, fmt.Errorf("chartengine: nil metrics reader")
	}
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.state.outstanding != 0 {
		return PlanAttempt{}, ErrOutstandingPlanAttempt
	}

	view, transition, retired, err := e.prepareTemplateTransition(opts)
	if err != nil {
		return PlanAttempt{}, err
	}
	plan, materialized, prepared, err := view.buildPlan(reader, retired)
	if view != e {
		// Attempt diagnostics describe builds, including aborted builds. They are
		// independent of the committed presentation and its candidate program.
		e.state.hints = view.state.hints
		e.state.buildSeq = view.state.buildSeq
		e.state.plannerBuildSeq = view.state.plannerBuildSeq
	}
	if err != nil {
		return PlanAttempt{}, err
	}
	if !prepared {
		return newNoopAttempt(plan), nil
	}
	attemptID := e.nextAttemptIDLocked()
	e.state.outstanding = attemptID
	attempt := newPreparedAttempt(e, plan, materialized, e.state.engineEpoch, e.state.commitSeq, attemptID)
	attempt.state.transition = transition
	return attempt, nil
}

func (e *Engine) nextAttemptIDLocked() uint64 {
	e.state.nextAttempt++
	if e.state.nextAttempt == 0 {
		e.state.nextAttempt++
	}
	return e.state.nextAttempt
}

func (e *Engine) commitAttempt(materialized materializedState, transition *templateTransition, epoch, commitSeq, attemptID uint64) error {
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

	if transition != nil {
		transition.install(&e.state)
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
