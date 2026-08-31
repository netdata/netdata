// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
)

type stagedJobResult struct {
	candidate     ConstructedJob
	owner         *stagedJobOwner
	storeSnapshot secretresolver.AtomicScopeSnapshot
	failure       *autoDetectionFailure
	err           error
}

// preparedJobCandidate contains no run-owned attachment capability. The
// process attempt owns its collector and staged Function state until the
// transaction acknowledges installation or rejects the candidate.
type preparedJobCandidate struct {
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

	resources       ConstructedJob
	attempts        jobmgr.ProcessAttemptAuthority
	candidateCtx    context.Context
	runtimeIdentity jobmgr.ProcessAttemptIdentity
	target          uint64
	ownership       stagedJobOwnership
	installing      bool
	decided         bool
	attached        bool
	startRequested  bool
	started         bool
	retiring        bool
	retirementCause error
	finalized       bool

	startRequests       chan stagedJobStartRequest
	retire              chan struct{}
	detached            chan struct{}
	done                chan struct{}
	transferred         chan struct{}
	rejectCandidate     chan struct{}
	retireOnce          sync.Once
	detachOnce          sync.Once
	doneOnce            sync.Once
	transferOnce        sync.Once
	candidateRejectOnce sync.Once
}

type stagedJobOwnership uint8

const (
	stagedJobOwnedByCandidate stagedJobOwnership = iota + 1
	stagedJobPromotionActive
	stagedJobOwnedByRuntime
	stagedJobOwnershipRejected
)

type stagedJobStartRequest struct {
	ctx    context.Context
	result chan<- error
}

func newStagedJobOwner(
	candidateCtx context.Context,
	candidate ConstructedJob,
	attempts jobmgr.ProcessAttemptAuthority,
	target uint64,
	runtimeIdentity jobmgr.ProcessAttemptIdentity,
) *stagedJobOwner {
	if candidateCtx == nil {
		candidateCtx = context.Background()
	}
	return &stagedJobOwner{
		resources:       candidate,
		attempts:        attempts,
		candidateCtx:    candidateCtx,
		runtimeIdentity: runtimeIdentity,
		target:          target,
		ownership:       stagedJobOwnedByCandidate,
		startRequests:   make(chan stagedJobStartRequest),
		retire:          make(chan struct{}),
		detached:        make(chan struct{}),
		done:            make(chan struct{}),
		transferred:     make(chan struct{}),
		rejectCandidate: make(chan struct{}),
	}
}

// AdoptAttachment transfers the process-managed wrapper and any completed or
// partial handler attachment to the process owner. Adoption remains valid while
// retiring so rejection can finalize every transferred capability.
func (sjo *stagedJobOwner) AdoptAttachment(resources ConstructedJob) error {
	if sjo == nil {
		return errors.New("job output: nil staged job attachment adoption")
	}
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.ownership != stagedJobOwnedByRuntime ||
		!sjo.attached ||
		sjo.decided ||
		sjo.finalized {
		return errors.New("job output: invalid staged job attachment adoption")
	}
	sjo.resources = resources
	return nil
}

// AcceptResources linearizes retirement against output and permit activation,
// then freezes the accepted cleanup contract.
func (sjo *stagedJobOwner) AcceptResources(permit lifecycle.LongLivedPermit) (ConstructedJob, error) {
	if sjo == nil {
		return ConstructedJob{}, errors.New("job output: nil staged job acceptance")
	}
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.ownership != stagedJobOwnedByRuntime ||
		!sjo.attached ||
		sjo.decided ||
		sjo.finalized {
		return ConstructedJob{}, errors.New("job output: invalid staged job acceptance")
	}
	if sjo.retiring {
		return ConstructedJob{}, sjo.retirementErrorLocked("job output: invalid staged job acceptance")
	}
	resources := sjo.resources
	if err := resources.outputGate.Activate(); err != nil {
		return ConstructedJob{}, err
	}
	if err := permit.ActivateExternal(); err != nil {
		return ConstructedJob{}, err
	}
	if resources.finalCleanup != nil {
		resources.CollectorCleanup = resources.finalCleanup
		resources.finalCleanup = nil
	}
	sjo.resources = resources
	return resources, nil
}

func (sjo *stagedJobOwner) BindAttachment() error {
	if sjo == nil {
		return errors.New("job output: nil staged job owner")
	}
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.ownership != stagedJobOwnedByRuntime || sjo.attached || sjo.decided {
		return errors.New("job output: invalid staged job attachment binding")
	}
	if sjo.retiring {
		return sjo.retirementErrorLocked("job output: invalid staged job attachment binding")
	}
	if sjo.finalized {
		return errors.New("job output: invalid staged job attachment binding")
	}
	sjo.attached = true
	return nil
}

func (sjo *stagedJobOwner) Reject() {
	sjo.reject(nil, false)
}

// rejectCandidateOnCut lets candidate-attempt cancellation win only before
// promotion owns the handoff. Once promotion starts, its runtime attempt owns
// retirement and the candidate cut must not invalidate that successor.
func (sjo *stagedJobOwner) rejectCandidateOnCut(cause error) bool {
	return sjo.reject(cause, true)
}

func (sjo *stagedJobOwner) reject(cause error, candidateOnly bool) bool {
	if sjo == nil {
		return false
	}
	sjo.mu.Lock()
	resources, ownership, rejected := sjo.rejectLocked(cause, candidateOnly)
	sjo.mu.Unlock()
	if rejected {
		sjo.finishRejection(resources, ownership)
	}
	return rejected
}

func (sjo *stagedJobOwner) rejectLocked(
	cause error,
	candidateOnly bool,
) (ConstructedJob, stagedJobOwnership, bool) {
	ownership := sjo.ownership
	if sjo.decided ||
		candidateOnly && ownership != stagedJobOwnedByCandidate {
		return ConstructedJob{}, 0, false
	}
	sjo.decided = true
	sjo.installing = false
	if ownership == stagedJobOwnedByCandidate {
		sjo.ownership = stagedJobOwnershipRejected
	}
	if !sjo.retiring {
		sjo.retirementCause = cause
	}
	sjo.retiring = true
	sjo.candidateCtx = nil
	resources := sjo.resources
	return resources, ownership, true
}

func (sjo *stagedJobOwner) finishRejection(
	resources ConstructedJob,
	ownership stagedJobOwnership,
) {
	resources.outputGate.Fence()
	sjo.retireOnce.Do(func() {
		close(sjo.retire)
	})
	sjo.Detached()
	if ownership == stagedJobOwnedByCandidate {
		sjo.candidateRejectOnce.Do(func() {
			close(sjo.rejectCandidate)
		})
	}
}

func (sjo *stagedJobOwner) Promote(ctx context.Context) error {
	if sjo == nil || ctx == nil {
		return errors.New("job output: invalid staged job promotion")
	}
	if cause := context.Cause(ctx); cause != nil {
		return cause
	}
	sjo.mu.Lock()
	if sjo.attempts == nil ||
		sjo.target == 0 ||
		sjo.runtimeIdentity.Namespace != jobmgr.ProcessAttemptJobRuntime ||
		sjo.runtimeIdentity.Key == "" ||
		sjo.runtimeIdentity.Resource == "" {
		sjo.mu.Unlock()
		return errors.New("job output: invalid staged job promotion")
	}
	if sjo.retiring {
		err := sjo.retirementErrorLocked("job output: invalid staged job promotion")
		sjo.mu.Unlock()
		return err
	}
	if sjo.ownership != stagedJobOwnedByCandidate || sjo.decided {
		sjo.mu.Unlock()
		return errors.New("job output: invalid staged job promotion")
	}
	if cause := context.Cause(sjo.candidateCtx); cause != nil {
		resources, ownership, _ := sjo.rejectLocked(cause, true)
		sjo.mu.Unlock()
		sjo.finishRejection(resources, ownership)
		return cause
	}
	sjo.ownership = stagedJobPromotionActive
	attempts := sjo.attempts
	candidateCtx := sjo.candidateCtx
	runtimeIdentity := sjo.runtimeIdentity
	target := sjo.target
	sjo.mu.Unlock()

	start := func() (jobmgr.ProcessAttempt, <-chan error, error) {
		admitted := make(chan error, 1)
		attempt, err := attempts.StartProcessAttempt(ctx, jobmgr.ProcessAttemptPlan{
			Identity:      runtimeIdentity,
			Target:        target,
			OnContainment: sjo.containRuntimeAttempt,
			Work: func(ctx context.Context, admission jobmgr.ProcessAttemptAdmission) error {
				admitErr := admission.Admit()
				admitted <- admitErr
				if admitErr != nil {
					return admitErr
				}
				return sjo.finish(ctx)
			},
		})
		if err != nil {
			return nil, nil, err
		}
		return attempt, admitted, nil
	}
	_, admitted, err := start()
	if errors.Is(err, jobmgr.ErrProcessAttemptBusy) {
		// Resource apply deliberately excludes task cancellation so it cannot
		// abandon an atomic graph transition. Until a successor exists, the
		// candidate attempt is the process-owned stop signal for supersession.
		err = attempts.SupersedeProcessAttempt(candidateCtx, runtimeIdentity)
		if err == nil {
			_, admitted, err = start()
		}
	}
	if err != nil {
		return sjo.failPromotion(err)
	}
	if err := <-admitted; err != nil {
		return sjo.failPromotion(err)
	}

	sjo.mu.Lock()
	var structuralErr error
	if sjo.ownership != stagedJobPromotionActive {
		structuralErr = errors.New("job output: invalid staged job promotion settlement")
	}
	sjo.ownership = stagedJobOwnedByRuntime
	sjo.candidateCtx = nil
	sjo.transferOnce.Do(func() {
		close(sjo.transferred)
	})
	var retirementErr error
	if sjo.retiring {
		retirementErr = sjo.retirementErrorLocked("job output: staged job retired during promotion")
	}
	sjo.mu.Unlock()
	return errors.Join(structuralErr, retirementErr)
}

func (sjo *stagedJobOwner) failPromotion(promotionErr error) error {
	sjo.mu.Lock()
	if sjo.ownership != stagedJobPromotionActive {
		sjo.mu.Unlock()
		return errors.Join(
			promotionErr,
			errors.New("job output: invalid failed staged job promotion settlement"),
		)
	}
	sjo.ownership = stagedJobOwnedByCandidate
	if sjo.retiring {
		cause := sjo.retirementErrorLocked("job output: staged job retired during promotion")
		resources, ownership, rejected := sjo.rejectLocked(cause, true)
		sjo.mu.Unlock()
		if rejected {
			sjo.finishRejection(resources, ownership)
		}
		return errors.Join(promotionErr, cause)
	}
	if cause := context.Cause(sjo.candidateCtx); cause != nil {
		resources, ownership, rejected := sjo.rejectLocked(cause, true)
		sjo.mu.Unlock()
		if rejected {
			sjo.finishRejection(resources, ownership)
		}
		return cause
	}
	sjo.mu.Unlock()
	return promotionErr
}

func (sjo *stagedJobOwner) ReserveInstallation() error {
	if sjo == nil {
		return errors.New("job output: nil staged job installation")
	}
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.decided || sjo.installing {
		return errors.New("job output: staged job cannot begin installation")
	}
	if sjo.retiring {
		return sjo.retirementErrorLocked("job output: staged job cannot begin installation")
	}
	sjo.installing = true
	return nil
}

func (sjo *stagedJobOwner) Install() error {
	if sjo == nil {
		return errors.New("job output: nil staged job installation")
	}
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.decided || !sjo.installing {
		return errors.New("job output: invalid staged job installation acknowledgement")
	}
	sjo.installing = false
	sjo.decided = true
	return nil
}

func (sjo *stagedJobOwner) Start(ctx context.Context) error {
	if sjo == nil || ctx == nil {
		return errors.New("job output: invalid process-owned job start")
	}
	if err := sjo.reserveStart(); err != nil {
		return err
	}
	result := make(chan error, 1)
	request := stagedJobStartRequest{
		ctx:    ctx,
		result: result,
	}
	select {
	case sjo.startRequests <- request:
	case <-sjo.retire:
		return sjo.retirementError("job output: process-owned job is retiring")
	case <-sjo.done:
		return errors.New("job output: process-owned job is released")
	case <-ctx.Done():
		return context.Cause(ctx)
	}
	select {
	case err := <-result:
		return err
	case <-sjo.retire:
		return sjo.retirementError("job output: process-owned job retired during start")
	case <-sjo.done:
		return errors.New("job output: process-owned job released during start")
	case <-ctx.Done():
		cause := context.Cause(ctx)
		sjo.requestRetirement(cause)
		return cause
	}
}

func (sjo *stagedJobOwner) reserveStart() error {
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.retiring {
		return sjo.retirementErrorLocked("job output: process-owned job is retiring")
	}
	if sjo.ownership != stagedJobOwnedByRuntime ||
		!sjo.attached ||
		sjo.startRequested ||
		sjo.started ||
		sjo.decided ||
		sjo.finalized {
		return errors.New("job output: invalid process-owned job start")
	}
	sjo.startRequested = true
	return nil
}

func (sjo *stagedJobOwner) Retire() {
	if sjo == nil {
		return
	}
	sjo.requestRetirement(nil)
}

func (sjo *stagedJobOwner) Detached() {
	if sjo == nil {
		return
	}
	sjo.detachOnce.Do(func() {
		sjo.mu.Lock()
		runtimeStage := sjo.resources.runtimeStage
		vnodeStage := sjo.resources.vnodeStage
		sjo.mu.Unlock()
		runtimeStage.close()
		vnodeStage.close()
		close(sjo.detached)
	})
}

func (sjo *stagedJobOwner) requestRetirement(cause error) {
	resources := sjo.beginRetirement(cause)
	resources.outputGate.Fence()
}

// containRuntimeAttempt publishes the authoritative cut without waiting for
// output leases or external cleanup while the attempt authority is locked.
func (sjo *stagedJobOwner) containRuntimeAttempt(cause error) {
	sjo.beginRetirement(cause)
}

func (sjo *stagedJobOwner) beginRetirement(cause error) ConstructedJob {
	if sjo == nil {
		return ConstructedJob{}
	}
	sjo.mu.Lock()
	if !sjo.retiring {
		sjo.retirementCause = cause
	}
	sjo.retiring = true
	resources := sjo.resources
	resources.outputGate.RevokeAdmissions()
	sjo.retireOnce.Do(func() {
		close(sjo.retire)
	})
	sjo.mu.Unlock()
	return resources
}

func (sjo *stagedJobOwner) retirementError(fallback string) error {
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	return sjo.retirementErrorLocked(fallback)
}

func (sjo *stagedJobOwner) retirementErrorLocked(fallback string) error {
	if sjo.retiring && sjo.retirementCause != nil {
		return sjo.retirementCause
	}
	return errors.New(fallback)
}

func (sjo *stagedJobOwner) finish(ctx context.Context) (resultErr error) {
	if sjo == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	defer sjo.doneOnce.Do(func() {
		close(sjo.done)
	})

	var request stagedJobStartRequest
	select {
	case request = <-sjo.startRequests:
	case <-sjo.retire:
		sjo.cutBeforeStart(nil)
		return sjo.finalize()
	case <-ctx.Done():
		sjo.cutBeforeStart(context.Cause(ctx))
		return sjo.finalize()
	}

	resources, retiring, err := sjo.beginManagedStart()
	if err != nil {
		request.result <- err
		if retiring {
			sjo.cutBeforeStart(nil)
		}
		return sjo.finalize()
	}
	job := resources.candidateJob
	if job == nil {
		request.result <- errors.New("job output: missing process-owned runtime job")
		sjo.requestRetirement(nil)
		return sjo.finalize()
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
		sjo.requestRetirement(nil)
	case <-sjo.retire:
		request.result <- sjo.retirementError("job output: process-owned job retired before readiness")
	case <-ctx.Done():
		cause := context.Cause(ctx)
		sjo.requestRetirement(cause)
		request.result <- cause
	case <-request.ctx.Done():
		cause := context.Cause(request.ctx)
		sjo.requestRetirement(cause)
		request.result <- cause
	}

	if !physicalExited {
		select {
		case loopErr := <-exited:
			resultErr = errors.Join(resultErr, loopErr)
			physicalExited = true
			sjo.Retire()
		case <-sjo.retire:
		case <-ctx.Done():
			sjo.requestRetirement(context.Cause(ctx))
		}
	}
	if !physicalExited {
		sjo.drainRetirement()
		stopErr := callJobLifecycle("process-owned managed runtime Stop", func() error {
			job.Stop()
			return nil
		})
		resultErr = errors.Join(resultErr, stopErr)
		resultErr = errors.Join(resultErr, <-exited)
	}
	return errors.Join(resultErr, sjo.finalize())
}

func (sjo *stagedJobOwner) beginManagedStart() (ConstructedJob, bool, error) {
	sjo.mu.Lock()
	defer sjo.mu.Unlock()
	if sjo.started || !sjo.startRequested {
		return ConstructedJob{}, false, errors.New("job output: duplicate process-owned job start")
	}
	if sjo.retiring {
		return ConstructedJob{}, true, sjo.retirementErrorLocked("job output: process-owned job retired before start")
	}
	sjo.started = true
	return sjo.resources, false, nil
}

func (sjo *stagedJobOwner) finishCandidate(ctx context.Context) error {
	if sjo == nil {
		return nil
	}
	if ctx == nil {
		ctx = context.Background()
	}
	select {
	case <-sjo.transferred:
		return nil
	case <-sjo.rejectCandidate:
		return sjo.finalize()
	case <-ctx.Done():
		sjo.rejectCandidateOnCut(context.Cause(ctx))
		select {
		case <-sjo.transferred:
			return nil
		case <-sjo.rejectCandidate:
			return sjo.finalize()
		}
	}
}

func (sjo *stagedJobOwner) cutBeforeStart(cause error) {
	sjo.requestRetirement(cause)
	sjo.mu.Lock()
	attached := sjo.attached
	sjo.mu.Unlock()
	if !attached {
		sjo.Detached()
	}
}

func (sjo *stagedJobOwner) drainRetirement() {
	if sjo == nil {
		return
	}
	sjo.mu.Lock()
	resources := sjo.resources
	sjo.mu.Unlock()
	resources.outputGate.Fence()
}

func (sjo *stagedJobOwner) finalize() error {
	<-sjo.detached
	sjo.mu.Lock()
	if sjo.finalized {
		sjo.mu.Unlock()
		return nil
	}
	sjo.finalized = true
	resources := sjo.resources
	sjo.resources = ConstructedJob{}
	sjo.mu.Unlock()
	resources.outputGate.Fence()
	return finalizeProcessOwnedConstructed(resources)
}

func (f *Factory) newCandidate(
	config confgroup.Config,
) (*preparedJobCandidate, error) {
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
	detached.Runtime = nil
	detached.HandlerAttacher = nil
	detached.Scheduler = nil
	detached.Observer = nil
	detached.Attempts = nil
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
	return &preparedJobCandidate{
		factory: &Factory{
			config:         detached,
			runtimeStaging: f.config.Runtime != nil,
		},
		attempts: f.config.Attempts,
		identity: jobAttemptIdentity(jobmgr.ProcessAttemptJob, config.FullName()),
		target:   f.config.Epoch,
		config:   config,
		ctx:      ctx,
		cancel:   cancel,
		ready:    make(chan struct{}),
	}, nil
}

// awaitCandidate starts process-owned materialization while the transaction's
// graph claim is yielded, then returns only after the stage is logically
// settled and the claim has been reacquired.
func (f *Factory) awaitCandidate(ctx context.Context, stage *preparedJobCandidate) error {
	if f == nil || ctx == nil || stage == nil || f.runWithoutClaims == nil {
		return errors.New("job output: invalid candidate wait")
	}
	waitErr, claimErr := f.runWithoutClaims(ctx, func(yielded context.Context) error {
		if cause := context.Cause(yielded); cause != nil {
			stage.Cancel(cause)
			return cause
		}
		stage.Start()
		select {
		case <-stage.Ready():
			return nil
		case <-yielded.Done():
			cause := context.Cause(yielded)
			stage.Cancel(cause)
			return cause
		}
	})
	return errors.Join(waitErr, claimErr)
}

func (pjc *preparedJobCandidate) Start() {
	if pjc == nil {
		return
	}
	pjc.startOnce.Do(func() {
		pjc.mu.Lock()
		pjc.started = true
		pjc.mu.Unlock()
		go pjc.start()
	})
}

func (pjc *preparedJobCandidate) Ready() <-chan struct{} {
	if pjc == nil {
		return nil
	}
	return pjc.ready
}

func (pjc *preparedJobCandidate) Cancel(cause error) {
	if pjc == nil {
		return
	}
	if cause == nil {
		cause = context.Canceled
	}
	pjc.cancel(cause)
	pjc.mu.Lock()
	attempt := pjc.attempt
	pjc.mu.Unlock()
	if attempt != nil {
		attempt.Cut(cause)
	}
}

func (pjc *preparedJobCandidate) Release() {
	if pjc == nil {
		return
	}
	pjc.mu.Lock()
	if pjc.released {
		pjc.mu.Unlock()
		return
	}
	pjc.released = true
	started := pjc.started
	taken := pjc.taken
	owner := pjc.result.owner
	attempt := pjc.attempt
	pjc.config = nil
	pjc.factory = nil
	pjc.mu.Unlock()
	if taken {
		return
	}
	pjc.cancel(context.Canceled)
	if owner != nil {
		owner.Reject()
	} else if attempt != nil {
		attempt.Cut(context.Canceled)
	}
	if !started {
		pjc.publish(stagedJobResult{err: context.Canceled})
	}
}

func (pjc *preparedJobCandidate) start() {
	if cause := context.Cause(pjc.ctx); cause != nil {
		pjc.publish(stagedJobResult{err: cause})
		return
	}
	if err := pjc.attempts.SupersedeProcessAttempt(pjc.ctx, pjc.identity); err != nil {
		pjc.publish(stagedJobResult{err: err})
		return
	}
	workerResult := make(chan stagedJobResult, 1)
	attempt, err := pjc.attempts.StartProcessAttempt(pjc.ctx, jobmgr.ProcessAttemptPlan{
		Identity: pjc.identity,
		Target:   pjc.target,
		Work: func(
			ctx context.Context,
			admission jobmgr.ProcessAttemptAdmission,
		) error {
			return pjc.run(ctx, admission, workerResult)
		},
	})
	if err != nil {
		pjc.publish(stagedJobResult{err: err})
		return
	}
	pjc.mu.Lock()
	pjc.attempt = attempt
	pjc.mu.Unlock()
	if cause := context.Cause(pjc.ctx); cause != nil {
		attempt.Cut(cause)
	}
	if err := attempt.Await(context.Background()); err != nil {
		pjc.publish(stagedJobResult{err: err})
		return
	}
	pjc.mu.Lock()
	settled := pjc.settled
	pjc.mu.Unlock()
	if settled {
		return
	}
	select {
	case result := <-workerResult:
		pjc.publish(result)
	default:
		pjc.publish(stagedJobResult{
			err: errors.New("job output: candidate attempt settled without a result"),
		})
	}
}

func (pjc *preparedJobCandidate) run(
	ctx context.Context,
	admission jobmgr.ProcessAttemptAdmission,
	workerResult chan<- stagedJobResult,
) error {
	pjc.mu.Lock()
	factory := pjc.factory
	config := pjc.config
	pjc.mu.Unlock()
	if factory == nil || config == nil {
		return context.Canceled
	}
	cloned, err := config.Clone()
	if err != nil {
		workerResult <- stagedJobResult{err: err}
		return nil
	}
	candidate, err := factory.build(ctx, cloned)
	config = nil
	if err != nil {
		factory = nil
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		err = joinRetainedCleanup(err, cleanupErr)
		if lifecycle.OwnershipRetained(err) {
			return err
		}
		workerResult <- stagedJobResult{err: err}
		return nil
	}
	probeErr := probeConstructed(ctx, candidate, candidate.autoDetection)
	if probeErr != nil {
		captureJobConfigLifecycle(&candidate)
		factory = nil
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		if cleanupErr != nil {
			err = joinRetainedCleanup(probeErr, cleanupErr)
			return err
		}
		var failure *autoDetectionFailure
		if errors.As(probeErr, &failure) {
			failure.jobConfigLifecycle = candidate.jobConfigSnapshot
			workerResult <- stagedJobResult{failure: failure}
			return nil
		}
		workerResult <- stagedJobResult{err: probeErr}
		return nil
	}
	stageErr := factory.stageCandidateHandlers(&candidate)
	factory = nil
	if stageErr != nil {
		failure := autoDetectionFailureFor(candidate, stageErr)
		captureJobConfigLifecycle(&candidate)
		failure.jobConfigLifecycle = candidate.jobConfigSnapshot
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		if lifecycle.OwnershipRetained(stageErr) || cleanupErr != nil {
			return joinRetainedCleanup(stageErr, cleanupErr)
		}
		workerResult <- stagedJobResult{failure: failure}
		return nil
	}
	if err := admission.Admit(); err != nil {
		cleanupErr := cleanupConstructed(context.Background(), candidate)
		return joinRetainedCleanup(err, cleanupErr)
	}
	captureJobConfigLifecycle(&candidate)
	storeSnapshot := candidate.storeSnapshot
	candidate.storeSnapshot = nil
	owner := newStagedJobOwner(
		ctx,
		candidate,
		pjc.attempts,
		pjc.target,
		jobAttemptIdentity(
			jobmgr.ProcessAttemptJobRuntime,
			candidate.candidateJob.FullName(),
		),
	)
	pjc.publish(stagedJobResult{
		candidate:     candidate,
		owner:         owner,
		storeSnapshot: storeSnapshot,
	})
	return owner.finishCandidate(ctx)
}

func (pjc *preparedJobCandidate) publish(result stagedJobResult) {
	pjc.readyOnce.Do(func() {
		pjc.mu.Lock()
		if pjc.released {
			if result.owner != nil {
				result.owner.Reject()
			}
		} else {
			pjc.result = result
		}
		pjc.settled = true
		pjc.config = nil
		pjc.mu.Unlock()
		close(pjc.ready)
	})
}

func (pjc *preparedJobCandidate) take() (stagedJobResult, error) {
	if pjc == nil {
		return stagedJobResult{}, errors.New("job output: nil candidate stage")
	}
	select {
	case <-pjc.ready:
	default:
		return stagedJobResult{}, errors.New("job output: candidate stage is not ready")
	}
	pjc.mu.Lock()
	defer pjc.mu.Unlock()
	if !pjc.settled || pjc.taken {
		return stagedJobResult{}, errors.New("job output: candidate stage already consumed")
	}
	pjc.taken = true
	result := pjc.result
	pjc.result = stagedJobResult{}
	return result, nil
}

func (f *Factory) prepareCandidate(
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
	stage *preparedJobCandidate,
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
	if err := validateStoreSnapshot(result.storeSnapshot); err != nil {
		result.owner.Reject()
		return PreparedJob{}, nil, err
	}
	prepared, err := prepareCandidateJob(
		identity,
		permit,
		result.candidate,
		f.attachment(),
		result.owner,
	)
	if err != nil {
		result.owner.Reject()
	}
	return prepared, nil, err
}

func validateStoreSnapshot(snapshot secretresolver.AtomicScopeSnapshot) (err error) {
	if snapshot == nil {
		return nil
	}
	defer func() {
		if recover() != nil {
			err = ErrStaleStoreGeneration
		}
	}()
	if !snapshot.Current() {
		return ErrStaleStoreGeneration
	}
	return nil
}

func candidatePreparationBusy(err error) bool {
	return errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
		errors.Is(err, ErrStaleStoreGeneration)
}

func (dcjc *DynCfgJobController) prepareContainedJob(
	ctx context.Context,
	config confgroup.Config,
	identity lifecycle.ResourceIdentity,
	permit lifecycle.LongLivedPermit,
) (PreparedJob, *autoDetectionFailure, error) {
	if dcjc == nil || dcjc.factory == nil {
		return PreparedJob{}, nil, errors.New("job output: invalid contained job preparation")
	}
	stage, err := dcjc.factory.newCandidate(config)
	if err != nil {
		return PreparedJob{}, nil, err
	}
	defer stage.Release()
	if err := dcjc.factory.awaitCandidate(ctx, stage); err != nil {
		return PreparedJob{}, nil, err
	}
	return dcjc.factory.prepareCandidate(identity, permit, stage)
}
