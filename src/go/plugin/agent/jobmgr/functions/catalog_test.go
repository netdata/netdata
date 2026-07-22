// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr"
	"github.com/netdata/netdata/go/plugins/plugin/agent/jobmgr/lifecycle"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFunctionCatalogLookup(t *testing.T) {
	tests := map[string]struct {
		declaration  Declaration
		lookup       jobmgr.FunctionLookup
		wantStatus   lifecycle.ControlStatus
		wantResource string
	}{
		"direct route": {
			declaration: testDeclaration("direct", "", ResourcePolicy{}),
			lookup:      jobmgr.FunctionLookup{UID: "direct", Route: "direct"},
		},
		"prefix route": {
			declaration: testDeclaration("config", "job:", ResourcePolicy{}),
			lookup:      jobmgr.FunctionLookup{UID: "prefix", Route: "config", Args: []string{"job:mysql"}},
		},
		"DynCfg resource": {
			declaration: testDeclaration("config", "go.d:collector:", DynCfgJobResource(0, "go.d:collector:")),
			lookup: jobmgr.FunctionLookup{
				UID: "resource", Route: "config",
				Args: []string{"go.d:collector:mysql:production", "update"},
			},
			wantResource: "mysql_production",
		},
		"missing prefix argument": {
			declaration: testDeclaration("config", "job:", ResourcePolicy{}),
			lookup:      jobmgr.FunctionLookup{UID: "missing-arg", Route: "config"},
			wantStatus:  lifecycle.ControlNotFound,
		},
		"unknown route": {
			declaration: testDeclaration("direct", "", ResourcePolicy{}),
			lookup:      jobmgr.FunctionLookup{UID: "missing", Route: "unknown"},
			wantStatus:  lifecycle.ControlNotFound,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var handled HandlerInput
			test.declaration.Generation.Handler = func(
				_ context.Context,
				input HandlerInput,
			) (lifecycle.SealedResult, error) {
				handled = input
				return lifecycle.NewControlResult(lifecycle.ControlInternal)
			}
			catalog, err := NewCatalog([]Declaration{test.declaration})
			require.NoError(t, err)

			decision, err := catalog.ResolveAndAcquire(test.lookup)
			require.NoError(t, err)
			require.Equal(t, test.wantStatus, decision.Rejected)
			if test.wantStatus != 0 {
				assert.False(t, decision.Lease.Valid())
				assert.Nil(t, decision.Plan.Work)
				return
			}
			require.True(t, decision.Lease.Valid())
			require.NotNil(t, decision.Plan.Work)
			assert.Equal(t, test.wantResource, decision.ResourceID)

			outcome, err := decision.Plan.Work(context.Background())
			require.NoError(t, err)
			assert.Equal(t, lifecycle.TaskOutcomeFrame, outcome.Kind())
			assert.Equal(t, test.lookup.UID, handled.UID)
			assert.Equal(t, "method", handled.Method)

			cleanup, err := catalog.ReleaseInvocation(decision.Lease)
			require.NoError(t, err)
			assert.False(t, cleanup.Valid())
			_, err = catalog.ReleaseInvocation(decision.Lease)
			require.Error(t, err)
		})
	}
}

func TestFunctionCatalogInitialPrefixValidation(t *testing.T) {
	tests := map[string]struct {
		prefixes []string
		wantErr  bool
	}{
		"distinct prefixes": {prefixes: []string{"job:", "module:"}},
		"duplicate prefix":  {prefixes: []string{"job:", "job:"}, wantErr: true},
		"shorter overlap":   {prefixes: []string{"job:", "job:mysql:"}, wantErr: true},
		"longer overlap":    {prefixes: []string{"job:mysql:", "job:"}, wantErr: true},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			declarations := make([]Declaration, 0, len(test.prefixes))
			for index, prefix := range test.prefixes {
				declaration := testDeclaration("config", prefix, ResourcePolicy{})
				declaration.ID = fmt.Sprintf("method-%d", index)
				declaration.Generation.ID = fmt.Sprintf("generation-%d", index)
				declarations = append(declarations, declaration)
			}
			_, err := NewCatalog(declarations)
			if test.wantErr {
				require.Error(t, err)
			} else {
				require.NoError(t, err)
			}
		})
	}
}

func TestFunctionPayloadValidationRunsInTask(t *testing.T) {
	var calls atomic.Int32
	declaration := testDeclaration("direct", "", ResourcePolicy{})
	declaration.Generation.Handler = func(context.Context, HandlerInput) (lifecycle.SealedResult, error) {
		calls.Add(1)
		return lifecycle.NewControlResult(lifecycle.ControlInternal)
	}
	catalog, err := NewCatalog([]Declaration{declaration})
	require.NoError(t, err)

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "invalid-json", Route: "direct", HasPayload: true, Payload: []byte("{"),
	})
	require.NoError(t, err)
	assert.Zero(t, calls.Load())

	outcome, err := decision.Plan.Work(context.Background())
	require.NoError(t, err)
	assert.Equal(t, lifecycle.TaskOutcomeFrame, outcome.Kind())
	assert.Zero(t, calls.Load())
	_, err = catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
}

func TestFunctionCatalogResourceTransactionHasNoArbitraryCountLimits(t *testing.T) {
	const commandArgument = 1_024
	commands := make([]ResourceTransactionCommand, 20)
	for index := range commands {
		claims := make([]string, 20)
		for claimIndex := range claims {
			claims[claimIndex] = fmt.Sprintf("claim-%02d-%02d", index, claimIndex)
		}
		commands[index] = ResourceTransactionCommand{Name: fmt.Sprintf("command-%02d", index), Claims: claims}
	}
	declaration := testDeclaration("config", "", DynCfgJobResource(0, "job:"))
	declaration.Transaction = &ResourceTransactionDeclaration{
		Prepare: func(
			context.Context,
			HandlerInput,
			lifecycle.ReadyResource,
			lifecycle.ResourceTransactionScope,
			lifecycle.LongLivedPermit,
		) (lifecycle.PreparedResourceTransaction, error) {
			return nil, nil
		},
		CommandArgument: commandArgument,
		GlobalClaim:     "global",
		Commands:        commands,
	}
	catalog, err := NewCatalog([]Declaration{declaration})
	require.NoError(t, err)
	arguments := make([]string, commandArgument+1)
	arguments[0] = "job:mysql"
	arguments[commandArgument] = "command-19"

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "transaction", Route: "config", Args: arguments,
	})
	require.NoError(t, err)
	require.NotNil(t, decision.Plan.Transaction)
	assert.Equal(t, "mysql", decision.ResourceID)
	assert.Len(t, decision.Plan.Claims, 21)
	assert.Equal(t, "global", decision.Plan.Claims[0])
	_, err = catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
}

func TestFunctionCatalogMutationLifecycle(t *testing.T) {
	tests := map[string]struct {
		change        func() RouteChange
		wantOldStatus lifecycle.ControlStatus
		wantNewMethod string
	}{
		"replace": {
			change: func() RouteChange {
				replacement := testDeclaration("work", "", ResourcePolicy{})
				replacement.ID = "replacement"
				return RouteChange{PublicName: "work", Declaration: &replacement}
			},
			wantOldStatus: lifecycle.ControlUnavailable,
			wantNewMethod: "replacement",
		},
		"remove": {
			change: func() RouteChange {
				return RouteChange{PublicName: "work"}
			},
			wantOldStatus: lifecycle.ControlUnavailable,
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			catalog, err := NewCatalog([]Declaration{
				testDeclaration("work", "", ResourcePolicy{}),
				testDeclaration("other", "", ResourcePolicy{}),
			})
			require.NoError(t, err)
			change := test.change()
			var handledMethod string
			if change.Declaration != nil {
				change.Declaration.Generation.Handler = func(
					_ context.Context,
					input HandlerInput,
				) (lifecycle.SealedResult, error) {
					handledMethod = input.Method
					return lifecycle.NewControlResult(lifecycle.ControlInternal)
				}
			}
			mutation, err := catalog.NewMutation(1, []RouteChange{change})
			require.NoError(t, err)
			require.NoError(t, catalog.BeginMutation(mutation))

			progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
			require.NoError(t, err)
			require.True(t, progress.Quiesced)
			oldDecision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "old", Route: "work"})
			require.NoError(t, err)
			assert.Equal(t, test.wantOldStatus, oldDecision.Rejected)
			otherDecision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "other", Route: "other"})
			require.NoError(t, err)
			require.True(t, otherDecision.Lease.Valid())
			_, err = catalog.ReleaseInvocation(otherDecision.Lease)
			require.NoError(t, err)

			require.NoError(t, catalog.ResumeMutation(mutation))
			progress, cleanups := catalog.AdvanceMutation(MaximumMutationQuantum)
			require.True(t, progress.Done)
			assert.Empty(t, cleanups)
			assert.EqualValues(t, 2, progress.Version)

			decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "new", Route: "work"})
			require.NoError(t, err)
			if test.wantNewMethod == "" {
				assert.Equal(t, lifecycle.ControlNotFound, decision.Rejected)
				return
			}
			require.True(t, decision.Lease.Valid())
			_, err = decision.Plan.Work(context.Background())
			require.NoError(t, err)
			assert.Equal(t, test.wantNewMethod, handledMethod)
			_, err = catalog.ReleaseInvocation(decision.Lease)
			require.NoError(t, err)
		})
	}
}

func TestFunctionCatalogMutationAbortRestoresAdmission(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("work", "", ResourcePolicy{})})
	require.NoError(t, err)
	replacement := testDeclaration("work", "", ResourcePolicy{})
	replacement.ID = "replacement"
	mutation, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work", Declaration: &replacement}})
	require.NoError(t, err)
	require.NoError(t, catalog.BeginMutation(mutation))
	_, err = catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
	require.NoError(t, err)
	require.NoError(t, catalog.AbortMutation(mutation))

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "old", Route: "work"})
	require.NoError(t, err)
	require.True(t, decision.Lease.Valid())
	assert.EqualValues(t, 1, catalog.census().Version)
	_, err = catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
}

func TestFunctionCatalogRejectsForeignMutationAbort(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("work", "", ResourcePolicy{})})
	require.NoError(t, err)
	firstReplacement := testDeclaration("work", "", ResourcePolicy{})
	secondReplacement := testDeclaration("work", "", ResourcePolicy{})
	first, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work", Declaration: &firstReplacement}})
	require.NoError(t, err)
	second, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work", Declaration: &secondReplacement}})
	require.NoError(t, err)
	defer func() { require.NoError(t, second.Discard()) }()
	require.NoError(t, catalog.BeginMutation(first))
	_, err = catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
	require.NoError(t, err)

	require.Error(t, catalog.AbortMutation(second))
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "blocked", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlUnavailable, decision.Rejected)
	require.NoError(t, catalog.AbortMutation(first))
}

func TestFunctionCatalogMutationRejectsDynamicPrefix(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	declaration := testDeclaration("config", "job:", ResourcePolicy{})
	_, err = catalog.NewMutation(1, []RouteChange{{PublicName: "config", Declaration: &declaration}})
	require.Error(t, err)
}

func TestFunctionCatalogMutationExceedsFormerCountLimit(t *testing.T) {
	const population = 300
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	changes := make([]RouteChange, 0, population)
	for index := range population {
		name := fmt.Sprintf("work-%03d", index)
		declaration := testDeclaration(name, "", ResourcePolicy{})
		changes = append(changes, RouteChange{PublicName: name, Declaration: &declaration})
	}
	mutation, err := catalog.NewMutation(1, changes)
	require.NoError(t, err)
	require.NoError(t, catalog.BeginMutation(mutation))
	preflightTurns := 0
	for {
		preflightTurns++
		progress, preflightErr := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
		require.NoError(t, preflightErr)
		if progress.Quiesced {
			break
		}
	}
	assert.Equal(t, 5, preflightTurns)
	require.NoError(t, catalog.ResumeMutation(mutation))

	turns := 0
	for {
		turns++
		progress, cleanups := catalog.AdvanceMutation(MaximumMutationQuantum)
		assert.Empty(t, cleanups)
		if progress.Done {
			break
		}
		decision, resolveErr := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
			UID: "pre-commit", Route: "work-000",
		})
		require.NoError(t, resolveErr)
		assert.Equal(t, lifecycle.ControlNotFound, decision.Rejected)
	}
	assert.Equal(t, 5, turns)
	assert.Equal(t, population, catalog.census().Routes)
}

func TestFunctionCatalogMutationPreflightIsBoundedAndClosesAdmission(t *testing.T) {
	const population = MaximumMutationQuantum + 1
	declarations := make([]Declaration, 0, population)
	changes := make([]RouteChange, 0, population)
	for index := range population {
		name := fmt.Sprintf("work-%03d", index)
		declarations = append(declarations, testDeclaration(name, "", ResourcePolicy{}))
		changes = append(changes, RouteChange{PublicName: name})
	}
	catalog, err := NewCatalog(declarations)
	require.NoError(t, err)
	mutation, err := catalog.NewMutation(1, changes)
	require.NoError(t, err)
	require.NoError(t, catalog.BeginMutation(mutation))

	progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
	require.NoError(t, err)
	assert.False(t, progress.Quiesced)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "preflight", Route: "work-000"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlUnavailable, decision.Rejected)
	require.Error(t, catalog.ResumeMutation(mutation))

	preflightTurns := 1
	for !progress.Quiesced {
		preflightTurns++
		progress, err = catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
		require.NoError(t, err)
	}
	assert.Equal(t, 3, preflightTurns)
	require.NoError(t, catalog.ResumeMutation(mutation))

	commitTurns := 0
	for {
		commitTurns++
		progress, cleanups := catalog.AdvanceMutation(MaximumMutationQuantum)
		assert.Empty(t, cleanups)
		if progress.Done {
			break
		}
	}
	assert.Equal(t, 2, commitTurns)
}

func TestFunctionCatalogRejectsStalePreparedMutation(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	firstDeclaration := testDeclaration("first", "", ResourcePolicy{})
	secondDeclaration := testDeclaration("second", "", ResourcePolicy{})
	first, err := catalog.NewMutation(1, []RouteChange{{PublicName: "first", Declaration: &firstDeclaration}})
	require.NoError(t, err)
	second, err := catalog.NewMutation(1, []RouteChange{{PublicName: "second", Declaration: &secondDeclaration}})
	require.NoError(t, err)
	commitMutation(t, catalog, first)
	require.Error(t, catalog.BeginMutation(second))
}

func TestFunctionCatalogMutationPreflightIsSideEffectFree(t *testing.T) {
	tests := map[string]struct {
		corrupt func(*Catalog, *handlerGeneration)
	}{
		"route reference underflow": {
			corrupt: func(_ *Catalog, generation *handlerGeneration) {
				generation.routeReferences = 0
			},
		},
		"cleanup dispatch collision": {
			corrupt: func(catalog *Catalog, generation *handlerGeneration) {
				catalog.cleanupByRef[generation.cleanupRef] = &handlerGeneration{}
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			first := testDeclaration("first", "", ResourcePolicy{})
			first.Generation.Cleanup = func(context.Context) error { return nil }
			second := testDeclaration("second", "", ResourcePolicy{})
			second.Generation.Cleanup = func(context.Context) error { return nil }
			catalog, err := NewCatalog([]Declaration{first, second})
			require.NoError(t, err)
			snapshot := catalog.snapshot.Load()
			firstGeneration := snapshot.routes["first"].direct.handler
			secondGeneration := snapshot.routes["second"].direct.handler

			mutation, err := catalog.NewMutation(1, []RouteChange{{PublicName: "first"}, {PublicName: "second"}})
			require.NoError(t, err)
			test.corrupt(catalog, secondGeneration)
			require.NoError(t, catalog.BeginMutation(mutation))

			progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
			require.Error(t, err)
			assert.False(t, progress.Quiesced)
			assert.Equal(t, 1, firstGeneration.routeReferences)
			assert.Zero(t, catalog.pendingCleanups)
			require.NoError(t, catalog.AbortMutation(mutation))
		})
	}
}

func TestFunctionCatalogMutationPreflightAggregatesSharedGeneration(t *testing.T) {
	generation := testGeneration("shared")
	catalog, err := NewCatalog([]Declaration{
		testDeclarationForGeneration(generation, "first", "", ResourcePolicy{}),
		testDeclarationForGeneration(generation, "second", "", ResourcePolicy{}),
	})
	require.NoError(t, err)
	mutation, err := catalog.NewMutation(1, []RouteChange{{PublicName: "first"}, {PublicName: "second"}})
	require.NoError(t, err)
	shared := catalog.snapshot.Load().routes["first"].direct.handler
	shared.routeReferences = 1
	require.NoError(t, catalog.BeginMutation(mutation))

	progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
	require.Error(t, err)
	assert.False(t, progress.Quiesced)
	assert.Equal(t, 1, shared.routeReferences)
	require.NoError(t, catalog.AbortMutation(mutation))
}

func TestFunctionCatalogCleanupWaitsForLastLease(t *testing.T) {
	var cleanupCalls atomic.Int32
	old := testDeclaration("work", "", ResourcePolicy{})
	old.Generation.Cleanup = func(context.Context) error {
		cleanupCalls.Add(1)
		return nil
	}
	catalog, err := NewCatalog([]Declaration{old})
	require.NoError(t, err)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "held", Route: "work"})
	require.NoError(t, err)

	replacement := testDeclaration("work", "", ResourcePolicy{})
	mutation, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work", Declaration: &replacement}})
	require.NoError(t, err)
	cleanups := commitMutation(t, catalog, mutation)
	assert.Empty(t, cleanups)
	assert.Zero(t, cleanupCalls.Load())

	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.True(t, cleanup.Valid())
	runCleanupPlan(t, catalog, cleanup)
	assert.EqualValues(t, 1, cleanupCalls.Load())
}

func TestFunctionCatalogCleanupOwnershipSurvivesPausedLeaseRelease(t *testing.T) {
	old := testDeclaration("work", "", ResourcePolicy{})
	old.Generation.Cleanup = func(context.Context) error { return nil }
	catalog, err := NewCatalog([]Declaration{old})
	require.NoError(t, err)
	held, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "held", Route: "work"})
	require.NoError(t, err)
	mutation, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work"}})
	require.NoError(t, err)
	require.NoError(t, catalog.BeginMutation(mutation))
	progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
	require.NoError(t, err)
	require.True(t, progress.Quiesced)

	cleanup, err := catalog.ReleaseInvocation(held.Lease)
	require.NoError(t, err)
	assert.False(t, cleanup.Valid())
	require.NoError(t, catalog.ResumeMutation(mutation))
	progress, cleanups := catalog.AdvanceMutation(MaximumMutationQuantum)
	require.True(t, progress.Done)
	require.Len(t, cleanups, 1)
	runCleanupPlan(t, catalog, cleanups[0])
}

func TestFunctionCatalogRemovedRouteStaysUnavailableUntilLeaseDrains(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("work", "", ResourcePolicy{})})
	require.NoError(t, err)
	held, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "held", Route: "work"})
	require.NoError(t, err)
	mutation, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work"}})
	require.NoError(t, err)
	commitMutation(t, catalog, mutation)

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "retiring", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlUnavailable, decision.Rejected)
	_, err = catalog.ReleaseInvocation(held.Lease)
	require.NoError(t, err)
	decision, err = catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "retired", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlNotFound, decision.Rejected)
}

func TestFunctionCatalogRemovedRouteWaitsForEveryRetiredGeneration(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("work", "", ResourcePolicy{})})
	require.NoError(t, err)
	first, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "first", Route: "work"})
	require.NoError(t, err)
	removeFirst, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work"}})
	require.NoError(t, err)
	commitMutation(t, catalog, removeFirst)

	secondDeclaration := testDeclaration("work", "", ResourcePolicy{})
	addSecond, err := catalog.NewMutation(2, []RouteChange{{PublicName: "work", Declaration: &secondDeclaration}})
	require.NoError(t, err)
	commitMutation(t, catalog, addSecond)
	second, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "second", Route: "work"})
	require.NoError(t, err)
	removeSecond, err := catalog.NewMutation(3, []RouteChange{{PublicName: "work"}})
	require.NoError(t, err)
	commitMutation(t, catalog, removeSecond)

	_, err = catalog.ReleaseInvocation(second.Lease)
	require.NoError(t, err)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "first-held", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlUnavailable, decision.Rejected)
	_, err = catalog.ReleaseInvocation(first.Lease)
	require.NoError(t, err)
	decision, err = catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "all-drained", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlNotFound, decision.Rejected)
}

func TestFunctionCatalogRemovalWaitsForReplacedGeneration(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("work", "", ResourcePolicy{})})
	require.NoError(t, err)
	first, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "first", Route: "work"})
	require.NoError(t, err)
	secondDeclaration := testDeclaration("work", "", ResourcePolicy{})
	replace, err := catalog.NewMutation(1, []RouteChange{{PublicName: "work", Declaration: &secondDeclaration}})
	require.NoError(t, err)
	commitMutation(t, catalog, replace)
	second, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "second", Route: "work"})
	require.NoError(t, err)
	remove, err := catalog.NewMutation(2, []RouteChange{{PublicName: "work"}})
	require.NoError(t, err)
	commitMutation(t, catalog, remove)

	_, err = catalog.ReleaseInvocation(second.Lease)
	require.NoError(t, err)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "first-held", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlUnavailable, decision.Rejected)
	_, err = catalog.ReleaseInvocation(first.Lease)
	require.NoError(t, err)
	decision, err = catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "all-drained", Route: "work"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlNotFound, decision.Rejected)
}

func TestFunctionCatalogRetiredRouteDrainsBeforeSharedGeneration(t *testing.T) {
	generation := testGeneration("shared")
	catalog, err := NewCatalog([]Declaration{
		testDeclarationForGeneration(generation, "one", "", ResourcePolicy{}),
		testDeclarationForGeneration(generation, "two", "", ResourcePolicy{}),
	})
	require.NoError(t, err)
	held, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "held", Route: "one"})
	require.NoError(t, err)
	remove, err := catalog.NewMutation(1, []RouteChange{{PublicName: "one"}})
	require.NoError(t, err)
	commitMutation(t, catalog, remove)
	_, err = catalog.ReleaseInvocation(held.Lease)
	require.NoError(t, err)

	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "retired", Route: "one"})
	require.NoError(t, err)
	assert.Equal(t, lifecycle.ControlNotFound, decision.Rejected)
	other, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "other", Route: "two"})
	require.NoError(t, err)
	require.True(t, other.Lease.Valid())
	_, err = catalog.ReleaseInvocation(other.Lease)
	require.NoError(t, err)
}

func TestFunctionCatalogCloseIsBoundedAndCleansSharedGenerationOnce(t *testing.T) {
	var cleanupCalls atomic.Int32
	generation := testGeneration("shared")
	generation.Cleanup = func(context.Context) error {
		cleanupCalls.Add(1)
		return nil
	}
	declarations := []Declaration{
		testDeclarationForGeneration(generation, "one", "", ResourcePolicy{}),
		testDeclarationForGeneration(generation, "two", "", ResourcePolicy{}),
	}
	catalog, err := NewCatalog(declarations)
	require.NoError(t, err)
	require.NoError(t, catalog.BeginClose())

	first, more, err := catalog.CloseStep(1)
	require.NoError(t, err)
	require.True(t, more)
	assert.Empty(t, first)
	second, more, err := catalog.CloseStep(1)
	require.NoError(t, err)
	require.False(t, more)
	require.Len(t, second, 1)
	runCleanupPlan(t, catalog, second[0])

	census := catalog.census()
	assert.True(t, census.Closed)
	assert.Zero(t, census.Routes)
	assert.Zero(t, census.PendingCleanups)
	assert.EqualValues(t, 1, cleanupCalls.Load())
}

func TestFunctionCatalogCloseStepRejectsInvalidBatchBeforeMutation(t *testing.T) {
	first := testDeclaration("first", "", ResourcePolicy{})
	first.Generation.Cleanup = func(context.Context) error { return nil }
	second := testDeclaration("second", "", ResourcePolicy{})
	second.Generation.Cleanup = func(context.Context) error { return nil }
	catalog, err := NewCatalog([]Declaration{first, second})
	require.NoError(t, err)
	firstGeneration := catalog.snapshot.Load().routes["first"].direct.handler
	secondGeneration := catalog.snapshot.Load().routes["second"].direct.handler
	secondGeneration.routeReferences = 0
	require.NoError(t, catalog.BeginClose())

	cleanups, more, err := catalog.CloseStep(2)
	require.Error(t, err)
	assert.Empty(t, cleanups)
	assert.False(t, more)
	assert.Zero(t, catalog.closeIndex)
	assert.Equal(t, 2, catalog.routeCount)
	assert.Equal(t, 1, firstGeneration.routeReferences)
	assert.Zero(t, catalog.pendingCleanups)
}

func TestFunctionCatalogCloseStepAggregatesSharedGeneration(t *testing.T) {
	generation := testGeneration("shared")
	catalog, err := NewCatalog([]Declaration{
		testDeclarationForGeneration(generation, "first", "", ResourcePolicy{}),
		testDeclarationForGeneration(generation, "second", "", ResourcePolicy{}),
	})
	require.NoError(t, err)
	shared := catalog.snapshot.Load().routes["first"].direct.handler
	shared.routeReferences = 1
	require.NoError(t, catalog.BeginClose())

	cleanups, more, err := catalog.CloseStep(2)
	require.Error(t, err)
	assert.Empty(t, cleanups)
	assert.False(t, more)
	assert.Zero(t, catalog.closeIndex)
	assert.Equal(t, 2, catalog.routeCount)
	assert.Equal(t, 1, shared.routeReferences)
}

func TestFunctionCatalogReleaseValidatesCleanupBeforeReleasingLease(t *testing.T) {
	declaration := testDeclaration("work", "", ResourcePolicy{})
	declaration.Generation.Cleanup = func(context.Context) error { return nil }
	catalog, err := NewCatalog([]Declaration{declaration})
	require.NoError(t, err)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "held", Route: "work"})
	require.NoError(t, err)
	resolved := catalog.snapshot.Load().routes["work"].direct
	generation := resolved.handler
	generation.routeReferences = 0
	catalog.cleanupByRef[generation.cleanupRef] = &handlerGeneration{}

	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	require.Error(t, err)
	assert.False(t, cleanup.Valid())
	assert.Equal(t, 1, resolved.invocationLeases)
	assert.Equal(t, 1, generation.invocationLeases)
	assert.Equal(t, 1, catalog.invocationCount)

	delete(catalog.cleanupByRef, generation.cleanupRef)
	cleanup, err = catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.True(t, cleanup.Valid())
}

func TestFunctionCatalogInvocationPopulationExceedsFormerLimit(t *testing.T) {
	const population = 300
	catalog, err := NewCatalog([]Declaration{testDeclaration("work", "", ResourcePolicy{})})
	require.NoError(t, err)
	leases := make([]jobmgr.FunctionInvocationRef, 0, population)
	for index := range population {
		decision, resolveErr := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
			UID: fmt.Sprintf("call-%03d", index), Route: "work",
		})
		require.NoError(t, resolveErr)
		require.True(t, decision.Lease.Valid())
		leases = append(leases, decision.Lease)
	}
	for _, lease := range leases {
		_, releaseErr := catalog.ReleaseInvocation(lease)
		require.NoError(t, releaseErr)
	}
}

func TestFunctionCatalogDeclarationMetadataBounds(t *testing.T) {
	tests := map[string]struct {
		mutate func(*Declaration)
	}{
		"route": {
			mutate: func(declaration *Declaration) {
				declaration.PublicName = strings.Repeat("r", maximumDeclarationMetadataBytes+1)
			},
		},
		"method": {
			mutate: func(declaration *Declaration) {
				declaration.ID = strings.Repeat("m", maximumDeclarationMetadataBytes+1)
			},
		},
		"generation": {
			mutate: func(declaration *Declaration) {
				declaration.Generation.ID = strings.Repeat("g", maximumDeclarationMetadataBytes+1)
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			declaration := testDeclaration("work", "", ResourcePolicy{})
			test.mutate(&declaration)
			_, err := NewCatalog([]Declaration{declaration})
			require.Error(t, err)
		})
	}
}

func commitMutation(t *testing.T, catalog *Catalog, mutation *Mutation) []jobmgr.FunctionCleanupPlan {
	t.Helper()
	require.NoError(t, catalog.BeginMutation(mutation))
	for {
		progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
		require.NoError(t, err)
		if progress.Quiesced {
			break
		}
	}
	require.NoError(t, catalog.ResumeMutation(mutation))
	var cleanups []jobmgr.FunctionCleanupPlan
	for {
		progress, batch := catalog.AdvanceMutation(MaximumMutationQuantum)
		cleanups = append(cleanups, batch...)
		if progress.Done {
			return cleanups
		}
	}
}

func testGeneration(id string) *HandlerGenerationDeclaration {
	return &HandlerGenerationDeclaration{
		ID: id,
		Handler: func(context.Context, HandlerInput) (lifecycle.SealedResult, error) {
			return lifecycle.NewControlResult(lifecycle.ControlInternal)
		},
	}
}

func testDeclaration(publicName, prefix string, resource ResourcePolicy) Declaration {
	return testDeclarationForGeneration(testGeneration(publicName), publicName, prefix, resource)
}

func testDeclarationForGeneration(
	generation *HandlerGenerationDeclaration,
	publicName, prefix string,
	resource ResourcePolicy,
) Declaration {
	return Declaration{
		ID: "method", Generation: generation, PublicName: publicName,
		Prefix: prefix, Resource: resource,
	}
}

func testCleanupDeclarations(count int) []Declaration {
	declarations := make([]Declaration, count)
	for index := range declarations {
		name := fmt.Sprintf("cleanup-%03d", index)
		declarations[index] = testDeclaration(name, "", ResourcePolicy{})
	}
	return declarations
}

func runCleanupPlan(t *testing.T, catalog *Catalog, cleanup jobmgr.FunctionCleanupPlan) {
	t.Helper()
	require.True(t, cleanup.Valid())
	require.NotNil(t, cleanup.Work())
	_, err := cleanup.Work()(context.Background())
	require.NoError(t, err)
	require.NoError(t, catalog.CompleteCleanup(cleanup.Ref()))
}

type catalogCensus struct {
	Version          uint64
	Routes           int
	InvocationLeases int
	PendingCleanups  int
	Closed           bool
	MutationActive   bool
}

func (c *Catalog) census() catalogCensus {
	return catalogCensus{
		Version:          c.version,
		Routes:           c.routeCount,
		InvocationLeases: c.invocationCount,
		PendingCleanups:  c.pendingCleanups,
		Closed:           c.closed,
		MutationActive:   c.mutation != nil,
	}
}
