// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/joboutput"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/netdata/netdata/go/plugins/plugin/framework/ownedjobs"
	"github.com/stretchr/testify/require"
)

type ownerTestService struct{ registries chan ownedjobs.Registry }

func (s ownerTestService) Run(ctx context.Context, registry ownedjobs.Registry) {
	s.registries <- registry
	<-ctx.Done()
}

func newOwnedRun(
	t *testing.T,
	create func() collectorapi.CollectorV1,
	ownerOnly bool,
	discovered ...confgroup.Config,
) (*runGeneration, ownedjobs.Registry, *processSynchronizedBuffer) {
	t.Helper()
	return newOwnedRunWithCreator(t, collectorapi.Creator{
		Create: create,
		Config: func() any { return &collectorapi.MockConfiguration{} },
	}, ownerOnly, discovered...)
}

func newOwnedRunWithCreator(
	t *testing.T,
	creator collectorapi.Creator,
	ownerOnly bool,
	discovered ...confgroup.Config,
) (*runGeneration, ownedjobs.Registry, *processSynchronizedBuffer) {
	t.Helper()
	output := newProcessSynchronizedBuffer()
	frames, err := lifecycle.NewFrameOwner(output)
	require.NoError(t, err)
	services := testRunJobServices(t)
	services.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	service := ownerTestService{
		registries: make(chan ownedjobs.Registry, 1),
	}
	services.OwnedJobServices = []ownedjobs.Service{service}
	services.OwnerOnlyModules = map[string]bool{"module": ownerOnly}
	discovery := testRunDiscoveryServices(t, discovered...)
	if ownerOnly {
		discovery.ExcludedModules = []string{"module"}
	}
	uids := lifecycle.NewUIDLedger()
	generation, err := newTestRunGeneration(t, runGenerationConfig{
		Generation:      1,
		ShutdownTimeout: 10 * time.Second,
		UIDs:            uids,
		Frames:          frames,
		Modules: collectorapi.Registry{
			"module": creator,
		},
		Jobs:      services,
		Discovery: discovery,
	})
	require.NoError(t, err)
	require.NoError(t, generation.start(context.Background()))
	t.Cleanup(func() {
		generation.Stop()
		require.NoError(t, generation.Wait(context.Background()))
		closeRunTestUIDs(t, uids)
	})
	select {
	case registry := <-service.registries:
		return generation, registry, output
	case <-time.After(time.Second):
		t.Fatal("owner service did not bind")
		return nil, nil, nil
	}
}

func ownedTestCollector() collectorapi.CollectorV1 {
	return &collectorapi.MockCollectorV1{
		ChartsFunc: func() *collectorapi.Charts {
			return &collectorapi.Charts{&collectorapi.Chart{
				ID:    "chart",
				Title: "chart",
				Units: "value",
				Dims: collectorapi.Dims{&collectorapi.Dim{
					ID: "value",
				}},
			}}
		},
		CollectFunc: func(context.Context) map[string]int64 { return map[string]int64{"value": 1} },
	}
}
func ownedTestChildren(count int) []ownedjobs.Child {
	configs := fullCapacityDiscoveredJobs(count)
	children := make([]ownedjobs.Child, count)
	for index, config := range configs {
		children[index] = ownedjobs.Child{
			Key:     fmt.Sprintf("child-%04d", index),
			Config:  config,
			Enabled: true,
		}
	}
	return children
}
func applyOwned(t *testing.T, owner ownedjobs.Owner, revision uint64, children []ownedjobs.Child) {
	t.Helper()
	snapshot, err := owner.Prepare(t.Context(), revision, children)
	require.NoError(t, err)
	require.NoError(t, snapshot.Apply(t.Context()))
}
func waitOwnedRunning(t *testing.T, owner ownedjobs.Owner, count int) []ownedjobs.State {
	t.Helper()
	var states []ownedjobs.State
	require.Eventually(t, func() bool {
		states = owner.Status()
		if len(states) != count {
			return false
		}
		for _, state := range states {
			if state.ConfigStatus != "running" || state.RuntimeStatus != "active" || state.Desired != state.Applied {
				return false
			}
		}
		return true
	}, 10*time.Second, time.Millisecond)
	return states
}

func TestOwnedJobsProductionCompositionDifferentialSmoke(t *testing.T) {
	var creates atomic.Int64
	generation, registry, output := newOwnedRun(
		t,
		func() collectorapi.CollectorV1 { creates.Add(1); return ownedTestCollector() },
		true,
	)
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(2)
	applyOwned(t, owner, 1, children)
	first := waitOwnedRunning(t, owner, 2)
	require.NotContains(t, output.String(), "CONFIG go.d:collector:module create accepted template")
	children[0].Config.Set("option_str", "changed")
	applyOwned(t, owner, 2, children)
	second := waitOwnedRunning(t, owner, 2)
	require.NotEqual(t, first[0].Generation, second[0].Generation)
	require.Equal(t, first[1].Generation, second[1].Generation)
	require.Equal(t, first[1].Desired, second[1].Desired)
	require.EqualValues(t, 3, creates.Load())
	// Whole-parent takeover changes provenance without replacing either child.
	for _, child := range children {
		child.Config.SetSourceType(confgroup.TypeUser).SetSource("file=site")
	}
	applyOwned(t, owner, 3, children)
	third := waitOwnedRunning(t, owner, 2)
	require.Equal(t, second[0].Generation, third[0].Generation)
	require.Equal(t, second[1].Generation, third[1].Generation)
	require.Equal(t, "user", third[0].SourceType)
	require.NoError(t, generation.run.DirtyCause())
	// Children advertise no independent mutation commands.
	for _, line := range strings.Split(output.String(), "\n") {
		if strings.HasPrefix(line, "CONFIG go.d:collector:module:job-") && strings.Contains(line, " create ") {
			require.NotContains(t, line, " update")
			require.NotContains(t, line, " enable")
			require.NotContains(t, line, " restart")
		}
	}
}

func TestOwnedJobsValidateWholeSnapshotAndFenceRetiredLifetime(t *testing.T) {
	_, registry, _ := newOwnedRun(t, ownedTestCollector, true)
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(2)
	applyOwned(t, owner, 1, children)
	before := waitOwnedRunning(t, owner, 2)
	cases := map[string][]ownedjobs.Child{
		"duplicate key":             {children[0], children[0]},
		"duplicate native identity": {children[0], {Key: "other", Config: children[0].Config, Enabled: true}},
		"invalid identity": {{Key: "invalid", Config: confgroup.Config{
			"module": "module",
			"name":   "bad:name",
		}.SetSourceType(confgroup.TypeUser), Enabled: true}},
		"untrusted source": {{Key: "untrusted", Config: confgroup.Config{
			"module": "module",
			"name":   "new",
		}.SetSourceType(confgroup.TypeDiscovered), Enabled: true}},
		"unknown module": {{Key: "unknown", Config: confgroup.Config{
			"module": "missing",
			"name":   "new",
		}.SetSourceType(confgroup.TypeUser), Enabled: true}},
	}
	for name, invalid := range cases {
		t.Run(name, func(t *testing.T) {
			_, err := owner.Prepare(t.Context(), 2, invalid)
			require.Error(t, err)
			require.Equal(t, before, owner.Status())
		})
	}
	other, err := registry.Bind(t.Context(), "other")
	require.NoError(t, err)
	_, err = other.Prepare(t.Context(), 1, children)
	require.ErrorContains(t, err, "another owner")
	stale, err := owner.Prepare(t.Context(), 2, children)
	require.NoError(t, err)
	require.NoError(t, owner.Retire(t.Context()))
	require.ErrorIs(t, stale.Apply(t.Context()), ownedjobs.ErrStale)
	_, err = owner.Prepare(t.Context(), 3, children)
	require.ErrorIs(t, err, ownedjobs.ErrStale)
	replacement, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	applyOwned(t, replacement, 1, children)
	after := waitOwnedRunning(t, replacement, 2)
	require.NotEqual(t, before[0].Desired, after[0].Desired)
	require.NotEqual(t, before[0].Generation, after[0].Generation)
}

func TestOwnedJobsOneBlockedCleanupPreservesSiblingAndRevokesWholeEdit(t *testing.T) {
	entered := make(chan struct{}, 1)
	release := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	generation, registry, _ := newOwnedRun(t, func() collectorapi.CollectorV1 {
		collector := ownedTestCollector().(*collectorapi.MockCollectorV1)
		collector.CleanupFunc = func(context.Context) {
			if collector.Config.OptionStr == "blocked" {
				select {
				case entered <- struct{}{}:
				default:
				}
				<-release
			}
		}
		return collector
	}, true)
	// Release precedes helper cleanup even if an assertion fails.
	t.Cleanup(func() { releaseOnce.Do(func() { close(release) }) })
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(3)
	children[0].Config.Set("option_str", "blocked")
	applyOwned(t, owner, 1, children)
	first := waitOwnedRunning(t, owner, 3)
	children[0].Config.Set("option_str", "replacement")
	children[1].Enabled = false
	applyOwned(t, owner, 2, children)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("native predecessor cleanup did not start")
	}
	require.Eventually(t, func() bool {
		states := owner.Status()
		return states[1].ConfigStatus == "disabled" && states[1].Applied == states[1].Desired
	}, time.Second, time.Millisecond)
	during := owner.Status()
	require.Equal(t, first[2].Desired, during[2].Desired)
	require.Equal(t, first[2].Generation, during[2].Generation)
	require.Equal(t, "active", during[2].RuntimeStatus)
	require.Equal(t, "pending", during[0].RuntimeStatus, "successor waits for physical predecessor release")
	require.NoError(t, generation.scheduler.Tick(t.Context(), 1))
	releaseOnce.Do(func() { close(release) })
	require.Eventually(t, func() bool {
		states := owner.Status()
		return states[0].ConfigStatus == "running" && states[0].RuntimeStatus == "active"
	}, time.Second, time.Millisecond)
	require.NoError(t, generation.run.DirtyCause())
}

func TestOwnedJobsMaximumSnapshotUsesIndependentNativeJobs(t *testing.T) {
	generation, registry, _ := newOwnedRun(t, ownedTestCollector, true)
	owner, err := registry.Bind(t.Context(), "maximum-site")
	require.NoError(t, err)
	children := ownedTestChildren(1000)
	applyOwned(t, owner, 1, children)
	before := waitOwnedRunning(t, owner, 1000)
	require.Len(t, generation.vnodes.graph.IDs(), 1000)
	children[500].Config.Set("option_str", "changed")
	applyOwned(t, owner, 2, children)
	after := waitOwnedRunning(t, owner, 1000)
	for index := range before {
		if index != 500 {
			require.Equal(t, before[index].Generation, after[index].Generation)
			require.Equal(t, before[index].Desired, after[index].Desired)
		}
	}
	require.NotEqual(t, before[500].Generation, after[500].Generation)
	require.NoError(t, owner.Retire(t.Context()))
	require.Eventually(
		t,
		func() bool { return len(generation.vnodes.graph.IDs()) == 0 },
		10*time.Second,
		time.Millisecond,
	)
	require.NoError(t, generation.run.DirtyCause())
}

func TestOwnedJobsNativeDependencyRestartPreservesDesiredIdentity(t *testing.T) {
	generation, registry, _ := newOwnedRun(t, ownedTestCollector, true)
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	applyOwned(t, owner, 1, ownedTestChildren(1))
	before := waitOwnedRunning(t, owner, 1)[0]
	stop, receipt, err := generation.dyncfg.PlanSecretDependentStop(before.JobID)
	require.NoError(t, err)
	require.NoError(t, generation.kernel.SubmitPreparedAndWait(t.Context(), jobmgr.Request{
		UID:     "owned-dependency-stop",
		LaneKey: before.JobID,
		Source:  lifecycle.SourceJobManager,
		Route:   "test/dependency",
	}, stop))
	stopped, err := receipt.Stopped()
	require.NoError(t, err)
	require.True(t, stopped)
	start, _, err := generation.dyncfg.PlanSecretDependentStart(before.JobID)
	require.NoError(t, err)
	require.NoError(t, generation.kernel.SubmitPreparedAndWait(t.Context(), jobmgr.Request{
		UID:     "owned-dependency-start",
		LaneKey: before.JobID,
		Source:  lifecycle.SourceJobManager,
		Route:   "test/dependency",
	}, start))
	after := waitOwnedRunning(t, owner, 1)[0]
	require.Equal(t, before.Desired, after.Desired)
	require.Greater(t, after.Generation, before.Generation)
	for range 20 {
		require.Equal(t, after.Generation, owner.Status()[0].Generation)
	}
	require.NoError(t, generation.run.DirtyCause())
}

func TestOwnedJobsConcurrentRetireAndApplyHaveOneLifetimeCut(t *testing.T) {
	generation, registry, _ := newOwnedRun(t, ownedTestCollector, true)
	for index := range 20 {
		owner, err := registry.Bind(t.Context(), fmt.Sprintf("site-%d", index))
		require.NoError(t, err)
		children := ownedTestChildren(1)
		children[0].Enabled = false
		children[0].Config.SetName(fmt.Sprintf("job-%d", index))
		applyOwned(t, owner, 1, children)
		snapshot, err := owner.Prepare(t.Context(), 2, children)
		require.NoError(t, err)
		start := make(chan struct{})
		applied := make(chan error, 1)
		retired := make(chan error, 1)
		go func() { <-start; applied <- snapshot.Apply(t.Context()) }()
		go func() { <-start; retired <- owner.Retire(t.Context()) }()
		close(start)
		applyErr, retireErr := <-applied, <-retired
		if applyErr != nil {
			require.ErrorIs(t, applyErr, ownedjobs.ErrStale)
		}
		if retireErr != nil {
			require.ErrorIs(t, retireErr, ownedjobs.ErrStale)
			require.NoError(t, owner.Retire(t.Context()))
		}
		require.Empty(t, owner.Status())
		_, err = owner.Prepare(t.Context(), 100, children)
		require.ErrorIs(t, err, ownedjobs.ErrStale)
	}
	require.NoError(t, generation.run.DirtyCause())
}

func TestOwnedJobsPublicCompositionResolvesTrustedParentSecrets(t *testing.T) {
	reader, writer := io.Pipe()
	output := newProcessSynchronizedBuffer()
	config := testProductionProcessConfig(reader, output)
	values := make(chan string, 4)
	config.Modules = collectorapi.Registry{
		"module": {Create: func() collectorapi.CollectorV1 {
			collector := ownedTestCollector().(*collectorapi.MockCollectorV1)
			collector.InitFunc = func(context.Context) error { values <- collector.Config.OptionStr; return nil }
			return collector
		}, Config: func() any { return &collectorapi.MockConfiguration{} }},
	}
	config.Defaults = confgroup.Registry{
		"module": {UpdateEvery: 1},
	}
	config.OwnerOnlyModules = []string{"module"}
	service := ownerTestService{
		registries: make(chan ownedjobs.Registry, 1),
	}
	config.OwnedJobServices = []ownedjobs.Service{service}
	resolver, err := secretresolver.NewAtomicResolver(
		map[string]secretresolver.AtomicProvider{
			"env": secretresolver.AtomicProviderFunc(
				func(context.Context, string) ([]byte, error) { return []byte("synthetic-secret-value"), nil },
			),
		},
	)
	require.NoError(t, err)
	config.Secrets.Providers.Resolver = resolver
	process, err := NewProcess(config)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- process.Run(ctx) }()
	t.Cleanup(func() {
		stopCtx, stop := context.WithTimeout(context.Background(), 3*time.Second)
		defer stop()
		require.NoError(t, process.Terminate(stopCtx))
		cancel()
		require.NoError(t, writer.Close())
		select {
		case err := <-done:
			require.NoError(t, err)
		case <-time.After(3 * time.Second):
			t.Fatal("owned process shutdown timed out")
		}
	})
	var registry ownedjobs.Registry
	select {
	case registry = <-service.registries:
	case <-time.After(time.Second):
		t.Fatal("public owner service did not bind")
	}
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(1)
	children[0].Config.Set("option_str", "${env:SYNTHETIC_OWNED}")
	applyOwned(t, owner, 1, children)
	select {
	case value := <-values:
		require.Equal(t, "synthetic-secret-value", value)
	case <-time.After(time.Second):
		t.Fatal("owned collector did not resolve its secret")
	}
	waitOwnedRunning(t, owner, 1)
	require.NotContains(t, fmt.Sprintf("%+v", owner.Status()), "synthetic-secret-value")
}

func TestOwnedJobsEditCancelsStartupBeforeBlockedChildLaneDrains(t *testing.T) {
	initEntered := make(chan struct{}, 1)
	initCanceled := make(chan struct{}, 1)
	generation, registry, _ := newOwnedRun(t, func() collectorapi.CollectorV1 {
		collector := ownedTestCollector().(*collectorapi.MockCollectorV1)
		collector.InitFunc = func(ctx context.Context) error {
			if collector.Config.OptionStr == "slow-start" {
				initEntered <- struct{}{}
				<-ctx.Done()
				initCanceled <- struct{}{}
				return ctx.Err()
			}
			return nil
		}
		return collector
	}, true)
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(2)
	children[0].Config.Set("option_str", "slow-start")
	applyOwned(t, owner, 1, children)
	select {
	case <-initEntered:
	case <-time.After(time.Second):
		t.Fatal("native Init did not start")
	}
	require.Eventually(
		t,
		func() bool { return owner.Status()[1].ConfigStatus == "running" },
		time.Second,
		time.Millisecond,
	)
	laneEntered := make(chan struct{})
	laneRelease := make(chan struct{})
	var releaseOnce sync.Once
	t.Cleanup(func() { releaseOnce.Do(func() { close(laneRelease) }) })
	id := children[0].Config.FullName()
	plan := jobmgr.WorkPlan{
		Claims:              []string{joboutput.DynCfgJobGraphClaim},
		NoResponse:          true,
		YieldClaimOnPrepare: joboutput.DynCfgJobGraphClaim,
		Transaction: &jobmgr.ResourceTransactionPlan{
			ID: id,
			Prepare: func(ctx context.Context, current lifecycle.ReadyResource, scope lifecycle.ResourceTransactionScope, permit lifecycle.LongLivedPermit) (lifecycle.PreparedResourceTransaction, error) {
				workErr, claimErr := jobmgr.RunWithoutClaims(ctx, func(ctx context.Context) error {
					close(laneEntered)
					select {
					case <-laneRelease:
						return nil
					case <-ctx.Done():
						return ctx.Err()
					}
				})
				if err := errors.Join(workErr, claimErr); err != nil {
					return nil, err
				}
				result, err := lifecycle.NewSealedResult(204, "application/json", nil)
				if err != nil {
					return nil, err
				}
				return joboutput.PrepareNoopResourceTransaction(
					scope,
					current,
					permit,
					result,
					func() error { return nil },
					nil,
				)
			},
		},
	}
	require.NoError(
		t,
		generation.kernel.SubmitPrepared(
			t.Context(),
			jobmgr.Request{
				UID:     "owned-held-startup-lane",
				LaneKey: id,
				Source:  lifecycle.SourceJobManager,
				Route:   "test/held-lane",
			},
			plan,
		),
	)
	select {
	case <-laneEntered:
	case <-time.After(time.Second):
		t.Fatal("child lane did not yield its graph claim")
	}
	children[0].Enabled = false
	children[1].Enabled = false
	applyOwned(t, owner, 2, children)
	select {
	case <-initCanceled:
	case <-time.After(time.Second):
		t.Error("owner edit left old native Init authorized behind a blocked child lane")
	}
	require.Eventually(
		t,
		func() bool { return owner.Status()[1].ConfigStatus == "disabled" },
		time.Second,
		time.Millisecond,
	)
	releaseOnce.Do(func() { close(laneRelease) })
	require.Eventually(
		t,
		func() bool { return owner.Status()[0].ConfigStatus == "disabled" },
		time.Second,
		time.Millisecond,
	)
	require.NoError(t, generation.run.DirtyCause())
}

// Source withdrawals carry no module metadata. Exercise the production inventory
// boundary against native jobs so rejected standalone observations cannot leave
// an empty native header after the parent's reservation has been reclaimed.
func TestOwnedJobsSourceWithdrawalAfterRemovalDoesNotReserveIdentity(t *testing.T) {
	generation, registry, _ := newOwnedRun(t, ownedTestCollector, true)
	inventory, err := generation.newInventoryIndex()
	require.NoError(t, err)
	children := ownedTestChildren(1)
	proposal, err := children[0].Config.Clone()
	require.NoError(t, err)
	proposal.SetSourceType(confgroup.TypeUser).SetProvider("test").SetSource("file=standalone")
	require.NoError(t, inventory.Apply(t.Context(), []*confgroup.Group{{Source: "file=standalone", Configs: []confgroup.Config{proposal}}}))
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	applyOwned(t, owner, 1, children)
	waitOwnedRunning(t, owner, 1)
	applyOwned(t, owner, 2, nil)
	replacementOwner, err := registry.Bind(t.Context(), "replacement-site")
	require.NoError(t, err)
	require.Eventually(t, func() bool {
		_, err := replacementOwner.Prepare(t.Context(), 1, children)
		return err == nil
	}, time.Second, time.Millisecond)
	// Apply waits for the complete native source withdrawal, not just input
	// enqueue or an unrelated sibling's observation.
	require.NoError(t, inventory.Apply(t.Context(), []*confgroup.Group{{Source: "file=standalone"}}))
	replacement, err := replacementOwner.Prepare(t.Context(), 1, children)
	require.NoError(t, err)
	require.NoError(t, replacement.Apply(t.Context()))
	waitOwnedRunning(t, replacementOwner, 1)
	require.NoError(t, generation.run.DirtyCause())
}

func TestOwnedJobsCompositionFreezesExcludedModules(t *testing.T) {
	config := testProductionProcessConfig(strings.NewReader(""), io.Discard)
	config.OwnerOnlyModules = []string{"test"}
	process, err := NewProcess(config)
	require.NoError(t, err)
	config.OwnerOnlyModules[0] = "changed"
	require.Equal(t, []string{"test"}, process.core.config.Discovery.ExcludedModules)
}

func TestOwnedJobsDeclaredDependencyWaitsThenStartsWithoutResettingSibling(t *testing.T) {
	var ready atomic.Bool
	var dependentInits atomic.Int64
	creator := collectorapi.Creator{
		Config: func() any { return &collectorapi.MockConfiguration{} },
		Create: func() collectorapi.CollectorV1 {
			collector := ownedTestCollector().(*collectorapi.MockCollectorV1)
			collector.InitFunc = func(context.Context) error {
				if collector.Config.OptionStr == "dependent" {
					dependentInits.Add(1)
				}
				return nil
			}
			return collector
		},
		ActivationDependencies: func(config map[string]any) ([]collectorapi.ActivationDependency, error) {
			if config["option_str"] != "dependent" {
				return nil, nil
			}
			return []collectorapi.ActivationDependency{{Kind: "owned-runtime", Name: "browser", Ready: func() error {
				if ready.Load() {
					return nil
				}
				return errors.New("runtime pending")
			}}}, nil
		},
	}
	generation, registry, _ := newOwnedRunWithCreator(t, creator, true)
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(2)
	children[0].Config.Set("option_str", "dependent")
	applyOwned(t, owner, 1, children)
	require.Eventually(t, func() bool {
		states := owner.Status()
		return states[0].ConfigStatus == "accepted" && states[0].Desired == states[0].Applied &&
			states[1].RuntimeStatus == "active"
	}, time.Second, time.Millisecond)
	pending := owner.Status()
	require.Zero(t, dependentInits.Load(), "unavailable declared dependency must prevent collector Init")
	ready.Store(true)
	generation.dyncfg.NotifyDependencyChanged("owned-runtime", "browser")
	after := waitOwnedRunning(t, owner, 2)
	require.EqualValues(t, 1, dependentInits.Load())
	require.Equal(t, pending[0].Desired, after[0].Desired)
	require.Equal(t, pending[1].Desired, after[1].Desired)
	require.Equal(t, pending[1].Generation, after[1].Generation)
	require.NoError(t, generation.run.DirtyCause())
}

func TestOwnedJobsParentEditSupersedesYieldedDependencyAdmission(t *testing.T) {
	entered, release := make(chan struct{}, 1), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	var blockNext atomic.Bool
	var oldInits, newInits atomic.Int64
	creator := collectorapi.Creator{
		Config: func() any { return &collectorapi.MockConfiguration{} },
		Create: func() collectorapi.CollectorV1 {
			if blockNext.CompareAndSwap(true, false) {
				entered <- struct{}{}
				<-release
			}
			collector := ownedTestCollector().(*collectorapi.MockCollectorV1)
			collector.InitFunc = func(context.Context) error {
				switch collector.Config.OptionStr {
				case "old":
					oldInits.Add(1)
				case "new":
					newInits.Add(1)
				}
				return nil
			}
			return collector
		},
		ActivationDependencies: func(map[string]any) ([]collectorapi.ActivationDependency, error) {
			return []collectorapi.ActivationDependency{
				{Kind: "owned-runtime", Name: "browser", Ready: func() error { return nil }},
			}, nil
		},
	}
	generation, registry, _ := newOwnedRunWithCreator(t, creator, true)
	t.Cleanup(unblock)
	owner, err := registry.Bind(t.Context(), "site")
	require.NoError(t, err)
	children := ownedTestChildren(2)
	applyOwned(t, owner, 1, children[1:])
	healthy := waitOwnedRunning(t, owner, 1)[0]
	children[0].Config.Set("option_str", "old")
	blockNext.Store(true)
	applyOwned(t, owner, 2, children)
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("declared-dependency admission did not enter configuration validation")
	}
	children[0].Config.Set("option_str", "new")
	applyCtx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	snapshot, err := owner.Prepare(applyCtx, 3, children)
	require.NoError(t, err)
	require.NoError(
		t,
		snapshot.Apply(applyCtx),
		"parent edit must acquire the yielded graph claim before validation drains",
	)
	during := owner.Status()
	require.Equal(t, healthy.Desired, during[1].Desired)
	require.Equal(t, healthy.Generation, during[1].Generation)
	unblock()
	after := waitOwnedRunning(t, owner, 2)
	require.Zero(t, oldInits.Load(), "superseded admission must not initialize its old configuration")
	require.EqualValues(t, 1, newInits.Load())
	require.Equal(t, healthy.Generation, after[1].Generation)
	require.NoError(t, generation.run.DirtyCause())
}
