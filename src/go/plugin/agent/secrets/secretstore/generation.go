// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"errors"
	"sync"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

// GenerationCarrier owns the admitted memory and external-resource facets of
// one configured Store generation.
type GenerationCarrier interface {
	Valid() bool
	Activate() error
	Release() error
}

// SecretStore is the single per-run authority for configured Store
// generations and lexical reader scopes.
type SecretStore struct {
	mu sync.Mutex

	state storeAuthorityState
	dirty error

	records map[string]*generationRecord
	head    *StoreGeneration
	tail    *StoreGeneration

	scopes          []*scopeSlot
	freeScope       uint32
	preparations    []*preparationSlot
	freePreparation uint32

	generations        int
	readers            int
	activeScopes       int
	activePreparations int
	nextGeneration     uint64
	resolver           *secretresolver.AtomicResolver
}

type storeAuthorityState uint8

const (
	storeAuthorityOpen storeAuthorityState = iota + 1
	storeAuthorityClosing
	storeAuthorityClosed
)

type generationRecord struct {
	key          string
	stateVersion uint64
	preparations int
	current      *StoreGeneration
	retiring     *StoreGeneration
}

// StoreGeneration is one immutable published provider generation.
type StoreGeneration struct {
	record     *generationRecord
	generation uint64
	config     Config
	hash       uint64
	published  PublishedStore
	carrier    GenerationCarrier
	readers    int
	previous   *StoreGeneration
	next       *StoreGeneration
}

// SecretStoreCensus is the exact retained-state census used by shutdown and
// migration publication gates.
type SecretStoreCensus struct {
	Current      int
	Retiring     int
	Generations  int
	Readers      int
	Scopes       int
	Preparations int
	Closing      bool
	Closed       bool
	Dirty        bool
}

func NewSecretStore(
	resolver *secretresolver.AtomicResolver,
) (*SecretStore, error) {
	if resolver == nil {
		return nil, errors.New("secretstore: nil process resolver")
	}
	return &SecretStore{
		state:    storeAuthorityOpen,
		records:  make(map[string]*generationRecord),
		resolver: resolver,
	}, nil
}

func (store *SecretStore) Census() SecretStoreCensus {
	if store == nil {
		return SecretStoreCensus{}
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	census := SecretStoreCensus{
		Generations:  store.generations,
		Readers:      store.readers,
		Scopes:       store.activeScopes,
		Preparations: store.activePreparations,
		Closing:      store.state == storeAuthorityClosing,
		Closed:       store.state == storeAuthorityClosed,
		Dirty:        store.dirty != nil,
	}
	for _, record := range store.records {
		if record.current != nil {
			census.Current++
		}
		if record.retiring != nil {
			census.Retiring++
		}
	}
	return census
}

func (store *SecretStore) Generation(key string) uint64 {
	if store == nil {
		return 0
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record := store.records[key]
	if record == nil || record.current == nil {
		return 0
	}
	return record.current.generation
}

func (store *SecretStore) Config(key string) (Config, bool) {
	if store == nil {
		return nil, false
	}
	store.mu.Lock()
	record := store.records[key]
	if record == nil || record.current == nil {
		store.mu.Unlock()
		return nil, false
	}
	config := record.current.config
	store.mu.Unlock()
	return cloneConfig(config), true
}

// Retire removes the matching current generation from admission and releases
// its carrier after all lexical readers have drained.
func (store *SecretStore) Retire(
	ctx context.Context,
	key string,
	generation uint64,
) error {
	if store == nil || ctx == nil || key == "" || generation == 0 {
		return errors.New("secretstore: invalid generation retirement")
	}
	store.mu.Lock()
	record := store.records[key]
	if record == nil ||
		record.current == nil ||
		record.current.generation != generation ||
		record.retiring != nil {
		store.mu.Unlock()
		return errors.New("secretstore: current generation differs")
	}
	retiring := record.current
	record.current = nil
	record.retiring = retiring
	record.stateVersion++
	release := retiring.readers == 0
	store.mu.Unlock()
	if release {
		return store.releaseGeneration(retiring)
	}
	return nil
}

// Close seals mutation/scope admission and acknowledges only an exact-zero
// generation census.
func (store *SecretStore) Close(context.Context) error {
	if store == nil {
		return errors.New("secretstore: nil Store authority")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state == storeAuthorityClosed {
		return store.dirty
	}
	store.state = storeAuthorityClosing
	if store.activeScopes != 0 ||
		store.activePreparations != 0 ||
		store.generations != 0 ||
		store.readers != 0 {
		return errors.Join(
			errors.New("secretstore: close with retained ownership"),
			store.dirty,
		)
	}
	store.state = storeAuthorityClosed
	return store.dirty
}

func (store *SecretStore) releaseGeneration(
	generation *StoreGeneration,
) error {
	if generation == nil || generation.carrier == nil {
		return errors.New("secretstore: invalid generation release")
	}
	releaseErr := callGenerationCarrierRelease(generation.carrier)
	store.mu.Lock()
	defer store.mu.Unlock()
	if releaseErr != nil {
		store.dirty = errors.Join(store.dirty, releaseErr)
		return releaseErr
	}
	record := generation.record
	if record == nil || record.retiring != generation {
		store.dirty = errors.Join(
			store.dirty,
			errors.New("secretstore: generation release lost ownership"),
		)
		return store.dirty
	}
	record.retiring = nil
	record.stateVersion++
	store.unlinkGeneration(generation)
	if record.current == nil && record.preparations == 0 {
		delete(store.records, record.key)
	}
	return nil
}

func (store *SecretStore) linkGeneration(generation *StoreGeneration) {
	generation.previous = store.tail
	if store.tail == nil {
		store.head = generation
	} else {
		store.tail.next = generation
	}
	store.tail = generation
	store.generations++
}

func (store *SecretStore) unlinkGeneration(generation *StoreGeneration) {
	if generation.previous == nil {
		store.head = generation.next
	} else {
		generation.previous.next = generation.next
	}
	if generation.next == nil {
		store.tail = generation.previous
	} else {
		generation.next.previous = generation.previous
	}
	generation.previous = nil
	generation.next = nil
	store.generations--
}

func callGenerationCarrierActivate(carrier GenerationCarrier) (err error) {
	if carrier == nil || !carrier.Valid() {
		return errors.New("secretstore: invalid generation carrier")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("secretstore: generation carrier activation panic")
		}
	}()
	return carrier.Activate()
}

func callGenerationCarrierRelease(carrier GenerationCarrier) (err error) {
	if carrier == nil {
		return errors.New("secretstore: nil generation carrier")
	}
	defer func() {
		if recovered := recover(); recovered != nil {
			err = errors.New("secretstore: generation carrier release panic")
		}
	}()
	return carrier.Release()
}
