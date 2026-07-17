// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"sync"
	"time"
)

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
}

type RunCensus struct {
	AdmissionRunDrained  bool
	Admission            AdmissionCensus
	TransientActive      int
	TransientPending     int
	Inherited            InheritedTaskCensus
	LongLived            LongLivedCensus
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

func (supervisor *RunSupervisor) OpenAdmission() error {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	if supervisor.terminal || supervisor.dirty != nil || supervisor.admission || supervisor.shutdown != nil {
		return errors.New("jobmgr run supervisor: cannot open admission")
	}
	supervisor.admission = true
	return nil
}

func (supervisor *RunSupervisor) BeginShutdown() (*ShutdownBudget, error) {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	if supervisor.shutdown != nil {
		return supervisor.shutdown, nil
	}
	if supervisor.terminal {
		return nil, errors.New("jobmgr run supervisor: shutdown after terminal")
	}
	supervisor.admission = false
	budget, err := newShutdownBudget(supervisor.clock, supervisor.timeout)
	if err != nil {
		return nil, err
	}
	supervisor.shutdown = budget
	return budget, nil
}

func (supervisor *RunSupervisor) FinishShutdown() error {
	supervisor.mu.Lock()
	budget := supervisor.shutdown
	supervisor.mu.Unlock()
	if budget == nil {
		return errors.New("jobmgr run supervisor: shutdown was not started")
	}
	return budget.close()
}

func (supervisor *RunSupervisor) CloseAdmission() {
	supervisor.mu.Lock()
	supervisor.admission = false
	supervisor.mu.Unlock()
}

func (supervisor *RunSupervisor) Admitting() bool {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	return supervisor.admission && supervisor.dirty == nil && !supervisor.terminal
}

func (supervisor *RunSupervisor) Dirty(cause error) {
	if cause == nil {
		cause = errors.New("jobmgr run supervisor: unspecified dirty cause")
	}
	supervisor.mu.Lock()
	if supervisor.dirty == nil {
		supervisor.dirty = cause
	}
	supervisor.admission = false
	supervisor.mu.Unlock()
}

func (supervisor *RunSupervisor) DirtyCause() error {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	return supervisor.dirty
}

func (supervisor *RunSupervisor) Terminal(census RunCensus) error {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	if supervisor.admission {
		return errors.New("jobmgr run supervisor: terminal while admitting")
	}
	quiescent := census.AdmissionRunDrained && census.TransientActive == 0 && census.TransientPending == 0 && census.Inherited.Active == 0 && census.LongLived == (LongLivedCensus{}) && census.RunFinalizerComplete
	if !quiescent {
		if supervisor.dirty == nil {
			supervisor.dirty = errors.New("jobmgr run supervisor: terminal with nonzero process census")
		}
	}
	supervisor.terminal = true
	supervisor.state = RunTerminalState{Reached: true, Quiescent: quiescent, Dirty: supervisor.dirty}
	return supervisor.dirty
}

func (supervisor *RunSupervisor) TerminalState() RunTerminalState {
	supervisor.mu.Lock()
	defer supervisor.mu.Unlock()
	return supervisor.state
}

func (supervisor *RunSupervisor) Generation() uint64 {
	return supervisor.generation
}
