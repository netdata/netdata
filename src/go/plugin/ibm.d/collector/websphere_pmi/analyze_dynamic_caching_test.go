// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"encoding/xml"
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"
)

func TestAnalyzeDynamicCaching(t *testing.T) {
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

	// Look specifically at Dynamic Caching metrics
	fmt.Println("=== DYNAMIC CACHING ANALYSIS ===")
	
	// Find all Dynamic Caching metrics
	dynamicCachingMetrics := make(map[string][]MetricTuple)
	for _, metric := range flatResult.Metrics {
		if strings.Contains(metric.Path, "Dynamic Caching") {
			dynamicCachingMetrics[metric.Path] = append(dynamicCachingMetrics[metric.Path], metric)
		}
	}

	// Show sample paths
	fmt.Printf("\nTotal Dynamic Caching metrics: %d\n", len(dynamicCachingMetrics))
	fmt.Println("\nSample Dynamic Caching paths:")
	
	paths := make([]string, 0, len(dynamicCachingMetrics))
	for path := range dynamicCachingMetrics {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	
	for i, path := range paths {
		if i >= 10 {
			fmt.Printf("... and %d more\n", len(paths)-10)
			break
		}
		fmt.Printf("  %s\n", path)
	}

	// Analyze structure
	fmt.Println("\n=== STRUCTURE ANALYSIS ===")
	
	// Group by pattern
	patterns := make(map[string]int)
	for path := range dynamicCachingMetrics {
		parts := strings.Split(path, "/")
		if len(parts) >= 3 {
			// Try to identify the pattern
			pattern := ""
			for i, part := range parts {
				if part == "Dynamic Caching" {
					// Next parts are relevant
					if i+1 < len(parts) {
						pattern = parts[i+1]
						if strings.HasPrefix(pattern, "Object:") && i+2 < len(parts) {
							// This is a complex object path
							pattern = "Object: <complex path>"
						}
					}
					break
				}
			}
			patterns[pattern]++
		}
	}

	fmt.Println("\nDynamic Caching patterns:")
	for pattern, count := range patterns {
		fmt.Printf("  %s: %d paths\n", pattern, count)
	}

	// Check if Dynamic Caching is detected as an array
	fmt.Println("\n=== ARRAY DETECTION ===")
	for _, array := range flatResult.Arrays {
		if strings.Contains(array.Path, "Dynamic Caching") {
			fmt.Printf("Array detected: %s with %d elements\n", array.Path, len(array.Elements))
			// Show first few elements
			for i, elem := range array.Elements {
				if i >= 5 {
					fmt.Printf("  ... and %d more\n", len(array.Elements)-5)
					break
				}
				fmt.Printf("  - %s\n", elem)
			}
		}
	}

	// Check contexts generated
	fmt.Println("\n=== CONTEXTS GENERATED ===")
	contexts := make(map[string]bool)
	for _, metric := range flatResult.Metrics {
		if strings.Contains(metric.Path, "Dynamic Caching") {
			contexts[metric.UniqueContext] = true
		}
	}
	
	fmt.Printf("\nUnique Dynamic Caching contexts: %d\n", len(contexts))
	
	// Show a few examples
	contextList := make([]string, 0, len(contexts))
	for ctx := range contexts {
		contextList = append(contextList, ctx)
	}
	sort.Strings(contextList)
	
	fmt.Println("\nSample contexts:")
	for i, ctx := range contextList {
		if i >= 5 {
			fmt.Printf("... and %d more\n", len(contextList)-5)
			break
		}
		fmt.Printf("  %s\n", ctx)
	}

	// Check if they're marked as array elements
	arrayElementCount := 0
	for _, metric := range flatResult.Metrics {
		if strings.Contains(metric.Path, "Dynamic Caching") && metric.IsArrayElement {
			arrayElementCount++
		}
	}
	fmt.Printf("\nDynamic Caching metrics marked as array elements: %d\n", arrayElementCount)

	// Show the issue
	fmt.Println("\n=== THE ISSUE ===")
	fmt.Println("Dynamic Caching has a complex path structure:")
	fmt.Println("  Dynamic Caching/Object: ws/com.ibm.workplace/ExtensionRegistryCache/Object Cache/...")
	fmt.Println("The object path itself contains slashes, which creates very long paths")
	fmt.Println("This is not being detected as an array because the structure is different")

	// Run correlator to see final result
	fmt.Println("\n=== AFTER CORRELATION ===")
	correlator := NewCorrelationEngine()
	charts := correlator.CorrelateMetrics(flatResult.Metrics)
	
	// Count Dynamic Caching contexts after correlation
	dcContexts := make(map[string]int)
	for _, chart := range charts {
		if strings.Contains(chart.Context, "dynamic_caching") {
			dcContexts[chart.Context]++
		}
	}
	
	fmt.Printf("\nDynamic Caching contexts after correlation: %d\n", len(dcContexts))
	for ctx, count := range dcContexts {
		fmt.Printf("  %s (%d charts)\n", ctx, count)
	}
}