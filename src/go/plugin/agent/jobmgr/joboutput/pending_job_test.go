// SPDX-License-Identifier: GPL-3.0-or-later

package joboutput

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/stretchr/testify/require"
)

func (pji *pendingJobIndex) joined() bool {
	if pji == nil {
		return true
	}
	pji.mu.Lock()
	bound := pji.bound
	done := pji.done
	pji.mu.Unlock()
	if !bound {
		return true
	}
	select {
	case <-done:
		return true
	default:
		return false
	}
}

func TestPendingJobRetainsOnlyLatestDesiredUntilItsIdentityReleases(t *testing.T) {
	commands := &autoDetectionRetryTestCommands{}
	index := newPendingJobIndex()
	type planned struct {
		config confgroup.Config
		token  pendingJobToken
	}
	plannedJobs := make(chan planned, 2)
	require.NoError(t, index.bind(
		commands,
		func(config confgroup.Config, token pendingJobToken) (jobmgr.WorkPlan, error) {
			plannedJobs <- planned{config: config, token: token}
			return jobmgr.WorkPlan{}, nil
		},
		9,
		func(error) {},
	))

	first := autoDetectionRetryTestConfig("job")
	first["version"] = 1
	second := autoDetectionRetryTestConfig("job")
	second["version"] = 2
	firstRelease := make(chan struct{})
	secondRelease := make(chan struct{})
	index.retain(first, firstRelease, "")
	index.retain(second, secondRelease, "")

	close(firstRelease)
	commands.waitForSubmissions(t, 0)
	close(secondRelease)
	commands.waitForSubmissions(t, 1)

	var latest planned
	select {
	case latest = <-plannedJobs:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "latest pending job was not planned")
	}
	require.EqualValues(t, 2, latest.config["version"])
	require.True(t, index.isCurrent(second.FullName(), latest.token))
	index.settle(second.FullName(), latest.token)
	require.False(t, index.isCurrent(second.FullName(), latest.token))

	index.stopWorker()
	require.NoError(t, index.wait(context.Background()))
	require.True(t, index.joined())
}

func TestDiscoveredBusyRetainsPendingDesiredAndRetriesOnRelease(t *testing.T) {
	controller, _, _, _, _ := newDynCfgJobTestHarness(t)
	release := make(chan struct{})
	controller.factory.config.Attempts = busyPendingJobAuthority{release: release}
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindAutoDetectionRetries(commands, 9, func(error) {}))
	t.Cleanup(func() {
		controller.scheduler.StopAutoDetectionRetries()
		require.NoError(t, controller.scheduler.WaitAutoDetectionRetries(context.Background()))
	})

	config := autoDetectionRetryTestConfig("job")
	config.SetSourceType(confgroup.TypeUser)
	config.SetSource("user")
	config.SetProvider("user")
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: config,
			Status: dyncfg.StatusRunning,
		},
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
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	controller.scheduler.pending.mu.Lock()
	pending := controller.scheduler.pending.entries[config.FullName()]
	controller.scheduler.pending.mu.Unlock()
	require.NotNil(t, pending)
	close(release)
	commands.waitForSubmissions(t, 1)
	submitted, plans, waited := commands.snapshot()
	require.Len(t, submitted, 1)
	require.Len(t, plans, 1)
	require.False(t, waited)
	require.Equal(t, config.FullName(), submitted[0].LaneKey)
	require.NotNil(t, plans[0].Transaction)
	require.Equal(t, config.FullName(), plans[0].Transaction.ID)
}

func TestDiscoveredStaleStoreCandidateRetainsPendingDesired(t *testing.T) {
	controller, _, _, _, state := newDynCfgJobTestHarness(t)
	scopeState := &factoryTestAtomicScope{value: "initial"}
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
	commands := &autoDetectionRetryTestCommands{}
	require.NoError(t, controller.BindAutoDetectionRetries(commands, 9, func(error) {}))
	t.Cleanup(func() {
		controller.scheduler.StopAutoDetectionRetries()
		require.NoError(t, controller.scheduler.WaitAutoDetectionRetries(context.Background()))
	})

	config := autoDetectionRetryTestConfig("job")
	config["option_str"] = "${store:vault:test:key}"
	config.SetSourceType(confgroup.TypeUser)
	config.SetSource("user")
	config.SetProvider("user")
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: config,
			Status: dyncfg.StatusRunning,
		},
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
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())

	commands.waitForSubmissions(t, 1)
	submitted, plans, waited := commands.snapshot()
	require.Len(t, submitted, 1)
	require.Len(t, plans, 1)
	require.False(t, waited)
	require.Equal(t, config.FullName(), submitted[0].LaneKey)
	require.Equal(t, config.FullName(), plans[0].Transaction.ID)
}

func TestDiscoveredQuarantinedCandidateCommitsFailedWithoutPendingRetry(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	controller.factory.config.Attempts = quarantinedPendingJobAuthority{}

	config := autoDetectionRetryTestConfig("job")
	config.SetSourceType(confgroup.TypeUser)
	config.SetSource("user")
	config.SetProvider("user")
	permit, tasks := issueTestJobPermit(t, config.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: config.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 1,
		},
	}
	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: config,
			Status: dyncfg.StatusRunning,
		},
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

	record, exists := graph.Lookup(config.FullName())
	require.True(t, exists)
	require.Equal(t, dyncfg.StatusFailed.String(), record.Status)
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
	controller.scheduler.pending.mu.Lock()
	pending := controller.scheduler.pending.entries[config.FullName()]
	controller.scheduler.pending.mu.Unlock()
	require.Nil(t, pending)
}

func TestFreshDesiredConfigCancelsDifferentPendingReplacement(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	incumbent := autoDetectionRetryTestConfig("job")
	incumbent.SetSourceType(confgroup.TypeUser)
	incumbent.SetSource("user-a")
	incumbent.SetProvider("user")
	incumbent["version"] = 1
	replacement := autoDetectionRetryTestConfig("job")
	replacement.SetSourceType(confgroup.TypeUser)
	replacement.SetSource("user-b")
	replacement.SetProvider("user")
	replacement["version"] = 2
	seedDynCfgJobGraphRecord(t, graph, incumbent, dyncfg.StatusRunning)

	token := pendingJobToken{
		uid:         replacement.UID(),
		baselineUID: incumbent.UID(),
		version:     1,
	}
	controller.scheduler.pending.entries[incumbent.FullName()] = &pendingJob{
		config: replacement,
		token:  token,
		update: make(chan struct{}, 1),
	}
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         incumbent.FullName(),
		Generation: 1,
	}
	events := []string{}
	current := &transactionTestReadyResource{
		identity: currentIdentity,
		prefix:   "current",
		events:   &events,
	}
	permit, tasks := issueTestJobPermit(t, incumbent.FullName(), 2)
	scope := lifecycle.ResourceTransactionScope{
		ID:      incumbent.FullName(),
		Current: currentIdentity,
		Successor: lifecycle.ResourceIdentity{
			ID:         incumbent.FullName(),
			Generation: 2,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config: incumbent,
			Status: dyncfg.StatusRunning,
		},
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, owned := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Same(t, current, owned)
	require.False(t, controller.scheduler.pending.isCurrent(incumbent.FullName(), token))
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestPendingDiscoveredJobDoesNotReplaceNewerEqualPriorityWinner(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := &unexpectedPendingJobAuthority{}
	controller.factory.config.Attempts = attempts

	pending := autoDetectionRetryTestConfig("job")
	pending.SetSourceType(confgroup.TypeUser)
	pending.SetSource("user-a")
	pending.SetProvider("user")
	pending["version"] = 1
	winner := autoDetectionRetryTestConfig("job")
	winner.SetSourceType(confgroup.TypeUser)
	winner.SetSource("user-b")
	winner.SetProvider("user")
	winner["version"] = 2
	seedDynCfgJobGraphRecord(t, graph, winner, dyncfg.StatusFailed)

	token := pendingJobToken{
		uid:         pending.UID(),
		baselineUID: "previous-winner",
		version:     1,
	}
	controller.scheduler.pending.entries[pending.FullName()] = &pendingJob{
		config: pending,
		token:  token,
		update: make(chan struct{}, 1),
	}
	permit, tasks := issueTestJobPermit(t, pending.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: pending.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         pending.FullName(),
			Generation: 1,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config:  pending,
			Status:  dyncfg.StatusRunning,
			Restart: true,
			pending: token,
		},
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
	require.False(t, controller.scheduler.pending.isCurrent(pending.FullName(), token))
	require.Zero(t, attempts.calls.Load())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestAbsentPendingRetriesAgainstOriginalEqualPriorityWinner(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := &unexpectedPendingJobAuthority{}
	controller.factory.config.Attempts = attempts

	incumbent := autoDetectionRetryTestConfig("job")
	incumbent.SetSourceType(confgroup.TypeUser)
	incumbent.SetSource("user-a")
	incumbent.SetProvider("user")
	incumbent["version"] = 1
	replacement := autoDetectionRetryTestConfig("job")
	replacement.SetSourceType(confgroup.TypeUser)
	replacement.SetSource("user-b")
	replacement.SetProvider("user")
	replacement["version"] = 2
	seedDynCfgJobGraphRecord(t, graph, incumbent, dyncfg.StatusFailed)

	token := pendingJobToken{
		uid:           replacement.UID(),
		baselineUID:   incumbent.UID(),
		version:       1,
		requireAbsent: true,
	}
	controller.scheduler.pending.entries[replacement.FullName()] = &pendingJob{
		config: replacement,
		token:  token,
		update: make(chan struct{}, 1),
	}
	permit, tasks := issueTestJobPermit(t, replacement.FullName(), 1)
	scope := lifecycle.ResourceTransactionScope{
		ID: replacement.FullName(),
		Successor: lifecycle.ResourceIdentity{
			ID:         replacement.FullName(),
			Generation: 1,
		},
	}

	_, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config:  replacement,
			Status:  dyncfg.StatusRunning,
			Restart: true,
			pending: token,
		},
		nil,
		scope,
		permit,
	)
	require.ErrorContains(t, err, "stale pending job superseded")
	require.EqualValues(t, 1, attempts.calls.Load())
	require.False(t, controller.scheduler.pending.isCurrent(replacement.FullName(), token))
	require.NoError(t, permit.AbortUnused())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

func TestAbsentPendingDoesNotRestartRunningJob(t *testing.T) {
	controller, graph, _, _, _ := newDynCfgJobTestHarness(t)
	attempts := &unexpectedPendingJobAuthority{}
	controller.factory.config.Attempts = attempts

	config := autoDetectionRetryTestConfig("job")
	config.SetSourceType(confgroup.TypeUser)
	config.SetSource("user")
	config.SetProvider("user")
	seedDynCfgJobGraphRecord(t, graph, config, dyncfg.StatusRunning)

	token := pendingJobToken{
		uid:           config.UID(),
		baselineUID:   config.UID(),
		version:       1,
		requireAbsent: true,
	}
	controller.scheduler.pending.entries[config.FullName()] = &pendingJob{
		config: config,
		token:  token,
		update: make(chan struct{}, 1),
	}
	currentIdentity := lifecycle.ResourceIdentity{
		ID:         config.FullName(),
		Generation: 1,
	}
	current := &transactionTestReadyResource{identity: currentIdentity}
	permit, tasks := issueTestJobPermit(t, config.FullName(), 2)
	scope := lifecycle.ResourceTransactionScope{
		ID:      config.FullName(),
		Current: currentIdentity,
		Successor: lifecycle.ResourceIdentity{
			ID:         config.FullName(),
			Generation: 2,
		},
	}

	transaction, err := controller.prepareDiscovered(
		context.Background(),
		DiscoveredJobChange{
			Config:  config,
			Status:  dyncfg.StatusRunning,
			Restart: true,
			pending: token,
		},
		current,
		scope,
		permit,
	)
	require.NoError(t, err)
	applied, err := transaction.Apply(context.Background())
	require.NoError(t, err)
	_, disposition, owned := applied.Ownership()
	require.Equal(t, lifecycle.ResourceTransactionUnchanged, disposition)
	require.Same(t, current, owned)
	require.False(t, controller.scheduler.pending.isCurrent(config.FullName(), token))
	require.Zero(t, attempts.calls.Load())
	require.EqualValues(t, lifecycle.LongLivedCensus{}, tasks.LongLivedCensus())
}

type busyPendingJobAuthority struct {
	release <-chan struct{}
}

type quarantinedPendingJobAuthority struct{}

func (quarantinedPendingJobAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return nil, errors.New("test: quarantined pending job attempt started")
}

func (quarantinedPendingJobAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	return jobmgr.ErrProcessAttemptQuarantined
}

func (quarantinedPendingJobAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (quarantinedPendingJobAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}

func (bpja busyPendingJobAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	return nil, errors.New("test: unexpected pending job attempt start")
}

func (busyPendingJobAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	return jobmgr.ErrProcessAttemptBusy
}

func (busyPendingJobAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (bpja busyPendingJobAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return bpja.release, true
}

type unexpectedPendingJobAuthority struct {
	calls atomic.Int32
}

func (upja *unexpectedPendingJobAuthority) StartProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptPlan,
) (jobmgr.ProcessAttempt, error) {
	upja.calls.Add(1)
	return nil, errors.New("test: stale pending job started")
}

func (upja *unexpectedPendingJobAuthority) SupersedeProcessAttempt(
	context.Context,
	jobmgr.ProcessAttemptIdentity,
) error {
	upja.calls.Add(1)
	return errors.New("test: stale pending job superseded")
}

func (*unexpectedPendingJobAuthority) CutProcessAttempt(
	jobmgr.ProcessAttemptIdentity,
	error,
) bool {
	return false
}

func (*unexpectedPendingJobAuthority) ProcessAttemptReleased(
	jobmgr.ProcessAttemptIdentity,
) (<-chan struct{}, bool) {
	return nil, false
}
