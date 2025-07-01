package websphere_pmi

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestContextProliferation(t *testing.T) {
	// Load real WebSphere PMI data
	xmlData, err := os.ReadFile("../../samples.d/traditional-9.0.5.x-pmi-full-port-9083.xml")
	if err != nil {
		t.Fatal(err)
	}

	var stats pmiStatsResponse
	err = xml.Unmarshal(xmlData, &stats)
	if err != nil {
		t.Fatal(err)
	}

	// Process with flattener
	flattener := NewXMLFlattener()
	flatResult := flattener.FlattenPMIStats(&stats)

	// Process with correlator
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)

	// Analyze contexts
	contextCount := make(map[string]int)
	categoryContexts := make(map[string][]string)
	
	for _, chart := range charts {
		contextCount[chart.Context]++
		
		// Extract category from context
		parts := strings.Split(chart.Context, ".")
		if len(parts) >= 3 {
			category := parts[2] // Skip "websphere_pmi.server"
			categoryContexts[category] = append(categoryContexts[category], chart.Context)
		}
	}

	fmt.Printf("Total charts created: %d\n", len(charts))
	fmt.Printf("Unique contexts: %d\n\n", len(contextCount))

	// Show categories with multiple contexts
	fmt.Println("Categories with context proliferation:")
	categories := make([]string, 0, len(categoryContexts))
	for cat := range categoryContexts {
		categories = append(categories, cat)
	}
	sort.Strings(categories)

	for _, cat := range categories {
		contexts := categoryContexts[cat]
		if len(contexts) > 5 { // Show categories with significant proliferation
			fmt.Printf("\n%s: %d contexts\n", cat, len(contexts))
			
			// Show first few examples
			sort.Strings(contexts)
			for i, ctx := range contexts {
				if i >= 5 {
					fmt.Printf("  ... and %d more\n", len(contexts)-5)
					break
				}
				fmt.Printf("  - %s\n", ctx)
			}
		}
	}

	// Analyze specific array types
	fmt.Println("\nArray element analysis:")
	
	// Thread pools
	threadPools := make(map[string]bool)
	for _, chart := range charts {
		if strings.Contains(chart.Context, "thread_pools") {
			// Extract the pool name from labels
			for _, dim := range chart.Dimensions {
				if pool := dim.Metric.Labels["pool"]; pool != "" {
					threadPools[pool] = true
				}
			}
		}
	}
	fmt.Printf("Thread pools found: %d\n", len(threadPools))

	// JDBC pools
	jdbcPools := make(map[string]bool)
	for _, chart := range charts {
		if strings.Contains(chart.Context, "jdbc_connection_pools") {
			for _, dim := range chart.Dimensions {
				if ds := dim.Metric.Labels["datasource"]; ds != "" {
					jdbcPools[ds] = true
				}
			}
		}
	}
	fmt.Printf("JDBC data sources found: %d\n", len(jdbcPools))

	// Show example of what should be consolidated
	fmt.Println("\nExample of contexts that should be consolidated:")
	fmt.Println("\nThread Pool ActiveCount metrics (currently separate contexts):")
	for _, chart := range charts {
		if strings.Contains(chart.Context, "thread_pools") && strings.Contains(chart.Context, "activecount") {
			fmt.Printf("  Context: %s\n", chart.Context)
			for _, dim := range chart.Dimensions {
				fmt.Printf("    - Dimension: %s (pool=%s)\n", dim.Name, dim.Metric.Labels["pool"])
			}
		}
	}

	// Count homogeneous metrics
	metricTypeCount := make(map[string]int)
	for _, metric := range flatResult.Metrics {
		// Extract the metric name (last part of path)
		parts := strings.Split(metric.Path, "/")
		if len(parts) > 0 {
			metricName := parts[len(parts)-1]
			metricTypeCount[metricName]++
		}
	}

	fmt.Println("\nTop repeated metric types across instances:")
	type metricCount struct {
		name  string
		count int
	}
	var counts []metricCount
	for name, count := range metricTypeCount {
		if count > 3 { // Only show metrics that appear in multiple instances
			counts = append(counts, metricCount{name, count})
		}
	}
	sort.Slice(counts, func(i, j int) bool {
		return counts[i].count > counts[j].count
	})
	for i, mc := range counts {
		if i >= 10 {
			break
		}
		fmt.Printf("  %s: appears %d times\n", mc.name, mc.count)
	}
}