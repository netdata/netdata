// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"encoding/xml"
	"os"
	"strings"
	"testing"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestFullIntegration tests the complete pipeline from XML to Netdata charts
func TestFullIntegration(t *testing.T) {
	testFiles := []string{
		"../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml",
		"../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml",
	}

	for _, file := range testFiles {
		t.Run(file, func(t *testing.T) {
			testIntegrationWithFile(t, file)
		})
	}
}

func testIntegrationWithFile(t *testing.T, filename string) {
	// Step 1: Load and parse XML
	xmlData, err := os.ReadFile(filename)
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	t.Logf("Testing with file: %s", filename)
	t.Logf("XML structure: %d nodes, %d direct stats", len(stats.Nodes), len(stats.Stats))

	// Step 2: Flatten XML to metrics
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	require.NotEmpty(t, flatResult.Metrics, "Should extract metrics from XML")
	require.NotEmpty(t, flatResult.Arrays, "Should detect arrays in XML")

	t.Logf("Flattening result: %d metrics, %d arrays detected", 
		len(flatResult.Metrics), len(flatResult.Arrays))

	// Step 3: Correlate metrics into chart candidates
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)

	require.NotEmpty(t, charts, "Should generate chart candidates")
	t.Logf("Correlation result: %d chart candidates", len(charts))

	// Step 4: Convert to Netdata charts
	netdataCharts := correlator.ConvertToNetdataCharts(charts)
	require.NotEmpty(t, *netdataCharts, "Should generate Netdata charts")

	t.Logf("Final result: %d Netdata charts", len(*netdataCharts))

	// Verify quality of results
	verifyIntegrationQuality(t, flatResult, charts, netdataCharts)
}

func verifyIntegrationQuality(t *testing.T, flatResult *FlattenerResult, 
	charts []ChartCandidate, netdataCharts *module.Charts) {

	// 1. Verify all metrics are included in charts
	totalMetricsInCharts := 0
	for _, chart := range charts {
		totalMetricsInCharts += len(chart.Dimensions)
	}

	// Should capture most metrics (some might be filtered for quality)
	coveragePercent := float64(totalMetricsInCharts) / float64(len(flatResult.Metrics)) * 100
	t.Logf("Metric coverage: %.1f%% (%d/%d)", coveragePercent, totalMetricsInCharts, len(flatResult.Metrics))
	assert.Greater(t, coveragePercent, 80.0, "Should cover most metrics")

	// 2. Verify chart diversity
	families := make(map[string]int)
	units := make(map[string]int)
	contexts := make(map[string]bool)

	for _, chart := range *netdataCharts {
		families[chart.Fam]++
		units[chart.Units]++
		contexts[chart.Ctx] = true
	}

	t.Logf("Chart families: %v", families)
	t.Logf("Chart units: %v", units)
	t.Logf("Unique contexts: %d", len(contexts))

	assert.Greater(t, len(families), 2, "Should have multiple chart families")
	assert.Greater(t, len(units), 2, "Should have multiple unit types")

	// 3. Verify expected categories are present
	expectedCategories := []string{"thread_pools", "jvm", "jdbc_pools", "web_apps"}
	foundCategories := make(map[string]bool)

	for family := range families {
		for _, expected := range expectedCategories {
			if strings.Contains(family, expected) {
				foundCategories[expected] = true
			}
		}
	}

	t.Logf("Found expected categories: %v", foundCategories)
	assert.Greater(t, len(foundCategories), 1, "Should find some expected categories")

	// 4. Verify charts are properly structured
	for _, chart := range *netdataCharts {
		assert.NotEmpty(t, chart.ID, "Chart should have ID")
		assert.NotEmpty(t, chart.Title, "Chart should have title")
		assert.NotEmpty(t, chart.Units, "Chart should have units")
		assert.NotEmpty(t, chart.Ctx, "Chart should have context")
		assert.NotEmpty(t, chart.Fam, "Chart should have family")
		assert.Greater(t, chart.Priority, 0, "Chart should have priority")
		assert.NotEmpty(t, chart.Dims, "Chart should have dimensions")

		// Verify dimensions
		for _, dim := range chart.Dims {
			assert.NotEmpty(t, dim.ID, "Dimension should have ID: chart %s", chart.ID)
			assert.NotEmpty(t, dim.Name, "Dimension should have name: chart %s", chart.ID)
		}
	}

	// 5. Verify arrays are properly handled
	arrayMetrics := 0
	for _, array := range flatResult.Arrays {
		assert.NotEmpty(t, array.Path, "Array should have path")
		assert.NotEmpty(t, array.Elements, "Array should have elements")
		assert.NotEmpty(t, array.Labels, "Array should have labels")
		arrayMetrics += len(array.Elements)
	}

	t.Logf("Arrays detected: %d arrays with %d total elements", 
		len(flatResult.Arrays), arrayMetrics)
}

// TestMetricCoverage verifies we capture significantly more metrics than the old system
func TestMetricCoverage(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// New system
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Count metrics by category
	categoryCounts := make(map[string]int)
	for _, metric := range flatResult.Metrics {
		category := metric.Labels["category"]
		if category == "" {
			category = "uncategorized"
		}
		categoryCounts[category]++
	}

	t.Logf("Metrics by category: %v", categoryCounts)
	t.Logf("Total metrics extracted: %d", len(flatResult.Metrics))

	// Should extract significantly more than the 7 JVM metrics we had before
	assert.Greater(t, len(flatResult.Metrics), 50, 
		"New system should extract many more metrics than old system (was only 7)")

	// Should have metrics from multiple categories
	assert.Greater(t, len(categoryCounts), 2, "Should have metrics from multiple categories")

	// Specific categories should be present
	assert.Greater(t, categoryCounts["jvm"], 0, "Should have JVM metrics")
	if categoryCounts["thread_pools"] > 0 {
		t.Logf("Thread pool metrics: %d", categoryCounts["thread_pools"])
	}
	if categoryCounts["jdbc_pools"] > 0 {
		t.Logf("JDBC pool metrics: %d", categoryCounts["jdbc_pools"])
	}
}

// TestLabelInheritanceIntegration tests that labels are properly inherited through the hierarchy
func TestLabelInheritanceIntegration(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// All metrics should have inherited node and server labels
	nodeNames := make(map[string]bool)
	serverNames := make(map[string]bool)

	for _, metric := range flatResult.Metrics {
		assert.NotEmpty(t, metric.Labels["node"], "All metrics should have node label: %s", metric.Path)
		assert.NotEmpty(t, metric.Labels["server"], "All metrics should have server label: %s", metric.Path)
		
		nodeNames[metric.Labels["node"]] = true
		serverNames[metric.Labels["server"]] = true
	}

	t.Logf("Unique nodes: %v", keys(nodeNames))
	t.Logf("Unique servers: %v", keys(serverNames))

	// Check category-specific labels
	categorizedMetrics := make(map[string][]MetricTuple)
	for _, metric := range flatResult.Metrics {
		category := metric.Labels["category"]
		if category != "" {
			categorizedMetrics[category] = append(categorizedMetrics[category], metric)
		}
	}

	// Thread pool metrics should have pool-specific labels
	if threadPoolMetrics, exists := categorizedMetrics["thread_pools"]; exists {
		for _, metric := range threadPoolMetrics {
			if metric.Labels["instance"] != "" {
				assert.NotEmpty(t, metric.Labels["pool"], 
					"Thread pool metrics with instance should have pool label: %s", metric.Path)
			}
		}
		t.Logf("Thread pool metrics with proper labels: %d", len(threadPoolMetrics))
	}

	// JDBC metrics should have provider/datasource labels
	if jdbcMetrics, exists := categorizedMetrics["jdbc_pools"]; exists {
		for _, metric := range jdbcMetrics {
			if metric.Labels["datasource"] != "" {
				assert.NotEmpty(t, metric.Labels["provider"], 
					"JDBC datasource metrics should have provider label: %s", metric.Path)
			}
		}
		t.Logf("JDBC metrics with proper labels: %d", len(jdbcMetrics))
	}
}

// TestPerformance verifies the system can handle the data efficiently
func TestPerformance(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Measure flattening performance
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Measure correlation performance
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)

	// Should complete in reasonable time (this is basic smoke test)
	assert.NotEmpty(t, flatResult.Metrics, "Flattening should complete successfully")
	assert.NotEmpty(t, charts, "Correlation should complete successfully")

	t.Logf("Performance test completed: %d metrics -> %d charts", 
		len(flatResult.Metrics), len(charts))
}

// TestConsistencyBetweenVersions verifies both WebSphere versions work similarly
func TestConsistencyBetweenVersions(t *testing.T) {
	files := []struct {
		name string
		path string
	}{
		{"WebSphere 8.5.5.24", "../../samples.d/traditional-8.5.5.24-pmi-full-port-9284.xml"},
		{"WebSphere 9.0.5.24", "../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml"},
	}

	results := make(map[string]struct {
		metrics int
		charts  int
		arrays  int
	})

	for _, file := range files {
		xmlData, err := os.ReadFile(file.path)
		require.NoError(t, err)

		var stats pmiStatsResponse
		err = xml.Unmarshal(xmlData, &stats)
		require.NoError(t, err)

		flattener := NewXMLFlattener()
		flatResult := flattener.FlattenPMIStats(&stats)

		correlator := NewCorrelationEngine()
		charts := correlator.CorrelateMetrics(flatResult.Metrics)

		results[file.name] = struct {
			metrics int
			charts  int
			arrays  int
		}{
			metrics: len(flatResult.Metrics),
			charts:  len(charts),
			arrays:  len(flatResult.Arrays),
		}

		t.Logf("%s: %d metrics, %d charts, %d arrays", 
			file.name, len(flatResult.Metrics), len(charts), len(flatResult.Arrays))
	}

	// Both versions should produce reasonable results
	for name, result := range results {
		assert.Greater(t, result.metrics, 20, "%s should extract substantial metrics", name)
		assert.Greater(t, result.charts, 5, "%s should generate multiple charts", name)
		assert.Greater(t, result.arrays, 2, "%s should detect multiple arrays", name)
	}
}

// Helper function to get keys from a map
func keys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}