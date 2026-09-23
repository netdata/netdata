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
	journal      planJournal // inline, so an attempt and its journal are one allocation
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
	journal := &a.state.journal
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
	return engine.commitAttempt(materialized, journal, transition, epoch, commitSeq, attemptID)
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
	journal := &a.state.journal
	attemptID := a.state.attemptID
	a.state.mu.Unlock()

	if !reserved || engine == nil {
		return
	}
	engine.abortAttempt(journal, attemptID)
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
	e.state.buildToken++
	state := &planAttemptState{
		journal: newPlanJournal(e.state.buildToken, e.state.hints.journal),
	}
	journal := &state.journal
	// Until the attempt is reserved nothing owns the staged changes, so any exit
	// before then, including a panic, rolls them back.
	defer func() {
		if !state.reserved {
			journal.rollback()
		}
	}()
	plan, materialized, prepared, err := view.buildPlan(reader, retired, journal)
	view.state.hints.journal = journal.sizing()
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
	state.engine = e
	state.plan = plan
	state.materialized = materialized
	state.transition = transition
	state.epoch = e.state.engineEpoch
	state.commitSeq = e.state.commitSeq
	state.attemptID = attemptID
	state.reserved = true
	return PlanAttempt{
		state: state,
	}, nil
}

func (e *Engine) nextAttemptIDLocked() uint64 {
	e.state.nextAttempt++
	if e.state.nextAttempt == 0 {
		e.state.nextAttempt++
	}
	return e.state.nextAttempt
}

func (e *Engine) commitAttempt(
	materialized materializedState,
	journal *planJournal,
	transition *templateTransition,
	epoch, commitSeq, attemptID uint64,
) error {
	if e == nil {
		return fmt.Errorf("chartengine: nil engine")
	}
	e.mu.Lock()
	defer e.mu.Unlock()

	// Load and ResetMaterialized replace the staged state and clear outstanding,
	// so a stale attempt has nothing left to roll back.
	if e.state.outstanding != attemptID || e.state.outstanding == 0 {
		return ErrStalePlanAttempt
	}
	if e.state.engineEpoch != epoch || e.state.commitSeq != commitSeq {
		journal.rollback()
		e.state.outstanding = 0
		return ErrStalePlanAttempt
	}

	if transition != nil {
		transition.install(&e.state)
	}
	e.state.materialized = materialized
	journal.compactAfterCommit(&e.state.materialized)
	e.state.commitSeq++
	e.state.outstanding = 0
	return nil
}

func (e *Engine) abortAttempt(journal *planJournal, attemptID uint64) {
	if e == nil {
		return
	}
	e.mu.Lock()
	if e.state.outstanding == attemptID {
		journal.rollback()
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
