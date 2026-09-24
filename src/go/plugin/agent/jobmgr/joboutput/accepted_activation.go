// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"gopkg.in/yaml.v2"
)

type acceptedActivationToken struct {
	run        uint64
	generation uint64
	uid        string
}

type acceptedActivationSpec struct {
	config       confgroup.Config
	dependencies []activationDependency
}

func newAcceptedActivationSpec(config confgroup.Config) (acceptedActivationSpec, error) {
	if config == nil || config.FullName() == "" || config.UID() == "" {
		return acceptedActivationSpec{}, errors.New("job output: invalid accepted activation config")
	}
	cloned, err := config.Clone()
	if err != nil {
		return acceptedActivationSpec{}, err
	}
	keys, err := dependencyKeys(cloned)
	if err != nil {
		return acceptedActivationSpec{}, err
	}
	return acceptedActivationSpec{config: cloned, dependencies: keys}, nil
}

type acceptedActivationAttempt struct {
	spec    acceptedActivationSpec
	token   acceptedActivationToken
	stage   *preparedJobCandidate
	err     error
	applied *atomic.Bool
}

func (attempt acceptedActivationAttempt) valid() bool {
	return attempt.spec.config != nil &&
		attempt.spec.config.FullName() != "" &&
		attempt.token.run != 0 &&
		attempt.token.generation != 0 &&
		attempt.token.uid != "" &&
		attempt.applied != nil &&
		(attempt.stage != nil) != (attempt.err != nil)
}

func (attempt acceptedActivationAttempt) markApplied() func() {
	return func() {
		attempt.applied.Store(true)
	}
}

type acceptedActivationPlanner func(acceptedActivationAttempt) (jobmgr.WorkPlan, error)

type acceptedActivationState uint8

const (
	acceptedActivationStaging acceptedActivationState = iota + 1
	acceptedActivationTerminal
	acceptedActivationInstalled
)

type acceptedActivationEntry struct {
	spec    acceptedActivationSpec
	token   acceptedActivationToken
	state   acceptedActivationState
	stage   *preparedJobCandidate
	cancel  chan struct{}
	wake    chan struct{}
	waiting bool
	release <-chan struct{}
	resume  chan struct{}
}

type acceptedActivationIndex struct {
	mu sync.Mutex
	wg sync.WaitGroup

	dependencies activationDependencyIndex
	entries      map[string]*acceptedActivationEntry
	factory      *Factory
	commands     jobmgr.PreparedCommandPort
	plan         acceptedActivationPlanner
	failure      func(error)
	run          uint64
	generation   uint64
	bound        bool
	closed       bool
	failed       bool
	terminalErr  error
	stop         chan struct{}
	done         chan struct{}
	retire       func(acceptedActivationToken)
}

func newAcceptedActivationIndex() *acceptedActivationIndex {
	return &acceptedActivationIndex{
		entries: make(map[string]*acceptedActivationEntry),
		stop:    make(chan struct{}),
		done:    make(chan struct{}),
	}
}

func (aai *acceptedActivationIndex) bind(
	factory *Factory,
	commands jobmgr.PreparedCommandPort,
	plan acceptedActivationPlanner,
	run uint64,
	failure func(error),
) error {
	if aai == nil || factory == nil || commands == nil || plan == nil || run == 0 || failure == nil {
		return errors.New("job output: invalid accepted activation binding")
	}
	aai.mu.Lock()
	if aai.bound || aai.closed {
		aai.mu.Unlock()
		return errors.New("job output: accepted activations already bound")
	}
	aai.factory = factory
	aai.commands = commands
	aai.plan = plan
	aai.failure = failure
	aai.run = run
	aai.bound = true
	aai.mu.Unlock()
	go aai.join()
	return nil
}

func (aai *acceptedActivationIndex) arm(spec acceptedActivationSpec) {
	aai.armAfterRelease(spec, nil)
}

func (aai *acceptedActivationIndex) armAfterRelease(spec acceptedActivationSpec, release <-chan struct{}) {
	aai.armWithGate(spec, release, nil, nil)
}

func (aai *acceptedActivationIndex) pause(spec acceptedActivationSpec) {
	gate := make(chan struct{})
	aai.armWithGate(spec, gate, gate, nil)
}

func (aai *acceptedActivationIndex) resumeFor(id string) func() {
	aai.mu.Lock()
	entry := aai.entries[id]
	aai.mu.Unlock()
	return func() {
		aai.mu.Lock()
		defer aai.mu.Unlock()
		if entry != nil && aai.entries[id] == entry && entry.resume != nil {
			close(entry.resume)
			entry.resume = nil
		}
	}
}

// armWithGate consumes stage, including when registration is no longer possible.
func (aai *acceptedActivationIndex) armWithGate(spec acceptedActivationSpec, release <-chan struct{}, resume chan struct{}, stage *preparedJobCandidate) {
	defer func() { stage.Release() }()
	if aai == nil || spec.config == nil || spec.config.FullName() == "" || spec.config.UID() == "" {
		return
	}
	id := spec.config.FullName()
	aai.mu.Lock()
	if !aai.bound || aai.closed || aai.failed {
		aai.mu.Unlock()
		return
	}
	if current := aai.entries[id]; current != nil && current.state != acceptedActivationInstalled && current.token.uid == spec.config.UID() && resume == nil && stage == nil {
		aai.mu.Unlock()
		return
	}
	aai.generation++
	if aai.generation == 0 {
		aai.mu.Unlock()
		aai.fail(errors.New("job output: accepted activation generation wrapped"))
		return
	}
	entry := &acceptedActivationEntry{
		spec: spec,
		token: acceptedActivationToken{
			run:        aai.run,
			generation: aai.generation,
			uid:        spec.config.UID(),
		},
		state:   acceptedActivationStaging,
		stage:   stage,
		cancel:  make(chan struct{}),
		wake:    make(chan struct{}, 1),
		release: release,
		resume:  resume,
	}
	previous := aai.detachLocked(id)
	aai.entries[id] = entry
	stage = nil // the entry now owns the retained candidate
	aai.dependencies.replace(id, spec.dependencies, entry.wake)
	aai.wg.Add(1)
	aai.mu.Unlock()
	releaseAcceptedActivationStage(previous, jobmgr.ErrProcessAttemptSuperseded)
	go aai.runEntry(id, entry)
}

// An installed generation owns startup; retain only enabled intent until its
// serialized Running/Failed transition. No activation worker is needed here.
func (aai *acceptedActivationIndex) trackInstalled(spec acceptedActivationSpec) {
	id := spec.config.FullName()
	aai.mu.Lock()
	if !aai.bound || aai.closed || aai.failed {
		aai.mu.Unlock()
		return
	}
	aai.generation++
	if aai.generation == 0 {
		aai.mu.Unlock()
		aai.fail(errors.New("job output: accepted activation generation wrapped"))
		return
	}
	previous := aai.detachLocked(id)
	aai.entries[id] = &acceptedActivationEntry{
		spec:   spec,
		token:  acceptedActivationToken{run: aai.run, generation: aai.generation, uid: spec.config.UID()},
		state:  acceptedActivationInstalled,
		cancel: make(chan struct{}),
	}
	aai.mu.Unlock()
	releaseAcceptedActivationStage(previous, jobmgr.ErrProcessAttemptSuperseded)
}

func (dcjc *DynCfgJobController) ActivationEnabled(id string) bool {
	return dcjc != nil && dcjc.scheduler != nil && dcjc.scheduler.accepted.enabled(id)
}

func (aai *acceptedActivationIndex) runEntry(id string, entry *acceptedActivationEntry) {
	defer aai.wg.Done()
	if entry.release != nil {
		select {
		case <-entry.release:
		case <-entry.cancel:
			return
		case <-aai.stop:
			return
		}
	}
	for sequence := uint64(1); ; sequence++ {
		// A notification consumed here precedes the next lookup. Later events
		// stay queued, including changes during candidate preparation.
		select {
		case <-entry.wake:
		default:
		}
		if !aai.runAttempt(id, entry, sequence) {
			return
		}
		aai.mu.Lock()
		if aai.entries[id] != entry {
			aai.mu.Unlock()
			return
		}
		waiting, release := entry.waiting, entry.release
		entry.stage = nil
		aai.mu.Unlock()
		if !waiting {
			aai.settle(id, entry.token)
			return
		}
		select {
		case <-entry.wake:
		case <-release:
		case <-entry.cancel:
			return
		case <-aai.stop:
			return
		}
		aai.mu.Lock()
		if aai.entries[id] != entry {
			aai.mu.Unlock()
			return
		}
		entry.state = acceptedActivationStaging
		entry.waiting = false
		entry.release = nil
		aai.mu.Unlock()
	}
}

// waitFor retains the same committed config authority through a dependency or
// physical-release wait. It runs only after the waiting postimage commits.
func (aai *acceptedActivationIndex) waitFor(id string, token acceptedActivationToken, release <-chan struct{}) {
	aai.mu.Lock()
	defer aai.mu.Unlock()
	if entry := aai.entries[id]; entry != nil && entry.token == token {
		entry.waiting = true
		entry.release = release
	}
}

// NotifyDependencyChanged wakes enabled jobs waiting for a named dependency.
func (dcjc *DynCfgJobController) NotifyDependencyChanged(kind, name string) {
	if dcjc != nil && dcjc.scheduler != nil && dcjc.scheduler.accepted != nil {
		dcjc.scheduler.accepted.dependencies.notify(kind, name)
	}
}

func (aai *acceptedActivationIndex) runAttempt(id string, entry *acceptedActivationEntry, sequence uint64) bool {
	aai.mu.Lock()
	if aai.closed || aai.failed || aai.entries[id] != entry || entry.state != acceptedActivationStaging {
		aai.mu.Unlock()
		return false
	}
	stage := entry.stage
	aai.mu.Unlock()
	var stageErr error
	if stage == nil {
		// Resume permission does not imply physical release. Probe only when
		// promotion can reuse the result instead of discarding it as busy.
		runtimeID := jobAttemptIdentity(jobmgr.ProcessAttemptJobRuntime, id)
		if released, exists := aai.factory.config.Attempts.ProcessAttemptReleased(runtimeID); exists {
			select {
			case <-released:
			case <-entry.cancel:
				return false
			case <-aai.stop:
				return false
			}
		}
		stage, stageErr = aai.factory.newCandidate(entry.spec.config)
	}
	aai.mu.Lock()
	if aai.closed || aai.failed || aai.entries[id] != entry || entry.state != acceptedActivationStaging {
		aai.mu.Unlock()
		releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
		return false
	}
	entry.stage = stage
	aai.mu.Unlock()

	if stage != nil {
		stage.Start()
		select {
		case <-stage.Ready():
		case <-entry.cancel:
			releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
			return false
		case <-aai.stop:
			releaseAcceptedActivationStage(stage, context.Canceled)
			return false
		}
	}

	aai.mu.Lock()
	if aai.closed || aai.failed || aai.entries[id] != entry || entry.state != acceptedActivationStaging {
		aai.mu.Unlock()
		releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
		return false
	}
	entry.state = acceptedActivationTerminal
	plan := aai.plan
	commands := aai.commands
	attempt := acceptedActivationAttempt{
		spec:    entry.spec,
		token:   entry.token,
		stage:   stage,
		err:     stageErr,
		applied: &atomic.Bool{},
	}
	aai.mu.Unlock()

	if stage != nil {
		defer stage.Release()
	}
	work, err := plan(attempt)
	if err == nil {
		err = commands.SubmitPreparedAndWait(context.Background(), jobmgr.Request{
			UID: fmt.Sprintf(
				"jobmgr-accepted-activation-%d-%d-%d",
				entry.token.run,
				entry.token.generation,
				sequence,
			),
			LaneKey: id,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/jobs/accepted-activation",
		}, work)
	}
	if err == nil {
		return true
	}
	if lifecycle.ContainsOnlyCurrentStoppingRejections(err, entry.token.run) {
		aai.settle(id, entry.token)
		return false
	}
	aai.failAttempt(id, entry.token, attempt.applied.Load(), err)
	return false
}

func (aai *acceptedActivationIndex) isCurrent(id string, token acceptedActivationToken) bool {
	if aai == nil || id == "" || token.run == 0 || token.generation == 0 || token.uid == "" {
		return false
	}
	aai.mu.Lock()
	defer aai.mu.Unlock()
	entry := aai.entries[id]
	return entry != nil && entry.token == token && entry.state == acceptedActivationTerminal
}

func (aai *acceptedActivationIndex) currentToken(id string) (acceptedActivationToken, bool) {
	aai.mu.Lock()
	defer aai.mu.Unlock()
	entry := aai.entries[id]
	if entry == nil {
		return acceptedActivationToken{}, false
	}
	return entry.token, true
}

func (aai *acceptedActivationIndex) enabled(id string) bool {
	if aai == nil {
		return false
	}
	aai.mu.Lock()
	defer aai.mu.Unlock()
	return aai.entries[id] != nil
}

func (aai *acceptedActivationIndex) settle(id string, token acceptedActivationToken) {
	if aai == nil || id == "" || token.generation == 0 {
		return
	}
	aai.mu.Lock()
	entry := aai.entries[id]
	if entry == nil || entry.token != token {
		aai.mu.Unlock()
		return
	}
	stage := aai.detachLocked(id)
	aai.mu.Unlock()
	releaseAcceptedActivationStage(stage, nil)
}

func (aai *acceptedActivationIndex) cancelUnless(id, keepUID string) {
	if aai == nil || id == "" {
		return
	}
	aai.mu.Lock()
	entry := aai.entries[id]
	if entry == nil || keepUID != "" && entry.token.uid == keepUID {
		aai.mu.Unlock()
		return
	}
	stage := aai.detachLocked(id)
	aai.mu.Unlock()
	releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
}

func (aai *acceptedActivationIndex) detachLocked(id string) *preparedJobCandidate {
	entry := aai.entries[id]
	if entry == nil {
		return nil
	}
	delete(aai.entries, id)
	if aai.retire != nil {
		aai.retire(entry.token)
	}
	aai.dependencies.remove(id)
	close(entry.cancel)
	if entry.state == acceptedActivationTerminal {
		// The terminal worker exclusively owns the stage while its transaction
		// may consume it. Revocation only removes authority; the worker releases
		// the stage after that transaction settles.
		return nil
	}
	stage := entry.stage
	entry.stage = nil
	return stage
}

func releaseAcceptedActivationStage(stage *preparedJobCandidate, cause error) {
	if stage == nil {
		return
	}
	stage.Cancel(cause)
	stage.Release()
}

func (aai *acceptedActivationIndex) stopWorker() {
	if aai == nil {
		return
	}
	aai.mu.Lock()
	if aai.closed {
		aai.mu.Unlock()
		return
	}
	aai.closed = true
	stages := make([]*preparedJobCandidate, 0, len(aai.entries))
	for id := range aai.entries {
		stages = append(stages, aai.detachLocked(id))
	}
	close(aai.stop)
	aai.mu.Unlock()
	for _, stage := range stages {
		releaseAcceptedActivationStage(stage, context.Canceled)
	}
}

func (aai *acceptedActivationIndex) wait(ctx context.Context) error {
	if aai == nil || ctx == nil {
		return errors.New("job output: invalid accepted activation wait")
	}
	aai.mu.Lock()
	bound := aai.bound
	done := aai.done
	aai.mu.Unlock()
	if !bound {
		return nil
	}
	select {
	case <-done:
		return aai.terminalError()
	case <-ctx.Done():
		return errors.Join(ctx.Err(), aai.terminalError())
	}
}

func (aai *acceptedActivationIndex) terminalError() error {
	aai.mu.Lock()
	defer aai.mu.Unlock()
	return aai.terminalErr
}

func (aai *acceptedActivationIndex) join() {
	<-aai.stop
	aai.wg.Wait()
	close(aai.done)
}

func (aai *acceptedActivationIndex) fail(err error) {
	aai.failAttempt("", acceptedActivationToken{}, true, err)
}

func (aai *acceptedActivationIndex) failAttempt(
	id string,
	token acceptedActivationToken,
	applied bool,
	err error,
) {
	if aai == nil || err == nil {
		return
	}
	aai.mu.Lock()
	if aai.failed {
		aai.mu.Unlock()
		return
	}
	entry := aai.entries[id]
	if !applied && (entry == nil || entry.token != token) {
		aai.mu.Unlock()
		return
	}
	aai.failed = true
	aai.terminalErr = errors.Join(aai.terminalErr, err)
	failure := aai.failure
	stages := make([]*preparedJobCandidate, 0, len(aai.entries))
	if !aai.closed {
		aai.closed = true
		for currentID := range aai.entries {
			stages = append(stages, aai.detachLocked(currentID))
		}
		close(aai.stop)
	}
	aai.mu.Unlock()
	for _, stage := range stages {
		releaseAcceptedActivationStage(stage, err)
	}
	if failure != nil {
		failure(err)
	}
}

func (dcjc *DynCfgJobController) planAcceptedActivation(
	attempt acceptedActivationAttempt,
) (jobmgr.WorkPlan, error) {
	if dcjc == nil || !attempt.valid() || attempt.spec.config.FullName() == "" {
		return jobmgr.WorkPlan{}, errors.New("job output: invalid accepted activation plan")
	}
	return jobmgr.WorkPlan{
		Claims:     []string{DynCfgJobGraphClaim},
		NoResponse: true,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID:                attempt.spec.config.FullName(),
			AllocateSuccessor: true,
			Permit:            lifecycle.NewJobLongLivedPlan(),
			Prepare: func(
				ctx context.Context,
				current lifecycle.ReadyResource,
				scope lifecycle.ResourceTransactionScope,
				permit lifecycle.LongLivedPermit,
			) (lifecycle.PreparedResourceTransaction, error) {
				return dcjc.prepareAcceptedActivation(ctx, attempt, current, scope, permit)
			},
		},
	}, nil
}

func (dcjc *DynCfgJobController) prepareAcceptedActivation(
	ctx context.Context,
	attempt acceptedActivationAttempt,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
) (lifecycle.PreparedResourceTransaction, error) {
	if dcjc == nil || ctx == nil || !attempt.valid() || !scope.Valid() || scope.ID != attempt.spec.config.FullName() {
		return nil, errors.New("job output: invalid accepted activation preparation")
	}
	record, exists := dcjc.graph.Lookup(scope.ID)
	if err := validateGraphResourcePair(record, exists, current, scope); err != nil {
		return nil, err
	}
	result := noResponseResult()
	reply := internalReply()
	if !dcjc.scheduler.accepted.isCurrent(scope.ID, attempt.token) ||
		!exists || record.Status != dyncfg.StatusAccepted.String() || current != nil {
		return dcjc.noop(scope, current, permit, result)
	}
	config, err := graphRecordConfig(record)
	if err != nil {
		return nil, err
	}
	if config.UID() != attempt.token.uid || config.UID() != attempt.spec.config.UID() {
		return dcjc.noop(scope, current, permit, result)
	}
	if attempt.stage == nil {
		return dcjc.prepareAcceptedActivationError(
			attempt,
			current,
			scope,
			permit,
			classifyActivationError(attempt.err),
		)
	}
	successor, probeFailure, err := dcjc.factory.prepareCandidate(scope.Successor, permit, attempt.stage)
	if err != nil {
		return dcjc.prepareAcceptedActivationError(attempt, current, scope, permit, classifyActivationError(err))
	}
	failedPostimage := graphConfig(record, dyncfg.StatusFailed)
	failurePlan := probeFailurePlan{
		postimage:      failedPostimage,
		failedCleanup:  dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
		removedCleanup: dcjc.configDeleteCleanup(dcjc.externalID(scope.ID)),
		reply:          internalFailureReply,
		afterApply: composeProbeFailureAfterApply(
			func(failure *autoDetectionFailure) {
				dcjc.completeActivationRestart(attempt.token, adoptedResult(dyncfg.CommandRestart, dyncfg.StatusFailed, collectorFailure(failure, "config restart failed: %v")))
				dcjc.scheduleAutoDetectionRetry(config, failure)
			},
			attempt.markApplied(),
		),
		removePlainStock: config.SourceType() == confgroup.TypeStock,
	}
	if probeFailure != nil {
		if activationWaitsForDependency(config, probeFailure) {
			return dcjc.prepareAcceptedActivationWait(attempt, current, scope, permit, probeFailure, nil)
		}
		return dcjc.prepareProbeFailure(
			scope,
			current,
			permit,
			autoDetectionRetryToken{},
			probeFailure,
			failurePlan,
		)
	}
	acceptedPostimage := graphConfig(record, dyncfg.StatusAccepted)
	return dcjc.prepareMutationWithActivationFallbacks(
		scope,
		current,
		successor,
		lifecycle.ResourceTransactionInstalled,
		&acceptedPostimage,
		reply,
		dcjc.configStatusCleanup(scope.ID, dyncfg.StatusAccepted),
		autoDetectionRetryToken{},
		composeAfterApply(func() { dcjc.installActivationRestart(attempt.token, scope.Successor) }, attempt.markApplied()),
		activationFallbackPlan{
			postimage: &acceptedPostimage,
			cleanup:   dcjc.configStatusCleanup(scope.ID, dyncfg.StatusAccepted),
			afterApply: composeAfterApply(
				func() {
					dcjc.scheduler.accepted.waitFor(scope.ID, attempt.token, dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJobRuntime))
				},
				attempt.markApplied(),
			),
		},
		activationFallbackPlan{
			postimage: &failedPostimage,
			cleanup:   dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			afterApply: composeAfterApply(func() {
				dcjc.completeActivationRestart(attempt.token, rejectedResult(jobFailure{class: failureUnavailable, message: "the job cannot start until the plugin restarts."}))
			}, attempt.markApplied()),
		},
	)
}

func (dcjc *DynCfgJobController) prepareAcceptedActivationError(
	attempt acceptedActivationAttempt,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
	activation activationFailure,
) (lifecycle.PreparedResourceTransaction, error) {
	err := activation.err
	if err == nil {
		return nil, errors.New("job output: accepted activation has no candidate outcome")
	}
	result := noResponseResult()
	reply := internalReply()
	if errors.Is(err, jobmgr.ErrProcessAttemptRetired) ||
		errors.Is(err, jobmgr.ErrProcessAttemptStopped) ||
		errors.Is(err, context.Canceled) {
		return dcjc.noop(scope, current, permit, result)
	}
	record, exists := dcjc.graph.Lookup(scope.ID)
	if !exists || record.Status != dyncfg.StatusAccepted.String() {
		return dcjc.noop(scope, current, permit, result)
	}
	failedPostimage := graphConfig(record, dyncfg.StatusFailed)
	config := attempt.spec.config
	if activationWaitsForDependency(config, err) {
		return dcjc.prepareAcceptedActivationWait(attempt, current, scope, permit, err, nil)
	}
	if activation.kind == activationFailureBusy || activation.kind == activationFailureStaleStore || activation.kind == activationFailureSuperseded {
		return dcjc.prepareAcceptedActivationWait(attempt, current, scope, permit, err,
			dcjc.activationRelease(scope.ID, jobmgr.ProcessAttemptJob))
	}
	switch activation.kind {
	case activationFailureQuarantined, activationFailureProposal:
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			permit,
			resourceRemovalDisposition(current),
			&failedPostimage,
			reply,
			dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			autoDetectionRetryToken{},
			composeAfterApply(func() { dcjc.completeActivationRestart(attempt.token, rejectedResult(testFailure(err))) }, attempt.markApplied()),
			jobConfigFailure(err, "activation"),
		)
	case activationFailureDeadline, activationFailureTransient:
		failure := transientActivationFailure(config, err)
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			permit,
			resourceRemovalDisposition(current),
			&failedPostimage,
			reply,
			dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			autoDetectionRetryToken{},
			composeAfterApply(func() {
				dcjc.completeActivationRestart(attempt.token, rejectedResult(testFailure(err)))
				dcjc.scheduleAutoDetectionRetry(config, failure)
			}, attempt.markApplied()),
			jobConfigFailure(err, "activation"),
		)
	default:
		return nil, err
	}
}

// A missing owner means release won the race with registration. A closed
// signal forces a fresh attempt against the committed dependency state.
func (dcjc *DynCfgJobController) activationRelease(id string, namespace jobmgr.ProcessAttemptNamespace) <-chan struct{} {
	if release, ok := dcjc.factory.config.Attempts.ProcessAttemptReleased(jobAttemptIdentity(namespace, id)); ok {
		return release
	}
	released := make(chan struct{})
	close(released)
	return released
}

func (dcjc *DynCfgJobController) prepareAcceptedActivationWait(
	attempt acceptedActivationAttempt,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
	cause error,
	release <-chan struct{},
) (lifecycle.PreparedResourceTransaction, error) {
	record, exists := dcjc.graph.Lookup(scope.ID)
	if !exists {
		return dcjc.noop(scope, current, permit, noResponseResult())
	}
	postimage := graphConfig(record, dyncfg.StatusAccepted)
	return dcjc.prepareMutationWithRetryAfterApply(
		scope, current, nil, permit, resourceRemovalDisposition(current), &postimage, internalReply(),
		dcjc.configStatusCleanup(scope.ID, dyncfg.StatusAccepted), autoDetectionRetryToken{},
		composeAfterApply(func() { dcjc.scheduler.accepted.waitFor(scope.ID, attempt.token, release) }, attempt.markApplied()),
		jobConfigFailure(cause, "activation"),
	)
}

func composeProbeFailureAfterApply(
	first func(*autoDetectionFailure),
	second func(),
) func(*autoDetectionFailure) {
	return func(failure *autoDetectionFailure) {
		if first != nil {
			first(failure)
		}
		if second != nil {
			second()
		}
	}
}

func (dcjc *DynCfgJobController) acceptedActivationAfterApply(
	id string,
	postimage *dyncfg.GraphConfig,
) (func(), error) {
	if dcjc == nil || dcjc.scheduler == nil || dcjc.scheduler.accepted == nil || id == "" {
		return nil, errors.New("job output: invalid accepted activation graph reconciliation")
	}
	keepUID := ""
	if postimage != nil && postimage.Status == dyncfg.StatusAccepted.String() {
		var config confgroup.Config
		if err := yaml.Unmarshal(postimage.Payload, &config); err != nil {
			return nil, fmt.Errorf("job output: invalid accepted graph payload: %w", err)
		}
		if config == nil || config.Module() != postimage.Module || config.Name() != postimage.Name {
			return nil, errors.New("job output: accepted graph payload identity differs")
		}
		keepUID = config.UID()
		if keepUID == "" {
			return nil, errors.New("job output: accepted graph payload has no UID")
		}
	}
	return func() {
		dcjc.scheduler.accepted.cancelUnless(id, keepUID)
	}, nil
}
