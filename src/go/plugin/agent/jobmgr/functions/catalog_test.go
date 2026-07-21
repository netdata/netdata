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
	"github.com/stretchr/testify/require"
)

func TestFunctionCatalogLookupLeaseSameTurn(t *testing.T) {
	tests := map[string]struct {
		declaration  Declaration
		lookup       jobmgr.FunctionLookup
		wantStatus   lifecycle.ControlStatus
		wantResource string
		wantMethod   string
	}{
		"direct route": {
			declaration: testDeclaration("direct", "", ResourcePolicy{}),
			lookup:      jobmgr.FunctionLookup{UID: "direct-uid", Route: "direct"},
			wantMethod:  "method",
		},
		"prefix route": {
			declaration: testDeclaration("config", "job:", ResourcePolicy{}),
			lookup: jobmgr.FunctionLookup{
				UID: "prefix-uid", Route: "config", Args: []string{"job:mysql"},
			},
			wantMethod: "method",
		},
		"DynCfg existing job resource": {
			declaration: testDeclaration("config", "go.d:collector:", DynCfgJobResource(0, "go.d:collector:")),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-update", Route: "config",
				Args: []string{
					"go.d:collector:mysql:production",
					"update",
				},
			},
			wantResource: "mysql_production",
			wantMethod:   "method",
		},
		"DynCfg add job resource": {
			declaration: testDeclaration("config", "go.d:collector:", DynCfgJobResource(0, "go.d:collector:")),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-add", Route: "config",
				Args: []string{
					"go.d:collector:mysql",
					"add",
					"production",
				},
			},
			wantResource: "mysql_production",
			wantMethod:   "method",
		},
		"scoped DynCfg existing resource identity": {
			declaration: testDeclaration(
				"config",
				"go.d:secretstore:",
				ScopedDynCfgJobResource(0, "go.d:secretstore:", "secretstore:"),
			),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-secret-update", Route: "config",
				Args: []string{
					"go.d:secretstore:vault:production",
					"update",
				},
			},
			wantResource: "secretstore:vault_production",
			wantMethod:   "method",
		},
		"scoped DynCfg add resource identity": {
			declaration: testDeclaration("config", "go.d:vnode", ScopedDynCfgJobResource(0, "go.d:vnode", "vnode:")),
			lookup: jobmgr.FunctionLookup{
				UID: "dyncfg-vnode-add", Route: "config",
				Args: []string{"go.d:vnode", "add", "production"},
			},
			wantResource: "vnode:production",
			wantMethod:   "method",
		},
		"prefix missing argument": {
			declaration: testDeclaration("config", "job:", ResourcePolicy{}),
			lookup:      jobmgr.FunctionLookup{UID: "missing-argument", Route: "config"},
			wantStatus:  lifecycle.ControlNotFound,
		},
		"unknown public route": {
			declaration: testDeclaration("direct", "", ResourcePolicy{}),
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
			require.NoError(t, err)
			decision, err := catalog.ResolveAndAcquire(test.lookup)
			require.NoError(t, err)
			require.EqualValues(t, test.wantStatus, decision.Rejected)
			if test.wantStatus != 0 {
				require.False(t, decision.Lease.Valid() || decision.Plan.Runner != nil)
				return
			}
			require.False(t, !decision.Lease.Valid() || decision.Plan.Runner == nil || decision.ResourceID != test.wantResource)

			require.EqualValues(t, 1, catalog.Census().InvocationLeases)

			outcome, err := decision.Plan.Runner.RunTask(context.Background())
			require.NoError(t, err)
			require.False(t, outcome.Kind() != lifecycle.TaskOutcomeFrame || handled.UID != test.lookup.UID ||
				handled.Method != test.wantMethod)
			cleanup, err := catalog.ReleaseInvocation(decision.Lease)
			require.NoError(t, err)
			require.False(t, cleanup.Ref.Valid())

			require.EqualValues(t, 0, catalog.Census().InvocationLeases)

			_, releaseInvocationErr := catalog.ReleaseInvocation(decision.Lease)
			require.Error(t, releaseInvocationErr)

		})
	}
}

func TestFunctionCatalogInvocationPopulationGrowsBeyondFormerLimit(t *testing.T) {
	tests := map[string]struct {
		declaration Declaration
		lookup      func(int) jobmgr.FunctionLookup
	}{
		"direct route": {
			declaration: testDeclaration("direct", "", ResourcePolicy{}),
			lookup: func(index int) jobmgr.FunctionLookup {
				return jobmgr.FunctionLookup{
					UID:   fmt.Sprintf("direct-%03d", index),
					Route: "direct",
				}
			},
		},
		"prefix route": {
			declaration: testDeclaration("config", "job:", ResourcePolicy{}),
			lookup: func(index int) jobmgr.FunctionLookup {
				return jobmgr.FunctionLookup{
					UID:   fmt.Sprintf("prefix-%03d", index),
					Route: "config",
					Args:  []string{"job:instance"},
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			catalog, err := NewCatalog([]Declaration{test.declaration})
			require.NoError(t, err)
			const population = 257
			leases := make([]jobmgr.FunctionInvocationRef, 0, population)
			for index := range population {
				decision, resolveErr := catalog.ResolveAndAcquire(test.lookup(index))
				require.NoError(t, resolveErr)
				require.False(t, decision.Rejected != 0 || !decision.Lease.Valid())
				leases = append(leases, decision.Lease)
			}
			for _, lease := range leases {

				_, releaseInvocationErr := catalog.ReleaseInvocation(lease)
				require.NoError(t, releaseInvocationErr)

			}
		})
	}
}

func TestFunctionPayloadValidationRunsInTaskChild(t *testing.T) {
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
	require.EqualValues(t, 0, calls.Load())
	outcome, err := decision.Plan.Runner.RunTask(context.Background())
	require.NoError(t, err)
	require.False(t, outcome.Kind() != lifecycle.TaskOutcomeFrame || calls.Load() != 0)

	_, releaseInvocationErr := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, releaseInvocationErr)

}

func TestFunctionCatalogReturnsSealedResourceTransactionPlan(t *testing.T) {
	permit, err := lifecycle.NewJobLongLivedPlan(4096)
	require.NoError(t, err)
	tests := map[string]struct {
		command           string
		allocateSuccessor bool
		wantClaims        []string
	}{
		"update allocates successor": {
			command: "update", allocateSuccessor: true,
			wantClaims: []string{
				"dyncfg:graph",
				"dyncfg:jobs",
			},
		},
		"remove has no successor": {
			command: "remove",
			wantClaims: []string{
				"dyncfg:graph",
			},
		},
	}

	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			declaration := testDeclaration("config", "job:", DynCfgJobResource(0, "job:"))
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
					{
						Name: "update", AllocateSuccessor: true,
						Claims: []string{"dyncfg:jobs"},
					},
					{Name: "remove"},
				},
			}
			catalog, err := NewCatalog([]Declaration{declaration})
			require.NoError(t, err)
			lookup := jobmgr.FunctionLookup{
				UID:        "transaction",
				Route:      "config",
				Args:       []string{"job:mysql", test.command},
				Payload:    []byte(`{"option":true}`),
				HasPayload: true,
			}
			decision, err := catalog.ResolveAndAcquire(lookup)
			require.NoError(t, err)
			plan := decision.Plan.Transaction
			require.False(t, plan == nil ||
				decision.Plan.Runner != nil ||
				plan.ID != "mysql" ||
				plan.AllocateSuccessor != test.allocateSuccessor ||
				!reflect.DeepEqual(decision.Plan.Claims, test.wantClaims))
			if test.allocateSuccessor {
				require.NoError(t, plan.Permit.Validate())
			} else {
				require.False(t, plan.Permit.Class() != 0 || plan.Permit.Bytes() != 0)
			}

			_, prepareErr := plan.Prepare(
				context.Background(),
				nil,
				lifecycle.ResourceTransactionScope{},
				lifecycle.LongLivedPermit{},
			)
			require.NoError(t, prepareErr)

			require.False(t, preparedInput.UID != lookup.UID ||
				!reflect.DeepEqual(preparedInput.Args, lookup.Args) ||
				!reflect.DeepEqual(preparedInput.Payload, lookup.Payload))

			_, releaseInvocationErr := catalog.ReleaseInvocation(decision.Lease)
			require.NoError(t, releaseInvocationErr)

		})
	}
}

func TestFunctionCatalogDerivesSuccessorPermitFromInvocation(t *testing.T) {
	tests := map[string]struct {
		payload   []byte
		wantBytes int64
	}{
		"payload sizes the permit": {
			payload:   []byte(`{"option":"value"}`),
			wantBytes: int64(len(`{"option":"value"}`)),
		},
		"empty payload retains minimum byte": {
			wantBytes: 1,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			declaration := testDeclaration("config", "job:", DynCfgJobResource(0, "job:"))
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
				PermitPolicy:    SuccessorPermitSecretStorePayload,
				CommandArgument: 1,
				GlobalClaim:     "dyncfg:graph",
				Commands: []ResourceTransactionCommand{
					{Name: "update", AllocateSuccessor: true},
				},
			}
			catalog, err := NewCatalog([]Declaration{declaration})
			require.NoError(t, err)
			decision, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID: "transaction", Route: "config",
					Args:    []string{"job:mysql", "update"},
					Payload: test.payload, HasPayload: true,
				},
			)
			require.NoError(t, err)
			require.False(t, decision.Plan.Transaction == nil || decision.Plan.Transaction.Permit.Bytes() != test.wantBytes)

			_, releaseInvocationErr := catalog.ReleaseInvocation(decision.Lease)
			require.NoError(t, releaseInvocationErr)

		})
	}
}

func TestHandlerLeaseLifecycle(t *testing.T) {
	var cleanupCalls atomic.Int32
	declaration := testDeclaration("direct", "", ResourcePolicy{})
	declaration.Generation.Cleanup = func(context.Context) error {
		cleanupCalls.Add(1)
		return nil
	}
	catalog, err := NewCatalog([]Declaration{declaration})
	require.NoError(t, err)
	decision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
		UID: "held", Route: "direct",
	})
	require.NoError(t, err)

	require.NoError(t, catalog.BeginClose())

	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	count, more, err := catalog.CloseStep(1, &cleanups)
	require.NoError(t, err)
	require.False(t, count != 0 || more || cleanupCalls.Load() != 0)
	cleanup, err := catalog.ReleaseInvocation(decision.Lease)
	require.NoError(t, err)
	require.False(t, !cleanup.Ref.Valid() || cleanup.Runner == nil)
	require.EqualValues(t, 0, cleanupCalls.Load())
	outcome, err := cleanup.Runner.RunTask(context.Background())
	require.NoError(t, err)
	require.False(t, outcome.Kind() != lifecycle.TaskOutcomeNone || cleanupCalls.Load() != 1)

	require.NoError(t, catalog.CompleteCleanup(cleanup.Ref))

	require.Error(t, catalog.CompleteCleanup(cleanup.Ref))

	census := catalog.Census()
	require.Zero(t, census.PendingCleanups)
}

func TestRetiredRouteRejectsDuringLeaseDrainThenDisappears(t *testing.T) {
	tests := map[string]struct {
		prefix   string
		args     []string
		resource ResourcePolicy
	}{
		"direct route": {
			resource: ResourcePolicy{},
		},
		"prefix route": {
			prefix:   "job:",
			args:     []string{"job:one"},
			resource: ResourcePolicy{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			var cleanupCalls atomic.Int32
			declaration := testDeclaration("work", test.prefix, test.resource)
			declaration.Generation.Cleanup = func(
				context.Context,
			) error {
				cleanupCalls.Add(1)
				return nil
			}
			catalog, err := NewCatalog([]Declaration{declaration})
			require.NoError(t, err)
			held, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "held",
					Route: "work",
					Args:  test.args,
				},
			)
			require.NoError(t, err)
			mutation, err := catalog.NewMutation(
				catalog.Census().Version,
				[]RouteChange{{
					PublicName: "work",
					Prefix:     test.prefix,
				}},
			)
			require.NoError(t, err)
			builder, err := catalog.startMutation(mutation)
			require.NoError(t, err)
			var postimage *MutationPostimage
			for {
				var done bool
				postimage, done, err = builder.PrepareStep(MaximumMutationQuantum)
				require.NoError(t, err)
				if done {
					break
				}
			}
			var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
			count, err := catalog.commitMutation(postimage, &cleanups)
			require.NoError(t, err)
			require.EqualValues(t, 0, count)

			rejected, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "during-drain",
					Route: "work",
					Args:  test.args,
				},
			)
			require.NoError(t, err)
			require.False(t, rejected.Rejected != lifecycle.ControlUnavailable || rejected.Lease.Valid())
			unknown, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "unknown",
					Route: "other",
					Args:  test.args,
				},
			)
			require.NoError(t, err)
			require.EqualValues(t, lifecycle.ControlNotFound, unknown.Rejected)

			cleanup, err := catalog.ReleaseInvocation(held.Lease)
			require.NoError(t, err)
			afterDrain, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "after-drain",
					Route: "work",
					Args:  test.args,
				},
			)
			require.NoError(t, err)
			require.EqualValues(t, lifecycle.ControlNotFound, afterDrain.Rejected)

			published := catalog.storage.published.Load()
			require.EqualValues(t, 0, published)

			runCleanupPlan(t, catalog, cleanup)
			require.EqualValues(t, 1, cleanupCalls.Load())
		})
	}
}

func TestFunctionCatalogSharedGenerationUsesRouteLocalLeaseDrain(t *testing.T) {
	tests := map[string]struct {
		prefix   string
		args     []string
		resource ResourcePolicy
	}{
		"direct route": {
			resource: ResourcePolicy{},
		},
		"prefix route": {
			prefix:   "job:",
			args:     []string{"job:one"},
			resource: ResourcePolicy{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			generation := testGeneration("shared")
			target := testDeclarationForGeneration(
				generation,
				"work",
				test.prefix,
				test.resource,
			)
			sibling := testDeclarationForGeneration(
				generation,
				"sibling",
				"",
				ResourcePolicy{},
			)
			catalog, err := NewCatalog([]Declaration{target, sibling})
			require.NoError(t, err)
			held, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "held-sibling",
					Route: "sibling",
				},
			)
			require.NoError(t, err)
			before := catalog.storage.published.Load()
			mutation, err := catalog.NewMutation(
				catalog.Census().Version,
				[]RouteChange{{
					PublicName: "work",
					Prefix:     test.prefix,
				}},
			)
			require.NoError(t, err)
			builder, err := catalog.startMutation(mutation)
			require.NoError(t, err)
			var postimage *MutationPostimage
			for {
				var done bool
				postimage, done, err = builder.PrepareStep(MaximumMutationQuantum)
				require.NoError(t, err)
				if done {
					break
				}
			}
			var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
			count, err := catalog.commitMutation(postimage, &cleanups)
			require.NoError(t, err)
			require.EqualValues(t, 0, count)
			removed, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "removed",
					Route: "work",
					Args:  test.args,
				},
			)
			require.NoError(t, err)
			require.False(t, removed.Rejected != lifecycle.ControlNotFound || removed.Lease.Valid())

			published := catalog.storage.published.Load()
			require.False(t, published >= before)

			if cleanup, err := catalog.ReleaseInvocation(held.Lease); err != nil {
				require.FailNow(t, "test failed", err)
			} else {
				require.False(t, cleanup.Ref.Valid())
			}
		})
	}
}

func TestFunctionCatalogReaddsRouteBeforeRetiredLeaseDrains(t *testing.T) {
	tests := map[string]struct {
		prefix   string
		args     []string
		resource ResourcePolicy
	}{
		"direct route": {
			resource: ResourcePolicy{},
		},
		"prefix route": {
			prefix:   "job:",
			args:     []string{"job:one"},
			resource: ResourcePolicy{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			oldGeneration := testGeneration("old-shared")
			target := testDeclarationForGeneration(
				oldGeneration,
				"work",
				test.prefix,
				test.resource,
			)
			sibling := testDeclarationForGeneration(
				oldGeneration,
				"sibling",
				"",
				ResourcePolicy{},
			)
			catalog, err := NewCatalog([]Declaration{target, sibling})
			require.NoError(t, err)
			held, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "held-old",
					Route: "work",
					Args:  test.args,
				},
			)
			require.NoError(t, err)
			commit := func(change RouteChange) {
				t.Helper()
				mutation, mutationErr := catalog.NewMutation(
					catalog.Census().Version,
					[]RouteChange{change},
				)
				require.NoError(t, mutationErr)
				builder, mutationErr := catalog.startMutation(mutation)
				require.NoError(t, mutationErr)
				var postimage *MutationPostimage
				for {
					var done bool
					postimage, done, mutationErr = builder.PrepareStep(MaximumMutationQuantum)
					require.NoError(t, mutationErr)
					if done {
						break
					}
				}
				var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan

				count, commitErr := catalog.commitMutation(postimage, &cleanups)
				require.False(t, commitErr != nil || count != 0)
			}
			commit(RouteChange{
				PublicName: "work",
				Prefix:     test.prefix,
			})
			replacement := testDeclaration("work", test.prefix, test.resource)
			commit(RouteChange{
				PublicName:  "work",
				Prefix:      test.prefix,
				Declaration: &replacement,
			})

			require.EqualValues(t, 2, catalog.Census().Routes)

			require.False(t, resolvedMethod(catalog, "sibling", nil) == "" || resolvedMethod(catalog, "work", test.args) == "")
			cleanup, err := catalog.ReleaseInvocation(held.Lease)
			require.NoError(t, err)
			require.False(t, cleanup.Ref.Valid())

			siblingInvocation, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{
				UID: "sibling", Route: "sibling",
			})
			require.NoError(t, err)
			cleanup, err = catalog.ReleaseInvocation(siblingInvocation.Lease)
			require.NoError(t, err)
			require.False(t, cleanup.Ref.Valid())
		})
	}
}

func TestFunctionCatalogQuiesceAbortRestoresAdmission(t *testing.T) {
	tests := map[string]struct {
		prefix   string
		args     []string
		resource ResourcePolicy
	}{
		"direct route": {
			resource: ResourcePolicy{},
		},
		"prefix route": {
			prefix:   "job:",
			args:     []string{"job:one"},
			resource: ResourcePolicy{},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			catalog, err := NewCatalog([]Declaration{
				testDeclaration("work", test.prefix, test.resource),
			})
			require.NoError(t, err)
			mutation, err := catalog.NewMutation(
				catalog.Census().Version,
				[]RouteChange{{
					PublicName: "work",
					Prefix:     test.prefix,
				}},
			)
			require.NoError(t, err)

			require.NoError(t, catalog.BeginMutation(mutation))

			for {
				progress, err := catalog.AdvanceMutationQuiesce(MaximumMutationQuantum)
				require.NoError(t, err)
				if progress.Quiesced {
					break
				}
			}
			rejected, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID: "quiesced", Route: "work", Args: test.args,
				},
			)
			require.NoError(t, err)
			require.False(t, rejected.Rejected != lifecycle.ControlUnavailable ||
				rejected.Lease.Valid() ||
				catalog.Census().Version != 1)

			require.NoError(t, catalog.ResumeMutation(mutation))

			var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan

			abortMutationCount, abortMutationErr := catalog.AbortMutation(&cleanups)
			require.False(t, abortMutationErr != nil || abortMutationCount != 0)

			restored, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID: "restored", Route: "work", Args: test.args,
				},
			)
			require.NoError(t, err)
			require.False(t, restored.Rejected != 0 || !restored.Lease.Valid() ||
				catalog.Census().Version != 1 ||
				catalog.Census().MutationActive)

			_, releaseInvocationErr := catalog.ReleaseInvocation(restored.Lease)
			require.NoError(t, releaseInvocationErr)

		})
	}
}

func TestRetiredRouteDrainDuringUnrelatedMutationDefersPhysicalPrune(t *testing.T) {
	var cleanupCalls atomic.Int32
	heldDeclaration := testDeclaration(
		"held",
		"",
		ResourcePolicy{},
	)
	heldDeclaration.Generation.Cleanup = func(
		context.Context,
	) error {
		cleanupCalls.Add(1)
		return nil
	}
	otherDeclaration := testDeclaration(
		"other",
		"",
		ResourcePolicy{},
	)
	catalog, err := NewCatalog(
		[]Declaration{heldDeclaration, otherDeclaration},
	)
	require.NoError(t, err)
	held, err := catalog.ResolveAndAcquire(
		jobmgr.FunctionLookup{UID: "held", Route: "held"},
	)
	require.NoError(t, err)
	remove, err := catalog.NewMutation(
		catalog.Census().Version,
		[]RouteChange{{PublicName: "held"}},
	)
	require.NoError(t, err)
	removeBuilder, err := catalog.startMutation(remove)
	require.NoError(t, err)
	var removePostimage *MutationPostimage
	for {
		var done bool
		removePostimage, done, err = removeBuilder.PrepareStep(MaximumMutationQuantum)
		require.NoError(t, err)
		if done {
			break
		}
	}
	var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan

	commitMutationCount, commitMutationErr := catalog.commitMutation(removePostimage, &cleanups)
	require.False(t, commitMutationErr != nil || commitMutationCount != 0)

	replacement := testDeclaration(
		"other",
		"",
		ResourcePolicy{},
	)
	unrelated, err := catalog.NewMutation(
		catalog.Census().Version,
		[]RouteChange{{
			PublicName:  "other",
			Declaration: &replacement,
		}},
	)
	require.NoError(t, err)
	unrelatedBuilder, err := catalog.startMutation(unrelated)
	require.NoError(t, err)
	for unrelatedBuilder.phase == mutationTopology {

		_, _, prepareStepErr := unrelatedBuilder.PrepareStep(1)
		require.NoError(t, prepareStepErr)

	}

	cleanup, err := catalog.ReleaseInvocation(held.Lease)
	require.NoError(t, err)
	require.NotNil(t, catalog.deferredPrune)
	afterDrain, err := catalog.ResolveAndAcquire(
		jobmgr.FunctionLookup{
			UID:   "after-drain",
			Route: "held",
		},
	)
	require.NoError(t, err)
	require.EqualValues(t, lifecycle.ControlNotFound, afterDrain.Rejected)

	var unrelatedPostimage *MutationPostimage
	for {
		var done bool
		unrelatedPostimage, done, err =
			unrelatedBuilder.PrepareStep(MaximumMutationQuantum)
		require.NoError(t, err)
		if done {
			break
		}
	}
	count, err := catalog.commitMutation(unrelatedPostimage, &cleanups)
	require.NoError(t, err)
	for index := range count {
		runCleanupPlan(t, catalog, cleanups[index])
	}
	require.Nil(t, catalog.deferredPrune)
	published := catalog.storage.published.Load()

	want := catalogPathStorage(catalog.routes)
	require.EqualValues(t, want, published)

	runCleanupPlan(t, catalog, cleanup)
	require.EqualValues(t, 1, cleanupCalls.Load())
}

func TestHandlerCleanupOnce(t *testing.T) {
	var cleanupCalls atomic.Int32
	generation := testGeneration("config-generation")
	generation.Cleanup = func(context.Context) error {
		cleanupCalls.Add(1)
		return nil
	}
	declarations := []Declaration{
		testDeclarationForGeneration(generation, "config", "job:", ResourcePolicy{}),
		testDeclarationForGeneration(generation, "config", "store:", ResourcePolicy{}),
	}
	catalog, err := NewCatalog(declarations)
	require.NoError(t, err)

	require.NoError(t, catalog.BeginClose())

	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	total := 0
	for {
		count, more, err := catalog.CloseStep(1, &cleanups)
		require.NoError(t, err)
		for _, cleanup := range cleanups[:count] {

			_, runTaskErr := cleanup.Runner.RunTask(context.Background())
			require.NoError(t, runTaskErr)

			require.NoError(t, catalog.CompleteCleanup(cleanup.Ref))

			total++
		}
		if !more {
			break
		}
	}
	require.False(t, total != 1 || cleanupCalls.Load() != 1)

	census := catalog.Census()
	require.Zero(t, census.Routes)
}

func TestFunctionCatalogRetainsGenerationStorageUntilCleanupCompletion(
	t *testing.T,
) {
	const population = 9

	tests := map[string]struct {
		newCatalog func(*testing.T, []Declaration) *Catalog
	}{
		"initial generations": {
			newCatalog: func(t *testing.T, declarations []Declaration) *Catalog {
				t.Helper()
				catalog, err := NewCatalog(declarations)
				require.NoError(t, err)
				return catalog
			},
		},
		"committed mutation generations": {
			newCatalog: func(t *testing.T, declarations []Declaration) *Catalog {
				t.Helper()
				catalog, err := NewCatalog(nil)
				require.NoError(t, err)
				changes := make([]RouteChange, 0, len(declarations))
				for index := range declarations {
					declaration := &declarations[index]
					changes = append(changes, RouteChange{
						PublicName:  declaration.PublicName,
						Declaration: declaration,
					})
				}
				mutation, err := catalog.NewMutation(catalog.Census().Version, changes)
				require.NoError(t, err)
				builder, err := catalog.startMutation(mutation)
				require.NoError(t, err)
				var postimage *MutationPostimage
				for {
					var done bool
					postimage, done, err = builder.PrepareStep(MaximumMutationQuantum)
					require.NoError(t, err)
					if done {
						break
					}
				}
				var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan

				commitMutationCount, commitMutationErr := catalog.commitMutation(postimage, &cleanups)
				require.False(t, commitMutationErr != nil || commitMutationCount != 0)

				return catalog
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			catalog := test.newCatalog(t, testCleanupDeclarations(population))

			require.NoError(t, catalog.BeginClose())

			var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
			count, more, err := catalog.CloseStep(MaximumCloseQuantum, &cleanups)
			require.NoError(t, err)
			require.False(t, count != population || more)
			wantRetentionBytes := int64(population) *
				catalogGenerationRetentionBytes

			published := catalog.storage.published.Load()
			require.EqualValues(t, 0, published)

			require.EqualValues(t, wantRetentionBytes, catalog.storage.total.Load())

			for _, cleanup := range cleanups[:count] {
				runCleanupPlan(t, catalog, cleanup)
			}

			require.EqualValues(t, 0, catalog.storage.total.Load())
		})
	}
}

func TestFunctionCatalogAbortRetainsInitializedGenerationStorage(
	t *testing.T,
) {
	const (
		population  = 9
		initialized = 3
	)

	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	declarations := testCleanupDeclarations(population)
	changes := make([]RouteChange, 0, population)
	for index := range declarations {
		declaration := &declarations[index]
		changes = append(changes, RouteChange{
			PublicName:  declaration.PublicName,
			Declaration: declaration,
		})
	}
	mutation, err := catalog.NewMutation(catalog.Census().Version, changes)
	require.NoError(t, err)
	builder, err := catalog.startMutation(mutation)
	require.NoError(t, err)
	for builder.phase == mutationTopology {

		_, prepareQuiesceStepErr := builder.PrepareQuiesceStep(MaximumMutationQuantum)
		require.NoError(t, prepareQuiesceStepErr)

	}

	_, _, prepareStepErr := builder.PrepareStep(initialized)
	require.NoError(t, prepareStepErr)

	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	count, err := builder.Abort(&cleanups)
	require.NoError(t, err)
	require.EqualValues(t, initialized, count)
	wantRetentionBytes := int64(initialized) *
		catalogGenerationRetentionBytes

	require.EqualValues(t, wantRetentionBytes, catalog.storage.total.Load())

	for _, cleanup := range cleanups[:count] {
		runCleanupPlan(t, catalog, cleanup)
	}

	require.EqualValues(t, 0, catalog.storage.total.Load())
}

func TestFunctionCatalogAtomicMutation(t *testing.T) {
	var oldCleanups atomic.Int32
	old := testGeneration("old-generation")
	old.Cleanup = func(context.Context) error {
		oldCleanups.Add(1)
		return nil
	}
	catalog, err := NewCatalog([]Declaration{
		testDeclarationForGeneration(old, "work", "", ResourcePolicy{}),
	})
	require.NoError(t, err)
	oldVersion := catalog.Census().Version
	oldDecision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "old", Route: "work"})
	require.NoError(t, err)

	short := testDeclarationForGeneration(testGeneration("short"), "config", "collector:", ResourcePolicy{})
	long := testDeclarationForGeneration(testGeneration("long"), "config", "collector:job:", ResourcePolicy{})
	invalid, err := catalog.NewMutation(oldVersion, []RouteChange{
		{PublicName: short.PublicName, Prefix: short.Prefix, Declaration: &short},
		{PublicName: long.PublicName, Prefix: long.Prefix, Declaration: &long},
	})
	require.NoError(t, err)
	invalidBuilder, err := catalog.startMutation(invalid)
	require.NoError(t, err)
	var mutationCleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
	for {
		_, _, prepareErr := invalidBuilder.PrepareStep(3)
		require.False(t, catalog.Census().Version != oldVersion || resolvedMethod(catalog, "work", nil) != "method")
		if prepareErr != nil {
			break
		}
	}

	abortCount, abortErr := invalidBuilder.Abort(&mutationCleanups)
	require.False(t, abortErr != nil || abortCount != 0)

	var newCleanups atomic.Int32
	replacementGeneration := testGeneration("new-generation")
	replacementGeneration.Cleanup = func(context.Context) error {
		newCleanups.Add(1)
		return nil
	}
	replacement := testDeclarationForGeneration(replacementGeneration, "work", "", ResourcePolicy{})
	mutation, err := catalog.NewMutation(oldVersion, []RouteChange{{
		PublicName: "work", Declaration: &replacement,
	}})
	require.NoError(t, err)
	builder, err := catalog.startMutation(mutation)
	require.NoError(t, err)
	var postimage *MutationPostimage
	for {
		var done bool
		postimage, done, err = builder.PrepareStep(3)
		require.NoError(t, err)
		require.False(t, catalog.Census().Version != oldVersion || resolvedMethod(catalog, "work", nil) != "method")
		if done {
			break
		}
	}
	count, err := catalog.commitMutation(postimage, &mutationCleanups)
	require.NoError(t, err)
	require.False(t, count != 0 || catalog.Census().Version != oldVersion+1 ||
		resolvedMethod(catalog, "work", nil) != "method")
	require.EqualValues(t, 0, oldCleanups.Load())
	oldCleanup, err := catalog.ReleaseInvocation(oldDecision.Lease)
	require.NoError(t, err)
	runCleanupPlan(t, catalog, oldCleanup)
	require.False(t, oldCleanups.Load() != 1 || newCleanups.Load() != 0)
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
		for index := range population {
			name := fmt.Sprintf("unrelated-%03d", index)
			declarations = append(declarations, testDeclaration(name, "", ResourcePolicy{}))
		}
		catalog, err := NewCatalog(declarations)
		require.NoError(t, err)
		prefix := strings.Repeat("p", 128)
		declaration := testDeclaration("config", prefix, ResourcePolicy{})
		mutation, err := catalog.NewMutation(catalog.Census().Version, []RouteChange{{
			PublicName: declaration.PublicName, Prefix: declaration.Prefix, Declaration: &declaration,
		}})
		require.NoError(t, err)
		builder, err := catalog.startMutation(mutation)
		require.NoError(t, err)
		previous := builder.Progress()
		turns := 0
		for {
			_, done, err := builder.PrepareStep(quantum)
			require.NoError(t, err)
			progress := builder.Progress()
			delta := progress.CompletedNodes - previous.CompletedNodes
			require.False(t, delta <= 0 || delta > quantum || progress.LastStepNodes != delta)

			_, ok := catalogRouteSet(catalog.routes, "config")
			require.False(t, ok)

			previous = progress
			turns++
			if done {
				require.EqualValues(t, progress.TotalNodes, progress.CompletedNodes)
				break
			}
		}
		var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan

		abortCount, abortErr := builder.Abort(&cleanups)
		require.False(t, abortErr != nil || abortCount != 0)

		return result{total: previous.TotalNodes, turns: turns}
	}

	small := run(t, 0)
	large := run(t, unrelatedRoutes)
	require.EqualValues(t, large, small)
}

func TestCatalogRejectsInvalidDeclarations(t *testing.T) {
	permit, err := lifecycle.NewJobLongLivedPlan(1)
	require.NoError(t, err)
	tests := map[string]Declaration{
		"missing handler": {
			ID: "method", Generation: &HandlerGenerationDeclaration{ID: "generation"},
			PublicName: "direct", Resource: ResourcePolicy{},
		},
		"invalid resource policy": {
			ID: "method", Generation: testGeneration("generation"),
			PublicName: "direct", Resource: ResourcePolicy{ScopePrefix: "resource:"},
		},
		"resource argument out of range": {
			ID: "method", Generation: testGeneration("generation"),
			PublicName: "direct",
			Resource:   ResourcePolicy{Argument: 1_024, Prefix: "job:"},
		},
		"transaction without resource policy": {
			ID:         "method",
			Generation: testGeneration("generation"),
			PublicName: "config",
			Prefix:     "job:",
			Transaction: &ResourceTransactionDeclaration{
				Prepare: func(
					context.Context,
					HandlerInput,
					lifecycle.ReadyResource,
					lifecycle.ResourceTransactionScope,
					lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return nil, nil
				},
				GlobalClaim: "claim",
				Commands:    []ResourceTransactionCommand{{Name: "get"}},
			},
		},
		"successor transaction with two permit sources": {
			ID:         "method",
			Generation: testGeneration("generation"),
			PublicName: "config",
			Prefix:     "job:",
			Resource:   DynCfgJobResource(0, "job:"),
			Transaction: &ResourceTransactionDeclaration{
				Prepare: func(
					context.Context,
					HandlerInput,
					lifecycle.ReadyResource,
					lifecycle.ResourceTransactionScope,
					lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return nil, nil
				},
				Permit:       permit,
				PermitPolicy: SuccessorPermitSecretStorePayload,
				GlobalClaim:  "claim",
				Commands: []ResourceTransactionCommand{
					{Name: "update", AllocateSuccessor: true},
				},
			},
		},
		"successor transaction without permit source": {
			ID:         "method",
			Generation: testGeneration("generation"),
			PublicName: "config",
			Prefix:     "job:",
			Resource:   DynCfgJobResource(0, "job:"),
			Transaction: &ResourceTransactionDeclaration{
				Prepare: func(
					context.Context,
					HandlerInput,
					lifecycle.ReadyResource,
					lifecycle.ResourceTransactionScope,
					lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return nil, nil
				},
				GlobalClaim: "claim",
				Commands: []ResourceTransactionCommand{
					{Name: "update", AllocateSuccessor: true},
				},
			},
		},
		"command repeats global claim": {
			ID:         "method",
			Generation: testGeneration("generation"),
			PublicName: "config",
			Prefix:     "job:",
			Resource:   DynCfgJobResource(0, "job:"),
			Transaction: &ResourceTransactionDeclaration{
				Prepare: func(
					context.Context,
					HandlerInput,
					lifecycle.ReadyResource,
					lifecycle.ResourceTransactionScope,
					lifecycle.LongLivedPermit,
				) (lifecycle.PreparedResourceTransaction, error) {
					return nil, nil
				},
				GlobalClaim: "claim",
				Commands: []ResourceTransactionCommand{{
					Name: "get", Claims: []string{"claim"},
				}},
			},
		},
		"duplicate direct": testDeclaration("direct", "", ResourcePolicy{}),
	}
	for name, declaration := range tests {
		t.Run(name, func(t *testing.T) {
			declarations := []Declaration{declaration}
			if name == "duplicate direct" {
				declarations = append(declarations, declaration)
			}

			_, err := NewCatalog(declarations)
			require.Error(t, err)
		})
	}
}

func TestCatalogRejectsPathStorageBeyondProcessBudgetBeforePublication(t *testing.T) {
	const pathCount = 10
	prefix := strings.Repeat("p", maximumDeclarationMetadataBytes)
	tests := map[string]func() error{
		"initial catalog": func() error {
			var declarations []Declaration
			for index := range pathCount {
				name := fmt.Sprintf("%d%s", index, strings.Repeat("n", maximumDeclarationMetadataBytes-1))
				declarations = append(
					declarations,
					testDeclaration(name, prefix, ResourcePolicy{}),
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
			for index := range pathCount {
				name := fmt.Sprintf("%d%s", index, strings.Repeat("n", maximumDeclarationMetadataBytes-1))
				declaration := testDeclaration(
					name,
					prefix,
					ResourcePolicy{},
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
			require.Error(t, run())
		})
	}
}

func TestCatalogPathStorageReturnsToPublishedPostimageAcrossChurn(t *testing.T) {
	catalog, err := NewCatalog(nil)
	require.NoError(t, err)
	publicName := strings.Repeat("n", 1_024)
	prefix := strings.Repeat("p", 1_024)
	apply := func(change RouteChange) {
		t.Helper()
		mutation, err := catalog.NewMutation(
			catalog.Census().Version,
			[]RouteChange{change},
		)
		require.NoError(t, err)
		builder, err := catalog.startMutation(mutation)
		require.NoError(t, err)
		var postimage *MutationPostimage
		for {
			var done bool
			postimage, done, err = builder.PrepareStep(MaximumMutationQuantum)
			require.NoError(t, err)
			if done {
				break
			}
		}
		var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
		count, err := catalog.commitMutation(postimage, &cleanups)
		require.NoError(t, err)
		for index := range count {
			runCleanupPlan(t, catalog, cleanups[index])
		}
	}

	for range 8 {
		declaration := testDeclaration(
			publicName,
			prefix,
			ResourcePolicy{},
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

		published := catalog.storage.published.Load()
		require.EqualValues(t, 0, published)

		total := catalog.storage.total.Load()
		require.EqualValues(t, 0, total)

		require.False(t, catalog.storage.preparation.Load())
	}
}

func BenchmarkBFunctionCatalogLookup(b *testing.B) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", ResourcePolicy{})})
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	lookup := jobmgr.FunctionLookup{UID: "benchmark", Route: "direct"}
	b.ReportAllocs()
	for b.Loop() {
		decision, err := catalog.ResolveAndAcquire(lookup)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func BenchmarkBHandlerLease(b *testing.B) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", ResourcePolicy{})})
	if err != nil {
		require.FailNow(b, "benchmark failed", err)
	}
	lookup := jobmgr.FunctionLookup{UID: "handler-lease", Route: "direct"}
	b.ReportAllocs()
	for b.Loop() {
		decision, err := catalog.ResolveAndAcquire(lookup)
		if err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
		if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
			require.FailNow(b, "benchmark failed", err)
		}
	}
}

func TestFunctionCatalogLookupAndHandlerLeaseAllocateNothing(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", ResourcePolicy{})})
	require.NoError(t, err)
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
	require.EqualValues(t, 0, allocations)
}

func testDeclaration(publicName, prefix string, resource ResourcePolicy) Declaration {
	return testDeclarationForGeneration(testGeneration(publicName), publicName, prefix, resource)
}

func testGeneration(id string) *HandlerGenerationDeclaration {
	return &HandlerGenerationDeclaration{ID: id, Handler: testHandler}
}

func testCleanupDeclarations(population int) []Declaration {
	declarations := make([]Declaration, 0, population)
	for index := range population {
		generation := testGeneration(fmt.Sprintf("cleanup-%02d", index))
		generation.Cleanup = func(context.Context) error { return nil }
		declarations = append(
			declarations,
			testDeclarationForGeneration(
				generation,
				fmt.Sprintf("route-%02d", index),
				"",
				ResourcePolicy{},
			),
		)
	}
	return declarations
}

func testDeclarationForGeneration(generation *HandlerGenerationDeclaration, publicName, prefix string, resource ResourcePolicy) Declaration {
	return Declaration{
		ID: "method", Generation: generation, PublicName: publicName, Prefix: prefix, Resource: resource,
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
	require.False(t, !cleanup.Ref.Valid() || cleanup.Runner == nil)

	_, err := cleanup.Runner.RunTask(context.Background())
	require.NoError(t, err)

	require.NoError(t, catalog.CompleteCleanup(cleanup.Ref))
}
