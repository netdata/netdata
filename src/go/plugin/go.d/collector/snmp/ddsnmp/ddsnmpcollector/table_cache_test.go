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
			// Create cache with 100ms TTL and 20% jitter
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
