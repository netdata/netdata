// SPDX-License-Identifier: GPL-3.0-or-later

package discovery

import (
	"errors"
	"sort"
	"strings"
)

// ProviderCatalog is the frozen process-wide provider-factory authority.
type ProviderCatalog struct {
	factories map[string]ProviderFactory
}

func NewProviderCatalog(factories []ProviderFactory) (*ProviderCatalog, error) {
	catalog := &ProviderCatalog{
		factories: make(map[string]ProviderFactory, len(factories)),
	}
	for _, factory := range factories {
		if factory == nil {
			return nil, errors.New("discovery provider catalog: nil factory")
		}
		name := strings.TrimSpace(factory.Name())
		if name == "" || name != factory.Name() {
			return nil, errors.New("discovery provider catalog: invalid factory name")
		}
		if _, exists := catalog.factories[name]; exists {
			return nil, errors.New("discovery provider catalog: duplicate factory")
		}
		catalog.factories[name] = factory
	}
	return catalog, nil
}

func (catalog *ProviderCatalog) Lookup(name string) (ProviderFactory, bool) {
	if catalog == nil {
		return nil, false
	}
	factory, ok := catalog.factories[name]
	return factory, ok
}

func (catalog *ProviderCatalog) Len() int {
	if catalog == nil {
		return 0
	}
	return len(catalog.factories)
}

// Factories returns the frozen factories in stable name order. Consumers use
// this snapshot to construct one run generation without creating a second
// provider-factory authority.
func (catalog *ProviderCatalog) Factories() []ProviderFactory {
	if catalog == nil {
		return nil
	}
	names := make([]string, 0, len(catalog.factories))
	for name := range catalog.factories {
		names = append(names, name)
	}
	sort.Strings(names)
	factories := make([]ProviderFactory, 0, len(names))
	for _, name := range names {
		factories = append(factories, catalog.factories[name])
	}
	return factories
}
