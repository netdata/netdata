// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// ChartCandidate represents a potential Netdata chart
type ChartCandidate struct {
	Context    string            // Chart context (e.g., websphere_pmi.thread_pools.active_count)
	Title      string            // Human-readable title
	Units      string            // Chart units
	Type       string            // Chart type (line, area, stacked)
	Family     string            // Chart family for grouping
	Priority   int               // Chart priority
	Dimensions []DimensionCandidate
	Labels     map[string]string // Common labels for all dimensions
}

// DimensionCandidate represents a potential chart dimension
type DimensionCandidate struct {
	ID     string // Dimension ID
	Name   string // Dimension name
	Metric MetricTuple
}

// CorrelationEngine groups metrics into charts using heuristics
type CorrelationEngine struct {
	// Heuristics configuration
	maxDimensionsPerChart int
	priorityBase          int
}

// NewCorrelationEngine creates a new correlation engine
func NewCorrelationEngine() *CorrelationEngine {
	return &CorrelationEngine{
		maxDimensionsPerChart: 50, // Netdata limit
		priorityBase:          70000,
	}
}

// CorrelateMetrics groups metrics into chart candidates
func (c *CorrelationEngine) CorrelateMetrics(metrics []MetricTuple) []ChartCandidate {
	// Group metrics by correlation keys
	groups := c.groupMetricsByCorrelation(metrics)
	
	// Convert groups to chart candidates
	charts := make([]ChartCandidate, 0, len(groups))
	priority := c.priorityBase
	
	for _, group := range groups {
		chart := c.createChartFromGroup(group, priority)
		charts = append(charts, chart)
		priority++
	}
	
	// Sort charts by priority
	sort.Slice(charts, func(i, j int) bool {
		return charts[i].Priority < charts[j].Priority
	})
	
	return charts
}

// MetricGroup represents a group of correlated metrics
type MetricGroup struct {
	CorrelationKey string
	BaseContext    string
	CommonLabels   map[string]string
	Metrics        []MetricTuple
	Unit           string
	Type           string
}

// groupMetricsByCorrelation groups metrics using correlation heuristics
func (c *CorrelationEngine) groupMetricsByCorrelation(metrics []MetricTuple) []MetricGroup {
	groupMap := make(map[string]*MetricGroup)
	
	for _, metric := range metrics {
		key := c.generateCorrelationKey(metric)
		
		if group, exists := groupMap[key]; exists {
			// Add to existing group if compatible
			if c.areMetricsCompatible(group.Metrics[0], metric) {
				group.Metrics = append(group.Metrics, metric)
			} else {
				// Create new group with suffix
				newKey := key + "_" + c.getMetricTypeSuffix(metric)
				if _, exists := groupMap[newKey]; !exists {
					groupMap[newKey] = c.createNewGroup(newKey, metric)
				} else {
					groupMap[newKey].Metrics = append(groupMap[newKey].Metrics, metric)
				}
			}
		} else {
			groupMap[key] = c.createNewGroup(key, metric)
		}
	}
	
	// Convert map to slice and filter groups
	groups := make([]MetricGroup, 0, len(groupMap))
	for _, group := range groupMap {
		if len(group.Metrics) > 0 {
			// Split large groups
			if len(group.Metrics) > c.maxDimensionsPerChart {
				subGroups := c.splitLargeGroup(group)
				groups = append(groups, subGroups...)
			} else {
				groups = append(groups, *group)
			}
		}
	}
	
	return groups
}

// generateCorrelationKey creates a key for grouping related metrics
func (c *CorrelationEngine) generateCorrelationKey(metric MetricTuple) string {
	parts := strings.Split(metric.Path, "/")
	
	// Extract semantic components
	var category, subcategory, metricFamily string
	
	// Identify category from labels or path
	if cat, ok := metric.Labels["category"]; ok {
		category = cat
	} else {
		// Infer from path
		for _, part := range parts {
			if c.isKnownCategory(part) {
				category = c.normalizeCategory(part)
				break
			}
		}
	}
	
	// Identify subcategory
	if subcat, ok := metric.Labels["subcategory"]; ok {
		subcategory = subcat
	}
	
	// Extract metric family (last part of path without instances)
	metricName := parts[len(parts)-1]
	metricFamily = c.extractMetricFamily(metricName)
	
	// Build correlation key
	keyParts := []string{"websphere_pmi"}
	
	if category != "" {
		keyParts = append(keyParts, category)
	}
	
	if subcategory != "" {
		keyParts = append(keyParts, subcategory)
	}
	
	if metricFamily != "" {
		keyParts = append(keyParts, metricFamily)
	} else {
		keyParts = append(keyParts, "metrics")
	}
	
	return strings.Join(keyParts, ".")
}

// isKnownCategory checks if a path part represents a known category
func (c *CorrelationEngine) isKnownCategory(part string) bool {
	knownCategories := []string{
		"Thread Pools", "JDBC Connection Pools", "Web Applications", 
		"JVM Runtime", "Servlets", "Portlets", "server",
	}
	
	for _, cat := range knownCategories {
		if part == cat {
			return true
		}
	}
	return false
}

// normalizeCategory converts category names to consistent identifiers
func (c *CorrelationEngine) normalizeCategory(category string) string {
	switch category {
	case "Thread Pools":
		return "thread_pools"
	case "JDBC Connection Pools":
		return "jdbc_pools"
	case "Web Applications":
		return "web_apps"
	case "JVM Runtime":
		return "jvm"
	case "Servlets":
		return "servlets"
	case "Portlets":
		return "portlets"
	default:
		return strings.ToLower(strings.ReplaceAll(category, " ", "_"))
	}
}

// extractMetricFamily groups related metrics by semantic meaning
func (c *CorrelationEngine) extractMetricFamily(metricName string) string {
	name := strings.ToLower(metricName)
	
	// Connection/pool metrics
	if strings.Contains(name, "active") || strings.Contains(name, "pool") {
		return "active_connections"
	}
	if strings.Contains(name, "create") || strings.Contains(name, "destroy") {
		return "connection_lifecycle"
	}
	if strings.Contains(name, "wait") || strings.Contains(name, "queue") {
		return "wait_times"
	}
	
	// Request metrics
	if strings.Contains(name, "request") || strings.Contains(name, "served") {
		return "requests"
	}
	if strings.Contains(name, "response") || strings.Contains(name, "time") {
		return "response_times"
	}
	
	// Error metrics
	if strings.Contains(name, "error") || strings.Contains(name, "fault") {
		return "errors"
	}
	
	// Memory metrics
	if strings.Contains(name, "heap") || strings.Contains(name, "memory") {
		return "memory"
	}
	if strings.Contains(name, "gc") || strings.Contains(name, "garbage") {
		return "garbage_collection"
	}
	
	// Utilization metrics
	if strings.Contains(name, "percent") || strings.Contains(name, "utilization") {
		return "utilization"
	}
	
	// Default: use the metric name itself (normalized)
	return strings.ToLower(strings.ReplaceAll(metricName, " ", "_"))
}

// areMetricsCompatible checks if two metrics can be in the same chart
func (c *CorrelationEngine) areMetricsCompatible(m1, m2 MetricTuple) bool {
	// Same unit is required for additive dimensions
	if m1.Unit != m2.Unit {
		return false
	}
	
	// Same type is preferred
	if m1.Type != m2.Type {
		// Some exceptions: count and bounded_range can be mixed if units match
		if !((m1.Type == "count" && m2.Type == "bounded_range") ||
			 (m1.Type == "bounded_range" && m2.Type == "count")) {
			return false
		}
	}
	
	// Check if dimensions would be semantically additive
	return c.areMetricsAdditive(m1, m2)
}

// areMetricsAdditive checks if metrics represent additive quantities
func (c *CorrelationEngine) areMetricsAdditive(m1, m2 MetricTuple) bool {
	// Metrics from the same instance/pool are typically additive
	if m1.Labels["instance"] != "" && m1.Labels["instance"] == m2.Labels["instance"] {
		return true
	}
	
	// Metrics with same semantic meaning are usually additive
	family1 := c.extractMetricFamily(strings.Split(m1.Path, "/")[len(strings.Split(m1.Path, "/"))-1])
	family2 := c.extractMetricFamily(strings.Split(m2.Path, "/")[len(strings.Split(m2.Path, "/"))-1])
	
	return family1 == family2
}

// createNewGroup creates a new metric group
func (c *CorrelationEngine) createNewGroup(key string, metric MetricTuple) *MetricGroup {
	return &MetricGroup{
		CorrelationKey: key,
		BaseContext:    key,
		CommonLabels:   c.extractCommonLabels([]MetricTuple{metric}),
		Metrics:        []MetricTuple{metric},
		Unit:           metric.Unit,
		Type:           c.inferChartType(metric),
	}
}

// extractCommonLabels finds labels common to all metrics in a group
func (c *CorrelationEngine) extractCommonLabels(metrics []MetricTuple) map[string]string {
	if len(metrics) == 0 {
		return make(map[string]string)
	}
	
	common := make(map[string]string)
	firstMetric := metrics[0]
	
	// Check each label in the first metric
	for key, value := range firstMetric.Labels {
		isCommon := true
		for i := 1; i < len(metrics); i++ {
			if metrics[i].Labels[key] != value {
				isCommon = false
				break
			}
		}
		if isCommon {
			common[key] = value
		}
	}
	
	return common
}

// inferChartType determines the appropriate chart type
func (c *CorrelationEngine) inferChartType(metric MetricTuple) string {
	// Time series are typically line charts
	if metric.Type == "time" {
		return "line"
	}
	
	// Utilization/percentage metrics are area charts
	if metric.Unit == "percent" {
		return "area"
	}
	
	// Count/rate metrics are line charts
	if strings.Contains(metric.Unit, "requests") {
		return "line"
	}
	
	// Default to line
	return "line"
}

// splitLargeGroup splits groups with too many dimensions
func (c *CorrelationEngine) splitLargeGroup(group *MetricGroup) []MetricGroup {
	subGroups := make([]MetricGroup, 0)
	
	// Split by instance if available
	instanceGroups := make(map[string][]MetricTuple)
	for _, metric := range group.Metrics {
		instance := metric.Labels["instance"]
		if instance == "" {
			instance = "default"
		}
		instanceGroups[instance] = append(instanceGroups[instance], metric)
	}
	
	for instance, metrics := range instanceGroups {
		subGroup := MetricGroup{
			CorrelationKey: group.CorrelationKey + "_" + instance,
			BaseContext:    group.BaseContext + "_" + instance,
			CommonLabels:   c.extractCommonLabels(metrics),
			Metrics:        metrics,
			Unit:           group.Unit,
			Type:           group.Type,
		}
		subGroups = append(subGroups, subGroup)
	}
	
	return subGroups
}

// createChartFromGroup converts a metric group to a chart candidate
func (c *CorrelationEngine) createChartFromGroup(group MetricGroup, priority int) ChartCandidate {
	// Generate dimensions
	dimensions := make([]DimensionCandidate, len(group.Metrics))
	for i, metric := range group.Metrics {
		dimensions[i] = DimensionCandidate{
			ID:     c.generateDimensionID(metric),
			Name:   c.generateDimensionName(metric),
			Metric: metric,
		}
	}
	
	// Generate chart properties
	chart := ChartCandidate{
		Context:    group.BaseContext,
		Title:      c.generateChartTitle(group),
		Units:      group.Unit,
		Type:       group.Type,
		Family:     c.generateChartFamily(group),
		Priority:   priority,
		Dimensions: dimensions,
		Labels:     group.CommonLabels,
	}
	
	return chart
}

// generateDimensionID creates a unique dimension ID
func (c *CorrelationEngine) generateDimensionID(metric MetricTuple) string {
	parts := []string{}
	
	// Add instance identifier if available
	if instance := metric.Labels["instance"]; instance != "" {
		parts = append(parts, instance)
	}
	
	// Add metric name (last part of path)
	pathParts := strings.Split(metric.Path, "/")
	metricName := pathParts[len(pathParts)-1]
	parts = append(parts, strings.ToLower(strings.ReplaceAll(metricName, " ", "_")))
	
	if len(parts) == 0 {
		return "metric"
	}
	
	return strings.Join(parts, "_")
}

// generateDimensionName creates a human-readable dimension name
func (c *CorrelationEngine) generateDimensionName(metric MetricTuple) string {
	// Use instance name if available
	if instance := metric.Labels["instance"]; instance != "" {
		return instance
	}
	
	// Use last part of path
	pathParts := strings.Split(metric.Path, "/")
	return pathParts[len(pathParts)-1]
}

// generateChartTitle creates a descriptive chart title
func (c *CorrelationEngine) generateChartTitle(group MetricGroup) string {
	parts := strings.Split(group.BaseContext, ".")
	
	// Extract meaningful parts
	var category, metricType string
	if len(parts) >= 2 {
		category = strings.ReplaceAll(parts[1], "_", " ")
		category = strings.Title(category)
	}
	if len(parts) >= 3 {
		metricType = strings.ReplaceAll(parts[2], "_", " ")
		metricType = strings.Title(metricType)
	}
	
	if category != "" && metricType != "" {
		return fmt.Sprintf("%s %s", category, metricType)
	} else if category != "" {
		return category
	} else {
		return "WebSphere PMI Metrics"
	}
}

// generateChartFamily creates a chart family for grouping
func (c *CorrelationEngine) generateChartFamily(group MetricGroup) string {
	parts := strings.Split(group.BaseContext, ".")
	if len(parts) >= 2 {
		return parts[1] // Use category as family
	}
	return "websphere"
}

// getMetricTypeSuffix generates a suffix based on metric type
func (c *CorrelationEngine) getMetricTypeSuffix(metric MetricTuple) string {
	switch metric.Type {
	case "count":
		return "count"
	case "time":
		return "time"
	case "bounded_range":
		return "current"
	case "range":
		return "range"
	case "double":
		return "value"
	default:
		return "metric"
	}
}

// ConvertToNetdataCharts converts chart candidates to Netdata module charts
func (c *CorrelationEngine) ConvertToNetdataCharts(candidates []ChartCandidate) *module.Charts {
	charts := &module.Charts{}
	
	for _, candidate := range candidates {
		chart := &module.Chart{
			ID:       strings.ReplaceAll(candidate.Context, ".", "_"),
			Title:    candidate.Title,
			Units:    candidate.Units,
			Fam:      candidate.Family,
			Ctx:      candidate.Context,
			Type:     module.ChartType(candidate.Type),
			Priority: candidate.Priority,
		}
		
		// Add dimensions
		for _, dim := range candidate.Dimensions {
			chart.Dims = append(chart.Dims, &module.Dim{
				ID:   dim.ID,
				Name: dim.Name,
			})
		}
		
		*charts = append(*charts, chart)
	}
	
	return charts
}