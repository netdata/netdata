// SPDX-License-Identifier: GPL-3.0-or-later

package composition

import (
	"bytes"
	"context"
	"errors"
	"testing"
	"time"

	agentdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	jobmgrdiscovery "github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/discovery"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func TestDiscoveryShutdownCancelsSupervisorBeforeProviders(t *testing.T) {
	identity := lifecycle.ResourceIdentity{
		ID:         discoveryResourceID,
		Generation: 1,
	}
	supervisorRef := lifecycle.InheritedTaskRef{
		Slot:       1,
		Generation: 1,
	}
	providerRefs := map[string]lifecycle.InheritedTaskRef{
		"file": {
			Slot:       2,
			Generation: 1,
		},
		"service-discovery": {
			Slot:       3,
			Generation: 1,
		},
	}
	supervisorCancelled := false
	var providers []string
	err := cancelDiscoveryTasks(
		func(
			ref lifecycle.InheritedTaskRef,
			gotIdentity lifecycle.ResourceIdentity,
		) error {
			if gotIdentity != identity {
				return errors.New("identity differs")
			}
			switch ref {
			case supervisorRef:
				supervisorCancelled = true
			case providerRefs["file"]:
				if !supervisorCancelled {
					return errors.New("provider canceled before supervisor")
				}
				providers = append(providers, "file")
			case providerRefs["service-discovery"]:
				if !supervisorCancelled {
					return errors.New("provider canceled before supervisor")
				}
				providers = append(providers, "service-discovery")
			default:
				return errors.New("unknown inherited task")
			}
			return nil
		},
		identity,
		supervisorRef,
		[]string{"file", "service-discovery"},
		providerRefs,
	)
	require.NoError(t, err)
	require.False(t, len(providers) != 2 || providers[0] != "file" || providers[1] != "service-discovery")
}

func TestDiscoveryChildrenWaitForPublication(t *testing.T) {
	entered := make(chan struct{})
	prepared := newPublicationTestDiscovery(
		t,
		publicationTestDiscoverer{entered: entered},
	)
	ready, err := prepared.AcceptStart(context.Background(), 1)
	require.NoError(t, err)

	enteredBeforePublish := false
	select {
	case <-entered:
		enteredBeforePublish = true
	case <-time.After(50 * time.Millisecond):
	}

	require.NoError(t, ready.Publish())

	select {
	case <-entered:
	case <-time.After(time.Second):
		require.FailNow(t, "test failed", "provider did not enter after discovery publication")
	}
	stopCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	require.NoError(t, ready.Stop(stopCtx))

	require.NoError(t, ready.Finalize())

	require.False(t, enteredBeforePublish)
}

func TestDiscoveryPermitFailurePaths(t *testing.T) {
	tests := map[string]struct {
		fail func(*testing.T, *preparedDiscovery)
	}{
		"prepared disposal": {
			fail: func(t *testing.T, prepared *preparedDiscovery) {
				t.Helper()

				require.NoError(t, prepared.Dispose(context.Background()))
			},
		},
		"partial start": {
			fail: func(t *testing.T, prepared *preparedDiscovery) {
				t.Helper()

				require.NoError(t, prepared.tasks.SealInherited())

				ready, err := prepared.AcceptStart(context.Background(), 1)
				require.False(t, err == nil || ready != nil)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			prepared := newPublicationTestDiscovery(
				t,
				publicationTestDiscoverer{},
			)
			test.fail(t, prepared)

			require.EqualValues(t, lifecycle.LongLivedCensus{}, prepared.tasks.LongLivedCensus())
		})
	}
}

func TestDiscoverySupervisorReturnsContainedPanic(t *testing.T) {
	config := confgroup.Config{}.SetName("job").SetModule("module").SetProvider("test").SetSourceType(confgroup.TypeStock).
		SetSource("source")
	pipeline := newDiscoveryTestPipeline(
		t,
		publicationTestDiscoverer{
			groups: []*confgroup.Group{{
				Source:  "source",
				Configs: []confgroup.Config{config},
			}},
		},
	)
	decisions, err := jobmgrdiscovery.NewDecisionIndex(
		jobmgrdiscovery.DecisionConfig{
			Generation: 1,
			Commands:   discoveryTestCommands{},
			Plan: func(
				jobmgrdiscovery.DiscoveredChange,
			) (jobmgr.WorkPlan, error) {
				panic("decision panic")
			},
		},
	)
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	providerErr := make(chan error, 1)
	go func() {
		providerErr <- pipeline.RunProvider(ctx, "provider")
	}()
	err = runDiscoverySupervisor(
		ctx,
		pipeline,
		decisions,
	)
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
	cancel()

	require.NoError(t, <-providerErr)
}

func TestRunGenerationOwnsFrozenDiscoveryChildren(t *testing.T) {
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{
			agentdiscovery.NewProviderFactory(
				"enabled",
				func(agentdiscovery.BuildContext) (
					agentdiscovery.Discoverer,
					bool,
					error,
				) {
					return runTestDiscoverer{}, true, nil
				},
			),
			agentdiscovery.NewProviderFactory(
				"disabled",
				func(agentdiscovery.BuildContext) (
					agentdiscovery.Discoverer,
					bool,
					error,
				) {
					return nil, false, nil
				},
			),
		},
	)
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		UIDs: uids, Frames: frames,
		Modules: collectorapi.Registry{},
		Jobs:    testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"test": {}},
			},
			Providers: catalog,
		},
	})
	require.NoError(t, err)

	require.NoError(t, generation.start(context.Background()))

	require.EqualValues(t, 2, generation.tasks.InheritedCensus().Active)

	require.EqualValues(t, lifecycle.LongLivedCensus{
		Active:         1,
		Pipelines:      1,
		GActive:        2,
		ExternalActive: 1,
	}, generation.tasks.LongLivedCensus(),
	)

	generation.Stop()

	require.NoError(t, generation.Wait(context.Background()))

	require.EqualValues(t, lifecycle.InheritedTaskCensus{}, generation.tasks.InheritedCensus())

	require.EqualValues(t, lifecycle.LongLivedCensus{}, generation.tasks.LongLivedCensus())

	closeRunTestUIDs(t, uids)
}

func newPublicationTestDiscovery(
	t *testing.T,
	discoverer agentdiscovery.Discoverer,
) *preparedDiscovery {
	t.Helper()
	pipeline := newDiscoveryTestPipeline(t, discoverer)
	decisions, err := jobmgrdiscovery.NewDecisionIndex(
		jobmgrdiscovery.DecisionConfig{
			Generation: 1,
			Commands:   discoveryTestCommands{},
			Plan: func(
				jobmgrdiscovery.DiscoveredChange,
			) (jobmgr.WorkPlan, error) {
				return jobmgr.WorkPlan{}, nil
			},
		},
	)
	require.NoError(t, err)
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	require.NoError(t, err)
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	require.NoError(t, err)
	plan, err := lifecycle.NewPipelineLongLivedPlan(
		[]string{"provider"},
	)
	require.NoError(t, err)
	identity := lifecycle.ResourceIdentity{
		ID: discoveryResourceID, Generation: 1,
	}
	permit, err := tasks.IssueLongLivedPermit(identity, plan)
	require.NoError(t, err)
	prepared, err := newPreparedDiscovery(
		pipeline,
		decisions,
		tasks,
		identity,
		permit,
	)
	require.NoError(t, err)
	return prepared
}

func newDiscoveryTestPipeline(
	t *testing.T,
	discoverer agentdiscovery.Discoverer,
) *agentdiscovery.PipelineGeneration {
	t.Helper()
	catalog, err := agentdiscovery.NewProviderCatalog(
		[]agentdiscovery.ProviderFactory{
			agentdiscovery.NewProviderFactory(
				"provider",
				func(agentdiscovery.BuildContext) (
					agentdiscovery.Discoverer,
					bool,
					error,
				) {
					return discoverer, true, nil
				},
			),
		},
	)
	require.NoError(t, err)
	pipeline, err := agentdiscovery.NewPipelineGeneration(
		agentdiscovery.PipelineConfig{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"module": {}},
			},
			Providers: catalog,
		},
	)
	require.NoError(t, err)
	return pipeline
}

type publicationTestDiscoverer struct {
	entered chan<- struct{}
	groups  []*confgroup.Group
}

func (ptd publicationTestDiscoverer) Run(
	ctx context.Context,
	out chan<- []*confgroup.Group,
) {
	if ptd.entered != nil {
		close(ptd.entered)
	}
	if ptd.groups != nil {
		select {
		case <-ctx.Done():
			return
		case out <- ptd.groups:
		}
	}
	<-ctx.Done()
}

type discoveryTestCommands struct{}

func (discoveryTestCommands) SubmitPreparedAndWait(
	context.Context,
	jobmgr.Request,
	jobmgr.WorkPlan,
) error {
	return nil
}
