// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

type stagedJobResult struct {
	candidate ConstructedJob
	owner     *stagedJobOwner
	failure   *autoDetectionFailure
	err       error
}

// PreparedJobCandidate contains no run-owned attachment capability. The
// process attempt owns its collector and staged Function state until the
// transaction acknowledges installation or rejects the candidate.
type PreparedJobCandidate struct {
	mu sync.Mutex

	factory  *Factory
	attempts jobmgr.ProcessAttemptAuthority
	identity jobmgr.ProcessAttemptIdentity
	target   uint64
	config   confgroup.Config
	attempt  jobmgr.ProcessAttempt
	result   stagedJobResult

	ctx      context.Context
	cancel   context.CancelCauseFunc
	ready    chan struct{}
	started  bool
	settled  bool
	taken    bool
	released bool

	startOnce sync.Once
	readyOnce sync.Once
}

type stagedJobOwner struct {
	mu sync.Mutex

	resources  ConstructedJob
	installing bool
	decided    bool
	attached   bool
	started    bool
	retiring   bool

	startRequests chan stagedJobStartRequest
	retire        chan struct{}
	detached      chan struct{}
	done          chan struct{}
	retireOnce    sync.Once
	detachOnce    sync.Once
	doneOnce      sync.Once
}

type stagedJobStartRequest struct {
	ctx    context.Context
	result chan<- error
}

func newStagedJobOwner(candidate ConstructedJob) *stagedJobOwner {
	return &stagedJobOwner{
		resources:     candidate,
		startRequests: make(chan stagedJobStartRequest),
		retire:        make(chan struct{}),
		detached:      make(chan struct{}),
		done:          make(chan struct{}),
	}
}

func (owner *stagedJobOwner) Replace(resources ConstructedJob) {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	if !owner.decided {
		owner.resources = resources
	}
	owner.mu.Unlock()
}

func (owner *stagedJobOwner) BindAttachment() error {
	if owner == nil {
		return errors.New("job output: nil staged job owner")
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.attached || owner.decided {
		return errors.New("job output: invalid staged job attachment binding")
	}
	owner.attached = true
	return nil
}

func (owner *stagedJobOwner) Reject() {
	if owner == nil {
		return
	}
	owner.mu.Lock()
	if owner.decided {
		owner.mu.Unlock()
		return
	}
	owner.decided = true
	owner.installing = false
	owner.mu.Unlock()
	owner.requestRetirement()
	owner.Detached()
}

func (owner *stagedJobOwner) ReserveInstallation() error {
	if owner == nil {
		return errors.New("job output: nil staged job installation")
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.decided || owner.installing || owner.retiring {
		return errors.New("job output: staged job cannot begin installation")
	}
	owner.installing = true
	return nil
}

func (owner *stagedJobOwner) Install() error {
	if owner == nil {
		return errors.New("job output: nil staged job installation")
	}
	owner.mu.Lock()
	defer owner.mu.Unlock()
	if owner.decided || !owner.installing {
		return errors.New("job output: invalid staged job installation acknowledgement")
	}
	owner.installing = false
	owner.decided = true
	return nil
}

func (owner *stagedJobOwner) Start(ctx context.Context) error {
	if owner == nil || ctx == nil {
		return errors.New("job output: invalid process-owned job start")
	}
	result := make(chan error, 1)
	request := stagedJobStartRequest{
		ctx:    ctx,
		result: result,
	}
	select {
	case owner.startRequests <- request:
	case <-owner.retire:
		return errors.New("job output: process-owned job is retiring")
	case <-owner.done:
		return errors.New("job output: process-owned job is released")
	case <-ctx.Done():
		return ctx.Err()
	}
	select {
	case err := <-result:
		return err
	case <-owner.retire:
		return errors.New("job output: process-owned job retired during start")
	case <-owner.done:
		return errors.New("job output: process-owned job released during start")
	case <-ctx.Done():
		owner.requestRetirement()
		return ctx.Err()
	}
}

func (owner *stagedJobOwner) Retire() {
	if owner == nil {
		return
	}
	owner.requestRetirement()
}

func (owner *stagedJobOwner) Detached() {
	if owner == nil {
		return
	}
	owner.detachOnce.Do(func() {
		owner.mu.Lock()
		runtimeStage := owner.resources.runtimeStage
		vnodeStage := owner.resources.vnodeStage
		owner.mu.Unlock()
		runtimeStage.close()
		vnodeStage.close()
		close(owner.detached)
	})
}

func (owner *stagedJobOwner) requestRetirement() {
	owner.mu.Lock()
	owner.retiring = true
	resources := owner.resources
	owner.mu.Unlock()
	resources.outputGate.Fence()
	owner.retireOnce.Do(func() {
		close(owner.retire)
	})
}

func (owner *stagedJobOwner) finish(ctx context.Context) (resultErr error) {
	if owner == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer owner.doneOnce.Do(func() {
		close(owner.done)
	})

	var request stagedJobStartRequest
	select {
	case request = <-owner.startRequests:
	case <-owner.retire:
		return owner.finalize()
	case <-ctx.Done():
		owner.cutBeforeStart()
		return owner.finalize()
	}

	owner.mu.Lock()
	if owner.started {
		owner.mu.Unlock()
		request.result <- errors.New("job output: duplicate process-owned job start")
		return owner.finalize()
	}
	owner.started = true
	resources := owner.resources
	job := resources.candidateJob
	owner.mu.Unlock()
	if job == nil {
		request.result <- errors.New("job output: missing process-owned runtime job")
		owner.requestRetirement()
		return owner.finalize()
	}

	ready := make(chan struct{})
	exited := make(chan error, 1)
	go func() {
		exited <- callJobLifecycle("process-owned managed runtime", func() error {
			job.StartManaged(ready)
			return nil
		})
	}()

	physicalExited := false
	select {
	case <-ready:
		request.result <- nil
	case resultErr = <-exited:
		physicalExited = true
		request.result <- errors.Join(
			errors.New("job output: process-owned managed loop exited before readiness"),
			resultErr,
		)
		owner.requestRetirement()
	case <-owner.retire:
		request.result <- errors.New("job output: process-owned job retired before readiness")
	case <-ctx.Done():
		owner.Retire()
		request.result <- context.Cause(ctx)
	case <-request.ctx.Done():
		owner.Retire()
		request.result <- request.ctx.Err()
	}

	if !physicalExited {
		select {
		case loopErr := <-exited:
			resultErr = errors.Join(resultErr, loopErr)
			physicalExited = true
			owner.Retire()
		case <-owner.retire:
		case <-ctx.Done():
			owner.Retire()
		}
	}
	if !physicalExited {
		stopErr := callJobLifecycle("process-owned managed runtime Stop", func() error {
			job.Stop()
			return nil
		})
		resultErr = errors.Join(resultErr, stopErr)
		resultErr = errors.Join(resultErr, <-exited)
	}
	return errors.Join(resultErr, owner.finalize())
}

func (owner *stagedJobOwner) cutBeforeStart() {
	owner.Retire()
	owner.mu.Lock()
	attached := owner.attached
	owner.mu.Unlock()
	if !attached {
		owner.Detached()
	}
}

func (owner *stagedJobOwner) finalize() error {
	<-owner.detached
	owner.mu.Lock()
	resources := owner.resources
	owner.resources = ConstructedJob{}
	owner.mu.Unlock()
	resources.outputGate.Fence()
	return finalizeProcessOwnedConstructed(resources)
}

func (f *Factory) NewCandidate(config confgroup.Config) (*PreparedJobCandidate, error) {
	if f == nil ||
		config == nil ||
		f.config.Epoch == 0 ||
		f.config.Attempts == nil ||
		config.FullName() == "" {
		return nil, errors.New("job output: invalid candidate stage")
	}
	vnode, err := f.lookupVNode(config)
	if err != nil {
		return nil, err
	}
	detached := f.config
	detached.Tasks = nil
	detached.RuntimeStaging = detached.Runtime != nil
	detached.Runtime = nil
	detached.HandlerAttacher = nil
	detached.Scheduler = nil
	detached.Observer = nil
	detached.Attempts = nil
	detached.RunWithoutClaims = nil
	if vnode.Vnode != nil {
		name := config.Vnode()
		detached.Vnode = func(candidate string) (jobruntime.VnodeSnapshot, bool) {
			return vnode, candidate == name
		}
	} else {
		detached.Vnode = func(string) (jobruntime.VnodeSnapshot, bool) {
			return jobruntime.VnodeSnapshot{}, false
		}
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &PreparedJobCandidate{
		factory: &Factory{
			config: detached,
		},
		attempts: f.config.Attempts,
		identity: jobmgr.ProcessAttemptIdentity{
			Namespace: jobmgr.ProcessAttemptJob,
			Key:       config.FullName(),
			Resource:  candidateDiagnosticResource(config.FullName()),
		},
		target: f.config.Epoch,
		config: config,
		ctx:    ctx,
		cancel: cancel,
		ready:  make(chan struct{}),
	}, nil
}

func candidateDiagnosticResource(name string) string {
	if name == "" || len(name) > 256 {
		return "collector job"
	}
	for _, char := range name {
		if char < ' ' || char == 0x7f {
			return "collector job"
		}
	}
	return name
}

func (stage *PreparedJobCandidate) Start() {
	if stage == nil {
		return
	}
	stage.startOnce.Do(func() {
		stage.mu.Lock()
		stage.started = true
		stage.mu.Unlock()
		go stage.start()
	})
}

func (stage *PreparedJobCandidate) Ready() <-chan struct{} {
	if stage == nil {
		return nil
	}
	return stage.ready
}

func (stage *PreparedJobCandidate) Cancel(cause error) {
	if stage == nil {
		return
	}
	if cause == nil {
		cause = context.Canceled
	}
	stage.cancel(cause)
	stage.mu.Lock()
	attempt := stage.attempt
	stage.mu.Unlock()
	if attempt != nil {
		attempt.Cut(cause)
	}
}

func (stage *PreparedJobCandidate) Release() {
	if stage == nil {
		return
	}
	stage.mu.Lock()
	if stage.released {
		stage.mu.Unlock()
		return
	}
	stage.released = true
	started := stage.started
	owner := stage.result.owner
	attempt := stage.attempt
	stage.config = nil
	stage.factory = nil
	stage.mu.Unlock()
	stage.cancel(context.Canceled)
	if owner != nil {
		owner.Reject()
	} else if attempt != nil {
		attempt.Cut(context.Canceled)
	}
	if !started {
		stage.publish(stagedJobResult{err: context.Canceled})
	}
}

func (stage *PreparedJobCandidate) start() {
	if cause := context.Cause(stage.ctx); cause != nil {
		stage.publish(stagedJobResult{err: cause})
		return
	}
	attemptReady := make(chan jobmgr.ProcessAttempt, 1)
	attempt, err := stage.attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
		Identity: stage.identity,
		Target:   stage.target,
		Work: func(ctx context.Context) error {
			owned := <-attemptReady
			return stage.run(ctx, owned)
		},
	})
	if err != nil {
		stage.publish(stagedJobResult{err: err})
		return
	}
	stage.mu.Lock()
	stage.attempt = attempt
	stage.mu.Unlock()
	attemptReady <- attempt
	if cause := context.Cause(stage.ctx); cause != nil {
		attempt.Cut(cause)
	}
	err = attempt.Await(context.Background())
	stage.publish(stagedJobResult{err: err})
}

func (stage *PreparedJobCandidate) run(
	ctx context.Context,
	attempt jobmgr.ProcessAttempt,
) error {
	stage.mu.Lock()
	factory := stage.factory
	config := stage.config
	stage.mu.Unlock()
	if factory == nil || config == nil {
		return context.Canceled
	}
	cloned, err := config.Clone()
	if err != nil {
		stage.publish(stagedJobResult{err: err})
		return err
	}
	candidate, err := factory.build(ctx, cloned)
	factory = nil
	config = nil
	if err != nil {
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		err = errors.Join(err, cleanupErr)
		stage.publish(stagedJobResult{err: err})
		return err
	}
	probeErr := probeConstructed(ctx, candidate, candidate.autoDetection)
	if probeErr != nil {
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		if cleanupErr != nil {
			err = errors.Join(probeErr, cleanupErr)
			stage.publish(stagedJobResult{err: err})
			return err
		}
		var failure *autoDetectionFailure
		if errors.As(probeErr, &failure) {
			stage.publish(stagedJobResult{failure: failure})
			return nil
		}
		stage.publish(stagedJobResult{err: probeErr})
		return probeErr
	}
	if err := attempt.Admit(); err != nil {
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		return errors.Join(err, cleanupErr)
	}
	owner := newStagedJobOwner(candidate)
	stage.publish(stagedJobResult{
		candidate: candidate,
		owner:     owner,
	})
	return owner.finish(ctx)
}

func (stage *PreparedJobCandidate) publish(result stagedJobResult) {
	stage.readyOnce.Do(func() {
		stage.mu.Lock()
		if stage.released {
			if result.owner != nil {
				result.owner.Reject()
			}
		} else {
			stage.result = result
		}
		stage.settled = true
		stage.config = nil
		stage.mu.Unlock()
		close(stage.ready)
	})
}

func (stage *PreparedJobCandidate) take() (stagedJobResult, error) {
	if stage == nil {
		return stagedJobResult{}, errors.New("job output: nil candidate stage")
	}
	select {
	case <-stage.ready:
	default:
		return stagedJobResult{}, errors.New("job output: candidate stage is not ready")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if !stage.settled || stage.taken {
		return stagedJobResult{}, errors.New("job output: candidate stage already consumed")
	}
	stage.taken = true
	result := stage.result
	stage.result.candidate = ConstructedJob{}
	return result, nil
}

func (f *Factory) PrepareCandidate(
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
	stage *PreparedJobCandidate,
) (PreparedJob, *autoDetectionFailure, error) {
	if f == nil || stage == nil || !identity.Valid() {
		return PreparedJob{}, nil, errors.New("job output: invalid staged candidate preparation")
	}
	result, err := stage.take()
	if err != nil {
		return PreparedJob{}, nil, err
	}
	if result.err != nil || result.failure != nil {
		return PreparedJob{}, result.failure, result.err
	}
	prepared, err := prepareCandidateJob(
		identity,
		permit,
		result.candidate,
		f.attachment(),
		result.owner,
		true,
	)
	if err != nil {
		result.owner.Reject()
	}
	return prepared, nil, err
}
