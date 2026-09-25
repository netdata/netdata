// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestAcceptedHandoffUpdateDuringStartupPreservesEnabledIntent(t *testing.T) {
	for name, test := range map[string]struct {
		payload    string
		staleStore bool
	}{
		"same uid":      {payload: `{"option_str":"same"}`},
		"different uid": {payload: `{"option_str":"changed"}`},
		"stale Store":   {payload: `{"option_str":"${store:vault:test:key}"}`, staleStore: true},
	} {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			release := make(chan struct{})
			entered := make(chan struct{})
			var releaseOnce sync.Once
			var constructions atomic.Int32
			var checks atomic.Int32
			retainedCleaned := make(chan struct{})
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				ordinal := constructions.Add(1)
				collector := &runtimeTestCollector{factoryTestV2: &factoryTestV2{}}
				collector.check = func(context.Context) error {
					checks.Add(1)
					return nil
				}
				if ordinal == 1 {
					collector.run = func(context.Context, func()) error {
						close(entered)
						<-release
						return nil
					}
				} else {
					collector.run = func(ctx context.Context, ready func()) error {
						ready()
						<-ctx.Done()
						return ctx.Err()
					}
				}
				if ordinal == 2 {
					collector.cleanup = func() { close(retainedCleaned) }
				}
				return collector
			})
			oldStore := &factoryTestAtomicScope{value: "old"}
			newStore := &factoryTestAtomicScope{value: "new"}
			oldStore.current.Store(true)
			newStore.current.Store(true)
			var resolutions atomic.Int32
			if test.staleStore {
				controller.factory.config.ConfigModules.config.Configs = testConfigResolver(t, testAtomicResolver(t), func([]string) (secretresolver.AtomicScope, error) {
					if resolutions.Add(1) == 1 {
						return oldStore, nil
					}
					return newStore, nil
				})
			}
			commands := bindHandoffTestWorkers(t, controller)
			var current lifecycle.ReadyResource
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(release) })
				stopRuntimeTestResource(t, current)
			})
			request := DynCfgJobRequest{
				Args:         []string{"go.d:collector:module:job", "update"},
				Payload:      []byte(`{"option_str":"same"}`),
				HasPayload:   true,
				ContentType:  "application/json",
				CallerSource: "user",
			}
			config, failure := controller.parseConfig(request, "module", "job")
			require.False(t, failure.valid)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusDisabled)
			applyAcceptedEnableForTest(t, controller, graph, config, 1)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
			require.NotNil(t, current)
			requireTestSignal(t, entered, "initial runtime did not start")
			if released, ok := controller.factory.config.Attempts.ProcessAttemptReleased(jobAttemptIdentity(jobmgr.ProcessAttemptJob, config.FullName())); ok {
				requireTestSignal(t, released, "construction identity did not release")
			}

			request.Payload = []byte(test.payload)
			scope := lifecycle.ResourceTransactionScope{
				ID:        config.FullName(),
				Current:   current.Identity(),
				Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 3},
			}
			permit, _ := issueTestJobPermit(t, config.FullName(), 3)
			prepared, err := controller.Prepare(t.Context(), request, current, scope, permit)
			require.NoError(t, err)
			applied, err := prepared.Apply(t.Context())
			require.NoError(t, err)
			require.Equal(t, 202, applied.ResultStatus())
			_, _, current = applied.Ownership()
			require.Nil(t, current, "acceptance must not wait for the predecessor to release")
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
			require.True(t, controller.ActivationEnabled(config.FullName()))
			if test.staleStore {
				// Invalidate only after successful preflight and acceptance, while
				// the old runtime still prevents promotion of the retained candidate.
				require.EqualValues(t, 1, resolutions.Load())
				oldStore.current.Store(false)
			}

			// Repeated ENABLE must preserve the handed-off candidate and release wait.
			permit, _ = issueTestJobPermit(t, config.FullName(), 4)
			prepared, err = controller.Prepare(t.Context(), DynCfgJobRequest{Args: []string{request.Args[0], "enable"}}, nil,
				lifecycle.ResourceTransactionScope{ID: config.FullName(), Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 4}}, permit)
			require.NoError(t, err)
			_, err = prepared.Apply(t.Context())
			require.NoError(t, err)
			releaseOnce.Do(func() { close(release) })

			// Process both the retired runtime's notification and the new activation.
			// The actual readiness transaction must publish Running for the successor.
			deadline := time.NewTimer(time.Second)
			defer deadline.Stop()
			nextGeneration := uint64(5)
			for {
				record, _ = graph.Lookup(config.FullName())
				if record.Status == dyncfg.StatusRunning.String() {
					break
				}
				select {
				case call := <-commands.queue:
					current = applyActivationTestSubmission(t, call, current, nextGeneration)
					if call.plan.Transaction.AllocateSuccessor {
						nextGeneration++
					}
				case <-deadline.C:
					t.Fatal("accepted replacement did not become running after predecessor release")
				}
			}
			require.NotNil(t, current)
			if test.staleStore {
				requireTestSignal(t, retainedCleaned, "candidate with stale Store generation was not cleaned up")
				require.EqualValues(t, 3, constructions.Load(), "stale candidate must be rebuilt")
				require.EqualValues(t, 2, resolutions.Load(), "replacement must resolve the current Store generation")
				require.EqualValues(t, 3, checks.Load(), "the rebuilt candidate must be probed before startup")
			} else {
				require.EqualValues(t, 2, constructions.Load(), "replacement must reuse its preflight candidate")
				require.EqualValues(t, 2, checks.Load(), "replacement must not probe twice")
			}
			require.False(t, controller.ActivationEnabled(config.FullName()), "runtime settlement must retire activation authority")
		})
	}
}

func TestAcceptedHandoffReleasesCandidateWhenActivationRevoked(t *testing.T) {
	for _, closed := range []bool{false, true} {
		t.Run(map[bool]string{false: "disable while waiting", true: "registration after shutdown"}[closed], func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			cleaned := make(chan struct{})
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{cleanup: func() { close(cleaned) }}
			})
			bindHandoffTestWorkers(t, controller)
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
			stage, err := controller.factory.newCandidate(config)
			require.NoError(t, err)
			require.NoError(t, controller.factory.awaitCandidate(t.Context(), stage))
			spec, err := controller.configModules.newAcceptedActivationSpec(config)
			require.NoError(t, err)
			if closed {
				controller.scheduler.StopBackgroundWorkers()
			}
			controller.scheduler.accepted.armWithGate(spec, make(chan struct{}), nil, stage)
			if !closed {
				prepared, err := controller.Prepare(t.Context(), DynCfgJobRequest{Args: []string{controller.externalID(config.FullName()), "disable"}}, nil,
					lifecycle.ResourceTransactionScope{ID: config.FullName()}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				_, err = prepared.Apply(t.Context())
				require.NoError(t, err)
			}
			requireTestSignal(t, cleaned, "revoked candidate was not cleaned up")
			requireFactoryAttemptsIdle(t, controller.factory)
			require.False(t, controller.ActivationEnabled(config.FullName()))
		})
	}
}

func TestAcceptedHandoffDisposeReleasesPreflightCandidate(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	cleaned := make(chan struct{})
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{cleanup: func() { close(cleaned) }}
	})
	commands := bindHandoffTestWorkers(t, controller)
	config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusFailed)
	before, _ := graph.Lookup(config.FullName())
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	prepared, err := controller.Prepare(t.Context(), DynCfgJobRequest{
		Args:         []string{controller.externalID(config.FullName()), "update"},
		Payload:      []byte(`{"option_str":"changed"}`),
		HasPayload:   true,
		ContentType:  "application/json",
		CallerSource: "user",
	}, nil, lifecycle.ResourceTransactionScope{ID: config.FullName(), Successor: lifecycle.ResourceIdentity{ID: config.FullName(), Generation: 1}}, permit)
	require.NoError(t, err)
	current, err := prepared.Dispose(t.Context())
	require.NoError(t, err)
	require.Nil(t, current)
	requireTestSignal(t, cleaned, "disposed candidate was not cleaned up")
	requireFactoryAttemptsIdle(t, controller.factory)
	require.Equal(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	after, _ := graph.Lookup(config.FullName())
	require.Equal(t, before.Status, after.Status)
	require.Equal(t, before.Payload(), after.Payload())
	require.False(t, controller.ActivationEnabled(config.FullName()))
	requireNoHandoffActivation(t, commands)
}

func bindHandoffTestWorkers(t *testing.T, controller *DynCfgJobController) *activationTestCommands {
	t.Helper()
	commands := &activationTestCommands{queue: make(chan activationTestSubmission, 16), stop: make(chan struct{})}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("activation worker failed: %v", err) }))
	t.Cleanup(func() {
		close(commands.stop)
		controller.scheduler.StopBackgroundWorkers()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
	})
	return commands
}

func requireNoHandoffActivation(t *testing.T, commands *activationTestCommands) {
	t.Helper()
	deadline := time.NewTimer(30 * time.Millisecond)
	defer deadline.Stop()
	for {
		select {
		case call := <-commands.queue:
			require.NotEqual(t, "internal/jobs/accepted-activation", call.request.Route, "revoked or paused intent must not activate")
			if call.ack != nil {
				call.ack <- nil
			}
		case <-deadline.C:
			return
		}
	}
}
