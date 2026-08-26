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
	config confgroup.Config
}

func newAcceptedActivationSpec(config confgroup.Config) (acceptedActivationSpec, error) {
	if config == nil || config.FullName() == "" || config.UID() == "" {
		return acceptedActivationSpec{}, errors.New("job output: invalid accepted activation config")
	}
	cloned, err := config.Clone()
	if err != nil {
		return acceptedActivationSpec{}, err
	}
	return acceptedActivationSpec{config: cloned}, nil
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
)

type acceptedActivationEntry struct {
	spec   acceptedActivationSpec
	token  acceptedActivationToken
	state  acceptedActivationState
	stage  *preparedJobCandidate
	cancel chan struct{}
}

type acceptedActivationIndex struct {
	mu sync.Mutex
	wg sync.WaitGroup

	entries     map[string]*acceptedActivationEntry
	factory     *Factory
	commands    jobmgr.PreparedCommandPort
	plan        acceptedActivationPlanner
	failure     func(error)
	run         uint64
	generation  uint64
	bound       bool
	closed      bool
	failed      bool
	terminalErr error
	stop        chan struct{}
	done        chan struct{}
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
	if aai == nil || spec.config == nil || spec.config.FullName() == "" || spec.config.UID() == "" {
		return
	}
	id := spec.config.FullName()
	aai.mu.Lock()
	if !aai.bound || aai.closed || aai.failed {
		aai.mu.Unlock()
		return
	}
	if current := aai.entries[id]; current != nil && current.token.uid == spec.config.UID() {
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
		state:  acceptedActivationStaging,
		cancel: make(chan struct{}),
	}
	previous := aai.detachLocked(id)
	aai.entries[id] = entry
	aai.wg.Add(1)
	aai.mu.Unlock()
	releaseAcceptedActivationStage(previous, jobmgr.ErrProcessAttemptSuperseded)
	go aai.runEntry(id, entry)
}

func (aai *acceptedActivationIndex) runEntry(id string, entry *acceptedActivationEntry) {
	defer aai.wg.Done()
	stage, stageErr := aai.factory.newCandidate(entry.spec.config)

	aai.mu.Lock()
	if aai.closed || aai.failed || aai.entries[id] != entry || entry.state != acceptedActivationStaging {
		aai.mu.Unlock()
		releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
		return
	}
	entry.stage = stage
	aai.mu.Unlock()

	if stage != nil {
		stage.Start()
		select {
		case <-stage.Ready():
		case <-entry.cancel:
			releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
			return
		case <-aai.stop:
			releaseAcceptedActivationStage(stage, context.Canceled)
			return
		}
	}

	aai.mu.Lock()
	if aai.closed || aai.failed || aai.entries[id] != entry || entry.state != acceptedActivationStaging {
		aai.mu.Unlock()
		releaseAcceptedActivationStage(stage, jobmgr.ErrProcessAttemptSuperseded)
		return
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
				"jobmgr-accepted-activation-%d-%d",
				entry.token.run,
				entry.token.generation,
			),
			LaneKey: id,
			Source:  lifecycle.SourceJobManager,
			Route:   "internal/jobs/accepted-activation",
		}, work)
	}
	if err == nil {
		aai.settle(id, entry.token)
		return
	}
	if lifecycle.ContainsOnlyCurrentStoppingRejections(err, entry.token.run) {
		aai.settle(id, entry.token)
		return
	}
	aai.failAttempt(id, entry.token, attempt.applied.Load(), err)
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
	result := mustDynCfgMessage(204, "")
	if !dcjc.scheduler.accepted.isCurrent(scope.ID, attempt.token) ||
		!exists || record.Status != dyncfg.StatusAccepted.String() {
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
		return dcjc.prepareAcceptedActivationError(attempt, current, scope, permit, attempt.err)
	}
	successor, probeFailure, err := dcjc.factory.prepareCandidate(scope.Successor, permit, attempt.stage)
	if err != nil {
		return dcjc.prepareAcceptedActivationError(attempt, current, scope, permit, err)
	}
	failedPostimage := graphConfig(record, dyncfg.StatusFailed)
	if probeFailure != nil {
		return dcjc.prepareProbeFailure(
			scope,
			current,
			permit,
			autoDetectionRetryToken{},
			probeFailure,
			probeFailurePlan{
				postimage:        failedPostimage,
				failedCleanup:    dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
				removedCleanup:   dcjc.configDeleteCleanup(dcjc.externalID(scope.ID)),
				result:           func(*autoDetectionFailure) lifecycle.SealedResult { return result },
				afterApply:       composeProbeFailureAfterApply(dcjc.scheduleRetryAfterApply(config), attempt.markApplied()),
				removePlainStock: config.SourceType() == confgroup.TypeStock,
			},
		)
	}
	runningPostimage := graphConfig(record, dyncfg.StatusRunning)
	busyFailure := transientActivationFailure(config, jobmgr.ErrProcessAttemptBusy)
	return dcjc.prepareMutationWithActivationFallbacks(
		scope,
		current,
		successor,
		lifecycle.ResourceTransactionInstalled,
		&runningPostimage,
		result,
		dcjc.configStatusCleanup(scope.ID, dyncfg.StatusRunning),
		autoDetectionRetryToken{},
		attempt.markApplied(),
		activationFallbackPlan{
			postimage:  &failedPostimage,
			result:     result,
			cleanup:    dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			afterApply: composeAfterApply(func() { dcjc.scheduleAutoDetectionRetry(config, busyFailure) }, attempt.markApplied()),
		},
		activationFallbackPlan{
			postimage:  &failedPostimage,
			result:     result,
			cleanup:    dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			afterApply: attempt.markApplied(),
		},
	)
}

func (dcjc *DynCfgJobController) prepareAcceptedActivationError(
	attempt acceptedActivationAttempt,
	current lifecycle.ReadyResource,
	scope lifecycle.ResourceTransactionScope,
	permit lifecycle.LongLivedPermit,
	err error,
) (lifecycle.PreparedResourceTransaction, error) {
	if err == nil {
		return nil, errors.New("job output: accepted activation has no candidate outcome")
	}
	result := mustDynCfgMessage(204, "")
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
	if errors.Is(err, jobmgr.ErrProcessAttemptQuarantined) {
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			permit,
			resourceRemovalDisposition(current),
			&failedPostimage,
			result,
			dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			autoDetectionRetryToken{},
			attempt.markApplied(),
		)
	}
	if errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
		errors.Is(err, jobmgr.ErrProcessAttemptSuperseded) ||
		errors.Is(err, jobmgr.ErrProcessAttemptDeadline) ||
		errors.Is(err, ErrStaleStoreGeneration) ||
		classifyConstructionError(err) == constructionErrorTransient {
		failure := transientActivationFailure(config, err)
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			permit,
			resourceRemovalDisposition(current),
			&failedPostimage,
			result,
			dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			autoDetectionRetryToken{},
			composeAfterApply(func() { dcjc.scheduleAutoDetectionRetry(config, failure) }, attempt.markApplied()),
		)
	}
	if classifyConstructionError(err) == constructionErrorProposal {
		return dcjc.prepareMutationWithRetryAfterApply(
			scope,
			current,
			nil,
			permit,
			resourceRemovalDisposition(current),
			&failedPostimage,
			result,
			dcjc.configStatusCleanup(scope.ID, dyncfg.StatusFailed),
			autoDetectionRetryToken{},
			attempt.markApplied(),
		)
	}
	return nil, err
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
