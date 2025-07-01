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

// groupMetricsByCorrelation groups metrics using the new simple correlation approach
func (c *CorrelationEngine) groupMetricsByCorrelation(metrics []MetricTuple) []MetricGroup {
	groupMap := make(map[string]*MetricGroup)
	
	for _, metric := range metrics {
		key := c.generateCorrelationKey(metric)
		
		if group, exists := groupMap[key]; exists {
			// Add to existing group - no compatibility check needed since
			// the correlation key ensures proper grouping
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

// generateCorrelationKey creates a key for grouping related metrics using the new approach.
// This function strips the metric name from the unique instance to create intentional collisions,
// allowing related metrics to be grouped into the same chart.
func (c *CorrelationEngine) generateCorrelationKey(metric MetricTuple) string {
	// Start with the fully unique instance from the flattener
	uniqueInstance := metric.UniqueInstance
	
	// Strip the last 3 components (path.type.unit) to get the chart grouping key
	// Example: websphere_pmi.server_thread_pools_webcontainer_activecount.count.requests
	//       -> websphere_pmi.server_thread_pools_webcontainer
	parts := strings.Split(uniqueInstance, ".")
	
	if len(parts) >= 3 {
		// Remove last 3 parts (metric_name, type, unit)
		chartKey := strings.Join(parts[:len(parts)-3], ".")
		return chartKey
	}
	
	// Fallback: if structure is unexpected, use the full instance
	return uniqueInstance
}

// isKnownCategory checks if a path part represents a known category
// This is now simplified since we use natural hierarchy
func (c *CorrelationEngine) isKnownCategory(part string) bool {
	// We don't need a whitelist anymore - any first-level component
	// after "server" is a valid category
	return part != "server" && part != ""
}

// normalizeCategory converts category names to consistent identifiers
// This is now simplified - just replace / to prevent submenus
func (c *CorrelationEngine) normalizeCategory(category string) string {
	// Only replace / with _ to prevent UI from making submenus
	return strings.ReplaceAll(category, "/", "_")
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
	
	// If metrics have different instances, they should NOT be grouped together
	// Each instance should have its own chart
	if m1.Labels["instance"] != "" && m2.Labels["instance"] != "" && 
	   m1.Labels["instance"] != m2.Labels["instance"] {
		return false
	}
	
	// For metrics without instances, only group if they have the same semantic meaning
	// AND are from the same component
	family1 := c.extractMetricFamily(strings.Split(m1.Path, "/")[len(strings.Split(m1.Path, "/"))-1])
	family2 := c.extractMetricFamily(strings.Split(m2.Path, "/")[len(strings.Split(m2.Path, "/"))-1])
	
	// Check if paths indicate same component (not just same metric family)
	path1Parts := strings.Split(m1.Path, "/")
	path2Parts := strings.Split(m2.Path, "/")
	
	// If paths differ significantly, don't group
	if len(path1Parts) != len(path2Parts) {
		return false
	}
	
	// Compare paths up to the metric name
	for i := 0; i < len(path1Parts)-1; i++ {
		if path1Parts[i] != path2Parts[i] {
			return false
		}
	}
	
	return family1 == family2
}

// createNewGroup creates a new metric group
func (c *CorrelationEngine) createNewGroup(key string, metric MetricTuple) *MetricGroup {
	// Generate chart context by stripping metric name from unique context
	chartContext := c.generateChartContext(metric)
	
	return &MetricGroup{
		CorrelationKey: key,
		BaseContext:    chartContext,
		CommonLabels:   c.extractCommonLabels([]MetricTuple{metric}),
		Metrics:        []MetricTuple{metric},
		Unit:           metric.Unit,
		Type:           c.inferChartType(metric),
	}
}

// generateChartContext strips the metric name from the unique context to create chart context
func (c *CorrelationEngine) generateChartContext(metric MetricTuple) string {
	// Start with the fully unique context from the flattener
	uniqueContext := metric.UniqueContext
	
	// Strip the last component (metric name) to get the chart context
	// Example: websphere_pmi.server_thread_pools_webcontainer_activecount
	//       -> websphere_pmi.server_thread_pools_webcontainer
	parts := strings.Split(uniqueContext, ".")
	
	if len(parts) >= 2 {
		// Remove last part (metric name)
		chartContext := strings.Join(parts[:len(parts)-1], ".")
		return chartContext
	}
	
	// Fallback: if structure is unexpected, use the full context
	return uniqueContext
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
		Labels:     make(map[string]string),
	}
	
	// Add common labels
	for k, v := range group.CommonLabels {
		chart.Labels[k] = v
	}
	
	// Add instance label if all metrics share the same instance
	if len(group.Metrics) > 0 {
		firstInstance := group.Metrics[0].Labels["instance"]
		if firstInstance != "" {
			allSameInstance := true
			for _, m := range group.Metrics {
				if m.Labels["instance"] != firstInstance {
					allSameInstance = false
					break
				}
			}
			if allSameInstance {
				chart.Labels["instance"] = firstInstance
			}
		}
	}
	
	return chart
}

// generateDimensionID creates a unique dimension ID
func (c *CorrelationEngine) generateDimensionID(metric MetricTuple) string {
	// For metrics in a chart, we need truly unique IDs
	// Since each chart should now contain metrics from only one instance,
	// we can use just the metric name as the dimension ID
	
	// Extract metric name (last part of path)
	pathParts := strings.Split(metric.Path, "/")
	metricName := pathParts[len(pathParts)-1]
	
	// Sanitize and return
	return sanitizeDimensionID(metricName)
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
	// Start with the category
	titleParts := []string{}
	
	if category := group.CommonLabels["category"]; category != "" {
		titleParts = append(titleParts, strings.Title(strings.ReplaceAll(category, "_", " ")))
	}
	
	// Add subcategory if present
	if subcategory := group.CommonLabels["subcategory"]; subcategory != "" {
		titleParts = append(titleParts, strings.Title(strings.ReplaceAll(subcategory, "_", " ")))
	}
	
	// Add instance information in parentheses
	instanceParts := []string{}
	for _, label := range []string{"app", "servlet", "portlet", "pool", "datasource", "provider"} {
		if value := group.CommonLabels[label]; value != "" {
			instanceParts = append(instanceParts, value)
		}
	}
	
	// Add primary instance if not already included
	if instance := group.CommonLabels["instance"]; instance != "" {
		// Check if instance is not already in the instance parts
		hasInstance := false
		for _, part := range instanceParts {
			if part == instance {
				hasInstance = true
				break
			}
		}
		if !hasInstance {
			instanceParts = append(instanceParts, instance)
		}
	}
	
	if len(instanceParts) > 0 {
		titleParts = append(titleParts, fmt.Sprintf("(%s)", strings.Join(instanceParts, " - ")))
	}
	
	// Add metric type
	if len(group.Metrics) > 0 {
		metricName := strings.Split(group.Metrics[0].Path, "/")
		metricType := c.extractMetricFamily(metricName[len(metricName)-1])
		if metricType != "" {
			titleParts = append(titleParts, strings.Title(strings.ReplaceAll(metricType, "_", " ")))
		}
	}
	
	if len(titleParts) > 0 {
		return strings.Join(titleParts, " ")
	}
	return "WebSphere PMI Metrics"
}

// isMetricType checks if a string represents a known metric type pattern
func (c *CorrelationEngine) isMetricType(s string) bool {
	metricTypes := []string{
		"active_connections", "connection_lifecycle", "wait_times",
		"requests", "response_times", "errors", "memory", 
		"garbage_collection", "utilization", "metrics",
		"active_count", "size", "usage", "count", "time",
	}
	
	s = strings.ToLower(s)
	for _, mt := range metricTypes {
		if s == mt || strings.Contains(s, mt) {
			return true
		}
	}
	return false
}

// generateChartFamily creates a chart family for grouping using 2-level hierarchy
func (c *CorrelationEngine) generateChartFamily(group MetricGroup) string {
	// SAFETY FIRST: Always ensure charts are visible, even if family is wrong
	fallbackFamily := "websphere/server" // Safe fallback that's always visible
	
	if len(group.Metrics) == 0 {
		return fallbackFamily
	}
	
	// Get the first metric to extract category information
	firstMetric := group.Metrics[0]
	
	// Use category from metric labels (set by flattener based on natural XML hierarchy)
	if category, exists := firstMetric.Labels["category"]; exists && category != "" {
		// Category is already normalized by flattener
		baseFamily := category
		
		// Create 2-level family: "baseFamily/instance"
		if instance, hasInstance := firstMetric.Labels["instance"]; hasInstance && instance != "" {
			return fmt.Sprintf("%s/%s", baseFamily, c.cleanInstanceName(instance))
		}
		
		// Try to extract instance from path
		pathParts := strings.Split(firstMetric.Path, "/")
		if len(pathParts) > 2 {
			// Skip "server" and category, use next part as instance
			for i, part := range pathParts {
				if part == "server" {
					continue
				}
				// After category, the next part could be instance
				if i > 0 && strings.EqualFold(c.normalizeCategory(part), baseFamily) && i+1 < len(pathParts) {
					instance := pathParts[i+1]
					return fmt.Sprintf("%s/%s", baseFamily, c.cleanInstanceName(instance))
				}
			}
		}
		
		// No instance - use "general" subfamily
		return fmt.Sprintf("%s/general", baseFamily)
	}
	
	// SAFETY FALLBACK: Ensure chart is always visible
	c.debugLog("Using fallback family for group: %s", group.BaseContext)
	return fallbackFamily
}

// cleanInstanceName cleans instance names for use in family hierarchy
func (c *CorrelationEngine) cleanInstanceName(instance string) string {
	// Only replace / with _ to prevent UI from making submenus
	clean := strings.ReplaceAll(instance, "/", "_")
	
	// Ensure non-empty result
	if clean == "" {
		clean = "unknown"
	}
	
	return clean
}

// extractInstanceFromPath attempts to extract instance name from metric path
func (c *CorrelationEngine) extractInstanceFromPath(path string) string {
	parts := strings.Split(path, "/")
	
	// Look for instance patterns in path
	for i, part := range parts {
		// Skip category parts
		if c.isKnownCategory(part) {
			// Next part might be the instance
			if i+1 < len(parts) && !c.isKnownCategory(parts[i+1]) {
				return parts[i+1]
			}
		}
	}
	
	return ""
}

// debugLog outputs debug messages (only visible in development)
func (c *CorrelationEngine) debugLog(format string, args ...interface{}) {
	// Debug logging - not visible in production
	// Could be enhanced to use actual logger if available
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