// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import (
	"time"
)

// NewRuntimeStore creates a dedicated runtime/internal metrics store with
// stateful-only, immediate-commit write semantics.
func NewRuntimeStore() RuntimeStore {
	core := &storeCore{
		instruments: make(map[string]*instrumentDescriptor),
	}
	core.snapshot.Store(&readSnapshot{
		collectMeta: CollectMeta{LastAttemptStatus: CollectStatusUnknown},
		series:      make(map[string]*committedSeries),
	})

	backend := &runtimeStoreBackend{core: core}
	backend.summarySketches = make(map[string]*summaryQuantileSketch)
	backend.retention = runtimeRetentionPolicy{
		ttl:       defaultRuntimeRetentionTTL,
		maxSeries: defaultRuntimeRetentionMaxSeries,
	}
	backend.compaction = runtimeCompactionPolicy{
		maxOverlayDepth:  defaultRuntimeCompactionDepth,
		maxOverlayWrites: defaultRuntimeCompactionWrites,
	}
	backend.now = time.Now
	return &runtimeStoreView{
		core:    core,
		backend: backend,
	}
}

func (s *runtimeStoreView) Read(opts ...ReadOption) Reader {
	cfg := resolveReadConfig(opts...)
	snap := s.core.snapshot.Load()
	if cfg.flatten {
		snap = flattenSnapshot(snap)
	}
	return &storeReader{snap: snap, raw: cfg.raw, flattened: cfg.flatten, hostScopeKey: cfg.hostScopeKey}
}

func (s *runtimeStoreView) Write() RuntimeWriter {
	return &runtimeWriteView{backend: s.backend}
}

func (w *runtimeWriteView) StatefulMeter(prefix string) StatefulMeter {
	return &statefulMeter{backend: w.backend, prefix: prefix}
}

func (r *runtimeStoreBackend) compileLabelSet(labels ...Label) LabelSet {
	return compileLabelSetForOwner(r, labels...)
}

func (r *runtimeStoreBackend) registerInstrument(name string, kind metricKind, mode metricMode, opts ...InstrumentOption) (*instrumentDescriptor, error) {
	if mode != modeStateful {
		return nil, errRuntimeSnapshotWrite
	}

	cfg := instrumentConfig{}
	for _, opt := range opts {
		if opt != nil {
			opt.apply(&cfg)
		}
	}
	if cfg.freshnessSet && cfg.freshness != FreshnessCommitted {
		return nil, errRuntimeFreshness
	}
	if cfg.windowSet && cfg.window == WindowCycle {
		return nil, errRuntimeWindowCycle
	}

	desc, err := r.core.registerInstrument(name, kind, modeStateful, opts...)
	if err != nil {
		return nil, err
	}
	if desc.freshness != FreshnessCommitted {
		return nil, errRuntimeFreshness
	}
	return desc, nil
}

var _ RuntimeStore = (*runtimeStoreView)(nil)
var _ RuntimeWriter = (*runtimeWriteView)(nil)
var _ meterBackend = (*runtimeStoreBackend)(nil)
