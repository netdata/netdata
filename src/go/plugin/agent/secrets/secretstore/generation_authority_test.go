// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
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
	carriers := make([]*generationTestCarrier, population)

	for index := range population {
		key := fmt.Sprintf("store-%d", index)
		carriers[index] = &generationTestCarrier{}
		mutation, err := store.PrepareMutation(
			t.Context(),
			catalog,
			carriers[index],
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

	replacement := &generationTestCarrier{}
	key := StoreKey(KindVault, "store-0")
	mutation, err := store.PrepareMutation(
		t.Context(),
		catalog,
		replacement,
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
	if carriers[0].released {
		t.Fatal("reader-pinned retiring generation released early")
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
	if !carriers[0].released {
		t.Fatal("drained retiring generation retained its carrier")
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
	for index, carrier := range carriers {
		if index == 0 {
			continue
		}
		if !carrier.released {
			t.Fatalf("carrier %d was not released", index)
		}
	}
	if !replacement.released {
		t.Fatal("replacement carrier was not released")
	}
}

func TestPreparedSecretMutationMatrix(t *testing.T) {
	tests := map[string]struct {
		action        string
		cancelCommit  bool
		wantCurrent   int
		wantActivated bool
		wantReleased  bool
	}{
		"commit transfers generation": {
			action:        "commit",
			wantCurrent:   1,
			wantActivated: true,
		},
		"abort disposes preparation": {
			action:       "abort",
			wantReleased: true,
		},
		"cancelled commit disposes preparation": {
			action:       "commit",
			cancelCommit: true,
			wantReleased: true,
		},
	}
	for name, test := range tests {
		t.Run(name, func(t *testing.T) {
			store := newGenerationTestSecretStore(t)
			carrier := &generationTestCarrier{}
			mutation, err := store.PrepareMutation(
				t.Context(),
				newGenerationTestCatalog(t),
				carrier,
				generationTestConfig("main", "value"),
				0,
			)
			if err != nil {
				t.Fatal(err)
			}
			if carrier.activated {
				t.Fatal(
					"preparation activated the generation carrier before commit",
				)
			}
			retainedBytes, err :=
				mutation.RetainedResourceBytes()
			if err != nil {
				t.Fatal(err)
			}
			if retainedBytes <= 1_024 {
				t.Fatalf(
					"retained-byte estimate=%d is not conservative",
					retainedBytes,
				)
			}
			switch test.action {
			case "abort":
				err = mutation.Abort(t.Context())
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
			if carrier.released != test.wantReleased {
				t.Fatalf(
					"carrier released=%v want=%v",
					carrier.released,
					test.wantReleased,
				)
			}
			if carrier.activated != test.wantActivated {
				t.Fatalf(
					"carrier activated=%v want=%v",
					carrier.activated,
					test.wantActivated,
				)
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

func TestSecretStorePreparationOwnershipRegressions(t *testing.T) {
	tests := map[string]struct {
		run func(*testing.T)
	}{
		"rejected reservation releases supplied carrier": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				if err := store.Close(t.Context()); err != nil {
					t.Fatal(err)
				}
				carrier := &generationTestCarrier{}
				if _, err := store.PrepareMutation(
					t.Context(),
					newGenerationTestCatalog(t),
					carrier,
					generationTestConfig("main", "value"),
					0,
				); err == nil {
					t.Fatal("closed Store accepted mutation preparation")
				}
				if !carrier.released {
					t.Fatal("rejected mutation retained the supplied carrier")
				}
			},
		},
		"removal preparation appears in exact census": {
			run: func(t *testing.T) {
				store := newGenerationTestSecretStore(t)
				carrier := &generationTestCarrier{}
				mutation, err := store.PrepareMutation(
					t.Context(),
					newGenerationTestCatalog(t),
					carrier,
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
				if err := removal.Abort(t.Context()); err != nil {
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
				initialCarrier := &generationTestCarrier{}
				initial, err := store.PrepareMutation(
					t.Context(),
					catalog,
					initialCarrier,
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
				staleCarrier := &generationTestCarrier{}
				stale, err := store.PrepareMutation(
					t.Context(),
					catalog,
					staleCarrier,
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
				recreatedCarrier := &generationTestCarrier{}
				recreated, err := store.PrepareMutation(
					t.Context(),
					catalog,
					recreatedCarrier,
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
				if !staleCarrier.released {
					t.Fatal("rejected stale mutation retained its carrier")
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
	}
	for name, test := range tests {
		t.Run(name, test.run)
	}
}

func TestSecretStoreCloseRejectsRetainedScope(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	carrier := &generationTestCarrier{}
	mutation, err := store.PrepareMutation(
		t.Context(),
		newGenerationTestCatalog(t),
		carrier,
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

func TestSecretStoreCommitRejectsUnderCapacityCarrier(t *testing.T) {
	store := newGenerationTestSecretStore(t)
	carrier := &generationTestCarrier{capacity: 1}
	mutation, err := store.PrepareMutation(
		t.Context(),
		newGenerationTestCatalog(t),
		carrier,
		generationTestConfig("main", "value"),
		0,
	)
	if err != nil {
		t.Fatal(err)
	}
	required, err := mutation.RetainedResourceBytes()
	if err != nil {
		t.Fatal(err)
	}
	if required <= carrier.capacity {
		t.Fatalf(
			"retained bytes=%d do not exceed carrier capacity=%d",
			required,
			carrier.capacity,
		)
	}
	result, err := mutation.Commit(t.Context())
	if err == nil ||
		!strings.Contains(
			err.Error(),
			"below retained state",
		) {
		t.Fatalf("under-capacity commit result=%+v error=%v", result, err)
	}
	if result.Applied || result.Retained ||
		carrier.activated ||
		!carrier.released {
		t.Fatalf(
			"under-capacity carrier result=%+v carrier=%+v",
			result,
			carrier,
		)
	}
	if census := store.Census(); census !=
		(SecretStoreCensus{}) {
		t.Fatalf(
			"under-capacity rejection retained ownership: %+v",
			census,
		)
	}
	if err := store.Close(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func BenchmarkBSecretStoreLease(b *testing.B) {
	store := newGenerationTestSecretStore(b)
	carrier := &generationTestCarrier{}
	mutation, err := store.PrepareMutation(
		context.Background(),
		newGenerationTestCatalog(b),
		carrier,
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
		carrier := &generationTestCarrier{}
		mutation, err := store.PrepareMutation(
			context.Background(),
			newGenerationTestCatalog(b),
			carrier,
			generationTestConfig("main", "value"),
			0,
		)
		if err != nil {
			b.Fatal(err)
		}
		if err := mutation.Abort(context.Background()); err != nil {
			b.Fatal(err)
		}
	}
}

type generationTestCarrier struct {
	activated bool
	released  bool
	capacity  int64
}

func (carrier *generationTestCarrier) Valid() bool {
	return carrier != nil && !carrier.released
}

func (carrier *generationTestCarrier) ResourceCapacityBytes() int64 {
	if carrier == nil {
		return 0
	}
	if carrier.capacity != 0 {
		return carrier.capacity
	}
	return 1 << 30
}

func (carrier *generationTestCarrier) Activate() error {
	if !carrier.Valid() || carrier.activated {
		return errors.New("invalid activation")
	}
	carrier.activated = true
	return nil
}

func (carrier *generationTestCarrier) Release() error {
	if !carrier.Valid() {
		return errors.New("invalid release")
	}
	carrier.released = true
	return nil
}

type generationTestStore struct {
	config struct {
		Value string `yaml:"value"`
	}
}

func (store *generationTestStore) Configuration() any {
	return &store.config
}

func (*generationTestStore) Init(context.Context) error { return nil }

func (store *generationTestStore) Publish() PublishedStore {
	return generationTestPublished(store.config.Value)
}

type generationTestPublished string

func (generationTestPublished) RetainedBytes() int64 { return 512 }

func (published generationTestPublished) Resolve(
	context.Context,
	ResolveRequest,
) (string, error) {
	return string(published), nil
}

func newGenerationTestCatalog(t testing.TB) *CreatorCatalog {
	t.Helper()
	catalog, err := NewCreatorCatalog([]Creator{{
		Kind:        KindVault,
		DisplayName: "Vault",
		Schema:      `{}`,
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
