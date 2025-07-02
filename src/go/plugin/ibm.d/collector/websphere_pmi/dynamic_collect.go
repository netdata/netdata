// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	"fmt"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// DynamicCollector provides dynamic metric collection using the flattener and correlator
type DynamicCollector struct {
	flattener  *XMLFlattener
	correlator *CorrelationEngine

	// Chart management
	chartMap     map[string]*module.Chart // Map from context to chart
	dimensionMap map[string]string        // Map from metric path to dimension ID

	// Cached results
	lastFlatResult   *FlattenerResult
	lastChartMapping map[string]ChartCandidate

	// Thread safety
	mu sync.RWMutex
}

// NewDynamicCollector creates a new dynamic collector
func NewDynamicCollector() *DynamicCollector {
	return &DynamicCollector{
		flattener:        NewXMLFlattener(),
		correlator:       NewCorrelationEngine(),
		chartMap:         make(map[string]*module.Chart),
		dimensionMap:     make(map[string]string),
		lastChartMapping: make(map[string]ChartCandidate),
	}
}

// ProcessStats processes PMI stats using the dynamic system
func (dc *DynamicCollector) ProcessStats(stats *pmiStatsResponse, charts *module.Charts) map[string]int64 {
	dc.mu.Lock()
	defer dc.mu.Unlock()

	// Step 1: Flatten the XML structure
	flatResult := dc.flattener.FlattenPMIStats(stats)
	dc.lastFlatResult = flatResult

	// Step 2: Correlate metrics into charts (only if this is first run or structure changed)
	if len(dc.chartMap) == 0 {
		dc.initializeCharts(flatResult, charts)
	}

	// Step 3: Collect metrics into Netdata format
	return dc.collectMetrics(flatResult)
}

// initializeCharts sets up the charts based on flattened metrics
func (dc *DynamicCollector) initializeCharts(flatResult *FlattenerResult, charts *module.Charts) {
	// Correlate metrics into chart candidates
	chartCandidates := dc.correlator.CorrelateMetrics(flatResult.Metrics)

	// Convert to Netdata charts and add to module
	netdataCharts := dc.correlator.ConvertToNetdataCharts(chartCandidates)

	// Build mapping structures with duplicate prevention
	for i, candidate := range chartCandidates {
		chart := (*netdataCharts)[i]

		// Ensure unique dimension IDs within each chart
		dimIDSeen := make(map[string]bool)
		for j, dim := range chart.Dims {
			baseID := dim.ID
			uniqueID := baseID
			counter := 1

			// If ID already exists, append a counter
			for dimIDSeen[uniqueID] {
				uniqueID = fmt.Sprintf("%s_%d", baseID, counter)
				counter++
			}

			dimIDSeen[uniqueID] = true
			dim.ID = uniqueID

			// Update the dimension mapping with the unique ID
			dc.dimensionMap[candidate.Dimensions[j].Metric.Path] = uniqueID
		}

		dc.chartMap[candidate.Context] = chart
		dc.lastChartMapping[candidate.Context] = candidate

		// Add chart to module charts
		*charts = append(*charts, chart)
	}
}

// collectMetrics collects current metric values
func (dc *DynamicCollector) collectMetrics(flatResult *FlattenerResult) map[string]int64 {
	collected := make(map[string]int64)

	// Collect all flattened metrics
	for _, metric := range flatResult.Metrics {
		if dimID, exists := dc.dimensionMap[metric.Path]; exists {
			collected[dimID] = metric.Value
		}
	}

	return collected
}

// GetDynamicCharts returns the dynamically generated charts for external access
func (dc *DynamicCollector) GetDynamicCharts() []ChartCandidate {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	charts := make([]ChartCandidate, 0, len(dc.lastChartMapping))
	for _, chart := range dc.lastChartMapping {
		charts = append(charts, chart)
	}
	return charts
}

// GetMetricCoverage returns statistics about metric coverage
func (dc *DynamicCollector) GetMetricCoverage() MetricCoverageStats {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	if dc.lastFlatResult == nil {
		return MetricCoverageStats{}
	}

	// Count metrics by category
	categoryCounts := make(map[string]int)
	typeCounts := make(map[string]int)

	for _, metric := range dc.lastFlatResult.Metrics {
		category := metric.Labels["category"]
		if category == "" {
			category = "uncategorized"
		}
		categoryCounts[category]++
		typeCounts[metric.Type]++
	}

	return MetricCoverageStats{
		TotalMetrics:   len(dc.lastFlatResult.Metrics),
		TotalArrays:    len(dc.lastFlatResult.Arrays),
		TotalCharts:    len(dc.chartMap),
		CategoryCounts: categoryCounts,
		TypeCounts:     typeCounts,
		DimensionCount: len(dc.dimensionMap),
	}
}

// MetricCoverageStats provides statistics about the dynamic collection
type MetricCoverageStats struct {
	TotalMetrics   int
	TotalArrays    int
	TotalCharts    int
	CategoryCounts map[string]int
	TypeCounts     map[string]int
	DimensionCount int
}

// EnableDynamicCollection enables dynamic collection on a WebSphere PMI collector
func (w *WebSpherePMI) EnableDynamicCollection() {
	w.dynamicCollector = NewDynamicCollector()
}

// collectDynamic performs dynamic collection using the new system
func (w *WebSpherePMI) collectDynamic(ctx context.Context, stats *pmiStatsResponse) map[string]int64 {
	if w.dynamicCollector == nil {
		w.EnableDynamicCollection()
	}

	return w.dynamicCollector.ProcessStats(stats, w.charts)
}
