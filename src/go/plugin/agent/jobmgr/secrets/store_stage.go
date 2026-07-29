// SPDX-License-Identifier: GPL-3.0-or-later

package secrets

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
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

// preparedMutationOwner keeps a prepared mutation abortable until a prepared
// transaction has accepted it.
type preparedMutationOwner struct {
	mutation *secretstore.PreparedSecretMutation
}

func (o *preparedMutationOwner) prepareTransaction(
	spec preparedSecretSpec,
) (lifecycle.PreparedResourceTransaction, error) {
	if o != nil {
		spec.mutation = o.mutation
	}
	transaction, err := newPreparedSecretTransaction(spec)
	if err != nil {
		return nil, err
	}
	if o != nil {
		o.mutation = nil
	}
	return transaction, nil
}

func (o *preparedMutationOwner) releaseUntransferred(
	transaction *lifecycle.PreparedResourceTransaction,
	resultErr *error,
) {
	if o == nil || o.mutation == nil {
		return
	}
	abortErr := o.mutation.Abort()
	o.mutation = nil
	if abortErr == nil {
		return
	}
	if transaction != nil {
		*transaction = nil
	}
	if resultErr != nil {
		*resultErr = lifecycle.RetainOwnership(errors.Join(*resultErr, abortErr))
	}
}

type takenStoreOperation struct {
	result   storeOperationResult
	mutation preparedMutationOwner
}

func (o *takenStoreOperation) releaseUntransferred(
	transaction *lifecycle.PreparedResourceTransaction,
	resultErr *error,
) {
	if o != nil {
		o.mutation.releaseUntransferred(transaction, resultErr)
	}
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

func (so *StoreOperations) immediate(
	result storeOperationResult,
) *PreparedStoreOperation {
	ctx, cancel := context.WithCancelCause(context.Background())
	return &PreparedStoreOperation{
		operations: so,
		ctx:        ctx,
		cancel:     cancel,
		ready:      make(chan struct{}),
		release:    make(chan struct{}),
		immediate:  &result,
	}
}

func (so *StoreOperations) prepare(
	spec storeOperationSpec,
) (*PreparedStoreOperation, error) {
	if so == nil ||
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
		operations: so,
		spec:       spec,
		ctx:        ctx,
		cancel:     cancel,
		ready:      make(chan struct{}),
		release:    make(chan struct{}),
	}, nil
}

func (pso *PreparedStoreOperation) Start() {
	if pso == nil {
		return
	}
	pso.startOnce.Do(func() {
		pso.mu.Lock()
		pso.started = true
		pso.mu.Unlock()
		go pso.start()
	})
}

func (pso *PreparedStoreOperation) Ready() <-chan struct{} {
	if pso == nil {
		return nil
	}
	return pso.ready
}

func (pso *PreparedStoreOperation) Cancel(cause error) {
	if pso == nil {
		return
	}
	if cause == nil {
		cause = context.Canceled
	}
	pso.cancel(cause)
	pso.mu.Lock()
	attempt := pso.attempt
	pso.mu.Unlock()
	if attempt != nil {
		attempt.Cut(cause)
	}
}

func (pso *PreparedStoreOperation) Release() {
	if pso == nil {
		return
	}
	pso.mu.Lock()
	if pso.released {
		pso.mu.Unlock()
		return
	}
	pso.released = true
	started := pso.started
	pso.immediate = nil
	pso.mu.Unlock()
	pso.cancel(context.Canceled)
	pso.releaseOnce.Do(func() {
		close(pso.release)
	})
	if !started {
		_ = pso.publish(storeOperationResult{err: context.Canceled})
	}
}

func (pso *PreparedStoreOperation) start() {
	pso.mu.Lock()
	immediate := pso.immediate
	pso.immediate = nil
	pso.mu.Unlock()
	if immediate != nil {
		result := *immediate
		_ = pso.publish(result)
		return
	}
	if err := context.Cause(pso.ctx); err != nil {
		_ = pso.publish(storeOperationResult{err: err})
		return
	}
	if pso.spec.mode == storeOperationRemoval {
		identity := pso.operations.identity(pso.spec.target.key, false, nil)
		pso.mu.Lock()
		pso.identity = identity
		pso.mu.Unlock()
		// This logically contains and cancels physical preparation, which may
		// continue until Released. prepareRemove commits the desired-version
		// fence that invalidates older retained ADDs.
		pso.operations.attempts.CutProcessAttempt(identity, jobmgr.ErrProcessAttemptSuperseded)
		_ = pso.publish(storeOperationResult{
			desiredVersion: pso.spec.desiredVersion,
			removal:        true,
		})
		return
	}

	config := pso.spec.config
	if config == nil {
		var err error
		config, err = materializeSecretConfig(pso.spec.input, pso.spec.target)
		if err != nil {
			_ = pso.publish(storeOperationResult{
				err:            err,
				desiredVersion: pso.spec.desiredVersion,
				validationOnly: pso.spec.validationOnly,
			})
			return
		}
	}
	identityPayload := []byte(nil)
	if pso.spec.testIdentity {
		identityPayload = fmt.Appendf(nil, "%016x", config.Hash())
	}
	pso.spec.input = CommandInput{}
	identity := pso.operations.identity(
		pso.spec.target.key,
		pso.spec.testIdentity,
		identityPayload,
	)
	pso.mu.Lock()
	pso.identity = identity
	pso.mu.Unlock()

	attempt, err := pso.startAttempt(identity, config)
	if err != nil {
		released, _ := pso.operations.attempts.ProcessAttemptReleased(identity)
		_ = pso.publish(storeOperationResult{
			config:         config,
			release:        released,
			err:            err,
			expected:       pso.spec.expected,
			desiredVersion: pso.spec.desiredVersion,
			validationOnly: pso.spec.validationOnly,
			retryable:      errors.Is(err, jobmgr.ErrProcessAttemptBusy),
		})
		return
	}
	pso.mu.Lock()
	pso.attempt = attempt
	pso.mu.Unlock()
	if cause := context.Cause(pso.ctx); cause != nil {
		attempt.Cut(cause)
	}
	pso.observeAttempt(attempt, config)
}

func (pso *PreparedStoreOperation) startAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	config secretstore.Config,
) (jobmgr.ProcessAttempt, error) {
	start := func() (jobmgr.ProcessAttempt, error) {
		attempt, err := pso.operations.attempts.StartProcessAttempt(pso.ctx, jobmgr.ProcessAttemptPlan{
			Identity: identity,
			Target:   pso.operations.epoch,
			Work: func(
				ctx context.Context,
				admission jobmgr.ProcessAttemptAdmission,
			) error {
				return pso.runAttempt(ctx, admission, config)
			},
		})
		if err != nil {
			return nil, err
		}
		return attempt, nil
	}

	attempt, err := start()
	if !errors.Is(err, jobmgr.ErrProcessAttemptBusy) || !pso.spec.supersede {
		return attempt, err
	}
	if err := pso.operations.attempts.SupersedeProcessAttempt(pso.ctx, identity); err != nil {
		return nil, err
	}
	return start()
}

func (pso *PreparedStoreOperation) runAttempt(
	ctx context.Context,
	admission jobmgr.ProcessAttemptAdmission,
	config secretstore.Config,
) error {
	result := storeOperationResult{
		config:         config,
		expected:       pso.spec.expected,
		desiredVersion: pso.spec.desiredVersion,
		validationOnly: pso.spec.validationOnly,
	}
	if pso.spec.mode == storeOperationValidation {
		result.err = pso.operations.store.Validate(
			ctx,
			pso.operations.creators,
			config,
		)
	} else {
		result.mutation, result.err = pso.operations.store.PrepareMutation(
			ctx,
			pso.operations.creators,
			config,
			pso.spec.expected,
		)
		if errors.Is(result.err, secretstore.ErrMutationBusy) {
			result.release = pso.operations.store.MutationReady(config.ExposedKey())
		}
	}
	result.retryable = containmentRetryable(result.err)
	if err := admission.Admit(); err != nil {
		cleanupErr := pso.releaseResult(result)
		if cleanupErr != nil {
			return lifecycle.RetainOwnership(errors.Join(err, cleanupErr))
		}
		return err
	}
	releaseErr := pso.publish(result)
	<-pso.release
	releaseErr = errors.Join(releaseErr, pso.clearUnownedResult())
	if releaseErr != nil {
		return lifecycle.RetainOwnership(releaseErr)
	}
	return nil
}

func (pso *PreparedStoreOperation) observeAttempt(attempt jobmgr.ProcessAttempt, config secretstore.Config) {
	err := attempt.Await(context.Background())
	_ = pso.publish(storeOperationResult{
		config:         config,
		release:        attempt.Released(),
		err:            err,
		expected:       pso.spec.expected,
		desiredVersion: pso.spec.desiredVersion,
		validationOnly: pso.spec.validationOnly,
		retryable:      containmentRetryable(err),
	})
}

func containmentRetryable(err error) bool {
	return errors.Is(err, jobmgr.ErrProcessAttemptBusy) ||
		errors.Is(err, jobmgr.ErrProcessAttemptDeadline) ||
		errors.Is(err, jobmgr.ErrProcessAttemptSuperseded) ||
		errors.Is(err, secretstore.ErrMutationBusy)
}

func (pso *PreparedStoreOperation) publish(result storeOperationResult) error {
	discard := false
	published := false
	pso.readyOnce.Do(func() {
		published = true
		pso.mu.Lock()
		if pso.taken || pso.released {
			discard = true
		} else {
			pso.result = result
		}
		pso.settled = true
		pso.spec.input = CommandInput{}
		pso.spec.config = nil
		pso.mu.Unlock()
		close(pso.ready)
	})
	if !published || discard {
		return pso.releaseResult(result)
	}
	return nil
}

func (pso *PreparedStoreOperation) take() (storeOperationResult, error) {
	if pso == nil {
		return storeOperationResult{}, errors.New("jobmgr secrets: nil Store operation")
	}
	select {
	case <-pso.ready:
	default:
		return storeOperationResult{}, errors.New("jobmgr secrets: Store operation is not ready")
	}
	pso.mu.Lock()
	defer pso.mu.Unlock()
	if !pso.settled || pso.taken {
		return storeOperationResult{}, errors.New("jobmgr secrets: Store operation already consumed")
	}
	pso.taken = true
	result := pso.result
	pso.result = storeOperationResult{}
	return result, nil
}

func (pso *PreparedStoreOperation) clearUnownedResult() error {
	pso.mu.Lock()
	if pso.taken {
		pso.mu.Unlock()
		return nil
	}
	result := pso.result
	pso.result = storeOperationResult{}
	pso.taken = true
	pso.mu.Unlock()
	return pso.releaseResult(result)
}

func (pso *PreparedStoreOperation) releaseResult(result storeOperationResult) error {
	if result.mutation == nil {
		return nil
	}
	if err := result.mutation.Abort(); err != nil {
		pso.mu.Lock()
		identity := pso.identity
		pso.mu.Unlock()
		jobmgr.ObserveDiagnostic(pso.operations.diagnostics, jobmgr.DiagnosticEvent{
			Level:      jobmgr.DiagnosticError,
			Name:       "secret Store staged mutation release failed",
			Resource:   identity.Resource,
			Generation: pso.operations.epoch,
			Err:        err,
		})
		return err
	}
	return nil
}

func (so *StoreOperations) identity(
	key string,
	test bool,
	payload []byte,
) jobmgr.ProcessAttemptIdentity {
	namespace := jobmgr.ProcessAttemptStore
	domain := "secret-store"
	fields := []string{strconv.FormatUint(so.epoch, 10), key}
	if test {
		namespace = jobmgr.ProcessAttemptStoreTest
		domain = "secret-store-test"
		fields = append(fields, string(payload))
	}
	return jobmgr.ProcessAttemptIdentity{
		Namespace: namespace,
		Key:       jobmgr.ProcessAttemptIdentityKey(domain, fields...),
		Resource:  jobmgr.ProcessAttemptDiagnosticResource(key, "secret Store"),
	}
}
