// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// Table Cache Overview:
// The table cache converts repeated SNMP walks into efficient GET operations.
// - First collection: Full walk, cache structure and tags
// - Subsequent collections: GET metrics only, use cached tags
// - Tables with dependencies expire together to maintain consistency
// - Supports multiple metric configurations per table

type tableCache struct {
	// Table OID -> config ID -> cached entry
	tables map[string]map[string]tableCacheEntry

	// Table OID -> when cached (table level, not config level)
	timestamps map[string]time.Time

	// Table OID -> specific TTL for this table (with jitter applied)
	tableTTLs map[string]time.Duration

	// Table OID -> list of dependent table OIDs (bidirectional)
	// If table A depends on table B, both A->B and B->A are stored
	tableDeps map[string]map[string]bool

	baseTTL   time.Duration
	jitterPct float64
	mu        sync.RWMutex
	rng       *rand.Rand
}

type tableCacheEntry struct {
	// Index -> column OID -> full OID
	oidMap map[string]map[string]string

	// Index -> tag name -> value
	tagValues map[string]map[string]string
}

func newTableCache(baseTTL time.Duration, jitterPct float64) *tableCache {
	return &tableCache{
		tables:     make(map[string]map[string]tableCacheEntry),
		timestamps: make(map[string]time.Time),
		tableTTLs:  make(map[string]time.Duration),
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

func (tc *tableCache) getCachedData(cfg ddprofiledefinition.MetricsConfig) (oids map[string]map[string]string, tags map[string]map[string]string, found bool) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.baseTTL == 0 {
		return nil, nil, false
	}

	tableOID := cfg.Table.OID
	configID := tc.generateConfigID(cfg)

	timestamp, ok := tc.timestamps[tableOID]
	if !ok {
		return nil, nil, false
	}

	ttl, ok := tc.tableTTLs[tableOID]
	if !ok || time.Since(timestamp) > ttl {
		return nil, nil, false
	}

	configEntries, ok := tc.tables[tableOID]
	if !ok {
		return nil, nil, false
	}

	entry, ok := configEntries[configID]
	if !ok {
		return nil, nil, false
	}

	return entry.oidMap, entry.tagValues, true
}

func (tc *tableCache) cacheData(cfg ddprofiledefinition.MetricsConfig, oidMap map[string]map[string]string, tagValues map[string]map[string]string, dependencies []string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.baseTTL == 0 {
		return
	}

	tableOID := cfg.Table.OID
	configID := tc.generateConfigID(cfg)

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

	// Create config entries map if it doesn't exist
	if tc.tables[tableOID] == nil {
		tc.tables[tableOID] = make(map[string]tableCacheEntry)
	}

	// Store the entry
	tc.tables[tableOID][configID] = tableCacheEntry{
		oidMap:    oidsCopy,
		tagValues: tagsCopy,
	}

	// Update table-level metadata only if this is the first config for this table
	if _, exists := tc.timestamps[tableOID]; !exists {
		tc.timestamps[tableOID] = time.Now()
		tc.tableTTLs[tableOID] = tc.calculateTableTTL()
	}

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
	visited := make(map[string]bool)
	var cascadeExpiration func(tableOID string)
	cascadeExpiration = func(tableOID string) {
		if visited[tableOID] {
			return
		}
		visited[tableOID] = true
		expiredTables[tableOID] = true

		for dep := range tc.tableDeps[tableOID] {
			cascadeExpiration(dep)
		}
	}

	// Start cascade from naturally expired tables
	for tableOID := range expiredTables {
		cascadeExpiration(tableOID)
	}

	// Clear all expired tables
	for tableOID := range expiredTables {
		delete(tc.tables, tableOID)
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
		tc.tables = make(map[string]map[string]tableCacheEntry)
		tc.timestamps = make(map[string]time.Time)
		tc.tableTTLs = make(map[string]time.Duration)
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

// Check if a specific table config is cached
func (tc *tableCache) isConfigCached(cfg ddprofiledefinition.MetricsConfig) bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	if tc.baseTTL == 0 {
		return false
	}

	tableOID := cfg.Table.OID
	configID := tc.generateConfigID(cfg)

	timestamp, ok := tc.timestamps[tableOID]
	if !ok {
		return false
	}

	ttl, ok := tc.tableTTLs[tableOID]
	if !ok || time.Since(timestamp) > ttl {
		return false
	}

	configEntries, ok := tc.tables[tableOID]
	if !ok {
		return false
	}

	_, ok = configEntries[configID]
	return ok
}

// generateConfigID creates a unique identifier for a MetricsConfig
func (tc *tableCache) generateConfigID(cfg ddprofiledefinition.MetricsConfig) string {
	var sb strings.Builder

	if cfg.Table.Name != "" {
		sb.WriteString(cfg.Table.Name)
	}

	names := make([]string, 0, len(cfg.Symbols))
	for _, sym := range cfg.Symbols {
		names = append(names, sym.Name)
	}
	sort.Strings(names)

	if sb.Len() > 0 && len(names) > 0 {
		sb.WriteString(",")
	}

	sb.WriteString(strings.Join(names, ","))

	return sb.String()
}

func (tc *tableCache) stats() (tables int, configs int, withDeps int, totalDeps int) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	tables = len(tc.tables)

	for _, configMap := range tc.tables {
		configs += len(configMap)
	}

	for _, deps := range tc.tableDeps {
		if len(deps) > 0 {
			withDeps++
			totalDeps += len(deps)
		}
	}

	return tables, configs, withDeps, totalDeps
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
