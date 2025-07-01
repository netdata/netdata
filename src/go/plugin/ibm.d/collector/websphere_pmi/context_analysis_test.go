package websphere_pmi

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestAnalyzeContextProliferation(t *testing.T) {
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
	fmt.Printf("\n=== CONTEXT PROLIFERATION ANALYSIS ===\n")
	fmt.Printf("Total metrics flattened: %d\n", len(flatResult.Metrics))
	fmt.Printf("Total charts created: %d\n", len(charts))
	fmt.Printf("Arrays detected: %d\n\n", len(flatResult.Arrays))

	// Group contexts by base pattern
	contextGroups := make(map[string][]string)
	for _, chart := range charts {
		// Extract base pattern (remove instance-specific parts)
		parts := strings.Split(chart.Context, ".")
		if len(parts) >= 4 {
			// Check if this looks like an array element (has specific instance name)
			basePattern := strings.Join(parts[:3], ".") // websphere_pmi.server.category
			contextGroups[basePattern] = append(contextGroups[basePattern], chart.Context)
		}
	}

	// Show groups with multiple contexts
	fmt.Println("Context groups with proliferation (should ideally be single context):")
	for pattern, contexts := range contextGroups {
		if len(contexts) > 3 {
			fmt.Printf("\nBase: %s\n", pattern)
			fmt.Printf("  Has %d different contexts (should be 1!)\n", len(contexts))

			// Show first few
			sort.Strings(contexts)
			for i, ctx := range contexts {
				if i >= 3 {
					fmt.Printf("  ... and %d more\n", len(contexts)-3)
					break
				}
				fmt.Printf("  - %s\n", ctx)
			}
		}
	}

	// Analyze homogeneous metrics in arrays
	fmt.Println("\n=== HOMOGENEOUS METRICS IN ARRAYS ===")

	// Look at specific array types
	for _, array := range flatResult.Arrays {
		if len(array.Elements) > 0 {
			fmt.Printf("\nArray: %s (%d elements)\n", array.Path, len(array.Elements))

			// Check what metrics are common across all elements
			commonMetrics := make(map[string]int)
			for _, metric := range flatResult.Metrics {
				if strings.HasPrefix(metric.Path, array.Path+"/") {
					// Extract metric name relative to array element
					relativePath := strings.TrimPrefix(metric.Path, array.Path+"/")
					parts := strings.SplitN(relativePath, "/", 2)
					if len(parts) == 2 {
						metricName := parts[1]
						commonMetrics[metricName]++
					}
				}
			}

			// Show metrics that appear in all elements
			fmt.Println("  Common metrics across all elements:")
			for metricName, count := range commonMetrics {
				if count == len(array.Elements) {
					fmt.Printf("    - %s (in all %d elements)\n", metricName, count)
				}
			}
		}
	}

	// Show the problem with specific examples
	fmt.Println("\n=== EXAMPLE: Thread Pools ===")
	threadPoolContexts := make(map[string]bool)
	for _, chart := range charts {
		if strings.Contains(chart.Context, "thread_pools") {
			threadPoolContexts[chart.Context] = true
		}
	}
	fmt.Printf("Thread pool contexts created: %d\n", len(threadPoolContexts))
	fmt.Println("Examples:")
	count := 0
	for ctx := range threadPoolContexts {
		fmt.Printf("  - %s\n", ctx)
		count++
		if count >= 5 {
			fmt.Printf("  ... and %d more\n", len(threadPoolContexts)-5)
			break
		}
	}

	fmt.Println("\n=== PROPOSED SOLUTION ===")
	fmt.Println("Instead of:")
	fmt.Println("  websphere_pmi.server.thread_pools.webcontainer.activecount")
	fmt.Println("  websphere_pmi.server.thread_pools.default.activecount")
	fmt.Println("  websphere_pmi.server.thread_pools.sib_jms_resource_adapter.activecount")
	fmt.Println("\nWe should have:")
	fmt.Println("  websphere_pmi.server.thread_pools.activecount")
	fmt.Println("    with dimensions: webcontainer, default, sib_jms_resource_adapter")
	fmt.Println("    and labels: pool=<pool_name> for each dimension")
}
