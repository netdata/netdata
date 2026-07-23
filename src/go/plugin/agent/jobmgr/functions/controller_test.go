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
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/stretchr/testify/require"
)

func TestControllerChangedRouteNames(t *testing.T) {
	firstGeneration := &HandlerGenerationDeclaration{
		ID: "first",
	}
	secondGeneration := &HandlerGenerationDeclaration{
		ID: "second",
	}
	base := controllerRoute{
		declaration: Declaration{
			Generation: firstGeneration,
		},
		publication: PublicationRecord{
			Name:       "method",
			Generation: 1,
		},
	}
	publicationChanged := base
	publicationChanged.publication.Generation++
	handlerGenerationChanged := base
	handlerGenerationChanged.declaration.Generation = secondGeneration
	tests := map[string]struct {
		current map[string]controllerRoute
		next    map[string]controllerRoute
		want    []string
	}{
		"unchanged": {
			current: map[string]controllerRoute{"method": base},
			next:    map[string]controllerRoute{"method": base},
		},
		"added": {
			next: map[string]controllerRoute{"method": base},
			want: []string{"method"},
		},
		"removed": {
			current: map[string]controllerRoute{"method": base},
			want:    []string{"method"},
		},
		"publication changed": {
			current: map[string]controllerRoute{"method": base},
			next:    map[string]controllerRoute{"method": publicationChanged},
			want:    []string{"method"},
		},
		"handler generation changed": {
			current: map[string]controllerRoute{"method": base},
			next:    map[string]controllerRoute{"method": handlerGenerationChanged},
			want:    []string{"method"},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			require.Equal(t, test.want, controllerChangedRouteNames(test.current, test.next))
		})
	}
}

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
	require.NoError(t, err)
	mutations := controllerTestMutationPort{
		catalog: catalog,
	}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	job := &controllerTestJob{
		fullName: "module_job",
		module:   "module",
		name:     "job",
		running:  true,
	}
	handle, err := controller.PrepareJob(lifecycle.ResourceIdentity{
		ID:         job.FullName(),
		Generation: 1,
	}, job)
	require.NoError(t, err)

	require.NoError(t, handle.Publish())

	require.EqualValues(t, 1, constructions)
	ctx := context.Background()
	allocations := testing.AllocsPerRun(1_000, func() {
		if err := controller.ReconcileModule(ctx, "module"); err != nil {
			panic(err)
		}
	})
	require.EqualValues(t, 0, allocations)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "request",
		Route: "module:method",
	})
	require.NoError(t, err)

	_, runTaskErr := decision.Plan.Work(context.Background())
	require.NoError(t, runTaskErr)

	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Valid())

	require.NoError(t, handle.CloseAndDrain(context.Background()))

	require.EqualValues(t, 1, handler.cleanupCount())

	got := publicationPort.events
	require.Equal(t, []string{"publish:module:method", "withdraw:module:method"}, got)

	decision, err = catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "after-close",
		Route: "module:method",
	})
	require.NoError(t, err)
	require.NotEqualValues(t, 0, decision.Rejected)
}

func TestFunctionControllerClosesAdmissionBeforeExternalWithdrawal(t *testing.T) {
	handler := &controllerTestHandler{}
	controller, catalog, err := NewController(1, collectorapi.Registry{
		"module": {
			SharedFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return handler
			},
		},
	})
	require.NoError(t, err)
	mutations := controllerTestMutationPort{
		catalog: catalog,
	}
	publicationPort := &blockingWithdrawPublicationPort{
		recordingPublicationPort: newRecordingPublicationPort(),
		entered:                  make(chan struct{}),
		release:                  make(chan struct{}),
	}
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	job := &controllerTestJob{
		fullName: "module_job",
		module:   "module",
		name:     "job",
		running:  true,
	}
	handle, err := controller.PrepareJob(lifecycle.ResourceIdentity{
		ID:         job.FullName(),
		Generation: 1,
	}, job)
	require.NoError(t, err)

	require.NoError(t, handle.Publish())

	closed := make(chan error, 1)
	go func() {
		closed <- handle.CloseAndDrain(context.Background())
	}()
	<-publicationPort.entered
	decision, resolveErr := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "during-withdraw",
		Route: "module:method",
	})
	if decision.Lease.Valid() {
		_, _ = catalog.ReleaseInvocation(decision.Lease)
	}
	close(publicationPort.release)
	closeErr := <-closed

	require.NoError(t, resolveErr)
	require.EqualValues(t, lifecycle.ControlUnavailable, decision.Rejected)
	require.NoError(t, closeErr)
}

func TestFunctionControllerWithdrawalFailureCleansUnpublishedSuccessor(t *testing.T) {
	available := false
	predecessor := &controllerTestHandler{}
	successor := &controllerTestHandler{}
	handlers := []*controllerTestHandler{predecessor, successor}
	constructions := 0
	controller, catalog, err := NewController(1, collectorapi.Registry{
		"module": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{
					{ID: "always"},
					{ID: "conditional", Available: func() bool { return available }},
				}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				handler := handlers[constructions]
				constructions++
				return handler
			},
		},
	})
	require.NoError(t, err)
	mutations := controllerTestMutationPort{
		catalog: catalog,
	}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&mutations, publication))
	require.NoError(t, controller.Activate())

	withdrawErr := errors.New("withdraw failed")
	publicationPort.withdrawErr = withdrawErr
	available = true
	err = controller.ReconcileModule(context.Background(), "module")
	require.ErrorIs(t, err, withdrawErr)
	require.EqualValues(t, 2, constructions)
	require.Zero(t, predecessor.cleanupCount())
	require.EqualValues(t, 1, successor.cleanupCount())

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "predecessor-restored",
		Route: "module:always",
	})
	require.NoError(t, err)
	require.True(t, decision.Lease.Valid())
	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, cleanup.Valid())

	require.NoError(t, catalog.BeginClose())
	for {
		cleanups, more, err := catalog.CloseStep(jobmgr.MaximumFunctionCloseQuantum)
		require.NoError(t, err)
		for _, cleanup := range cleanups {
			runCleanupPlan(t, catalog, cleanup)
		}
		if !more {
			break
		}
	}
	require.EqualValues(t, 1, predecessor.cleanupCount())
	require.EqualValues(t, 1, successor.cleanupCount())
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
	require.NoError(t, err)
	mutations := controllerTestMutationPort{
		catalog: catalog,
	}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	lookup := jobmgr.FunctionLookup{
		UID:          "raw-request",
		Route:        "module:raw",
		Args:         []string{"info", "arg"},
		Payload:      []byte(`{"value":1}`),
		ContentType:  "application/json",
		Permissions:  "0x0013",
		CallerSource: "cloud",
		Timeout:      17 * time.Second,
		HasPayload:   true,
	}
	decision, err := catalog.ResolveAndAcquire(lookup)
	require.NoError(t, err)

	_, runTaskErr := decision.Plan.Work(context.Background())
	require.NoError(t, runTaskErr)

	if cleanup, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
		require.FailNow(t, "test failed", err)
	} else {
		require.False(t, cleanup.Valid())
	}
	got := handler.rawRequest()
	require.False(t, got.Method != "raw" || !got.Info ||
		got.ContentType != lookup.ContentType ||
		got.Permissions != lookup.Permissions ||
		got.Source != lookup.CallerSource ||
		got.Timeout != lookup.Timeout ||
		string(got.Payload) != string(lookup.Payload))
	got.Args[0] = "changed"
	got.Payload[0] = 'X'
	require.False(t, lookup.Args[0] != "info" || lookup.Payload[0] != '{')
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
	require.NoError(t, err)
	mutations := controllerTestMutationPort{
		catalog: catalog,
	}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)

	require.NoError(t, controller.Bind(&mutations, publication))

	require.NoError(t, controller.Activate())

	require.EqualValues(t, 0, len(publicationPort.events))
	available = true

	require.NoError(t, controller.ReconcileModule(context.Background(), "module"))

	available = false

	require.NoError(t, controller.ReconcileModule(context.Background(), "module"))

	got := publicationPort.events
	require.Equal(t, []string{"publish:module:delayed"}, got)

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID:   "still-published",
		Route: "module:delayed",
	})
	require.NoError(t, err)
	require.EqualValues(t, 0, decision.Rejected)

	_, releaseInvocationErr := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, releaseInvocationErr)

}

func TestFunctionControllerRejectsInvalidDeclarations(t *testing.T) {
	tests := map[string]struct {
		methods []funcapi.FunctionConfig
	}{
		"empty method ID":       {methods: []funcapi.FunctionConfig{{}}},
		"duplicate method ID":   {methods: []funcapi.FunctionConfig{{ID: "same"}, {ID: "same"}}},
		"empty alias":           {methods: []funcapi.FunctionConfig{{ID: "method", Aliases: []string{""}}}},
		"quote in help":         {methods: []funcapi.FunctionConfig{{ID: "method", Help: `invalid "help"`}}},
		"quote in tags":         {methods: []funcapi.FunctionConfig{{ID: "method", Tags: `invalid "tags"`}}},
		"backslash in help":     {methods: []funcapi.FunctionConfig{{ID: "method", Help: `invalid\help`}}},
		"backslash in tags":     {methods: []funcapi.FunctionConfig{{ID: "method", Tags: `invalid\tags`}}},
		"control in help":       {methods: []funcapi.FunctionConfig{{ID: "method", Help: "invalid\nhelp"}}},
		"control in tags":       {methods: []funcapi.FunctionConfig{{ID: "method", Tags: "invalid\ttags"}}},
		"delete in help":        {methods: []funcapi.FunctionConfig{{ID: "method", Help: "invalid\x7fhelp"}}},
		"delete in tags":        {methods: []funcapi.FunctionConfig{{ID: "method", Tags: "invalid\x7ftags"}}},
		"invalid UTF-8 in help": {methods: []funcapi.FunctionConfig{{ID: "method", Help: string([]byte{0xff})}}},
		"invalid UTF-8 in tags": {methods: []funcapi.FunctionConfig{{ID: "method", Tags: string([]byte{0xff})}}},
		"duplicate primary alias": {
			methods: []funcapi.FunctionConfig{{ID: "method", Aliases: []string{"module:method"}}},
		},
		"duplicate alias":     {methods: []funcapi.FunctionConfig{{ID: "method", Aliases: []string{"alias", "alias"}}}},
		"invalid public name": {methods: []funcapi.FunctionConfig{{ID: "method", FunctionName: "invalid name"}}},
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
			require.Error(t, err)
		})
	}
}

func TestFunctionControllerRejectsInvalidInstanceMetadataBeforeMutation(t *testing.T) {
	handler := &controllerTestHandler{}
	constructions := 0
	controller, catalog, err := NewController(1, collectorapi.Registry{
		"module": {
			InstanceFunctions: func(job collectorapi.RuntimeJob) []funcapi.FunctionConfig {
				help := "valid help"
				if job.Name() == "invalid" {
					help = `invalid "help"`
				}
				return []funcapi.FunctionConfig{{ID: job.Name() + ":method", Help: help}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				constructions++
				return handler
			},
		},
	})
	require.NoError(t, err)
	mutations := controllerTestMutationPort{
		catalog: catalog,
	}
	publicationPort := newRecordingPublicationPort()
	publication, err := NewPublication(1, publicationPort)
	require.NoError(t, err)
	require.NoError(t, controller.Bind(&mutations, publication))
	require.NoError(t, controller.Activate())

	invalidJob := &controllerTestJob{
		fullName: "module_invalid",
		module:   "module",
		name:     "invalid",
		running:  true,
	}
	invalidHandle, err := controller.PrepareJob(lifecycle.ResourceIdentity{
		ID:         invalidJob.FullName(),
		Generation: 1,
	}, invalidJob)
	require.NoError(t, err)
	require.ErrorContains(t, invalidHandle.Publish(), "invalid Function help")
	require.Empty(t, publicationPort.events)
	require.Zero(t, constructions)

	validJob := &controllerTestJob{
		fullName: "module_valid",
		module:   "module",
		name:     "valid",
		running:  true,
	}
	validHandle, err := controller.PrepareJob(lifecycle.ResourceIdentity{
		ID:         validJob.FullName(),
		Generation: 1,
	}, validJob)
	require.NoError(t, err)
	require.NoError(t, validHandle.Publish())
	require.Equal(t, []string{"publish:module:valid:method"}, publicationPort.events)
	require.EqualValues(t, 1, constructions)

	require.NoError(t, validHandle.CloseAndDrain(context.Background()))
	require.Equal(t, []string{
		"publish:module:valid:method",
		"withdraw:module:valid:method",
	}, publicationPort.events)
	require.EqualValues(t, 1, handler.cleanupCount())
}

func TestFunctionControllerCleansNewGenerationWhenLaterGroupBuildFails(t *testing.T) {
	tests := map[string]struct {
		panic bool
	}{
		"returned error": {},
		"panic":          {panic: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handlers := make(map[string][]*controllerTestHandler)
			fresh := &controllerPanickingCleanupHandler{}
			failZ := false
			controller, catalog, err := NewController(1, collectorapi.Registry{
				"module": {
					InstanceFunctions: func(job collectorapi.RuntimeJob) []funcapi.FunctionConfig {
						return []funcapi.FunctionConfig{{ID: job.Name() + ":method"}}
					},
					MethodHandler: func(job collectorapi.RuntimeJob) funcapi.MethodHandler {
						if job.Name() == "z" && failZ {
							if test.panic {
								panic("test handler construction panic")
							}
							return nil
						}
						if job.Name() == "m" && failZ {
							return fresh
						}
						handler := &controllerTestHandler{}
						handlers[job.Name()] = append(handlers[job.Name()], handler)
						return handler
					},
				},
			})
			require.NoError(t, err)
			mutations := controllerTestMutationPort{
				catalog: catalog,
			}
			publication, err := NewPublication(1, newRecordingPublicationPort())
			require.NoError(t, err)
			require.NoError(t, controller.Bind(&mutations, publication))
			require.NoError(t, controller.Activate())

			jobs := map[string]*controllerTestJob{
				"a": {
					fullName: "module_a",
					module:   "module",
					name:     "a",
					running:  true,
				},
				"m": {
					fullName: "module_m",
					module:   "module",
					name:     "m",
					running:  true,
				},
				"z": {
					fullName: "module_z",
					module:   "module",
					name:     "z",
					running:  true,
				},
			}
			handles := make(map[string]*JobHandle, len(jobs))
			for _, jobName := range []string{"a", "m", "z"} {
				job := jobs[jobName]
				handle, err := controller.PrepareJob(lifecycle.ResourceIdentity{
					ID:         job.FullName(),
					Generation: 1,
				}, job)
				require.NoError(t, err)
				require.NoError(t, handle.Publish())
				handles[jobName] = handle
				require.Len(t, handlers[jobName], 1)
			}

			jobs["m"].running = false
			jobs["z"].running = false
			failZ = true
			reconcile := func() {
				err = controller.ReconcileModule(context.Background(), "module")
			}
			if test.panic {
				require.PanicsWithValue(t, "test handler construction panic", reconcile)
			} else {
				require.NotPanics(t, reconcile)
				require.ErrorContains(t, err, `nil handler for job "z"`)
				require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
			}
			require.EqualValues(t, 1, fresh.cleanupCount())
			require.Zero(t, handlers["a"][0].cleanupCount())
			require.Zero(t, handlers["m"][0].cleanupCount())
			require.Zero(t, handlers["z"][0].cleanupCount())

			failZ = false
			jobs["m"].running = true
			jobs["z"].running = true
			for _, jobName := range []string{"a", "m", "z"} {
				require.NoError(t, handles[jobName].CloseAndDrain(context.Background()))
				require.EqualValues(t, 1, handlers[jobName][0].cleanupCount())
			}
			require.EqualValues(t, 1, fresh.cleanupCount())
		})
	}
}

func TestMethodGenerationCleansPartialHandlerConstruction(t *testing.T) {
	tests := map[string]struct {
		panic bool
	}{
		"nil handler": {},
		"panic":       {panic: true},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			handler := &controllerTestHandler{}
			constructions := 0
			creator := collectorapi.Creator{
				MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
					constructions++
					if constructions != 2 {
						return handler
					}
					if test.panic {
						panic("test handler construction panic")
					}
					return nil
				},
			}
			jobs := map[string]collectorapi.RuntimeJob{
				"a": &controllerTestJob{
					fullName: "module_a",
					module:   "module",
					name:     "a",
					running:  true,
				},
				"b": &controllerTestJob{
					fullName: "module_b",
					module:   "module",
					name:     "b",
					running:  true,
				},
			}
			var generation *methodGeneration
			var err error
			construct := func() {
				generation, err = newMethodGeneration(
					"generation",
					"module",
					methodGenerationShared,
					creator,
					[]funcapi.FunctionConfig{{ID: "method"}},
					jobs,
				)
			}
			if test.panic {
				require.PanicsWithValue(t, "test handler construction panic", construct)
			} else {
				require.NotPanics(t, construct)
				require.ErrorContains(t, err, "nil handler")
			}
			require.Nil(t, generation)
			require.EqualValues(t, 1, handler.cleanupCount())
		})
	}
}

func TestFunctionControllerReportsInitialRouteCleanupFailure(t *testing.T) {
	cleanupErr := errors.New("test cleanup")
	tests := map[string]struct {
		cleanup func(context.Context) error
		assert  func(*testing.T, error)
	}{
		"returned error": {
			cleanup: func(context.Context) error { return cleanupErr },
			assert: func(t *testing.T, err error) {
				require.ErrorIs(t, err, cleanupErr)
			},
		},
		"panic": {
			cleanup: func(context.Context) error {
				panic("test cleanup panic")
			},
			assert: func(t *testing.T, err error) {
				require.ErrorContains(t, err, "initial route cleanup panic: test cleanup panic")
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			generation := &HandlerGenerationDeclaration{
				ID: "initial",
				Handler: func(context.Context, HandlerInput) (lifecycle.SealedResult, error) {
					return lifecycle.SealedResult{}, nil
				},
				Cleanup: test.cleanup,
			}
			valid := InitialRoute{
				Declaration: Declaration{
					ID:         "initial",
					Generation: generation,
					PublicName: "initial",
				},
			}
			invalid := valid
			invalid.Declaration.PublicName = ""

			_, _, err := NewController(1, collectorapi.Registry{}, valid, invalid)
			require.ErrorContains(t, err, "invalid declaration")
			test.assert(t, err)
		})
	}
}

func TestFunctionControllerReportsUnpublishedGroupCleanupFailure(t *testing.T) {
	_, _, err := NewController(1, collectorapi.Registry{
		"a": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return &controllerPanickingCleanupHandler{}
			},
		},
		"z": {
			AgentFunctions: func() []funcapi.FunctionConfig {
				return []funcapi.FunctionConfig{{ID: "method"}}
			},
			MethodHandler: func(collectorapi.RuntimeJob) funcapi.MethodHandler {
				return nil
			},
		},
	})
	require.ErrorContains(t, err, "nil agent handler")
	require.ErrorIs(t, err, lifecycle.ErrTaskPanic)
	require.ErrorContains(t, err, "test handler cleanup panic")
}

type controllerTestMutationPort struct {
	catalog *Catalog
}

func (ctmp *controllerTestMutationPort) QuiesceFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if ctmp == nil || ctmp.catalog == nil {
		return errors.New("nil mutation port")
	}
	if err := ctmp.catalog.BeginMutation(mutation); err != nil {
		return err
	}
	for {
		progress, err := ctmp.catalog.AdvanceMutationQuiesce(jobmgr.MaximumFunctionMutationQuantum)
		if err != nil {
			return err
		}
		if progress.Quiesced {
			return nil
		}
	}
}

func (ctmp *controllerTestMutationPort) CommitFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) (uint64, error) {
	if ctmp == nil || ctmp.catalog == nil {
		return 0, errors.New("nil mutation port")
	}
	if err := ctmp.catalog.ResumeMutation(mutation); err != nil {
		return 0, err
	}
	for {
		progress, cleanups := ctmp.catalog.AdvanceMutation(jobmgr.MaximumFunctionMutationQuantum)
		for _, cleanup := range cleanups {
			_, cleanupErr := cleanup.Work()(context.Background())
			if err := ctmp.catalog.CompleteCleanup(cleanup.Ref()); err != nil {
				return 0, errors.Join(cleanupErr, err)
			}
		}
		if progress.Done {
			return progress.Version, nil
		}
	}
}

func (ctmp *controllerTestMutationPort) AbortFunctions(
	_ context.Context,
	mutation jobmgr.FunctionCatalogMutation,
) error {
	if ctmp == nil || ctmp.catalog == nil {
		return errors.New("nil mutation port")
	}
	return ctmp.catalog.AbortMutation(mutation)
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

type controllerPanickingCleanupHandler struct {
	controllerTestHandler
}

func (handler *controllerPanickingCleanupHandler) Cleanup(ctx context.Context) {
	handler.controllerTestHandler.Cleanup(ctx)
	panic("test handler cleanup panic")
}

type blockingWithdrawPublicationPort struct {
	*recordingPublicationPort
	entered chan struct{}
	release chan struct{}
}

func (bwpp *blockingWithdrawPublicationPort) Withdraw(name string) error {
	close(bwpp.entered)
	<-bwpp.release
	return bwpp.recordingPublicationPort.Withdraw(name)
}

func (cth *controllerTestHandler) MethodParams(context.Context, string) ([]funcapi.ParamConfig, error) {
	return nil, nil
}

func (cth *controllerTestHandler) Handle(context.Context, string, funcapi.ResolvedParams) *funcapi.FunctionResponse {
	return &funcapi.FunctionResponse{
		Status: 200,
	}
}

func (cth *controllerTestHandler) HandleRaw(
	_ context.Context,
	request funcapi.RawMethodRequest,
) *funcapi.FunctionResponse {
	cth.mu.Lock()
	cth.raw = request
	cth.mu.Unlock()
	return &funcapi.FunctionResponse{
		Status: 200,
	}
}

func (cth *controllerTestHandler) Cleanup(context.Context) {
	cth.mu.Lock()
	cth.cleanups++
	cth.mu.Unlock()
}

func (cth *controllerTestHandler) rawRequest() funcapi.RawMethodRequest {
	cth.mu.Lock()
	defer cth.mu.Unlock()
	return cth.raw
}

func (cth *controllerTestHandler) cleanupCount() int {
	cth.mu.Lock()
	defer cth.mu.Unlock()
	return cth.cleanups
}
