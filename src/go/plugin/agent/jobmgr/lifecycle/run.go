// SPDX-License-Identifier: GPL-3.0-or-later

package lifecycle

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

var (
	ErrRunTerminalReached      = errors.New("jobmgr run supervisor: terminal already reached")
	ErrRunTerminalNonQuiescent = errors.New("jobmgr run supervisor: terminal with nonzero process census")
)

type StoppingRejection struct {
	Generation uint64
}

func (sr *StoppingRejection) Error() string {
	return fmt.Sprintf("jobmgr run %d is stopping", sr.Generation)
}

// ContainsOnlyCurrentStoppingRejections reports whether every leaf in err is a
// stopping rejection for generation. Mixed or malformed error trees fail closed.
func ContainsOnlyCurrentStoppingRejections(err error, generation uint64) bool {
	if generation == 0 {
		return false
	}
	return AllErrorLeavesMatch(err, func(leaf error) bool {
		stopping, ok := leaf.(*StoppingRejection)
		return ok && stopping.Generation == generation
	})
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
	stopCause  *StoppingRejection // the generation-bound stopping rejection
	stopped    bool               // stopping cut has been published
}

type RunCensus struct {
	KernelDrained          bool
	FunctionCatalogDrained bool
	UIDActive              int
	TransientActive        int
	TransientPending       int
	InheritedActive        int
	LongLived              LongLivedCensus
	Frame                  FrameCensus
	Abandoned              TaskAbandonmentCensus
	RunFinalizerComplete   bool
}

func (rc RunCensus) Drained() bool {
	frameDrained := !rc.Frame.Poisoned && !rc.Frame.Busy &&
		!rc.Frame.PendingControl && rc.Frame.RetainedBytes == 0
	return rc.KernelDrained &&
		rc.FunctionCatalogDrained && rc.UIDActive == 0 &&
		rc.TransientActive == 0 && rc.TransientPending == 0 &&
		rc.InheritedActive == 0 &&
		rc.LongLived == (LongLivedCensus{}) && frameDrained &&
		rc.RunFinalizerComplete
}

func (rc RunCensus) Quiescent() bool {
	return rc.Drained() && rc.Abandoned.Empty()
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
		stopCause: &StoppingRejection{
			Generation: generation,
		},
	}, nil
}

func (rs *RunSupervisor) BindRuntimeObserver(observer RuntimeObserver) error {
	if rs == nil || observer == nil {
		return errors.New("jobmgr run supervisor: invalid runtime observer")
	}
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.observer != nil || rs.admission || rs.shutdown != nil || rs.stopped || rs.terminal || rs.dirty != nil {
		return errors.New("jobmgr run supervisor: runtime observer bound after activation")
	}
	rs.observer = observer
	return nil
}

func (rs *RunSupervisor) OpenAdmission() error {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.terminal || rs.stopped || rs.dirty != nil || rs.admission || rs.shutdown != nil {
		return errors.New("jobmgr run supervisor: cannot open admission")
	}
	rs.admission = true
	return nil
}

func (rs *RunSupervisor) BeginShutdown() (*ShutdownBudget, error) {
	return rs.beginShutdown(rs.timeout)
}

// BeginShutdownWithTimeout starts the one run shutdown budget with a
// caller-owned remaining duration. Repeated calls preserve the first budget.
func (rs *RunSupervisor) BeginShutdownWithTimeout(
	timeout time.Duration,
) (*ShutdownBudget, error) {
	return rs.beginShutdown(timeout)
}

func (rs *RunSupervisor) beginShutdown(timeout time.Duration) (*ShutdownBudget, error) {
	rs.mu.Lock()
	defer rs.mu.Unlock()
	if rs.shutdown != nil {
		return rs.shutdown, nil
	}
	if rs.terminal {
		return nil, errors.New("jobmgr run supervisor: shutdown after terminal")
	}
	if timeout <= 0 {
		return nil, errors.New("jobmgr run supervisor: invalid shutdown budget")
	}
	rs.publishStoppingLocked()
	budget, err := newShutdownBudget(rs.clock, timeout)
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
	return rs.admission && !rs.stopped && rs.dirty == nil && !rs.terminal
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
			fmt.Errorf("%w: %+v", ErrRunTerminalNonQuiescent, census),
		)
		if first && rs.observer != nil {
			rs.observer.AddRuntimeCounter(RuntimeCounterDirtyRuns, 1)
		}
	}
	rs.terminal = true
	rs.state = RunTerminalState{
		Reached:   true,
		Quiescent: quiescent,
		Dirty:     rs.dirty,
	}
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
