// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"math/rand"
	"sync"
	"time"
)

// Table Cache Overview:
// The table cache stores table structure (which rows exist) to convert repeated
// SNMP walks into efficient GET operations.
// - First collection: Full walk, cache row indexes
// - Subsequent collections: GET only the columns needed for each MetricsConfig

type tableCache struct {
	// Table OID -> list of indexes (rows) that exist
	tableIndexes map[string][]string

	// Table OID -> when cached
	timestamps map[string]time.Time

	// Table OID -> specific TTL for this table (with jitter applied)
	tableTTLs map[string]time.Duration

	// Table OID -> list of dependent table OIDs (bidirectional)
	tableDeps map[string]map[string]bool

	baseTTL   time.Duration
	jitterPct float64
	mu        sync.RWMutex
	rng       *rand.Rand
}

func newTableCache(baseTTL time.Duration, jitterPct float64) *tableCache {
	return &tableCache{
		tableIndexes: make(map[string][]string),
		timestamps:   make(map[string]time.Time),
		tableTTLs:    make(map[string]time.Duration),
		tableDeps:    make(map[string]map[string]bool),
		baseTTL:      baseTTL,
		jitterPct:    jitterPct,
		rng:          rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (tc *tableCache) calculateTableTTL() time.Duration {
	base := float64(tc.baseTTL)
	jitter := tc.jitterPct

	// Random jitter between 0 and +jitterPct
	randFloat := tc.rng.Float64()
	multiplier := 1.0 + randFloat*jitter

	return time.Duration(base * multiplier)
}

func (tc *tableCache) getCachedIndexes(tableOID string) ([]string, bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.baseTTL == 0 {
		return nil, false
	}

	timestamp, ok := tc.timestamps[tableOID]
	if !ok {
		return nil, false
	}

	ttl, ok := tc.tableTTLs[tableOID]
	if !ok || time.Since(timestamp) > ttl {
		return nil, false
	}

	indexes := tc.tableIndexes[tableOID]
	return indexes, true
}

func (tc *tableCache) cacheIndexes(tableOID string, indexes []string) {
	tc.cacheIndexesWithDeps(tableOID, indexes, nil)
}

func (tc *tableCache) cacheIndexesWithDeps(tableOID string, indexes []string, dependencies []string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.baseTTL == 0 {
		return
	}

	// Deep copy the indexes
	indexesCopy := make([]string, len(indexes))
	copy(indexesCopy, indexes)

	tc.tableIndexes[tableOID] = indexesCopy
	tc.timestamps[tableOID] = time.Now()
	tc.tableTTLs[tableOID] = tc.calculateTableTTL()

	// Set up bidirectional dependencies
	if len(dependencies) > 0 {
		if tc.tableDeps[tableOID] == nil {
			tc.tableDeps[tableOID] = make(map[string]bool)
		}

		for _, depTable := range dependencies {
			// Add forward dependency
			tc.tableDeps[tableOID][depTable] = true

			// Add reverse dependency
			if tc.tableDeps[depTable] == nil {
				tc.tableDeps[depTable] = make(map[string]bool)
			}
			tc.tableDeps[depTable][tableOID] = true
		}
	}
}

func (tc *tableCache) clearExpired() []string {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	var expired []string
	now := time.Now()

	// First pass: find naturally expired tables
	expiredTables := make(map[string]bool)
	for tableOID, timestamp := range tc.timestamps {
		ttl := tc.tableTTLs[tableOID]
		if now.Sub(timestamp) > ttl {
			expiredTables[tableOID] = true
		}
	}

	// Second pass: cascade expiration to dependent tables
	for tableOID := range expiredTables {
		for dep := range tc.tableDeps[tableOID] {
			expiredTables[dep] = true
		}
	}

	// Clear all expired tables
	for tableOID := range expiredTables {
		delete(tc.tableIndexes, tableOID)
		delete(tc.timestamps, tableOID)
		delete(tc.tableTTLs, tableOID)

		// Clean up dependencies
		if deps, ok := tc.tableDeps[tableOID]; ok {
			// Remove this table from other tables' dependency lists
			for depTable := range deps {
				if otherDeps, ok := tc.tableDeps[depTable]; ok {
					delete(otherDeps, tableOID)
					if len(otherDeps) == 0 {
						delete(tc.tableDeps, depTable)
					}
				}
			}
			delete(tc.tableDeps, tableOID)
		}

		expired = append(expired, tableOID)
	}

	return expired
}

func (tc *tableCache) setTTL(baseTTL time.Duration, jitterPct float64) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.baseTTL = baseTTL
	tc.jitterPct = jitterPct

	if baseTTL == 0 {
		// Clear cache if caching is disabled
		tc.tableIndexes = make(map[string][]string)
		tc.timestamps = make(map[string]time.Time)
		tc.tableTTLs = make(map[string]time.Duration)
		tc.tableDeps = make(map[string]map[string]bool)
	}
}
