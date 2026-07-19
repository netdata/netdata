// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"time"
)

var ErrRunTerminalReached = errors.New("jobmgr run supervisor: terminal already reached")

type RunSupervisor struct {
	mu         sync.Mutex
	generation uint64
	admission  bool
	dirty      error
	terminal   bool
	clock      Clock
	timeout    time.Duration
	shutdown   *ShutdownBudget
	state      RunTerminalState
	observer   RuntimeObserver
}

type RunCensus struct {
	AdmissionRunDrained  bool
	Admission            AdmissionCensus
	TransientActive      int
	TransientPending     int
	Inherited            InheritedTaskCensus
	LongLived            LongLivedCensus
	Frame                FrameCensus
	RunFinalizerComplete bool
}

type RunTerminalState struct {
	Reached   bool
	Quiescent bool
	Dirty     error
}

func NewRunSupervisor(generation uint64, clock Clock, shutdownTimeout time.Duration) (*RunSupervisor, error) {
	if generation == 0 || clock == nil || shutdownTimeout <= 0 {
		return nil, errors.New("jobmgr run supervisor: invalid generation or shutdown budget")
	}
	return &RunSupervisor{generation: generation, clock: clock, timeout: shutdownTimeout}, nil
}

func (rs *RunSupervisor) BindRuntimeObserver(
	observer RuntimeObserver,
) error {
	if rs == nil || observer == nil {
		return errors.New("jobmgr run supervisor: invalid runtime observer")
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.observer != nil || rs.admission ||
		rs.shutdown != nil || rs.terminal ||
		rs.dirty != nil {
		return errors.New("jobmgr run supervisor: runtime observer bound after activation")
	}
	rs.observer = observer
	return nil
}

func (rs *RunSupervisor) OpenAdmission() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.terminal || rs.dirty != nil || rs.admission || rs.shutdown != nil {
		return errors.New("jobmgr run supervisor: cannot open admission")
	}
	rs.admission = true
	return nil
}

func (rs *RunSupervisor) BeginShutdown() (*ShutdownBudget, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.shutdown != nil {
		return rs.shutdown, nil
	}
	if rs.terminal {
		return nil, errors.New("jobmgr run supervisor: shutdown after terminal")
	}
	rs.admission = false
	budget, err := newShutdownBudget(rs.clock, rs.timeout)
	if err != nil {
		return nil, err
	}
	rs.shutdown = budget
	return budget, nil
}

func (rs *RunSupervisor) FinishShutdown() error {
	rs.mu.Lock()
	budget := rs.shutdown
	rs.mu.Unlock()
	if budget == nil {
		return errors.New("jobmgr run supervisor: shutdown was not started")
	}
	return budget.close()
}

func (rs *RunSupervisor) CloseAdmission() {
	rs.mu.Lock()
	rs.admission = false
	rs.mu.Unlock()
}

func (rs *RunSupervisor) Admitting() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.admission && rs.dirty == nil && !rs.terminal
}

func (rs *RunSupervisor) Dirty(cause error) {
	if cause == nil {
		cause = errors.New("jobmgr run supervisor: unspecified dirty cause")
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.terminal {
		return
	}
	first := rs.dirty == nil
	if first {
		rs.dirty = cause
	}
	rs.admission = false
	observer := rs.observer
	if first && observer != nil {
		observer.AddRuntimeCounter(RuntimeCounterDirtyRuns, 1)
	}
}

func (rs *RunSupervisor) DirtyCause() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.dirty
}

func (rs *RunSupervisor) Terminal(census RunCensus) error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.terminal {
		return errors.Join(ErrRunTerminalReached, rs.state.Dirty)
	}
	if rs.admission {
		return errors.New("jobmgr run supervisor: terminal while admitting")
	}
	frameDrained := !census.Frame.Poisoned && !census.Frame.Busy &&
		!census.Frame.PendingControl && census.Frame.RetainedBytes == 0
	quiescent := census.AdmissionRunDrained && census.TransientActive == 0 &&
		census.TransientPending == 0 && census.Inherited.Active == 0 &&
		census.LongLived == (LongLivedCensus{}) && frameDrained &&
		census.RunFinalizerComplete
	if !quiescent {
		first := rs.dirty == nil
		rs.dirty = errors.Join(
			rs.dirty,
			fmt.Errorf(
				"jobmgr run supervisor: terminal with nonzero process census: %+v",
				census,
			),
		)
		if first && rs.observer != nil {
			rs.observer.AddRuntimeCounter(
				RuntimeCounterDirtyRuns,
				1,
			)
		}
	}
	rs.terminal = true
	rs.state = RunTerminalState{Reached: true, Quiescent: quiescent, Dirty: rs.dirty}
	return rs.dirty
}

func (rs *RunSupervisor) TerminalState() RunTerminalState {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.state
}

func (rs *RunSupervisor) Generation() uint64 {
	return rs.generation
}

// NewRollbackContext returns one run-owned context bounded by the configured
// shutdown budget. It deliberately does not inherit a cancelled command
// context.
func (rs *RunSupervisor) NewRollbackContext() (
	context.Context,
	context.CancelFunc,
	error,
) {
	if rs == nil {
		return nil, nil,
			errors.New("jobmgr run supervisor: nil rollback owner")
	}
	rs.mu.Lock()
	if rs.terminal {
		rs.mu.Unlock()
		return nil, nil,
			errors.New("jobmgr run supervisor: rollback after terminal")
	}
	timeout := rs.timeout
	shutdown := rs.shutdown
	rs.mu.Unlock()
	if shutdown != nil {
		ctx, cancel := context.WithDeadline(
			context.Background(),
			shutdown.Deadline(),
		)
		return ctx, cancel, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	return ctx, cancel, nil
}
