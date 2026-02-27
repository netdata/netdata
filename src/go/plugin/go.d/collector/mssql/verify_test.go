// SPDX-License-Identifier: GPL-3.0-or-later

//go:build integration

package mssql

import (
	"context"
	"os"
	"sort"
	"strings"
	"testing"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// getDSN returns the DSN from MSSQL_DSN environment variable.
// If not set, the test is skipped.
func getDSN(t *testing.T) string {
	dsn := os.Getenv("MSSQL_DSN")
	if dsn == "" {
		t.Skip("MSSQL_DSN environment variable not set")
	}
	return dsn
}

// TestIntegration_FullCollection runs a complete integration test against a real SQL Server.
// Run with: MSSQL_DSN="sqlserver://user:pass@host:port" go test -tags=integration -v -run TestIntegration
func TestIntegration_FullCollection(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	c.Timeout = confopt.Duration(time.Second * 10)

	// Initialize
	require.NoError(t, c.Init(context.Background()), "Init should succeed")

	// Check (first collection)
	require.NoError(t, c.Check(context.Background()), "Check should succeed")

	t.Logf("Connected to SQL Server version: %s", c.version)

	// Collect multiple times to verify stability
	for i := 1; i <= 3; i++ {
		t.Logf("\n=== Collection cycle %d ===", i)

		mx := c.Collect(context.Background())
		require.NotNil(t, mx, "Collect should return metrics")
		require.NotEmpty(t, mx, "Metrics should not be empty")

		// Verify charts were created
		charts := c.Charts()
		require.NotNil(t, charts)
		t.Logf("Charts count: %d", len(*charts))

		// Group and report metrics
		reportMetrics(t, mx)

		// Verify key metrics exist
		assertKeyMetrics(t, mx)

		time.Sleep(time.Second)
	}

	// Cleanup
	c.Cleanup(context.Background())
	assert.Nil(t, c.db, "DB connection should be closed after cleanup")
}

func reportMetrics(t *testing.T, mx map[string]int64) {
	// Group metrics by prefix
	groups := make(map[string][]string)
	for k := range mx {
		prefix := getMetricPrefix(k)
		groups[prefix] = append(groups[prefix], k)
	}

	// Sort and print
	var prefixes []string
	for p := range groups {
		prefixes = append(prefixes, p)
	}
	sort.Strings(prefixes)

	t.Logf("Total metrics: %d", len(mx))
	for _, prefix := range prefixes {
		keys := groups[prefix]
		sort.Strings(keys)
		t.Logf("  [%s]: %d metrics", prefix, len(keys))

		// Show a few sample values
		for i, k := range keys {
			if i >= 3 {
				t.Logf("    ... and %d more", len(keys)-3)
				break
			}
			t.Logf("    %s = %d", k, mx[k])
		}
	}
}

func getMetricPrefix(key string) string {
	parts := strings.Split(key, "_")
	if len(parts) >= 2 {
		// Handle special cases
		if parts[0] == "database" {
			return "database"
		}
		if parts[0] == "wait" {
			return "wait"
		}
		if parts[0] == "locks" {
			return "locks"
		}
		if parts[0] == "job" {
			return "job"
		}
		return parts[0]
	}
	return key
}

func assertKeyMetrics(t *testing.T, mx map[string]int64) {
	// Instance metrics
	requiredMetrics := []string{
		"batch_requests",
		"sql_compilations",
		"sql_recompilations",
	}

	// Optional metrics (may not exist depending on config)
	optionalMetrics := []string{
		"user_connections",
		"blocked_processes",
		"buffer_cache_hit_ratio",
		"buffer_page_life_expectancy",
		"buffer_page_reads",
		"buffer_page_writes",
		"memory_total",
		"page_splits",
	}

	for _, m := range requiredMetrics {
		_, exists := mx[m]
		assert.True(t, exists, "Required metric %s should exist", m)
	}

	foundOptional := 0
	for _, m := range optionalMetrics {
		if _, exists := mx[m]; exists {
			foundOptional++
		}
	}
	t.Logf("Found %d/%d optional instance metrics", foundOptional, len(optionalMetrics))

	// Check for database metrics (should have at least system databases)
	dbMetrics := 0
	for k := range mx {
		if strings.HasPrefix(k, "database_") {
			dbMetrics++
		}
	}
	assert.Greater(t, dbMetrics, 0, "Should have database metrics")
	t.Logf("Found %d database metrics", dbMetrics)

	// Check for wait metrics
	waitMetrics := 0
	for k := range mx {
		if strings.HasPrefix(k, "wait_") {
			waitMetrics++
		}
	}
	assert.Greater(t, waitMetrics, 0, "Should have wait metrics")
	t.Logf("Found %d wait metrics", waitMetrics)
}

// TestIntegration_ChartsCreation verifies dynamic chart creation
func TestIntegration_ChartsCreation(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	c.Timeout = confopt.Duration(time.Second * 10)

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	// First collection creates charts
	mx1 := c.Collect(context.Background())
	require.NotNil(t, mx1)

	charts := c.Charts()
	initialChartCount := len(*charts)
	t.Logf("Charts after first collection: %d", initialChartCount)

	// List chart IDs
	var chartIDs []string
	for _, ch := range *charts {
		chartIDs = append(chartIDs, ch.ID)
	}
	sort.Strings(chartIDs)

	t.Log("Chart IDs:")
	for _, id := range chartIDs {
		t.Logf("  - %s", id)
	}

	// Verify we have expected chart categories
	hasInstance := false
	hasDatabase := false
	hasWait := false

	for _, id := range chartIDs {
		if strings.Contains(id, "user_connections") || strings.Contains(id, "batch_requests") {
			hasInstance = true
		}
		if strings.Contains(id, "database_") && strings.Contains(id, "_transactions") {
			hasDatabase = true
		}
		if strings.Contains(id, "wait_") {
			hasWait = true
		}
	}

	assert.True(t, hasInstance, "Should have instance charts")
	assert.True(t, hasDatabase, "Should have database charts")
	assert.True(t, hasWait, "Should have wait charts")

	c.Cleanup(context.Background())
}

// TestIntegration_MetricValues verifies metric values are sensible
func TestIntegration_MetricValues(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	c.Timeout = confopt.Duration(time.Second * 10)

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	mx := c.Collect(context.Background())
	require.NotNil(t, mx)

	// Buffer cache hit ratio should be 0-100
	if v, ok := mx["buffer_cache_hit_ratio"]; ok {
		assert.GreaterOrEqual(t, v, int64(0), "Cache hit ratio >= 0")
		assert.LessOrEqual(t, v, int64(100), "Cache hit ratio <= 100")
		t.Logf("Buffer cache hit ratio: %d%%", v)
	}

	// Page life expectancy should be positive
	if v, ok := mx["buffer_page_life_expectancy"]; ok {
		assert.GreaterOrEqual(t, v, int64(0), "PLE should be >= 0")
		t.Logf("Page life expectancy: %d seconds", v)
	}

	// User connections should be at least 1 (our connection)
	if v, ok := mx["user_connections"]; ok {
		assert.GreaterOrEqual(t, v, int64(1), "Should have at least 1 connection")
		t.Logf("User connections: %d", v)
	}

	// Memory should be positive
	if v, ok := mx["memory_total"]; ok {
		assert.Greater(t, v, int64(0), "Memory should be > 0")
		t.Logf("Total memory: %d bytes (%.2f MB)", v, float64(v)/1024/1024)
	}

	// Blocked processes should be non-negative
	if v, ok := mx["blocked_processes"]; ok {
		assert.GreaterOrEqual(t, v, int64(0), "Blocked processes >= 0")
		t.Logf("Blocked processes: %d", v)
	}

	c.Cleanup(context.Background())
}

// TestIntegration_ErrorHandling verifies graceful error handling
func TestIntegration_ErrorHandling(t *testing.T) {
	// Test with invalid DSN - doesn't need a real server
	c := New()
	c.DSN = "sqlserver://invalid:invalid@localhost:9999?connection+timeout=2"
	c.Timeout = confopt.Duration(time.Second * 3)

	require.NoError(t, c.Init(context.Background()), "Init should succeed (just validates config)")

	// Check should fail with connection error
	err := c.Check(context.Background())
	assert.Error(t, err, "Check should fail with invalid connection")
	t.Logf("Expected error: %v", err)
}

// TestIntegration_Databases lists all discovered databases
func TestIntegration_Databases(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	c.Timeout = confopt.Duration(time.Second * 10)

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	_ = c.Collect(context.Background())

	t.Log("Discovered databases:")
	for db := range c.seenDatabases {
		t.Logf("  - %s", db)
	}

	// Should have system databases
	assert.True(t, c.seenDatabases["master"], "Should see master database")
	assert.True(t, c.seenDatabases["tempdb"], "Should see tempdb database")
	assert.True(t, c.seenDatabases["msdb"], "Should see msdb database")
	assert.True(t, c.seenDatabases["model"], "Should see model database")

	c.Cleanup(context.Background())
}

// TestIntegration_WaitTypes lists all discovered wait types
func TestIntegration_WaitTypes(t *testing.T) {
	c := New()
	c.DSN = getDSN(t)
	c.Timeout = confopt.Duration(time.Second * 10)

	require.NoError(t, c.Init(context.Background()))
	require.NoError(t, c.Check(context.Background()))

	_ = c.Collect(context.Background())

	t.Logf("Discovered wait types: %d", len(c.seenWaitTypes))

	// Group by category
	byCategory := make(map[string][]string)
	for wt := range c.seenWaitTypes {
		cat := getWaitCategory(wt)
		byCategory[cat] = append(byCategory[cat], wt)
	}

	for cat, types := range byCategory {
		sort.Strings(types)
		t.Logf("  [%s]: %d types", cat, len(types))
		for i, wt := range types {
			if i >= 5 {
				t.Logf("    ... and %d more", len(types)-5)
				break
			}
			t.Logf("    - %s", wt)
		}
	}

	c.Cleanup(context.Background())
}
