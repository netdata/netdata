// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func runtimeTestNotifications(t *testing.T, controller *DynCfgJobController) *autoDetectionRetryTestCommands {
	t.Helper()
	commands := &autoDetectionRetryTestCommands{}
	controller.bindRuntimeFailures(commands, 9, func(err error) { t.Errorf("runtime dispatch: %v", err) })
	return commands
}

func runtimeTestNotificationPlan(t *testing.T, commands *autoDetectionRetryTestCommands, route string) jobmgr.WorkPlan {
	t.Helper()
	var result jobmgr.WorkPlan
	require.Eventually(t, func() bool {
		requests, plans, _ := commands.snapshot()
		for i, request := range requests {
			if request.Route == route {
				result = plans[i]
				return true
			}
		}
		return false
	}, time.Second, time.Millisecond, "runtime callback %s was not dispatched", route)
	return result
}

func runtimeTestApplyNotification(t *testing.T, plan jobmgr.WorkPlan, current lifecycle.ReadyResource) lifecycle.ReadyResource {
	t.Helper()
	scope := lifecycle.ResourceTransactionScope{ID: plan.Transaction.ID}
	if current != nil {
		scope.Current = current.Identity()
	}
	prepared, err := plan.Transaction.Prepare(t.Context(), current, scope, lifecycle.LongLivedPermit{})
	require.NoError(t, err)
	return runtimeTestApply(t, prepared)
}

func runtimeTestStartAndSettle(t *testing.T, controller *DynCfgJobController, cfg confgroup.Config, current lifecycle.ReadyResource, generation uint64) lifecycle.ReadyResource {
	t.Helper()
	commands := runtimeTestNotifications(t, controller)
	current = runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, current, generation))
	require.NotNil(t, current)
	plan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
	return runtimeTestApplyNotification(t, plan, current)
}

func TestRuntimeSettlementFailureWinsRegardlessOfNotificationOrder(t *testing.T) {
	for _, first := range []string{"runtime-ready", "runtime-failure"} {
		t.Run(first, func(t *testing.T) {
			controller, graph, _, output, _ := newDynCfgJobTestHarness(t)
			commands := runtimeTestNotifications(t, controller)
			release := make(chan struct{})
			configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
				return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error {
					ready()
					<-release
					return errors.New("failed before publication")
				}}
			})
			cfg := factoryTestConfig(false).Set("autodetection_retry", 7)
			current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
			require.NotNil(t, current)
			readyPlan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
			close(release)
			failurePlan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-failure")
			plans := []jobmgr.WorkPlan{readyPlan, failurePlan}
			if first == "runtime-failure" {
				plans = []jobmgr.WorkPlan{failurePlan, readyPlan}
			}
			for _, plan := range plans {
				current = runtimeTestApplyNotification(t, plan, current)
			}
			require.Nil(t, current)
			record, exists := graph.Lookup(cfg.FullName())
			require.True(t, exists)
			require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
			require.False(t, runtimeTestHasRetry(controller, cfg.FullName()), "post-ready failure retains manual restart policy")
			require.NotContains(t, output.String(), "status running")
			requireFactoryAttemptsIdle(t, controller.factory)
		})
	}
}

func TestRuntimeSettlementRejectsStaleReadinessAfterReplacement(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	commands := runtimeTestNotifications(t, controller)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { ready(); <-ctx.Done(); return ctx.Err() }}
	})
	cfg := factoryTestConfig(false)
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
	stale := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
	stopRuntimeTestResource(t, current)
	requireFactoryAttemptsIdle(t, controller.factory)
	next := runtimeTestStartAndSettle(t, controller, cfg, nil, 2)
	require.NotNil(t, next)
	require.Same(t, next, runtimeTestApplyNotification(t, stale, next))
	record, _ := graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.Equal(t, uint64(4), next.Identity().Generation)
	stopRuntimeTestResource(t, next)
	requireFactoryAttemptsIdle(t, controller.factory)
}

func TestRuntimeSettlementOnlyCurrentReadinessPublishes(t *testing.T) {
	controller, graph, _, output, _ := newDynCfgJobTestHarness(t)
	commands := runtimeTestNotifications(t, controller)
	readyGate := make(chan struct{})
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{run: func(ctx context.Context, ready func()) error { <-readyGate; ready(); <-ctx.Done(); return ctx.Err() }}
	})
	cfg := factoryTestConfig(false)
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
	record, _ := graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Empty(t, controller.scheduler.jobs)
	controller.factory.notifyRunReady(current.Identity())
	pending := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
	require.Same(t, current, runtimeTestApplyNotification(t, pending, current), "premature notification must remain a clean no-op")
	close(readyGate)
	require.NoError(t, current.(*JobGeneration).AwaitReady(t.Context()))
	plan := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-ready")
	require.Same(t, current, runtimeTestApplyNotification(t, plan, current))
	record, _ = graph.Lookup(cfg.FullName())
	require.Equal(t, dyncfg.StatusRunning.String(), record.Status)
	require.Len(t, controller.scheduler.jobs, 1)
	controller.factory.notifyRunFailure(current.Identity(), nil)
	duplicate := runtimeTestNotificationPlan(t, commands, "internal/jobs/runtime-failure")
	require.Same(t, current, runtimeTestApplyNotification(t, duplicate, current), "notification alone cannot fail a healthy run")
	require.NotContains(t, output.String(), "status failed")
	stopRuntimeTestResource(t, current)
	requireFactoryAttemptsIdle(t, controller.factory)
}

func TestDiscoveredReplayPreservesStartingStatus(t *testing.T) {
	controller, graph, supervisor, output, _ := newDynCfgJobTestHarness(t)
	configureRuntimeTestCollector(controller, func() *runtimeTestCollector {
		return &runtimeTestCollector{run: func(ctx context.Context, _ func()) error {
			<-ctx.Done()
			return ctx.Err()
		}}
	})
	cfg := factoryTestConfig(false)
	current := runtimeTestApply(t, prepareRuntimeTestChange(t, controller, cfg, nil, 1))
	require.NotNil(t, current)
	t.Cleanup(func() {
		stopRuntimeTestResource(t, current)
		requireFactoryAttemptsIdle(t, controller.factory)
	})
	scope := lifecycle.ResourceTransactionScope{ID: cfg.FullName(), Current: current.Identity()}
	plan, err := lifecycle.NewResourceTransactionTaskPlan(
		lifecycle.SourceJobManager, time.Time{}, lifecycle.TransactionTaskPhases, current, scope,
		func(ctx context.Context, active lifecycle.ReadyResource, taskScope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
			return controller.prepareDiscovered(ctx, DiscoveredJobChange{Config: cfg, Status: dyncfg.StatusRunning}, active, taskScope, permit)
		})
	require.NoError(t, err)
	disposition, replayed := applyAndEncodeDynCfgJobTestTask(t, supervisor, plan, scope, "replay")
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Same(t, current, replayed)
	record, exists := graph.Lookup(cfg.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusAccepted.String(), record.Status)
	require.Empty(t, controller.scheduler.jobs)
	require.NotContains(t, output.String(), "create running", "replayed discovery must not announce readiness")
	require.NotContains(t, output.String(), "status running")
}
