// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"testing"
)

func TestSecretCreatorCatalog(t *testing.T) {
	creator := Creator{
		Kind: KindVault,
		Create: func() Store {
			return catalogTestStore{}
		},
	}
	catalog, err := NewCreatorCatalog([]Creator{creator})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := catalog.Lookup(KindVault)
	if !ok || got.Kind != KindVault || got.Create == nil {
		t.Fatalf("lookup=%+v ok=%v", got, ok)
	}
	if _, ok := catalog.Lookup("unknown"); ok {
		t.Fatal("unknown creator resolved")
	}
	if _, err := NewCreatorCatalog([]Creator{creator, creator}); err == nil {
		t.Fatal("duplicate creator accepted")
	}
}

func TestSecretCreatorCatalogLookupAllocatesNothing(t *testing.T) {
	catalog, err := NewCreatorCatalog([]Creator{{
		Kind: KindVault,
		Create: func() Store {
			return catalogTestStore{}
		},
	}})
	if err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(1_000, func() {
		if _, ok := catalog.Lookup(KindVault); !ok {
			panic("creator disappeared")
		}
	})
	if allocations != 0 {
		t.Fatalf("creator lookup allocations=%f, want 0", allocations)
	}
}

func BenchmarkBSecretCreatorLookup(b *testing.B) {
	catalog, err := NewCreatorCatalog([]Creator{{
		Kind: KindVault,
		Create: func() Store {
			return catalogTestStore{}
		},
	}})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = catalog.Lookup(KindVault)
	}
}

type catalogTestStore struct{}

func (catalogTestStore) Configuration() any         { return nil }
func (catalogTestStore) Init(context.Context) error { return nil }
func (catalogTestStore) Publish() PublishedStore    { return nil }
