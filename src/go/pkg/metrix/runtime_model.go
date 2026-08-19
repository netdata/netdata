// SPDX-License-Identifier: GPL-3.0-or-later

package metrix

import "time"

type runtimeStoreView struct {
	core    *storeCore
	backend *runtimeStoreBackend
}

type runtimeStoreBackend struct {
	core                  *storeCore
	summarySketches       map[string]*summaryQuantileSketch
	retention             runtimeRetentionPolicy
	compaction            runtimeCompactionPolicy
	writesSinceCompaction uint64
	now                   func() time.Time
}

type runtimeWriteView struct {
	backend *runtimeStoreBackend
}

type runtimeRetentionPolicy struct {
	ttl       time.Duration
	maxSeries int
}

type runtimeCompactionPolicy struct {
	maxOverlayDepth  int
	maxOverlayWrites uint64
}

const (
	defaultRuntimeRetentionTTL       = 30 * time.Minute
	defaultRuntimeRetentionMaxSeries = 0 // disabled
	defaultRuntimeCompactionDepth    = 64
	defaultRuntimeCompactionWrites   = 64
)
