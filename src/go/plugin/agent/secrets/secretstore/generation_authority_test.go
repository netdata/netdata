// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
	"github.com/stretchr/testify/require"
)

func newGenerationTestSecretStore(t testing.TB) *SecretStore {
	t.Helper()
	resolver, err := secretresolver.NewAtomicResolver(nil)
	if err != nil {
		t.Fatal(err)
	}
	store, err := NewSecretStore(resolver)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

func TestSecretStoreLeaseRetirementAndDynamicPopulation(t *testing.T) {
	const population = 9
	store := newGenerationTestSecretStore(t)
	catalog := newGenerationTestCatalog(t)

	for index := range population {
		key := fmt.Sprintf("store-%d", index)
		mutation, err := store.PrepareMutation(
			t.Context(),
			catalog,
			generationTestConfig(key, "initial"),
			0,
		)
		if err != nil {
			t.Fatalf("prepare %s: %v", key, err)
		}
		if result, err := mutation.Commit(t.Context()); err != nil ||
			!result.Applied ||
			result.Generation != uint64(index+1) {
			t.Fatalf("commit %s: result=%+v err=%v", key, result, err)
		}
	}
	if census := store.Census(); census.Current != population ||
		census.Generations != population ||
		census.Preparations != 0 {
		t.Fatalf("initial census=%+v", census)
	}

	scopes := make([]*ResolutionScope, population)
	for index := range population {
		key := StoreKey(KindVault, fmt.Sprintf("store-%d", index))
		scope, err := store.AcquireScope([]string{key, key})
		if err != nil {
			t.Fatalf("scope %s: %v", key, err)
		}
		scopes[index] = scope
	}
	if census := store.Census(); census.Scopes != population ||
		census.Readers != population {
		t.Fatalf("scope census=%+v", census)
	}

	key := StoreKey(KindVault, "store-0")
	mutation, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("store-0", "replacement"),
		1,
	)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := mutation.Commit(t.Context()); err != nil ||
		!result.Applied ||
		result.Generation != population+1 {
		t.Fatalf("replacement commit=%+v err=%v", result, err)
	}
	value, err := scopes[0].Resolve(t.Context(), key, "key")
	if err != nil || string(value) != "initial" {
		t.Fatalf("old scope resolve=%q err=%v", value, err)
	}
	fresh, err := store.AcquireScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	value, err = fresh.Resolve(t.Context(), key, "key")
	if err != nil || string(value) != "replacement" {
		t.Fatalf("fresh scope resolve=%q err=%v", value, err)
	}
	if err := scopes[0].Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	for _, scope := range scopes[1:] {
		if err := scope.Release(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if err := fresh.Release(t.Context()); err != nil {
		t.Fatal(err)
	}

	for index := range population {
		key := StoreKey(KindVault, fmt.Sprintf("store-%d", index))
		generation := uint64(index + 1)
		if index == 0 {
			generation = population + 1
		}
		if err := store.Retire(t.Context(), key, generation); err != nil {
			t.Fatalf("retire %s: %v", key, err)
		}
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
	if census := store.Census(); census != (SecretStoreCensus{Closed: true}) {
		t.Fatalf("terminal census=%+v", census)
	}
}

func TestResolutionScopeSnapshotTracksUpdateAndRemoval(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	catalog := newGenerationTestCatalog(t)
	key := StoreKey(KindVault, "main")

	initial, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "initial"),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	initialResult, err := initial.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initialScope, err := store.AcquireScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	initialSnapshot := initialScope.Snapshot()
	if initialSnapshot == nil || !initialSnapshot.Current() {
		t.Fatal("current Store generation produced a stale snapshot")
	}

	replacement, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "replacement"),
		initialResult.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	replacementResult, err := replacement.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if initialSnapshot.Current() {
		t.Fatal("superseded Store generation remained current")
	}
	replacementScope, err := store.AcquireScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}
	replacementSnapshot := replacementScope.Snapshot()
	if replacementSnapshot == nil || !replacementSnapshot.Current() {
		t.Fatal("replacement Store generation produced a stale snapshot")
	}

	removal, err := store.PrepareRemoval(key, replacementResult.Generation)
	if err != nil {
		t.Fatal(err)
	}
	if result, err := removal.Commit(t.Context()); err != nil || !result.Applied {
		t.Fatalf("removal result=%+v err=%v", result, err)
	}
	if replacementSnapshot.Current() {
		t.Fatal("removed Store generation remained current")
	}

	if err := initialScope.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := replacementScope.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestPreparedSecretMutationMatrix(t *testing.T) {
	tests := map[string]struct {
		action       string
		cancelCommit bool
		wantCurrent  int
	}{
		"commit transfers generation": {
			action:      "commit",
			wantCurrent: 1,
		},
		"abort disposes preparation": {
			action: "abort",
		},
		"cancelled commit disposes preparation": {
			action:       "commit",
			cancelCommit: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := newGenerationTestSecretStore(t)
			mutation, err := store.PrepareMutation(
				t.Context(),
				newGenerationTestCatalog(t),
				generationTestConfig("main", "value"),
				0,
			)
			if err != nil {
				t.Fatal(err)
			}
			switch test.action {
			case "abort":
				err = mutation.Abort()
			case "commit":
				ctx := t.Context()
				if test.cancelCommit {
					cancelled, cancel := context.WithCancel(ctx)
					cancel()
					ctx = cancelled
				}
				_, err = mutation.Commit(ctx)
			default:
				t.Fatalf("unknown action %q", test.action)
			}
			if test.cancelCommit && !errors.Is(err, context.Canceled) {
				t.Fatalf("commit error=%v want cancellation", err)
			}
			if !test.cancelCommit && err != nil {
				t.Fatal(err)
			}
			if census := store.Census(); census.Current != test.wantCurrent ||
				census.Preparations != 0 {
				t.Fatalf("census=%+v", census)
			}
			if test.wantCurrent == 1 {
				if err := store.Retire(
					t.Context(),
					StoreKey(KindVault, "main"),
					1,
				); err != nil {
					t.Fatal(err)
				}
			}
			if err := store.Close(t.Context()); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestPreparedSecretMutationAliasesShareLinearState(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	mutation, err := store.PrepareMutation(
		t.Context(),
		newGenerationTestCatalog(t),
		generationTestConfig("main", "value"),
		0,
	)
	require.NoError(t, err)
	alias := mutation
	require.True(t, mutation.Valid())
	require.True(t, alias.Valid())

	require.NoError(t, alias.Abort())

	require.False(t, mutation.Valid())
	require.Error(t, mutation.Abort())
	require.Zero(t, store.Census().Preparations)
	require.NoError(t, store.Close(t.Context()))
}

func TestSecretStorePreparationOwnershipRegressions(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T)
	}{
		"closed Store rejects mutation without retaining preparation": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				if err := store.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
				if _, err := store.PrepareMutation(
					t.Context(),
					newGenerationTestCatalog(t),
					generationTestConfig("main", "value"),
					0,
				); err == nil {
					t.Fatal("closed Store accepted mutation preparation")
				}
				if census := store.Census(); census != (SecretStoreCensus{Closed: true}) {
					t.Fatalf("closed Store retained mutation state: %+v", census)
				}
			},
		},
		"accepted preparation failure returns owned abort token": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				mutation, err := store.PrepareMutation(
					t.Context(),
					newGenerationTestCatalog(t),
					generationTestConfig("main", "invalid"),
					0,
				)
				if err == nil || !mutation.Valid() {
					t.Fatalf(
						"failed preparation mutation valid=%v error=%v",
						mutation.Valid(),
						err,
					)
				}
				if census := store.Census(); census.Preparations != 1 {
					t.Fatalf("failed preparation census=%+v", census)
				}
				if err := mutation.Abort(); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
		"removal preparation appears in exact census": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				mutation, err := store.PrepareMutation(
					t.Context(),
					newGenerationTestCatalog(t),
					generationTestConfig("main", "value"),
					0,
				)
				if err != nil {
					t.Fatal(err)
				}
				result, err := mutation.Commit(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				removal, err := store.PrepareRemoval(
					StoreKey(KindVault, "main"),
					result.Generation,
				)
				if err != nil {
					t.Fatal(err)
				}
				if census := store.Census(); census.Preparations != 1 {
					t.Fatalf("removal preparation census=%+v", census)
				}
				if err := removal.Abort(); err != nil {
					t.Fatal(err)
				}
				if err := store.Retire(
					t.Context(),
					StoreKey(KindVault, "main"),
					result.Generation,
				); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
		"delete and recreate cannot satisfy stale generation": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				catalog := newGenerationTestCatalog(t)
				initial, err := store.PrepareMutation(
					t.Context(),
					catalog,
					generationTestConfig("main", "initial"),
					0,
				)
				if err != nil {
					t.Fatal(err)
				}
				initialResult, err := initial.Commit(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				stale, err := store.PrepareMutation(
					t.Context(),
					catalog,
					generationTestConfig("main", "stale"),
					initialResult.Generation,
				)
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Retire(
					t.Context(),
					StoreKey(KindVault, "main"),
					initialResult.Generation,
				); err != nil {
					t.Fatal(err)
				}
				recreated, err := store.PrepareMutation(
					t.Context(),
					catalog,
					generationTestConfig("main", "recreated"),
					0,
				)
				if err != nil {
					t.Fatal(err)
				}
				recreatedResult, err := recreated.Commit(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if recreatedResult.Generation ==
					initialResult.Generation {
					t.Fatalf(
						"recreated generation=%d reused retired generation",
						recreatedResult.Generation,
					)
				}
				staleResult, staleErr := stale.Commit(t.Context())
				if staleErr == nil || staleResult.Applied {
					t.Fatalf(
						"stale mutation result=%+v error=%v",
						staleResult,
						staleErr,
					)
				}
				if err := store.Retire(
					t.Context(),
					StoreKey(KindVault, "main"),
					recreatedResult.Generation,
				); err != nil {
					t.Fatal(err)
				}
				if err := store.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
		"absent delete recreate cannot satisfy stale preparation": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				catalog := newGenerationTestCatalog(t)
				stale, err := store.PrepareMutation(
					t.Context(),
					catalog,
					generationTestConfig("main", "stale"),
					0,
				)
				if err != nil {
					t.Fatal(err)
				}
				intermediate, err := store.PrepareMutation(
					t.Context(),
					catalog,
					generationTestConfig("main", "intermediate"),
					0,
				)
				if err != nil {
					t.Fatal(err)
				}
				intermediateResult, err := intermediate.Commit(t.Context())
				if err != nil {
					t.Fatal(err)
				}
				if err := store.Retire(
					t.Context(),
					StoreKey(KindVault, "main"),
					intermediateResult.Generation,
				); err != nil {
					t.Fatal(err)
				}
				result, err := stale.Commit(t.Context())
				if err == nil || result.Applied {
					t.Fatalf(
						"absent-state stale mutation result=%+v error=%v",
						result,
						err,
					)
				}
				if err := store.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
			},
		},
	}
	for name, test := range tests {
		t.Run(name, test.run)
	}
}

func TestSecretStoreCloseRejectsRetainedScope(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	mutation, err := store.PrepareMutation(
		t.Context(),
		newGenerationTestCatalog(t),
		generationTestConfig("main", "value"),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := mutation.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	scope, err := store.AcquireScope(
		[]string{StoreKey(KindVault, "main")},
	)
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retire(
		t.Context(),
		StoreKey(KindVault, "main"),
		1,
	); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(t.Context()); err == nil {
		t.Fatal("close acknowledged a retained reader scope")
	}
	if census := store.Census(); !census.Closing ||
		census.Readers != 1 ||
		census.Retiring != 1 {
		t.Fatalf("retained close census=%+v", census)
	}
	if err := scope.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSecretStoreSealFencesMutationAndClosesAfterRetainedScopeDrains(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	catalog := newGenerationTestCatalog(t)
	current, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "current"),
		0,
	)
	require.NoError(t, err)
	currentResult, err := current.Commit(t.Context())
	require.NoError(t, err)

	key := StoreKey(KindVault, "main")
	scope, err := store.AcquireScope([]string{key})
	require.NoError(t, err)
	late, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "late"),
		currentResult.Generation,
	)
	require.NoError(t, err)

	require.NoError(t, store.Seal())
	select {
	case <-store.Done():
		t.Fatal("sealed Store closed while it retained a scope and preparation")
	default:
	}

	lateResult, lateErr := late.Commit(t.Context())
	require.Error(t, lateErr)
	require.False(t, lateResult.Applied)
	require.Nil(t, late.owner)
	require.NoError(t, store.Retire(t.Context(), key, currentResult.Generation))

	value, err := scope.Resolve(t.Context(), key, "key")
	require.NoError(t, err)
	require.Equal(t, "current", string(value))
	select {
	case <-store.Done():
		t.Fatal("sealed Store closed while its retiring generation was reader-pinned")
	default:
	}

	require.NoError(t, scope.Release(t.Context()))
	require.Nil(t, scope.owner)
	require.Nil(t, scope.pins)
	select {
	case <-store.Done():
	case <-time.After(time.Second):
		t.Fatal("sealed Store did not close after its retained ownership drained")
	}
	require.Equal(t, SecretStoreCensus{Closed: true}, store.Census())
}

func BenchmarkBSecretStoreLease(b *testing.B) {
	store := newGenerationTestSecretStore(b)
	mutation, err := store.PrepareMutation(
		context.Background(),
		newGenerationTestCatalog(b),
		generationTestConfig("main", "value"),
		0,
	)
	if err != nil {
		b.Fatal(err)
	}
	if _, err := mutation.Commit(context.Background()); err != nil {
		b.Fatal(err)
	}
	key := StoreKey(KindVault, "main")
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		scope, err := store.AcquireScope([]string{key})
		if err != nil {
			b.Fatal(err)
		}
		if err := scope.Release(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkBSecretMutationControl(b *testing.B) {
	for range b.N {
		store := newGenerationTestSecretStore(b)
		mutation, err := store.PrepareMutation(
			context.Background(),
			newGenerationTestCatalog(b),
			generationTestConfig("main", "value"),
			0,
		)
		if err != nil {
			b.Fatal(err)
		}
		if err := mutation.Abort(); err != nil {
			b.Fatal(err)
		}
	}
}

func TestSecretStoreRemovalTombstonesTwoReaderPinnedGenerations(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	catalog := newGenerationTestCatalog(t)
	key := StoreKey(KindVault, "main")

	initial, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "initial"),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	initialResult, err := initial.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initialScope, err := store.AcquireScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}

	replacement, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "replacement"),
		initialResult.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	replacementResult, err := replacement.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	replacementScope, err := store.AcquireScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}

	removal, err := store.PrepareRemoval(key, replacementResult.Generation)
	if err != nil {
		t.Fatal(err)
	}
	removalResult, err := removal.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if !removalResult.Applied {
		t.Fatal("removal did not apply")
	}
	if generation := store.Generation(key); generation != 0 {
		t.Fatalf("removed Store remained admitted as generation %d", generation)
	}
	if _, ok := store.Config(key); ok {
		t.Fatal("removed Store configuration remained admitted")
	}
	if _, err := store.AcquireScope([]string{key}); err == nil {
		t.Fatal("removed Store admitted a new scope")
	}
	census := store.Census()
	if census.Current != 0 ||
		census.Retiring != 2 ||
		census.Generations != 2 ||
		census.Readers != 2 {
		t.Fatalf("removed Store retained unexpected ownership: %+v", census)
	}
	ready := store.MutationReady(key)
	select {
	case <-ready:
		t.Fatal("mutation became ready while two generations were retiring")
	default:
	}

	if err := initialScope.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
		t.Fatal("mutation became ready while one generation was still retiring")
	default:
	}
	if err := replacementScope.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ready:
	case <-time.After(time.Second):
		t.Fatal("mutation readiness did not follow final retirement")
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestSecretStoreRetireCurrentWhilePriorGenerationIsReaderPinned(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	catalog := newGenerationTestCatalog(t)
	key := StoreKey(KindVault, "main")

	initial, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "initial"),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	initialResult, err := initial.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	initialScope, err := store.AcquireScope([]string{key})
	if err != nil {
		t.Fatal(err)
	}

	replacement, err := store.PrepareMutation(
		t.Context(),
		catalog,
		generationTestConfig("main", "replacement"),
		initialResult.Generation,
	)
	if err != nil {
		t.Fatal(err)
	}
	replacementResult, err := replacement.Commit(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if err := store.Retire(t.Context(), key, replacementResult.Generation); err != nil {
		t.Fatal(err)
	}
	census := store.Census()
	if census.Current != 0 ||
		census.Retiring != 1 ||
		census.Generations != 1 ||
		census.Readers != 1 {
		t.Fatalf("Store retirement retained unexpected ownership: %+v", census)
	}

	if err := initialScope.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

type generationTestStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (store *generationTestStore) Configuration() any {
	return &store.config
}

func (store *generationTestStore) Init(context.Context) error {
	if store.config.Value == "invalid" {
		return errors.New("invalid Store value")
	}
	return nil
}

func (store *generationTestStore) Publish() PublishedStore {
	return generationTestPublished(store.config.Value)
}

type generationTestPublished string

func (published generationTestPublished) Resolve(
	context.Context,
	ResolveRequest,
) (string, error) {
	return string(published), nil
}

func newGenerationTestCatalog(t testing.TB) *CreatorCatalog {
	t.Helper()
	catalog, err := NewCreatorCatalog([]Creator{{
		Kind:   KindVault,
		Schema: `{}`,
		Create: func() Store {
			return &generationTestStore{}
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	return catalog
}

func generationTestConfig(name string, value string) Config {
	return Config{
		"name":            name,
		"kind":            string(KindVault),
		"value":           value,
		"__source__":      confgroup.TypeDyncfg,
		"__source_type__": confgroup.TypeDyncfg,
	}
}
