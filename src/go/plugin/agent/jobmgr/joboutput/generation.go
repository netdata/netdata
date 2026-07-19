// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

var (
	ErrPreparedJobConsumed   = errors.New("job output: prepared job consumed")
	ErrJobGenerationMismatch = errors.New("job output: job generation mismatch")
)

type JobVariant uint8

const (
	JobVariantV1 JobVariant = iota + 1
	JobVariantV2
)

func (variant JobVariant) Valid() bool {
	return variant == JobVariantV1 || variant == JobVariantV2
}

func (variant JobVariant) String() string {
	switch variant {
	case JobVariantV1:
		return "v1"
	case JobVariantV2:
		return "v2"
	default:
		return "invalid"
	}
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

type ConstructedJob struct {
	Variant          JobVariant
	Runtime          jobruntime.Runtime
	FrameOwner       *lifecycle.FrameOwner
	PrepareCleanup   func(uint64) (PreparedVNodeFrame, error)
	SuppressCleanup  bool
	Handlers         HandlerLifecycle
	ReleaseVNode     func() error
	CollectorCleanup func(context.Context) error
	Carrier          lifecycle.LongLivedCarrier
	Observer         lifecycle.RuntimeObserver
}

func (constructed ConstructedJob) validate() error {
	if !constructed.Variant.Valid() ||
		constructed.Runtime == nil ||
		constructed.CollectorCleanup == nil ||
		constructed.Carrier == nil ||
		!constructed.Carrier.Valid() ||
		constructed.Carrier.Class() != lifecycle.LongLivedJob {
		return errors.New("job output: incomplete constructed job")
	}
	if (constructed.PrepareCleanup == nil) == !constructed.SuppressCleanup {
		return errors.New("job output: cleanup output must be prepared or explicitly suppressed")
	}
	if constructed.PrepareCleanup != nil && constructed.FrameOwner == nil {
		return errors.New("job output: cleanup output has no FrameOwner")
	}
	return nil
}

type PreparedJob struct {
	state *preparedJobState
}

type preparedJobState struct {
	mu          sync.Mutex
	consumed    bool
	id          string
	generation  uint64
	constructed ConstructedJob
}

func PrepareJob(
	ctx context.Context,
	id string,
	generation uint64,
	carrier lifecycle.LongLivedCarrier,
	build func(context.Context) (ConstructedJob, error),
) (PreparedJob, error) {
	identity := lifecycle.ResourceIdentity{ID: id, Generation: generation}
	if ctx == nil ||
		id == "" ||
		generation == 0 ||
		carrier == nil ||
		!carrier.Valid() ||
		carrier.Owner() != identity ||
		carrier.Class() != lifecycle.LongLivedJob ||
		build == nil {
		return PreparedJob{}, errors.New("job output: invalid construction attempt")
	}
	constructed, err := build(ctx)
	constructed.Carrier = carrier
	cleanupCtx := context.WithoutCancel(ctx)
	if hasConstructedJobResources(constructed) {
		if activateErr := carrier.ActivateExternal(lifecycle.LongLivedEJobResources); activateErr != nil {
			return PreparedJob{}, errors.Join(err, activateErr, disposeConstructed(cleanupCtx, constructed))
		}
	}
	if err != nil {
		return PreparedJob{}, errors.Join(err, disposeConstructed(cleanupCtx, constructed))
	}
	if err := constructed.validate(); err != nil {
		return PreparedJob{}, errors.Join(err, disposeConstructed(cleanupCtx, constructed))
	}
	return PreparedJob{state: &preparedJobState{
		id: id, generation: generation, constructed: constructed,
	}}, nil
}

func (prepared PreparedJob) Valid() bool {
	if prepared.state == nil {
		return false
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	return !prepared.state.consumed
}

func (prepared PreparedJob) Identity() lifecycle.ResourceIdentity {
	if prepared.state == nil {
		return lifecycle.ResourceIdentity{}
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.consumed {
		return lifecycle.ResourceIdentity{}
	}
	return lifecycle.ResourceIdentity{
		ID: prepared.state.id, Generation: prepared.state.generation,
	}
}

func (prepared PreparedJob) AcceptStart(
	ctx context.Context,
	expected uint64,
) (lifecycle.ReadyResource, error) {
	generation, err := prepared.Accept(ctx, expected)
	if err != nil {
		if errors.Is(err, ErrJobGenerationMismatch) {
			return nil, errors.Join(err, prepared.Dispose(ctx))
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

func (prepared PreparedJob) Accept(
	_ context.Context,
	generation uint64,
) (*JobGeneration, error) {
	if prepared.state == nil {
		return nil, errors.New("job output: unprepared job")
	}
	prepared.state.mu.Lock()
	if prepared.state.consumed {
		prepared.state.mu.Unlock()
		return nil, ErrPreparedJobConsumed
	}
	if generation != prepared.state.generation {
		prepared.state.mu.Unlock()
		return nil, ErrJobGenerationMismatch
	}
	prepared.state.consumed = true
	state := prepared.state
	prepared.state.mu.Unlock()
	return &JobGeneration{
		ID: state.id, Generation: state.generation, Variant: state.constructed.Variant,
		resources: state.constructed, state: JobAllocated,
		done: make(chan struct{}), stopDone: make(chan struct{}),
	}, nil
}

func (prepared PreparedJob) Dispose(ctx context.Context) error {
	state, err := prepared.take()
	if err != nil {
		return err
	}
	return disposeConstructed(ctx, state.constructed)
}

func (prepared PreparedJob) take() (*preparedJobState, error) {
	if prepared.state == nil {
		return nil, errors.New("job output: unprepared job")
	}
	prepared.state.mu.Lock()
	defer prepared.state.mu.Unlock()
	if prepared.state.consumed {
		return nil, ErrPreparedJobConsumed
	}
	prepared.state.consumed = true
	return prepared.state, nil
}

type JobGeneration struct {
	ID         string
	Generation uint64
	Variant    JobVariant

	mu             sync.Mutex
	resources      ConstructedJob
	state          JobState
	terminalErr    error
	done           chan struct{}
	finished       bool
	stopDone       chan struct{}
	stopErr        error
	stopFinished   bool
	observedActive bool
}

func (generation *JobGeneration) Identity() lifecycle.ResourceIdentity {
	if generation == nil {
		return lifecycle.ResourceIdentity{}
	}
	return lifecycle.ResourceIdentity{
		ID: generation.ID, Generation: generation.Generation,
	}
}

func (generation *JobGeneration) Start(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("job output: invalid JobGeneration start")
	}
	generation.mu.Lock()
	if generation.state != JobAllocated {
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("job output: start from state %d", state)
	}
	generation.state = JobActivating
	generation.mu.Unlock()

	if err := callJobLifecycle("runtime Start", func() error {
		return generation.resources.Runtime.Start(ctx)
	}); err != nil {
		cleanupErr := disposeConstructed(context.WithoutCancel(ctx), generation.resources)
		state := JobAborted
		if cleanupErr != nil {
			state = JobRetained
		}
		return generation.finish(state, errors.Join(err, cleanupErr))
	}
	generation.mu.Lock()
	generation.state = JobReady
	generation.mu.Unlock()
	return nil
}

func (generation *JobGeneration) Publish() error {
	if generation == nil {
		return errors.New("job output: nil JobGeneration")
	}
	generation.mu.Lock()
	if generation.state != JobReady {
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("job output: publish from state %d", state)
	}
	generation.state = JobPublishing
	handlers := generation.resources.Handlers
	generation.mu.Unlock()
	if handlers != nil {
		if err := callJobLifecycle("job publication", handlers.Publish); err != nil {
			generation.mu.Lock()
			generation.state = JobReady
			generation.mu.Unlock()
			return err
		}
	}
	generation.mu.Lock()
	generation.state = JobActive
	observer := generation.resources.Observer
	generation.observedActive = true
	generation.mu.Unlock()
	if observer != nil {
		observer.AddRuntimeGauge(
			lifecycle.RuntimeGaugeJobsActive,
			1,
		)
	}
	return nil
}

func (generation *JobGeneration) AbortReady(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("job output: invalid ready abort")
	}
	generation.mu.Lock()
	if generation.state != JobReady {
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("job output: ready abort from state %d", state)
	}
	generation.state = JobStopping
	generation.mu.Unlock()
	err := disposeConstructed(ctx, generation.resources)
	state := JobAborted
	if err != nil {
		state = JobRetained
	}
	return generation.finish(state, err)
}

func (generation *JobGeneration) Stop(ctx context.Context) error {
	if generation == nil || ctx == nil {
		return errors.New("job output: invalid JobGeneration stop")
	}
	generation.mu.Lock()
	switch generation.state {
	case JobStopped:
		err := generation.stopErr
		generation.mu.Unlock()
		return err
	case JobTerminal, JobAborted, JobRetained:
		err := generation.terminalErr
		generation.mu.Unlock()
		return err
	case JobStopping:
		done := generation.stopDone
		generation.mu.Unlock()
		select {
		case <-done:
			generation.mu.Lock()
			err := generation.stopErr
			generation.mu.Unlock()
			return err
		case <-ctx.Done():
			return ctx.Err()
		}
	case JobActive:
		generation.state = JobStopping
		observer := generation.resources.Observer
		wasActive := generation.observedActive
		generation.observedActive = false
		generation.mu.Unlock()
		if wasActive && observer != nil {
			observer.AddRuntimeGauge(
				lifecycle.RuntimeGaugeJobsActive,
				-1,
			)
		}
	default:
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("job output: stop from state %d", state)
	}

	if generation.resources.Handlers != nil {
		if err := callJobLifecycle("handler close/drain", func() error {
			return generation.resources.Handlers.CloseAndDrain(ctx)
		}); err != nil {
			return generation.finishStop(JobRetained, err)
		}
	}
	if err := callJobLifecycle("runtime Stop", func() error {
		return generation.resources.Runtime.Stop(ctx)
	}); err != nil {
		return generation.finishStop(JobRetained, err)
	}
	if generation.resources.PrepareCleanup != nil {
		prepared, err := callPreparedCleanup(
			generation.resources.PrepareCleanup,
			generation.Generation,
		)
		if err != nil {
			return generation.finishStop(JobRetained, err)
		}
		if prepared.Generation() != generation.Generation {
			return generation.finishStop(
				JobRetained,
				errors.Join(ErrJobGenerationMismatch, prepared.Abort()),
			)
		}
		if err := callJobLifecycle("cleanup frame transfer", func() error {
			return prepared.Transfer(generation.resources.FrameOwner)
		}); err != nil {
			return generation.finishStop(JobRetained, err)
		}
	}
	if err := callJobLifecycle("runtime post-cleanup release", func() error {
		return generation.resources.Runtime.ReleaseAfterCleanup(ctx)
	}); err != nil {
		return generation.finishStop(JobRetained, err)
	}
	if generation.resources.ReleaseVNode != nil {
		if err := callJobLifecycle("vnode release", generation.resources.ReleaseVNode); err != nil {
			return generation.finishStop(JobRetained, err)
		}
	}
	if generation.resources.Handlers != nil {
		if err := callJobLifecycle("handler Cleanup", func() error {
			return generation.resources.Handlers.Cleanup(ctx)
		}); err != nil {
			return generation.finishStop(JobRetained, err)
		}
	}
	if err := callJobLifecycle("collector Cleanup", func() error {
		return generation.resources.CollectorCleanup(ctx)
	}); err != nil {
		return generation.finishStop(JobRetained, err)
	}
	if err := callJobLifecycle("job external facet release", func() error {
		return generation.resources.Carrier.ReleaseExternal(lifecycle.LongLivedEJobResources)
	}); err != nil {
		return generation.finishStop(JobRetained, err)
	}
	if err := callJobLifecycle("job byte facet release", generation.resources.Carrier.ReleaseBytes); err != nil {
		return generation.finishStop(JobRetained, err)
	}
	return generation.finishStop(JobStopped, nil)
}

func (generation *JobGeneration) Finalize() error {
	if generation == nil {
		return errors.New("job output: nil JobGeneration")
	}
	generation.mu.Lock()
	switch generation.state {
	case JobTerminal:
		err := generation.terminalErr
		generation.mu.Unlock()
		return err
	case JobRetained, JobAborted:
		err := generation.terminalErr
		generation.mu.Unlock()
		return err
	case JobStopped:
		generation.state = JobFinalizing
		generation.mu.Unlock()
	default:
		state := generation.state
		generation.mu.Unlock()
		return fmt.Errorf("job output: finalize from state %d", state)
	}
	if err := generation.resources.Carrier.Return(); err != nil {
		return generation.finish(JobRetained, err)
	}
	return generation.finish(JobTerminal, nil)
}

func (generation *JobGeneration) State() JobState {
	if generation == nil {
		return 0
	}
	generation.mu.Lock()
	defer generation.mu.Unlock()
	return generation.state
}

func (generation *JobGeneration) finish(state JobState, err error) error {
	generation.mu.Lock()
	defer generation.mu.Unlock()
	if generation.finished {
		return generation.terminalErr
	}
	generation.state = state
	generation.terminalErr = err
	generation.finished = true
	close(generation.done)
	return err
}

func (generation *JobGeneration) finishStop(state JobState, err error) error {
	generation.mu.Lock()
	defer generation.mu.Unlock()
	if generation.stopFinished {
		return generation.stopErr
	}
	generation.state = state
	generation.stopErr = err
	generation.stopFinished = true
	close(generation.stopDone)
	if state == JobRetained && !generation.finished {
		generation.terminalErr = err
		generation.finished = true
		close(generation.done)
	}
	return err
}

func disposeConstructed(ctx context.Context, constructed ConstructedJob) error {
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
	if constructed.Handlers != nil {
		if err := callJobLifecycle("handler Cleanup", func() error {
			return constructed.Handlers.Cleanup(ctx)
		}); err != nil {
			return err
		}
	}
	if constructed.ReleaseVNode != nil {
		if err := callJobLifecycle("vnode release", constructed.ReleaseVNode); err != nil {
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
	if constructed.Carrier != nil {
		if err := callJobLifecycle("job external facet release", func() error {
			return constructed.Carrier.ReleaseExternal(lifecycle.LongLivedEJobResources)
		}); err != nil {
			return err
		}
		if err := callJobLifecycle("job byte facet release", constructed.Carrier.ReleaseBytes); err != nil {
			return err
		}
		if err := callJobLifecycle("permit return", constructed.Carrier.Return); err != nil {
			return err
		}
	}
	return nil
}

func hasConstructedJobResources(constructed ConstructedJob) bool {
	return constructed.Runtime != nil ||
		constructed.PrepareCleanup != nil ||
		constructed.Handlers != nil ||
		constructed.ReleaseVNode != nil ||
		constructed.CollectorCleanup != nil
}

func callPreparedCleanup(
	prepare func(uint64) (PreparedVNodeFrame, error),
	generation uint64,
) (prepared PreparedVNodeFrame, err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			prepared = PreparedVNodeFrame{}
			err = fmt.Errorf("job output: cleanup preparation panic: %v", recovered)
		}
	}()
	return prepare(generation)
}

func callJobLifecycle(name string, call func() error) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("job output: %s panic: %v", name, recovered)
		}
	}()
	return call()
}
