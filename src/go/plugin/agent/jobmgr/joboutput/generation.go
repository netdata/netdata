// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

var (
	ErrPreparedJobConsumed   = errors.New("job output: prepared job consumed")
	ErrJobGenerationMismatch = errors.New("job output: job generation mismatch")
	ErrStaleStoreGeneration  = errors.New("job output: Store changed during candidate preparation")
)

type autoDetectionFailure struct {
	cause              error
	retry              bool
	retryAfter         int
	coded              bool
	code               int
	jobConfigLifecycle collectorapi.JobConfigLifecycleSnapshot
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
	Handlers           ProcessHandlerLifecycle     // Function-handler lifecycle; nil when the job has no Functions
	StagedHandlers     StagedHandlerLifecycle      // run-detached Function handlers before attachment
	Observer           lifecycle.RuntimeObserver   // runtime gauge sink for active-job accounting
	CollectorCleanup   func(context.Context) error // opaque collector teardown; swapped reject->final on Accept
	Variant            JobVariant                  // V1 or V2 collector shape
	autoDetection      func(context.Context) error // managed auto-detection probe run before acceptance
	autoDetectionEvery func() int                  // retry cadence (seconds) reported by the collector
	finalCleanup       func(context.Context) error // Cleanup() variant installed once the job is accepted
	retryAutoDetection func() bool                 // whether a failed auto-detection should be rescheduled
	resolvedReferences bool                        // lifecycle failures may contain resolved secret values
	stageFunctions     bool                        // job Function lifecycle still needs post-probe staging
	attachProjections  func() error                // binds external runtime/vnode projections before acceptance
	attach             func(lifecycle.ResourceIdentity, *stagedJobOwner) (constructedJobAttachment, error)
	candidateJob       RuntimeJob
	runtimeStage       *stagedRuntimeService
	vnodeStage         *stagedVNodeLookup
	outputGate         *generationOutputGate
	storeSnapshot      secretresolver.AtomicScopeSnapshot
	processOwner       *stagedJobOwner
	jobConfigIdentity  collectorapi.JobConfigIdentity
	jobConfigLifecycle collectorapi.JobConfigLifecycle
	jobConfigSnapshot  collectorapi.JobConfigLifecycleSnapshot
}

// constructedJobAttachment reports whether process ownership transferred even
// when later handler attachment returned an error.
type constructedJobAttachment struct {
	resources   ConstructedJob
	transferred bool
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
	owner       *stagedJobOwner           // process owner for staged candidates
}

func prepareCandidateJob(
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
	candidate ConstructedJob,
	attachment factoryAttachment,
	owner *stagedJobOwner,
) (PreparedJob, error) {
	if !identity.Valid() ||
		!permit.Valid() ||
		permit.Owner() != identity ||
		permit.Class() != lifecycle.LongLivedJob ||
		!candidate.Variant.Valid() ||
		candidate.Runtime != nil ||
		candidate.attach != nil ||
		candidate.candidateJob == nil ||
		candidate.candidateJob.FullName() != identity.ID ||
		candidate.CollectorCleanup == nil ||
		candidate.stageFunctions ||
		owner == nil {
		return PreparedJob{}, errors.New("job output: invalid candidate preparation")
	}
	if err := permit.ValidateLive(); err != nil {
		return PreparedJob{}, err
	}
	staged := candidate
	candidate.attach = func(
		identity lifecycle.ResourceIdentity,
		owner *stagedJobOwner,
	) (constructedJobAttachment, error) {
		return attachment.attach(staged, identity, owner)
	}
	return PreparedJob{
		state: &preparedJobState{
			id:          identity.ID,
			generation:  identity.Generation,
			constructed: candidate,
			permit:      permit,
			owner:       owner,
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

func (pj PreparedJob) jobConfigLifecycleState() preparedJobConfigLifecycle {
	if pj.state == nil {
		return preparedJobConfigLifecycle{}
	}
	pj.state.mu.Lock()
	defer pj.state.mu.Unlock()
	if pj.state.consumed {
		return preparedJobConfigLifecycle{}
	}
	return preparedJobConfigLifecycle{
		identity: pj.state.constructed.jobConfigIdentity,
		snapshot: pj.state.constructed.jobConfigSnapshot,
		runtime:  pj.state.constructed.candidateJob,
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
	if err := state.owner.Promote(ctx); err != nil {
		state.owner.Reject()
		return nil, errors.Join(err, state.permit.AbortUnused())
	}
	attachment, attachErr := state.constructed.attach(
		lifecycle.ResourceIdentity{
			ID:         state.id,
			Generation: state.generation,
		},
		state.owner,
	)
	if attachment.transferred {
		state.constructed = attachment.resources
		if err := state.owner.AdoptAttachment(attachment.resources); err != nil {
			state.owner.Reject()
			return nil, errors.Join(attachErr, err, state.permit.AbortUnused())
		}
	} else if attachErr == nil {
		attachErr = errors.New("job output: attachment completed without ownership transfer")
	}
	if attachErr != nil {
		state.owner.Reject()
		return nil, errors.Join(attachErr, state.permit.AbortUnused())
	}
	if state.constructed.attachProjections != nil {
		if err := callJobLifecycle("projection attachment", state.constructed.attachProjections); err != nil {
			state.owner.Reject()
			return nil, errors.Join(err, state.permit.AbortUnused())
		}
	}
	accepted, err := state.owner.AcceptResources(state.permit)
	if err != nil {
		state.owner.Reject()
		return nil, errors.Join(err, state.permit.AbortUnused())
	}
	state.constructed = accepted
	return &JobGeneration{
		ID:           state.id,
		Generation:   state.generation,
		resources:    state.constructed,
		state:        JobAllocated,
		stopDone:     make(chan struct{}),
		permit:       state.permit,
		owner:        state.owner,
		processOwner: state.constructed.processOwner,
	}, nil
}

func probeConstructed(
	ctx context.Context,
	constructed ConstructedJob,
	probe func(context.Context) error,
) error {
	var result error
	if probe != nil {
		result = callJobLifecycle("collector autodetection", func() error {
			return probe(ctx)
		})
	}
	if result == nil {
		return nil
	}
	if constructed.resolvedReferences {
		result = redactResolvedLifecycleError(result)
	}
	return autoDetectionFailureFor(constructed, result)
}

func (pj PreparedJob) Dispose(_ context.Context) error {
	state, err := pj.take()
	if err != nil {
		return err
	}
	state.owner.Reject()
	return state.permit.AbortUnused()
}

func (pj PreparedJob) reject(_ context.Context) error {
	state, err := pj.take()
	if err != nil {
		return err
	}
	state.owner.Reject()
	return nil
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

func autoDetectionFailureFor(constructed ConstructedJob, err error) *autoDetectionFailure {
	failure := &autoDetectionFailure{
		cause: err,
	}
	if constructed.retryAutoDetection != nil {
		failure.retry = constructed.retryAutoDetection()
	}
	if constructed.autoDetectionEvery != nil {
		failure.retryAfter = constructed.autoDetectionEvery()
	}
	if coded, ok := errors.AsType[dyncfg.CodedError](err); ok {
		failure.coded = true
		failure.code = coded.DyncfgCode()
		if !dyncfg.IsRetryableError(err) {
			failure.retry = false
		}
	}
	return failure
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
	owner          *stagedJobOwner           // process owner pending installation acknowledgement
	processOwner   *stagedJobOwner           // process owner through physical runtime finalization
}

func (jg *JobGeneration) reserveInstallation() error {
	if jg == nil {
		return errors.New("job output: nil installation reservation")
	}
	jg.mu.Lock()
	owner := jg.owner
	jg.mu.Unlock()
	if owner == nil {
		return nil
	}
	return owner.ReserveInstallation()
}

func (jg *JobGeneration) acknowledgeInstallation() error {
	if jg == nil {
		return errors.New("job output: nil installation acknowledgement")
	}
	jg.mu.Lock()
	owner := jg.owner
	jg.mu.Unlock()
	if owner == nil {
		return nil
	}
	if err := owner.Install(); err != nil {
		return err
	}
	jg.mu.Lock()
	if jg.owner == owner {
		jg.owner = nil
	}
	jg.mu.Unlock()
	return nil
}

func (jg *JobGeneration) installationPending() bool {
	if jg == nil {
		return false
	}
	jg.mu.Lock()
	defer jg.mu.Unlock()
	return jg.owner != nil
}

func (jg *JobGeneration) settleFailedInstallation(
	ctx context.Context,
) (*JobGeneration, error) {
	if jg == nil || ctx == nil {
		return jg, errors.New("job output: invalid pending installation settlement")
	}
	jg.mu.Lock()
	if jg.owner == nil {
		jg.mu.Unlock()
		return jg, errors.New("job output: pending installation lost its process owner")
	}
	switch jg.state {
	case JobReady, JobActive:
	case JobRetained:
		jg.mu.Unlock()
		return jg, nil
	default:
		state := jg.state
		jg.mu.Unlock()
		return jg, fmt.Errorf("job output: pending installation settlement from state %s", state)
	}
	observer := jg.resources.Observer
	wasActive := jg.observedActive
	jg.observedActive = false
	jg.state = JobStopping
	jg.mu.Unlock()
	if wasActive && observer != nil {
		observer.AddRuntimeGauge(lifecycle.RuntimeGaugeJobsActive, -1)
	}
	err := jg.abortProcessOwned(ctx)
	state := JobAborted
	if err != nil {
		state = JobRetained
	}
	err = jg.finish(state, err)
	if state == JobRetained {
		return jg, err
	}
	return nil, err
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
		if jg.resources.resolvedReferences {
			err = redactResolvedLifecycleError(err)
		}
		cleanupErr := jg.abortProcessOwned(context.WithoutCancel(ctx))
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
			if jg.resources.resolvedReferences {
				err = redactResolvedLifecycleError(err)
			}
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
	err := jg.abortProcessOwned(ctx)
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

	return jg.stopProcessOwned(ctx)
}

func (jg *JobGeneration) stopProcessOwned(ctx context.Context) error {
	jg.resources.outputGate.Fence()
	var detachErr error
	if handlers := jg.resources.Handlers; handlers != nil {
		detachErr = callJobLifecycle("handler detach", func() error {
			return handlers.Detach(ctx)
		})
	}
	runtimeErr := callJobLifecycle("runtime Stop", func() error {
		return jg.resources.Runtime.Stop(ctx)
	})
	if runtimeErr == nil {
		runtimeErr = callJobLifecycle("runtime projection release", func() error {
			return jg.resources.Runtime.ReleaseAfterCleanup(ctx)
		})
	}
	err := errors.Join(detachErr, runtimeErr)
	if err != nil {
		if jg.resources.resolvedReferences {
			err = redactResolvedLifecycleError(err)
		}
		// Detach failure may leave scheduler or Function projections referring
		// to this generation. Keep its process owner attached so final cleanup
		// cannot invalidate resources those projections may still reach.
		return jg.finishStop(JobRetained, err)
	}
	jg.processOwner.Detached()
	if err := callJobLifecycle("job external resource release", func() error {
		return jg.permit.ReleaseExternal()
	}); err != nil {
		return jg.finishStop(JobRetained, err)
	}
	return jg.finishStop(JobStopped, nil)
}

func (jg *JobGeneration) abortProcessOwned(ctx context.Context) error {
	jg.resources.outputGate.Fence()
	jg.processOwner.Retire()
	var detachErr error
	if handlers := jg.resources.Handlers; handlers != nil {
		detachErr = callJobLifecycle("handler detach", func() error {
			return handlers.Detach(ctx)
		})
	}
	runtimeErr := callJobLifecycle("runtime Abort", func() error {
		return jg.resources.Runtime.Abort(ctx)
	})
	if err := errors.Join(detachErr, runtimeErr); err != nil {
		if jg.resources.resolvedReferences {
			err = redactResolvedLifecycleError(err)
		}
		// Detach failure may leave scheduler or Function projections referring
		// to this generation. Keep its process owner attached so final cleanup
		// cannot invalidate resources those projections may still reach.
		return err
	}
	jg.processOwner.Detached()
	jg.processOwner.Reject()
	if err := jg.permit.ReleaseExternal(); err != nil {
		return err
	}
	return jg.permit.Return()
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

func cleanupConstructed(ctx context.Context, constructed ConstructedJob) (err error) {
	if ctx == nil {
		ctx = context.Background()
	}
	constructed.outputGate.Fence()
	defer func() {
		if constructed.resolvedReferences {
			err = redactResolvedLifecycleError(err)
		}
	}()
	if constructed.Handlers != nil {
		if err := callJobLifecycle("handler close/drain", func() error {
			return constructed.Handlers.CloseAndDrain(ctx)
		}); err != nil {
			return err
		}
	}
	if constructed.StagedHandlers != nil {
		if err := callJobLifecycle("staged handler close/drain", func() error {
			return constructed.StagedHandlers.CloseAndDrain(ctx)
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

func callJobLifecycle(name string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("%w in job %s: %v", lifecycle.ErrTaskPanic, name, recovered)
		}
	}()
	return call()
}
