// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
)

func TestFunctionCatalogLookupLeaseSameTurn(t *testing.T) {
	tests := map[string]struct {
		declaration Declaration
		lookup      jobmgr.FunctionLookup
		wantStatus  lifecycle.ControlStatus
		wantScope   string
		wantMethod  string
	}{
		"direct route": {
			declaration: testDeclaration("direct", "", RouteLane()),
			lookup:      jobmgr.FunctionLookup{UID: "direct-uid", Route: "direct"},
			wantMethod:  "method",
		},
		"prefix argument lane": {
			declaration: testDeclaration("config", "job:", ArgumentLane(0)),
			lookup: jobmgr.FunctionLookup{
				UID: "prefix-uid", Route: "config", Args: []string{"job:mysql"},
			},
			wantScope:  "job:mysql",
			wantMethod: "method",
		},
		"DynCfg existing job lane": {
			declaration: testDeclaration(
				"config",
				"go.d:collector:",
				DynCfgJobLane(0, "go.d:collector:"),
			),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-update", Route: "config",
				Args: []string{
					"go.d:collector:mysql:production",
					"update",
				},
			},
			wantScope:  "mysql_production",
			wantMethod: "method",
		},
		"DynCfg add job lane": {
			declaration: testDeclaration(
				"config",
				"go.d:collector:",
				DynCfgJobLane(0, "go.d:collector:"),
			),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-add", Route: "config",
				Args: []string{
					"go.d:collector:mysql",
					"add",
					"production",
				},
			},
			wantScope:  "mysql_production",
			wantMethod: "method",
		},
		"scoped DynCfg existing resource lane": {
			declaration: testDeclaration(
				"config",
				"go.d:secretstore:",
				ScopedDynCfgJobLane(
					0,
					"go.d:secretstore:",
					"secretstore:",
				),
			),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-secret-update", Route: "config",
				Args: []string{
					"go.d:secretstore:vault:production",
					"update",
				},
			},
			wantScope:  "secretstore:vault_production",
			wantMethod: "method",
		},
		"scoped DynCfg add resource lane": {
			declaration: testDeclaration(
				"config",
				"go.d:vnode",
				ScopedDynCfgJobLane(0, "go.d:vnode", "vnode:"),
			),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-vnode-add", Route: "config",
				Args: []string{"go.d:vnode", "add", "production"},
			},
			wantScope:  "vnode:production",
			wantMethod: "method",
		},
		"prefix missing argument": {
			declaration: testDeclaration("config", "job:", ArgumentLane(0)),
			lookup:      jobmgr.FunctionLookup{UID: "missing-argument", Route: "config"},
			wantStatus:  lifecycle.ControlNotFound,
		},
		"unknown public route": {
			declaration: testDeclaration("direct", "", RouteLane()),
			lookup:      jobmgr.FunctionLookup{UID: "missing-route", Route: "missing"},
			wantStatus:  lifecycle.ControlNotFound,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var handled HandlerInput
			test.declaration.Generation.Handler = func(_ context.Context, input HandlerInput) (lifecycle.SealedResult, error) {
				handled = input
				return lifecycle.NewControlResult(lifecycle.ControlInternal)
			}
			catalog, err := NewCatalog([]Declaration{test.declaration})
			if err != nil {
				t.Fatal(err)
			}
			decision, err := catalog.ResolveAndAcquire(test.lookup)
			if err != nil {
				t.Fatal(err)
			}
			if decision.Rejected != test.wantStatus {
				t.Fatalf("status=%d, want %d", decision.Rejected, test.wantStatus)
			}
			if test.wantStatus != 0 {
				if decision.Lease.Valid() || decision.Plan.Runner != nil {
					t.Fatalf("rejection owns work or lease: %+v", decision)
				}
				return
			}
			if !decision.Lease.Valid() || decision.Plan.Runner == nil ||
				decision.Lane.Route == 0 || decision.Lane.Scope != test.wantScope {
				t.Fatalf("resolved decision differs: %+v", decision)
			}
			if census := catalog.Census(); census.InvocationLeases != 1 {
				t.Fatalf("lookup and lease did not linearize together: %+v", census)
			}
			outcome, err := decision.Plan.Runner.RunTask(context.Background())
			if err != nil {
				t.Fatal(err)
			}
			if outcome.Kind() != lifecycle.TaskOutcomeFrame || handled.UID != test.lookup.UID ||
				handled.Method != test.wantMethod {
				t.Fatalf("handler input/outcome differs: input=%+v kind=%d", handled, outcome.Kind())
			}
			cleanup, err := catalog.ReleaseInvocation(decision.Lease)
			if err != nil {
				t.Fatal(err)
			}
			if cleanup.Ref.Valid() {
				t.Fatalf("live handler unexpectedly requested cleanup: %+v", cleanup)
			}
			if census := catalog.Census(); census.InvocationLeases != 0 {
				t.Fatalf("release retained invocation: %+v", census)
			}
			if _, err := catalog.ReleaseInvocation(decision.Lease); err == nil {
				t.Fatal("duplicate invocation release was accepted")
			}
		})
	}
}

func TestFunctionPayloadValidationRunsInTaskChild(t *testing.T) {
	var calls atomic.Int32
	declaration := testDeclaration("direct", "", RouteLane())
	declaration.Generation.Handler = func(context.Context, HandlerInput) (lifecycle.SealedResult, error) {
		calls.Add(1)
		return lifecycle.NewControlResult(lifecycle.ControlInternal)
	}
	catalog, err := NewCatalog([]Declaration{declaration})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "invalid-json", Route: "direct", HasPayload: true, Payload: []byte("{"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if calls.Load() != 0 {
		t.Fatal("lookup invoked the Function handler")
	}
	outcome, err := decision.Plan.Runner.RunTask(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind() != lifecycle.TaskOutcomeFrame || calls.Load() != 0 {
		t.Fatalf("invalid JSON reached handler or returned wrong outcome: kind=%d calls=%d",
			outcome.Kind(), calls.Load())
	}
	if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
		t.Fatal(err)
	}
}

func TestFunctionCatalogReturnsSealedResourceTransactionPlan(t *testing.T) {
	permit, err := lifecycle.NewJobLongLivedPlan(4096)
	if err != nil {
		t.Fatal(err)
	}
	tests := map[string]struct {
		command           string
		allocateSuccessor bool
	}{
		"update allocates successor": {
			command: "update", allocateSuccessor: true,
		},
		"remove has no successor": {
			command: "remove",
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			declaration := testDeclaration(
				"config",
				"job:",
				ArgumentLane(0),
			)
			var preparedInput HandlerInput
			declaration.Transaction = &ResourceTransactionDeclaration{
				Prepare: func(
					_ context.Context,
					input HandlerInput,
					_ lifecycle.ReadyResource,
					_ lifecycle.ResourceTransactionScope,
					_ lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					preparedInput = input
					return nil, nil
				},
				Permit:          permit,
				CommandArgument: 1,
				GlobalClaim:     "dyncfg:graph",
				Commands: []ResourceTransactionCommand{
					{Name: "update", AllocateSuccessor: true},
					{Name: "remove"},
				},
			}
			catalog, err := NewCatalog([]Declaration{declaration})
			if err != nil {
				t.Fatal(err)
			}
			lookup := jobmgr.FunctionLookup{
				UID:        "transaction",
				Route:      "config",
				Args:       []string{"job:mysql", test.command},
				Payload:    []byte(`{"option":true}`),
				HasPayload: true,
			}
			decision, err := catalog.ResolveAndAcquire(lookup)
			if err != nil {
				t.Fatal(err)
			}
			plan := decision.Plan.Transaction
			if plan == nil ||
				decision.Plan.Runner != nil ||
				plan.ID != "job:mysql" ||
				plan.AllocateSuccessor != test.allocateSuccessor ||
				len(decision.Plan.Claims) != 1 ||
				decision.Plan.Claims[0] != "dyncfg:graph" {
				t.Fatalf("transaction decision=%+v", decision)
			}
			if test.allocateSuccessor {
				if err := plan.Permit.Validate(); err != nil {
					t.Fatal(err)
				}
			} else if plan.Permit.Class() != 0 ||
				plan.Permit.Bytes() != 0 {
				t.Fatal("remove transaction retained a successor permit")
			}
			if _, err := plan.Prepare(
				context.Background(),
				nil,
				lifecycle.ResourceTransactionScope{},
				lifecycle.LongLivedPermit{},
			); err != nil {
				t.Fatal(err)
			}
			if preparedInput.UID != lookup.UID ||
				!reflect.DeepEqual(preparedInput.Args, lookup.Args) ||
				!reflect.DeepEqual(preparedInput.Payload, lookup.Payload) {
				t.Fatalf("prepared input=%+v, lookup=%+v", preparedInput, lookup)
			}
			if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestHandlerLeaseLifecycle(t *testing.T) {
	var cleanupCalls atomic.Int32
	declaration := testDeclaration("direct", "", RouteLane())
	declaration.Generation.Cleanup = func(context.Context) error {
		cleanupCalls.Add(1)
		return nil
	}
	catalog, err := NewCatalog([]Declaration{declaration})
	if err != nil {
		t.Fatal(err)
	}
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "held", Route: "direct",
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.BeginClose(); err != nil {
		t.Fatal(err)
	}
	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	count, more, err := catalog.CloseStep(1, &cleanups)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || more || cleanupCalls.Load() != 0 {
		t.Fatalf("close bypassed held invocation: count=%d more=%v calls=%d",
			count, more, cleanupCalls.Load())
	}
	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	if err != nil {
		t.Fatal(err)
	}
	if !cleanup.Ref.Valid() || cleanup.Runner == nil {
		t.Fatalf("drained handler did not return cleanup work: %+v", cleanup)
	}
	if cleanupCalls.Load() != 0 {
		t.Fatal("lease release invoked Cleanup on KernelLoop")
	}
	outcome, err := cleanup.Runner.RunTask(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if outcome.Kind() != lifecycle.TaskOutcomeNone || cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup work differs: kind=%d calls=%d", outcome.Kind(), cleanupCalls.Load())
	}
	if err := catalog.CompleteCleanup(cleanup.Ref, nil); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteCleanup(cleanup.Ref, nil); err == nil {
		t.Fatal("duplicate cleanup completion was accepted")
	}
	if census := catalog.Census(); census.PendingCleanups != 0 ||
		census.CompletedCleanups != 1 || census.FailedCleanups != 0 {
		t.Fatalf("cleanup census differs: %+v", census)
	}
}

func TestHandlerCleanupOnce(t *testing.T) {
	var cleanupCalls atomic.Int32
	generation := testGeneration("config-generation")
	generation.Cleanup = func(context.Context) error {
		cleanupCalls.Add(1)
		return nil
	}
	declarations := []Declaration{
		testDeclarationForGeneration(generation, "config", "job:", ArgumentLane(0)),
		testDeclarationForGeneration(generation, "config", "store:", ArgumentLane(0)),
	}
	catalog, err := NewCatalog(declarations)
	if err != nil {
		t.Fatal(err)
	}
	if err := catalog.BeginClose(); err != nil {
		t.Fatal(err)
	}
	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	total := 0
	for {
		count, more, err := catalog.CloseStep(1, &cleanups)
		if err != nil {
			t.Fatal(err)
		}
		for _, cleanup := range cleanups[:count] {
			if _, err := cleanup.Runner.RunTask(context.Background()); err != nil {
				t.Fatal(err)
			}
			if err := catalog.CompleteCleanup(cleanup.Ref, nil); err != nil {
				t.Fatal(err)
			}
			total++
		}
		if !more {
			break
		}
	}
	if total != 1 || cleanupCalls.Load() != 1 {
		t.Fatalf("cleanup count=%d calls=%d, want one generation cleanup", total, cleanupCalls.Load())
	}
	if census := catalog.Census(); census.Routes != 0 || census.CloseRoutesPending != 0 ||
		census.CompletedCleanups != 1 {
		t.Fatalf("closed catalog census differs: %+v", census)
	}
}

func TestFunctionCatalogAtomicMutation(t *testing.T) {
	var oldCleanups atomic.Int32
	old := testGeneration("old-generation")
	old.Cleanup = func(context.Context) error {
		oldCleanups.Add(1)
		return nil
	}
	catalog, err := NewCatalog([]Declaration{
		testDeclarationForGeneration(old, "work", "", RouteLane()),
	})
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := catalog.Census().Version
	oldDecision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "old", Route: "work"})
	if err != nil {
		t.Fatal(err)
	}

	short := testDeclarationForGeneration(testGeneration("short"), "config", "collector:", ArgumentLane(0))
	long := testDeclarationForGeneration(testGeneration("long"), "config", "collector:job:", ArgumentLane(0))
	invalid, err := catalog.NewMutation(oldVersion, []RouteChange{
		{PublicName: short.PublicName, Prefix: short.Prefix, Declaration: &short},
		{PublicName: long.PublicName, Prefix: long.Prefix, Declaration: &long},
	})
	if err != nil {
		t.Fatal(err)
	}
	invalidBuilder, err := catalog.startMutation(invalid)
	if err != nil {
		t.Fatal(err)
	}
	var mutationCleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
	for {
		_, _, prepareErr := invalidBuilder.PrepareStep(3)
		if catalog.Census().Version != oldVersion || resolvedMethod(catalog, "work", nil) != "method" {
			t.Fatal("private or failed mutation changed visible catalog state")
		}
		if prepareErr != nil {
			break
		}
	}
	if count, err := invalidBuilder.Abort(&mutationCleanups); err != nil || count != 0 {
		t.Fatalf("invalid topology constructed private handlers: count=%d err=%v", count, err)
	}

	var newCleanups atomic.Int32
	replacementGeneration := testGeneration("new-generation")
	replacementGeneration.Cleanup = func(context.Context) error {
		newCleanups.Add(1)
		return nil
	}
	replacement := testDeclarationForGeneration(replacementGeneration, "work", "", RouteLane())
	mutation, err := catalog.NewMutation(oldVersion, []RouteChange{{
		PublicName: "work", Declaration: &replacement,
	}})
	if err != nil {
		t.Fatal(err)
	}
	builder, err := catalog.startMutation(mutation)
	if err != nil {
		t.Fatal(err)
	}
	var postimage *MutationPostimage
	for {
		var done bool
		postimage, done, err = builder.PrepareStep(3)
		if err != nil {
			t.Fatal(err)
		}
		if catalog.Census().Version != oldVersion || resolvedMethod(catalog, "work", nil) != "method" {
			t.Fatal("replacement became visible before commit")
		}
		if done {
			break
		}
	}
	count, err := catalog.commitMutation(postimage, &mutationCleanups)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || catalog.Census().Version != oldVersion+1 ||
		resolvedMethod(catalog, "work", nil) != "method" {
		t.Fatalf("atomic commit differs: cleanup=%d census=%+v", count, catalog.Census())
	}
	if oldCleanups.Load() != 0 {
		t.Fatal("retired handler cleaned before its invocation lease drained")
	}
	oldCleanup, err := catalog.ReleaseInvocation(oldDecision.Lease)
	if err != nil {
		t.Fatal(err)
	}
	runCleanupPlan(t, catalog, oldCleanup)
	if oldCleanups.Load() != 1 || newCleanups.Load() != 0 {
		t.Fatalf("generation cleanup differs: old=%d new=%d", oldCleanups.Load(), newCleanups.Load())
	}
}

func TestFunctionCatalogBoundedMutationTurns(t *testing.T) {
	const quantum = 7
	const unrelatedRoutes = 192
	type result struct {
		total int
		turns int
	}
	run := func(t *testing.T, population int) result {
		t.Helper()
		declarations := make([]Declaration, 0, population)
		for index := 0; index < population; index++ {
			name := fmt.Sprintf("unrelated-%03d", index)
			declarations = append(declarations, testDeclaration(name, "", RouteLane()))
		}
		catalog, err := NewCatalog(declarations)
		if err != nil {
			t.Fatal(err)
		}
		prefix := strings.Repeat("p", 128)
		declaration := testDeclaration("config", prefix, ArgumentLane(0))
		mutation, err := catalog.NewMutation(catalog.Census().Version, []RouteChange{{
			PublicName: declaration.PublicName, Prefix: declaration.Prefix, Declaration: &declaration,
		}})
		if err != nil {
			t.Fatal(err)
		}
		builder, err := catalog.startMutation(mutation)
		if err != nil {
			t.Fatal(err)
		}
		previous := builder.Progress()
		turns := 0
		for {
			_, done, err := builder.PrepareStep(quantum)
			if err != nil {
				t.Fatal(err)
			}
			progress := builder.Progress()
			delta := progress.CompletedNodes - previous.CompletedNodes
			if delta <= 0 || delta > quantum || progress.LastStepNodes != delta {
				t.Fatalf("mutation turn work=%d last=%d, want 1..%d", delta, progress.LastStepNodes, quantum)
			}
			if _, ok := catalogRouteSet(catalog.routes, "config"); ok {
				t.Fatal("private postimage became visible during bounded preparation")
			}
			previous = progress
			turns++
			if done {
				if progress.CompletedNodes != progress.TotalNodes {
					t.Fatalf("completed work=%d total=%d", progress.CompletedNodes, progress.TotalNodes)
				}
				break
			}
		}
		var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
		if count, err := builder.Abort(&cleanups); err != nil || count != 0 {
			t.Fatalf("mutation abort count=%d err=%v", count, err)
		}
		return result{total: previous.TotalNodes, turns: turns}
	}

	small := run(t, 0)
	large := run(t, unrelatedRoutes)
	if small != large {
		t.Fatalf("mutation work scaled with total catalog population: small=%+v large=%+v", small, large)
	}
}

func TestCatalogRejectsInvalidDeclarations(t *testing.T) {
	tests := map[string]Declaration{
		"missing handler": {
			ID: "method", Generation: &HandlerGenerationDeclaration{ID: "generation"},
			PublicName: "direct", Lane: RouteLane(),
		},
		"unknown lane": {
			ID: "method", Generation: testGeneration("generation"),
			PublicName: "direct",
		},
		"direct argument lane": {
			ID: "method", Generation: testGeneration("generation"),
			PublicName: "direct", Lane: ArgumentLane(0),
		},
		"duplicate direct": testDeclaration("direct", "", RouteLane()),
	}
	for name, declaration := range tests {
		t.Run(name, func(t *testing.T) {
			declarations := []Declaration{declaration}
			if name == "duplicate direct" {
				declarations = append(declarations, declaration)
			}
			if _, err := NewCatalog(declarations); err == nil {
				t.Fatal("invalid declaration was accepted")
			}
		})
	}
}

func TestCatalogRejectsPathStorageBeyondProcessBudgetBeforePublication(t *testing.T) {
	prefix := strings.Repeat("p", maximumDeclarationMetadataBytes)
	tests := map[string]func() error{
		"initial catalog": func() error {
			var declarations []Declaration
			for index := 0; index < 5; index++ {
				name := fmt.Sprintf(
					"%d%s",
					index,
					strings.Repeat(
						"n",
						maximumDeclarationMetadataBytes-1,
					),
				)
				declarations = append(
					declarations,
					testDeclaration(name, prefix, ArgumentLane(0)),
				)
			}
			_, err := NewCatalog(declarations)
			return err
		},
		"mutation postimage": func() error {
			catalog, err := NewCatalog(nil)
			if err != nil {
				return err
			}
			var changes []RouteChange
			for index := 0; index < 3; index++ {
				name := fmt.Sprintf(
					"%d%s",
					index,
					strings.Repeat(
						"n",
						maximumDeclarationMetadataBytes-1,
					),
				)
				declaration := testDeclaration(
					name,
					prefix,
					ArgumentLane(0),
				)
				changes = append(changes, RouteChange{
					PublicName:  name,
					Prefix:      prefix,
					Declaration: &declaration,
				})
			}
			_, err = catalog.NewMutation(1, changes)
			return err
		},
	}
	for name, run := range tests {
		t.Run(name, func(t *testing.T) {
			if err := run(); err == nil {
				t.Fatal("path storage beyond the process budget was accepted")
			}
		})
	}
}

func TestCatalogPathStorageReturnsToPublishedPostimageAcrossChurn(t *testing.T) {
	catalog, err := NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
	publicName := strings.Repeat("n", 1_024)
	prefix := strings.Repeat("p", 1_024)
	apply := func(change RouteChange) {
		t.Helper()
		mutation, err := catalog.NewMutation(
			catalog.Census().Version,
			[]RouteChange{change},
		)
		if err != nil {
			t.Fatal(err)
		}
		builder, err := catalog.startMutation(mutation)
		if err != nil {
			t.Fatal(err)
		}
		var postimage *MutationPostimage
		for {
			var done bool
			postimage, done, err = builder.PrepareStep(
				MaximumMutationQuantum,
			)
			if err != nil {
				t.Fatal(err)
			}
			if done {
				break
			}
		}
		var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
		count, err := catalog.commitMutation(postimage, &cleanups)
		if err != nil {
			t.Fatal(err)
		}
		for index := 0; index < count; index++ {
			runCleanupPlan(t, catalog, cleanups[index])
		}
	}

	for iteration := 0; iteration < 8; iteration++ {
		declaration := testDeclaration(
			publicName,
			prefix,
			ArgumentLane(0),
		)
		apply(RouteChange{
			PublicName:  publicName,
			Prefix:      prefix,
			Declaration: &declaration,
		})
		apply(RouteChange{
			PublicName: publicName,
			Prefix:     prefix,
		})
		if published := catalog.storage.published.Load(); published != 0 {
			t.Fatalf(
				"iteration %d retained published path bytes: %d",
				iteration,
				published,
			)
		}
		if total := catalog.storage.total.Load(); total != 0 {
			t.Fatalf(
				"iteration %d retained total path bytes: %d",
				iteration,
				total,
			)
		}
		if catalog.storage.preparation.Load() {
			t.Fatalf(
				"iteration %d retained a mutation reservation",
				iteration,
			)
		}
	}
}

func BenchmarkBFunctionCatalogLookup(b *testing.B) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", RouteLane())})
	if err != nil {
		b.Fatal(err)
	}
	lookup := jobmgr.FunctionLookup{UID: "benchmark", Route: "direct"}
	b.ReportAllocs()
	for b.Loop() {
		decision, err := catalog.ResolveAndAcquire(lookup)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBHandlerLease(b *testing.B) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", RouteLane())})
	if err != nil {
		b.Fatal(err)
	}
	lookup := jobmgr.FunctionLookup{UID: "handler-lease", Route: "direct"}
	b.ReportAllocs()
	for b.Loop() {
		decision, err := catalog.ResolveAndAcquire(lookup)
		if err != nil {
			b.Fatal(err)
		}
		if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
			b.Fatal(err)
		}
	}
}

func TestFunctionCatalogLookupAllocations(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", RouteLane())})
	if err != nil {
		t.Fatal(err)
	}
	lookup := jobmgr.FunctionLookup{UID: "allocations", Route: "direct"}
	allocations := testing.AllocsPerRun(1_000, func() {
		decision, resolveErr := catalog.ResolveAndAcquire(lookup)
		if resolveErr != nil {
			panic(resolveErr)
		}
		if _, releaseErr := catalog.ReleaseInvocation(decision.Lease); releaseErr != nil {
			panic(releaseErr)
		}
	})
	if allocations != 0 {
		t.Fatalf("lookup+lease allocations=%f, want 0", allocations)
	}
}

func testDeclaration(publicName, prefix string, lane LanePolicy) Declaration {
	return testDeclarationForGeneration(testGeneration(publicName), publicName, prefix, lane)
}

func testGeneration(id string) *HandlerGenerationDeclaration {
	return &HandlerGenerationDeclaration{ID: id, Handler: testHandler}
}

func testDeclarationForGeneration(generation *HandlerGenerationDeclaration, publicName, prefix string, lane LanePolicy) Declaration {
	return Declaration{
		ID: "method", Generation: generation, PublicName: publicName, Prefix: prefix, Lane: lane,
		CooperativeCancel: true, CooperativeDeadline: true,
	}
}

func testHandler(context.Context, HandlerInput) (lifecycle.SealedResult, error) {
	return lifecycle.NewControlResult(lifecycle.ControlInternal)
}

func resolvedMethod(catalog *Catalog, publicName string, args []string) string {
	set, ok := catalogRouteSet(catalog.routes, publicName)
	if !ok {
		return ""
	}
	resolved := set.resolve(args)
	if resolved == nil {
		return ""
	}
	return resolved.method
}

func runCleanupPlan(t *testing.T, catalog *Catalog, cleanup jobmgr.FunctionCleanupPlan) {
	t.Helper()
	if !cleanup.Ref.Valid() || cleanup.Runner == nil {
		t.Fatalf("invalid cleanup plan: %+v", cleanup)
	}
	if _, err := cleanup.Runner.RunTask(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := catalog.CompleteCleanup(cleanup.Ref, nil); err != nil {
		t.Fatal(err)
	}
}
