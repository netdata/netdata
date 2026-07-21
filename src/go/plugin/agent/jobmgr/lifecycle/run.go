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

type StoppingRejection struct {
	Generation uint64
}

func (sr *StoppingRejection) Error() string {
	return fmt.Sprintf(
		"jobmgr run %d is stopping",
		sr.Generation,
	)
}

type RunSupervisor struct {
	mu         sync.Mutex         // guards all fields
	generation uint64             // this run's generation number
	admission  bool               // admission is open (external commands accepted)
	dirty      error              // first fatal cause; run is failing closed
	terminal   bool               // run has reached a terminal (quiescent) state
	clock      Clock              // logical/real clock
	timeout    time.Duration      // shutdown budget duration
	shutdown   *ShutdownBudget    // active shutdown budget once BeginShutdown ran
	state      RunTerminalState   // terminal state classification
	observer   RuntimeObserver    // sink for run-level runtime counters
	stopping   chan struct{}      // closed once the stopping cut is published
	stopCause  *StoppingRejection // the generation-bound stopping rejection
	stopped    bool               // stopping cut has been published
}

type RunCensus struct {
	AdmissionRunDrained    bool
	Admission              AdmissionCensus
	KernelDrained          bool
	FunctionCatalogDrained bool
	UIDActive              int
	TransientActive        int
	TransientPending       int
	Inherited              InheritedTaskCensus
	LongLived              LongLivedCensus
	Frame                  FrameCensus
	RunFinalizerComplete   bool
}

func (census RunCensus) Quiescent() bool {
	frameDrained := !census.Frame.Poisoned && !census.Frame.Busy &&
		!census.Frame.PendingControl && census.Frame.RetainedBytes == 0
	return census.AdmissionRunDrained && census.KernelDrained &&
		census.FunctionCatalogDrained && census.UIDActive == 0 &&
		census.TransientActive == 0 && census.TransientPending == 0 &&
		census.Inherited.Active == 0 &&
		census.LongLived == (LongLivedCensus{}) && frameDrained &&
		census.RunFinalizerComplete
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
	return &RunSupervisor{
		generation: generation,
		clock:      clock,
		timeout:    shutdownTimeout,
		stopping:   make(chan struct{}),
		stopCause: &StoppingRejection{
			Generation: generation,
		},
	}, nil
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
		rs.shutdown != nil || rs.stopped || rs.terminal ||
		rs.dirty != nil {
		return errors.New("jobmgr run supervisor: runtime observer bound after activation")
	}
	rs.observer = observer
	return nil
}

func (rs *RunSupervisor) OpenAdmission() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.terminal ||
		rs.stopped ||
		rs.dirty != nil ||
		rs.admission ||
		rs.shutdown != nil {
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
	rs.publishStoppingLocked()
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

func (rs *RunSupervisor) Admitting() bool {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.admission &&
		!rs.stopped &&
		rs.dirty == nil &&
		!rs.terminal
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
	rs.publishStoppingLocked()
	observer := rs.observer
	if first && observer != nil {
		observer.AddRuntimeCounter(RuntimeCounterDirtyRuns, 1)
	}
}

func (rs *RunSupervisor) BeginStopping() {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	rs.publishStoppingLocked()
}

func (rs *RunSupervisor) StoppingCause() *StoppingRejection {
	if rs == nil {
		return nil
	}
	return rs.stopCause
}

func (rs *RunSupervisor) IsStopping() bool {
	if rs == nil {
		return false
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	return rs.stopped
}

func (rs *RunSupervisor) publishStoppingLocked() {
	rs.admission = false
	if rs.stopped {
		return
	}
	rs.stopped = true
	close(rs.stopping)
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
	rs.publishStoppingLocked()
	quiescent := census.Quiescent()
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
