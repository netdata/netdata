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

func TestFinalContextReduction(t *testing.T) {
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

	// Count unique contexts
	contextCount := make(map[string]int)
	for _, chart := range charts {
		contextCount[chart.Context]++
	}

	fmt.Printf("\n=== FINAL RESULTS ===\n")
	fmt.Printf("Total metrics: %d\n", len(flatResult.Metrics))
	fmt.Printf("Total charts: %d (was 118)\n", len(charts))
	fmt.Printf("Unique contexts: %d\n", len(contextCount))
	fmt.Printf("Arrays detected: %d\n\n", len(flatResult.Arrays))

	// Show array consolidation success
	fmt.Println("=== ARRAY CONSOLIDATION SUCCESS ===")
	type ArrayResult struct {
		Name           string
		Elements       int
		UniqueContexts int
		Contexts       []string
	}

	arrayResults := []ArrayResult{}

	for _, array := range flatResult.Arrays {
		// Find unique contexts for this array
		arrayContexts := make(map[string]bool)
		for _, metric := range flatResult.Metrics {
			if metric.IsArrayElement && metric.ArrayPath == array.Path {
				arrayContexts[metric.UniqueContext] = true
			}
		}

		// Get sorted context list
		contexts := make([]string, 0, len(arrayContexts))
		for ctx := range arrayContexts {
			contexts = append(contexts, ctx)
		}
		sort.Strings(contexts)

		result := ArrayResult{
			Name:           array.Path,
			Elements:       len(array.Elements),
			UniqueContexts: len(arrayContexts),
			Contexts:       contexts,
		}
		arrayResults = append(arrayResults, result)
	}

	// Sort by name for consistent output
	sort.Slice(arrayResults, func(i, j int) bool {
		return arrayResults[i].Name < arrayResults[j].Name
	})

	for _, result := range arrayResults {
		fmt.Printf("\n%s:\n", result.Name)
		fmt.Printf("  Elements: %d\n", result.Elements)
		fmt.Printf("  Unique contexts: %d (ideal ratio)\n", result.UniqueContexts)

		// Show first few contexts
		for i, ctx := range result.Contexts {
			if i >= 3 {
				fmt.Printf("  ... and %d more contexts\n", len(result.Contexts)-3)
				break
			}
			// Extract just the last part for readability
			parts := strings.Split(ctx, ".")
			shortCtx := parts[len(parts)-1]
			fmt.Printf("  - ...%s\n", shortCtx)
		}
	}

	// Category breakdown
	fmt.Println("\n=== CONTEXTS BY CATEGORY ===")
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

	// Verify we achieved our goals
	assert.Less(t, len(charts), 100, "Should have fewer than 100 charts")
	assert.Less(t, len(contextCount), 90, "Should have fewer than 90 unique contexts")

	// Specific verifications
	threadPoolContexts := 0
	dynamicCachingContexts := 0
	for ctx := range contextCount {
		if strings.Contains(ctx, "thread_pools") {
			threadPoolContexts++
		}
		if strings.Contains(ctx, "dynamic_caching") {
			dynamicCachingContexts++
		}
	}

	fmt.Printf("\n=== SPECIFIC IMPROVEMENTS ===\n")
	fmt.Printf("Thread Pools: %d contexts (was ~110)\n", threadPoolContexts)
	fmt.Printf("Dynamic Caching: %d contexts (was 96)\n", dynamicCachingContexts)

	assert.Less(t, threadPoolContexts, 15, "Thread pools should have < 15 contexts")
	assert.Less(t, dynamicCachingContexts, 10, "Dynamic caching should have < 10 contexts")
}
