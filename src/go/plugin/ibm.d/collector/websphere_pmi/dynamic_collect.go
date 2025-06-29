// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	"strings"
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
	
	// Build mapping structures
	for i, candidate := range chartCandidates {
		chart := (*netdataCharts)[i]
		dc.chartMap[candidate.Context] = chart
		dc.lastChartMapping[candidate.Context] = candidate
		
		// Map dimensions to their IDs
		for j, dim := range candidate.Dimensions {
			dimID := chart.Dims[j].ID
			dc.dimensionMap[dim.Metric.Path] = dimID
		}
		
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
		TotalMetrics:    len(dc.lastFlatResult.Metrics),
		TotalArrays:     len(dc.lastFlatResult.Arrays),
		TotalCharts:     len(dc.chartMap),
		CategoryCounts:  categoryCounts,
		TypeCounts:      typeCounts,
		DimensionCount:  len(dc.dimensionMap),
	}
}

// MetricCoverageStats provides statistics about the dynamic collection
type MetricCoverageStats struct {
	TotalMetrics    int
	TotalArrays     int
	TotalCharts     int
	CategoryCounts  map[string]int
	TypeCounts      map[string]int
	DimensionCount  int
}

// EnableDynamicCollection enables dynamic collection on a WebSphere PMI collector
func (w *WebSpherePMI) EnableDynamicCollection() {
	w.dynamicCollector = NewDynamicCollector()
	w.useDynamicCollection = true
}

// collectDynamic performs dynamic collection using the new system
func (w *WebSpherePMI) collectDynamic(ctx context.Context, stats *pmiStatsResponse) map[string]int64 {
	if w.dynamicCollector == nil {
		w.EnableDynamicCollection()
	}
	
	return w.dynamicCollector.ProcessStats(stats, w.charts)
}

// Utility function for demonstrating the improvement
func (w *WebSpherePMI) CompareLegacyVsDynamic(ctx context.Context) (legacyCount, dynamicCount int) {
	// Fetch PMI stats
	stats, err := w.fetchPMIStats(ctx)
	if err != nil {
		return 0, 0
	}
	
	// Count legacy metrics
	legacyMx := make(map[string]int64)
	if len(stats.Nodes) > 0 && len(stats.Nodes[0].Servers) > 0 && len(stats.Nodes[0].Servers[0].Stats) > 0 {
		w.processStat(&stats.Nodes[0].Servers[0].Stats[0], "", legacyMx)
	}
	legacyCount = len(legacyMx)
	
	// Count dynamic metrics
	if w.dynamicCollector == nil {
		w.EnableDynamicCollection()
	}
	dynamicMx := w.collectDynamic(ctx, stats)
	dynamicCount = len(dynamicMx)
	
	return legacyCount, dynamicCount
}

// Integration helper: Modify processStat to build path correctly (this was our original bug fix)
func (w *WebSpherePMI) processStatWithPath(stat *pmiStat, parentPath string, mx map[string]int64) {
	// Build full path
	fullPath := stat.Path
	if fullPath == "" && parentPath != "" {
		fullPath = parentPath + "/" + stat.Name
	} else if fullPath == "" {
		fullPath = stat.Name
	}
	
	// CRITICAL: Assign the built path back to the stat so extraction functions can use it
	stat.Path = fullPath
	
	// Extract metrics from this stat
	w.extractFromStat(stat, mx)
	
	// Process substats recursively
	for _, subStat := range stat.SubStats {
		w.processStatWithPath(&subStat, fullPath, mx)
	}
}

// Mock extraction function (would call the real extraction functions)
func (w *WebSpherePMI) extractFromStat(stat *pmiStat, mx map[string]int64) {
	// This would call the existing extraction functions like:
	// w.extractJVMRuntimeMetrics, w.extractThreadPoolMetrics, etc.
	// For demonstration, we'll just count the available metrics
	
	metricCount := len(stat.CountStatistics) + len(stat.TimeStatistics) + 
		len(stat.BoundedRangeStatistics) + len(stat.RangeStatistics) + 
		len(stat.DoubleStatistics)
		
	if metricCount > 0 {
		// Generate a synthetic metric name for this stat
		metricName := strings.ReplaceAll(stat.Path, "/", "_")
		metricName = strings.ReplaceAll(metricName, " ", "_")
		metricName = strings.ToLower(metricName)
		
		mx[metricName+"_metric_count"] = int64(metricCount)
	}
}