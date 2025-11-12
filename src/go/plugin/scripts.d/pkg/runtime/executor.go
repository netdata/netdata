// SPDX-License-Identifier: GPL-3.0-or-later

package runtime

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/output"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/spec"
)

// ErrExecutorStopped indicates enqueueing was attempted after the executor stopped.
var ErrExecutorStopped = errors.New("nagios executor stopped")

// ErrNoWorkFunc is returned when no worker function was provided.
var ErrNoWorkFunc = errors.New("nagios executor work function not configured")

// JobRuntime wraps the static job spec plus runtime metadata (IDs, vnode, etc.).
type JobRuntime struct {
	ID    string
	Spec  spec.JobSpec
	Vnode VnodeInfo
}

// ExecutionResult contains the outcome of a plugin invocation.
type ExecutionResult struct {
	Job       JobRuntime
	Output    []byte
	Err       error
	State     string
	ExitCode  int
	Command   string
	Parsed    output.ParsedOutput
	Start     time.Time
	End       time.Time
	Attempt   int
	Duration  time.Duration
	WorkerIdx int
	Usage     ndexec.ResourceUsage
}

// WorkFunc executes a Nagios job and returns the result.
type WorkFunc func(context.Context, JobRuntime) ExecutionResult

// ExecutorConfig encapsulates runtime parameters for the job executor.
type ExecutorConfig struct {
	Logger        *logger.Logger
	Workers       int
	QueueCapacity int
	Work          WorkFunc
}

// Executor orchestrates asynchronous job execution using a bounded queue and worker pool.
type Executor struct {
	cfg ExecutorConfig

	ctx    context.Context
	cancel context.CancelFunc

	queue       chan JobRuntime
	results     chan ExecutionResult
	resetNeeded bool

	waitingMu sync.Mutex
	waiting   map[string]struct{}
	executing map[string]struct{}

	wg sync.WaitGroup

	skipped atomic.Uint64
}

// ExecutorStats captures instantaneous telemetry for the executor internals.
type ExecutorStats struct {
	QueueDepth     int
	Executing      int
	Waiting        int
	SkippedEnqueue uint64
}

// NewExecutor returns an executor configured with the provided parameters.
func NewExecutor(cfg ExecutorConfig) (*Executor, error) {
	if cfg.Workers <= 0 {
		return nil, fmt.Errorf("executor workers must be > 0")
	}
	if cfg.QueueCapacity <= 0 {
		return nil, fmt.Errorf("executor queue capacity must be > 0")
	}
	if cfg.Work == nil {
		return nil, ErrNoWorkFunc
	}

	exec := &Executor{
		cfg:       cfg,
		waiting:   make(map[string]struct{}),
		executing: make(map[string]struct{}),
	}
	exec.resetChannels()

	return exec, nil
}

func (e *Executor) resetChannels() {
	e.queue = make(chan JobRuntime, e.cfg.QueueCapacity)
	e.results = make(chan ExecutionResult, e.cfg.QueueCapacity)
}

// Start initializes the worker pool and begins draining the waiting queue.
func (e *Executor) Start(ctx context.Context) {
	if e.ctx != nil {
		return
	}
	if e.resetNeeded {
		e.resetChannels()
		e.resetNeeded = false
	}
	e.ctx, e.cancel = context.WithCancel(ctx)
	for i := 0; i < e.cfg.Workers; i++ {
		e.wg.Add(1)
		go e.workerLoop(i)
	}
}

// Stop cancels workers and waits for them to exit.
func (e *Executor) Stop() {
	if e.cancel == nil {
		return
	}
	e.cancel()
	e.wg.Wait()
	close(e.results)
	e.cancel = nil
	e.ctx = nil
	e.resetNeeded = true
}

// Enqueue schedules a job for execution respecting single-flight semantics.
// Returns true when the job was queued, false if it was already queued or executing.
func (e *Executor) Enqueue(job JobRuntime) (bool, error) {
	if e.ctx == nil {
		return false, ErrExecutorStopped
	}

	e.waitingMu.Lock()
	if _, running := e.executing[job.ID]; running {
		e.waitingMu.Unlock()
		e.skipped.Add(1)
		return false, nil
	}
	if _, queued := e.waiting[job.ID]; queued {
		e.waitingMu.Unlock()
		e.skipped.Add(1)
		return false, nil
	}
	e.waiting[job.ID] = struct{}{}
	e.waitingMu.Unlock()

	select {
	case e.queue <- job:
		return true, nil
	case <-e.ctx.Done():
		e.waitingMu.Lock()
		delete(e.waiting, job.ID)
		e.waitingMu.Unlock()
		return false, ErrExecutorStopped
	}
}

// Results exposes the channel of execution results for the scheduler/state machine.
func (e *Executor) Results() <-chan ExecutionResult {
	return e.results
}

// Stats returns a snapshot of executor internals for telemetry.
func (e *Executor) Stats() ExecutorStats {
	e.waitingMu.Lock()
	waiting := len(e.waiting)
	executing := len(e.executing)
	e.waitingMu.Unlock()

	return ExecutorStats{
		QueueDepth:     len(e.queue),
		Waiting:        waiting,
		Executing:      executing,
		SkippedEnqueue: e.skipped.Load(),
	}
}

func (e *Executor) workerLoop(idx int) {
	defer e.wg.Done()

	for {
		select {
		case <-e.ctx.Done():
			return
		case job := <-e.queue:
			// Mark the job as executing and remove from waiting map.
			e.waitingMu.Lock()
			delete(e.waiting, job.ID)
			e.executing[job.ID] = struct{}{}
			e.waitingMu.Unlock()

			res := e.cfg.Work(e.ctx, job)
			res.WorkerIdx = idx

			e.waitingMu.Lock()
			delete(e.executing, job.ID)
			e.waitingMu.Unlock()

			select {
			case e.results <- res:
			case <-e.ctx.Done():
				return
			}
		}
	}
}
