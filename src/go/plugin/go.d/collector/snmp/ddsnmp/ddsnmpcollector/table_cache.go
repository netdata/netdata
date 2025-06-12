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
// - Per-table TTL with jitter prevents simultaneous refreshes

type tableCache struct {
	// Table OID -> row index -> column OID -> full OID
	tables map[string]map[string]map[string]string

	// Table OID -> when cached
	timestamps map[string]time.Time

	// Table OID -> specific TTL for this table (with jitter applied)
	tableTTLs map[string]time.Duration

	// Table OID -> tag values (index -> tag name -> value)
	tagValues map[string]map[string]map[string]string

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
		baseTTL:    baseTTL,
		jitterPct:  jitterPct,
		rng:        rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

func (tc *tableCache) calculateTableTTL() time.Duration {
	base := float64(tc.baseTTL)
	jitter := tc.jitterPct

	// Random jitter between 0 and +jitterPct
	// Note: This is called from within lock, so don't acquire lock here
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
}

func (tc *tableCache) clearExpired() []string {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	var expired []string
	now := time.Now()

	for tableOID, timestamp := range tc.timestamps {
		ttl := tc.tableTTLs[tableOID]
		if now.Sub(timestamp) > ttl {
			delete(tc.tables, tableOID)
			delete(tc.timestamps, tableOID)
			delete(tc.tableTTLs, tableOID)
			delete(tc.tagValues, tableOID)
			expired = append(expired, tableOID)
		}
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
	}
}
