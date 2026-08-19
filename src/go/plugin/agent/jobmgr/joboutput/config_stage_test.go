// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestDynCfgTestContainsNonCooperativeCollectorPerRawConfigIdentity(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := controller.factory.config.Attempts.(*containment.Authority)
	entered := make(chan struct{})
	release := make(chan struct{})
	var enteredOnce sync.Once
	var cleanup atomic.Int32
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := &collectorapi.MockCollectorV1{}
		module.CheckFunc = func(context.Context) error {
			if module.Config.OptionStr == "block" {
				enteredOnce.Do(func() { close(entered) })
				<-release
			}
			return nil
		}
		module.CleanupFunc = func(context.Context) {
			cleanup.Add(1)
		}
		return module
	}
	controller.modules["module"] = creator
	controller.factory.config.Modules = controller.modules
	controller.configModules.config.Modules = controller.modules

	blocked := DynCfgJobRequest{
		Args:         []string{"go.d:collector:module", "test", "job"},
		Payload:      []byte(`{"option_str":"block","option_int":1}`),
		ContentType:  "application/json",
		CallerSource: "user=test",
		HasPayload:   true,
	}
	ctx, cancel := context.WithCancel(context.Background())
	firstDone := make(chan error, 1)
	go func() {
		_, err := controller.Handle(ctx, blocked)
		firstDone <- err
	}()
	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "collector Check was not entered")
	}
	cancel()
	select {
	case err := <-firstDone:
		require.ErrorIs(t, err, context.Canceled)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "canceled DynCfg test did not settle")
	}

	result, err := controller.Handle(context.Background(), blocked)
	require.NoError(t, err)
	require.Equal(t, mustDynCfgMessage(503, jobmgr.ErrProcessAttemptBusy.Error()), result)

	healthy := blocked
	healthy.Payload = []byte(`{"option_str":"healthy","option_int":2}`)
	result, err = controller.Handle(context.Background(), healthy)
	require.NoError(t, err)
	require.Equal(t, mustDynCfgMessage(200, ""), result)

	blockedConfig, failure := controller.parseConfig(blocked, "module", "job")
	require.False(t, failure.valid)
	_, err = controller.runConfigOperation(
		context.Background(),
		blockedConfig,
		configOperationValidate,
		true,
	)
	require.NoError(t, err)

	close(release)
	require.Eventually(t, func() bool {
		return attempts.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 3, cleanup.Load())
}

func TestContainedConfigValidationYieldsGraphClaimForAllCallbacks(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	var outsideClaim atomic.Bool
	var callbacks atomic.Int32
	var violations atomic.Int32
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		callbacks.Add(1)
		if !outsideClaim.Load() {
			violations.Add(1)
		}
		module := &collectorapi.MockCollectorV1{}
		module.CleanupFunc = func(context.Context) {
			callbacks.Add(1)
			if !outsideClaim.Load() {
				violations.Add(1)
			}
		}
		return module
	}
	controller.modules["module"] = creator
	controller.factory.config.Modules = controller.modules
	controller.configModules.config.Modules = controller.modules
	controller.factory.runWithoutClaims = func(
		ctx context.Context,
		work func(context.Context) error,
	) (error, error) {
		outsideClaim.Store(true)
		defer outsideClaim.Store(false)
		return work(ctx), nil
	}

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	_, err := controller.runConfigOperation(
		context.Background(),
		config,
		configOperationValidate,
		true,
	)
	require.NoError(t, err)
	require.EqualValues(t, 2, callbacks.Load())
	require.Zero(t, violations.Load())
}

func TestConfigOperationAwaitDoesNotStartAfterCallerCancellation(t *testing.T) {
	attempts := &validationSupersedeAuthority{}
	factory := &Factory{
		config: FactoryConfig{
			Epoch:    1,
			Attempts: attempts,
		},
	}
	var calls atomic.Int32
	stage, err := newPreparedConfigOperation(
		factory,
		factoryTestConfig(false),
		configOperationTest,
		func(context.Context, confgroup.Config) ([]byte, error) {
			calls.Add(1)
			return nil, nil
		},
	)
	require.NoError(t, err)
	t.Cleanup(stage.Release)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err = stage.Await(ctx)

	require.ErrorIs(t, err, context.Canceled)
	select {
	case <-stage.Ready():
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "canceled config operation did not settle")
	}
	stage.mu.Lock()
	started := stage.started
	stage.mu.Unlock()
	require.False(t, started)
	require.Zero(t, attempts.started.Load())
	require.Zero(t, calls.Load())
}

func TestConfigValidationSupersedesEarlierSameJobOperation(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := &validationSupersedeAuthority{}
	controller.factory.config.Attempts = attempts
	config := factoryTestConfig(false)

	_, err := controller.runConfigOperation(
		context.Background(),
		config,
		configOperationValidate,
		true,
	)
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptBusy)
	require.EqualValues(t, 1, attempts.superseded.Load())
	require.EqualValues(t, 1, attempts.started.Load())
}

func TestConfigOperationRejectsSuccessReturnedAfterLogicalCut(t *testing.T) {
	attempts := &delayedDispositionAuthority{}
	factory := &Factory{
		config: FactoryConfig{
			Epoch:    1,
			Attempts: attempts,
		},
	}
	config := factoryTestConfig(false)
	entered := make(chan struct{})
	release := make(chan struct{})
	stage, err := newPreparedConfigOperation(
		factory,
		config,
		configOperationTest,
		func(context.Context, confgroup.Config) ([]byte, error) {
			close(entered)
			<-release
			return []byte(`{"accepted":true}`), nil
		},
	)
	require.NoError(t, err)
	stage.Start()
	<-entered
	require.Eventually(t, func() bool {
		stage.mu.Lock()
		defer stage.mu.Unlock()
		return stage.attempt != nil
	}, time.Second, time.Millisecond)

	stage.Cancel(jobmgr.ErrProcessAttemptSuperseded)
	close(release)
	require.NoError(t, stage.Await(context.Background()))
	payload, err := stage.take()
	require.ErrorIs(t, err, jobmgr.ErrProcessAttemptSuperseded)
	require.Nil(t, payload)
	stage.Release()
}

type validationSupersedeAuthority struct {
	superseded atomic.Int32
	started    atomic.Int32
}

type delayedDispositionAuthority struct{}

type delayedDispositionAttempt struct {
	mu       sync.Mutex
	cancel   context.CancelCauseFunc
	cut      error
	workErr  error
	released chan struct{}
}

func (*delayedDispositionAuthority) StartProcessAttempt(
	_ context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	ctx, cancel := context.WithCancelCause(context.Background())
	attempt := &delayedDispositionAttempt{
		cancel:   cancel,
		released: make(chan struct{}),
	}
	go func() {
		workErr := plan.Work(ctx, successfulProcessAttemptAdmission{})
		attempt.mu.Lock()
		attempt.workErr = workErr
		attempt.mu.Unlock()
		close(attempt.released)
	}()
	return attempt, nil
}

type successfulProcessAttemptAdmission struct{}

func (successfulProcessAttemptAdmission) Admit() error {
	return nil
}

func (*delayedDispositionAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	return nil
}

func (*delayedDispositionAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (*delayedDispositionAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}

func (*delayedDispositionAttempt) Admit() error {
	return nil
}

func (dda *delayedDispositionAttempt) Cut(cause error) bool {
	dda.mu.Lock()
	defer dda.mu.Unlock()
	if dda.cut != nil {
		return false
	}
	dda.cut = cause
	dda.cancel(cause)
	return true
}

func (dda *delayedDispositionAttempt) Await(ctx context.Context) error {
	select {
	case <-dda.released:
	case <-ctx.Done():
		dda.Cut(context.Cause(ctx))
		<-dda.released
	}
	dda.mu.Lock()
	defer dda.mu.Unlock()
	if dda.cut != nil {
		return dda.cut
	}
	return dda.workErr
}

func (dda *delayedDispositionAttempt) Released() <-chan struct{} {
	return dda.released
}

func (vsa *validationSupersedeAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	vsa.started.Add(1)
	return nil, jobmgr.ErrProcessAttemptBusy
}

func (vsa *validationSupersedeAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	vsa.superseded.Add(1)
	return nil
}

func (*validationSupersedeAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (*validationSupersedeAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}
