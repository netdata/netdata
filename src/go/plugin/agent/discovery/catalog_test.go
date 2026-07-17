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
