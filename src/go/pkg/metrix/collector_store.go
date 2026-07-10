// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

// NewCollectorStore creates a collection store with staged writes and immutable read snapshots.
func NewCollectorStore(opts ...CollectorStoreOption) CollectorStore {
	cfg := collectorStoreConfig{
		expireAfterSuccessCycles: defaultCollectorExpireAfterSuccessCycles,
		maxSeries:                defaultCollectorMaxSeries,
	}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	// Grace defaults to the (possibly overridden) expire so the invariant grace >= expire
	// holds by construction; an explicit WithDescriptorGraceCycles decouples them.
	grace := cfg.expireAfterSuccessCycles
	if cfg.graceSet {
		grace = cfg.graceCycles
	}
	core := &storeCore{
		instruments: make(map[string]*instrumentDescriptor),
		retention: collectorRetentionPolicy{
			expireAfterSuccessCycles: cfg.expireAfterSuccessCycles,
			maxSeries:                cfg.maxSeries,
			descriptorGraceCycles:    grace,
		},
	}
	core.snapshot.Store(&readSnapshot{
		collectMeta: CollectMeta{LastAttemptStatus: CollectStatusUnknown},
		series:      make(map[string]*committedSeries),
		byName:      make(map[string][]*committedSeries),
	})
	return &storeView{core: core}
}

// AsCycleManagedStore exposes runtime cycle control for stores created by NewCollectorStore.
// This is intended for runtime internals, not collector code.
func AsCycleManagedStore(s CollectorStore) (CycleManagedStore, bool) {
	switch v := s.(type) {
	case *managedStore:
		return v, true
	case *storeView:
		return &managedStore{core: v.core}, true
	default:
		return nil, false
	}
}

func (s *storeView) Read(opts ...ReadOption) Reader {
	cfg := resolveReadConfig(opts...)
	snap := s.core.snapshot.Load()
	if cfg.flatten {
		snap = flattenSnapshot(snap)
	}
	return &storeReader{snap: snap, raw: cfg.raw, flattened: cfg.flatten, hostScopeKey: cfg.hostScopeKey}
}

func (s *storeView) Write() Writer {
	return &writeView{backend: s.core}
}

func (s *managedStore) Read(opts ...ReadOption) Reader {
	return (&storeView{core: s.core}).Read(opts...)
}

func (s *managedStore) Write() Writer {
	return (&storeView{core: s.core}).Write()
}

func (s *managedStore) CycleController() CycleController {
	return &storeCycleController{core: s.core}
}

// DescriptorRetention (optional interface) - both store facades delegate to storeCore.

func (s *storeView) DescriptorRetentionWindow() uint64 { return s.core.descriptorRetentionWindow() }
func (s *storeView) SuccessfulCommits() uint64         { return s.core.successfulCommits() }

func (s *managedStore) DescriptorRetentionWindow() uint64 { return s.core.descriptorRetentionWindow() }
func (s *managedStore) SuccessfulCommits() uint64         { return s.core.successfulCommits() }
