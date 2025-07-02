// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestArrayContextConsolidation(t *testing.T) {
	// Create a simple test with array elements
	testStats := &pmiStatsResponse{
		Nodes: []pmiNode{
			{
				Name: "TestNode",
				Servers: []pmiServer{
					{
						Name: "TestServer",
						Stats: []pmiStat{
							{
								Name: "Thread Pools",
								SubStats: []pmiStat{
									{
										Name: "WebContainer",
										CountStatistics: []countStat{
											{Name: "ActiveCount", Count: "10"},
											{Name: "CreateCount", Count: "100"},
										},
										BoundedRangeStatistics: []boundedRangeStat{
											{Name: "PoolSize", Current: "20"},
										},
									},
									{
										Name: "Default",
										CountStatistics: []countStat{
											{Name: "ActiveCount", Count: "5"},
											{Name: "CreateCount", Count: "50"},
										},
										BoundedRangeStatistics: []boundedRangeStat{
											{Name: "PoolSize", Current: "10"},
										},
									},
									{
										Name: "SIB JMS Resource Adapter",
										CountStatistics: []countStat{
											{Name: "ActiveCount", Count: "3"},
											{Name: "CreateCount", Count: "30"},
										},
										BoundedRangeStatistics: []boundedRangeStat{
											{Name: "PoolSize", Current: "8"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Process with flattener
	flattener := NewXMLFlattener()
	result := flattener.FlattenPMIStats(testStats)

	// Verify array detection
	assert.Len(t, result.Arrays, 1, "Should detect Thread Pools as array")
	assert.Equal(t, "Thread Pools", result.Arrays[0].Path)
	assert.Len(t, result.Arrays[0].Elements, 3, "Should have 3 thread pools")

	// Check that metrics are marked as array elements
	arrayMetrics := 0
	for _, metric := range result.Metrics {
		if metric.IsArrayElement {
			arrayMetrics++
			assert.Equal(t, "Thread Pools", metric.ArrayPath)
			assert.Contains(t, []string{"WebContainer", "Default", "SIB JMS Resource Adapter"}, metric.ElementName)
		}
	}
	assert.Equal(t, 9, arrayMetrics, "Should have 9 array metrics (3 pools × 3 metrics)")

	// Verify contexts are consolidated
	contextMap := make(map[string][]MetricTuple)
	for _, metric := range result.Metrics {
		contextMap[metric.UniqueContext] = append(contextMap[metric.UniqueContext], metric)
	}

	// Should have only 3 unique contexts for the array metrics
	expectedContexts := []string{
		"websphere_pmi.thread_pools.activecount",
		"websphere_pmi.thread_pools.createcount",
		"websphere_pmi.thread_pools.poolsize",
	}

	for _, expectedCtx := range expectedContexts {
		metrics, exists := contextMap[expectedCtx]
		assert.True(t, exists, "Should have context: %s", expectedCtx)
		assert.Len(t, metrics, 3, "Context %s should have 3 metrics (one per pool)", expectedCtx)
	}

	// Print context summary
	t.Log("Unique contexts created:")
	var contexts []string
	for ctx := range contextMap {
		contexts = append(contexts, ctx)
	}
	sort.Strings(contexts)
	for _, ctx := range contexts {
		t.Logf("  %s (%d metrics)", ctx, len(contextMap[ctx]))
	}
}

func TestRealXMLContextReduction(t *testing.T) {
	// Load real WebSphere PMI data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	require.NoError(t, err)

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	require.NoError(t, err)

	// Process with flattener
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Process with correlator
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)

	// Analyze contexts
	contextCount := make(map[string]int)
	contextInstances := make(map[string]map[string]bool)

	for _, chart := range charts {
		contextCount[chart.Context]++

		// Track unique instances per context
		if contextInstances[chart.Context] == nil {
			contextInstances[chart.Context] = make(map[string]bool)
		}

		// Extract instance from chart ID or labels
		for _, dim := range chart.Dimensions {
			if instance := dim.Metric.Labels["instance"]; instance != "" {
				contextInstances[chart.Context][instance] = true
			}
		}
	}

	// Print summary
	fmt.Printf("\n=== CONTEXT CONSOLIDATION RESULTS ===\n")
	fmt.Printf("Total metrics: %d\n", len(flatResult.Metrics))
	fmt.Printf("Total charts: %d\n", len(charts))
	fmt.Printf("Unique contexts: %d\n", len(contextCount))
	fmt.Printf("Arrays detected: %d\n\n", len(flatResult.Arrays))

	// Show arrays and their consolidated contexts
	fmt.Println("Array consolidation:")
	for _, array := range flatResult.Arrays {
		fmt.Printf("\nArray: %s (%d elements)\n", array.Path, len(array.Elements))

		// Find contexts for this array
		arrayContexts := make(map[string]bool)
		for _, metric := range flatResult.Metrics {
			if metric.IsArrayElement && metric.ArrayPath == array.Path {
				// Extract the metric name (last part after element name)
				parts := strings.Split(metric.Path, "/")
				if len(parts) > 0 {
					context := metric.UniqueContext
					arrayContexts[context] = true
				}
			}
		}

		fmt.Printf("  Consolidated to %d contexts:\n", len(arrayContexts))
		contexts := make([]string, 0, len(arrayContexts))
		for ctx := range arrayContexts {
			contexts = append(contexts, ctx)
		}
		sort.Strings(contexts)
		for _, ctx := range contexts {
			// Count instances
			instanceCount := 0
			for _, metric := range flatResult.Metrics {
				if metric.UniqueContext == ctx && metric.IsArrayElement && metric.ArrayPath == array.Path {
					instanceCount++
				}
			}
			fmt.Printf("    - %s (%d instances)\n", ctx, instanceCount)
		}
	}

	// Show reduction statistics
	fmt.Println("\n=== REDUCTION STATISTICS ===")

	// Count contexts by category
	categoryContexts := make(map[string]int)
	for ctx := range contextCount {
		parts := strings.Split(ctx, ".")
		if len(parts) >= 3 {
			category := parts[2]
			categoryContexts[category]++
		}
	}

	categories := make([]string, 0, len(categoryContexts))
	for cat := range categoryContexts {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		fmt.Printf("%s: %d contexts\n", cat, categoryContexts[cat])
	}

	// Verify significant reduction
	assert.Less(t, len(charts), 100, "Should have fewer than 100 charts (was 118)")
	assert.Less(t, len(contextCount), 60, "Should have fewer than 60 unique contexts")
}

func TestContextNaming(t *testing.T) {
	metric := MetricTuple{
		Path:           "Thread Pools/WebContainer/ActiveCount",
		IsArrayElement: true,
		ArrayPath:      "Thread Pools",
		ElementName:    "WebContainer",
		Type:           "count",
		Unit:           "requests",
	}

	flattener := NewXMLFlattener()
	context, instance := flattener.generateUniqueContextAndInstance(&metric)

	// Context should not include the element name
	assert.Equal(t, "websphere_pmi.thread_pools.activecount", context)
	assert.NotContains(t, context, "webcontainer", "Context should not contain instance name")

	// Instance should still be unique
	assert.Equal(t, "websphere_pmi.thread_pools.activecount.count.requests", instance)
}
