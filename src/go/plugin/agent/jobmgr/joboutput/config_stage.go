// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"strconv"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

type configOperationKind uint8

const (
	configOperationValidate configOperationKind = iota + 1
	configOperationTest
	configOperationGet
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
	operation func(context.Context, confgroup.Config) ([]byte, error)
	supersede bool
	attempt   jobmgr.ProcessAttempt
	result    configOperationResult

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
	operation func(context.Context, confgroup.Config) ([]byte, error),
) (*preparedConfigOperation, error) {
	if factory == nil || factory.config.Attempts == nil || factory.config.Epoch == 0 ||
		config == nil || config.FullName() == "" || operation == nil {
		return nil, errors.New("job output: invalid config operation")
	}
	cloned, err := config.Clone()
	if err != nil {
		return nil, err
	}
	namespace := jobmgr.ProcessAttemptJobTest
	key := strconv.Itoa(int(kind)) + "\x00" + cloned.FullName() + "\x00" +
		strconv.FormatUint(cloned.Hash(), 10)
	if kind == configOperationValidate {
		namespace = jobmgr.ProcessAttemptJob
		key = cloned.FullName()
	}
	ctx, cancel := context.WithCancelCause(context.Background())
	return &preparedConfigOperation{
		attempts: factory.config.Attempts,
		identity: jobmgr.ProcessAttemptIdentity{
			Namespace: namespace,
			Key:       key,
			Resource:  candidateDiagnosticResource(cloned.FullName()),
		},
		target:    factory.config.Epoch,
		config:    cloned,
		operation: operation,
		supersede: kind == configOperationValidate,
		ctx:       ctx,
		cancel:    cancel,
		ready:     make(chan struct{}),
	}, nil
}

func (stage *preparedConfigOperation) Start() {
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

func (stage *preparedConfigOperation) Ready() <-chan struct{} {
	if stage == nil {
		return nil
	}
	return stage.ready
}

func (stage *preparedConfigOperation) Cancel(cause error) {
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

func (stage *preparedConfigOperation) Release() {
	if stage == nil {
		return
	}
	stage.mu.Lock()
	if stage.released {
		stage.mu.Unlock()
		return
	}
	stage.released = true
	taken := stage.taken
	started := stage.started
	attempt := stage.attempt
	stage.config = nil
	stage.operation = nil
	stage.mu.Unlock()
	if taken {
		return
	}
	stage.Cancel(context.Canceled)
	if !started {
		stage.publish(configOperationResult{err: context.Canceled})
	} else if attempt != nil {
		attempt.Cut(context.Canceled)
	}
}

func (stage *preparedConfigOperation) Await(ctx context.Context) error {
	if stage == nil || ctx == nil {
		return errors.New("job output: invalid config operation wait")
	}
	stage.Start()
	select {
	case <-stage.Ready():
		return nil
	case <-ctx.Done():
		stage.Cancel(context.Cause(ctx))
		return context.Cause(ctx)
	}
}

func (stage *preparedConfigOperation) start() {
	if cause := context.Cause(stage.ctx); cause != nil {
		stage.publish(configOperationResult{err: cause})
		return
	}
	if stage.supersede {
		if err := stage.attempts.SupersedeProcessAttempt(stage.ctx, stage.identity); err != nil {
			stage.publish(configOperationResult{err: err})
			return
		}
	}
	workerResult := make(chan configOperationResult, 1)
	attempt, err := stage.attempts.StartProcessAttempt(jobmgr.ProcessAttemptPlan{
		Identity: stage.identity,
		Target:   stage.target,
		Work: func(ctx context.Context, _ jobmgr.ProcessAttemptAdmission) error {
			stage.mu.Lock()
			config := stage.config
			operation := stage.operation
			stage.mu.Unlock()
			if config == nil || operation == nil {
				return context.Canceled
			}
			payload, workErr := operation(ctx, config)
			if cause := context.Cause(ctx); cause != nil {
				return cause
			}
			workerResult <- configOperationResult{
				payload: payload,
				err:     workErr,
			}
			return nil
		},
	})
	if err != nil {
		stage.publish(configOperationResult{err: err})
		return
	}
	stage.mu.Lock()
	stage.attempt = attempt
	stage.mu.Unlock()
	if cause := context.Cause(stage.ctx); cause != nil {
		attempt.Cut(cause)
	}
	if err := attempt.Await(context.Background()); err != nil {
		stage.publish(configOperationResult{err: err})
		return
	}
	stage.publish(<-workerResult)
}

func (stage *preparedConfigOperation) publish(result configOperationResult) {
	stage.readyOnce.Do(func() {
		stage.mu.Lock()
		if !stage.released {
			stage.result = result
		}
		stage.settled = true
		stage.config = nil
		stage.mu.Unlock()
		close(stage.ready)
	})
}

func (stage *preparedConfigOperation) take() ([]byte, error) {
	if stage == nil {
		return nil, errors.New("job output: nil config operation")
	}
	select {
	case <-stage.ready:
	default:
		return nil, errors.New("job output: config operation is not ready")
	}
	stage.mu.Lock()
	defer stage.mu.Unlock()
	if !stage.settled || stage.taken {
		return nil, errors.New("job output: config operation already consumed")
	}
	stage.taken = true
	result := stage.result
	stage.result = configOperationResult{}
	return result.payload, result.err
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
	var operation func(context.Context, confgroup.Config) ([]byte, error)
	switch kind {
	case configOperationValidate:
		operation = func(ctx context.Context, config confgroup.Config) ([]byte, error) {
			return nil, dcjc.factory.ValidateConfig(ctx, config)
		}
	case configOperationTest:
		operation = func(ctx context.Context, config confgroup.Config) ([]byte, error) {
			return nil, dcjc.configModules.Test(ctx, config)
		}
	case configOperationGet:
		operation = dcjc.configModules.Configuration
	default:
		return nil, errors.New("job output: unknown config operation")
	}
	stage, err := newPreparedConfigOperation(dcjc.factory, config, kind, operation)
	if err != nil {
		return nil, err
	}
	defer stage.Release()
	if yieldClaims {
		waitErr, claimErr := dcjc.factory.config.RunWithoutClaims(
			ctx,
			func(yielded context.Context) error {
				return stage.Await(yielded)
			},
		)
		if err := errors.Join(waitErr, claimErr); err != nil {
			return nil, err
		}
	} else if err := stage.Await(ctx); err != nil {
		return nil, err
	}
	return stage.take()
}
