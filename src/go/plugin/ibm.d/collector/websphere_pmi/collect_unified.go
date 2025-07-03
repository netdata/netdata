// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// MetricDefinition defines a metric and its corresponding chart configuration
type MetricDefinition struct {
	ChartContext string
	ChartTitle   string
	ChartUnits   string
	ChartType    string
	ChartFamily  string
	Priority     int
	DimensionID  string // The dimension name within the chart
}

// DiscoveredMetric represents a metric found in the PMI data
type DiscoveredMetric struct {
	Definition   *MetricDefinition
	InstanceName string
	Labels       map[string]string
	Value        int64
}

// UnifiedCollector implements synchronized chart creation and value extraction
type UnifiedCollector struct {
	mu       sync.RWMutex
	charts   *module.Charts
	registry *MetricRegistry

	// Track created charts to avoid duplicates
	createdCharts map[string]bool
}

// NewUnifiedCollector creates a new unified collector
func NewUnifiedCollector(charts *module.Charts) *UnifiedCollector {
	return &UnifiedCollector{
		charts:        charts,
		registry:      NewMetricRegistry(),
		createdCharts: make(map[string]bool),
	}
}

// collectUnified implements the new unified collection approach
func (w *WebSpherePMI) collectUnified(ctx context.Context, stats *pmiStatsResponse) map[string]int64 {
	if w.unifiedCollector == nil {
		w.unifiedCollector = NewUnifiedCollector(w.charts)
	}

	// Phase 1: Discover all metrics and their values
	discovered := w.discoverMetrics(stats)

	// Phase 2: Create/update charts and extract values atomically
	mx := w.unifiedCollector.ProcessDiscoveredMetrics(discovered, w)

	// Phase 3: Cleanup charts for metrics no longer present
	w.unifiedCollector.CleanupAbsentCharts(discovered)

	return mx
}

// discoverMetrics walks the PMI data and discovers all available metrics
func (w *WebSpherePMI) discoverMetrics(stats *pmiStatsResponse) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	// Process each node/server hierarchy
	for _, node := range stats.Nodes {
		for _, server := range node.Servers {
			baseLabels := map[string]string{
				"node":   node.Name,
				"server": server.Name,
			}
			discovered = append(discovered, w.discoverServerMetrics(node.Name, server.Name, baseLabels, server.Stats)...)
		}
	}

	// Process direct stats (for Liberty and other variants)
	if len(stats.Stats) > 0 {
		baseLabels := map[string]string{
			"node":   "local",
			"server": "server",
		}
		discovered = append(discovered, w.discoverServerMetrics("local", "server", baseLabels, stats.Stats)...)
	}

	return discovered
}

// discoverServerMetrics discovers metrics in server stats
func (w *WebSpherePMI) discoverServerMetrics(nodeName, serverName string, baseLabels map[string]string, stats []pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	for _, stat := range stats {
		discovered = append(discovered, w.discoverStatMetrics(nodeName, serverName, baseLabels, "", stat)...)
	}

	return discovered
}

// discoverStatMetrics recursively discovers metrics in a stat entry
func (w *WebSpherePMI) discoverStatMetrics(nodeName, serverName string, baseLabels map[string]string, parentPath string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	currentPath := parentPath
	if currentPath != "" && stat.Name != "" {
		currentPath = currentPath + "." + stat.Name
	} else if stat.Name != "" {
		currentPath = stat.Name
	}

	// Process sub-stats first
	if len(stat.SubStats) > 0 {
		for _, subStat := range stat.SubStats {
			discovered = append(discovered, w.discoverStatMetrics(nodeName, serverName, baseLabels, currentPath, subStat)...)
		}
	}

	// Extract metrics from this stat
	if w.hasDirectMetrics(stat) {
		discovered = append(discovered, w.extractAndDefineMetrics(nodeName, serverName, baseLabels, currentPath, stat)...)
	}

	return discovered
}

// extractAndDefineMetrics extracts metrics and creates their definitions
func (w *WebSpherePMI) extractAndDefineMetrics(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	// Determine the metric category and context
	category := w.categorizeStatByContext(stat.Name, path)

	switch category {
	case "threading":
		discovered = append(discovered, w.extractThreadPoolMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "web":
		discovered = append(discovered, w.extractWebMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "connections":
		discovered = append(discovered, w.extractConnectionMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "server":
		discovered = append(discovered, w.extractServerMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "jvm":
		discovered = append(discovered, w.extractJVMMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "security":
		discovered = append(discovered, w.extractSecurityMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "transactions":
		discovered = append(discovered, w.extractTransactionMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	case "caching":
		discovered = append(discovered, w.extractCacheMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	default:
		// For unknown categories, extract generic metrics
		discovered = append(discovered, w.extractGenericMetricsUnified(nodeName, serverName, baseLabels, path, stat)...)
	}

	return discovered
}

// extractThreadPoolMetricsUnified extracts thread pool metrics with their definitions
func (w *WebSpherePMI) extractThreadPoolMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	poolName := stat.Name
	if poolName == "Thread Pools" {
		return discovered // Skip container
	}

	instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}
	labels["pool"] = poolName

	// Extract metrics from BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		if brs.Current == "" {
			continue
		}

		val, err := strconv.ParseInt(brs.Current, 10, 64)
		if err != nil {
			continue
		}

		var dimensionName string
		switch brs.Name {
		case "ActiveCount":
			dimensionName = "active"
		case "PoolSize":
			dimensionName = "pool_size"
		case "MaxPoolSize":
			dimensionName = "maximum_size"
		default:
			continue
		}

		discovered = append(discovered, DiscoveredMetric{
			Definition: &MetricDefinition{
				ChartContext: "websphere_pmi.threading.pools",
				ChartTitle:   "Thread Pool Usage",
				ChartUnits:   "threads",
				ChartType:    "stacked",
				ChartFamily:  "threading/pools",
				Priority:     70800,
				DimensionID:  dimensionName,
			},
			InstanceName: instanceName,
			Labels:       labels,
			Value:        val,
		})
	}

	// Also check for maximum_size in UpperBound
	for _, brs := range stat.BoundedRangeStatistics {
		if brs.Name == "PoolSize" && brs.UpperBound != "" {
			if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
				discovered = append(discovered, DiscoveredMetric{
					Definition: &MetricDefinition{
						ChartContext: "websphere_pmi.threading.pools",
						ChartTitle:   "Thread Pool Usage",
						ChartUnits:   "threads",
						ChartType:    "stacked",
						ChartFamily:  "threading/pools",
						Priority:     70800,
						DimensionID:  "maximum_size",
					},
					InstanceName: instanceName,
					Labels:       labels,
					Value:        val,
				})
			}
		}
	}

	return discovered
}

// extractWebMetricsUnified extracts web metrics with their definitions
func (w *WebSpherePMI) extractWebMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	// Check if this is a session metric for a web application
	if strings.Contains(stat.Name, "#") && strings.Contains(stat.Name, ".war") {
		appName := stat.Name
		instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName))

		labels := make(map[string]string)
		for k, v := range baseLabels {
			labels[k] = v
		}
		labels["application"] = appName

		// Extract session metrics
		for _, cs := range stat.CountStatistics {
			if cs.Count == "" {
				continue
			}

			val, err := strconv.ParseInt(cs.Count, 10, 64)
			if err != nil {
				continue
			}

			var dimensionName string
			switch cs.Name {
			case "ActiveCount", "LiveCount":
				dimensionName = "active"
			case "CreateCount", "CreatedCount":
				dimensionName = "created"
			case "InvalidateCount", "InvalidatedCount":
				dimensionName = "invalidated"
			case "SessionObjectSize":
				dimensionName = "lifetime"
			default:
				continue
			}

			discovered = append(discovered, DiscoveredMetric{
				Definition: &MetricDefinition{
					ChartContext: "websphere_pmi.web.sessions",
					ChartTitle:   "Web Application Sessions",
					ChartUnits:   "sessions",
					ChartType:    "line",
					ChartFamily:  "web/sessions",
					Priority:     70500,
					DimensionID:  dimensionName,
				},
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}
	}

	return discovered
}

// extractConnectionMetricsUnified extracts connection pool metrics with their definitions
func (w *WebSpherePMI) extractConnectionMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	pathLower := strings.ToLower(path)
	poolName := w.extractPoolName(path)
	if poolName == "" || !w.shouldCollectPool(poolName) {
		return discovered
	}

	instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}
	labels["pool"] = poolName

	// Determine if JDBC or JCA
	chartContext := "websphere_pmi.connections.jdbc"
	chartFamily := "connections/jdbc"
	priority := 70600
	if strings.Contains(pathLower, "jca") {
		chartContext = "websphere_pmi.connections.jca"
		chartFamily = "connections/jca"
		priority = 70700
	}

	// Extract metrics from BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		if brs.Current == "" {
			continue
		}

		val, err := strconv.ParseInt(brs.Current, 10, 64)
		if err != nil {
			continue
		}

		var dimensionName string
		switch brs.Name {
		case "ActiveCount":
			dimensionName = "active"
		case "FreePoolSize":
			dimensionName = "free"
		case "PoolSize":
			dimensionName = "total"
		default:
			continue
		}

		discovered = append(discovered, DiscoveredMetric{
			Definition: &MetricDefinition{
				ChartContext: chartContext,
				ChartTitle:   "Connection Pool",
				ChartUnits:   "connections",
				ChartType:    "stacked",
				ChartFamily:  chartFamily,
				Priority:     priority,
				DimensionID:  dimensionName,
			},
			InstanceName: instanceName,
			Labels:       labels,
			Value:        val,
		})
	}

	return discovered
}

// extractServerMetricsUnified extracts server metrics with their definitions
func (w *WebSpherePMI) extractServerMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	statNameLower := strings.ToLower(stat.Name)

	// Extension Registry metrics
	if strings.Contains(statNameLower, "extensionregistrystats") {
		labels := make(map[string]string)
		for k, v := range baseLabels {
			labels[k] = v
		}

		// Extract extension registry metrics
		for _, cs := range stat.CountStatistics {
			if cs.Count == "" {
				continue
			}

			val, err := strconv.ParseInt(cs.Count, 10, 64)
			if err != nil {
				continue
			}

			var dimensionName string
			switch cs.Name {
			case "ExtensionRequestCount":
				dimensionName = "requests"
			case "ExtensionCacheHits":
				dimensionName = "hits"
			default:
				continue
			}

			discovered = append(discovered, DiscoveredMetric{
				Definition: &MetricDefinition{
					ChartContext: "websphere_pmi.server.extensions",
					ChartTitle:   "Server Extensions",
					ChartUnits:   "requests",
					ChartType:    "stacked",
					ChartFamily:  "server",
					Priority:     70000,
					DimensionID:  dimensionName,
				},
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}

		// Calculate hit rate if we have the data
		for _, ds := range stat.DoubleStatistics {
			if ds.Name == "ExtensionCacheHitRate" && ds.Double != "" {
				if val, err := strconv.ParseFloat(ds.Double, 64); err == nil {
					discovered = append(discovered, DiscoveredMetric{
						Definition: &MetricDefinition{
							ChartContext: "websphere_pmi.server.extensions",
							ChartTitle:   "Server Extensions",
							ChartUnits:   "requests",
							ChartType:    "stacked",
							ChartFamily:  "server",
							Priority:     70000,
							DimensionID:  "hit_rate",
						},
						InstanceName: instanceName,
						Labels:       labels,
						Value:        int64(val * 1000), // Convert to per-thousand
					})
				}
			}
		}
	}

	return discovered
}

// extractJVMMetricsUnified extracts JVM metrics with their definitions
func (w *WebSpherePMI) extractJVMMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	statNameLower := strings.ToLower(stat.Name)

	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}

	// JVM Runtime metrics
	if strings.Contains(statNameLower, "jvm runtime") {
		// Extract memory metrics
		for _, cs := range stat.CountStatistics {
			if cs.Count == "" {
				continue
			}

			val, err := strconv.ParseInt(cs.Count, 10, 64)
			if err != nil {
				continue
			}

			var chartContext, dimensionName string
			var priority int

			switch cs.Name {
			case "HeapSize":
				chartContext = "websphere_pmi.jvm.memory"
				dimensionName = "HeapSize_current"
				priority = 70100
			case "FreeMemory":
				chartContext = "websphere_pmi.jvm.memory"
				dimensionName = "FreeMemory"
				priority = 70100
			case "UsedMemory":
				chartContext = "websphere_pmi.jvm.memory"
				dimensionName = "UsedMemory"
				priority = 70100
			case "ProcessCpuUsage":
				chartContext = "websphere_pmi.jvm.runtime"
				dimensionName = "ProcessCpuUsage"
				priority = 70101
			case "UpTime":
				chartContext = "websphere_pmi.jvm.runtime"
				dimensionName = "UpTime"
				priority = 70101
			default:
				continue
			}

			def := &MetricDefinition{
				ChartContext: chartContext,
				DimensionID:  dimensionName,
				Priority:     priority,
			}

			// Set chart-specific properties
			if chartContext == "websphere_pmi.jvm.memory" {
				def.ChartTitle = "JVM Memory"
				def.ChartUnits = "KB"
				def.ChartType = "stacked"
				def.ChartFamily = "jvm/memory"
			} else {
				def.ChartTitle = "JVM Runtime"
				def.ChartUnits = "value"
				def.ChartType = "line"
				def.ChartFamily = "jvm/runtime"
			}

			discovered = append(discovered, DiscoveredMetric{
				Definition:   def,
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}
	}

	// Object Pool metrics
	if strings.Contains(statNameLower, "object pool") {
		for _, cs := range stat.CountStatistics {
			if cs.Count == "" {
				continue
			}

			val, err := strconv.ParseInt(cs.Count, 10, 64)
			if err != nil {
				continue
			}

			var dimensionName string
			switch cs.Name {
			case "ObjectsCreatedCount":
				dimensionName = "ObjectsCreatedCount"
			case "ObjectsAllocatedCount":
				dimensionName = "ObjectsAllocatedCount_current"
			case "ObjectsReturnedCount":
				dimensionName = "ObjectsReturnedCount_current"
			case "IdleObjectsSize":
				dimensionName = "IdleObjectsSize_current"
			default:
				continue
			}

			discovered = append(discovered, DiscoveredMetric{
				Definition: &MetricDefinition{
					ChartContext: "websphere_pmi.jvm.object_pools",
					ChartTitle:   "JVM Object Pools",
					ChartUnits:   "objects",
					ChartType:    "line",
					ChartFamily:  "jvm/object_pools",
					Priority:     70102,
					DimensionID:  dimensionName,
				},
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}
	}

	return discovered
}

// extractSecurityMetricsUnified extracts security metrics with their definitions
func (w *WebSpherePMI) extractSecurityMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	pathLower := strings.ToLower(path)

	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}

	// Authentication metrics
	if strings.Contains(pathLower, "auth") && !strings.Contains(pathLower, "authz") {
		for _, cs := range stat.CountStatistics {
			if cs.Count == "" {
				continue
			}

			val, err := strconv.ParseInt(cs.Count, 10, 64)
			if err != nil {
				continue
			}

			var dimensionName string
			switch cs.Name {
			case "SuccessfulAuthenticationCount":
				dimensionName = "successful"
			case "FailedAuthenticationCount":
				dimensionName = "failed"
			case "ActiveSubjectsCount":
				dimensionName = "active_subjects"
			default:
				continue
			}

			discovered = append(discovered, DiscoveredMetric{
				Definition: &MetricDefinition{
					ChartContext: "websphere_pmi.security.authentication",
					ChartTitle:   "Authentication Events",
					ChartUnits:   "events/s",
					ChartType:    "stacked",
					ChartFamily:  "security",
					Priority:     71000,
					DimensionID:  dimensionName,
				},
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}
	}

	// Authorization metrics
	if strings.Contains(pathLower, "authz") || strings.Contains(pathLower, "authorization") {
		for _, cs := range stat.CountStatistics {
			if cs.Count == "" {
				continue
			}

			val, err := strconv.ParseInt(cs.Count, 10, 64)
			if err != nil {
				continue
			}

			var dimensionName string
			switch cs.Name {
			case "GrantedAuthorizationCount":
				dimensionName = "granted"
			case "DeniedAuthorizationCount":
				dimensionName = "denied"
			default:
				continue
			}

			discovered = append(discovered, DiscoveredMetric{
				Definition: &MetricDefinition{
					ChartContext: "websphere_pmi.security.authorization",
					ChartTitle:   "Authorization Events",
					ChartUnits:   "events/s",
					ChartType:    "stacked",
					ChartFamily:  "security",
					Priority:     71001,
					DimensionID:  dimensionName,
				},
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}
	}

	return discovered
}

// extractTransactionMetricsUnified extracts transaction metrics with their definitions
func (w *WebSpherePMI) extractTransactionMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}

	for _, cs := range stat.CountStatistics {
		if cs.Count == "" {
			continue
		}

		val, err := strconv.ParseInt(cs.Count, 10, 64)
		if err != nil {
			continue
		}

		var dimensionName string
		switch cs.Name {
		case "CommittedCount", "LocalCommittedCount":
			dimensionName = "committed"
		case "RolledbackCount", "LocalRolledbackCount":
			dimensionName = "rolled_back"
		case "ActiveCount", "LocalActiveCount":
			dimensionName = "active"
		default:
			continue
		}

		discovered = append(discovered, DiscoveredMetric{
			Definition: &MetricDefinition{
				ChartContext: "websphere_pmi.system.transactions",
				ChartTitle:   "Transactions",
				ChartUnits:   "transactions/s",
				ChartType:    "stacked",
				ChartFamily:  "system",
				Priority:     70900,
				DimensionID:  dimensionName,
			},
			InstanceName: instanceName,
			Labels:       labels,
			Value:        val,
		})
	}

	return discovered
}

// extractCacheMetricsUnified extracts cache metrics with their definitions
func (w *WebSpherePMI) extractCacheMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}

	for _, cs := range stat.CountStatistics {
		if cs.Count == "" {
			continue
		}

		val, err := strconv.ParseInt(cs.Count, 10, 64)
		if err != nil {
			continue
		}

		var dimensionName string
		nameLower := strings.ToLower(cs.Name)

		if strings.Contains(nameLower, "create") || strings.Contains(nameLower, "insert") {
			dimensionName = "creates"
		} else if strings.Contains(nameLower, "remove") || strings.Contains(nameLower, "evict") {
			dimensionName = "removes"
		} else {
			continue
		}

		discovered = append(discovered, DiscoveredMetric{
			Definition: &MetricDefinition{
				ChartContext: "websphere_pmi.caching.dynacache",
				ChartTitle:   "Dynamic Cache",
				ChartUnits:   "operations/s",
				ChartType:    "stacked",
				ChartFamily:  "caching",
				Priority:     71300,
				DimensionID:  dimensionName,
			},
			InstanceName: instanceName,
			Labels:       labels,
			Value:        val,
		})
	}

	return discovered
}

// extractGenericMetricsUnified extracts generic metrics with their definitions
func (w *WebSpherePMI) extractGenericMetricsUnified(nodeName, serverName string, baseLabels map[string]string, path string, stat pmiStat) []DiscoveredMetric {
	var discovered []DiscoveredMetric

	// For unknown metric types, create a generic chart if we have values
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	labels := make(map[string]string)
	for k, v := range baseLabels {
		labels[k] = v
	}
	labels["metric_path"] = stat.Name

	// Check for a simple value
	if stat.Value != nil && stat.Value.Value != "" {
		if val, err := strconv.ParseInt(stat.Value.Value, 10, 64); err == nil {
			discovered = append(discovered, DiscoveredMetric{
				Definition: &MetricDefinition{
					ChartContext: "websphere_pmi.monitoring.other",
					ChartTitle:   "Other Metrics",
					ChartUnits:   "value",
					ChartType:    "line",
					ChartFamily:  "monitoring",
					Priority:     79000,
					DimensionID:  "value",
				},
				InstanceName: instanceName,
				Labels:       labels,
				Value:        val,
			})
		}
	}

	return discovered
}

// ProcessDiscoveredMetrics creates charts and extracts values atomically
func (uc *UnifiedCollector) ProcessDiscoveredMetrics(discovered []DiscoveredMetric, w *WebSpherePMI) map[string]int64 {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	mx := make(map[string]int64)
	chartMetrics := make(map[string][]DiscoveredMetric) // Group by chart

	// Group metrics by chart
	for _, metric := range discovered {
		chartKey := fmt.Sprintf("%s|%s", metric.Definition.ChartContext, metric.InstanceName)
		chartMetrics[chartKey] = append(chartMetrics[chartKey], metric)
	}

	// Process each chart
	for chartKey, metrics := range chartMetrics {
		if len(metrics) == 0 {
			continue
		}

		// Get first metric to extract chart info
		firstMetric := metrics[0]
		def := firstMetric.Definition

		// Create chart ID
		contextWithoutModule := strings.TrimPrefix(def.ChartContext, "websphere_pmi.")
		chartID := fmt.Sprintf("%s_%s", contextWithoutModule, w.sanitizeForChartID(firstMetric.InstanceName))

		// Create chart if needed
		if !uc.charts.Has(chartID) {
			// Collect all dimension names for this chart
			dimensionMap := make(map[string]bool)
			for _, m := range metrics {
				dimensionMap[m.Definition.DimensionID] = true
			}

			dimensions := make([]string, 0, len(dimensionMap))
			for dim := range dimensionMap {
				dimensions = append(dimensions, dim)
			}

			// Create the chart
			chart := &module.Chart{
				ID:       chartID,
				Title:    def.ChartTitle,
				Units:    def.ChartUnits,
				Fam:      def.ChartFamily,
				Ctx:      def.ChartContext,
				Type:     module.ChartType(def.ChartType),
				Priority: def.Priority,
				Dims:     make(module.Dims, 0, len(dimensions)),
				Labels:   make([]module.Label, 0),
			}

			// Add dimensions
			for _, dim := range dimensions {
				dimID := fmt.Sprintf("%s_%s", chartID, dim)
				chart.Dims = append(chart.Dims, &module.Dim{
					ID:   dimID,
					Name: dim,
				})
			}

			// Add labels
			for key, value := range firstMetric.Labels {
				chart.Labels = append(chart.Labels, module.Label{
					Key:   key,
					Value: value,
				})
			}

			// Add additional labels from WebSpherePMI
			if w.ClusterName != "" {
				chart.Labels = append(chart.Labels, module.Label{Key: "cluster", Value: w.ClusterName})
			}
			if w.CellName != "" {
				chart.Labels = append(chart.Labels, module.Label{Key: "cell", Value: w.CellName})
			}
			if w.NodeName != "" && w.NodeName != firstMetric.Labels["node"] {
				chart.Labels = append(chart.Labels, module.Label{Key: "config_node", Value: w.NodeName})
			}
			if w.ServerType != "" {
				chart.Labels = append(chart.Labels, module.Label{Key: "server_type", Value: w.ServerType})
			}
			if w.wasVersion != "" {
				chart.Labels = append(chart.Labels, module.Label{Key: "websphere_version", Value: w.wasVersion})
			}
			if w.wasEdition != "" {
				chart.Labels = append(chart.Labels, module.Label{Key: "websphere_edition", Value: w.wasEdition})
			}

			// Add custom labels
			for k, v := range w.CustomLabels {
				chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
			}

			// Add the chart
			if err := uc.charts.Add(chart); err != nil {
				w.Warningf("failed to add chart %s: %v", chartID, err)
				continue
			}

			uc.createdCharts[chartKey] = true
			w.Debugf("created chart %s with %d dimensions", chartID, len(dimensions))
		}

		// Extract values for all metrics in this chart
		for _, metric := range metrics {
			metricKey := fmt.Sprintf("%s_%s", chartID, metric.Definition.DimensionID)
			mx[metricKey] = metric.Value
		}
	}

	return mx
}

// CleanupAbsentCharts removes charts for instances no longer present
func (uc *UnifiedCollector) CleanupAbsentCharts(currentMetrics []DiscoveredMetric) {
	uc.mu.Lock()
	defer uc.mu.Unlock()

	// Build set of current chart keys
	currentCharts := make(map[string]bool)
	for _, metric := range currentMetrics {
		chartKey := fmt.Sprintf("%s|%s", metric.Definition.ChartContext, metric.InstanceName)
		currentCharts[chartKey] = true
	}

	// Remove charts that are no longer present
	for chartKey := range uc.createdCharts {
		if !currentCharts[chartKey] {
			// Extract context and instance from key
			parts := strings.Split(chartKey, "|")
			if len(parts) == 2 {
				context := parts[0]
				instance := parts[1]
				contextWithoutModule := strings.TrimPrefix(context, "websphere_pmi.")
				chartID := fmt.Sprintf("%s_%s", contextWithoutModule, sanitizeForChartID(instance))

				if uc.charts.Has(chartID) {
					uc.charts.Remove(chartID)
				}
			}
			delete(uc.createdCharts, chartKey)
		}
	}
}

// Helper function for sanitization
func sanitizeForChartID(input string) string {
	sanitized := ""
	for _, char := range input {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '.' {
			sanitized += string(char)
		} else {
			sanitized += "_"
		}
	}
	return sanitized
}

// MetricRegistry manages metric definitions and mappings
type MetricRegistry struct {
	mu          sync.RWMutex
	definitions map[string]*MetricDefinition
}

// NewMetricRegistry creates a new metric registry
func NewMetricRegistry() *MetricRegistry {
	return &MetricRegistry{
		definitions: make(map[string]*MetricDefinition),
	}
}
