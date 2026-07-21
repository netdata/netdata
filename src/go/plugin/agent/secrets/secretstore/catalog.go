// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"errors"
	"slices"
)

// CreatorCatalog is the frozen process-wide SecretStore factory authority.
type CreatorCatalog struct {
	creators map[StoreKind]Creator
}

func NewCreatorCatalog(creators []Creator) (*CreatorCatalog, error) {
	catalog := &CreatorCatalog{
		creators: make(map[StoreKind]Creator, len(creators)),
	}
	for _, creator := range creators {
		if creator.Kind == "" || creator.Create == nil {
			return nil, errors.New("secretstore creator catalog: invalid creator")
		}
		if _, exists := catalog.creators[creator.Kind]; exists {
			return nil, errors.New("secretstore creator catalog: duplicate creator")
		}
		catalog.creators[creator.Kind] = creator
	}
	return catalog, nil
}

func (catalog *CreatorCatalog) Lookup(kind StoreKind) (Creator, bool) {
	if catalog == nil {
		return Creator{}, false
	}
	creator, ok := catalog.creators[kind]
	return creator, ok
}

func (catalog *CreatorCatalog) Len() int {
	if catalog == nil {
		return 0
	}
	return len(catalog.creators)
}

// Creators returns the frozen creators in stable kind order. The returned
// values are the process catalog's factories, not a second mutable registry.
func (catalog *CreatorCatalog) Creators() []Creator {
	if catalog == nil {
		return nil
	}
	kinds := make([]StoreKind, 0, len(catalog.creators))
	for kind := range catalog.creators {
		kinds = append(kinds, kind)
	}
	slices.Sort(kinds)
	creators := make([]Creator, 0, len(kinds))
	for _, kind := range kinds {
		creators = append(creators, catalog.creators[kind])
	}
	return creators
}

func (catalog *CreatorCatalog) Kinds() []StoreKind {
	creators := catalog.Creators()
	kinds := make([]StoreKind, 0, len(creators))
	for _, creator := range creators {
		kinds = append(kinds, creator.Kind)
	}
	return kinds
}

func (catalog *CreatorCatalog) DisplayName(
	kind StoreKind,
) (string, bool) {
	creator, ok := catalog.Lookup(kind)
	return creator.DisplayName, ok
}

func (catalog *CreatorCatalog) Schema(
	kind StoreKind,
) (string, bool) {
	creator, ok := catalog.Lookup(kind)
	return creator.Schema, ok
}

func (catalog *CreatorCatalog) New(
	kind StoreKind,
) (Store, bool) {
	creator, ok := catalog.Lookup(kind)
	if !ok || creator.Create == nil {
		return nil, false
	}
	store := creator.Create()
	return store, store != nil
}
