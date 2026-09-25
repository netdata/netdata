// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/jobruntime"
	"github.com/stretchr/testify/require"
)

func TestSecretDependentStopQuiescesEnabledIntent(t *testing.T) {
	for name, test := range map[string]struct {
		status    dyncfg.Status
		installed bool
		stop      bool
	}{
		"running":  {dyncfg.StatusRunning, true, true},
		"starting": {dyncfg.StatusAccepted, true, true},
		"waiting":  {dyncfg.StatusAccepted, false, true},
		"passive":  {dyncfg.StatusAccepted, false, false},
		"disabled": {dyncfg.StatusDisabled, false, false},
		"failed":   {dyncfg.StatusFailed, false, false},
	} {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			commands := bindHandoffTestWorkers(t, controller)
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error {
					if name == "running" {
						ready()
					}
					<-ctx.Done()
					return ctx.Err()
				}}
			})
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			var current lifecycle.ReadyResource
			t.Cleanup(func() { stopRuntimeTestResource(t, current) })
			if test.stop {
				if name == "waiting" {
					config.Set("vnode", "missing")
					controller.factory.config.Vnode = func(string) (jobruntime.VnodeSnapshot, bool) {
						return jobruntime.VnodeSnapshot{}, false
					}
				}
				seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusDisabled)
				applyAcceptedEnableForTest(t, controller, graph, config, 1)
				current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/accepted-activation"), nil, 2)
				if test.installed {
					require.NotNil(t, current)
				} else {
					require.Nil(t, current)
				}
				if name == "running" {
					current = applyActivationTestSubmission(t, commands.next(t, "internal/jobs/runtime-ready"), current, 0)
				} else {
					require.True(t, controller.ActivationEnabled(config.FullName()))
				}
			} else {
				seedDynCfgJobGraphRecord(t, graph, config, test.status)
			}
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, test.status.String(), record.Status)
			scope := lifecycle.ResourceTransactionScope{ID: config.FullName()}
			if current != nil {
				scope.Current = current.Identity()
			}
			work, stopped, err := controller.PlanSecretDependentStop(scope.ID)
			require.NoError(t, err)
			transaction, err := work.Transaction.Prepare(t.Context(), current, scope, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			applied, err := transaction.Apply(t.Context())
			require.NoError(t, err)
			didStop, err := stopped.Stopped()
			require.NoError(t, err)
			require.Equal(t, test.stop, didStop)
			_, disposition, resource := applied.Ownership()
			current = resource
			require.Nil(t, resource)
			if test.installed {
				require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
			}
			record, exists = graph.Lookup(scope.ID)
			require.True(t, exists)
			if test.stop {
				require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
				require.True(t, controller.ActivationEnabled(scope.ID))
			} else {
				require.Equal(t, test.status.String(), record.Status)
				require.False(t, controller.ActivationEnabled(scope.ID))
			}
			requireNoHandoffActivation(t, commands)
		})
	}
}

func TestSecretDependentStartResumesWithoutPreflight(t *testing.T) {
	for _, deadline := range []bool{false, true} {
		t.Run(map[bool]string{false: "apply", true: "child deadline"}[deadline], func(t *testing.T) {
			controller, graph, _, _, state := newDynCfgJobTestHarness(t)
			var constructions atomic.Int32
			creator := controller.modules["module"]
			creator.Create = func() collectorapi.CollectorV1 {
				constructions.Add(1)
				return state.module(nil, false)
			}
			controller.modules["module"] = creator
			commands := bindSecretRestartTestWorkers(t, controller)
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
			spec, err := controller.configModules.newAcceptedActivationSpec(config)
			require.NoError(t, err)
			controller.scheduler.accepted.pause(spec)
			work, started, err := controller.PlanSecretDependentStart(config.FullName())
			require.NoError(t, err)
			require.False(t, work.Transaction.AllocateSuccessor)
			require.Empty(t, work.YieldClaimOnPrepare)
			require.Zero(t, constructions.Load())
			if deadline {
				started.RetainPending()
				started.RetainPending()
			} else {
				transaction, err := work.Transaction.Prepare(t.Context(), nil,
					lifecycle.ResourceTransactionScope{ID: config.FullName()}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				require.Zero(t, constructions.Load(), "resume preparation must not construct a collector")
				_, err = transaction.Apply(t.Context())
				require.NoError(t, err)
			}
			require.NoError(t, started.Err())
			commands.waitForSubmissions(t, 1)
			submissions, _, _ := commands.snapshot()
			require.Len(t, submissions, 1)
			require.Equal(t, "internal/jobs/accepted-activation", submissions[0].Route)
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
		})
	}
}

func TestSecretDependentStartDoesNotEnablePassiveDisabledOrRemovedJobs(t *testing.T) {
	for name, status := range map[string]dyncfg.Status{
		"passive": dyncfg.StatusAccepted, "disabled": dyncfg.StatusDisabled, "removed": "",
	} {
		t.Run(name, func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			commands := bindSecretRestartTestWorkers(t, controller)
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			if name != "removed" {
				seedDynCfgJobGraphRecord(t, graph, config, status)
			}
			work, started, err := controller.PlanSecretDependentStart(config.FullName())
			require.NoError(t, err)
			transaction, err := work.Transaction.Prepare(t.Context(), nil,
				lifecycle.ResourceTransactionScope{ID: config.FullName()}, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			_, err = transaction.Apply(t.Context())
			require.NoError(t, err)
			started.RetainPending()
			commands.waitForSubmissions(t, 0)
			record, exists := graph.Lookup(config.FullName())
			require.Equal(t, name != "removed", exists)
			if exists {
				require.Equal(t, status.String(), record.Status)
			}
		})
	}
}

func TestSecretDependentDeadlineCannotResumeRevokedGeneration(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "disabled", true: "removed"}[remove], func(t *testing.T) {
			controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
			commands := bindSecretRestartTestWorkers(t, controller)
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusAccepted)
			spec, err := controller.configModules.newAcceptedActivationSpec(config)
			require.NoError(t, err)
			controller.scheduler.accepted.pause(spec)
			_, started, err := controller.PlanSecretDependentStart(config.FullName())
			require.NoError(t, err)
			record, _ := graph.Lookup(config.FullName())
			target := dynCfgTarget{module: config.Module(), name: config.Name(), resourceID: config.FullName()}
			scope := lifecycle.ResourceTransactionScope{ID: config.FullName()}
			var transaction lifecycle.PreparedResourceTransaction
			if remove {
				transaction, err = controller.prepareRemove(target, record, true, nil, scope)
			} else {
				transaction, err = controller.prepareDisable(target, record, true, nil, scope)
			}
			require.NoError(t, err)
			_, err = transaction.Apply(t.Context())
			require.NoError(t, err)
			require.False(t, controller.ActivationEnabled(config.FullName()))
			started.RetainPending()
			commands.waitForSubmissions(t, 0)
		})
	}
}

func bindSecretRestartTestWorkers(t *testing.T, controller *DynCfgJobController) *autoDetectionRetryTestCommands {
	t.Helper()
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindBackgroundWorkers(commands, 9, func(err error) { t.Errorf("activation worker failed: %v", err) }))
	t.Cleanup(func() {
		controller.scheduler.StopBackgroundWorkers()
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, controller.scheduler.WaitBackgroundWorkers(ctx))
	})
	return commands
}
