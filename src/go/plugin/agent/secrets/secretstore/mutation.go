// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"errors"
	"sync"
)

type preparationRef struct {
	slot       uint32
	generation uint64
}

type preparationSlot struct {
	generation    uint64
	active        bool
	freeNext      uint32
	key           string
	expected      uint64
	expectedState uint64
	record        *generationRecord
	removal       bool
	prepared      preparedStore
	carrier       GenerationCarrier
}

// PreparedSecretMutation owns one initialized Store and its admitted carrier
// until Commit or Abort consumes it.
type PreparedSecretMutation struct {
	mu       sync.Mutex
	consumed bool
	owner    *SecretStore
	ref      preparationRef
	key      string
	expected uint64
	removal  bool
}

type SecretMutationResult struct {
	Generation uint64
	Applied    bool
	Retained   bool
}

func (store *SecretStore) PrepareMutation(
	ctx context.Context,
	catalog *CreatorCatalog,
	carrier GenerationCarrier,
	cfg Config,
	expected uint64,
) (PreparedSecretMutation, error) {
	if store == nil || ctx == nil || catalog == nil || cfg == nil ||
		expected == ^uint64(0) || carrier == nil || !carrier.Valid() {
		return PreparedSecretMutation{},
			errors.New("secretstore: invalid mutation preparation")
	}
	key := cfg.ExposedKey()
	if key == "" {
		return PreparedSecretMutation{},
			errors.New("secretstore: invalid mutation key")
	}
	ref, err := store.reservePreparation(key, expected, carrier)
	if err != nil {
		return PreparedSecretMutation{}, err
	}
	prepared, prepareErr := preparePublishedConfig(
		ctx,
		store.resolver,
		cfg,
		func(kind StoreKind) (Store, bool) {
			creator, ok := catalog.Lookup(kind)
			if !ok || creator.Create == nil {
				return nil, false
			}
			return creator.Create(), true
		},
	)
	if prepareErr != nil {
		return newPreparedSecretMutation(
			store,
			ref,
			key,
			expected,
		), prepareErr
	}
	store.mu.Lock()
	slot, err := store.preparation(ref)
	if err == nil {
		slot.prepared = prepared
	}
	admissible := err == nil &&
		store.state == storeAuthorityOpen &&
		store.dirty == nil
	store.mu.Unlock()
	if err != nil {
		return newPreparedSecretMutation(
			store,
			ref,
			key,
			expected,
		), err
	}
	if !admissible {
		return newPreparedSecretMutation(
				store,
				ref,
				key,
				expected,
			),
			errors.New("secretstore: preparation completed after close")
	}
	return newPreparedSecretMutation(
		store,
		ref,
		key,
		expected,
	), nil
}

func newPreparedSecretMutation(
	owner *SecretStore,
	ref preparationRef,
	key string,
	expected uint64,
) PreparedSecretMutation {
	return PreparedSecretMutation{
		owner:    owner,
		ref:      ref,
		key:      key,
		expected: expected,
	}
}

func (store *SecretStore) Validate(
	ctx context.Context,
	catalog *CreatorCatalog,
	cfg Config,
) error {
	if store == nil || ctx == nil || catalog == nil {
		return errors.New("secretstore: invalid validation")
	}
	_, err := preparePublishedConfig(
		ctx,
		store.resolver,
		cfg,
		func(kind StoreKind) (Store, bool) {
			creator, ok := catalog.Lookup(kind)
			if !ok || creator.Create == nil {
				return nil, false
			}
			return creator.Create(), true
		},
	)
	return err
}

func (store *SecretStore) PrepareRemoval(
	key string,
	expected uint64,
) (PreparedSecretMutation, error) {
	if store == nil || key == "" || expected == 0 {
		return PreparedSecretMutation{},
			errors.New("secretstore: invalid removal preparation")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record := store.records[key]
	if store.state != storeAuthorityOpen ||
		store.dirty != nil ||
		record == nil ||
		record.current == nil ||
		record.current.generation != expected ||
		record.retiring != nil {
		return PreparedSecretMutation{},
			errors.New("secretstore: removal generation differs")
	}
	index, slot, err := store.allocatePreparation()
	if err != nil {
		return PreparedSecretMutation{}, err
	}
	next := slot.generation + 1
	if next == 0 {
		store.releaseUnusedPreparationSlot(index, slot)
		return PreparedSecretMutation{},
			errors.New("secretstore: preparation generation wrapped")
	}
	*slot = preparationSlot{
		generation:    next,
		active:        true,
		key:           key,
		expected:      expected,
		expectedState: record.stateVersion,
		record:        record,
		removal:       true,
	}
	record.preparations++
	store.activePreparations++
	return PreparedSecretMutation{
		owner: store,
		ref: preparationRef{
			slot:       index,
			generation: next,
		},
		key: key, expected: expected, removal: true,
	}, nil
}

func (store *SecretStore) reservePreparation(
	key string,
	expected uint64,
	carrier GenerationCarrier,
) (preparationRef, error) {
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state != storeAuthorityOpen || store.dirty != nil {
		return preparationRef{},
			errors.New("secretstore: mutation admission is closed")
	}
	record := store.records[key]
	current := uint64(0)
	if record != nil && record.current != nil {
		current = record.current.generation
	}
	if record != nil && record.retiring != nil {
		return preparationRef{},
			errors.New("secretstore: prior generation is still retiring")
	}
	if current != expected {
		return preparationRef{},
			errors.New("secretstore: expected generation differs")
	}
	index, slot, err := store.allocatePreparation()
	if err != nil {
		return preparationRef{}, err
	}
	next := slot.generation + 1
	if next == 0 {
		store.releaseUnusedPreparationSlot(index, slot)
		return preparationRef{},
			errors.New("secretstore: preparation generation wrapped")
	}
	if record == nil {
		record = &generationRecord{key: key}
		store.records[key] = record
	}
	*slot = preparationSlot{
		generation:    next,
		active:        true,
		key:           key,
		expected:      expected,
		expectedState: record.stateVersion,
		record:        record,
		carrier:       carrier,
	}
	record.preparations++
	store.activePreparations++
	return preparationRef{slot: index, generation: next}, nil
}

func (store *SecretStore) allocatePreparation() (
	uint32,
	*preparationSlot,
	error,
) {
	if store.freePreparation == 0 {
		if uint64(len(store.preparations)) > uint64(^uint32(0)) {
			return 0, nil, errors.New(
				"secretstore: preparation reference exhausted",
			)
		}
		index := uint32(len(store.preparations))
		slot := &preparationSlot{}
		store.preparations = append(store.preparations, slot)
		return index, slot, nil
	}
	index := store.freePreparation - 1
	slot := store.preparations[index]
	if slot == nil {
		return 0, nil, errors.New(
			"secretstore: invalid free preparation",
		)
	}
	store.freePreparation = slot.freeNext
	return index, slot, nil
}

func (store *SecretStore) releaseUnusedPreparationSlot(
	index uint32,
	slot *preparationSlot,
) {
	generation := slot.generation
	*slot = preparationSlot{
		generation: generation,
		freeNext:   store.freePreparation,
	}
	store.freePreparation = index + 1
}

func (mutation *PreparedSecretMutation) Commit(
	ctx context.Context,
) (SecretMutationResult, error) {
	owner, ref, key, expected, removal, err := mutation.take()
	if err != nil {
		return SecretMutationResult{Retained: true}, err
	}
	if removal {
		return owner.commitRemoval(ctx, ref, key, expected)
	}
	return owner.commitPreparation(ctx, ref, key, expected)
}

func (mutation *PreparedSecretMutation) Abort(context.Context) error {
	owner, ref, _, _, removal, err := mutation.take()
	if err != nil {
		return err
	}
	if removal {
		return owner.abortRemoval(ref)
	}
	return owner.abortPreparation(ref)
}

func (mutation *PreparedSecretMutation) Valid() bool {
	if mutation == nil || mutation.owner == nil {
		return false
	}
	mutation.mu.Lock()
	defer mutation.mu.Unlock()
	return !mutation.consumed
}

func (mutation *PreparedSecretMutation) take() (
	*SecretStore,
	preparationRef,
	string,
	uint64,
	bool,
	error,
) {
	if mutation == nil || mutation.owner == nil {
		return nil, preparationRef{}, "", 0, false,
			errors.New("secretstore: empty prepared mutation")
	}
	mutation.mu.Lock()
	defer mutation.mu.Unlock()
	if mutation.consumed {
		return nil, preparationRef{}, "", 0, false,
			errors.New("secretstore: prepared mutation consumed")
	}
	mutation.consumed = true
	return mutation.owner, mutation.ref, mutation.key, mutation.expected,
		mutation.removal, nil
}

func (store *SecretStore) commitRemoval(
	ctx context.Context,
	ref preparationRef,
	key string,
	expected uint64,
) (SecretMutationResult, error) {
	if ctx == nil || ctx.Err() != nil {
		abortErr := store.abortRemoval(ref)
		if ctx == nil {
			return SecretMutationResult{Retained: abortErr != nil},
				errors.Join(
					errors.New("secretstore: nil removal context"),
					abortErr,
				)
		}
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(ctx.Err(), abortErr)
	}
	store.mu.Lock()
	slot, preparationErr := store.preparation(ref)
	record := store.records[key]
	if preparationErr != nil ||
		!slot.removal ||
		slot.key != key ||
		slot.expected != expected ||
		store.state != storeAuthorityOpen ||
		store.dirty != nil ||
		record == nil ||
		record.current == nil ||
		record.current.generation != expected ||
		slot.record != record ||
		slot.expectedState != record.stateVersion ||
		record.retiring != nil {
		store.mu.Unlock()
		abortErr := store.abortRemoval(ref)
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(
				errors.New("secretstore: removal CAS rejected"),
				preparationErr,
				abortErr,
			)
	}
	old := record.current
	record.current = nil
	record.retiring = old
	record.stateVersion++
	store.clearPreparation(ref.slot, slot)
	releaseOld := old.readers == 0
	store.mu.Unlock()
	result := SecretMutationResult{Generation: expected, Applied: true}
	if releaseOld {
		if err := store.releaseGeneration(old); err != nil {
			result.Retained = true
			return result, err
		}
	}
	return result, nil
}

func (store *SecretStore) commitPreparation(
	ctx context.Context,
	ref preparationRef,
	key string,
	expected uint64,
) (SecretMutationResult, error) {
	if ctx == nil || ctx.Err() != nil {
		var err error
		if ctx == nil {
			err = errors.New("secretstore: nil mutation context")
		} else {
			err = ctx.Err()
		}
		abortErr := store.abortPreparation(ref)
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(err, abortErr)
	}
	store.mu.Lock()
	slot, err := store.preparation(ref)
	if err != nil ||
		slot.key != key ||
		slot.expected != expected ||
		slot.removal ||
		store.state != storeAuthorityOpen ||
		store.dirty != nil {
		store.mu.Unlock()
		abortErr := store.abortPreparation(ref)
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(
				errors.New("secretstore: mutation CAS rejected"),
				err,
				abortErr,
			)
	}
	carrier := slot.carrier
	store.mu.Unlock()
	if err := callGenerationCarrierActivate(carrier); err != nil {
		abortErr := store.abortPreparation(ref)
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(err, abortErr)
	}
	store.mu.Lock()
	slot, err = store.preparation(ref)
	if err != nil {
		store.mu.Unlock()
		return SecretMutationResult{}, err
	}
	record := store.records[key]
	current := uint64(0)
	if record != nil && record.current != nil {
		current = record.current.generation
	}
	if store.state != storeAuthorityOpen ||
		store.dirty != nil ||
		current != expected ||
		slot.record != record ||
		record == nil ||
		slot.expectedState != record.stateVersion ||
		record != nil && record.retiring != nil {
		store.mu.Unlock()
		abortErr := store.abortPreparation(ref)
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(
				errors.New("secretstore: mutation CAS rejected"),
				abortErr,
			)
	}
	nextGeneration := store.nextGeneration + 1
	if nextGeneration == 0 {
		store.mu.Unlock()
		abortErr := store.abortPreparation(ref)
		return SecretMutationResult{Retained: abortErr != nil},
			errors.Join(
				errors.New("secretstore: Store generation wrapped"),
				abortErr,
			)
	}
	store.nextGeneration = nextGeneration
	generation := &StoreGeneration{
		record:     record,
		generation: nextGeneration,
		config:     cloneConfig(slot.prepared.rawConfig),
		hash:       slot.prepared.configHash,
		status:     cloneStoreStatus(slot.prepared.status),
		published:  slot.prepared.published,
		carrier:    slot.carrier,
	}
	old := record.current
	record.current = generation
	if old != nil {
		record.retiring = old
	}
	record.stateVersion++
	store.linkGeneration(generation)
	store.clearPreparation(ref.slot, slot)
	releaseOld := old != nil && old.readers == 0
	store.mu.Unlock()
	result := SecretMutationResult{
		Generation: generation.generation,
		Applied:    true,
	}
	if releaseOld {
		if err := store.releaseGeneration(old); err != nil {
			result.Retained = true
			return result, err
		}
	}
	return result, nil
}

func (store *SecretStore) abortPreparation(ref preparationRef) error {
	store.mu.Lock()
	slot, err := store.preparation(ref)
	if err != nil {
		store.mu.Unlock()
		return err
	}
	if slot.removal {
		store.mu.Unlock()
		return errors.New(
			"secretstore: removal used mutation abort path",
		)
	}
	carrier := slot.carrier
	store.mu.Unlock()
	releaseErr := callGenerationCarrierRelease(carrier)
	store.mu.Lock()
	defer store.mu.Unlock()
	slot, err = store.preparation(ref)
	if err != nil {
		return errors.Join(releaseErr, err)
	}
	if releaseErr != nil {
		store.dirty = errors.Join(store.dirty, releaseErr)
		return releaseErr
	}
	store.clearPreparation(ref.slot, slot)
	return nil
}

func (store *SecretStore) abortRemoval(ref preparationRef) error {
	store.mu.Lock()
	defer store.mu.Unlock()
	slot, err := store.preparation(ref)
	if err != nil {
		return err
	}
	if !slot.removal || slot.carrier != nil {
		return errors.New(
			"secretstore: mutation used removal abort path",
		)
	}
	store.clearPreparation(ref.slot, slot)
	return nil
}

func (store *SecretStore) preparation(
	ref preparationRef,
) (*preparationSlot, error) {
	if uint64(ref.slot) >= uint64(len(store.preparations)) ||
		ref.generation == 0 {
		return nil, errors.New("secretstore: invalid preparation reference")
	}
	slot := store.preparations[ref.slot]
	if slot == nil ||
		!slot.active ||
		slot.generation != ref.generation {
		return nil, errors.New("secretstore: stale preparation reference")
	}
	return slot, nil
}

func (store *SecretStore) clearPreparation(
	index uint32,
	slot *preparationSlot,
) {
	record := slot.record
	generation := slot.generation
	*slot = preparationSlot{
		generation: generation,
		freeNext:   store.freePreparation,
	}
	store.freePreparation = index + 1
	store.activePreparations--
	record.preparations--
	if record.preparations == 0 &&
		record.current == nil &&
		record.retiring == nil &&
		store.records[record.key] == record {
		delete(store.records, record.key)
	}
}
