// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"errors"
	"sync"

	secretresolver "github.com/netdata/netdata/go/plugins/plugin/agent/secrets/resolver"
)

// SecretStore is the single per-epoch authority for configured Store
// generations and lexical reader scopes.
type SecretStore struct {
	mu sync.Mutex

	state storeAuthorityState
	dirty error
	done  chan struct{}

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
	key           string
	stateVersion  uint64
	preparations  int
	current       *StoreGeneration
	retiring      [2]*StoreGeneration
	mutationReady chan struct{}
}

func (record *generationRecord) retirementCount() int {
	if record == nil {
		return 0
	}
	count := 0
	for _, generation := range record.retiring {
		if generation != nil {
			count++
		}
	}
	return count
}

func (record *generationRecord) hasRetiring() bool {
	return record.retirementCount() != 0
}

func (record *generationRecord) retirementFull() bool {
	if record == nil {
		return false
	}
	return record.retirementCount() == len(record.retiring)
}

func (record *generationRecord) addRetiring(generation *StoreGeneration) bool {
	if record == nil ||
		generation == nil ||
		generation.record != record ||
		record.isRetiring(generation) {
		return false
	}
	for index := range record.retiring {
		if record.retiring[index] == nil {
			record.retiring[index] = generation
			if record.mutationReady == nil {
				record.mutationReady = make(chan struct{})
			}
			return true
		}
	}
	return false
}

func (record *generationRecord) isRetiring(generation *StoreGeneration) bool {
	if record == nil || generation == nil {
		return false
	}
	for _, current := range record.retiring {
		if current == generation {
			return true
		}
	}
	return false
}

func (record *generationRecord) removeRetiring(generation *StoreGeneration) bool {
	if record == nil || generation == nil {
		return false
	}
	for index, current := range record.retiring {
		if current != generation {
			continue
		}
		copy(record.retiring[index:], record.retiring[index+1:])
		record.retiring[len(record.retiring)-1] = nil
		if !record.hasRetiring() && record.mutationReady != nil {
			close(record.mutationReady)
			record.mutationReady = nil
		}
		return true
	}
	return false
}

var immediateMutationReady = func() <-chan struct{} {
	ready := make(chan struct{})
	close(ready)
	return ready
}()

// StoreGeneration is one immutable published provider generation.
type StoreGeneration struct {
	record     *generationRecord
	generation uint64
	config     Config
	hash       uint64
	published  PublishedStore
	readers    int
	previous   *StoreGeneration
	next       *StoreGeneration
}

// SecretStoreCensus is the exact retained-state census used by ownership and
// leak verification.
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
		done:     make(chan struct{}),
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
		census.Retiring += record.retirementCount()
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

// MutationReady closes when every generation whose retirement currently blocks
// a same-key mutation has physically released.
func (store *SecretStore) MutationReady(key string) <-chan struct{} {
	if store == nil || key == "" {
		return immediateMutationReady
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record := store.records[key]
	if record == nil || !record.hasRetiring() {
		return immediateMutationReady
	}
	if record.mutationReady == nil {
		record.mutationReady = make(chan struct{})
	}
	return record.mutationReady
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
// it after all lexical readers have drained.
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
		record.retirementFull() {
		store.mu.Unlock()
		return errors.New("secretstore: current generation differs")
	}
	retiring := record.current
	if !record.addRetiring(retiring) {
		store.mu.Unlock()
		return errors.New("secretstore: current generation retirement capacity exhausted")
	}
	record.current = nil
	record.stateVersion++
	release := retiring.readers == 0
	store.mu.Unlock()
	if release {
		return store.releaseGeneration(retiring)
	}
	return nil
}

// Seal closes mutation and scope admission. Physical ownership may drain after
// Seal returns; Done closes when the exact retained-state census reaches zero.
func (store *SecretStore) Seal() error {
	if store == nil {
		return errors.New("secretstore: nil Store authority")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if store.state == storeAuthorityClosed {
		return store.dirty
	}
	store.state = storeAuthorityClosing
	store.finishCloseLocked()
	return store.dirty
}

// Done closes after a sealed Store releases every generation, reader scope,
// and preparation.
func (store *SecretStore) Done() <-chan struct{} {
	if store == nil {
		return nil
	}
	return store.done
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
	store.finishCloseLocked()
	if store.state == storeAuthorityClosed {
		return store.dirty
	}
	return errors.Join(
		errors.New("secretstore: close with retained ownership"),
		store.dirty,
	)
}

func (store *SecretStore) finishCloseLocked() {
	if store.state != storeAuthorityClosing ||
		store.activeScopes != 0 ||
		store.activePreparations != 0 ||
		store.generations != 0 ||
		store.readers != 0 {
		return
	}
	store.state = storeAuthorityClosed
	close(store.done)
}

func (store *SecretStore) releaseGeneration(
	generation *StoreGeneration,
) error {
	if generation == nil {
		return errors.New("secretstore: invalid generation release")
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	record := generation.record
	if record == nil || !record.removeRetiring(generation) {
		store.dirty = errors.Join(
			store.dirty,
			errors.New("secretstore: generation release lost ownership"),
		)
		return store.dirty
	}
	record.stateVersion++
	store.unlinkGeneration(generation)
	if record.current == nil && record.preparations == 0 && !record.hasRetiring() {
		delete(store.records, record.key)
	}
	store.finishCloseLocked()
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
