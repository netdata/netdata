// SPDX-License-Identifier: GPL-3.0-or-later

package functions

import (
	"context"
	"errors"
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
			declaration: testDeclaration(
				"config",
				"go.d:collector:",
				DynCfgJobResource(0, "go.d:collector:"),
			),
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
			declaration: testDeclaration(
				"config",
				"go.d:collector:",
				DynCfgJobResource(0, "go.d:collector:"),
			),
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
				ScopedDynCfgJobResource(
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
			wantResource: "secretstore:vault_production",
			wantMethod:   "method",
		},
		"scoped DynCfg add resource identity": {
			declaration: testDeclaration(
				"config",
				"go.d:vnode",
				ScopedDynCfgJobResource(0, "go.d:vnode", "vnode:"),
			),
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
				decision.ResourceID != test.wantResource {
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
			if err != nil {
				t.Fatal(err)
			}
			const population = 257
			leases := make([]jobmgr.FunctionInvocationRef, 0, population)
			for index := 0; index < population; index++ {
				decision, resolveErr := catalog.ResolveAndAcquire(
					test.lookup(index),
				)
				if resolveErr != nil {
					t.Fatalf("resolve invocation %d: %v", index, resolveErr)
				}
				if decision.Rejected != 0 || !decision.Lease.Valid() {
					t.Fatalf(
						"invocation %d rejected=%d lease=%+v",
						index,
						decision.Rejected,
						decision.Lease,
					)
				}
				leases = append(leases, decision.Lease)
			}
			for _, lease := range leases {
				if _, err := catalog.ReleaseInvocation(lease); err != nil {
					t.Fatal(err)
				}
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
			declaration := testDeclaration(
				"config",
				"job:",
				DynCfgJobResource(0, "job:"),
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
					{
						Name: "update", AllocateSuccessor: true,
						Claims: []string{"dyncfg:jobs"},
					},
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
				plan.ID != "mysql" ||
				plan.AllocateSuccessor != test.allocateSuccessor ||
				!reflect.DeepEqual(
					decision.Plan.Claims,
					test.wantClaims,
				) {
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

func TestFunctionCatalogDerivesSuccessorPermitFromInvocation(t *testing.T) {
	tests := map[string]struct {
		payload      []byte
		factoryError error
		wantBytes    int64
		wantError    bool
	}{
		"payload sizes the permit": {
			payload:   []byte(`{"option":"value"}`),
			wantBytes: 4_096 + lifecycle.TaskChildExecutionBytes,
		},
		"factory rejection owns no invocation": {
			payload:      []byte(`{"option":"value"}`),
			factoryError: errors.New("rejected permit"),
			wantError:    true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			declaration := testDeclaration(
				"config",
				"job:",
				DynCfgJobResource(0, "job:"),
			)
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
				PermitFor: func(input HandlerInput) (
					lifecycle.LongLivedPlan,
					error,
				) {
					if !reflect.DeepEqual(input.Payload, test.payload) {
						t.Fatalf("permit input=%q want=%q", input.Payload, test.payload)
					}
					if test.factoryError != nil {
						return lifecycle.LongLivedPlan{},
							test.factoryError
					}
					return lifecycle.NewJobLongLivedPlan(
						test.wantBytes -
							lifecycle.TaskChildExecutionBytes,
					)
				},
				CommandArgument: 1,
				GlobalClaim:     "dyncfg:graph",
				Commands: []ResourceTransactionCommand{
					{Name: "update", AllocateSuccessor: true},
				},
			}
			catalog, err := NewCatalog([]Declaration{declaration})
			if err != nil {
				t.Fatal(err)
			}
			decision, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID: "transaction", Route: "config",
					Args:    []string{"job:mysql", "update"},
					Payload: test.payload, HasPayload: true,
				},
			)
			if test.wantError {
				if !errors.Is(err, test.factoryError) {
					t.Fatalf("resolve error=%v want=%v", err, test.factoryError)
				}
				if census := catalog.LifecycleCensus(); census.InvocationLeases != 0 {
					t.Fatalf("failed permit factory retained lease: %+v", census)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if decision.Plan.Transaction == nil ||
				decision.Plan.Transaction.Permit.Bytes() !=
					test.wantBytes {
				t.Fatalf("transaction decision=%+v", decision)
			}
			if _, err := catalog.ReleaseInvocation(decision.Lease); err != nil {
				t.Fatal(err)
			}
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
			declaration := testDeclaration(
				"work",
				test.prefix,
				test.resource,
			)
			declaration.Generation.Cleanup = func(
				context.Context,
			) error {
				cleanupCalls.Add(1)
				return nil
			}
			catalog, err := NewCatalog([]Declaration{declaration})
			if err != nil {
				t.Fatal(err)
			}
			held, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "held",
					Route: "work",
					Args:  test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			mutation, err := catalog.NewMutation(
				catalog.Census().Version,
				[]RouteChange{{
					PublicName: "work",
					Prefix:     test.prefix,
				}},
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
			count, err := catalog.commitMutation(
				postimage,
				&cleanups,
			)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal(
					"retired generation cleaned before its lease drained",
				)
			}

			rejected, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "during-drain",
					Route: "work",
					Args:  test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if rejected.Rejected != lifecycle.ControlUnavailable ||
				rejected.Lease.Valid() {
				t.Fatalf(
					"retired route decision=%+v, want unavailable without lease",
					rejected,
				)
			}
			unknown, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "unknown",
					Route: "other",
					Args:  test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if unknown.Rejected != lifecycle.ControlNotFound {
				t.Fatalf(
					"unrelated route decision=%+v, want not found",
					unknown,
				)
			}

			cleanup, err := catalog.ReleaseInvocation(held.Lease)
			if err != nil {
				t.Fatal(err)
			}
			afterDrain, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "after-drain",
					Route: "work",
					Args:  test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if afterDrain.Rejected != lifecycle.ControlNotFound {
				t.Fatalf(
					"drained route decision=%+v, want not found",
					afterDrain,
				)
			}
			if published := catalog.storage.published.Load(); published != 0 {
				t.Fatalf(
					"drained tombstone retained %d path bytes",
					published,
				)
			}
			runCleanupPlan(t, catalog, cleanup)
			if cleanupCalls.Load() != 1 {
				t.Fatalf(
					"cleanup calls=%d, want 1",
					cleanupCalls.Load(),
				)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			held, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "held-sibling",
					Route: "sibling",
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			before := catalog.storage.published.Load()
			mutation, err := catalog.NewMutation(
				catalog.Census().Version,
				[]RouteChange{{
					PublicName: "work",
					Prefix:     test.prefix,
				}},
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
			count, err := catalog.commitMutation(
				postimage,
				&cleanups,
			)
			if err != nil {
				t.Fatal(err)
			}
			if count != 0 {
				t.Fatal("shared generation cleaned with one live route")
			}
			removed, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "removed",
					Route: "work",
					Args:  test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if removed.Rejected != lifecycle.ControlNotFound ||
				removed.Lease.Valid() {
				t.Fatalf(
					"lease-free retired route decision=%+v, want not found",
					removed,
				)
			}
			if published := catalog.storage.published.Load(); published >= before {
				t.Fatalf(
					"lease-free retired route retained path storage: before=%d after=%d",
					before,
					published,
				)
			}
			if cleanup, err := catalog.ReleaseInvocation(held.Lease); err != nil {
				t.Fatal(err)
			} else if cleanup.Ref.Valid() {
				t.Fatal("shared generation cleaned before sibling route retired")
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
			if err != nil {
				t.Fatal(err)
			}
			var oldRef jobmgr.FunctionCleanupRef
			for ref, generation := range catalog.generations {
				if generation.id == "old-shared" {
					oldRef = ref
					break
				}
			}
			if !oldRef.Valid() {
				t.Fatal("old shared generation is absent")
			}
			held, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID:   "held-old",
					Route: "work",
					Args:  test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			commit := func(change RouteChange) {
				t.Helper()
				mutation, mutationErr := catalog.NewMutation(
					catalog.Census().Version,
					[]RouteChange{change},
				)
				if mutationErr != nil {
					t.Fatal(mutationErr)
				}
				builder, mutationErr := catalog.startMutation(mutation)
				if mutationErr != nil {
					t.Fatal(mutationErr)
				}
				var postimage *MutationPostimage
				for {
					var done bool
					postimage, done, mutationErr = builder.PrepareStep(
						MaximumMutationQuantum,
					)
					if mutationErr != nil {
						t.Fatal(mutationErr)
					}
					if done {
						break
					}
				}
				var cleanups [MaximumMutationChanges]jobmgr.FunctionCleanupPlan
				if count, commitErr := catalog.commitMutation(
					postimage,
					&cleanups,
				); commitErr != nil || count != 0 {
					t.Fatalf(
						"mutation cleanup count=%d error=%v",
						count,
						commitErr,
					)
				}
			}
			commit(RouteChange{
				PublicName: "work",
				Prefix:     test.prefix,
			})
			replacement := testDeclaration(
				"work",
				test.prefix,
				test.resource,
			)
			commit(RouteChange{
				PublicName:  "work",
				Prefix:      test.prefix,
				Declaration: &replacement,
			})
			if census := catalog.Census(); census.Routes != 2 {
				t.Fatalf(
					"re-add route census=%+v, want two published routes",
					census,
				)
			}
			if resolvedMethod(catalog, "sibling", nil) == "" ||
				resolvedMethod(catalog, "work", test.args) == "" {
				t.Fatal("re-add lost sibling or replacement route")
			}
			cleanup, err := catalog.ReleaseInvocation(held.Lease)
			if err != nil {
				t.Fatal(err)
			}
			if cleanup.Ref.Valid() {
				t.Fatal(
					"shared generation cleaned while sibling route remained",
				)
			}
			if census := catalog.HandlerCensus(oldRef); census.RouteReferences != 1 ||
				census.InvocationLeases != 0 ||
				census.AdmissionClosed {
				t.Fatalf(
					"old shared generation census=%+v",
					census,
				)
			}
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
			if err != nil {
				t.Fatal(err)
			}
			mutation, err := catalog.NewMutation(
				catalog.Census().Version,
				[]RouteChange{{
					PublicName: "work",
					Prefix:     test.prefix,
				}},
			)
			if err != nil {
				t.Fatal(err)
			}
			if err := catalog.BeginMutation(mutation); err != nil {
				t.Fatal(err)
			}
			for {
				progress, err := catalog.AdvanceMutationQuiesce(
					MaximumMutationQuantum,
				)
				if err != nil {
					t.Fatal(err)
				}
				if progress.Quiesced {
					break
				}
			}
			rejected, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID: "quiesced", Route: "work", Args: test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if rejected.Rejected != lifecycle.ControlUnavailable ||
				rejected.Lease.Valid() ||
				catalog.Census().Version != 1 {
				t.Fatalf(
					"quiesced decision=%+v census=%+v",
					rejected,
					catalog.Census(),
				)
			}
			if err := catalog.ResumeMutation(mutation); err != nil {
				t.Fatal(err)
			}
			var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
			if count, err := catalog.AbortMutation(
				&cleanups,
			); err != nil || count != 0 {
				t.Fatalf("abort count=%d err=%v", count, err)
			}
			restored, err := catalog.ResolveAndAcquire(
				jobmgr.FunctionLookup{
					UID: "restored", Route: "work", Args: test.args,
				},
			)
			if err != nil {
				t.Fatal(err)
			}
			if restored.Rejected != 0 || !restored.Lease.Valid() ||
				catalog.Census().Version != 1 ||
				catalog.Census().MutationActive {
				t.Fatalf(
					"restored decision=%+v census=%+v",
					restored,
					catalog.Census(),
				)
			}
			if _, err := catalog.ReleaseInvocation(
				restored.Lease,
			); err != nil {
				t.Fatal(err)
			}
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
	if err != nil {
		t.Fatal(err)
	}
	held, err := catalog.ResolveAndAcquire(
		jobmgr.FunctionLookup{UID: "held", Route: "held"},
	)
	if err != nil {
		t.Fatal(err)
	}
	remove, err := catalog.NewMutation(
		catalog.Census().Version,
		[]RouteChange{{PublicName: "held"}},
	)
	if err != nil {
		t.Fatal(err)
	}
	removeBuilder, err := catalog.startMutation(remove)
	if err != nil {
		t.Fatal(err)
	}
	var removePostimage *MutationPostimage
	for {
		var done bool
		removePostimage, done, err = removeBuilder.PrepareStep(
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
	if count, err := catalog.commitMutation(
		removePostimage,
		&cleanups,
	); err != nil || count != 0 {
		t.Fatalf(
			"held-route retirement count=%d err=%v",
			count,
			err,
		)
	}

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
	if err != nil {
		t.Fatal(err)
	}
	unrelatedBuilder, err := catalog.startMutation(unrelated)
	if err != nil {
		t.Fatal(err)
	}
	for unrelatedBuilder.phase == mutationTopology {
		if _, _, err := unrelatedBuilder.PrepareStep(1); err != nil {
			t.Fatal(err)
		}
	}

	cleanup, err := catalog.ReleaseInvocation(held.Lease)
	if err != nil {
		t.Fatal(err)
	}
	if catalog.deferredPrune == nil {
		t.Fatal(
			"lease drain physically pruned a route during another mutation",
		)
	}
	afterDrain, err := catalog.ResolveAndAcquire(
		jobmgr.FunctionLookup{
			UID:   "after-drain",
			Route: "held",
		},
	)
	if err != nil {
		t.Fatal(err)
	}
	if afterDrain.Rejected != lifecycle.ControlNotFound {
		t.Fatalf(
			"semantically drained route decision=%+v, want not found",
			afterDrain,
		)
	}

	var unrelatedPostimage *MutationPostimage
	for {
		var done bool
		unrelatedPostimage, done, err =
			unrelatedBuilder.PrepareStep(
				MaximumMutationQuantum,
			)
		if err != nil {
			t.Fatal(err)
		}
		if done {
			break
		}
	}
	count, err := catalog.commitMutation(
		unrelatedPostimage,
		&cleanups,
	)
	if err != nil {
		t.Fatal(err)
	}
	for index := 0; index < count; index++ {
		runCleanupPlan(t, catalog, cleanups[index])
	}
	if catalog.deferredPrune != nil {
		t.Fatal("unrelated mutation retained deferred route pruning")
	}
	published := catalog.storage.published.Load()
	if want := catalogPathStorage(catalog.routes); published != want {
		t.Fatalf(
			"published storage=%d, live trie=%d",
			published,
			want,
		)
	}
	runCleanupPlan(t, catalog, cleanup)
	if cleanupCalls.Load() != 1 {
		t.Fatalf(
			"cleanup calls=%d, want 1",
			cleanupCalls.Load(),
		)
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
		testDeclarationForGeneration(generation, "config", "job:", ResourcePolicy{}),
		testDeclarationForGeneration(generation, "config", "store:", ResourcePolicy{}),
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

func TestFunctionCatalogRetainsCleanupExecutionStorageUntilCompletion(
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
				if err != nil {
					t.Fatal(err)
				}
				return catalog
			},
		},
		"committed mutation generations": {
			newCatalog: func(t *testing.T, declarations []Declaration) *Catalog {
				t.Helper()
				catalog, err := NewCatalog(nil)
				if err != nil {
					t.Fatal(err)
				}
				changes := make([]RouteChange, 0, len(declarations))
				for index := range declarations {
					declaration := &declarations[index]
					changes = append(changes, RouteChange{
						PublicName:  declaration.PublicName,
						Declaration: declaration,
					})
				}
				mutation, err := catalog.NewMutation(
					catalog.Census().Version,
					changes,
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
				var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
				if count, err := catalog.commitMutation(
					postimage,
					&cleanups,
				); err != nil || count != 0 {
					t.Fatalf(
						"new-generation mutation cleanup count=%d err=%v",
						count,
						err,
					)
				}
				return catalog
			},
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			catalog := test.newCatalog(
				t,
				testCleanupDeclarations(population),
			)
			if err := catalog.BeginClose(); err != nil {
				t.Fatal(err)
			}
			var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
			count, more, err := catalog.CloseStep(
				MaximumCloseQuantum,
				&cleanups,
			)
			if err != nil {
				t.Fatal(err)
			}
			if count != population || more {
				t.Fatalf(
					"catalog close cleanup count=%d more=%v, want %d,false",
					count,
					more,
					population,
				)
			}
			wantExecutionBytes := int64(population) *
				lifecycle.TaskChildExecutionBytes
			if published := catalog.storage.published.Load(); published != 0 {
				t.Fatalf("closed catalog retained %d published path bytes", published)
			}
			if total := catalog.storage.total.Load(); total != wantExecutionBytes {
				t.Fatalf(
					"pending cleanup storage=%d, want %d",
					total,
					wantExecutionBytes,
				)
			}
			for _, cleanup := range cleanups[:count] {
				runCleanupPlan(t, catalog, cleanup)
			}
			if total := catalog.storage.total.Load(); total != 0 {
				t.Fatalf("completed cleanup retained %d storage bytes", total)
			}
		})
	}
}

func TestFunctionCatalogAbortRetainsInitializedCleanupExecutionStorage(
	t *testing.T,
) {
	const (
		population  = 9
		initialized = 3
	)

	catalog, err := NewCatalog(nil)
	if err != nil {
		t.Fatal(err)
	}
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
	if err != nil {
		t.Fatal(err)
	}
	builder, err := catalog.startMutation(mutation)
	if err != nil {
		t.Fatal(err)
	}
	for builder.phase == mutationTopology {
		if _, err := builder.PrepareQuiesceStep(
			MaximumMutationQuantum,
		); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, err := builder.PrepareStep(initialized); err != nil {
		t.Fatal(err)
	}
	var cleanups [jobmgr.MaximumFunctionCleanupBatch]jobmgr.FunctionCleanupPlan
	count, err := builder.Abort(&cleanups)
	if err != nil {
		t.Fatal(err)
	}
	if count != initialized {
		t.Fatalf("aborted initialized cleanup count=%d, want %d", count, initialized)
	}
	wantExecutionBytes := int64(initialized) *
		lifecycle.TaskChildExecutionBytes
	if total := catalog.storage.total.Load(); total != wantExecutionBytes {
		t.Fatalf(
			"aborted cleanup storage=%d, want %d",
			total,
			wantExecutionBytes,
		)
	}
	for _, cleanup := range cleanups[:count] {
		runCleanupPlan(t, catalog, cleanup)
	}
	if total := catalog.storage.total.Load(); total != 0 {
		t.Fatalf("completed aborted cleanup retained %d storage bytes", total)
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
		testDeclarationForGeneration(old, "work", "", ResourcePolicy{}),
	})
	if err != nil {
		t.Fatal(err)
	}
	oldVersion := catalog.Census().Version
	oldDecision, err := catalog.ResolveAndAcquire(jobmgr.FunctionLookup{UID: "old", Route: "work"})
	if err != nil {
		t.Fatal(err)
	}

	short := testDeclarationForGeneration(testGeneration("short"), "config", "collector:", ResourcePolicy{})
	long := testDeclarationForGeneration(testGeneration("long"), "config", "collector:job:", ResourcePolicy{})
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
	replacement := testDeclarationForGeneration(replacementGeneration, "work", "", ResourcePolicy{})
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
			declarations = append(declarations, testDeclaration(name, "", ResourcePolicy{}))
		}
		catalog, err := NewCatalog(declarations)
		if err != nil {
			t.Fatal(err)
		}
		prefix := strings.Repeat("p", 128)
		declaration := testDeclaration("config", prefix, ResourcePolicy{})
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
	permit, err := lifecycle.NewJobLongLivedPlan(1)
	if err != nil {
		t.Fatal(err)
	}
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
				Permit: permit,
				PermitFor: func(HandlerInput) (
					lifecycle.LongLivedPlan,
					error,
				) {
					return permit, nil
				},
				GlobalClaim: "claim",
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
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", ResourcePolicy{})})
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
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", ResourcePolicy{})})
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

func TestFunctionCatalogLookupAndHandlerLeaseAllocateNothing(t *testing.T) {
	catalog, err := NewCatalog([]Declaration{testDeclaration("direct", "", ResourcePolicy{})})
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

func testDeclaration(publicName, prefix string, resource ResourcePolicy) Declaration {
	return testDeclarationForGeneration(testGeneration(publicName), publicName, prefix, resource)
}

func testGeneration(id string) *HandlerGenerationDeclaration {
	return &HandlerGenerationDeclaration{ID: id, Handler: testHandler}
}

func testCleanupDeclarations(population int) []Declaration {
	declarations := make([]Declaration, 0, population)
	for index := 0; index < population; index++ {
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
