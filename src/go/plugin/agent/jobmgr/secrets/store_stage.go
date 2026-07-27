// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/secrets/secretstore"
)

type StoreOperationsConfig struct {
	Epoch       uint64
	Attempts    jobmgr.ProcessAttemptAuthority
	Store       *secretstore.SecretStore
	Creators    *secretstore.CreatorCatalog
	Diagnostics jobmgr.DiagnosticObserver
}

// StoreOperations contains provider-authored configuration materialization in
// process-owned attempts. It owns no run, graph, or Function capability.
type StoreOperations struct {
	epoch       uint64
	attempts    jobmgr.ProcessAttemptAuthority
	store       *secretstore.SecretStore
	creators    *secretstore.CreatorCatalog
	diagnostics jobmgr.DiagnosticObserver
}

func NewStoreOperations(config StoreOperationsConfig) (*StoreOperations, error) {
	if config.Epoch == 0 ||
		config.Attempts == nil ||
		config.Store == nil ||
		config.Creators == nil {
		return nil, errors.New("jobmgr secrets: invalid process Store operations")
	}
	return &StoreOperations{
		epoch:       config.Epoch,
		attempts:    config.Attempts,
		store:       config.Store,
		creators:    config.Creators,
		diagnostics: config.Diagnostics,
	}, nil
}

type storeOperationMode uint8

const (
	storeOperationMutation storeOperationMode = iota + 1
	storeOperationValidation
	storeOperationRemoval
)

type storeOperationSpec struct {
	target         secretTarget
	input          CommandInput
	config         secretstore.Config
	expected       uint64
	mode           storeOperationMode
	validationOnly bool
	supersede      bool
	testIdentity   bool
	desiredVersion uint64
}

type storeOperationResult struct {
	config         secretstore.Config
	mutation       *secretstore.PreparedSecretMutation
	release        <-chan struct{}
	err            error
	expected       uint64
	desiredVersion uint64
	validationOnly bool
	retryable      bool
	removal        bool
}

// PreparedStoreOperation is a process-owned pre-claim materialization.
type PreparedStoreOperation struct {
	mu sync.Mutex

	operations *StoreOperations
	spec       storeOperationSpec
	identity   jobmgr.ProcessAttemptIdentity
	attempt    jobmgr.ProcessAttempt
	immediate  *storeOperationResult

	ctx      context.Context
	cancel   context.CancelCauseFunc
	ready    chan struct{}
	release  chan struct{}
	started  bool
	settled  bool
	taken    bool
	released bool
	result   storeOperationResult

	startOnce   sync.Once
	readyOnce   sync.Once
	releaseOnce sync.Once
}

func (operations *StoreOperations) immediate(
	result storeOperationResult,
) *PreparedStoreOperation {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &PreparedStoreOperation{
		operations: operations,
		ctx:        ctx,
		cancel:     cancel,
		ready:      make(chan struct{}),
		release:    make(chan struct{}),
		immediate:  &result,
	}
}

func (operations *StoreOperations) prepare(
	spec storeOperationSpec,
) (*PreparedStoreOperation, error) {
	if operations == nil ||
		spec.target.key == "" ||
		spec.mode < storeOperationMutation ||
		spec.mode > storeOperationRemoval {
		return nil, errors.New("jobmgr secrets: invalid Store operation preparation")
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	input := spec.input
	input.Args = append([]string(nil), input.Args...)
	input.Payload = append([]byte(nil), input.Payload...)
	spec.input = input
	return &PreparedStoreOperation{
		operations: operations,
		spec:       spec,
		ctx:        ctx,
		cancel:     cancel,
		ready:      make(chan struct{}),
		release:    make(chan struct{}),
	}, nil
}

func (operation *PreparedStoreOperation) Start() {
	if operation == nil {
		return
	}
	operation.startOnce.Do(func() {
		operation.mu.Lock()
		operation.started = true
		operation.mu.Unlock()
		go operation.start()
	})
}

func (operation *PreparedStoreOperation) Ready() <-chan struct{} {
	if operation == nil {
		return nil
	}
	return operation.ready
}

func (operation *PreparedStoreOperation) Cancel(cause error) {
	if operation == nil {
		return
	}
	if cause == nil {
		cause = context.Canceled
	}
	operation.cancel(cause)
	operation.mu.Lock()
	attempt := operation.attempt
	operation.mu.Unlock()
	if attempt != nil {
		attempt.Cut(cause)
	}
}

func (operation *PreparedStoreOperation) Release() {
	if operation == nil {
		return
	}
	operation.mu.Lock()
	if operation.released {
		operation.mu.Unlock()
		return
	}
	operation.released = true
	started := operation.started
	operation.immediate = nil
	operation.mu.Unlock()
	operation.cancel(context.Canceled)
	operation.releaseOnce.Do(func() {
		close(operation.release)
	})
	if !started {
		operation.publish(storeOperationResult{err: context.Canceled})
	}
	operation.clearUnownedResult()
}

func (operation *PreparedStoreOperation) start() {
	operation.mu.Lock()
	immediate := operation.immediate
	operation.immediate = nil
	operation.mu.Unlock()
	if immediate != nil {
		result := *immediate
		operation.publish(result)
		return
	}
	if err := context.Cause(operation.ctx); err != nil {
		operation.publish(storeOperationResult{err: err})
		return
	}
	if operation.spec.mode == storeOperationRemoval {
		identity := operation.operations.identity(operation.spec.target.key, false, nil)
		operation.mu.Lock()
		operation.identity = identity
		operation.mu.Unlock()
		operation.operations.attempts.CutProcessAttempt(identity, jobmgr.ErrProcessAttemptSuperseded)
		operation.publish(storeOperationResult{
			desiredVersion: operation.spec.desiredVersion,
			removal:        true,
		})
		return
	}

	config := operation.spec.config
	if config == nil {
		var err error
		config, err = materializeSecretConfig(operation.spec.input, operation.spec.target)
		if err != nil {
			operation.publish(storeOperationResult{
				err:            err,
				desiredVersion: operation.spec.desiredVersion,
				validationOnly: operation.spec.validationOnly,
			})
			return
		}
	}
	identityPayload := []byte(nil)
	if operation.spec.testIdentity {
		identityPayload = []byte(fmt.Sprintf("%016x", config.Hash()))
	}
	operation.spec.input = CommandInput{}
	identity := operation.operations.identity(
		operation.spec.target.key,
		operation.spec.testIdentity,
		identityPayload,
	)
	operation.mu.Lock()
	operation.identity = identity
	operation.mu.Unlock()

	attempt, err := operation.startAttempt(identity, config)
	if err != nil {
		released, _ := operation.operations.attempts.ProcessAttemptReleased(identity)
		operation.publish(storeOperationResult{
			config:         config,
			release:        released,
			err:            err,
			expected:       operation.spec.expected,
			desiredVersion: operation.spec.desiredVersion,
			validationOnly: operation.spec.validationOnly,
			retryable:      errors.Is(err, jobmgr.ErrProcessAttemptBusy),
		})
		return
	}
	operation.mu.Lock()
	operation.attempt = attempt
	operation.mu.Unlock()
	if cause := context.Cause(operation.ctx); cause != nil {
		attempt.Cut(cause)
	}
	operation.observeAttempt(attempt, config)
}

func (operation *PreparedStoreOperation) startAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	config secretstore.Config,
) (jobmgr.ProcessAttempt, error) {
	start := func() (jobmgr.ProcessAttempt, error) {
		attemptReady := make(chan jobmgr.ProcessAttempt, 1)
		attempt, err := operation.operations.attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
			Identity: identity,
			Target:   operation.operations.epoch,
			Work: func(ctx context.Context) error {
				owned := <-attemptReady
				return operation.runAttempt(ctx, owned, config)
			},
		})
		if err != nil {
			return nil, err
		}
		attemptReady <- attempt
		return attempt, nil
	}

	attempt, err := start()
	if !errors.Is(err, jobmgr.ErrProcessAttemptBusy) || !operation.spec.supersede {
		return attempt, err
	}
	if err := operation.operations.attempts.SupersedeProcessAttempt(operation.ctx, identity); err != nil {
		return nil, err
	}
	return start()
}

func (operation *PreparedStoreOperation) runAttempt(
	ctx context.Context,
	attempt jobmgr.ProcessAttempt,
	config secretstore.Config,
) error {
	result := storeOperationResult{
		config:         config,
		expected:       operation.spec.expected,
		desiredVersion: operation.spec.desiredVersion,
		validationOnly: operation.spec.validationOnly,
	}
	if operation.spec.mode == storeOperationValidation {
		result.err = operation.operations.store.Validate(
			ctx,
			operation.operations.creators,
			config,
		)
	} else {
		result.mutation, result.err = operation.operations.store.PrepareMutation(
			ctx,
			operation.operations.creators,
			config,
			operation.spec.expected,
		)
	}
	if err := attempt.Admit(); err != nil {
		if result.mutation != nil {
			_ = result.mutation.Abort()
		}
		return err
	}
	operation.publish(result)
	<-operation.release
	operation.clearUnownedResult()
	return nil
}

func (operation *PreparedStoreOperation) observeAttempt(
	attempt jobmgr.ProcessAttempt,
	config secretstore.Config,
) {
	err := attempt.Await(context.Background())
	operation.publish(storeOperationResult{
		config:         config,
		release:        attempt.Released(),
		err:            err,
		expected:       operation.spec.expected,
		desiredVersion: operation.spec.desiredVersion,
		validationOnly: operation.spec.validationOnly,
		retryable:      containmentRetryable(err),
	})
}

func containmentRetryable(err error) bool {
	return errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
		errors.Is(err, jobmgr.ErrProcessAttemptDeadline) ||
		errors.Is(err, jobmgr.ErrProcessAttemptSuperseded)
}

func (operation *PreparedStoreOperation) publish(result storeOperationResult) {
	discard := false
	published := false
	operation.readyOnce.Do(func() {
		published = true
		operation.mu.Lock()
		if operation.taken || operation.released {
			discard = true
		} else {
			operation.result = result
		}
		operation.settled = true
		operation.spec.input = CommandInput{}
		operation.spec.config = nil
		operation.mu.Unlock()
		close(operation.ready)
	})
	if !published || discard {
		operation.releaseResult(result)
	}
}

func (operation *PreparedStoreOperation) take() (storeOperationResult, error) {
	if operation == nil {
		return storeOperationResult{}, errors.New("jobmgr secrets: nil Store operation")
	}
	select {
	case <-operation.ready:
	default:
		return storeOperationResult{}, errors.New("jobmgr secrets: Store operation is not ready")
	}
	operation.mu.Lock()
	defer operation.mu.Unlock()
	if !operation.settled || operation.taken {
		return storeOperationResult{}, errors.New("jobmgr secrets: Store operation already consumed")
	}
	operation.taken = true
	result := operation.result
	operation.result = storeOperationResult{}
	return result, nil
}

func (operation *PreparedStoreOperation) clearUnownedResult() {
	operation.mu.Lock()
	if operation.taken {
		operation.mu.Unlock()
		return
	}
	result := operation.result
	operation.result = storeOperationResult{}
	operation.taken = true
	operation.mu.Unlock()
	operation.releaseResult(result)
}

func (operation *PreparedStoreOperation) releaseResult(result storeOperationResult) {
	if result.mutation == nil {
		return
	}
	if err := result.mutation.Abort(); err != nil {
		operation.mu.Lock()
		identity := operation.identity
		operation.mu.Unlock()
		jobmgr.ObserveDiagnostic(operation.operations.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "secret Store staged mutation release failed",
			Resource:   identity.Resource,
			Generation: operation.operations.epoch,
			Err:        err,
		})
	}
}

func (operations *StoreOperations) identity(
	key string,
	test bool,
	payload []byte,
) jobmgr.ProcessAttemptIdentity {
	namespace := jobmgr.ProcessAttemptStore
	opaque := fmt.Sprintf("%d/%s", operations.epoch, key)
	if test {
		namespace = jobmgr.ProcessAttemptStoreTest
		hash := sha256.Sum256(payload)
		opaque = fmt.Sprintf("%s/%x", opaque, hash)
	}
	return jobmgr.ProcessAttemptIdentity{
		Namespace: namespace,
		Key:       opaque,
		Resource:  key,
	}
}
