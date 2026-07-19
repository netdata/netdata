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
					return errors.New(
						"provider canceled before supervisor",
					)
				}
				providers = append(providers, "file")
			case providerRefs["service-discovery"]:
				if !supervisorCancelled {
					return errors.New(
						"provider canceled before supervisor",
					)
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
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 ||
		providers[0] != "file" ||
		providers[1] != "service-discovery" {
		t.Fatalf("provider cancellation order=%v", providers)
	}
}

func TestDiscoveryChildrenWaitForPublication(t *testing.T) {
	entered := make(chan struct{})
	prepared, admission, admissionRef := newPublicationTestDiscovery(
		t,
		publicationTestDiscoverer{entered: entered},
	)
	ready, err := prepared.AcceptStart(context.Background(), 1)
	if err != nil {
		t.Fatal(err)
	}

	enteredBeforePublish := false
	select {
	case <-entered:
		enteredBeforePublish = true
	case <-time.After(50 * time.Millisecond):
	}
	if err := ready.Publish(); err != nil {
		t.Fatal(err)
	}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("provider did not enter after discovery publication")
	}
	stopCtx, cancel := context.WithTimeout(
		context.Background(),
		time.Second,
	)
	defer cancel()
	if err := ready.Stop(stopCtx); err != nil {
		t.Fatal(err)
	}
	if err := ready.Finalize(); err != nil {
		t.Fatal(err)
	}
	if _, err := admission.ReleaseOrdinary(admissionRef); err != nil {
		t.Fatal(err)
	}
	if enteredBeforePublish {
		t.Fatal("provider entered before discovery publication")
	}
}

func TestDiscoverySupervisorPanicFailsRun(t *testing.T) {
	config := confgroup.Config{}.
		SetName("job").
		SetModule("module").
		SetProvider("test").
		SetSourceType(confgroup.TypeStock).
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
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	providerErr := make(chan error, 1)
	go func() {
		providerErr <- pipeline.RunProvider(ctx, "provider")
	}()
	failed := make(chan error, 1)
	err = runDiscoverySupervisor(
		ctx,
		pipeline,
		decisions,
		func(err error) { failed <- err },
	)
	if !errors.Is(err, lifecycle.ErrTaskPanic) {
		t.Fatalf("supervisor error=%v", err)
	}
	select {
	case failErr := <-failed:
		if !errors.Is(failErr, lifecycle.ErrTaskPanic) {
			t.Fatalf("fail-stop error=%v", failErr)
		}
	case <-time.After(time.Second):
		t.Fatal("supervisor panic did not fail-stop the run")
	}
	cancel()
	if err := <-providerErr; err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	admission := lifecycle.NewAdmissionLedger()
	uids := lifecycle.NewUIDLedger()
	generation, err := newRunGeneration(runGenerationConfig{
		Generation: 1, ShutdownTimeout: time.Second,
		Clock: lifecycle.RealClock{}, Admission: admission,
		UIDs: uids, Frames: frames,
		Modules: collectorapi.Registry{},
		Jobs:    testRunJobServices(t),
		Discovery: runDiscoveryServices{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"test": {}},
			},
			Providers: catalog,
		},
		Planner: func(
			runPlannerCapabilities,
		) (jobmgr.Planner, jobmgr.RunFinalizer, error) {
			return runRejectingPlanner{},
				jobmgr.RunFinalizerFunc(
					func(context.Context, uint64) error {
						return nil
					},
				),
				nil
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := generation.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	if census := generation.tasks.InheritedCensus(); census.Active != 2 {
		t.Fatalf("inherited census=%+v, want two active records", census)
	}
	if census := generation.tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{
		Active:         1,
		Pipelines:      1,
		Bytes:          3 * lifecycle.TaskChildExecutionBytes,
		GActive:        2,
		ExternalActive: 1,
	}) {
		t.Fatalf("long-lived census=%+v", census)
	}

	generation.Stop()
	if err := generation.Wait(context.Background()); err != nil {
		t.Fatal(err)
	}
	if census := generation.tasks.InheritedCensus(); census != (lifecycle.InheritedTaskCensus{}) {
		t.Fatalf("final inherited census=%+v", census)
	}
	if census := generation.tasks.LongLivedCensus(); census != (lifecycle.LongLivedCensus{}) {
		t.Fatalf("final long-lived census=%+v", census)
	}
	if err := admission.CloseDrained(1); err != nil {
		t.Fatal(err)
	}
	closeRunTestUIDs(t, uids)
}

func newPublicationTestDiscovery(
	t *testing.T,
	discoverer agentdiscovery.Discoverer,
) (
	*preparedDiscovery,
	*lifecycle.AdmissionLedger,
	lifecycle.AdmissionRef,
) {
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
	if err != nil {
		t.Fatal(err)
	}
	frames, err := lifecycle.NewFrameOwner(&bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	tasks, err := lifecycle.NewTaskSupervisor(frames)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := lifecycle.NewPipelineLongLivedPlan(
		[]string{"provider"},
	)
	if err != nil {
		t.Fatal(err)
	}
	admission := lifecycle.NewAdmissionLedger()
	requested := admission.RequestOrdinary(
		1,
		lifecycle.AdmissionLaneRef{Slot: 1, Generation: 1},
		plan.Bytes()+lifecycle.TaskChildExecutionBytes,
	)
	if requested.Rejected != nil {
		t.Fatal(requested.Rejected)
	}
	var grants [4]lifecycle.AdmissionGrant
	count, _, err := admission.TakeGrants(1, &grants)
	if err != nil || count != 1 || grants[0].Ref != requested.Ref {
		t.Fatalf(
			"admission grant count=%d grant=%+v err=%v",
			count,
			grants[0],
			err,
		)
	}
	identity := lifecycle.ResourceIdentity{
		ID: discoveryResourceID, Generation: 1,
	}
	permit, err := tasks.IssueLongLivedPermit(
		admission,
		requested.Ref,
		identity,
		plan,
	)
	if err != nil {
		t.Fatal(err)
	}
	prepared, err := newPreparedDiscovery(
		pipeline,
		decisions,
		tasks,
		identity,
		permit,
		func(error) {},
	)
	if err != nil {
		t.Fatal(err)
	}
	return prepared, admission, requested.Ref
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
	if err != nil {
		t.Fatal(err)
	}
	pipeline, err := agentdiscovery.NewPipelineGeneration(
		agentdiscovery.PipelineConfig{
			BuildContext: agentdiscovery.BuildContext{
				Registry: confgroup.Registry{"module": {}},
			},
			Providers: catalog,
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	return pipeline
}

type publicationTestDiscoverer struct {
	entered chan<- struct{}
	groups  []*confgroup.Group
}

func (discoverer publicationTestDiscoverer) Run(
	ctx context.Context,
	out chan<- []*confgroup.Group,
) {
	if discoverer.entered != nil {
		close(discoverer.entered)
	}
	if discoverer.groups != nil {
		select {
		case <-ctx.Done():
			return
		case out <- discoverer.groups:
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
