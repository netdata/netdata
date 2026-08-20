// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"maps"
	"math/rand"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
)

// Table Cache Overview:
// The cache retains only instance OIDs and tags for ordinary value tables; it
// never retains SNMP values. Later collections GET current values for those
// instances. Topology presence, BGP, and licensing tables always walk fresh.
// Tables with dependencies expire together to maintain tag consistency.

type tableCache struct {
	// Table OID -> config ID -> cached entry
	tables map[string]map[string]tableCacheEntry

	// Table OID -> when cached (table level, not config level)
	timestamps map[string]time.Time

	// Table OID -> specific TTL for this table (with jitter applied)
	tableTTLs map[string]time.Duration

	// Cache dependency direction is retained across collections so settlement
	// can invalidate reverse dependents omitted from the current session.
	dependenciesByTable map[string]map[string]bool
	dependentsByTable   map[string]map[string]bool

	baseTTL   time.Duration
	jitterPct float64
	mu        sync.RWMutex
	rng       *rand.Rand
}

type tableCacheEntryKind uint8

const (
	tableCacheEntryInvalid tableCacheEntryKind = iota
	tableCacheEntryRows
	tableCacheEntryMarker
)

type tableCacheEntry struct {
	kind tableCacheEntryKind

	// Index -> column OID -> full OID
	oidMap map[string]map[string]string

	// Index -> tag name -> value
	tagValues map[string]map[string]string
}

func newTableCache(baseTTL time.Duration, jitterPct float64) *tableCache {
	return &tableCache{
		tables:              make(map[string]map[string]tableCacheEntry),
		timestamps:          make(map[string]time.Time),
		tableTTLs:           make(map[string]time.Duration),
		dependenciesByTable: make(map[string]map[string]bool),
		dependentsByTable:   make(map[string]map[string]bool),
		baseTTL:             baseTTL,
		jitterPct:           jitterPct,
		rng:                 rand.New(rand.NewSource(time.Now().UnixNano())),
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
	if !ok || entry.kind != tableCacheEntryRows {
		return nil, nil, false
	}

	return entry.oidMap, entry.tagValues, true
}

func (tc *tableCache) cacheRows(cfg ddprofiledefinition.MetricsConfig, oidMap map[string]map[string]string, tagValues map[string]map[string]string, dependencies []string) {
	if len(oidMap) == 0 {
		return
	}

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
		maps.Copy(columnsCopy, columns)
		oidsCopy[index] = columnsCopy
	}

	tagsCopy := make(map[string]map[string]string, len(tagValues))
	for index, tags := range tagValues {
		tagCopy := make(map[string]string, len(tags))
		maps.Copy(tagCopy, tags)
		tagsCopy[index] = tagCopy
	}

	// Create config entries map if it doesn't exist
	if tc.tables[tableOID] == nil {
		tc.tables[tableOID] = make(map[string]tableCacheEntry)
	}

	// Store the entry
	tc.tables[tableOID][configID] = tableCacheEntry{
		kind:      tableCacheEntryRows,
		oidMap:    oidsCopy,
		tagValues: tagsCopy,
	}

	// Update table-level metadata only if this is the first config for this table
	if _, exists := tc.timestamps[tableOID]; !exists {
		tc.timestamps[tableOID] = time.Now()
		tc.tableTTLs[tableOID] = tc.calculateTableTTL()
	}

	// Retain both directions: expiry uses the full component, while settlement
	// rollback follows only reverse dependents.
	if len(dependencies) > 0 {
		if tc.dependenciesByTable[tableOID] == nil {
			tc.dependenciesByTable[tableOID] = make(map[string]bool)
		}

		for _, depTable := range dependencies {
			tc.dependenciesByTable[tableOID][depTable] = true
			if tc.dependentsByTable[depTable] == nil {
				tc.dependentsByTable[depTable] = make(map[string]bool)
			}
			tc.dependentsByTable[depTable][tableOID] = true
		}
	}
}

// cacheMarker records a successful symbol-free auxiliary walk for route
// planning. Markers never provide row data to cached collection.
func (tc *tableCache) cacheMarker(cfg ddprofiledefinition.MetricsConfig) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	if tc.baseTTL == 0 {
		return
	}

	tableOID := cfg.Table.OID
	if tc.tables[tableOID] == nil {
		tc.tables[tableOID] = make(map[string]tableCacheEntry)
	}
	tc.tables[tableOID][tc.generateConfigID(cfg)] = tableCacheEntry{kind: tableCacheEntryMarker}

	if _, exists := tc.timestamps[tableOID]; !exists {
		tc.timestamps[tableOID] = time.Now()
		tc.tableTTLs[tableOID] = tc.calculateTableTTL()
	}
}

func (tc *tableCache) invalidateConfig(cfg ddprofiledefinition.MetricsConfig) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tableOID := cfg.Table.OID
	configs, ok := tc.tables[tableOID]
	if !ok {
		return
	}

	delete(configs, tc.generateConfigID(cfg))
	if len(configs) > 0 {
		return
	}

	tc.clearTablesLocked(map[string]bool{tableOID: true})
}

func (tc *tableCache) invalidateTable(tableOID string) {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	tc.clearTablesLocked(map[string]bool{tableOID: true})
}

func (tc *tableCache) discardTables(tableOIDs []string) {
	if len(tableOIDs) == 0 {
		return
	}

	tc.mu.Lock()
	defer tc.mu.Unlock()
	if tc.baseTTL == 0 {
		return
	}

	tables := make(map[string]bool, len(tableOIDs))
	var addDependents func(string)
	addDependents = func(tableOID string) {
		if tables[tableOID] {
			return
		}
		tables[tableOID] = true
		for dependent := range tc.dependentsByTable[tableOID] {
			addDependents(dependent)
		}
	}
	for _, tableOID := range tableOIDs {
		addDependents(tableOID)
	}

	for tableOID := range tables {
		tc.discardTableLocked(tableOID)
	}
}

func (tc *tableCache) enabled() bool {
	tc.mu.RLock()
	defer tc.mu.RUnlock()
	return tc.baseTTL != 0
}

func (tc *tableCache) clearExpired() []string {
	tc.mu.Lock()
	defer tc.mu.Unlock()

	now := time.Now()

	expiredTables := make(map[string]bool)
	for tableOID, timestamp := range tc.timestamps {
		ttl := tc.tableTTLs[tableOID]
		if now.Sub(timestamp) > ttl {
			expiredTables[tableOID] = true
		}
	}
	return tc.clearTablesLocked(expiredTables)
}

func (tc *tableCache) clearTablesLocked(tables map[string]bool) []string {
	collected := make(map[string]bool)
	var addRelatedTables func(tableOID string)
	addRelatedTables = func(tableOID string) {
		if collected[tableOID] {
			return
		}
		collected[tableOID] = true

		for dependency := range tc.dependenciesByTable[tableOID] {
			addRelatedTables(dependency)
		}
		for dependent := range tc.dependentsByTable[tableOID] {
			addRelatedTables(dependent)
		}
	}

	for tableOID := range tables {
		addRelatedTables(tableOID)
	}

	cleared := make([]string, 0, len(collected))
	for tableOID := range collected {
		tc.discardTableLocked(tableOID)
		cleared = append(cleared, tableOID)
	}

	return cleared
}

func (tc *tableCache) discardTableLocked(tableOID string) {
	delete(tc.tables, tableOID)
	delete(tc.timestamps, tableOID)
	delete(tc.tableTTLs, tableOID)

	for dependency := range tc.dependenciesByTable[tableOID] {
		delete(tc.dependentsByTable[dependency], tableOID)
		if len(tc.dependentsByTable[dependency]) == 0 {
			delete(tc.dependentsByTable, dependency)
		}
	}
	for dependent := range tc.dependentsByTable[tableOID] {
		delete(tc.dependenciesByTable[dependent], tableOID)
		if len(tc.dependenciesByTable[dependent]) == 0 {
			delete(tc.dependenciesByTable, dependent)
		}
	}
	delete(tc.dependenciesByTable, tableOID)
	delete(tc.dependentsByTable, tableOID)
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
		tc.dependenciesByTable = make(map[string]map[string]bool)
		tc.dependentsByTable = make(map[string]map[string]bool)
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

	entry, ok := configEntries[configID]
	return ok && (entry.kind == tableCacheEntryRows || entry.kind == tableCacheEntryMarker)
}

// generateConfigID creates a unique identifier for a MetricsConfig
func (tc *tableCache) generateConfigID(cfg ddprofiledefinition.MetricsConfig) string {
	var sb strings.Builder

	if cfg.MIB != "" {
		sb.WriteString(cfg.MIB)
		sb.WriteString("|")
	}

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

	for _, deps := range tc.dependenciesByTable {
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

	deps, ok := tc.dependenciesByTable[tableOID]
	if !ok {
		return nil
	}

	result := make([]string, 0, len(deps))
	for dep := range deps {
		result = append(result, dep)
	}
	return result
}

func (tc *tableCache) getDependents(tableOID string) []string {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	dependents := tc.dependentsByTable[tableOID]
	result := make([]string, 0, len(dependents))
	for dependent := range dependents {
		result = append(result, dependent)
	}
	return result
}
