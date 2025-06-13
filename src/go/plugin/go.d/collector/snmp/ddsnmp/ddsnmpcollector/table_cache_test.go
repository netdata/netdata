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
			oidMap := map[string]map[string]string{
				"1": {
					"1.3.6.1.2.1.2.2.1.2":  "1.3.6.1.2.1.2.2.1.2.1",
					"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.1",
				},
				"2": {
					"1.3.6.1.2.1.2.2.1.2":  "1.3.6.1.2.1.2.2.1.2.2",
					"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.2",
				},
			}
			tagValues := map[string]map[string]string{
				"1": {"interface": "eth0"},
				"2": {"interface": "eth1"},
			}

			// Cache data
			cache.cacheData(tableOID, oidMap, tagValues)

			// Retrieve cached data - should work
			cachedOIDs, cachedTags, found := cache.getCachedData(tableOID)
			assert.True(t, found)
			assert.Equal(t, oidMap, cachedOIDs)
			assert.Equal(t, tagValues, cachedTags)

			// Wait for expiration (considering jitter)
			time.Sleep(150 * time.Millisecond)

			// Should be expired now
			_, _, found = cache.getCachedData(tableOID)
			assert.False(t, found)

			// Clean expired entries
			expired := cache.clearExpired()
			assert.Contains(t, expired, tableOID)

			// Cache should be empty now
			assert.Empty(t, cache.tables)
			assert.Empty(t, cache.timestamps)
			assert.Empty(t, cache.tableTTLs)
			assert.Empty(t, cache.tagValues)
		})
	}
}

func TestTableCacheDependencies(t *testing.T) {
	cache := newTableCache(200*time.Millisecond, 0)

	// Test data for three related tables
	table1OID := "1.3.6.1.2.1.2.2"     // ifTable
	table2OID := "1.3.6.1.2.1.31.1.1"  // ifXTable
	table3OID := "1.3.6.1.4.1.9.9.276" // cieIfInterfaceTable

	oidMap1 := map[string]map[string]string{
		"1": {"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.1"},
	}
	tagValues1 := map[string]map[string]string{
		"1": {"interface": "eth0"},
	}

	oidMap2 := map[string]map[string]string{
		"1": {"1.3.6.1.2.1.31.1.1.1.1": "1.3.6.1.2.1.31.1.1.1.1.1"},
	}
	tagValues2 := map[string]map[string]string{
		"1": {"ifname": "GigabitEthernet0/1"},
	}

	oidMap3 := map[string]map[string]string{
		"1": {"1.3.6.1.4.1.9.9.276.1.1": "1.3.6.1.4.1.9.9.276.1.1.1"},
	}

	// Cache tables with dependencies
	// table1 and table2 depend on each other
	cache.cacheDataWithDeps(table1OID, oidMap1, tagValues1, []string{table2OID})
	cache.cacheDataWithDeps(table2OID, oidMap2, tagValues2, []string{table1OID})

	// table3 depends on table2
	cache.cacheDataWithDeps(table3OID, oidMap3, nil, []string{table2OID})

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
	assert.Empty(t, cache.tables)
	assert.Empty(t, cache.tableDeps)
}

func TestTableCacheDependenciesCascade(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Create a chain: A -> B -> C -> D
	tableA := "1.3.6.1.2.1.1"
	tableB := "1.3.6.1.2.1.2"
	tableC := "1.3.6.1.2.1.3"
	tableD := "1.3.6.1.2.1.4"

	data := map[string]map[string]string{"1": {"col": "val"}}

	// Cache with chain dependencies
	cache.cacheDataWithDeps(tableA, data, nil, []string{tableB})
	cache.cacheDataWithDeps(tableB, data, nil, []string{tableA, tableC})
	cache.cacheDataWithDeps(tableC, data, nil, []string{tableB, tableD})
	cache.cacheDataWithDeps(tableD, data, nil, []string{tableC})

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

	data := map[string]map[string]string{"1": {"col": "val"}}

	// Cache tables
	cache.cacheDataWithDeps(table1, data, nil, []string{table2})
	cache.cacheDataWithDeps(table2, data, nil, []string{table1})
	cache.cacheData(table3, data, nil) // No dependencies

	// All should be cached
	_, _, found1 := cache.getCachedData(table1)
	_, _, found2 := cache.getCachedData(table2)
	_, _, found3 := cache.getCachedData(table3)
	assert.True(t, found1)
	assert.True(t, found2)
	assert.True(t, found3)

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
	oidMap := map[string]map[string]string{
		"1": {"1.3.6.1.2.1.2.2.1.2": "1.3.6.1.2.1.2.2.1.2.1"},
	}
	tagValues := map[string]map[string]string{
		"1": {"interface": "eth0"},
	}

	// Try to cache data
	cache.cacheData(tableOID, oidMap, tagValues)

	// Should not find anything
	_, _, found := cache.getCachedData(tableOID)
	assert.False(t, found)

	// Try to cache with dependencies
	cache.cacheDataWithDeps(tableOID, oidMap, tagValues, []string{"other.table"})

	// Should not find anything
	_, _, found = cache.getCachedData(tableOID)
	assert.False(t, found)
	assert.False(t, cache.areTablesCached([]string{tableOID}))

	// Cache should remain empty
	assert.Empty(t, cache.tables)
}

func TestTableCacheDeepCopy(t *testing.T) {
	cache := newTableCache(1*time.Hour, 0)

	// Original data
	oidMap := map[string]map[string]string{
		"1": {"col1": "1.2.3.4.1"},
	}
	tagValues := map[string]map[string]string{
		"1": {"tag1": "value1"},
	}

	// Cache the data
	cache.cacheData("table1", oidMap, tagValues)

	// Modify original maps
	oidMap["1"]["col2"] = "should not appear"
	tagValues["1"]["tag2"] = "should not appear"

	// Retrieve cached data
	cachedOIDs, cachedTags, found := cache.getCachedData("table1")
	require.True(t, found)

	// Cached data should not have the modifications
	assert.NotContains(t, cachedOIDs["1"], "col2")
	assert.NotContains(t, cachedTags["1"], "tag2")
}

func TestTableCacheDependencyCleanup(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Create circular dependencies
	table1 := "1.3.6.1.2.1.1"
	table2 := "1.3.6.1.2.1.2"

	data := map[string]map[string]string{"1": {"col": "val"}}

	// Cache with circular deps
	cache.cacheDataWithDeps(table1, data, nil, []string{table2})
	cache.cacheDataWithDeps(table2, data, nil, []string{table1})

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
	cache.cacheDataWithDeps("tableA",
		map[string]map[string]string{"1": {"col": "val"}},
		nil,
		[]string{"tableB"})

	// tableA should be cached
	_, _, found := cache.getCachedData("tableA")
	assert.True(t, found)

	// tableB should not be cached
	_, _, found = cache.getCachedData("tableB")
	assert.False(t, found)

	// Dependencies should exist
	assert.Contains(t, cache.getDependencies("tableA"), "tableB")
	assert.Contains(t, cache.getDependencies("tableB"), "tableA")

	// Now cache tableB
	cache.cacheDataWithDeps("tableB",
		map[string]map[string]string{"1": {"col": "val"}},
		nil,
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
