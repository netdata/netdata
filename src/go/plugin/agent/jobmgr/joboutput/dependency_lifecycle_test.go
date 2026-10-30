// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func prepareAdmissionTestEnable(
	t *testing.T,
	controller *DynCfgJobController,
	config confgroup.Config,
) (lifecycle.PreparedResourceTransaction, error) {
	t.Helper()
	return controller.Prepare(
		t.Context(),
		DynCfgJobRequest{
			Args: []string{controller.externalID(config.FullName()), "enable"},
		},
		nil,
		lifecycle.ResourceTransactionScope{
			ID: config.FullName(),
		},
		lifecycle.LongLivedPermit{},
	)
}

func TestNamedDependencyWaitRunningFailureAndDisable(t *testing.T) {
	for _, ending := range []string{"disable", "remove", "joined retirement"} {
		t.Run(ending, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			failure := make(chan struct{})
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{
					run: func(ctx context.Context, ready func()) error {
						ready()
						select {
						case <-failure:
							return collectorapi.TemporaryError(errors.New("runtime failed"))
						case <-ctx.Done():
							return ctx.Err()
						}
					},
				}
			})
			var available atomic.Bool
			creator := controller.modules["module"]
			creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
				return []collectorapi.ActivationDependency{{Kind: "runtime", Name: "browser", Ready: func() error {
					if !available.Load() {
						return errors.New("preparing")
					}
					return nil
				}}}, nil
			}
			controller.modules["module"] = creator
			commands := &activationTestCommands{
				queue: make(chan activationTestSubmission, 16),
				stop:  make(chan struct{}),
			}
			require.NoError(
				t,
				controller.BindBackgroundWorkers(
					commands,
					9,
					func(err error) { t.Errorf("background failure: %v", err) },
				),
			)
			var current lifecycle.ReadyResource
			var stopOnce sync.Once
			t.Cleanup(func() {
				stopOnce.Do(func() { close(commands.stop) })
				controller.scheduler.StopBackgroundWorkers()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
				stopRuntimeTestResource(t, current)
			})
			config := acceptedActivationTestConfig(confgroup.TypeDyncfg).Set("autodetection_retry", 0)
			seedDynCfgJobGraphRecord(t, controller, graph, config, dyncfg.StatusAccepted)
			applyAcceptedEnableForTest(t, controller, graph, config, 1)
			waiting := commands.next(t, "internal/jobs/accepted-activation")
			// Notify after the failed lookup but before the waiting transaction.
			available.Store(true)
			controller.NotifyDependencyChanged("runtime", "browser")
			current = applyActivationTestSubmission(t, waiting, nil, 2)
			require.Nil(t, current)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/candidate-resume"), nil, 0)
			require.Nil(t, current)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 3)
			require.NotNil(t, current)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
			record, _ := graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
			require.False(t, controller.ActivationEnabled(config.FullName()), "transient activation ended")

			close(failure)
			current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-failure"), current, 0)
			require.Nil(t, current)
			record, _ = graph.Lookup(config.FullName())
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			if ending == "joined retirement" {
				stopOnce.Do(func() { close(commands.stop) })
				controller.scheduler.StopBackgroundWorkers()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
			} else {
				transaction, err := controller.Prepare(t.Context(), DynCfgJobRequest{
					Args: []string{controller.externalID(config.FullName()), ending},
				}, nil,
					lifecycle.ResourceTransactionScope{
						ID: config.FullName(),
					}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				_, err = transaction.Apply(t.Context())
				require.NoError(t, err)
			}
		})
	}
}

func TestNamedDependenciesLookupOnlyDuringTest(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	creator := controller.modules["module"]
	var lookups atomic.Int32
	creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
		return []collectorapi.ActivationDependency{
			{
				Kind:  "runtime",
				Name:  "browser",
				Ready: func() error { lookups.Add(1); return errors.New("not installed") },
			},
		}, nil
	}
	controller.modules["module"] = creator
	config := acceptedActivationTestConfig(confgroup.TypeDyncfg)
	require.NoError(t, controller.configModules.Validate(t.Context(), config))
	require.Zero(t, lookups.Load())
	err := controller.configModules.Test(t.Context(), config)
	var dependency *collectorapi.ActivationDependencyUnavailableError
	require.ErrorAs(t, err, &dependency)
	require.True(t, controller.configModules.activationWaitsForDependency(config, redactResolvedLifecycleError(err)))
	require.EqualValues(t, 1, lookups.Load())
}

func TestDependencyRejectedAndAbortedUpdatePreservesIncumbent(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() },
		}
	})
	var unavailable atomic.Bool
	creator := controller.modules["module"]
	creator.ActivationDependencies = func(map[string]any) ([]collectorapi.ActivationDependency, error) {
		return []collectorapi.ActivationDependency{{Kind: "runtime", Name: "browser", Ready: func() error {
			if unavailable.Load() {
				return errors.New("unavailable")
			}
			return nil
		}}}, nil
	}
	controller.modules["module"] = creator
	config := acceptedActivationTestConfig(confgroup.TypeDyncfg)
	commands := bindHandoffTestWorkers(t, controller)
	seedDynCfgJobGraphRecord(t, controller, graph, config, dyncfg.StatusDisabled)
	applyAcceptedEnableForTest(t, controller, graph, config, 1)
	current := applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
	t.Cleanup(func() { stopRuntimeTestResource(t, current) })
	require.NotNil(t, current)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
	before, _ := graph.Lookup(config.FullName())
	request := DynCfgJobRequest{
		Args: []string{controller.externalID(config.FullName()), "update"},
		Payload: []byte(
			`{"option_str":"replacement"}`,
		),
		HasPayload:   true,
		ContentType:  "application/json",
		CallerSource: "user",
	}
	for i, mode := range []string{"unavailable candidate", "aborted adoption", "committed adoption"} {
		t.Run(mode, func(t *testing.T) {
			unavailable.Store(mode == "unavailable candidate")
			generation := uint64(10 + i)
			permit, _ := issueTestJobPermit(t, config.FullName(), generation)
			scope := lifecycle.ResourceTransactionScope{
				ID:      config.FullName(),
				Current: current.Identity(),
				Successor: lifecycle.ResourceIdentity{
					ID:         config.FullName(),
					Generation: generation,
				},
			}
			prepared, err := controller.Prepare(t.Context(), request, current, scope, permit)
			require.NoError(t, err)
			if mode == "aborted adoption" {
				kept, err := prepared.Dispose(t.Context())
				require.NoError(t, err)
				require.Same(t, current, kept)
			} else {
				applied, err := prepared.Apply(t.Context())
				require.NoError(t, err)
				_, _, next := applied.Ownership()
				if mode == "unavailable candidate" {
					require.Equal(t, 503, applied.ResultStatus())
					require.Same(t, current, next)
				} else {
					current = next
					require.Equal(t, 202, applied.ResultStatus())
				}
			}
			if mode != "committed adoption" {
				after, _ := graph.Lookup(config.FullName())
				require.Equal(t, before, after)
			}
		})
	}
}

func TestStartupFailureAutomaticallyRetries(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	var runs atomic.Int32
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{
			run: func(ctx context.Context, ready func()) error {
				if runs.Add(1) == 1 {
					return collectorapi.TemporaryError(errors.New("startup unavailable"))
				}
				ready()
				<-ctx.Done()
				return ctx.Err()
			},
		}
	})
	commands := &activationTestCommands{
		queue: make(chan activationTestSubmission, 16),
		stop:  make(chan struct{}),
	}
	require.NoError(
		t,
		controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("background failure: %v", err) }),
	)
	var current lifecycle.ReadyResource
	t.Cleanup(func() {
		close(commands.stop)
		controller.scheduler.StopBackgroundWorkers()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
		stopRuntimeTestResource(t, current)
	})
	require.NoError(t, controller.scheduler.Tick(t.Context(), 0))
	config := acceptedActivationTestConfig(confgroup.TypeUser).Set("autodetection_retry", 1)
	seedDynCfgJobGraphRecord(t, controller, graph, config, dyncfg.StatusDisabled)
	applyAcceptedEnableForTest(t, controller, graph, config, 1)
	current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
	sawFailed, sawRetry := false, false
	deadline := time.After(3 * time.Second)
	for {
		var call activationTestSubmission
		select {
		case call = <-commands.queue:
		case <-deadline:
			t.Fatal("startup retry did not reach Running")
		}
		var generation uint64
		switch call.request.Route {
		case "internal/jobs/autodetection-retry":
			require.True(t, sawFailed)
			sawRetry = true
		case "internal/jobs/accepted-activation":
			generation = 3
		case "internal/jobs/runtime-ready", "internal/jobs/runtime-failure":
		default:
			t.Fatalf("unexpected route %s", call.request.Route)
		}
		current = applyActivationTestSubmission(t, call, current, generation)
		record, _ := graph.Lookup(config.FullName())
		if record.Status == dyncfg.StatusFailed.String() && !sawFailed {
			sawFailed = true
			require.NoError(t, controller.scheduler.Tick(t.Context(), 1))
		}
		if record.Status == dyncfg.StatusRunning.String() {
			break
		}
	}
	require.True(t, sawFailed)
	require.True(t, sawRetry)
}
