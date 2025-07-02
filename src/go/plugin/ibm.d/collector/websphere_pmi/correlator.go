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
	Context    string // Chart context (e.g., websphere_pmi.thread_pools.active_count)
	Title      string // Human-readable title
	Units      string // Chart units
	Type       string // Chart type (line, area, stacked)
	Family     string // Chart family for grouping
	Priority   int    // Chart priority
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
	MetricName     string // The specific metric being measured
	CategoryPath   string // The hierarchical category path
}

// groupMetricsByCorrelation groups metrics using NIDL-compliant correlation
func (c *CorrelationEngine) groupMetricsByCorrelation(metrics []MetricTuple) []MetricGroup {
	groupMap := make(map[string]*MetricGroup)

	for _, metric := range metrics {
		key := c.generateCorrelationKey(metric)

		if group, exists := groupMap[key]; exists {
			// Add to existing group
			group.Metrics = append(group.Metrics, metric)
		} else {
			// Create new group
			groupMap[key] = c.createNewGroup(key, metric)
		}
	}

	// Convert map to slice and split large groups if needed
	groups := make([]MetricGroup, 0, len(groupMap))
	for _, group := range groupMap {
		if len(group.Metrics) > 0 {
			// Split large groups if they exceed Netdata's limit
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

// generateCorrelationKey creates a key for grouping related metrics according to NIDL principles
// The key should group metrics with:
// 1. Same category (e.g., Thread Pools)
// 2. Same metric name (e.g., ActiveCount)
// 3. Different instances (e.g., WebContainer, Default)
func (c *CorrelationEngine) generateCorrelationKey(metric MetricTuple) string {
	// Extract the category path and metric name
	categoryPath, metricName := c.extractCategoryAndMetric(metric)

	// Determine the proper unit for this metric
	properUnit := c.determineProperUnit(metricName, metric)

	// Create a key that groups same metric across different instances
	// Format: category_path.metric_name.unit
	return fmt.Sprintf("%s.%s.%s",
		sanitizeDimensionID(categoryPath),
		sanitizeDimensionID(metricName),
		sanitizeDimensionID(properUnit))
}

// extractCategoryAndMetric extracts the category path and metric name from a metric
func (c *CorrelationEngine) extractCategoryAndMetric(metric MetricTuple) (categoryPath, metricName string) {
	pathParts := strings.Split(metric.Path, "/")

	if len(pathParts) == 0 {
		return "", ""
	}

	// The last part is always the metric name
	metricName = pathParts[len(pathParts)-1]

	// Remove "server/" prefix if present
	cleanPath := strings.TrimPrefix(metric.Path, "server/")
	cleanParts := strings.Split(cleanPath, "/")

	// For NIDL compliance, we need to determine the semantic category
	// not the full hierarchical path
	if len(cleanParts) > 1 {
		primaryCat := cleanParts[0]

		switch primaryCat {
		case "Thread Pools":
			// For thread pools, the category is just "Thread Pools"
			// Instance would be the pool name (e.g., "WebContainer")
			categoryPath = "thread_pools"

		case "JDBC Connection Pools":
			// For JDBC pools, category is "JDBC Connection Pools"
			// Instance would be the datasource name
			categoryPath = "jdbc_connection_pools"

		case "JCA Connection Pools":
			// For JCA pools, include the adapter type if present
			if len(cleanParts) > 2 {
				// e.g., "JCA Connection Pools/SIB JMS Resource Adapter"
				categoryPath = fmt.Sprintf("jca_connection_pools.%s", sanitizeDimensionID(cleanParts[1]))
			} else {
				categoryPath = "jca_connection_pools"
			}

		case "Web Applications":
			// For web apps, we need to look deeper to find the semantic category
			if c.containsServlets(cleanParts) {
				categoryPath = "web_applications.servlets"
			} else if c.containsPortlets(cleanParts) {
				categoryPath = "web_applications.portlets"
			} else {
				categoryPath = "web_applications"
			}

		case "Servlet Session Manager":
			categoryPath = "servlet_session_manager"

		case "Dynamic Caching":
			categoryPath = "dynamic_caching"

		case "Transaction Manager":
			categoryPath = "transaction_manager"

		case "Security":
			// Include the security type (Authentication/Authorization)
			if len(cleanParts) > 1 {
				categoryPath = fmt.Sprintf("security.%s", sanitizeDimensionID(cleanParts[1]))
			} else {
				categoryPath = "security"
			}

		case "JVM Runtime":
			categoryPath = "jvm_runtime"

		case "ORB":
			categoryPath = "orb"

		default:
			// For unknown categories, use a simplified version
			categoryPath = sanitizeDimensionID(primaryCat)
		}
	} else {
		// Single-level path
		categoryPath = ""
	}

	return categoryPath, metricName
}

// containsServlets checks if the path contains servlet-related components
func (c *CorrelationEngine) containsServlets(parts []string) bool {
	for _, part := range parts {
		if strings.Contains(strings.ToLower(part), "servlet") {
			return true
		}
	}
	return false
}

// containsPortlets checks if the path contains portlet-related components
func (c *CorrelationEngine) containsPortlets(parts []string) bool {
	for _, part := range parts {
		if strings.Contains(strings.ToLower(part), "portlet") {
			return true
		}
	}
	return false
}

// determineProperUnit determines the correct unit for a metric based on its name and type
func (c *CorrelationEngine) determineProperUnit(metricName string, metric MetricTuple) string {
	name := strings.ToLower(metricName)

	// Pool/thread/connection counts
	if strings.Contains(name, "poolsize") || strings.Contains(name, "pool size") ||
		strings.Contains(name, "activecount") || strings.Contains(name, "active count") ||
		strings.Contains(name, "currentthreads") || strings.Contains(name, "peakthreads") ||
		strings.Contains(name, "daemonthreads") || strings.Contains(name, "maxpoolsize") ||
		strings.Contains(name, "connectioncount") || strings.Contains(name, "handlecount") {
		return "connections"
	}

	// Thread counts specifically
	if strings.Contains(name, "thread") && (strings.Contains(name, "count") ||
		strings.Contains(name, "hung") || strings.Contains(name, "cleared")) {
		return "threads"
	}

	// Session counts and related metrics
	if strings.Contains(name, "session") && (strings.Contains(name, "count") ||
		strings.Contains(name, "active") || strings.Contains(name, "invalidate") ||
		strings.Contains(name, "invalidation")) {
		return "sessions"
	}

	// Session object size
	if strings.Contains(name, "sessionobjectsize") || (strings.Contains(name, "session") && strings.Contains(name, "size")) {
		return "bytes"
	}

	// Time metrics (but not invalidation counts or other count metrics ending in "time")
	if metric.Type == "time" ||
		(strings.Contains(name, "time") && !strings.Contains(name, "timeoutinvalidation")) ||
		strings.Contains(name, "duration") || strings.Contains(name, "latency") {
		return "milliseconds"
	}

	// Count metrics that should be rates
	if strings.Contains(name, "createcount") || strings.Contains(name, "destroycount") ||
		strings.Contains(name, "allocatecount") || strings.Contains(name, "returncount") ||
		strings.Contains(name, "requestcount") || strings.Contains(name, "errorcount") ||
		strings.Contains(name, "faultcount") || strings.Contains(name, "discardcount") ||
		strings.Contains(name, "misscount") || strings.Contains(name, "hitcount") ||
		strings.Contains(name, "invalidationcount") || strings.Contains(name, "hitsinmemorycount") ||
		strings.Contains(name, "hitsondiskcount") {
		return "operations/s"
	}

	// Percentage metrics
	if strings.Contains(name, "percent") || strings.Contains(name, "utilization") {
		return "percent"
	}

	// Size metrics (but not cache entry counts)
	if strings.Contains(name, "size") && !strings.Contains(name, "pool") &&
		!strings.Contains(name, "cacheentrycount") {
		return "bytes"
	}

	// Cache entry counts
	if strings.Contains(name, "cacheentrycount") || strings.Contains(name, "entrycount") {
		return "entries"
	}

	// Memory metrics
	if strings.Contains(name, "memory") || strings.Contains(name, "heap") {
		return "bytes"
	}

	// Default to original unit
	return metric.Unit
}

// createNewGroup creates a new metric group
func (c *CorrelationEngine) createNewGroup(key string, metric MetricTuple) *MetricGroup {
	categoryPath, metricName := c.extractCategoryAndMetric(metric)
	properUnit := c.determineProperUnit(metricName, metric)

	// Extract common labels from the first metric
	commonLabels := make(map[string]string)
	for k, v := range metric.Labels {
		// Skip instance-specific labels
		if k != "instance" && k != "index" {
			commonLabels[k] = v
		}
	}

	return &MetricGroup{
		CorrelationKey: key,
		BaseContext:    c.generateChartContext(categoryPath, metricName),
		CommonLabels:   commonLabels,
		Metrics:        []MetricTuple{metric},
		Unit:           properUnit,
		Type:           c.inferChartType(metric),
		MetricName:     metricName,
		CategoryPath:   categoryPath,
	}
}

// generateChartContext creates the context for a chart based on category and metric
func (c *CorrelationEngine) generateChartContext(categoryPath, metricName string) string {
	// Clean up the category path
	categoryPath = strings.ToLower(strings.ReplaceAll(categoryPath, " ", "_"))
	categoryPath = strings.ReplaceAll(categoryPath, "/", ".")

	// Clean up the metric name
	metricName = strings.ToLower(strings.ReplaceAll(metricName, " ", "_"))

	// Build context: websphere_pmi.category.metric_name
	if categoryPath != "" {
		return fmt.Sprintf("websphere_pmi.%s.%s", categoryPath, metricName)
	}
	return fmt.Sprintf("websphere_pmi.%s", metricName)
}

// inferChartType determines the appropriate chart type
func (c *CorrelationEngine) inferChartType(metric MetricTuple) string {
	metricName := strings.ToLower(metric.Path)

	// Memory/size metrics should be stacked to show resource usage
	if strings.Contains(metricName, "memory") || strings.Contains(metricName, "heap") ||
		strings.Contains(metricName, "size") || metric.Unit == "bytes" {
		// Check if this looks like a pair (used/free, etc.)
		if strings.Contains(metricName, "used") || strings.Contains(metricName, "free") ||
			strings.Contains(metricName, "committed") {
			return "stacked"
		}
	}

	// Pool sizes should be stacked
	if strings.Contains(metricName, "poolsize") || strings.Contains(metricName, "pool size") {
		return "stacked"
	}

	// Bidirectional metrics (in/out, read/write) should be area
	if strings.Contains(metricName, "sent") || strings.Contains(metricName, "received") ||
		strings.Contains(metricName, "read") || strings.Contains(metricName, "write") ||
		strings.Contains(metricName, "input") || strings.Contains(metricName, "output") {
		return "area"
	}

	// Utilization/percentage metrics with potential for stacking
	if metric.Unit == "percent" || strings.Contains(metricName, "percent") {
		return "area"
	}

	// Default to line for independent metrics
	return "line"
}

// splitLargeGroup splits groups with too many dimensions
func (c *CorrelationEngine) splitLargeGroup(group *MetricGroup) []MetricGroup {
	// For now, we'll split by creating multiple charts with a suffix
	// This is a simple implementation - could be enhanced
	subGroups := make([]MetricGroup, 0)

	chunkSize := c.maxDimensionsPerChart
	for i := 0; i < len(group.Metrics); i += chunkSize {
		end := i + chunkSize
		if end > len(group.Metrics) {
			end = len(group.Metrics)
		}

		subGroup := MetricGroup{
			CorrelationKey: fmt.Sprintf("%s_%d", group.CorrelationKey, i/chunkSize),
			BaseContext:    fmt.Sprintf("%s_%d", group.BaseContext, i/chunkSize),
			CommonLabels:   group.CommonLabels,
			Metrics:        group.Metrics[i:end],
			Unit:           group.Unit,
			Type:           group.Type,
			MetricName:     group.MetricName,
			CategoryPath:   group.CategoryPath,
		}
		subGroups = append(subGroups, subGroup)
	}

	return subGroups
}

// createChartFromGroup converts a metric group to a chart candidate
func (c *CorrelationEngine) createChartFromGroup(group MetricGroup, priority int) ChartCandidate {
	// Generate dimensions - each instance becomes a dimension
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

// generateDimensionID creates a unique dimension ID for an instance
func (c *CorrelationEngine) generateDimensionID(metric MetricTuple) string {
	// For NIDL compliance, the dimension ID should identify the instance

	// First check explicit instance label
	if instance := metric.Labels["instance"]; instance != "" {
		return sanitizeDimensionID(instance)
	}

	// For array elements, use the element name
	if metric.IsArrayElement && metric.ElementName != "" {
		return sanitizeDimensionID(metric.ElementName)
	}

	// Extract instance from path based on category
	pathParts := strings.Split(strings.TrimPrefix(metric.Path, "server/"), "/")
	categoryPath, metricName := c.extractCategoryAndMetric(metric)

	// IMPORTANT: Never use the metric name as a dimension
	// This prevents issues like having both "PoolSize" and "Derby JDBC Provider (XA)" as dimensions
	if len(pathParts) > 0 && pathParts[len(pathParts)-1] == metricName {
		// Remove the metric name from consideration
		pathParts = pathParts[:len(pathParts)-1]
	}

	// Based on category, extract the appropriate instance identifier
	switch {
	case strings.HasPrefix(categoryPath, "thread_pools"):
		// For thread pools, instance is the pool name (e.g., "WebContainer")
		if len(pathParts) > 1 {
			return sanitizeDimensionID(pathParts[1])
		}

	case strings.HasPrefix(categoryPath, "jdbc_connection_pools"):
		// For JDBC, instance is the datasource name
		// Handle both:
		// - "JDBC Connection Pools/Derby JDBC Provider (XA)" -> "Derby JDBC Provider (XA)"
		// - "JDBC Connection Pools" -> skip, no valid instance
		if len(pathParts) > 1 && pathParts[0] == "JDBC Connection Pools" {
			// pathParts[1] should be the datasource name
			return sanitizeDimensionID(pathParts[1])
		} else if len(pathParts) == 1 && pathParts[0] == "JDBC Connection Pools" {
			// This is a category-level metric without a specific datasource
			// We'll use a generic identifier
			return "all_pools"
		}

	case strings.HasPrefix(categoryPath, "web_applications"):
		// For web apps, extract the app#war and servlet/portlet name
		for i, part := range pathParts {
			if strings.Contains(part, "#") && strings.Contains(part, ".war") {
				// Found the app, now look for servlet/portlet
				for j := i + 1; j < len(pathParts)-1; j++ {
					if pathParts[j] == "Servlets" && j+1 < len(pathParts)-1 {
						// Return app#war/servletname
						return sanitizeDimensionID(fmt.Sprintf("%s_%s", part, pathParts[j+1]))
					}
				}
				// Just the app if no servlet found
				return sanitizeDimensionID(part)
			}
		}
	}

	// Fallback: use a simplified version of the path
	// Note: pathParts already has the metric name removed
	if len(pathParts) > 1 {
		// Use the most specific instance identifier
		return sanitizeDimensionID(pathParts[len(pathParts)-1])
	} else if len(pathParts) == 1 {
		return sanitizeDimensionID(pathParts[0])
	}

	// Last resort - use "default"
	return "default"
}

// generateDimensionName creates a human-readable dimension name
func (c *CorrelationEngine) generateDimensionName(metric MetricTuple) string {
	// For NIDL compliance, the dimension name should be the instance name

	// First check explicit instance label
	if instance := metric.Labels["instance"]; instance != "" {
		return instance
	}

	// For array elements, use the element name
	if metric.IsArrayElement && metric.ElementName != "" {
		return metric.ElementName
	}

	// Extract a human-readable instance name from the path
	pathParts := strings.Split(strings.TrimPrefix(metric.Path, "server/"), "/")
	categoryPath, metricName := c.extractCategoryAndMetric(metric)

	// Remove the metric name from path parts if it's at the end
	if len(pathParts) > 0 && pathParts[len(pathParts)-1] == metricName {
		pathParts = pathParts[:len(pathParts)-1]
	}

	// Based on category, create appropriate display name
	switch {
	case strings.HasPrefix(categoryPath, "thread_pools"):
		if len(pathParts) > 1 {
			return pathParts[1] // Pool name
		}

	case strings.HasPrefix(categoryPath, "jdbc_connection_pools"):
		if len(pathParts) > 1 && pathParts[0] == "JDBC Connection Pools" {
			return pathParts[1] // Datasource name
		} else if len(pathParts) == 1 && pathParts[0] == "JDBC Connection Pools" {
			return "All Pools" // Category-level metric
		}

	case strings.HasPrefix(categoryPath, "web_applications"):
		// For web apps, show app and servlet/portlet
		for i, part := range pathParts {
			if strings.Contains(part, "#") && strings.Contains(part, ".war") {
				// Found the app
				appName := part

				// Look for servlet/portlet
				for j := i + 1; j < len(pathParts)-1; j++ {
					if pathParts[j] == "Servlets" && j+1 < len(pathParts)-1 {
						return fmt.Sprintf("%s - %s", appName, pathParts[j+1])
					}
				}
				return appName
			}
		}
	}

	// Fallback: create a readable name from path components
	// Note: pathParts already has the metric name removed
	if len(pathParts) > 1 {
		return pathParts[len(pathParts)-1]
	} else if len(pathParts) == 1 {
		return pathParts[0]
	}

	return "default"
}

// generateChartTitle creates a descriptive chart title
func (c *CorrelationEngine) generateChartTitle(group MetricGroup) string {
	// Build a clear title that describes what's being measured
	// Following NIDL: "What am I monitoring?"

	// Clean up category path for display
	categoryDisplay := strings.Title(strings.ReplaceAll(group.CategoryPath, "_", " "))
	categoryDisplay = strings.ReplaceAll(categoryDisplay, ".", " ")

	// Clean up metric name for display
	metricDisplay := c.formatMetricNameForDisplay(group.MetricName)

	// Build title that answers "What instances am I monitoring?"
	switch group.CategoryPath {
	case "thread_pools":
		return fmt.Sprintf("Thread Pools - %s", metricDisplay)
	case "jdbc_connection_pools":
		return fmt.Sprintf("JDBC Connection Pools - %s", metricDisplay)
	case "web_applications.servlets":
		return fmt.Sprintf("Servlet %s", metricDisplay)
	case "servlet_session_manager":
		return fmt.Sprintf("Session Manager %s", metricDisplay)
	default:
		if categoryDisplay != "" {
			return fmt.Sprintf("%s - %s", categoryDisplay, metricDisplay)
		}
		return metricDisplay
	}
}

// formatMetricNameForDisplay formats a metric name for human-readable display
func (c *CorrelationEngine) formatMetricNameForDisplay(metricName string) string {
	// Special cases for common patterns
	replacements := map[string]string{
		"ActiveCount":        "Active Connections",
		"CreateCount":        "Created",
		"DestroyCount":       "Destroyed",
		"PoolSize":           "Pool Size",
		"FreePoolSize":       "Free Pool Size",
		"MaxPoolSize":        "Maximum Pool Size",
		"UseTime":            "Use Time",
		"WaitTime":           "Wait Time",
		"ServiceTime":        "Service Time",
		"ActiveTime":         "Active Time",
		"PercentUsed":        "Percent Used",
		"PercentMaxed":       "Percent at Maximum",
		"RequestCount":       "Requests",
		"ErrorCount":         "Errors",
		"LoadedServletCount": "Loaded Servlets",
		"CurrentThreads":     "Current Threads",
		"PeakThreads":        "Peak Threads",
		"DaemonThreads":      "Daemon Threads",
	}

	if display, exists := replacements[metricName]; exists {
		return display
	}

	// Generic camelCase to title case conversion
	// Insert spaces before uppercase letters
	result := ""
	for i, r := range metricName {
		if i > 0 && 'A' <= r && r <= 'Z' {
			result += " "
		}
		result += string(r)
	}

	return strings.Title(result)
}

// generateChartFamily creates a chart family for grouping
func (c *CorrelationEngine) generateChartFamily(group MetricGroup) string {
	// Use the category as the family
	if group.CategoryPath != "" {
		// Take the first level of the category path
		parts := strings.Split(group.CategoryPath, "/")
		if len(parts) > 0 {
			return strings.Title(strings.ReplaceAll(parts[0], "_", " "))
		}
	}

	// Fallback to generic family
	return "WebSphere"
}

// isMetricName checks if a string looks like a metric name rather than an instance name
func isMetricName(name string) bool {
	// Common metric name patterns for WebSphere PMI
	metricPatterns := []string{
		"Count", "Size", "Time", "Percent", "Rate", "Ratio",
		"Total", "Average", "Mean", "Max", "Min", "Current",
		"Threads", "Memory", "Heap", "Sessions", "Requests",
		"Connections", "Pool", "Active", "Free", "Used",
	}

	// Check if the name ends with common metric suffixes
	nameLower := strings.ToLower(name)
	for _, pattern := range metricPatterns {
		if strings.HasSuffix(nameLower, strings.ToLower(pattern)) {
			return true
		}
	}

	// Check for exact matches of common metric names
	commonMetrics := map[string]bool{
		"PoolSize": true, "FreePoolSize": true, "CreateCount": true,
		"DestroyCount": true, "UseTime": true, "WaitTime": true,
		"ActiveCount": true, "AllocateCount": true, "ReturnCount": true,
		"ManagedConnectionCount": true, "ConnectionHandleCount": true,
	}

	return commonMetrics[name]
}

// sanitizeDimensionID removes or replaces invalid characters for Netdata dimension IDs
func sanitizeDimensionID(id string) string {
	// Replace spaces and other invalid characters with underscores
	id = strings.ReplaceAll(id, " ", "_")
	id = strings.ReplaceAll(id, "-", "_")
	id = strings.ReplaceAll(id, ".", "_")
	id = strings.ReplaceAll(id, "/", "_")
	id = strings.ReplaceAll(id, "\\", "_")
	id = strings.ReplaceAll(id, "(", "_")
	id = strings.ReplaceAll(id, ")", "_")
	id = strings.ReplaceAll(id, "[", "_")
	id = strings.ReplaceAll(id, "]", "_")
	id = strings.ReplaceAll(id, "{", "_")
	id = strings.ReplaceAll(id, "}", "_")
	id = strings.ReplaceAll(id, "#", "_")
	id = strings.ReplaceAll(id, "$", "_")
	id = strings.ReplaceAll(id, "%", "_")
	id = strings.ReplaceAll(id, "&", "_")
	id = strings.ReplaceAll(id, "*", "_")
	id = strings.ReplaceAll(id, "+", "_")
	id = strings.ReplaceAll(id, "=", "_")
	id = strings.ReplaceAll(id, ":", "_")
	id = strings.ReplaceAll(id, ";", "_")
	id = strings.ReplaceAll(id, "\"", "_")
	id = strings.ReplaceAll(id, "'", "_")
	id = strings.ReplaceAll(id, "<", "_")
	id = strings.ReplaceAll(id, ">", "_")
	id = strings.ReplaceAll(id, "?", "_")
	id = strings.ReplaceAll(id, "@", "_")
	id = strings.ReplaceAll(id, "^", "_")
	id = strings.ReplaceAll(id, "`", "_")
	id = strings.ReplaceAll(id, "|", "_")
	id = strings.ReplaceAll(id, "~", "_")

	// Convert to lowercase for consistency
	id = strings.ToLower(id)

	// Remove multiple consecutive underscores
	for strings.Contains(id, "__") {
		id = strings.ReplaceAll(id, "__", "_")
	}

	// Trim underscores from start and end
	id = strings.Trim(id, "_")

	// If empty after sanitization, return a default
	if id == "" {
		return "metric"
	}

	return id
}

// ConvertToNetdataCharts converts chart candidates to Netdata module charts
func (c *CorrelationEngine) ConvertToNetdataCharts(candidates []ChartCandidate) *module.Charts {
	charts := &module.Charts{}

	for _, candidate := range candidates {
		// Sanitize the chart ID to remove invalid characters
		chartID := sanitizeDimensionID(candidate.Context)

		chart := &module.Chart{
			ID:       chartID,
			Title:    candidate.Title,
			Units:    candidate.Units,
			Fam:      candidate.Family,
			Ctx:      candidate.Context,
			Type:     module.ChartType(candidate.Type),
			Priority: candidate.Priority,
		}

		// Convert labels
		if len(candidate.Labels) > 0 {
			chart.Labels = make([]module.Label, 0, len(candidate.Labels))
			for k, v := range candidate.Labels {
				chart.Labels = append(chart.Labels, module.Label{
					Key:   k,
					Value: v,
				})
			}
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
