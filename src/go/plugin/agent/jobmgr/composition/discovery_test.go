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
