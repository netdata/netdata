// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"math/rand"
	"sync"
	"time"
)

// Table Cache Overview:
// The table cache converts repeated SNMP walks into efficient GET operations.
// - First collection: Full walk, cache structure and tags
// - Subsequent collections: GET metrics only, use cached tags
// - Tables with dependencies expire together to maintain consistency

type tableCache struct {
	// Table OID -> row index -> column OID -> full OID
	tables map[string]map[string]map[string]string

	// Table OID -> when cached
	timestamps map[string]time.Time

	// Table OID -> specific TTL for this table (with jitter applied)
	tableTTLs map[string]time.Duration

	// Table OID -> tag values (index -> tag name -> value)
	tagValues map[string]map[string]map[string]string

	// Table OID -> list of dependent table OIDs (bidirectional)
	// If table A depends on table B, both A->B and B->A are stored
	tableDeps map[string]map[string]bool

	baseTTL   time.Duration
	jitterPct float64
	mu        sync.RWMutex
	rng       *rand.Rand
}

func newTableCache(baseTTL time.Duration, jitterPct float64) *tableCache {
	return &tableCache{
		tables:     make(map[string]map[string]map[string]string),
		timestamps: make(map[string]time.Time),
		tableTTLs:  make(map[string]time.Duration),
		tagValues:  make(map[string]map[string]map[string]string),
		tableDeps:  make(map[string]map[string]bool),
		baseTTL:    baseTTL,
		jitterPct:  jitterPct,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
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

func (tc *tableCache) getCachedData(tableOID string) (oids map[string]map[string]string, tags map[string]map[string]string, found bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.baseTTL == 0 {
		return nil, nil, false
	}

	timestamp, ok := tc.timestamps[tableOID]
	if !ok {
		return nil, nil, false
	}

	ttl, ok := tc.tableTTLs[tableOID]
	if !ok || time.Since(timestamp) > ttl {
		return nil, nil, false
	}

	oids = tc.tables[tableOID]
	tags = tc.tagValues[tableOID]
	return oids, tags, true
}

func (tc *tableCache) cacheData(tableOID string, oidMap map[string]map[string]string, tagValues map[string]map[string]string) {
	tc.cacheDataWithDeps(tableOID, oidMap, tagValues, nil)
}

func (tc *tableCache) cacheDataWithDeps(tableOID string, oidMap map[string]map[string]string, tagValues map[string]map[string]string, dependencies []string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.baseTTL == 0 {
		return
	}

	// Deep copy the maps to avoid reference issues
	oidsCopy := make(map[string]map[string]string, len(oidMap))
	for index, columns := range oidMap {
		columnsCopy := make(map[string]string, len(columns))
		for colOID, fullOID := range columns {
			columnsCopy[colOID] = fullOID
		}
		oidsCopy[index] = columnsCopy
	}

	tagsCopy := make(map[string]map[string]string, len(tagValues))
	for index, tags := range tagValues {
		tagCopy := make(map[string]string, len(tags))
		for name, value := range tags {
			tagCopy[name] = value
		}
		tagsCopy[index] = tagCopy
	}

	tc.tables[tableOID] = oidsCopy
	tc.tagValues[tableOID] = tagsCopy
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
		delete(tc.tables, tableOID)
		delete(tc.timestamps, tableOID)
		delete(tc.tableTTLs, tableOID)
		delete(tc.tagValues, tableOID)

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
		tc.tables = make(map[string]map[string]map[string]string)
		tc.timestamps = make(map[string]time.Time)
		tc.tableTTLs = make(map[string]time.Duration)
		tc.tagValues = make(map[string]map[string]map[string]string)
		tc.tableDeps = make(map[string]map[string]bool)
	}
}

// Helper method to check if a group of tables is cached
// All tables must be cached and not expired
func (tc *tableCache) areTablesCached(tableOIDs []string) bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.baseTTL == 0 {
		return false
	}

	now := time.Now()
	for _, tableOID := range tableOIDs {
		timestamp, ok := tc.timestamps[tableOID]
		if !ok {
			return false
		}

		ttl, ok := tc.tableTTLs[tableOID]
		if !ok || now.Sub(timestamp) > ttl {
			return false
		}
	}

	return true
}

func (tc *tableCache) stats() (tables int, withDeps int, totalDeps int) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	tables = len(tc.tables)

	for _, deps := range tc.tableDeps {
		if len(deps) > 0 {
			withDeps++
			totalDeps += len(deps)
		}
	}

	return tables, withDeps, totalDeps
}

func (tc *tableCache) getDependencies(tableOID string) []string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	deps, ok := tc.tableDeps[tableOID]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}
	return result
}
