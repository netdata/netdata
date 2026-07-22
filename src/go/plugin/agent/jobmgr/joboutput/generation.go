// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

var (
	ErrPreparedJobConsumed   = errors.New("job output: prepared job consumed")
	ErrJobGenerationMismatch = errors.New("job output: job generation mismatch")
)

type autoDetectionFailure struct {
	cause      error
	retry      bool
	retryAfter int
	coded      bool
	code       int
}

func (adf *autoDetectionFailure) Error() string {
	return adf.cause.Error()
}

func (adf *autoDetectionFailure) Unwrap() error {
	return adf.cause
}

type JobVariant uint8

const (
	JobVariantV1 JobVariant = iota + 1
	JobVariantV2
)

func (jv JobVariant) Valid() bool {
	return jv == JobVariantV1 || jv == JobVariantV2
}

type JobState uint8

const (
	JobAllocated JobState = iota + 1
	JobActivating
	JobReady
	JobPublishing
	JobActive
	JobStopping
	JobStopped
	JobFinalizing
	JobTerminal
	JobAborted
	JobRetained
)

func (js JobState) String() string {
	switch js {
	case JobAllocated:
		return "allocated"
	case JobActivating:
		return "activating"
	case JobReady:
		return "ready"
	case JobPublishing:
		return "publishing"
	case JobActive:
		return "active"
	case JobStopping:
		return "stopping"
	case JobStopped:
		return "stopped"
	case JobFinalizing:
		return "finalizing"
	case JobTerminal:
		return "terminal"
	case JobAborted:
		return "aborted"
	case JobRetained:
		return "retained"
	default:
		return "invalid"
	}
}

type ConstructedJob struct {
	Runtime            jobruntime.Runtime          // collector run loop wrapped as a jobruntime.Runtime
	Handlers           HandlerLifecycle            // Function-handler lifecycle; nil when the job has no Functions
	Observer           lifecycle.RuntimeObserver   // runtime gauge sink for active-job accounting
	CollectorCleanup   func(context.Context) error // opaque collector teardown; swapped reject->final on Accept
	Variant            JobVariant                  // V1 or V2 collector shape
	autoDetection      func(context.Context) error // managed auto-detection probe run during Accept
	autoDetectionEvery func() int                  // retry cadence (seconds) reported by the collector
	finalCleanup       func(context.Context) error // Cleanup() variant installed once the job is accepted
	retryAutoDetection func() bool                 // whether a failed auto-detection should be rescheduled
}

func (cj ConstructedJob) validate() error {
	if !cj.Variant.Valid() || cj.Runtime == nil || cj.CollectorCleanup == nil {
		return errors.New("job output: incomplete constructed job")
	}
	return nil
}

type PreparedJob struct {
	state *preparedJobState
}

type preparedJobState struct {
	mu          sync.Mutex                // guards consumed
	consumed    bool                      // the prepared job has been taken (accept/dispose/reject)
	id          string                    // job full name
	generation  uint64                    // job generation this candidate targets
	constructed ConstructedJob            // the assembled but not-yet-started job
	permit      lifecycle.LongLivedPermit // long-lived permit held until accepted or disposed
}

func prepareJob(
	ctx context.Context,
	id string,
	generation uint64,
	permit lifecycle.LongLivedPermit,
	build func(context.Context) (ConstructedJob, error),
) (PreparedJob, error) {
	identity := lifecycle.ResourceIdentity{
		ID:         id,
		Generation: generation,
	}
	if ctx == nil ||
		id == "" ||
		generation == 0 ||
		!permit.Valid() ||
		permit.Owner() != identity ||
		permit.Class() != lifecycle.LongLivedJob ||
		build == nil {
		return PreparedJob{}, errors.New("job output: invalid construction attempt")
	}
	if err := permit.ValidateLive(); err != nil {
		return PreparedJob{}, err
	}
	if err := permit.ActivateExternal(); err != nil {
		return PreparedJob{}, err
	}
	constructed, err, returned := callConstructJob(ctx, build)
	if !returned {
		return PreparedJob{}, lifecycle.RetainOwnership(err)
	}
	cleanupCtx := context.WithoutCancel(ctx)
	if err != nil {
		if lifecycle.OwnershipRetained(err) {
			return PreparedJob{}, lifecycle.RetainOwnership(errors.Join(
				err,
				cleanupConstructed(cleanupCtx, constructed),
			))
		}
		return PreparedJob{}, errors.Join(err, rejectConstructed(cleanupCtx, constructed, permit))
	}
	if err := constructed.validate(); err != nil {
		return PreparedJob{}, errors.Join(err, rejectConstructed(cleanupCtx, constructed, permit))
	}
	return PreparedJob{
		state: &preparedJobState{
			id:          id,
			generation:  generation,
			constructed: constructed,
			permit:      permit,
		},
	}, nil
}

func (pj PreparedJob) Valid() bool {
	if pj.state == nil {
		return false
	}
	pj.state.mu.Lock()
	defer pj.state.mu.Unlock()
	return !pj.state.consumed
}

func (pj PreparedJob) Identity() lifecycle.ResourceIdentity {
	if pj.state == nil {
		return lifecycle.ResourceIdentity{}
	}
	pj.state.mu.Lock()
	defer pj.state.mu.Unlock()
	if pj.state.consumed {
		return lifecycle.ResourceIdentity{}
	}
	return lifecycle.ResourceIdentity{
		ID:         pj.state.id,
		Generation: pj.state.generation,
	}
}

func (pj PreparedJob) AcceptStart(ctx context.Context, expected uint64) (lifecycle.ReadyResource, error) {
	generation, err := pj.Accept(ctx, expected)
	if err != nil {
		if errors.Is(err, ErrJobGenerationMismatch) {
			return nil, errors.Join(err, pj.Dispose(ctx))
		}
		return nil, err
	}
	if err := generation.Start(ctx); err != nil {
		if generation.State() == JobRetained {
			return generation, err
		}
		return nil, err
	}
	return generation, nil
}

func (pj PreparedJob) Accept(ctx context.Context, generation uint64) (*JobGeneration, error) {
	if ctx == nil {
		return nil, errors.New("job output: nil job acceptance context")
	}
	state, err := pj.takeForGeneration(generation)
	if err != nil {
		return nil, err
	}
	if state.constructed.autoDetection != nil {
		if err := callJobLifecycle("collector autodetection", func() error {
			return state.constructed.autoDetection(ctx)
		}); err != nil {
			failure := &autoDetectionFailure{
				cause: err,
			}
			if state.constructed.retryAutoDetection != nil {
				failure.retry = state.constructed.retryAutoDetection()
			}
			if state.constructed.autoDetectionEvery != nil {
				failure.retryAfter = state.constructed.autoDetectionEvery()
			}
			if coded, ok := errors.AsType[dyncfg.CodedError](err); ok {
				failure.coded = true
				failure.code = coded.DyncfgCode()
				if !dyncfg.IsRetryableError(err) {
					failure.retry = false
				}
			}
			cleanupErr := disposeConstructed(context.WithoutCancel(ctx), state.constructed, state.permit)
			if cleanupErr != nil || ctx.Err() != nil {
				return nil, errors.Join(err, cleanupErr)
			}
			return nil, failure
		}
	}
	if state.constructed.finalCleanup != nil {
		state.constructed.CollectorCleanup = state.constructed.finalCleanup
	}
	return &JobGeneration{
		ID:         state.id,
		Generation: state.generation,
		resources:  state.constructed,
		state:      JobAllocated,
		stopDone:   make(chan struct{}),
		permit:     state.permit,
	}, nil
}

func (pj PreparedJob) Dispose(ctx context.Context) error {
	state, err := pj.take()
	if err != nil {
		return err
	}
	return disposeConstructed(ctx, state.constructed, state.permit)
}

func (pj PreparedJob) reject(ctx context.Context) error {
	state, err := pj.take()
	if err != nil {
		return err
	}
	return rejectConstructed(ctx, state.constructed, state.permit)
}

func (pj PreparedJob) validateLivePermit() error {
	if pj.state == nil {
		return errors.New("job output: unprepared job")
	}
	pj.state.mu.Lock()
	defer pj.state.mu.Unlock()
	if pj.state.consumed {
		return ErrPreparedJobConsumed
	}
	return pj.state.permit.ValidateLive()
}

func (pj PreparedJob) take() (*preparedJobState, error) {
	if pj.state == nil {
		return nil, errors.New("job output: unprepared job")
	}
	pj.state.mu.Lock()
	defer pj.state.mu.Unlock()
	if pj.state.consumed {
		return nil, ErrPreparedJobConsumed
	}
	pj.state.consumed = true
	return pj.state, nil
}

func (pj PreparedJob) takeForGeneration(generation uint64) (*preparedJobState, error) {
	if pj.state == nil {
		return nil, errors.New("job output: unprepared job")
	}
	pj.state.mu.Lock()
	defer pj.state.mu.Unlock()
	if pj.state.consumed {
		return nil, ErrPreparedJobConsumed
	}
	if generation != pj.state.generation {
		return nil, ErrJobGenerationMismatch
	}
	pj.state.consumed = true
	return pj.state, nil
}

type JobGeneration struct {
	resources      ConstructedJob            // the constructed job this generation owns
	permit         lifecycle.LongLivedPermit // long-lived permit backing the generation
	stopErr        error                     // memoized result of the terminal Stop path
	terminalErr    error                     // memoized result of finish()/Finalize()
	stopDone       chan struct{}             // closed when Stop() reaches a terminal stop state
	ID             string                    // job full name
	Generation     uint64                    // lifecycle generation counter for this instance
	mu             sync.Mutex                // guards state + the *Err/*finished fields
	state          JobState                  // current JobState in the lifecycle FSM
	finished       bool                      // finish() has recorded the terminal result
	stopFinished   bool                      // finishStop() has run (stopDone closed)
	observedActive bool                      // active-job gauge currently reflects this generation
}

func (jg *JobGeneration) Identity() lifecycle.ResourceIdentity {
	if jg == nil {
		return lifecycle.ResourceIdentity{}
	}
	return lifecycle.ResourceIdentity{
		ID:         jg.ID,
		Generation: jg.Generation,
	}
}

func (jg *JobGeneration) Start(ctx context.Context) error {
	if jg == nil || ctx == nil {
		return errors.New("job output: invalid JobGeneration start")
	}
	jg.mu.Lock()
	if jg.state != JobAllocated {
		state := jg.state
		jg.mu.Unlock()
		return fmt.Errorf("job output: start from state %s", state)
	}
	jg.state = JobActivating
	jg.mu.Unlock()

	if err := callJobLifecycle("runtime Start", func() error {
		return jg.resources.Runtime.Start(ctx)
	}); err != nil {
		cleanupErr := disposeConstructed(context.WithoutCancel(ctx), jg.resources, jg.permit)
		state := JobAborted
		if cleanupErr != nil {
			state = JobRetained
		}
		return jg.finish(state, errors.Join(err, cleanupErr))
	}
	jg.mu.Lock()
	jg.state = JobReady
	jg.mu.Unlock()
	return nil
}

func (jg *JobGeneration) Publish() error {
	if jg == nil {
		return errors.New("job output: nil JobGeneration")
	}
	jg.mu.Lock()
	if jg.state != JobReady {
		state := jg.state
		jg.mu.Unlock()
		return fmt.Errorf("job output: publish from state %s", state)
	}
	jg.state = JobPublishing
	handlers := jg.resources.Handlers
	jg.mu.Unlock()
	if handlers != nil {
		if err := callJobLifecycle("job publication", handlers.Publish); err != nil {
			jg.mu.Lock()
			jg.state = JobReady
			jg.mu.Unlock()
			return err
		}
	}
	jg.mu.Lock()
	jg.state = JobActive
	observer := jg.resources.Observer
	jg.observedActive = true
	jg.mu.Unlock()
	if observer != nil {
		observer.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, 1)
	}
	return nil
}

func (jg *JobGeneration) AbortReady(ctx context.Context) error {
	if jg == nil || ctx == nil {
		return errors.New("job output: invalid ready abort")
	}
	jg.mu.Lock()
	if jg.state != JobReady {
		state := jg.state
		jg.mu.Unlock()
		return fmt.Errorf("job output: ready abort from state %s", state)
	}
	jg.state = JobStopping
	jg.mu.Unlock()
	err := disposeConstructed(ctx, jg.resources, jg.permit)
	state := JobAborted
	if err != nil {
		state = JobRetained
	}
	return jg.finish(state, err)
}

func (jg *JobGeneration) Stop(ctx context.Context) error {
	if jg == nil || ctx == nil {
		return errors.New("job output: invalid JobGeneration stop")
	}
	jg.mu.Lock()
	switch jg.state {
	case JobStopped:
		err := jg.stopErr
		jg.mu.Unlock()
		return err
	case JobTerminal, JobAborted, JobRetained:
		err := jg.terminalErr
		jg.mu.Unlock()
		return err
	case JobStopping:
		done := jg.stopDone
		jg.mu.Unlock()
		select {
		case <-done:
			jg.mu.Lock()
			err := jg.stopErr
			jg.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case JobActive:
		jg.state = JobStopping
		observer := jg.resources.Observer
		wasActive := jg.observedActive
		jg.observedActive = false
		jg.mu.Unlock()
		if wasActive && observer != nil {
			observer.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, -1)
		}
	default:
		state := jg.state
		jg.mu.Unlock()
		return fmt.Errorf("job output: stop from state %s", state)
	}

	if jg.resources.Handlers != nil {
		if err := callJobLifecycle("handler close/drain", func() error {
			return jg.resources.Handlers.CloseAndDrain(ctx)
		}); err != nil {
			return jg.finishStop(JobRetained, err)
		}
	}
	if err := callJobLifecycle("runtime Stop", func() error {
		return jg.resources.Runtime.Stop(ctx)
	}); err != nil {
		return jg.finishStop(JobRetained, err)
	}
	if err := callJobLifecycle("runtime post-cleanup release", func() error {
		return jg.resources.Runtime.ReleaseAfterCleanup(ctx)
	}); err != nil {
		return jg.finishStop(JobRetained, err)
	}
	if err := callJobLifecycle("collector Cleanup", func() error {
		return jg.resources.CollectorCleanup(ctx)
	}); err != nil {
		return jg.finishStop(JobRetained, err)
	}
	if err := callJobLifecycle("job external resource release", func() error {
		return jg.permit.ReleaseExternal()
	}); err != nil {
		return jg.finishStop(JobRetained, err)
	}
	return jg.finishStop(JobStopped, nil)
}

func (jg *JobGeneration) Finalize() error {
	if jg == nil {
		return errors.New("job output: nil JobGeneration")
	}
	jg.mu.Lock()
	switch jg.state {
	case JobTerminal:
		err := jg.terminalErr
		jg.mu.Unlock()
		return err
	case JobRetained, JobAborted:
		err := jg.terminalErr
		jg.mu.Unlock()
		return err
	case JobStopped:
		jg.state = JobFinalizing
		jg.mu.Unlock()
	default:
		state := jg.state
		jg.mu.Unlock()
		return fmt.Errorf("job output: finalize from state %s", state)
	}
	if err := jg.permit.Return(); err != nil {
		return jg.finish(JobRetained, err)
	}
	return jg.finish(JobTerminal, nil)
}

func (jg *JobGeneration) State() JobState {
	if jg == nil {
		return 0
	}
	jg.mu.Lock()
	defer jg.mu.Unlock()
	return jg.state
}

func (jg *JobGeneration) finish(state JobState, err error) error {
	jg.mu.Lock()
	defer jg.mu.Unlock()
	if jg.finished {
		return jg.terminalErr
	}
	jg.state = state
	jg.terminalErr = err
	jg.finished = true
	return err
}

func (jg *JobGeneration) finishStop(state JobState, err error) error {
	jg.mu.Lock()
	defer jg.mu.Unlock()
	if jg.stopFinished {
		return jg.stopErr
	}
	jg.state = state
	jg.stopErr = err
	jg.stopFinished = true
	close(jg.stopDone)
	if state == JobRetained && !jg.finished {
		jg.terminalErr = err
		jg.finished = true
	}
	return err
}

func rejectConstructed(ctx context.Context, constructed ConstructedJob, permit lifecycle.LongLivedPermit) error {
	if err := cleanupConstructed(ctx, constructed); err != nil {
		return lifecycle.RetainOwnership(err)
	}
	if err := callJobLifecycle("job external resource release", func() error {
		return permit.ReleaseExternal()
	}); err != nil {
		return lifecycle.RetainOwnership(err)
	}
	return nil
}

func cleanupConstructed(ctx context.Context, constructed ConstructedJob) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if constructed.Handlers != nil {
		if err := callJobLifecycle("handler close/drain", func() error {
			return constructed.Handlers.CloseAndDrain(ctx)
		}); err != nil {
			return err
		}
	}
	if constructed.Runtime != nil {
		if err := callJobLifecycle("runtime Abort", func() error {
			return constructed.Runtime.Abort(ctx)
		}); err != nil {
			return err
		}
	}
	if constructed.CollectorCleanup != nil {
		if err := callJobLifecycle("collector Cleanup", func() error {
			return constructed.CollectorCleanup(ctx)
		}); err != nil {
			return err
		}
	}
	return nil
}

func disposeConstructed(ctx context.Context, constructed ConstructedJob, permit lifecycle.LongLivedPermit) error {
	if err := rejectConstructed(ctx, constructed, permit); err != nil {
		return err
	}
	return callJobLifecycle("permit return", permit.Return)
}

func callConstructJob(
	ctx context.Context,
	build func(context.Context) (ConstructedJob, error),
) (constructed ConstructedJob, err error, returned bool) {
	defer func() {
		if recovered := recover(); recovered != nil {
			constructed = ConstructedJob{}
			err = lifecycle.RetainOwnership(fmt.Errorf(
				"%w in job construction: %v",
				lifecycle.ErrTaskPanic,
				recovered,
			))
		}
	}()
	constructed, err = build(ctx)
	return constructed, err, true
}

func callJobLifecycle(name string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in job %s: %v", lifecycle.ErrTaskPanic, name, recovered)
		}
	}()
	return call()
}
