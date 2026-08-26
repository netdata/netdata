// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestAcceptedActivationInstallsV1AndV2Collectors(t *testing.T) {
	tests := map[string]func(*factoryTestState) collectorapi.Creator{
		"V1": func(state *factoryTestState) collectorapi.Creator {
			return collectorapi.Creator{
				Create: func() collectorapi.CollectorV1 {
					module := state.module(nil, false)
					charts := collectorapi.Charts{
						&collectorapi.Chart{
							ID:    "work",
							Title: "Work",
							Units: "units",
							Dims:  collectorapi.Dims{{ID: "value"}},
						},
					}
					module.ChartsFunc = func() *collectorapi.Charts { return &charts }
					module.CollectFunc = func(context.Context) map[string]int64 {
						return map[string]int64{"value": 1}
					}
					return module
				},
			}
		},
		"V2": func(state *factoryTestState) collectorapi.Creator {
			return collectorapi.Creator{
				CreateV2: func() collectorapi.CollectorV2 {
					return &factoryTestV2{
						state:    state,
						store:    metrix.NewCollectorStore(),
						template: factoryTestChartTemplate,
					}
				},
			}
		},
	}
	for name, newCreator := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			creator := newCreator(state)
			controller.modules["module"] = creator

			releaseSubmission := make(chan struct{})
			var releaseOnce sync.Once
			commands := &autoDetectionRetryTestCommands{block: releaseSubmission}
			require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(error) {}))
			var active lifecycle.ReadyResource
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(releaseSubmission) })
				controller.scheduler.StopBackgroundWorkers()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
				if active != nil {
					require.NoError(t, active.Stop(context.Background()))
					require.NoError(t, active.Finalize())
				}
			})

			config := acceptedActivationTestConfig(confgroup.TypeUser)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
			applyAcceptedEnableForTest(t, controller, graph, config, 1)

			commands.waitForSubmissions(t, 1)
			submitted, plans, waited := commands.snapshot()
			require.Len(t, submitted, 1)
			require.Len(t, plans, 1)
			require.True(t, waited)
			require.Equal(t, "internal/jobs/accepted-activation", submitted[0].Route)

			permit, tasks := issueTestJobPermit(t, config.FullName(), 2)
			terminal, err := plans[0].Transaction.Prepare(
				context.Background(),
				nil,
				lifecycle.ResourceTransactionScope{
					ID: config.FullName(),
					Successor: lifecycle.ResourceIdentity{
						ID: config.FullName(), Generation: 2,
					},
				},
				permit,
			)
			require.NoError(t, err)
			applied, err := terminal.Apply(context.Background())
			require.NoError(t, err)
			var disposition lifecycle.ResourceTransactionDisposition
			_, disposition, active = applied.Ownership()
			require.Equal(t, lifecycle.ResourceTransactionInstalled, disposition)
			require.NotNil(t, active)
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusRunning.String(), record.Status)

			releaseOnce.Do(func() { close(releaseSubmission) })
			controller.scheduler.StopBackgroundWorkers()
			require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
			require.NoError(t, active.Stop(context.Background()))
			require.NoError(t, active.Finalize())
			active = nil
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
			requireFactoryAttemptsIdle(t, controller.factory)
			require.EqualValues(t, 1, state.collectorCleanup)
		})
	}
}

func TestAcceptedActivationCoalescesRepeatedEnableForSameConfig(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	checkRelease := make(chan struct{})
	var checkCalls atomic.Int32
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		return state.module(func(context.Context) error {
			checkCalls.Add(1)
			<-checkRelease
			return nil
		}, false)
	}
	controller.modules["module"] = creator

	releaseSubmission := make(chan struct{})
	var checkReleaseOnce sync.Once
	var submissionReleaseOnce sync.Once
	commands := &autoDetectionRetryTestCommands{block: releaseSubmission}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(error) {}))
	t.Cleanup(func() {
		checkReleaseOnce.Do(func() { close(checkRelease) })
		submissionReleaseOnce.Do(func() { close(releaseSubmission) })
		controller.scheduler.StopBackgroundWorkers()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
	})

	config := acceptedActivationTestConfig(confgroup.TypeUser)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
	applyAcceptedEnableForTest(t, controller, graph, config, 1)
	require.Eventually(t, func() bool {
		return checkCalls.Load() == 1
	}, time.Second, time.Millisecond)
	applyAcceptedEnableForTest(t, controller, graph, config, 2)
	require.EqualValues(t, 1, checkCalls.Load())

	checkReleaseOnce.Do(func() { close(checkRelease) })
	commands.waitForSubmissions(t, 1)
	time.Sleep(20 * time.Millisecond)
	submitted, _, waited := commands.snapshot()
	require.Len(t, submitted, 1)
	require.True(t, waited)
}

func TestAcceptedActivationArmsOnlyAfterEnableApplies(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	var checkCalls atomic.Int32
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		return state.module(func(context.Context) error {
			checkCalls.Add(1)
			return nil
		}, false)
	}
	controller.modules["module"] = creator
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(error) {}))
	t.Cleanup(func() {
		controller.scheduler.StopBackgroundWorkers()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(context.Background()))
	})

	config := acceptedActivationTestConfig(confgroup.TypeUser)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	transaction, err := controller.prepareEnable(
		context.Background(),
		dynCfgTarget{
			module:     config.Module(),
			name:       config.Name(),
			resourceID: config.FullName(),
			creator:    creator,
		},
		record,
		true,
		nil,
		lifecycle.ResourceTransactionScope{
			ID: config.FullName(),
			Successor: lifecycle.ResourceIdentity{
				ID: config.FullName(), Generation: 1,
			},
		},
		permit,
	)
	require.NoError(t, err)
	current, err := transaction.Dispose(context.Background())
	require.NoError(t, err)
	require.Nil(t, current)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	commands.waitForSubmissions(t, 0)
	require.Zero(t, checkCalls.Load())
	controller.scheduler.accepted.mu.Lock()
	entries := len(controller.scheduler.accepted.entries)
	controller.scheduler.accepted.mu.Unlock()
	require.Zero(t, entries)
}

func TestAcceptedActivationFailsRunWhenTerminalSubmissionFails(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	sentinel := errors.New("terminal submission failed")
	commands := &autoDetectionRetryTestCommands{submitErr: sentinel}
	failed := make(chan error, 1)
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) {
		failed <- err
	}))
	t.Cleanup(func() {
		controller.scheduler.StopBackgroundWorkers()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.ErrorIs(t, controller.scheduler.WaitBackgroundWorkers(ctx), sentinel)
	})

	config := acceptedActivationTestConfig(confgroup.TypeUser)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
	applyAcceptedEnableForTest(t, controller, graph, config, 1)

	select {
	case err := <-failed:
		require.ErrorIs(t, err, sentinel)
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "terminal submission failure did not fail the run")
	}
	_, plans, waited := commands.snapshot()
	require.Len(t, plans, 1)
	require.True(t, waited)
}

func TestAcceptedActivationCurrentContentionConvergesToFailed(t *testing.T) {
	tests := map[string]func(*DynCfgJobController, *factoryTestState, confgroup.Config){
		"busy candidate identity": func(controller *DynCfgJobController, _ *factoryTestState, _ confgroup.Config) {
			controller.factory.config.Attempts = busySupersessionAuthority{
				ProcessAttemptAuthority: controller.factory.config.Attempts,
			}
		},
		"stale secret-store snapshot": func(
			controller *DynCfgJobController,
			state *factoryTestState,
			config confgroup.Config,
		) {
			scopeState := &factoryTestAtomicScope{value: "resolved"}
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
				module := state.module(func(context.Context) error {
					scopeState.current.Store(false)
					return nil
				}, false)
				charts := collectorapi.Charts{}
				module.ChartsFunc = func() *collectorapi.Charts { return &charts }
				return module
			}
			controller.modules["module"] = creator
			controller.factory.config.ConfigModules.config.StoreScope =
				func([]string) (secretresolver.AtomicScope, error) {
					scopeState.current.Store(true)
					return scopeState, nil
				}
			config["option_str"] = "${store:vault:test:key}"
		},
	}
	for name, arrange := range tests {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			config := acceptedActivationTestConfig(confgroup.TypeUser)
			config.Set("autodetection_retry", 1)
			arrange(controller, state, config)

			releaseSubmission := make(chan struct{})
			var releaseOnce sync.Once
			commands := &autoDetectionRetryTestCommands{block: releaseSubmission}
			require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(error) {}))
			t.Cleanup(func() {
				releaseOnce.Do(func() { close(releaseSubmission) })
				controller.scheduler.StopBackgroundWorkers()
				ctx, cancel := context.WithTimeout(context.Background(), time.Second)
				defer cancel()
				require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
			})

			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
			applyAcceptedEnableForTest(t, controller, graph, config, 1)
			commands.waitForSubmissions(t, 1)
			_, plans, waited := commands.snapshot()
			require.Len(t, plans, 1)
			require.True(t, waited)

			permit, tasks := issueTestJobPermit(t, config.FullName(), 2)
			terminal, err := plans[0].Transaction.Prepare(
				context.Background(),
				nil,
				lifecycle.ResourceTransactionScope{
					ID: config.FullName(),
					Successor: lifecycle.ResourceIdentity{
						ID: config.FullName(), Generation: 2,
					},
				},
				permit,
			)
			require.NoError(t, err)
			applied, err := terminal.Apply(context.Background())
			require.NoError(t, err)
			_, disposition, current := applied.Ownership()
			require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
			require.Nil(t, current)
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

			controller.scheduler.retries.mu.Lock()
			_, retryScheduled := controller.scheduler.retries.entries[config.FullName()]
			controller.scheduler.retries.mu.Unlock()
			require.True(t, retryScheduled)
			releaseOnce.Do(func() { close(releaseSubmission) })
		})
	}
}

func acceptedActivationTestConfig(sourceType string) confgroup.Config {
	config := factoryTestConfig(false)
	config.SetProvider(sourceType)
	config.SetSourceType(sourceType)
	config.SetSource("test")
	return config
}

func applyAcceptedEnableForTest(
	t *testing.T,
	controller *DynCfgJobController,
	graph *dyncfg.Graph,
	config confgroup.Config,
	generation uint64,
) {
	t.Helper()
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	permit, tasks := issueTestJobPermit(t, config.FullName(), generation)
	transaction, err := controller.prepareEnable(
		context.Background(),
		dynCfgTarget{
			module:     config.Module(),
			name:       config.Name(),
			resourceID: config.FullName(),
			creator:    controller.modules[config.Module()],
		},
		record,
		true,
		nil,
		lifecycle.ResourceTransactionScope{
			ID: config.FullName(),
			Successor: lifecycle.ResourceIdentity{
				ID: config.FullName(), Generation: generation,
			},
		},
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	require.Equal(t, 202, applied.ResultStatus())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}
