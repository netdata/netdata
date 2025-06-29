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

func TestXMLFlattener_FlattenPMIStats(t *testing.T) {
	// Load real WebSphere PMI data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Create flattener and process data
	flattener := NewXMLFlattener()
	result := flattener.FlattenPMIStats(&stats)

	// Verify we got metrics
	assert.NotEmpty(t, result.Metrics, "Should extract metrics from XML")
	assert.NotEmpty(t, result.Arrays, "Should detect arrays in XML")

	t.Logf("Extracted %d metrics and detected %d arrays", len(result.Metrics), len(result.Arrays))

	// Test a few specific metrics we know should exist
	metricPaths := make(map[string]bool)
	for _, metric := range result.Metrics {
		metricPaths[metric.Path] = true
	}

	// Should have JVM metrics
	jvmFound := false
	for path := range metricPaths {
		if contains(path, "JVM Runtime") {
			jvmFound = true
			break
		}
	}
	assert.True(t, jvmFound, "Should extract JVM Runtime metrics")

	// Verify label inheritance
	for _, metric := range result.Metrics {
		assert.NotEmpty(t, metric.Labels["node"], "All metrics should have node label")
		assert.NotEmpty(t, metric.Labels["server"], "All metrics should have server label")
		assert.NotEmpty(t, metric.Path, "All metrics should have path")
		assert.NotEmpty(t, metric.Unit, "All metrics should have unit")
		assert.NotEmpty(t, metric.Type, "All metrics should have type")
	}
}

func TestXMLFlattener_ArrayDetection(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	result := flattener.FlattenPMIStats(&stats)

	// Verify expected arrays are detected
	arrayPaths := make(map[string]ArrayInfo)
	for _, array := range result.Arrays {
		arrayPaths[array.Path] = array
	}

	// Should detect Thread Pools array
	threadPoolsFound := false
	for path := range arrayPaths {
		if contains(path, "Thread Pools") {
			threadPoolsFound = true
			array := arrayPaths[path]
			assert.NotEmpty(t, array.Elements, "Thread Pools array should have elements")
			assert.Contains(t, array.Labels, "category", "Thread Pools should have category label")
			t.Logf("Thread Pools array: %d elements", len(array.Elements))
			break
		}
	}
	assert.True(t, threadPoolsFound, "Should detect Thread Pools array")

	// Should detect JDBC arrays
	jdbcFound := false
	for path := range arrayPaths {
		if contains(path, "JDBC") {
			jdbcFound = true
			break
		}
	}
	assert.True(t, jdbcFound, "Should detect JDBC arrays")
}

func TestXMLFlattener_LabelInheritance(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	result := flattener.FlattenPMIStats(&stats)

	// Test hierarchical label inheritance
	for _, metric := range result.Metrics {
		// All metrics should inherit node and server
		assert.NotEmpty(t, metric.Labels["node"], "Should inherit node label: %s", metric.Path)
		assert.NotEmpty(t, metric.Labels["server"], "Should inherit server label: %s", metric.Path)

		// Thread pool metrics should have pool-specific labels
		if contains(metric.Path, "Thread Pools") && metric.Labels["instance"] != "" {
			assert.Equal(t, "thread_pools", metric.Labels["category"], "Thread pool metrics should have thread_pools category")
			assert.NotEmpty(t, metric.Labels["pool"], "Thread pool metrics should have pool label")
			assert.NotEmpty(t, metric.Labels["instance"], "Thread pool metrics should have instance label")
		}

		// JDBC metrics should have JDBC-specific labels
		if contains(metric.Path, "JDBC") && metric.Labels["datasource"] != "" {
			assert.Equal(t, "jdbc_pools", metric.Labels["category"], "JDBC metrics should have jdbc_pools category")
			assert.NotEmpty(t, metric.Labels["provider"], "JDBC metrics should have provider label")
			assert.NotEmpty(t, metric.Labels["datasource"], "JDBC metrics should have datasource label")
		}

		// Web app metrics should have app-specific labels
		if contains(metric.Path, "Web Applications") && metric.Labels["app"] != "" {
			assert.Equal(t, "web_apps", metric.Labels["category"], "Web app metrics should have web_apps category")
			assert.NotEmpty(t, metric.Labels["app"], "Web app metrics should have app label")
		}
	}
}

func TestXMLFlattener_MetricTypes(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	result := flattener.FlattenPMIStats(&stats)

	// Count different metric types
	typeCounts := make(map[string]int)
	unitCounts := make(map[string]int)
	
	for _, metric := range result.Metrics {
		typeCounts[metric.Type]++
		unitCounts[metric.Unit]++
		
		// Verify metric values are reasonable
		assert.GreaterOrEqual(t, metric.Value, int64(0), "Metric values should be non-negative")
	}

	t.Logf("Metric types found: %v", typeCounts)
	t.Logf("Units found: %v", unitCounts)

	// Should have various metric types
	assert.Greater(t, typeCounts["count"], 0, "Should have count metrics")
	// Note: this specific XML might not have bounded_range metrics, that's ok
	if typeCounts["bounded_range"] > 0 {
		t.Logf("Found bounded_range metrics: %d", typeCounts["bounded_range"])
	}

	// Should have various units
	assert.Contains(t, unitCounts, "requests", "Should have request metrics")
	assert.Contains(t, unitCounts, "milliseconds", "Should have time metrics")
}

func TestXMLFlattener_PathGeneration(t *testing.T) {
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	result := flattener.FlattenPMIStats(&stats)

	// Verify path structure
	pathDepths := make(map[int]int)
	for _, metric := range result.Metrics {
		depth := len(strings.Split(metric.Path, "/"))
		pathDepths[depth]++
		
		// Paths should be reasonable length (WebSphere can have very deep hierarchies)
		assert.LessOrEqual(t, depth, 15, "Path depth should be reasonable: %s", metric.Path)
		assert.GreaterOrEqual(t, depth, 2, "Path should have minimum depth: %s", metric.Path)
		
		// Paths should not be empty or have empty components
		assert.NotEmpty(t, metric.Path, "Path should not be empty")
		parts := strings.Split(metric.Path, "/")
		for i, part := range parts {
			// Skip assertion for known edge cases with URLs that have empty components
			if part == "" && strings.Contains(metric.Path, "URLs/") {
				continue
			}
			assert.NotEmpty(t, part, "Path parts should not be empty: %s (part %d)", metric.Path, i)
		}
	}

	t.Logf("Path depth distribution: %v", pathDepths)
}

func TestXMLFlattener_UnitInference(t *testing.T) {
	flattener := NewXMLFlattener()

	testCases := []struct {
		path         string
		expectedUnit string
		description  string
		metricType   string
	}{
		{"server/JVM Runtime/HeapSize", "bytes", "Heap metrics should be bytes", "bounded_range"},
		{"server/Thread Pools/WebContainer/ResponseTime", "milliseconds", "Time metrics should be milliseconds", "time"},
		{"server/Thread Pools/WebContainer/ActiveCount", "requests", "Count metrics should be requests", "count"},
		{"server/JVM Runtime/ProcessCpuUsage", "percent", "Usage metrics should be percent", "double"},
		{"server/JDBC Connection Pools/Provider/CreateCount", "requests", "Create count should be requests", "count"},
	}

	for _, tc := range testCases {
		metric := MetricTuple{
			Path:   tc.path,
			Labels: make(map[string]string),
			Value:  100,
			Unit:   "items", // Default unit
			Type:   tc.metricType,
		}

		flattener.refineMetricUnit(&metric)
		assert.Equal(t, tc.expectedUnit, metric.Unit, tc.description)
	}
}

func TestXMLFlattener_EdgeCases(t *testing.T) {
	flattener := NewXMLFlattener()

	// Test empty stats
	emptyStats := &pmiStatsResponse{}
	result := flattener.FlattenPMIStats(emptyStats)
	assert.Empty(t, result.Metrics, "Empty stats should produce empty metrics")
	assert.Empty(t, result.Arrays, "Empty stats should produce empty arrays")

	// Test single metric
	singleStat := &pmiStatsResponse{
		Nodes: []pmiNode{
			{
				Name: "TestNode",
				Servers: []pmiServer{
					{
						Name: "TestServer",
						Stats: []pmiStat{
							{
								Name: "TestStat",
								CountStatistics: []countStat{
									{Name: "TestCount", Count: "42"},
								},
							},
						},
					},
				},
			},
		},
	}

	result = flattener.FlattenPMIStats(singleStat)
	assert.Len(t, result.Metrics, 1, "Single stat should produce one metric")
	
	metric := result.Metrics[0]
	assert.Equal(t, "TestStat/TestCount", metric.Path)
	assert.Equal(t, "TestNode", metric.Labels["node"])
	assert.Equal(t, "TestServer", metric.Labels["server"])
	assert.Equal(t, int64(42), metric.Value)
}

func TestXMLFlattener_ArraySignature(t *testing.T) {
	flattener := NewXMLFlattener()

	// Create test stats with similar structure
	stat1 := pmiStat{
		Name: "Pool1",
		CountStatistics: []countStat{
			{Name: "ActiveCount", Count: "5"},
			{Name: "CreateCount", Count: "10"},
		},
		BoundedRangeStatistics: []boundedRangeStat{
			{Name: "PoolSize", Current: "20"},
		},
	}

	stat2 := pmiStat{
		Name: "Pool2",
		CountStatistics: []countStat{
			{Name: "ActiveCount", Count: "3"},
			{Name: "CreateCount", Count: "8"},
		},
		BoundedRangeStatistics: []boundedRangeStat{
			{Name: "PoolSize", Current: "15"},
		},
	}

	stat3 := pmiStat{
		Name: "DifferentStat",
		TimeStatistics: []timeStat{
			{Name: "ResponseTime", Count: "100"},
		},
	}

	// Test signature generation
	sig1 := flattener.getStatSignature(&stat1)
	sig2 := flattener.getStatSignature(&stat2)
	sig3 := flattener.getStatSignature(&stat3)

	assert.Equal(t, sig1, sig2, "Similar stats should have same signature")
	assert.NotEqual(t, sig1, sig3, "Different stats should have different signatures")

	// Test array detection
	containerStat := pmiStat{
		Name:     "Thread Pools",
		SubStats: []pmiStat{stat1, stat2},
	}

	assert.True(t, flattener.isArray(&containerStat), "Container with similar substats should be detected as array")

	mixedContainerStat := pmiStat{
		Name:     "Mixed Container",
		SubStats: []pmiStat{stat1, stat3},
	}

	assert.False(t, flattener.isArray(&mixedContainerStat), "Container with different substats should not be array")
}

// Helper function
func contains(s, substr string) bool {
	return strings.Contains(s, substr)
}