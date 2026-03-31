// SPDX-License-Identifier: GPL-3.0-or-later

package secretstore

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"sync/atomic"
	"time"

	"gopkg.in/yaml.v2"
)

type storeRecord struct {
	rawConfig  Config
	configHash uint64
	status     StoreStatus
	published  PublishedStore
}

type preparedStore struct {
	key        string
	rawConfig  Config
	configHash uint64
	status     StoreStatus
	published  PublishedStore
}

type serviceState struct {
	snapshot *Snapshot
	records  map[string]storeRecord
}

type creatorRegistry struct {
	kinds  []StoreKind
	byKind map[StoreKind]Creator
}

type inMemoryService struct {
	mu       sync.Mutex
	state    atomic.Pointer[serviceState]
	now      func() time.Time
	resolver *runtimeResolver
	registry creatorRegistry
}

func NewService(creators ...Creator) Service {
	return newInMemoryService(creators...)
}

func newInMemoryService(creators ...Creator) Service {
	s := &inMemoryService{
		now:      time.Now,
		resolver: newRuntimeResolver(),
		registry: newCreatorRegistry(creators...),
	}
	s.state.Store(&serviceState{
		snapshot: &Snapshot{
			generation:  0,
			publishedAt: s.now().UTC(),
			stores:      map[string]publishedRecord{},
		},
		records: map[string]storeRecord{},
	})
	return s
}

func (s *inMemoryService) Capture() *Snapshot {
	state := s.state.Load()
	if state == nil {
		return &Snapshot{stores: map[string]publishedRecord{}}
	}
	return cloneSnapshot(state.snapshot)
}

func (s *inMemoryService) Resolve(ctx context.Context, snapshot *Snapshot, ref, original string) (string, error) {
	if ctx != nil {
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		default:
		}
	}
	return s.resolver.resolveContext(ctx, snapshot, ref, original)
}

func (s *inMemoryService) Kinds() []StoreKind {
	return append([]StoreKind(nil), s.registry.kinds...)
}

func (s *inMemoryService) DisplayName(kind StoreKind) (string, bool) {
	creator, ok := s.registry.byKind[kind]
	if !ok {
		return "", false
	}
	return creator.DisplayName, true
}

func (s *inMemoryService) Schema(kind StoreKind) (string, bool) {
	creator, ok := s.registry.byKind[kind]
	if !ok {
		return "", false
	}
	return creator.Schema, true
}

func (s *inMemoryService) New(kind StoreKind) (Store, bool) {
	creator, ok := s.registry.byKind[kind]
	if !ok || creator.Create == nil {
		return nil, false
	}
	store := creator.Create()
	if store == nil {
		return nil, false
	}
	return store, true
}

func (s *inMemoryService) GetStatus(key string) (StoreStatus, bool) {
	key, err := normalizeStoreKey(key)
	if err != nil {
		return StoreStatus{}, false
	}

	state := s.state.Load()
	if state == nil {
		return StoreStatus{}, false
	}
	record, ok := state.records[key]
	if !ok {
		return StoreStatus{}, false
	}
	return cloneStoreStatus(record.status), true
}

func (s *inMemoryService) Validate(ctx context.Context, cfg Config) error {
	_, err := s.prepareConfig(ctx, cfg)
	return err
}

func (s *inMemoryService) ValidateStored(ctx context.Context, key string) error {
	key, err := normalizeStoreKey(key)
	if err != nil {
		return err
	}

	state := s.state.Load()
	if state == nil {
		return storeNotConfiguredError(key)
	}
	record, ok := state.records[key]
	if !ok {
		return storeNotConfiguredError(key)
	}
	validatedHash := record.configHash

	_, err = s.prepareConfig(ctx, record.rawConfig)

	validation := &ValidationStatus{
		CheckedAt: s.now().UTC(),
		OK:        err == nil,
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.state.Load()
	if current == nil {
		return storeNotConfiguredError(key)
	}
	updated, ok := current.records[key]
	if !ok {
		return storeNotConfiguredError(key)
	}
	if updated.configHash != validatedHash {
		return fmt.Errorf("store '%s' changed during validation; retry", key)
	}
	updated.status.LastValidation = validation
	if err != nil {
		updated.status.LastErrorSummary = err.Error()
	} else {
		updated.status.LastErrorSummary = ""
	}

	records := cloneRecords(current.records)
	records[key] = updated
	s.state.Store(&serviceState{
		snapshot: current.snapshot,
		records:  records,
	})

	return err
}

func (s *inMemoryService) Add(ctx context.Context, cfg Config) error {
	prepared, err := s.prepareConfig(ctx, cfg)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.state.Load()
	if state == nil {
		state = &serviceState{
			snapshot: &Snapshot{stores: map[string]publishedRecord{}},
			records:  map[string]storeRecord{},
		}
	}
	if _, ok := state.records[prepared.key]; ok {
		return storeAlreadyExistsError(prepared.key)
	}

	records := cloneRecords(state.records)
	records[prepared.key] = prepared.record()
	snapshot := newSnapshot(state.snapshot.Generation()+1, s.now().UTC(), records)
	s.state.Store(&serviceState{snapshot: snapshot, records: records})
	return nil
}

func (s *inMemoryService) Update(ctx context.Context, key string, cfg Config) error {
	key, err := normalizeStoreKey(key)
	if err != nil {
		return err
	}

	state := s.state.Load()
	if state == nil {
		return storeNotConfiguredError(key)
	}
	before, ok := state.records[key]
	if !ok {
		return storeNotConfiguredError(key)
	}

	prepared, err := s.prepareConfig(ctx, cfg)
	if err != nil {
		return err
	}
	if prepared.key != key {
		return fmt.Errorf("store key mismatch: path key '%s' differs from config key '%s'", key, prepared.key)
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	current := s.state.Load()
	if current == nil {
		return storeNotConfiguredError(key)
	}
	before, ok = current.records[key]
	if !ok {
		return storeNotConfiguredError(key)
	}

	if before.configHash == prepared.configHash {
		return nil
	}

	records := cloneRecords(current.records)
	records[key] = prepared.record()
	snapshot := newSnapshot(current.snapshot.Generation()+1, s.now().UTC(), records)
	s.state.Store(&serviceState{snapshot: snapshot, records: records})
	return nil
}

func (s *inMemoryService) Remove(key string) error {
	key, err := normalizeStoreKey(key)
	if err != nil {
		return err
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	state := s.state.Load()
	if state == nil {
		return storeNotConfiguredError(key)
	}
	if _, ok := state.records[key]; !ok {
		return storeNotConfiguredError(key)
	}

	records := cloneRecords(state.records)
	delete(records, key)
	snapshot := newSnapshot(state.snapshot.Generation()+1, s.now().UTC(), records)
	s.state.Store(&serviceState{snapshot: snapshot, records: records})
	return nil
}

func newCreatorRegistry(creators ...Creator) creatorRegistry {
	reg := creatorRegistry{
		byKind: make(map[StoreKind]Creator, len(creators)),
	}
	for _, creator := range creators {
		if creator.Kind == "" || creator.Create == nil {
			continue
		}
		reg.byKind[creator.Kind] = creator
	}
	reg.kinds = make([]StoreKind, 0, len(reg.byKind))
	for kind := range reg.byKind {
		reg.kinds = append(reg.kinds, kind)
	}
	sort.Slice(reg.kinds, func(i, j int) bool { return reg.kinds[i] < reg.kinds[j] })
	return reg
}

func (s *inMemoryService) prepareConfig(ctx context.Context, cfg Config) (preparedStore, error) {
	if cfg == nil {
		return preparedStore{}, fmt.Errorf("store config is nil")
	}
	if ctx == nil {
		ctx = context.Background()
	}

	raw := cloneConfig(cfg)
	if raw == nil {
		return preparedStore{}, fmt.Errorf("store config is nil")
	}
	if err := raw.Validate(); err != nil {
		return preparedStore{}, err
	}
	rawConfig := cloneConfig(raw)
	rawHash := raw.Hash()
	resolvedPayload, err := resolveProviderPayload(ctx, raw)
	if err != nil {
		return preparedStore{}, err
	}

	kind := raw.Kind()
	name := raw.Name()
	key := raw.ExposedKey()

	store, ok := s.New(kind)
	if !ok {
		return preparedStore{}, fmt.Errorf("store kind '%s' is not supported", kind)
	}
	if store.Configuration() == nil {
		return preparedStore{}, fmt.Errorf("store '%s': configuration is nil", key)
	}

	bs, err := yaml.Marshal(raw)
	if err != nil {
		return preparedStore{}, fmt.Errorf("store '%s': marshaling raw config: %w", key, err)
	}
	if len(resolvedPayload) != 0 {
		for k, v := range resolvedPayload {
			raw[k] = v
		}
		bs, err = yaml.Marshal(raw)
		if err != nil {
			return preparedStore{}, fmt.Errorf("store '%s': marshaling resolved config: %w", key, err)
		}
	}
	if err := yaml.Unmarshal(bs, store.Configuration()); err != nil {
		return preparedStore{}, fmt.Errorf("store '%s': invalid provider payload: %w", key, err)
	}

	if err := store.Init(ctx); err != nil {
		return preparedStore{}, err
	}

	published := store.Publish()
	if published == nil {
		return preparedStore{}, fmt.Errorf("store '%s': published resolver state is nil", key)
	}

	return preparedStore{
		key:        key,
		rawConfig:  rawConfig,
		configHash: rawHash,
		status: StoreStatus{
			Name: name,
			Kind: kind,
		},
		published: published,
	}, nil
}

func newSnapshot(generation uint64, publishedAt time.Time, records map[string]storeRecord) *Snapshot {
	stores := make(map[string]publishedRecord, len(records))
	for key, record := range records {
		stores[key] = publishedRecord{
			published: record.published,
		}
	}
	return &Snapshot{
		generation:  generation,
		publishedAt: publishedAt,
		stores:      stores,
	}
}

func cloneRecords(in map[string]storeRecord) map[string]storeRecord {
	if len(in) == 0 {
		return map[string]storeRecord{}
	}
	out := make(map[string]storeRecord, len(in))
	for key, record := range in {
		out[key] = storeRecord{
			rawConfig:  cloneConfig(record.rawConfig),
			configHash: record.configHash,
			status:     cloneStoreStatus(record.status),
			published:  record.published,
		}
	}
	return out
}

func normalizeStoreKey(key string) (string, error) {
	kind, name, err := ParseStoreKey(key)
	if err != nil {
		return "", err
	}
	return StoreKey(kind, name), nil
}

func (p preparedStore) record() storeRecord {
	return storeRecord{
		rawConfig:  cloneConfig(p.rawConfig),
		configHash: p.configHash,
		status:     cloneStoreStatus(p.status),
		published:  p.published,
	}
}

type wrappedStoreError struct {
	msg string
	err error
}

func (e wrappedStoreError) Error() string { return e.msg }
func (e wrappedStoreError) Unwrap() error { return e.err }

func storeAlreadyExistsError(key string) error {
	return wrappedStoreError{
		msg: fmt.Sprintf("store '%s' already exists", key),
		err: ErrStoreExists,
	}
}

func storeNotConfiguredError(key string) error {
	return wrappedStoreError{
		msg: fmt.Sprintf("store '%s' is not configured", key),
		err: ErrStoreNotFound,
	}
}
