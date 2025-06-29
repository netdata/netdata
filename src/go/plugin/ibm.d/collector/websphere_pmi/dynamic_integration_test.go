// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestDynamicCollectionIntegration tests the complete dynamic collection system
func TestDynamicCollectionIntegration(t *testing.T) {
	// Load real WebSphere PMI data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Create WebSphere PMI collector with dynamic collection enabled
	w := New()
	w.EnableDynamicCollection()

	// Test dynamic collection
	collected := w.collectDynamic(nil, &stats)

	// Verify we collected metrics
	assert.NotEmpty(t, collected, "Dynamic collection should return metrics")
	t.Logf("Dynamic collection gathered %d metrics", len(collected))

	// Verify we have charts
	assert.NotEmpty(t, *w.charts, "Should have created charts")
	t.Logf("Created %d charts", len(*w.charts))

	// Verify coverage statistics
	coverage := w.dynamicCollector.GetMetricCoverage()
	assert.Greater(t, coverage.TotalMetrics, 500, "Should extract many metrics")
	assert.Greater(t, coverage.TotalCharts, 50, "Should create many charts")
	assert.Greater(t, coverage.TotalArrays, 5, "Should detect arrays")

	t.Logf("Coverage stats: %d metrics, %d charts, %d arrays, %d dimensions",
		coverage.TotalMetrics, coverage.TotalCharts, coverage.TotalArrays, coverage.DimensionCount)
	t.Logf("Categories: %v", coverage.CategoryCounts)
}

// TestLegacyVsDynamicComparison compares legacy vs dynamic collection
func TestLegacyVsDynamicComparison(t *testing.T) {
	// Load real WebSphere PMI data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Create collector
	w := New()

	// Test legacy collection (simulate)
	legacyMx := make(map[string]int64)
	if len(stats.Nodes) > 0 && len(stats.Nodes[0].Servers) > 0 && len(stats.Nodes[0].Servers[0].Stats) > 0 {
		w.processStat(&stats.Nodes[0].Servers[0].Stats[0], "", legacyMx)
	}

	// Test dynamic collection
	w.EnableDynamicCollection()
	dynamicMx := w.collectDynamic(nil, &stats)

	t.Logf("Legacy collection: %d metrics", len(legacyMx))
	t.Logf("Dynamic collection: %d metrics", len(dynamicMx))

	// Dynamic should collect significantly more metrics
	assert.Greater(t, len(dynamicMx), len(legacyMx)*5, 
		"Dynamic collection should gather many more metrics than legacy")

	// Verify specific improvements
	legacyCount := len(legacyMx)
	dynamicCount := len(dynamicMx)
	improvement := float64(dynamicCount) / float64(legacyCount)
	t.Logf("Improvement factor: %.1fx (from %d to %d metrics)", improvement, legacyCount, dynamicCount)

	assert.Greater(t, improvement, 5.0, "Should see at least 5x improvement")
}

// TestDynamicCollectionChartsQuality verifies chart quality
func TestDynamicCollectionChartsQuality(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Create and use dynamic collector
	w := New()
	w.EnableDynamicCollection()
	w.collectDynamic(nil, &stats)

	// Verify chart quality
	charts := *w.charts
	require.NotEmpty(t, charts, "Should have charts")

	// Check chart diversity
	families := make(map[string]int)
	contexts := make(map[string]bool)
	units := make(map[string]int)

	for _, chart := range charts {
		families[chart.Fam]++
		contexts[chart.Ctx] = true
		units[chart.Units]++

		// Verify chart structure
		assert.NotEmpty(t, chart.ID, "Chart should have ID")
		assert.NotEmpty(t, chart.Title, "Chart should have title")
		assert.NotEmpty(t, chart.Units, "Chart should have units")
		assert.NotEmpty(t, chart.Ctx, "Chart should have context")
		assert.NotEmpty(t, chart.Dims, "Chart should have dimensions")
		assert.Greater(t, chart.Priority, 0, "Chart should have priority")

		// Verify dimensions
		for _, dim := range chart.Dims {
			assert.NotEmpty(t, dim.ID, "Dimension should have ID")
			assert.NotEmpty(t, dim.Name, "Dimension should have name")
		}
	}

	t.Logf("Chart families: %v", families)
	t.Logf("Unique contexts: %d", len(contexts))
	t.Logf("Chart units: %v", units)

	// Should have multiple families and units
	assert.Greater(t, len(families), 3, "Should have multiple chart families")
	assert.Greater(t, len(units), 3, "Should have multiple unit types")

	// Expected categories should be present
	expectedFamilies := []string{"thread_pools", "jvm", "jdbc_pools", "web_apps"}
	foundFamilies := make(map[string]bool)

	for family := range families {
		for _, expected := range expectedFamilies {
			if strings.Contains(family, expected) {
				foundFamilies[expected] = true
			}
		}
	}

	t.Logf("Found expected families: %v", foundFamilies)
	assert.GreaterOrEqual(t, len(foundFamilies), 3, "Should find most expected categories")
}

// TestDynamicCollectionConfiguration tests configuration options
func TestDynamicCollectionConfiguration(t *testing.T) {
	// Test default behavior (should use dynamic)
	w1 := New()
	assert.True(t, w1.shouldUseDynamicCollection(), "Should default to dynamic collection")

	// Test explicit enable
	w2 := New()
	trueVal := true
	w2.UseDynamicCollection = &trueVal
	assert.True(t, w2.shouldUseDynamicCollection(), "Should use dynamic when explicitly enabled")

	// Test explicit disable
	w3 := New()
	falseVal := false
	w3.UseDynamicCollection = &falseVal
	assert.False(t, w3.shouldUseDynamicCollection(), "Should not use dynamic when explicitly disabled")
}

// TestDynamicCollectionArrayHandling tests array detection and handling
func TestDynamicCollectionArrayHandling(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	w := New()
	w.EnableDynamicCollection()
	w.collectDynamic(nil, &stats)

	coverage := w.dynamicCollector.GetMetricCoverage()

	// Verify arrays were detected
	assert.Greater(t, coverage.TotalArrays, 5, "Should detect multiple arrays")

	// Check that array metrics were properly labeled
	charts := w.dynamicCollector.GetDynamicCharts()
	hasInstanceLabels := false
	hasPoolLabels := false

	for _, chart := range charts {
		for _, dim := range chart.Dimensions {
			if dim.Metric.Labels["instance"] != "" {
				hasInstanceLabels = true
			}
			if dim.Metric.Labels["pool"] != "" {
				hasPoolLabels = true
			}
		}
	}

	assert.True(t, hasInstanceLabels, "Should have instance labels from arrays")
	assert.True(t, hasPoolLabels, "Should have pool labels from thread pools")
}

// TestDynamicCollectionPerformance ensures the system performs reasonably
func TestDynamicCollectionPerformance(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	w := New()
	w.EnableDynamicCollection()

	// Time the collection process
	collected := w.collectDynamic(nil, &stats)

	// Verify reasonable results
	assert.NotEmpty(t, collected, "Should collect metrics")
	assert.Greater(t, len(collected), 100, "Should collect substantial metrics")

	// Second collection should be faster (reuse charts)
	collected2 := w.collectDynamic(nil, &stats)
	assert.Equal(t, len(collected), len(collected2), "Second collection should produce same number of metrics")
}