// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"context"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/framework/confgroup"
)

func TestDiscoveryProviderCatalog(t *testing.T) {
	factory := NewProviderFactory("file", func(BuildContext) (Discoverer, bool, error) {
		return catalogTestDiscoverer{}, true, nil
	})
	catalog, err := NewProviderCatalog([]ProviderFactory{factory})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := catalog.Lookup("file")
	if !ok || got.Name() != "file" {
		t.Fatalf("lookup=%v ok=%v", got, ok)
	}
	if _, ok := catalog.Lookup("unknown"); ok {
		t.Fatal("unknown provider resolved")
	}
	if _, err := NewProviderCatalog([]ProviderFactory{factory, factory}); err == nil {
		t.Fatal("duplicate provider accepted")
	}
}

func TestDiscoveryProviderCatalogRetainsFrozenNames(t *testing.T) {
	name := "file"
	factory := mutableNameProviderFactory{name: &name}
	catalog, err := NewProviderCatalog([]ProviderFactory{factory})
	if err != nil {
		t.Fatal(err)
	}
	name = "changed"
	names := catalog.Names()
	if len(names) != 1 || names[0] != "file" {
		t.Fatalf("catalog names=%v", names)
	}
	if _, ok := catalog.Lookup("file"); !ok {
		t.Fatal("frozen provider identity disappeared")
	}
	if _, ok := catalog.Lookup("changed"); ok {
		t.Fatal("mutable factory identity replaced frozen catalog identity")
	}
}

func TestDiscoveryProviderCatalogLookupAllocatesNothing(t *testing.T) {
	catalog, err := NewProviderCatalog([]ProviderFactory{
		NewProviderFactory(
			"file",
			func(BuildContext) (Discoverer, bool, error) {
				return catalogTestDiscoverer{}, true, nil
			},
		),
	})
	if err != nil {
		t.Fatal(err)
	}
	allocations := testing.AllocsPerRun(1_000, func() {
		if _, ok := catalog.Lookup("file"); !ok {
			panic("provider disappeared")
		}
	})
	if allocations != 0 {
		t.Fatalf("provider lookup allocations=%f, want 0", allocations)
	}
}

func BenchmarkBDiscoveryFactoryLookup(b *testing.B) {
	catalog, err := NewProviderCatalog([]ProviderFactory{
		NewProviderFactory("file", func(BuildContext) (Discoverer, bool, error) {
			return catalogTestDiscoverer{}, true, nil
		}),
	})
	if err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		_, _ = catalog.Lookup("file")
	}
}

type catalogTestDiscoverer struct{}

func (catalogTestDiscoverer) Run(context.Context, chan<- []*confgroup.Group) {}

type mutableNameProviderFactory struct {
	name *string
}

func (factory mutableNameProviderFactory) Name() string {
	return *factory.name
}

func (mutableNameProviderFactory) Build(
	BuildContext,
) (Discoverer, bool, error) {
	return catalogTestDiscoverer{}, true, nil
}
