// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/containment"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func TestSecretDependentStartCommitsFailedAndWaitsForBusyRuntimeRelease(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	delegate := controller.factory.config.Attempts.(*containment.Authority)
	release := make(chan struct{})
	controller.factory.config.Attempts = runtimeBusyPendingAuthority{
		delegate: delegate,
		release:  release,
	}
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(nil, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	controller.factory.config.Modules = controller.modules
	controller.configModules.config.Modules = controller.modules

	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindAutoDetectionRetries(commands, 9, func(error) {}))
	t.Cleanup(func() {
		controller.scheduler.StopAutoDetectionRetries()
		require.NoError(t, controller.scheduler.WaitAutoDetectionRetries(context.Background()))
	})

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)

	work, startState, err := controller.PlanSecretDependentStart(config.FullName())
	require.NoError(t, err)
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	transaction, err := work.Transaction.Prepare(
		context.Background(),
		nil,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, current)
	require.ErrorIs(t, startState.Err(), jobmgr.ErrProcessAttemptBusy)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	require.Eventually(t, func() bool {
		return delegate.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 1, state.collectorCleanup)

	controller.scheduler.pending.mu.Lock()
	pending := controller.scheduler.pending.entries[config.FullName()]
	controller.scheduler.pending.mu.Unlock()
	require.NotNil(t, pending)
	commands.waitForSubmissions(t, 0)
	close(release)
	commands.waitForSubmissions(t, 1)
}

func TestSecretDependentStartCommitsFailedWithoutRetryForQuarantinedRuntime(t *testing.T) {
	controller, graph, _, _, state := newDynCfgJobTestHarness(t)
	delegate := controller.factory.config.Attempts.(*containment.Authority)
	controller.factory.config.Attempts = runtimeQuarantinedTestAuthority{delegate: delegate}
	creator := controller.modules["module"]
	creator.Create = func() collectorapi.CollectorV1 {
		module := state.module(nil, false)
		charts := collectorapi.Charts{}
		module.ChartsFunc = func() *collectorapi.Charts { return &charts }
		return module
	}
	controller.modules["module"] = creator
	controller.factory.config.Modules = controller.modules
	controller.configModules.config.Modules = controller.modules

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)

	work, startState, err := controller.PlanSecretDependentStart(config.FullName())
	require.NoError(t, err)
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	transaction, err := work.Transaction.Prepare(
		context.Background(),
		nil,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, current := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Nil(t, current)
	require.ErrorIs(t, startState.Err(), jobmgr.ErrProcessAttemptQuarantined)
	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	require.Eventually(t, func() bool {
		return delegate.Census() == (containment.Census{})
	}, time.Second, time.Millisecond)
	require.EqualValues(t, 1, state.collectorCleanup)

	controller.scheduler.pending.mu.Lock()
	pending := controller.scheduler.pending.entries[config.FullName()]
	controller.scheduler.pending.mu.Unlock()
	require.Nil(t, pending)
}

func TestSecretDependentStopCommitsFailedBeforeRestart(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)

	work, stopState, err := controller.PlanSecretDependentStop(config.FullName())
	require.NoError(t, err)
	var events []string
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Current: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	current := &transactionTestReadyResource{
		identity: scope.Current,
		prefix:   "current",
		events:   &events,
	}
	transaction, err := work.Transaction.Prepare(
		context.Background(),
		current,
		scope,
		lifecycle.LongLivedPermit{},
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, active := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionRemoved, disposition)
	require.Nil(t, active)
	stopped, err := stopState.Stopped()
	require.NoError(t, err)
	require.True(t, stopped)

	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
}

func TestSecretDependentStartRetainsAbsentPendingAfterExternalDeadline(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	release := make(chan struct{})
	controller.factory.config.Attempts = busyPendingJobAuthority{release: release}
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindAutoDetectionRetries(commands, 9, func(error) {}))
	t.Cleanup(func() {
		controller.scheduler.StopAutoDetectionRetries()
		require.NoError(t, controller.scheduler.WaitAutoDetectionRetries(context.Background()))
	})

	config := factoryTestConfig(false)
	config.SetSourceType(confgroup.TypeDyncfg)
	config.SetSource("user=test")
	config.SetProvider(confgroup.TypeDyncfg)
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusFailed)

	_, startState, err := controller.PlanSecretDependentStart(config.FullName())
	require.NoError(t, err)
	startState.RetainPending()

	controller.scheduler.pending.mu.Lock()
	pending := controller.scheduler.pending.entries[config.FullName()]
	controller.scheduler.pending.mu.Unlock()
	require.NotNil(t, pending)
	require.True(t, pending.token.requireAbsent)

	close(release)
	commands.waitForSubmissions(t, 1)
}

type runtimeBusyPendingAuthority struct {
	delegate jobmgr.ProcessAttemptAuthority
	release  <-chan struct{}
}

type runtimeQuarantinedTestAuthority struct {
	delegate jobmgr.ProcessAttemptAuthority
}

func (rqta runtimeQuarantinedTestAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		return nil, jobmgr.ErrProcessAttemptQuarantined
	}
	return rqta.delegate.StartProcessAttempt(ctx, plan)
}

func (rqta runtimeQuarantinedTestAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	return rqta.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (rqta runtimeQuarantinedTestAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return rqta.delegate.CutProcessAttempt(identity, cause)
}

func (rqta runtimeQuarantinedTestAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return rqta.delegate.ProcessAttemptReleased(identity)
}

func (rbpa runtimeBusyPendingAuthority) StartProcessAttempt(
	ctx context.Context,
	plan jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	if plan.Identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		return nil, jobmgr.ErrProcessAttemptBusy
	}
	return rbpa.delegate.StartProcessAttempt(ctx, plan)
}

func (rbpa runtimeBusyPendingAuthority) SupersedeProcessAttempt(
	ctx context.Context,
	identity jobmgr.ProcessAttemptIdentity,
) error {
	if identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		return jobmgr.ErrProcessAttemptBusy
	}
	return rbpa.delegate.SupersedeProcessAttempt(ctx, identity)
}

func (rbpa runtimeBusyPendingAuthority) CutProcessAttempt(
	identity jobmgr.ProcessAttemptIdentity,
	cause error,
) bool {
	return rbpa.delegate.CutProcessAttempt(identity, cause)
}

func (rbpa runtimeBusyPendingAuthority) ProcessAttemptReleased(
	identity jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	if identity.Namespace == jobmgr.ProcessAttemptJobRuntime {
		return rbpa.release, true
	}
	return rbpa.delegate.ProcessAttemptReleased(identity)
}
