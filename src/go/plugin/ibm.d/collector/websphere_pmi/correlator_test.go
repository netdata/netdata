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

func TestCorrelationEngine_CorrelateMetrics(t *testing.T) {
	// Load and flatten real data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Correlate metrics into charts
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)

	assert.NotEmpty(t, charts, "Should generate chart candidates")
	t.Logf("Generated %d chart candidates", len(charts))

	// Verify chart structure
	for _, chart := range charts {
		assert.NotEmpty(t, chart.Context, "Chart should have context")
		assert.NotEmpty(t, chart.Title, "Chart should have title")
		assert.NotEmpty(t, chart.Units, "Chart should have units")
		assert.NotEmpty(t, chart.Type, "Chart should have type")
		assert.NotEmpty(t, chart.Family, "Chart should have family")
		assert.Greater(t, chart.Priority, 0, "Chart should have priority")
		assert.NotEmpty(t, chart.Dimensions, "Chart should have dimensions")

		// Verify all dimensions have same unit
		if len(chart.Dimensions) > 1 {
			firstUnit := chart.Dimensions[0].Metric.Unit
			for _, dim := range chart.Dimensions {
				assert.Equal(t, firstUnit, dim.Metric.Unit, 
					"All dimensions in chart should have same unit: %s", chart.Context)
			}
		}

		t.Logf("Chart: %s (%s) - %d dimensions, unit: %s", 
			chart.Context, chart.Title, len(chart.Dimensions), chart.Units)
	}
}

func TestCorrelationEngine_GroupingLogic(t *testing.T) {
	correlator := NewCorrelationEngine()

	// Create test metrics that should be grouped together
	threadPoolMetrics := []MetricTuple{
		{
			Path:   "server/Thread Pools/WebContainer/ActiveCount",
			Labels: map[string]string{"node": "node1", "server": "server1", "category": "thread_pools", "pool": "WebContainer"},
			Value:  5, Unit: "requests", Type: "count",
		},
		{
			Path:   "server/Thread Pools/Default/ActiveCount",
			Labels: map[string]string{"node": "node1", "server": "server1", "category": "thread_pools", "pool": "Default"},
			Value:  3, Unit: "requests", Type: "count",
		},
		{
			Path:   "server/Thread Pools/WebContainer/PoolSize",
			Labels: map[string]string{"node": "node1", "server": "server1", "category": "thread_pools", "pool": "WebContainer"},
			Value:  20, Unit: "requests", Type: "bounded_range",
		},
	}

	charts := correlator.CorrelateMetrics(threadPoolMetrics)

	// Should group by correlation key
	assert.NotEmpty(t, charts, "Should generate charts for thread pool metrics")

	// Find charts with active count metrics
	activeCountCharts := 0
	totalActiveCountDimensions := 0
	for _, chart := range charts {
		if strings.Contains(chart.Context, "active") {
			activeCountCharts++
			totalActiveCountDimensions += len(chart.Dimensions)
			assert.Equal(t, "requests", chart.Units, "Should use consistent units")
		}
	}

	// Should group similar metrics (might be in one or more charts depending on correlation logic)
	assert.Greater(t, activeCountCharts, 0, "Should have at least one active count chart")
	assert.GreaterOrEqual(t, totalActiveCountDimensions, 2, "Should include both pool metrics")
}

func TestCorrelationEngine_CorrelationKeys(t *testing.T) {
	correlator := NewCorrelationEngine()

	testCases := []struct {
		metric   MetricTuple
		expected string
		description string
	}{
		{
			metric: MetricTuple{
				Path:   "server/Thread Pools/WebContainer/ActiveCount",
				Labels: map[string]string{"category": "thread_pools"},
			},
			expected:    "websphere_pmi.thread_pools.active_connections",
			description: "Thread pool active count should group by active connections",
		},
		{
			metric: MetricTuple{
				Path:   "server/JVM Runtime/HeapSize",
				Labels: map[string]string{"category": "jvm"},
			},
			expected:    "websphere_pmi.jvm.memory",
			description: "JVM heap metrics should group by memory",
		},
		{
			metric: MetricTuple{
				Path:   "server/JDBC Connection Pools/Provider1/DataSource1/CreateCount",
				Labels: map[string]string{"category": "jdbc_pools"},
			},
			expected:    "websphere_pmi.jdbc_pools.connection_lifecycle",
			description: "JDBC create count should group by connection lifecycle",
		},
	}

	for _, tc := range testCases {
		key := correlator.generateCorrelationKey(tc.metric)
		assert.Equal(t, tc.expected, key, tc.description)
	}
}

func TestCorrelationEngine_MetricCompatibility(t *testing.T) {
	correlator := NewCorrelationEngine()

	// Compatible metrics (same unit, similar semantic)
	metric1 := MetricTuple{
		Path: "server/Thread Pools/WebContainer/ActiveCount",
		Labels: map[string]string{"pool": "WebContainer"},
		Unit: "requests", Type: "count",
	}
	metric2 := MetricTuple{
		Path: "server/Thread Pools/Default/ActiveCount",
		Labels: map[string]string{"pool": "Default"},
		Unit: "requests", Type: "count",
	}

	assert.True(t, correlator.areMetricsCompatible(metric1, metric2), 
		"Metrics with same unit and type should be compatible")

	// Incompatible metrics (different units)
	metric3 := MetricTuple{
		Path: "server/JVM Runtime/HeapSize",
		Unit: "bytes", Type: "bounded_range",
	}

	assert.False(t, correlator.areMetricsCompatible(metric1, metric3),
		"Metrics with different units should not be compatible")

	// Semi-compatible (count and bounded_range with same unit)
	metric4 := MetricTuple{
		Path: "server/Thread Pools/WebContainer/PoolSize",
		Unit: "requests", Type: "bounded_range",
	}

	assert.True(t, correlator.areMetricsCompatible(metric1, metric4),
		"Count and bounded_range with same unit should be compatible")
}

func TestCorrelationEngine_ChartGeneration(t *testing.T) {
	correlator := NewCorrelationEngine()

	// Test chart generation from metric group
	metrics := []MetricTuple{
		{
			Path:   "server/Thread Pools/WebContainer/ActiveCount",
			Labels: map[string]string{"node": "node1", "server": "server1", "pool": "WebContainer", "instance": "WebContainer"},
			Value:  5, Unit: "requests", Type: "count",
		},
		{
			Path:   "server/Thread Pools/Default/ActiveCount",
			Labels: map[string]string{"node": "node1", "server": "server1", "pool": "Default", "instance": "Default"},
			Value:  3, Unit: "requests", Type: "count",
		},
	}

	group := MetricGroup{
		CorrelationKey: "websphere_pmi.thread_pools.active_connections",
		BaseContext:    "websphere_pmi.thread_pools.active_connections",
		CommonLabels:   map[string]string{"node": "node1", "server": "server1"},
		Metrics:        metrics,
		Unit:          "requests",
		Type:          "line",
	}

	chart := correlator.createChartFromGroup(group, 70000)

	assert.Equal(t, "websphere_pmi.thread_pools.active_connections", chart.Context)
	assert.Equal(t, "Thread Pools Active Connections", chart.Title)
	assert.Equal(t, "requests", chart.Units)
	assert.Equal(t, "line", chart.Type)
	assert.Equal(t, "thread_pools", chart.Family)
	assert.Equal(t, 70000, chart.Priority)
	assert.Len(t, chart.Dimensions, 2)

	// Check dimensions
	dimNames := make([]string, len(chart.Dimensions))
	for i, dim := range chart.Dimensions {
		dimNames[i] = dim.Name
		assert.NotEmpty(t, dim.ID, "Dimension should have ID")
		assert.NotEmpty(t, dim.Name, "Dimension should have name")
	}

	assert.Contains(t, dimNames, "WebContainer", "Should have WebContainer dimension")
	assert.Contains(t, dimNames, "Default", "Should have Default dimension")
}

func TestCorrelationEngine_LargeGroupSplitting(t *testing.T) {
	correlator := NewCorrelationEngine()
	correlator.maxDimensionsPerChart = 3 // Set low limit for testing

	// Create a large group
	metrics := make([]MetricTuple, 5)
	for i := 0; i < 5; i++ {
		metrics[i] = MetricTuple{
			Path:   "server/Thread Pools/Pool" + string(rune('A'+i)) + "/ActiveCount",
			Labels: map[string]string{"instance": "Pool" + string(rune('A'+i))},
			Unit:   "requests",
			Type:   "count",
		}
	}

	group := &MetricGroup{
		CorrelationKey: "test.large.group",
		BaseContext:    "test.large.group",
		Metrics:        metrics,
		Unit:          "requests",
		Type:          "line",
	}

	subGroups := correlator.splitLargeGroup(group)
	assert.Greater(t, len(subGroups), 1, "Large group should be split")

	totalMetrics := 0
	for _, subGroup := range subGroups {
		totalMetrics += len(subGroup.Metrics)
		assert.LessOrEqual(t, len(subGroup.Metrics), correlator.maxDimensionsPerChart,
			"Split groups should respect dimension limit")
	}

	assert.Equal(t, len(metrics), totalMetrics, "All metrics should be preserved after splitting")
}

func TestCorrelationEngine_NetdataChartsConversion(t *testing.T) {
	correlator := NewCorrelationEngine()

	candidates := []ChartCandidate{
		{
			Context:  "websphere_pmi.thread_pools.active_connections",
			Title:    "Thread Pool Active Connections",
			Units:    "requests",
			Type:     "line",
			Family:   "thread_pools",
			Priority: 70000,
			Dimensions: []DimensionCandidate{
				{ID: "webcontainer_active", Name: "WebContainer"},
				{ID: "default_active", Name: "Default"},
			},
		},
	}

	charts := correlator.ConvertToNetdataCharts(candidates)

	assert.Len(t, *charts, 1, "Should convert all candidates")

	chart := (*charts)[0]
	assert.Equal(t, "websphere_pmi_thread_pools_active_connections", chart.ID)
	assert.Equal(t, "Thread Pool Active Connections", chart.Title)
	assert.Equal(t, "requests", chart.Units)
	assert.Equal(t, "thread_pools", chart.Fam)
	assert.Equal(t, "websphere_pmi.thread_pools.active_connections", chart.Ctx)
	assert.Len(t, chart.Dims, 2)

	dimIDs := make([]string, len(chart.Dims))
	dimNames := make([]string, len(chart.Dims))
	for i, dim := range chart.Dims {
		dimIDs[i] = dim.ID
		dimNames[i] = dim.Name
	}

	assert.Contains(t, dimIDs, "webcontainer_active")
	assert.Contains(t, dimIDs, "default_active")
	assert.Contains(t, dimNames, "WebContainer")
	assert.Contains(t, dimNames, "Default")
}

func TestCorrelationEngine_RealDataIntegration(t *testing.T) {
	// Load real WebSphere data and test end-to-end processing
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Flatten data
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Correlate into charts
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)

	// Convert to Netdata format
	netdataCharts := correlator.ConvertToNetdataCharts(charts)

	assert.NotEmpty(t, *netdataCharts, "Should generate Netdata charts from real data")
	t.Logf("Generated %d Netdata charts from %d metrics", len(*netdataCharts), len(flatResult.Metrics))

	// Verify chart diversity (should have different categories)
	families := make(map[string]int)
	contexts := make(map[string]bool)
	
	for _, chart := range *netdataCharts {
		families[chart.Fam]++
		contexts[chart.Ctx] = true
		
		// Verify chart structure
		assert.NotEmpty(t, chart.ID, "Chart should have ID")
		assert.NotEmpty(t, chart.Title, "Chart should have title")
		assert.NotEmpty(t, chart.Units, "Chart should have units")
		assert.NotEmpty(t, chart.Ctx, "Chart should have context")
		assert.NotEmpty(t, chart.Dims, "Chart should have dimensions")
		
		// Each dimension should have ID and name
		for _, dim := range chart.Dims {
			assert.NotEmpty(t, dim.ID, "Dimension should have ID")
			assert.NotEmpty(t, dim.Name, "Dimension should have name")
		}
	}

	t.Logf("Chart families: %v", families)
	t.Logf("Unique contexts: %d", len(contexts))

	// Should have multiple families (categories), indicating good grouping
	assert.Greater(t, len(families), 1, "Should generate charts for multiple categories")

	// Should have reasonable number of charts (not too many, not too few)
	assert.Greater(t, len(*netdataCharts), 5, "Should generate reasonable number of charts")
	assert.Less(t, len(*netdataCharts), 150, "Should not generate excessive charts")
}

func TestCorrelationEngine_MetricFamilyExtraction(t *testing.T) {
	correlator := NewCorrelationEngine()

	testCases := []struct {
		metricName string
		expected   string
		description string
	}{
		{"ActiveCount", "active_connections", "Active count should be active connections"},
		{"CreateCount", "connection_lifecycle", "Create count should be connection lifecycle"},
		{"WaitTime", "wait_times", "Wait time should be wait times"},
		{"ResponseTime", "response_times", "Response time should be response times"},
		{"HeapSize", "memory", "Heap size should be memory"},
		{"ErrorCount", "errors", "Error count should be errors"},
		{"CpuUtilization", "utilization", "CPU utilization should be utilization"},
		{"SomeCustomMetric", "somecustommetric", "Unknown metrics should use normalized name"},
	}

	for _, tc := range testCases {
		result := correlator.extractMetricFamily(tc.metricName)
		assert.Equal(t, tc.expected, result, tc.description)
	}
}