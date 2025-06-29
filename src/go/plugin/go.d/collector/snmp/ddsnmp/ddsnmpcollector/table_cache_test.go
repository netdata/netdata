// SPDX-License-Identifier: GPL-3.0-or-later

package ddsnmpcollector

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/snmp/ddsnmp/ddprofiledefinition"
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
			cfg := ddprofiledefinition.MetricsConfig{
				Table: ddprofiledefinition.SymbolConfig{
					OID:  "1.3.6.1.2.1.2.2",
					Name: "ifTable",
				},
				Symbols: []ddprofiledefinition.SymbolConfig{
					{
						OID:  "1.3.6.1.2.1.2.2.1.10",
						Name: "ifInOctets",
					},
					{
						OID:  "1.3.6.1.2.1.2.2.1.16",
						Name: "ifOutOctets",
					},
				},
			}

			oidMap := map[string]map[string]string{
				"1": {
					"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.1",
					"1.3.6.1.2.1.2.2.1.16": "1.3.6.1.2.1.2.2.1.16.1",
				},
				"2": {
					"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.2",
					"1.3.6.1.2.1.2.2.1.16": "1.3.6.1.2.1.2.2.1.16.2",
				},
			}
			tagValues := map[string]map[string]string{
				"1": {"interface": "eth0"},
				"2": {"interface": "eth1"},
			}

			// Cache data
			cache.cacheData(cfg, oidMap, tagValues, nil)

			// Retrieve cached data - should work
			cachedOIDs, cachedTags, found := cache.getCachedData(cfg)
			assert.True(t, found)
			assert.Equal(t, oidMap, cachedOIDs)
			assert.Equal(t, tagValues, cachedTags)

			// Try to get with different config (different symbols)
			cfg2 := cfg
			cfg2.Symbols = []ddprofiledefinition.SymbolConfig{
				{
					OID:  "1.3.6.1.2.1.2.2.1.7",
					Name: "ifAdminStatus",
				},
			}
			_, _, found = cache.getCachedData(cfg2)
			assert.False(t, found)

			// Wait for expiration (considering jitter)
			time.Sleep(150 * time.Millisecond)

			// Should be expired now
			_, _, found = cache.getCachedData(cfg)
			assert.False(t, found)

			// Clean expired entries
			expired := cache.clearExpired()
			assert.Contains(t, expired, cfg.Table.OID)

			// Cache should be empty now
			assert.Empty(t, cache.tables)
			assert.Empty(t, cache.timestamps)
			assert.Empty(t, cache.tableTTLs)
		})
	}
}

func TestTableCacheMultipleConfigs(t *testing.T) {
	cache := newTableCache(1*time.Hour, 0)

	tableOID := "1.3.6.1.2.1.2.2"

	// Config 1 - packet counters
	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{
				OID:  "1.3.6.1.2.1.2.2.1.10",
				Name: "ifInOctets",
			},
			{
				OID:  "1.3.6.1.2.1.2.2.1.16",
				Name: "ifOutOctets",
			},
		},
	}
	oidMap1 := map[string]map[string]string{
		"1": {
			"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.1",
			"1.3.6.1.2.1.2.2.1.16": "1.3.6.1.2.1.2.2.1.16.1",
		},
	}
	tagValues1 := map[string]map[string]string{
		"1": {"interface": "eth0"},
	}

	// Config 2 - status metrics (same table, different metrics)
	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{
				OID:  "1.3.6.1.2.1.2.2.1.7",
				Name: "ifAdminStatus",
			},
			{
				OID:  "1.3.6.1.2.1.2.2.1.8",
				Name: "ifOperStatus",
			},
		},
	}
	oidMap2 := map[string]map[string]string{
		"1": {
			"1.3.6.1.2.1.2.2.1.7": "1.3.6.1.2.1.2.2.1.7.1",
			"1.3.6.1.2.1.2.2.1.8": "1.3.6.1.2.1.2.2.1.8.1",
		},
	}
	tagValues2 := map[string]map[string]string{
		"1": {"interface": "eth0", "type": "ethernet"},
	}

	// Cache both configs
	cache.cacheData(cfg1, oidMap1, tagValues1, nil)
	cache.cacheData(cfg2, oidMap2, tagValues2, nil)

	// Both should be retrievable
	cachedOIDs1, cachedTags1, found1 := cache.getCachedData(cfg1)
	assert.True(t, found1)
	assert.Equal(t, oidMap1, cachedOIDs1)
	assert.Equal(t, tagValues1, cachedTags1)

	cachedOIDs2, cachedTags2, found2 := cache.getCachedData(cfg2)
	assert.True(t, found2)
	assert.Equal(t, oidMap2, cachedOIDs2)
	assert.Equal(t, tagValues2, cachedTags2)

	// Stats should show 1 table, 2 configs
	tables, configs, _, _ := cache.stats()
	assert.Equal(t, 1, tables)
	assert.Equal(t, 2, configs)

	// Both configs should report as cached
	assert.True(t, cache.isConfigCached(cfg1))
	assert.True(t, cache.isConfigCached(cfg2))

	// Test that config ID generation is consistent
	// Create same config with symbols in different order
	cfg1Reordered := ddprofiledefinition.MetricsConfig{
		Table: cfg1.Table,
		Symbols: []ddprofiledefinition.SymbolConfig{
			cfg1.Symbols[1], // ifOutOctets first
			cfg1.Symbols[0], // ifInOctets second
		},
	}

	// Should find the same cached data (because symbols are sorted in configID)
	cachedOIDs1Reordered, _, found1Reordered := cache.getCachedData(cfg1Reordered)
	assert.True(t, found1Reordered)
	assert.Equal(t, cachedOIDs1, cachedOIDs1Reordered)
}

func TestTableCacheDependencies(t *testing.T) {
	cache := newTableCache(200*time.Millisecond, 0)

	// Test data for three related tables
	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2.2",
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{
				OID:  "1.3.6.1.2.1.2.2.1.10",
				Name: "ifInOctets",
			},
		},
	}

	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.31.1.1",
			Name: "ifXTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{
				OID:  "1.3.6.1.2.1.31.1.1.1.6",
				Name: "ifHCInOctets",
			},
		},
	}

	cfg3 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.4.1.9.9.276",
			Name: "cieIfInterfaceTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{
				OID:  "1.3.6.1.4.1.9.9.276.1.1",
				Name: "cieIfResetCount",
			},
		},
	}

	oidMap := map[string]map[string]string{
		"1": {"dummy": "dummy.1"},
	}
	tagValues := map[string]map[string]string{
		"1": {"interface": "eth0"},
	}

	// Cache tables with dependencies
	// table1 and table2 depend on each other
	cache.cacheData(cfg1, oidMap, tagValues, []string{cfg2.Table.OID})
	cache.cacheData(cfg2, oidMap, tagValues, []string{cfg1.Table.OID})

	// table3 depends on table2
	cache.cacheData(cfg3, oidMap, nil, []string{cfg2.Table.OID})

	// All tables should be cached
	assert.True(t, cache.areTablesCached([]string{cfg1.Table.OID, cfg2.Table.OID, cfg3.Table.OID}))

	// Check dependencies
	deps1 := cache.getDependencies(cfg1.Table.OID)
	assert.Contains(t, deps1, cfg2.Table.OID)

	deps2 := cache.getDependencies(cfg2.Table.OID)
	assert.Contains(t, deps2, cfg1.Table.OID)
	assert.Contains(t, deps2, cfg3.Table.OID) // Bidirectional

	deps3 := cache.getDependencies(cfg3.Table.OID)
	assert.Contains(t, deps3, cfg2.Table.OID)

	// Get cache stats
	tables, configs, withDeps, totalDeps := cache.stats()
	assert.Equal(t, 3, tables)
	assert.Equal(t, 3, configs)
	assert.Equal(t, 3, withDeps)
	assert.Equal(t, 4, totalDeps) // 1->2, 2->1, 2->3, 3->2

	// Sleep to let table1 expire naturally
	time.Sleep(220 * time.Millisecond)

	// Clear expired - should cascade to all dependent tables
	expired := cache.clearExpired()

	// All three tables should be expired due to dependencies
	assert.Len(t, expired, 3)
	assert.Contains(t, expired, cfg1.Table.OID)
	assert.Contains(t, expired, cfg2.Table.OID)
	assert.Contains(t, expired, cfg3.Table.OID)

	// Cache should be empty
	assert.Empty(t, cache.tables)
	assert.Empty(t, cache.tableDeps)
}

func TestTableCacheDependenciesCascade(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Create a chain: A -> B -> C -> D
	cfgA := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.1",
			Name: "tableA",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.1.1", Name: "metricA"},
		},
	}

	cfgB := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2",
			Name: "tableB",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.1", Name: "metricB"},
		},
	}

	cfgC := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.3",
			Name: "tableC",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.3.1", Name: "metricC"},
		},
	}

	cfgD := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.4",
			Name: "tableD",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.4.1", Name: "metricD"},
		},
	}

	data := map[string]map[string]string{"1": {"col": "val"}}

	// Cache with chain dependencies
	cache.cacheData(cfgA, data, nil, []string{cfgB.Table.OID})
	cache.cacheData(cfgB, data, nil, []string{cfgA.Table.OID, cfgC.Table.OID})
	cache.cacheData(cfgC, data, nil, []string{cfgB.Table.OID, cfgD.Table.OID})
	cache.cacheData(cfgD, data, nil, []string{cfgC.Table.OID})

	// All should be cached
	assert.True(t, cache.areTablesCached([]string{cfgA.Table.OID, cfgB.Table.OID, cfgC.Table.OID, cfgD.Table.OID}))

	// Wait for A to expire
	time.Sleep(120 * time.Millisecond)

	// Clear expired - should cascade through entire chain
	expired := cache.clearExpired()

	// All tables should expire due to cascade
	assert.Len(t, expired, 4)
	assert.Contains(t, expired, cfgA.Table.OID)
	assert.Contains(t, expired, cfgB.Table.OID)
	assert.Contains(t, expired, cfgC.Table.OID)
	assert.Contains(t, expired, cfgD.Table.OID)
}

func TestTableCacheMixedDependencies(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Tables with deps
	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.1",
			Name: "table1",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.1.1", Name: "metric1"},
		},
	}

	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2",
			Name: "table2",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.1", Name: "metric2"},
		},
	}

	// Table without deps
	cfg3 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.3",
			Name: "table3",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.3.1", Name: "metric3"},
		},
	}

	data := map[string]map[string]string{"1": {"col": "val"}}

	// Cache tables
	cache.cacheData(cfg1, data, nil, []string{cfg2.Table.OID})
	cache.cacheData(cfg2, data, nil, []string{cfg1.Table.OID})
	cache.cacheData(cfg3, data, nil, nil) // No dependencies

	// All should be cached
	_, _, found1 := cache.getCachedData(cfg1)
	_, _, found2 := cache.getCachedData(cfg2)
	_, _, found3 := cache.getCachedData(cfg3)
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

	cfg := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2.2",
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
		},
	}

	oidMap := map[string]map[string]string{
		"1": {"1.3.6.1.2.1.2.2.1.10": "1.3.6.1.2.1.2.2.1.10.1"},
	}
	tagValues := map[string]map[string]string{
		"1": {"interface": "eth0"},
	}

	// Try to cache data
	cache.cacheData(cfg, oidMap, tagValues, nil)

	// Should not find anything
	_, _, found := cache.getCachedData(cfg)
	assert.False(t, found)

	// Try to cache with dependencies
	cache.cacheData(cfg, oidMap, tagValues, []string{"other.table"})

	// Should not find anything
	_, _, found = cache.getCachedData(cfg)
	assert.False(t, found)
	assert.False(t, cache.areTablesCached([]string{cfg.Table.OID}))

	// Cache should remain empty
	assert.Empty(t, cache.tables)
}

func TestTableCacheDeepCopy(t *testing.T) {
	cache := newTableCache(1*time.Hour, 0)

	cfg := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.1",
			Name: "table1",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.1.1", Name: "metric1"},
		},
	}

	// Original data
	oidMap := map[string]map[string]string{
		"1": {"col1": "1.2.3.4.1"},
	}
	tagValues := map[string]map[string]string{
		"1": {"tag1": "value1"},
	}

	// Cache the data
	cache.cacheData(cfg, oidMap, tagValues, nil)

	// Modify original maps
	oidMap["1"]["col2"] = "should not appear"
	tagValues["1"]["tag2"] = "should not appear"

	// Retrieve cached data
	cachedOIDs, cachedTags, found := cache.getCachedData(cfg)
	require.True(t, found)

	// Cached data should not have the modifications
	assert.NotContains(t, cachedOIDs["1"], "col2")
	assert.NotContains(t, cachedTags["1"], "tag2")
}

func TestTableCacheDependencyCleanup(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	// Create circular dependencies
	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.1",
			Name: "table1",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.1.1", Name: "metric1"},
		},
	}

	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2",
			Name: "table2",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.1", Name: "metric2"},
		},
	}

	data := map[string]map[string]string{"1": {"col": "val"}}

	// Cache with circular deps
	cache.cacheData(cfg1, data, nil, []string{cfg2.Table.OID})
	cache.cacheData(cfg2, data, nil, []string{cfg1.Table.OID})

	// Check initial state
	tables, configs, withDeps, totalDeps := cache.stats()
	assert.Equal(t, 2, tables)
	assert.Equal(t, 2, configs)
	assert.Equal(t, 2, withDeps)
	assert.Equal(t, 2, totalDeps)

	// Wait for expiration
	time.Sleep(120 * time.Millisecond)

	// Clear expired
	cache.clearExpired()

	// Check cleanup
	tables, configs, withDeps, totalDeps = cache.stats()
	assert.Equal(t, 0, tables)
	assert.Equal(t, 0, configs)
	assert.Equal(t, 0, withDeps)
	assert.Equal(t, 0, totalDeps)

	// Dependencies should be cleaned up
	assert.Empty(t, cache.tableDeps)
}

func TestTableCacheNonExistentDependency(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	cfgA := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.1",
			Name: "tableA",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.1.1", Name: "metricA"},
		},
	}

	cfgB := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2",
			Name: "tableB",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.1", Name: "metricB"},
		},
	}

	// Cache tableA with dependency on non-existent tableB
	cache.cacheData(cfgA,
		map[string]map[string]string{"1": {"col": "val"}},
		nil,
		[]string{cfgB.Table.OID})

	// tableA should be cached
	_, _, found := cache.getCachedData(cfgA)
	assert.True(t, found)

	// tableB should not be cached
	_, _, found = cache.getCachedData(cfgB)
	assert.False(t, found)

	// Dependencies should exist
	assert.Contains(t, cache.getDependencies(cfgA.Table.OID), cfgB.Table.OID)
	assert.Contains(t, cache.getDependencies(cfgB.Table.OID), cfgA.Table.OID)

	// Now cache tableB
	cache.cacheData(cfgB,
		map[string]map[string]string{"1": {"col": "val"}},
		nil,
		[]string{cfgA.Table.OID})

	// Both should be cached
	assert.True(t, cache.areTablesCached([]string{cfgA.Table.OID, cfgB.Table.OID}))

	// Wait for expiration
	time.Sleep(120 * time.Millisecond)

	// Clear expired - both should expire together
	expired := cache.clearExpired()
	assert.Len(t, expired, 2)
	assert.Contains(t, expired, cfgA.Table.OID)
	assert.Contains(t, expired, cfgB.Table.OID)
}

func TestTableCacheMultipleConfigsSameTableExpiration(t *testing.T) {
	cache := newTableCache(100*time.Millisecond, 0)

	tableOID := "1.3.6.1.2.1.2.2"

	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
		},
	}

	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"},
		},
	}

	cfg3 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.7", Name: "ifAdminStatus"},
			{OID: "1.3.6.1.2.1.2.2.1.8", Name: "ifOperStatus"},
		},
	}

	// Cache multiple configs for the same table
	cache.cacheData(cfg1, map[string]map[string]string{"1": {"col1": "val1"}}, nil, nil)
	cache.cacheData(cfg2, map[string]map[string]string{"1": {"col2": "val2"}}, nil, nil)
	cache.cacheData(cfg3, map[string]map[string]string{"1": {"col3": "val3"}}, nil, nil)

	// All configs should be cached
	assert.True(t, cache.isConfigCached(cfg1))
	assert.True(t, cache.isConfigCached(cfg2))
	assert.True(t, cache.isConfigCached(cfg3))

	// Wait for expiration
	time.Sleep(120 * time.Millisecond)

	// Clear expired
	expired := cache.clearExpired()

	// Should expire the table once (not per config)
	assert.Len(t, expired, 1)
	assert.Contains(t, expired, tableOID)

	// All configs should be gone
	assert.False(t, cache.isConfigCached(cfg1))
	assert.False(t, cache.isConfigCached(cfg2))
	assert.False(t, cache.isConfigCached(cfg3))
}

func TestTableCacheConfigIsolation(t *testing.T) {
	cache := newTableCache(1*time.Hour, 0)

	tableOID := "1.3.6.1.2.1.2.2"

	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
		},
	}

	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  tableOID,
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
			{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"},
		},
	}

	// Different tag values for same table, different configs
	config1Tags := map[string]map[string]string{
		"1": {"interface": "eth0"},
	}
	config2Tags := map[string]map[string]string{
		"1": {"interface": "eth0", "type": "ethernet"},
	}

	// Cache configs with different tags
	cache.cacheData(cfg1, nil, config1Tags, nil)
	cache.cacheData(cfg2, nil, config2Tags, nil)

	// Retrieve and verify isolation
	_, tags1, found1 := cache.getCachedData(cfg1)
	assert.True(t, found1)
	assert.Equal(t, config1Tags, tags1)
	assert.NotContains(t, tags1["1"], "type") // Should not have config2's tag

	_, tags2, found2 := cache.getCachedData(cfg2)
	assert.True(t, found2)
	assert.Equal(t, config2Tags, tags2)
	assert.Contains(t, tags2["1"], "type") // Should have its own tag
}

func TestTableCacheConfigIDGeneration(t *testing.T) {
	cache := newTableCache(1*time.Hour, 0)

	// Test that config ID is deterministic and based on symbol names
	cfg1 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2.2",
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
			{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"},
		},
	}

	// Same symbols but in different order
	cfg2 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2.2",
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.16", Name: "ifOutOctets"}, // Swapped order
			{OID: "1.3.6.1.2.1.2.2.1.10", Name: "ifInOctets"},
		},
	}

	// Different symbols
	cfg3 := ddprofiledefinition.MetricsConfig{
		Table: ddprofiledefinition.SymbolConfig{
			OID:  "1.3.6.1.2.1.2.2",
			Name: "ifTable",
		},
		Symbols: []ddprofiledefinition.SymbolConfig{
			{OID: "1.3.6.1.2.1.2.2.1.7", Name: "ifAdminStatus"},
			{OID: "1.3.6.1.2.1.2.2.1.8", Name: "ifOperStatus"},
		},
	}

	// Generate config IDs
	id1 := cache.generateConfigID(cfg1)
	id2 := cache.generateConfigID(cfg2)
	id3 := cache.generateConfigID(cfg3)

	// Same symbols (even in different order) should generate same ID
	assert.Equal(t, id1, id2)

	// Different symbols should generate different ID
	assert.NotEqual(t, id1, id3)

	// IDs should be human-readable
	assert.Equal(t, "ifTable,ifInOctets,ifOutOctets", id1)
	assert.Equal(t, "ifTable,ifAdminStatus,ifOperStatus", id3)
}
