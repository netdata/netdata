// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type configOperationKind uint8

const (
	configOperationValidate configOperationKind = iota + 1
	configOperationTest
	configOperationGet
	// Admission validation must not supersede a retained candidate.
	configOperationAdmit
)

type configOperationResult struct {
	payload []byte
	err     error
}

type preparedConfigOperation struct {
	mu sync.Mutex

	attempts  jobmgr.ProcessAttemptAuthority
	identity  jobmgr.ProcessAttemptIdentity
	target    uint64
	config    confgroup.Config
	operation func(context.Context, confgroup.Config) configOperationResult
	supersede bool
	attempt   jobmgr.ProcessAttempt
	result    configOperationResult
	resultErr error

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

func newPreparedConfigOperation(
	factory *Factory,
	config confgroup.Config,
	kind configOperationKind,
	operation func(context.Context, confgroup.Config) configOperationResult,
) (*preparedConfigOperation, error) {
	if factory == nil || factory.config.Attempts == nil || factory.config.Epoch == 0 ||
		config == nil || config.FullName() == "" || operation == nil {
		return nil, errors.New("job output: invalid config operation")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, err
	}
	identity := jobTestAttemptIdentity(kind, cloned)
	if kind == configOperationValidate {
		identity = jobAttemptIdentity(jobmgr.ProcessAttemptJob, cloned.FullName())
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &preparedConfigOperation{
		attempts:  factory.config.Attempts,
		identity:  identity,
		target:    factory.config.Epoch,
		config:    cloned,
		operation: operation,
		supersede: kind == configOperationValidate,
		ctx:       ctx,
		cancel:    cancel,
		ready:     make(chan struct{}),
	}, nil
}

func (pco *preparedConfigOperation) Start() {
	if pco == nil {
		return
	}
	pco.startOnce.Do(func() {
		pco.mu.Lock()
		pco.started = true
		pco.mu.Unlock()
		go pco.start()
	})
}

func (pco *preparedConfigOperation) Ready() <-chan struct{} {
	if pco == nil {
		return nil
	}
	return pco.ready
}

func (pco *preparedConfigOperation) Cancel(cause error) {
	if pco == nil {
		return
	}
	if cause == nil {
		cause = context.Canceled
	}
	pco.cancel(cause)
	pco.mu.Lock()
	attempt := pco.attempt
	pco.mu.Unlock()
	if attempt != nil {
		attempt.Cut(cause)
	}
}

func (pco *preparedConfigOperation) Release() {
	if pco == nil {
		return
	}
	pco.mu.Lock()
	if pco.released {
		pco.mu.Unlock()
		return
	}
	pco.released = true
	taken := pco.taken
	started := pco.started
	attempt := pco.attempt
	pco.config = nil
	pco.operation = nil
	pco.mu.Unlock()
	if taken {
		return
	}
	pco.Cancel(context.Canceled)
	if !started {
		pco.publish(configOperationResult{}, context.Canceled)
	} else if attempt != nil {
		attempt.Cut(context.Canceled)
	}
}

func (pco *preparedConfigOperation) Await(ctx context.Context) error {
	if pco == nil || ctx == nil {
		return errors.New("job output: invalid config operation wait")
	}
	if cause := context.Cause(ctx); cause != nil {
		pco.Cancel(cause)
		pco.publish(configOperationResult{}, cause)
		return cause
	}
	pco.Start()
	select {
	case <-pco.Ready():
		return nil
	case <-ctx.Done():
		pco.Cancel(context.Cause(ctx))
		return context.Cause(ctx)
	}
}

func (pco *preparedConfigOperation) start() {
	if cause := context.Cause(pco.ctx); cause != nil {
		pco.publish(configOperationResult{}, cause)
		return
	}
	if pco.supersede {
		if err := pco.attempts.SupersedeProcessAttempt(pco.ctx, pco.identity); err != nil {
			pco.publish(configOperationResult{}, err)
			return
		}
	}
	workerResult := make(chan configOperationResult, 1)
	attempt, err := pco.attempts.StartProcessAttempt(pco.ctx, jobmgr.ProcessAttemptPlan{
		Identity: pco.identity,
		Target:   pco.target,
		Work: func(ctx context.Context, _ jobmgr.ProcessAttemptAdmission) error {
			pco.mu.Lock()
			config := pco.config
			operation := pco.operation
			pco.mu.Unlock()
			if config == nil || operation == nil {
				return context.Canceled
			}
			result := operation(ctx, config)
			if cause := context.Cause(ctx); cause != nil {
				return errors.Join(cause, result.err)
			}
			if lifecycle.OwnershipRetained(result.err) {
				return result.err
			}
			workerResult <- result
			return nil
		},
	})
	if err != nil {
		pco.publish(configOperationResult{}, err)
		return
	}
	pco.mu.Lock()
	pco.attempt = attempt
	pco.mu.Unlock()
	if cause := context.Cause(pco.ctx); cause != nil {
		attempt.Cut(cause)
	}
	if err := attempt.Await(context.Background()); err != nil {
		pco.publish(configOperationResult{}, err)
		return
	}
	pco.mu.Lock()
	settled := pco.settled
	pco.mu.Unlock()
	if !settled {
		pco.publish(<-workerResult, nil)
	}
}

func (pco *preparedConfigOperation) publish(result configOperationResult, err error) {
	pco.readyOnce.Do(func() {
		pco.mu.Lock()
		if !pco.released {
			pco.result = result
			pco.resultErr = err
		}
		pco.settled = true
		pco.config = nil
		pco.mu.Unlock()
		close(pco.ready)
	})
}

func (pco *preparedConfigOperation) take() (configOperationResult, error) {
	if pco == nil {
		return configOperationResult{}, errors.New("job output: nil config operation")
	}
	select {
	case <-pco.ready:
	default:
		return configOperationResult{}, errors.New("job output: config operation is not ready")
	}
	pco.mu.Lock()
	if !pco.settled || pco.taken || pco.released {
		pco.mu.Unlock()
		return configOperationResult{}, errors.New("job output: config operation already consumed")
	}
	if cause := context.Cause(pco.ctx); cause != nil {
		pco.mu.Unlock()
		return configOperationResult{}, cause
	}
	pco.taken = true
	result := pco.result
	err := pco.resultErr
	pco.result = configOperationResult{}
	pco.mu.Unlock()
	return result, err
}

func (dcjc *DynCfgJobController) runConfigOperation(
	ctx context.Context,
	config confgroup.Config,
	kind configOperationKind,
	yieldClaims bool,
) ([]byte, error) {
	if dcjc == nil || dcjc.factory == nil || dcjc.configModules == nil {
		return nil, errors.New("job output: invalid contained config operation")
	}
	var operation func(context.Context, confgroup.Config) configOperationResult
	switch kind {
	case configOperationValidate, configOperationAdmit:
		operation = func(ctx context.Context, config confgroup.Config) configOperationResult {
			return configOperationResult{err: dcjc.factory.ValidateConfig(ctx, config)}
		}
	case configOperationTest:
		operation = func(ctx context.Context, config confgroup.Config) configOperationResult {
			return configOperationResult{err: dcjc.configModules.Test(ctx, config)}
		}
	case configOperationGet:
		operation = func(ctx context.Context, config confgroup.Config) configOperationResult {
			payload, err := dcjc.configModules.Configuration(ctx, config)
			return configOperationResult{payload: payload, err: err}
		}
	default:
		return nil, errors.New("job output: unknown config operation")
	}
	result, err := dcjc.runPreparedConfigOperation(ctx, config, kind, yieldClaims, operation)
	return result.payload, errors.Join(err, result.err)
}

func (dcjc *DynCfgJobController) runPreparedConfigOperation(
	ctx context.Context,
	config confgroup.Config,
	kind configOperationKind,
	yieldClaims bool,
	operation func(context.Context, confgroup.Config) configOperationResult,
) (configOperationResult, error) {
	stage, err := newPreparedConfigOperation(dcjc.factory, config, kind, operation)
	if err != nil {
		return configOperationResult{}, err
	}
	defer stage.Release()
	if yieldClaims {
		waitErr, claimErr := dcjc.factory.runWithoutClaims(
			ctx,
			func(yielded context.Context) error {
				return stage.Await(yielded)
			},
		)
		if err := errors.Join(waitErr, claimErr); err != nil {
			return configOperationResult{}, err
		}
	} else if err := stage.Await(ctx); err != nil {
		return configOperationResult{}, err
	}
	if cause := context.Cause(ctx); cause != nil {
		return configOperationResult{}, cause
	}
	return stage.take()
}
