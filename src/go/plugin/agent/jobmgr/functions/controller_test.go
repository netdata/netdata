// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

func TestFunctionControllerJobLifecycle(t *testing.T) {
	handler := &controllerTestHandler{}
	constructions := 0
	modules := collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				constructions++
				return handler
			},
		},
	}
	controller, catalog, err := NewController(1, modules)
	if err != nil {
		t.Fatal(err)
	}
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Bind(&mutations, publication); err != nil {
		t.Fatal(err)
	}
	if err := controller.Activate(); err != nil {
		t.Fatal(err)
	}
	job := &controllerTestJob{
		fullName: "module_job", module: "module", name: "job", running: true,
	}
	handle, err := controller.PrepareJob(
		JobIdentity{ID: job.FullName(), Generation: 1},
		job,
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Publish(); err != nil {
		t.Fatal(err)
	}
	if constructions != 1 {
		t.Fatalf("handler constructions=%d want=1", constructions)
	}
	ctx := context.Background()
	allocations := testing.AllocsPerRun(1_000, func() {
		if err := controller.ReconcileModule(ctx, "module"); err != nil {
			panic(err)
		}
	})
	if allocations != 0 {
		t.Fatalf(
			"unchanged module reconciliation allocations=%f want=0",
			allocations,
		)
	}
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "request", Route: "module:method",
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decision.Plan.Runner.RunTask(context.Background()); err != nil {
		t.Fatal(err)
	}
	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	if err != nil {
		t.Fatal(err)
	}
	if cleanup.Ref.Valid() {
		t.Fatal("live generation produced cleanup")
	}
	if err := handle.CloseAndDrain(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := handle.Cleanup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if handler.cleanupCount() != 1 {
		t.Fatalf("handler cleanups=%d want=1", handler.cleanupCount())
	}
	if got := publicationPort.events; !equalPublicationEvents(
		got,
		[]string{"publish:module:method", "withdraw:module:method"},
	) {
		t.Fatalf("publication events=%v", got)
	}
	decision, err = catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "after-close", Route: "module:method",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Rejected == 0 {
		t.Fatal("closed job route still resolved")
	}
}

func TestFunctionControllerRawRequestFidelity(t *testing.T) {
	handler := &controllerTestHandler{}
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "raw", RawRequest: true}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	}
	controller, catalog, err := NewController(1, modules)
	if err != nil {
		t.Fatal(err)
	}
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Bind(&mutations, publication); err != nil {
		t.Fatal(err)
	}
	if err := controller.Activate(); err != nil {
		t.Fatal(err)
	}
	lookup := jobmgr.FunctionLookup{
		UID: "raw-request", Route: "module:raw", Args: []string{"info", "arg"},
		Payload: []byte(`{"value":1}`), ContentType: "application/json",
		Permissions: "0x0013", CallerSource: "cloud",
		Timeout: 17 * time.Second, HasPayload: true,
	}
	decision, err := catalog.ResolveAndAcquire(lookup)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decision.Plan.Runner.RunTask(context.Background()); err != nil {
		t.Fatal(err)
	}
	if cleanup, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
		t.Fatal(err)
	} else if cleanup.Ref.Valid() {
		t.Fatal("live agent generation produced cleanup")
	}
	got := handler.rawRequest()
	if got.Method != "raw" || !got.Info ||
		got.ContentType != lookup.ContentType ||
		got.Permissions != lookup.Permissions ||
		got.Source != lookup.CallerSource ||
		got.Timeout != lookup.Timeout ||
		string(got.Payload) != string(lookup.Payload) {
		t.Fatalf("raw request=%+v", got)
	}
	got.Args[0] = "changed"
	got.Payload[0] = 'X'
	if lookup.Args[0] != "info" || lookup.Payload[0] != '{' {
		t.Fatal("handler request aliases lookup-owned slices")
	}
}

func TestFunctionControllerAgentAvailabilityIsMonotonic(t *testing.T) {
	available := false
	handler := &controllerTestHandler{}
	modules := collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{
					ID: "delayed", Available: func() bool { return available },
				}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	}
	controller, catalog, err := NewController(1, modules)
	if err != nil {
		t.Fatal(err)
	}
	mutations := controllerTestMutationPort{catalog: catalog}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	if err != nil {
		t.Fatal(err)
	}
	if err := controller.Bind(&mutations, publication); err != nil {
		t.Fatal(err)
	}
	if err := controller.Activate(); err != nil {
		t.Fatal(err)
	}
	if len(publicationPort.events) != 0 {
		t.Fatalf("unavailable function was published: %v", publicationPort.events)
	}
	available = true
	if err := controller.ReconcileModule(context.Background(), "module"); err != nil {
		t.Fatal(err)
	}
	available = false
	if err := controller.ReconcileModule(context.Background(), "module"); err != nil {
		t.Fatal(err)
	}
	if got := publicationPort.events; !equalPublicationEvents(
		got,
		[]string{"publish:module:delayed"},
	) {
		t.Fatalf("availability publication events=%v", got)
	}
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "still-published", Route: "module:delayed",
	})
	if err != nil {
		t.Fatal(err)
	}
	if decision.Rejected != 0 {
		t.Fatalf("published agent function was withdrawn: %v", decision.Rejected)
	}
	if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
		t.Fatal(err)
	}
}

func TestFunctionControllerRejectsInvalidDeclarations(t *testing.T) {
	tests := map[string]struct {
		methods []funcapi.FunctionConfig
	}{
		"empty method ID": {
			methods: []funcapi.FunctionConfig{{}},
		},
		"duplicate method ID": {
			methods: []funcapi.FunctionConfig{{ID: "same"}, {ID: "same"}},
		},
		"empty alias": {
			methods: []funcapi.FunctionConfig{{ID: "method", Aliases: []string{""}}},
		},
		"duplicate primary alias": {
			methods: []funcapi.FunctionConfig{{
				ID: "method", Aliases: []string{"module:method"},
			}},
		},
		"duplicate alias": {
			methods: []funcapi.FunctionConfig{{
				ID: "method", Aliases: []string{"alias", "alias"},
			}},
		},
		"invalid public name": {
			methods: []funcapi.FunctionConfig{{
				ID: "method", FunctionName: "invalid name",
			}},
		},
		"unmarshalable presentation": {
			methods: []funcapi.FunctionConfig{(funcapi.FunctionConfig{
				ID: "method",
			}).WithPresentation(func() {})},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			_, _, err := NewController(1, collectorapi.Registry{
				"module": {
					AgentFunctions: func() []funcapi.FunctionConfig {
						return test.methods
					},
					MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
						return &controllerTestHandler{}
					},
				},
			})
			if err == nil {
				t.Fatal("invalid Function declaration was accepted")
			}
		})
	}
}

type controllerTestMutationPort struct {
	catalog *Catalog
}

func (port *controllerTestMutationPort) MutateFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) (uint64, error) {
	if port == nil || port.catalog == nil {
		return 0, errors.New("nil mutation port")
	}
	if err := port.catalog.BeginMutation(mutation); err != nil {
		return 0, err
	}
	for {
		var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
		progress, count, err := port.catalog.AdvanceMutation(
			jobmgr.MaximumFunctionMutationQuantum,
			&cleanups,
		)
		if err != nil {
			return 0, err
		}
		for index := 0; index < count; index++ {
			cleanup := cleanups[index]
			_, cleanupErr := cleanup.Runner.RunTask(context.Background())
			if err := port.catalog.CompleteCleanup(cleanup.Ref, cleanupErr); err != nil {
				return 0, errors.Join(cleanupErr, err)
			}
		}
		if progress.Done {
			return progress.Version, nil
		}
	}
}

type controllerTestJob struct {
	fullName  string
	module    string
	name      string
	running   bool
	collector any
}

func (job *controllerTestJob) FullName() string   { return job.fullName }
func (job *controllerTestJob) ModuleName() string { return job.module }
func (job *controllerTestJob) Name() string       { return job.name }
func (job *controllerTestJob) IsRunning() bool    { return job.running }
func (job *controllerTestJob) Collector() any     { return job.collector }

type controllerTestHandler struct {
	mu       sync.Mutex
	raw      funcapi.RawMethodRequest
	cleanups int
}

func (handler *controllerTestHandler) MethodParams(
	context.Context,
	string,
) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (handler *controllerTestHandler) Handle(
	context.Context,
	string,
	funcapi.ResolvedParams,
) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{Status: 200}
}

func (handler *controllerTestHandler) HandleRaw(
	_ context.Context,
	request funcapi.RawMethodRequest,
) *funcapi.FunctionResponse {
	handler.mu.Lock()
	handler.raw = request
	handler.mu.Unlock()
	return &funcapi.FunctionResponse{Status: 200}
}

func (handler *controllerTestHandler) Cleanup(context.Context) {
	handler.mu.Lock()
	handler.cleanups++
	handler.mu.Unlock()
}

func (handler *controllerTestHandler) rawRequest() funcapi.RawMethodRequest {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.raw
}

func (handler *controllerTestHandler) cleanupCount() int {
	handler.mu.Lock()
	defer handler.mu.Unlock()
	return handler.cleanups
}
