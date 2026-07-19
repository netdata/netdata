// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"errors"
	"sync"
)

type scopeRef struct {
	slot       uint32
	generation uint64
}

type scopeSlot struct {
	generation uint64
	active     bool
	freeNext   uint32
	pins       []*StoreGeneration
}

// ResolutionScope pins one immutable generation for each distinct requested
// Store key until Release.
type ResolutionScope struct {
	mu       sync.Mutex
	released bool
	owner    *SecretStore
	ref      scopeRef
	pins     map[string]*StoreGeneration
}

func (store *SecretStore) AcquireScope(
	keys []string,
) (*ResolutionScope, error) {
	if store == nil || len(keys) == 0 {
		return nil, errors.New("secretstore: invalid scope request")
	}
	unique := make(map[string]*StoreGeneration, len(keys))
	ordered := make([]string, 0, len(keys))
	for _, key := range keys {
		if key == "" {
			return nil, errors.New("secretstore: empty scope key")
		}
		if _, exists := unique[key]; !exists {
			unique[key] = nil
			ordered = append(ordered, key)
		}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state != storeAuthorityOpen || store.dirty != nil {
		return nil, errors.New("secretstore: scope admission is closed")
	}
	index, slot, err := store.allocateScope()
	if err != nil {
		return nil, err
	}
	pins := make([]*StoreGeneration, 0, len(ordered))
	for _, key := range ordered {
		record := store.records[key]
		if record == nil || record.current == nil {
			store.releaseUnusedScopeSlot(index, slot)
			return nil, storeNotConfiguredError(key)
		}
		unique[key] = record.current
		pins = append(pins, record.current)
	}
	next := slot.generation + 1
	if next == 0 {
		store.releaseUnusedScopeSlot(index, slot)
		return nil, errors.New("secretstore: scope generation wrapped")
	}
	for _, generation := range pins {
		generation.readers++
		store.readers++
	}
	*slot = scopeSlot{
		generation: next,
		active:     true,
		pins:       pins,
	}
	store.activeScopes++
	return &ResolutionScope{
		owner: store,
		ref:   scopeRef{slot: index, generation: next},
		pins:  unique,
	}, nil
}

func (store *SecretStore) allocateScope() (
	uint32,
	*scopeSlot,
	error,
) {
	if store.freeScope == 0 {
		if uint64(len(store.scopes)) > uint64(^uint32(0)) {
			return 0, nil,
				errors.New("secretstore: scope reference exhausted")
		}
		index := uint32(len(store.scopes))
		slot := &scopeSlot{}
		store.scopes = append(store.scopes, slot)
		return index, slot, nil
	}
	index := store.freeScope - 1
	slot := store.scopes[index]
	if slot == nil {
		return 0, nil, errors.New("secretstore: invalid free scope")
	}
	store.freeScope = slot.freeNext
	return index, slot, nil
}

func (store *SecretStore) releaseUnusedScopeSlot(
	index uint32,
	slot *scopeSlot,
) {
	generation := slot.generation
	*slot = scopeSlot{
		generation: generation,
		freeNext:   store.freeScope,
	}
	store.freeScope = index + 1
}

func (scope *ResolutionScope) Resolve(
	ctx context.Context,
	storeKey string,
	secretKey string,
) ([]byte, error) {
	if scope == nil || ctx == nil || storeKey == "" || secretKey == "" {
		return nil, errors.New("secretstore: invalid scoped resolution")
	}
	scope.mu.Lock()
	defer scope.mu.Unlock()
	if scope.released {
		return nil, errors.New("secretstore: scope released")
	}
	generation := scope.pins[storeKey]
	if generation == nil || generation.published == nil {
		return nil, errors.New("secretstore: key is outside the scope")
	}
	kind, name, err := ParseStoreKey(storeKey)
	if err != nil {
		return nil, err
	}
	value, err := generation.published.Resolve(ctx, ResolveRequest{
		StoreKey:  storeKey,
		StoreKind: kind,
		StoreName: name,
		Operand:   secretKey,
		Original:  "${store:" + storeKey + ":" + secretKey + "}",
	})
	return []byte(value), err
}

func (scope *ResolutionScope) Release(context.Context) error {
	if scope == nil || scope.owner == nil {
		return errors.New("secretstore: empty scope")
	}
	scope.mu.Lock()
	if scope.released {
		scope.mu.Unlock()
		return errors.New("secretstore: scope released twice")
	}
	scope.released = true
	owner, ref := scope.owner, scope.ref
	scope.mu.Unlock()
	return owner.releaseScope(ref)
}

func (store *SecretStore) releaseScope(ref scopeRef) error {
	store.mu.Lock()
	if uint64(ref.slot) >= uint64(len(store.scopes)) ||
		ref.generation == 0 {
		store.mu.Unlock()
		return errors.New("secretstore: invalid scope reference")
	}
	slot := store.scopes[ref.slot]
	if slot == nil ||
		!slot.active ||
		slot.generation != ref.generation {
		store.mu.Unlock()
		return errors.New("secretstore: stale scope reference")
	}
	retiring := make([]*StoreGeneration, 0, len(slot.pins))
	for _, generation := range slot.pins {
		if generation.readers <= 0 {
			store.mu.Unlock()
			return errors.New("secretstore: reader count underflow")
		}
		generation.readers--
		store.readers--
		if generation.readers == 0 &&
			generation.record.retiring == generation {
			retiring = append(retiring, generation)
		}
	}
	generation := slot.generation
	*slot = scopeSlot{
		generation: generation,
		freeNext:   store.freeScope,
	}
	store.freeScope = ref.slot + 1
	store.activeScopes--
	store.mu.Unlock()
	var result error
	for _, generation := range retiring {
		result = errors.Join(
			result,
			store.releaseGeneration(generation),
		)
	}
	return result
}
