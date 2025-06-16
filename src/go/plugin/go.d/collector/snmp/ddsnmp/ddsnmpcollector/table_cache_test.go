// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestTableCache(t *testing.T) {
	tests := map[string]struct {
		name string
	}{
		"basic cache operations": {},
	}

	for name := range tests {
		t.Run(name, func(t *testing.T) {
			cache := newTableCache(100*time.Millisecond, 0.2)

			// Test data
			tableOID := "1.3.6.1.2.1.2.2"
			indexes := []string{"1", "2", "3"}

			// Cache data
			cache.cacheIndexes(tableOID, indexes)

			// Retrieve cached data - should work
			cachedIndexes, found := cache.getCachedIndexes(tableOID)
			assert.True(t, found)
			assert.Equal(t, indexes, cachedIndexes)

			// Wait for expiration (considering jitter)
			time.Sleep(150 * time.Millisecond)

			// Should be expired now
			_, found = cache.getCachedIndexes(tableOID)
			assert.False(t, found)

			// Clean expired entries
			expired := cache.clearExpired()
			assert.Contains(t, expired, tableOID)

			// Cache should be empty now
			assert.Empty(t, cache.tableIndexes)
			assert.Empty(t, cache.timestamps)
			assert.Empty(t, cache.tableTTLs)
		})
	}
}

func TestTableCacheDependencies(t *testing.T) {
	cache := newTableCache(200*time.Millisecond, 0)

	// Test data for three related tables
	table1OID := "1.3.6.1.2.1.2.2"     // ifTable
	table2OID := "1.3.6.1.2.1.31.1.1"  // ifXTable
	table3OID := "1.3.6.1.4.1.9.9.276" // cieIfInterfaceTable

	indexes1 := []string{"1", "2"}
	indexes2 := []string{"1", "2"}
	indexes3 := []string{"1", "2"}

	// Cache tables with dependencies
	// table1 and table2 depend on each other
	cache.cacheIndexesWithDeps(table1OID, indexes1, []string{table2OID})
	cache.cacheIndexesWithDeps(table2OID, indexes2, []string{table1OID})

	// table3 depends on table2
	cache.cacheIndexesWithDeps(table3OID, indexes3, []string{table2OID})

	// All tables should be cached
	assert.True(t, cache.areTablesCached([]string{table1OID, table2OID, table3OID}))

	// Check dependencies
	deps1 := cache.getDependencies(table1OID)
	assert.Contains(t, deps1, table2OID)

	deps2 := cache.getDependencies(table2OID)
	assert.Contains(t, deps2, table1OID)
	assert.Contains(t, deps2, table3OID) // Bidirectional

	deps3 := cache.getDependencies(table3OID)
	assert.Contains(t, deps3, table2OID)

	// Get cache stats
	tables, withDeps, totalDeps := cache.stats()
	assert.Equal(t, 3, tables)
	assert.Equal(t, 3, withDeps)
	assert.Equal(t, 4, totalDeps) // 1->2, 2->1, 2->3, 3->2

	// Sleep to let table1 expire naturally
	time.Sleep(220 * time.Millisecond)

	// Clear expired - should cascade to all dependent tables
	expired := cache.clearExpired()

	// All three tables should be expired due to dependencies
	assert.Len(t, expired, 3)
	assert.Contains(t, expired, table1OID)
	assert.Contains(t, expired, table2OID)
	assert.Contains(t, expired, table3OID)

	// Cache should be empty
	assert.Empty(t, cache.tableIndexes)
	assert.Empty(t, cache.tableDeps)
}

func TestTableCacheDependenciesCascade(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Create a chain: A -> B -> C -> D
	tableA := "1.3.6.1.2.1.1"
	tableB := "1.3.6.1.2.1.2"
	tableC := "1.3.6.1.2.1.3"
	tableD := "1.3.6.1.2.1.4"

	indexes := []string{"1"}

	// Cache with chain dependencies
	cache.cacheIndexesWithDeps(tableA, indexes, []string{tableB})
	cache.cacheIndexesWithDeps(tableB, indexes, []string{tableA, tableC})
	cache.cacheIndexesWithDeps(tableC, indexes, []string{tableB, tableD})
	cache.cacheIndexesWithDeps(tableD, indexes, []string{tableC})

	// All should be cached
	assert.True(t, cache.areTablesCached([]string{tableA, tableB, tableC, tableD}))

	// Wait for A to expire
	time.Sleep(120 * time.Millisecond)

	// Clear expired - should cascade through entire chain
	expired := cache.clearExpired()

	// All tables should expire due to cascade
	assert.Len(t, expired, 4)
	assert.Contains(t, expired, tableA)
	assert.Contains(t, expired, tableB)
	assert.Contains(t, expired, tableC)
	assert.Contains(t, expired, tableD)
}

func TestTableCacheMixedDependencies(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Tables with deps
	table1 := "1.3.6.1.2.1.1"
	table2 := "1.3.6.1.2.1.2"

	// Table without deps
	table3 := "1.3.6.1.2.1.3"

	indexes := []string{"1", "2", "3"}

	// Cache tables
	cache.cacheIndexesWithDeps(table1, indexes, []string{table2})
	cache.cacheIndexesWithDeps(table2, indexes, []string{table1})
	cache.cacheIndexes(table3, indexes) // No dependencies

	// All should be cached
	indexes1, found1 := cache.getCachedIndexes(table1)
	indexes2, found2 := cache.getCachedIndexes(table2)
	indexes3, found3 := cache.getCachedIndexes(table3)
	assert.True(t, found1)
	assert.True(t, found2)
	assert.True(t, found3)
	assert.Equal(t, indexes, indexes1)
	assert.Equal(t, indexes, indexes2)
	assert.Equal(t, indexes, indexes3)

	// Wait for expiration
	time.Sleep(120 * time.Millisecond)

	// Clear expired
	expired := cache.clearExpired()

	// All should be expired (table3 independently, table1&2 together)
	assert.Len(t, expired, 3)
}

func TestTableCacheJitter(t *testing.T) {
	cache := newTableCache(1*time.Second, 0.2) // 1 second with 20% jitter

	// Calculate multiple TTLs to verify jitter
	ttls := make([]time.Duration, 10)
	for i := range ttls {
		ttls[i] = cache.calculateTableTTL()
		time.Sleep(1 * time.Millisecond) // Ensure different timestamps
	}

	// All TTLs should be between 800ms and 1200ms (±20%)
	for _, ttl := range ttls {
		assert.GreaterOrEqual(t, ttl, 800*time.Millisecond)
		assert.LessOrEqual(t, ttl, 1200*time.Millisecond)
	}

	// Verify they're not all the same (jitter is working)
	uniqueTTLs := make(map[time.Duration]bool)
	for _, ttl := range ttls {
		uniqueTTLs[ttl] = true
	}
	assert.Greater(t, len(uniqueTTLs), 1, "Expected different TTLs due to jitter")
}

func TestTableCacheDisabled(t *testing.T) {
	cache := newTableCache(0, 0) // Disabled cache

	tableOID := "1.3.6.1.2.1.2.2"
	indexes := []string{"1", "2", "3"}

	// Try to cache data
	cache.cacheIndexes(tableOID, indexes)

	// Should not find anything
	_, found := cache.getCachedIndexes(tableOID)
	assert.False(t, found)

	// Try to cache with dependencies
	cache.cacheIndexesWithDeps(tableOID, indexes, []string{"other.table"})

	// Should not find anything
	_, found = cache.getCachedIndexes(tableOID)
	assert.False(t, found)
	assert.False(t, cache.areTablesCached([]string{tableOID}))

	// Cache should remain empty
	assert.Empty(t, cache.tableIndexes)
}

func TestTableCacheDeepCopy(t *testing.T) {
	cache := newTableCache(1*time.Hour, 0)

	// Original data
	indexes := []string{"1", "2", "3"}

	// Cache the data
	cache.cacheIndexes("table1", indexes)

	// Modify original slice
	indexes[0] = "999"
	indexes = append(indexes, "4")

	// Retrieve cached data
	cachedIndexes, found := cache.getCachedIndexes("table1")
	require.True(t, found)

	// Cached data should not have the modifications
	assert.Equal(t, []string{"1", "2", "3"}, cachedIndexes)
	assert.NotContains(t, cachedIndexes, "999")
	assert.NotContains(t, cachedIndexes, "4")
}

func TestTableCacheDependencyCleanup(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Create circular dependencies
	table1 := "1.3.6.1.2.1.1"
	table2 := "1.3.6.1.2.1.2"

	indexes := []string{"1"}

	// Cache with circular deps
	cache.cacheIndexesWithDeps(table1, indexes, []string{table2})
	cache.cacheIndexesWithDeps(table2, indexes, []string{table1})

	// Check initial state
	tables, withDeps, totalDeps := cache.stats()
	assert.Equal(t, 2, tables)
	assert.Equal(t, 2, withDeps)
	assert.Equal(t, 2, totalDeps)

	// Wait for expiration
	time.Sleep(120 * time.Millisecond)

	// Clear expired
	cache.clearExpired()

	// Check cleanup
	tables, withDeps, totalDeps = cache.stats()
	assert.Equal(t, 0, tables)
	assert.Equal(t, 0, withDeps)
	assert.Equal(t, 0, totalDeps)

	// Dependencies should be cleaned up
	assert.Empty(t, cache.tableDeps)
}

func TestTableCacheNonExistentDependency(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Cache tableA with dependency on non-existent tableB
	cache.cacheIndexesWithDeps("tableA",
		[]string{"1", "2"},
		[]string{"tableB"})

	// tableA should be cached
	indexes, found := cache.getCachedIndexes("tableA")
	assert.True(t, found)
	assert.Equal(t, []string{"1", "2"}, indexes)

	// tableB should not be cached
	_, found = cache.getCachedIndexes("tableB")
	assert.False(t, found)

	// Dependencies should exist
	assert.Contains(t, cache.getDependencies("tableA"), "tableB")
	assert.Contains(t, cache.getDependencies("tableB"), "tableA")

	// Now cache tableB
	cache.cacheIndexesWithDeps("tableB",
		[]string{"1", "2"},
		[]string{"tableA"})

	// Both should be cached
	assert.True(t, cache.areTablesCached([]string{"tableA", "tableB"}))

	// Wait for expiration
	time.Sleep(120 * time.Millisecond)

	// Clear expired - both should expire together
	expired := cache.clearExpired()
	assert.Len(t, expired, 2)
	assert.Contains(t, expired, "tableA")
	assert.Contains(t, expired, "tableB")
}

// Helper methods that need to be added to tableCache for tests
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

func (tc *tableCache) stats() (tables int, withDeps int, totalDeps int) {
	tc.mu.RLock()
	defer tc.mu.RUnlock()

	tables = len(tc.tableIndexes)

	for _, deps := range tc.tableDeps {
		if len(deps) > 0 {
			withDeps++
			totalDeps += len(deps)
		}
	}

	return tables, withDeps, totalDeps
}
