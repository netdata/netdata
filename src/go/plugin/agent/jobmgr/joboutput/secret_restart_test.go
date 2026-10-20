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
				return &runtimeTestCollector{
					run: func(ctx context.Context, ready func()) error {
						if name == "running" {
							ready()
						}
						<-ctx.Done()
						return ctx.Err()
					},
				}
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
				seedDynCfgJobGraphRecord(t, controller, graph, config, dyncfg.StatusDisabled)
				applyAcceptedEnableForTest(t, controller, graph, config, 1)
				current = applyActivationTestSubmission(
					t,
					commands.next(t, "internal/jobs/accepted-activation"),
					nil,
					2,
				)
				if test.installed {
					require.NotNil(t, current)
				} else {
					require.Nil(t, current)
				}
				if name == "running" {
					current = applyActivationTestSubmission(
						t,
						commands.next(t, "internal/jobs/runtime-ready"),
						current,
						0,
					)
				} else {
					require.True(t, controller.ActivationEnabled(config.FullName()))
				}
			} else {
				seedDynCfgJobGraphRecord(t, controller, graph, config, test.status)
			}
			record, exists := graph.Lookup(config.FullName())
			require.True(t, exists)
			require.Equal(t, test.status.String(), record.Status)
			scope := lifecycle.ResourceTransactionScope{
				ID: config.FullName(),
			}
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
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			seedSecretPausedTest(t, controller, graph, config)
			commands := bindSecretRestartTestWorkers(t, controller)
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
					lifecycle.ResourceTransactionScope{
						ID: config.FullName(),
					}, lifecycle.LongLivedPermit{})
				require.NoError(t, err)
				require.Zero(t, constructions.Load(), "resume preparation must not construct a collector")
				_, err = transaction.Apply(t.Context())
				require.NoError(t, err)
			}
			require.NoError(t, started.Err())

			if deadline {
				resume := commands.next(t, "internal/jobs/candidate-resume")
				require.NotNil(t, resume.ack, "resume delivery is supervised")
				require.Zero(t, constructions.Load(), "retaining resume does not construct a collector")
				require.Nil(t, applyActivationTestSubmission(t, resume, nil, 0))
			}
			activation := commands.next(t, "internal/jobs/accepted-activation")
			require.NotNil(t, activation.ack)
			require.True(t, activation.plan.Transaction.AllocateSuccessor)
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
				seedDynCfgJobGraphRecord(t, controller, graph, config, status)
			}
			work, started, err := controller.PlanSecretDependentStart(config.FullName())
			require.NoError(t, err)
			transaction, err := work.Transaction.Prepare(t.Context(), nil,
				lifecycle.ResourceTransactionScope{
					ID: config.FullName(),
				}, lifecycle.LongLivedPermit{})
			require.NoError(t, err)
			_, err = transaction.Apply(t.Context())
			require.NoError(t, err)
			started.RetainPending()
			requireNoSecretSubmission(t, commands)
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
			config := factoryTestConfig(false).SetSourceType(confgroup.TypeDyncfg)
			seedSecretPausedTest(t, controller, graph, config)
			commands := bindSecretRestartTestWorkers(t, controller)
			_, started, err := controller.PlanSecretDependentStart(config.FullName())
			require.NoError(t, err)
			record, _ := graph.Lookup(config.FullName())
			target := dynCfgTarget{
				module:     config.Module(),
				name:       config.Name(),
				resourceID: config.FullName(),
			}
			scope := lifecycle.ResourceTransactionScope{
				ID: config.FullName(),
			}
			var transaction lifecycle.PreparedResourceTransaction
			if remove {
				transaction, err = controller.prepareRemove(t.Context(), target, record, true, nil, scope)
			} else {
				transaction, err = controller.prepareDisable(t.Context(), target, record, true, nil, scope)
			}
			require.NoError(t, err)
			_, err = transaction.Apply(t.Context())
			require.NoError(t, err)
			require.False(t, controller.ActivationEnabled(config.FullName()))
			started.RetainPending()
			requireNoSecretSubmission(t, commands)
		})
	}
}

func bindSecretRestartTestWorkers(t *testing.T, controller *DynCfgJobController) *activationTestCommands {
	t.Helper()
	return bindHandoffTestWorkers(t, controller)
}

func seedSecretPausedTest(t *testing.T, controller *DynCfgJobController, graph *dyncfg.Graph, config confgroup.Config) {
	t.Helper()
	seedDynCfgJobGraphRecord(t, controller, graph, config, dyncfg.StatusDisabled)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
	}
	enabled, err := controller.Prepare(
		t.Context(),
		DynCfgJobRequest{
			Args: []string{controller.configID(config.Module(), config.Name()), "enable"},
		},
		nil,
		scope,
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	require.Nil(t, runtimeTestApply(t, enabled))
	plan, stopped, err := controller.PlanSecretDependentStop(config.FullName())
	require.NoError(t, err)
	prepared, err := plan.Transaction.Prepare(t.Context(), nil, scope, lifecycle.LongLivedPermit{})
	require.NoError(t, err)
	require.Nil(t, runtimeTestApply(t, prepared))
	didStop, err := stopped.Stopped()
	require.NoError(t, err)
	require.True(t, didStop)
	require.True(t, controller.ActivationEnabled(config.FullName()))
}

func requireNoSecretSubmission(t *testing.T, commands *activationTestCommands) {
	t.Helper()
	select {
	case call := <-commands.queue:
		if call.ack != nil {
			call.ack <- nil
		}
		t.Fatalf("unexpected secret continuation command %s", call.request.Route)
	case <-time.After(30 * time.Millisecond):
	}
}
