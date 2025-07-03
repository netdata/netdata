// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

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
	Unit           string // Will be determined by first metric in group
	Type           string
	InstanceName   string // The instance being monitored (e.g., "WebContainer")
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
// NEW APPROACH: Group by instance, not by metric type
// This creates one chart per instance (e.g., per thread pool) with metrics as dimensions
func (c *CorrelationEngine) generateCorrelationKey(metric MetricTuple) string {
	// Extract the category path and metric name
	categoryPath, _ := c.extractCategoryAndMetric(metric)

	// Extract the instance identifier
	instanceID := c.extractInstanceIdentifier(metric)

	// If no category path, use "general" to avoid empty family
	if categoryPath == "" {
		categoryPath = "general"
	}

	// Create a key that groups metrics by instance
	// Format: category_path.instance_id
	return fmt.Sprintf("%s.%s",
		sanitizeDimensionID(categoryPath),
		sanitizeDimensionID(instanceID))
}

// extractInstanceIdentifier extracts the unique instance ID from a metric path
func (c *CorrelationEngine) extractInstanceIdentifier(metric MetricTuple) string {
	// Remove "server/" prefix if present
	cleanPath := strings.TrimPrefix(metric.Path, "server/")
	pathParts := strings.Split(cleanPath, "/")

	if len(pathParts) == 0 {
		return "default"
	}

	// The last part is the metric name, we need the instance before that
	metricName := pathParts[len(pathParts)-1]

	// First check explicit instance label
	if instance := metric.Labels["instance"]; instance != "" {
		return instance
	}

	// For array elements, use the element name
	if metric.IsArrayElement && metric.ElementName != "" {
		return metric.ElementName
	}

	// Extract based on the primary category
	if len(pathParts) > 1 {
		primaryCat := pathParts[0]

		switch primaryCat {
		case "Thread Pools":
			// For thread pools, instance is the pool name (e.g., "WebContainer")
			if len(pathParts) > 2 {
				return pathParts[1]
			}

		case "JDBC Connection Pools":
			// For JDBC, instance is the datasource name
			if len(pathParts) > 2 {
				return pathParts[1]
			}

		case "JCA Connection Pools":
			// For JCA, instance is after the category and adapter
			if len(pathParts) > 3 {
				return pathParts[2]
			} else if len(pathParts) > 2 {
				return pathParts[1]
			}

		case "ORB":
			// For ORB, instance is the interceptor name
			if len(pathParts) > 2 {
				return pathParts[1]
			}

		case "Web Applications":
			// For web apps, look for the app#war pattern
			for i, part := range pathParts {
				if strings.Contains(part, "#") && strings.Contains(part, ".war") {
					// If there's a servlet after, combine them
					if i+2 < len(pathParts) && pathParts[i+1] == "Servlets" && i+2 < len(pathParts)-1 {
						return fmt.Sprintf("%s.%s", part, pathParts[i+2])
					}
					return part
				}
			}

		case "Security Authentication":
			// For security auth, extract the auth type from metric name
			return c.ExtractAuthenticationType(metricName)
		}

		// Default: use the second-to-last part if it's not the metric name
		if len(pathParts) > 2 {
			candidate := pathParts[len(pathParts)-2]
			if !isCategoryName(candidate) {
				return candidate
			}
		}
	}

	return "default"
}

// extractCategoryAndMetric extracts the category path and metric name from a metric
func (c *CorrelationEngine) extractCategoryAndMetric(metric MetricTuple) (categoryPath, metricName string) {
	// Remove "server/" prefix if present
	cleanPath := strings.TrimPrefix(metric.Path, "server/")
	pathParts := strings.Split(cleanPath, "/")

	if len(pathParts) == 0 {
		return "", ""
	}

	// The last part is always the metric name
	metricName = pathParts[len(pathParts)-1]
	cleanParts := pathParts

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

		case "Security Authentication":
			// Handle "Security Authentication" as a single category
			categoryPath = "security.authentication"

		case "Security Authorization":
			// Handle "Security Authorization" as a single category
			categoryPath = "security.authorization"

		case "JVM Runtime":
			categoryPath = "jvm_runtime"

		case "ORB":
			categoryPath = "orb"

		case "System Data":
			categoryPath = "system_data"

		case "Alarm Manager":
			categoryPath = "alarm_manager"

		case "HAManager":
			categoryPath = "hamanager"

		case "Object Pool":
			categoryPath = "object_pool"

		case "Extension Registry Stats":
			categoryPath = "extension_registry"

		case "Async Beans":
			categoryPath = "async_beans"

		case "PMIWebServiceModule":
			categoryPath = "pmi_webservice"

		case "Data Collection":
			categoryPath = "data_collection"

		default:
			// For unknown categories, use a simplified version
			// But ensure it's not empty to avoid "-" family
			if primaryCat != "" {
				categoryPath = sanitizeDimensionID(primaryCat)
			} else {
				categoryPath = "general"
			}
		}
	} else {
		// Single-level path - try to determine category from metric name
		if strings.Contains(strings.ToLower(metricName), "jvm") ||
			strings.Contains(strings.ToLower(metricName), "heap") ||
			strings.Contains(strings.ToLower(metricName), "gc") ||
			strings.Contains(strings.ToLower(metricName), "uptime") {
			categoryPath = "jvm_runtime"
		} else if strings.Contains(strings.ToLower(metricName), "cpu") ||
			strings.Contains(strings.ToLower(metricName), "processor") ||
			strings.Contains(strings.ToLower(metricName), "load") {
			categoryPath = "system_data"
		} else {
			categoryPath = "general"
		}
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

// NormalizeAuthenticationMetric converts specific authentication metric names to general patterns
// for NIDL-compliant grouping. This groups all count metrics together and all time metrics together.
func (c *CorrelationEngine) NormalizeAuthenticationMetric(metricName string) string {
	name := strings.ToLower(metricName)

	// Group all authentication count metrics
	if strings.Contains(name, "count") {
		return "count"
	}

	// Group all authentication time metrics
	if strings.Contains(name, "time") {
		return "time"
	}

	// Keep other metrics as-is (e.g., CredentialCreationTime might be special)
	return metricName
}

// ExtractAuthenticationType extracts the authentication type from the metric name
// to use as the instance identifier (dimension ID)
func (c *CorrelationEngine) ExtractAuthenticationType(metricName string) string {
	name := strings.ToLower(metricName)

	// Map specific authentication metrics to their types
	switch {
	case strings.Contains(name, "basicauthentication"):
		return "basic"
	case strings.Contains(name, "tokenauthentication"):
		return "token"
	case strings.Contains(name, "webauthentication"):
		return "web"
	case strings.Contains(name, "rmiauthentication"):
		return "rmi"
	case strings.Contains(name, "jaasbasicauthentication"):
		return "jaas_basic"
	case strings.Contains(name, "jaastokenauthentication"):
		return "jaas_token"
	case strings.Contains(name, "jaasidentityassertion"):
		return "jaas_identity_assertion"
	case strings.Contains(name, "identityassertion"):
		return "identity_assertion"
	case strings.Contains(name, "tairequest"):
		return "tai_request"
	case strings.Contains(name, "credentialcreation"):
		return "credential_creation"
	default:
		// Fallback to the original metric name, sanitized
		return sanitizeDimensionID(metricName)
	}
}

// getAuthenticationTypeDisplayName provides human-readable names for authentication types
func (c *CorrelationEngine) getAuthenticationTypeDisplayName(authType string) string {
	switch authType {
	case "basic":
		return "Basic Authentication"
	case "token":
		return "Token Authentication"
	case "web":
		return "Web Authentication"
	case "rmi":
		return "RMI Authentication"
	case "jaas_basic":
		return "JAAS Basic Authentication"
	case "jaas_token":
		return "JAAS Token Authentication"
	case "jaas_identity_assertion":
		return "JAAS Identity Assertion"
	case "identity_assertion":
		return "Identity Assertion"
	case "tai_request":
		return "TAI Request"
	case "credential_creation":
		return "Credential Creation"
	default:
		// Fallback to title case of the auth type
		return strings.Title(strings.ReplaceAll(authType, "_", " "))
	}
}

// createNewGroup creates a new metric group
func (c *CorrelationEngine) createNewGroup(key string, metric MetricTuple) *MetricGroup {
	categoryPath, _ := c.extractCategoryAndMetric(metric)
	instanceID := c.extractInstanceIdentifier(metric)

	// Extract common labels from the first metric
	commonLabels := make(map[string]string)
	for k, v := range metric.Labels {
		// Skip instance-specific labels
		if k != "instance" && k != "index" {
			commonLabels[k] = v
		}
	}

	// Add the instance identifier as a label
	commonLabels["websphere_instance"] = instanceID

	// Generate context based on category and instance
	baseContext := c.generateInstanceContext(categoryPath, instanceID)

	// For the unit, we'll use the unit of the first metric added
	// Different metrics in the same instance might have different units
	// This will be handled during chart creation

	return &MetricGroup{
		CorrelationKey: key,
		BaseContext:    baseContext,
		CommonLabels:   commonLabels,
		Metrics:        []MetricTuple{metric},
		Unit:           metric.Unit, // Will be updated as we determine common unit
		Type:           "line",      // Default, will be refined
		InstanceName:   instanceID,
		CategoryPath:   categoryPath,
	}
}

// generateInstanceContext creates the context for a chart based on category and instance
func (c *CorrelationEngine) generateInstanceContext(categoryPath, instanceID string) string {
	// Clean up the category path
	categoryPath = strings.ToLower(strings.ReplaceAll(categoryPath, " ", "_"))
	categoryPath = strings.ReplaceAll(categoryPath, "/", ".")

	// Clean up the instance ID
	instanceID = sanitizeDimensionID(instanceID)

	// Build context: websphere_pmi.category.instance
	if categoryPath == "" || categoryPath == "general" {
		return fmt.Sprintf("websphere_pmi.%s", instanceID)
	}
	return fmt.Sprintf("websphere_pmi.%s.%s", categoryPath, instanceID)
}

// generateChartContext creates the context for a chart based on category and metric
func (c *CorrelationEngine) generateChartContext(categoryPath, metricName string) string {
	// Clean up the category path
	categoryPath = strings.ToLower(strings.ReplaceAll(categoryPath, " ", "_"))
	categoryPath = strings.ReplaceAll(categoryPath, "/", ".")

	// Clean up the metric name - convert to snake_case
	metricName = convertToSnakeCase(metricName)

	// Build context: websphere_pmi.category.metric_name
	// Always ensure we have a category to avoid empty families
	if categoryPath == "" || categoryPath == "general" {
		return fmt.Sprintf("websphere_pmi.%s", metricName)
	}
	return fmt.Sprintf("websphere_pmi.%s.%s", categoryPath, metricName)
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
			InstanceName:   group.InstanceName,
			CategoryPath:   group.CategoryPath,
		}
		subGroups = append(subGroups, subGroup)
	}

	return subGroups
}

// createChartFromGroup converts a metric group to a chart candidate
func (c *CorrelationEngine) createChartFromGroup(group MetricGroup, priority int) ChartCandidate {
	// NEW: Each metric in the group becomes a dimension
	// (e.g., ActiveCount, PoolSize, CreateCount for a thread pool)
	dimensions := make([]DimensionCandidate, 0, len(group.Metrics))

	// Group metrics with the same unit together
	// We may need to split this group if metrics have incompatible units
	unitGroups := make(map[string][]MetricTuple)
	for _, metric := range group.Metrics {
		_, metricName := c.extractCategoryAndMetric(metric)
		unit := c.determineProperUnit(metricName, metric)
		unitGroups[unit] = append(unitGroups[unit], metric)
	}

	// For now, use the most common unit
	// TODO: In the future, we might want to create separate charts for different units
	maxCount := 0
	primaryUnit := ""
	for unit, metrics := range unitGroups {
		if len(metrics) > maxCount {
			maxCount = len(metrics)
			primaryUnit = unit
		}
	}

	// Create dimensions from metrics with the primary unit
	for _, metric := range unitGroups[primaryUnit] {
		_, metricName := c.extractCategoryAndMetric(metric)
		dimensions = append(dimensions, DimensionCandidate{
			ID:     sanitizeDimensionID(metricName),
			Name:   c.humanizeMetricName(metricName),
			Metric: metric,
		})
	}

	// Generate chart properties
	family := c.generateChartFamily(group)
	title := c.generateInstanceTitle(group)

	chart := ChartCandidate{
		Context:    group.BaseContext,
		Title:      title,
		Units:      primaryUnit,
		Type:       c.inferChartTypeForInstance(dimensions),
		Family:     family,
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
		if len(pathParts) > 1 && pathParts[0] == "JDBC Connection Pools" {
			// pathParts[1] should be the datasource name
			return sanitizeDimensionID(pathParts[1])
		}
		// Check if we have labels that identify the instance
		if instance := metric.Labels["datasource"]; instance != "" {
			return sanitizeDimensionID(instance)
		}
		return "default"

	case strings.HasPrefix(categoryPath, "jca_connection_pools"):
		// For JCA, instance is usually after the category
		if len(pathParts) > 2 {
			return sanitizeDimensionID(pathParts[2])
		} else if len(pathParts) > 1 {
			return sanitizeDimensionID(pathParts[1])
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

	case strings.HasPrefix(categoryPath, "security.authentication"):
		// For Security Authentication, extract the authentication type from the metric name
		// Convert specific metrics like "BasicAuthenticationCount" to "basic"
		authType := c.ExtractAuthenticationType(metricName)
		return sanitizeDimensionID(authType)

	case strings.HasPrefix(categoryPath, "orb"):
		// For ORB metrics, each interceptor gets its own chart
		// So the dimension is just the metric name
		return sanitizeDimensionID(metricName)
	}

	// For array elements, always use the element name
	if metric.IsArrayElement && metric.ElementName != "" {
		return sanitizeDimensionID(metric.ElementName)
	}

	// Fallback: use instance from path, but not if it's the metric name
	if len(pathParts) > 1 {
		// Get the second-to-last part (last is metric name)
		candidate := pathParts[len(pathParts)-2]
		// Don't use category names as instances
		if !isCategoryName(candidate) {
			return sanitizeDimensionID(candidate)
		}
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
		}
		// Check labels
		if instance := metric.Labels["datasource"]; instance != "" {
			return instance
		}
		return "Default"

	case strings.HasPrefix(categoryPath, "jca_connection_pools"):
		if len(pathParts) > 2 {
			return pathParts[2]
		} else if len(pathParts) > 1 {
			return pathParts[1]
		}
		return "Default"

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

	case strings.HasPrefix(categoryPath, "security.authentication"):
		// For Security Authentication, provide human-readable authentication type names
		authType := c.ExtractAuthenticationType(metricName)
		return c.getAuthenticationTypeDisplayName(authType)

	case strings.HasPrefix(categoryPath, "orb"):
		// For ORB metrics, show the interceptor/component name
		if len(pathParts) > 2 && pathParts[0] == "ORB" {
			// Return the interceptor name as-is for readability
			return pathParts[1]
		}
	}

	// For array elements, always use the element name
	if metric.IsArrayElement && metric.ElementName != "" {
		return metric.ElementName
	}

	// Fallback: create a readable name from path components
	if len(pathParts) > 1 {
		candidate := pathParts[len(pathParts)-2]
		if !isCategoryName(candidate) {
			return candidate
		}
	}

	return "Default"
}

// humanizeMetricName converts a metric name to a human-readable dimension name
func (c *CorrelationEngine) humanizeMetricName(metricName string) string {
	// Convert CamelCase to Title Case
	result := ""
	for i, r := range metricName {
		if i > 0 && unicode.IsUpper(r) {
			result += " "
		}
		result += string(r)
	}
	return result
}

// generateInstanceTitle creates a title for an instance-based chart
func (c *CorrelationEngine) generateInstanceTitle(group MetricGroup) string {
	// Create a title based on the instance name and category
	categoryDisplay := c.getCategoryDisplayName(group.CategoryPath)

	// Clean up instance name for display
	instanceDisplay := group.InstanceName
	instanceDisplay = strings.ReplaceAll(instanceDisplay, "_", " ")
	instanceDisplay = strings.ReplaceAll(instanceDisplay, ".", " ")

	return fmt.Sprintf("%s - %s", categoryDisplay, instanceDisplay)
}

// getCategoryDisplayName returns a human-readable category name
func (c *CorrelationEngine) getCategoryDisplayName(categoryPath string) string {
	switch categoryPath {
	case "thread_pools":
		return "Thread Pool"
	case "jdbc_connection_pools":
		return "JDBC Pool"
	case "jca_connection_pools":
		return "JCA Pool"
	case "web_applications":
		return "Web Application"
	case "web_applications.servlets":
		return "Servlet"
	case "orb":
		return "ORB Interceptor"
	case "servlet_session_manager":
		return "Session Manager"
	case "dynamic_caching":
		return "Dynamic Cache"
	case "transaction_manager":
		return "Transaction Manager"
	case "security.authentication":
		return "Security Authentication"
	case "jvm_runtime":
		return "JVM Runtime"
	default:
		// Convert snake_case to Title Case
		parts := strings.Split(categoryPath, ".")
		if len(parts) > 0 {
			return strings.Title(strings.ReplaceAll(parts[0], "_", " "))
		}
		return "General"
	}
}

// inferChartTypeForInstance determines chart type based on the dimensions
func (c *CorrelationEngine) inferChartTypeForInstance(dimensions []DimensionCandidate) string {
	// If we have bidirectional metrics, use area
	hasIn := false
	hasOut := false
	hasRead := false
	hasWrite := false

	for _, dim := range dimensions {
		nameLower := strings.ToLower(dim.Name)
		if strings.Contains(nameLower, "in") || strings.Contains(nameLower, "received") {
			hasIn = true
		}
		if strings.Contains(nameLower, "out") || strings.Contains(nameLower, "sent") {
			hasOut = true
		}
		if strings.Contains(nameLower, "read") {
			hasRead = true
		}
		if strings.Contains(nameLower, "write") {
			hasWrite = true
		}
	}

	if (hasIn && hasOut) || (hasRead && hasWrite) {
		return "area"
	}

	// Default to line
	return "line"
}

// generateChartTitle creates a descriptive chart title
func (c *CorrelationEngine) generateChartTitle(group MetricGroup) string {
	// Build a clear title that describes what's being measured
	// Following NIDL: "What am I monitoring?"

	// This function is kept for compatibility but not used in instance-based grouping
	metricDisplay := "Metrics"

	// Build title that answers "What instances am I monitoring?"
	switch group.CategoryPath {
	case "thread_pools":
		return fmt.Sprintf("Thread Pools - %s", metricDisplay)
	case "jdbc_connection_pools":
		return fmt.Sprintf("JDBC Connection Pools - %s", metricDisplay)
	case "jca_connection_pools":
		return fmt.Sprintf("JCA Connection Pools - %s", metricDisplay)
	case "jca_connection_pools.sib_jms_resource_adapter":
		return fmt.Sprintf("SIB JMS Resource Adapter - %s", metricDisplay)
	case "web_applications":
		return fmt.Sprintf("Web Applications - %s", metricDisplay)
	case "web_applications.servlets":
		return fmt.Sprintf("Servlet %s", metricDisplay)
	case "web_applications.portlets":
		return fmt.Sprintf("Portlet %s", metricDisplay)
	case "servlet_session_manager":
		return fmt.Sprintf("Session Manager %s", metricDisplay)
	case "dynamic_caching":
		return fmt.Sprintf("Dynamic Caching %s", metricDisplay)
	case "transaction_manager":
		return fmt.Sprintf("Transaction Manager %s", metricDisplay)
	case "security.authentication":
		return fmt.Sprintf("Security Authentication - %s", metricDisplay)
	case "security.authorization":
		return fmt.Sprintf("Security Authorization - %s", metricDisplay)
	case "jvm_runtime":
		return fmt.Sprintf("JVM %s", metricDisplay)
	case "system_data":
		return fmt.Sprintf("System %s", metricDisplay)
	case "hamanager":
		return fmt.Sprintf("HA Manager %s", metricDisplay)
	case "object_pool":
		return fmt.Sprintf("Object Pool %s", metricDisplay)
	case "extension_registry":
		return fmt.Sprintf("Extension Registry %s", metricDisplay)
	case "orb":
		return fmt.Sprintf("ORB %s", metricDisplay)
	case "pmi_webservice":
		return fmt.Sprintf("PMI WebService %s", metricDisplay)
	case "data_collection":
		return fmt.Sprintf("Data Collection %s", metricDisplay)
	default:
		// Clean up category path for display
		categoryDisplay := strings.Title(strings.ReplaceAll(group.CategoryPath, "_", " "))
		categoryDisplay = strings.ReplaceAll(categoryDisplay, ".", " ")

		if categoryDisplay != "" && categoryDisplay != "General" {
			return fmt.Sprintf("%s - %s", categoryDisplay, metricDisplay)
		}
		return metricDisplay
	}
}

// formatMetricNameForDisplay formats a metric name for human-readable display
func (c *CorrelationEngine) formatMetricNameForDisplay(metricName string) string {
	// Special cases for common patterns
	replacements := map[string]string{
		// Thread pool metrics
		"ActiveCount":             "Active Count",
		"ActiveTime":              "Active Time",
		"PoolSize":                "Pool Size",
		"MaxPoolSize":             "Maximum Pool Size",
		"CreateCount":             "Create Count",
		"DestroyCount":            "Destroy Count",
		"ClearedThreadHangCount":  "Cleared Thread Hang Count",
		"DeclaredThreadHungCount": "Declared Thread Hung Count",
		"PercentUsed":             "Percent Used",
		"PercentMaxed":            "Percent Maxed",

		// Connection pool metrics
		"FreePoolSize":              "Free Pool Size",
		"UseTime":                   "Use Time",
		"WaitTime":                  "Wait Time",
		"AllocateCount":             "Allocate Count",
		"ReturnCount":               "Return Count",
		"FaultCount":                "Fault Count",
		"ConnectionHandleCount":     "Connection Handle Count",
		"ManagedConnectionCount":    "Managed Connection Count",
		"CloseCount":                "Close Count",
		"PrepStmtCacheDiscardCount": "Prepared Statement Cache Discard Count",
		"JDBCTime":                  "JDBC Time",
		"FreedCount":                "Freed Count",

		// JVM metrics
		"FreeMemory":      "Free Memory",
		"UsedMemory":      "Used Memory",
		"HeapSize":        "Heap Size",
		"Uptime":          "Uptime",
		"ProcessCPUUsage": "Process CPU Usage",
		"CurrentThreads":  "Current Threads",
		"PeakThreads":     "Peak Threads",
		"DaemonThreads":   "Daemon Threads",

		// Transaction metrics
		"CommittedCount":             "Committed Count",
		"RolledbackCount":            "Rolled Back Count",
		"TimeoutCount":               "Timeout Count",
		"OptimizationCount":          "Optimization Count",
		"GlobalBegunCount":           "Global Begun Count",
		"GlobalInvolvedCount":        "Global Involved Count",
		"LocalBegunCount":            "Local Begun Count",
		"GlobalTranTime":             "Global Transaction Time",
		"GlobalCommitTime":           "Global Commit Time",
		"GlobalPrepareTime":          "Global Prepare Time",
		"GlobalBeforeCompletionTime": "Global Before Completion Time",
		"LocalTranTime":              "Local Transaction Time",
		"LocalCommitTime":            "Local Commit Time",
		"LocalBeforeCompletionTime":  "Local Before Completion Time",
		"LocalActiveCount":           "Local Active Count",
		"LocalCommittedCount":        "Local Committed Count",
		"LocalRolledbackCount":       "Local Rolled Back Count",
		"LocalTimeoutCount":          "Local Timeout Count",

		// Web metrics
		"RequestCount":               "Request Count",
		"ServiceTime":                "Service Time",
		"ErrorCount":                 "Error Count",
		"LoadedServletCount":         "Loaded Servlet Count",
		"ReloadCount":                "Reload Count",
		"AsyncContext_Response_Time": "Async Context Response Time",
		"URIRequestCount":            "URI Request Count",
		"URIServiceTime":             "URI Service Time",

		// Session metrics
		"CreatedCount":                 "Created Count",
		"InvalidatedCount":             "Invalidated Count",
		"SessionObjectSize":            "Session Object Size",
		"ExternalReadTime":             "External Read Time",
		"ExternalWriteTime":            "External Write Time",
		"AffinityBreakCount":           "Affinity Break Count",
		"TimeoutInvalidationCount":     "Timeout Invalidation Count",
		"ActivateNonExistSessionCount": "Activate Non-Exist Session Count",
		"CacheDiscardCount":            "Cache Discard Count",
		"NoRoomForNewSessionCount":     "No Room For New Session Count",

		// Cache metrics
		"HitCount":                        "Hit Count",
		"MissCount":                       "Miss Count",
		"ExplicitInvalidationCount":       "Explicit Invalidation Count",
		"LruInvalidationCount":            "LRU Invalidation Count",
		"InMemoryCacheEntryCount":         "In Memory Cache Entry Count",
		"MaxInMemoryCacheEntryCount":      "Max In Memory Cache Entry Count",
		"HitsInMemoryCount":               "Hits In Memory Count",
		"HitsOnDiskCount":                 "Hits On Disk Count",
		"RemoteHitCount":                  "Remote Hit Count",
		"RemoteCreationCount":             "Remote Creation Count",
		"RemoteExplicitInvalidationCount": "Remote Explicit Invalidation Count",
		"LocalExplicitInvalidationCount":  "Local Explicit Invalidation Count",
		"InMemoryAndDiskCacheEntryCount":  "In Memory And Disk Cache Entry Count",
		"ClientRequestCount":              "Client Request Count",
		"DistributedRequestCount":         "Distributed Request Count",
		"ExplicitMemoryInvalidationCount": "Explicit Memory Invalidation Count",
		"ExplicitDiskInvalidationCount":   "Explicit Disk Invalidation Count",

		// Security metrics
		"BasicAuthenticationCount":     "Basic Authentication Count",
		"BasicAuthenticationTime":      "Basic Authentication Time",
		"TokenAuthenticationCount":     "Token Authentication Count",
		"TokenAuthenticationTime":      "Token Authentication Time",
		"IdentityAssertionCount":       "Identity Assertion Count",
		"IdentityAssertionTime":        "Identity Assertion Time",
		"WebAuthenticationCount":       "Web Authentication Count",
		"WebAuthenticationTime":        "Web Authentication Time",
		"RMIAuthenticationCount":       "RMI Authentication Count",
		"RMIAuthenticationTime":        "RMI Authentication Time",
		"TAIRequestCount":              "TAI Request Count",
		"TAIRequestTime":               "TAI Request Time",
		"CredentialCreationTime":       "Credential Creation Time",
		"JAASBasicAuthenticationCount": "JAAS Basic Authentication Count",
		"JAASBasicAuthenticationTime":  "JAAS Basic Authentication Time",
		"JAASTokenAuthenticationCount": "JAAS Token Authentication Count",
		"JAASTokenAuthenticationTime":  "JAAS Token Authentication Time",
		"JAASIdentityAssertionCount":   "JAAS Identity Assertion Count",
		"JAASIdentityAssertionTime":    "JAAS Identity Assertion Time",
		"WebAuthorizationTime":         "Web Authorization Time",
		"EJBAuthorizationTime":         "EJB Authorization Time",
		"AdminAuthorizationTime":       "Admin Authorization Time",
		"JACCAuthorizationTime":        "JACC Authorization Time",

		// System metrics
		"CPUUsageSinceLastMeasurement": "CPU Usage Since Last Measurement",

		// ORB metrics
		"LookupTime":     "Lookup Time",
		"ProcessingTime": "Processing Time",

		// Other metrics
		"LoadedCount":   "Loaded Count",
		"UnloadedCount": "Unloaded Count",
	}

	if display, exists := replacements[metricName]; exists {
		return display
	}

	// Generic camelCase to title case conversion
	// Insert spaces before uppercase letters
	result := ""
	for i, r := range metricName {
		if i > 0 && 'A' <= r && r <= 'Z' && i > 0 && metricName[i-1] >= 'a' && metricName[i-1] <= 'z' {
			result += " "
		}
		result += string(r)
	}

	return strings.Title(result)
}

// generateChartFamily creates a chart family for grouping
func (c *CorrelationEngine) generateChartFamily(group MetricGroup) string {
	// Use the category as the family
	if group.CategoryPath != "" && group.CategoryPath != "general" {
		// Take the first level of the category path
		parts := strings.Split(group.CategoryPath, ".")
		if len(parts) > 0 {
			// Convert to title case and replace underscores
			family := strings.ReplaceAll(parts[0], "_", " ")
			// Proper case for known families
			switch strings.ToLower(family) {
			case "jvm runtime":
				return "JVM Runtime"
			case "thread pools":
				return "Thread Pools"
			case "jdbc connection pools":
				return "JDBC Connection Pools"
			case "jca connection pools":
				return "JCA Connection Pools"
			case "web applications":
				return "Web Applications"
			case "servlet session manager":
				return "Session Manager"
			case "dynamic caching":
				return "Dynamic Caching"
			case "transaction manager":
				return "Transaction Manager"
			case "security":
				return "Security"
			case "system data":
				return "System"
			case "hamanager":
				return "HA Manager"
			case "object pool":
				return "Object Pool"
			case "extension registry":
				return "Extension Registry"
			case "orb":
				return "ORB"
			case "pmi webservice":
				return "PMI WebService"
			case "data collection":
				return "Data Collection"
			default:
				return strings.Title(family)
			}
		}
	}

	// Fallback to generic family
	return "General"
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

// isCategoryName checks if a string is a known category name
func isCategoryName(name string) bool {
	categories := map[string]bool{
		"Thread Pools": true, "JDBC Connection Pools": true, "JCA Connection Pools": true,
		"Web Applications": true, "Servlet Session Manager": true, "Dynamic Caching": true,
		"Transaction Manager": true, "Security": true, "JVM Runtime": true, "ORB": true,
		"System Data": true, "HAManager": true, "Object Pool": true, "Servlets": true,
		"Portlets": true, "Authentication": true, "Authorization": true,
		"Extension Registry Stats": true, "PMIWebServiceModule": true, "Data Collection": true,
		"Async Beans": true, "Alarm Manager": true,
	}

	return categories[name] || categories[strings.Title(name)]
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

// convertToSnakeCase converts camelCase or PascalCase to snake_case
func convertToSnakeCase(s string) string {
	// Handle some common patterns first
	s = strings.ReplaceAll(s, "CPU", "Cpu")
	s = strings.ReplaceAll(s, "JVM", "Jvm")
	s = strings.ReplaceAll(s, "JDBC", "Jdbc")
	s = strings.ReplaceAll(s, "JCA", "Jca")
	s = strings.ReplaceAll(s, "JMS", "Jms")
	s = strings.ReplaceAll(s, "EJB", "Ejb")
	s = strings.ReplaceAll(s, "URI", "Uri")
	s = strings.ReplaceAll(s, "URL", "Url")
	s = strings.ReplaceAll(s, "RMI", "Rmi")
	s = strings.ReplaceAll(s, "TAI", "Tai")
	s = strings.ReplaceAll(s, "JAAS", "Jaas")
	s = strings.ReplaceAll(s, "JACC", "Jacc")
	s = strings.ReplaceAll(s, "LRU", "Lru")
	s = strings.ReplaceAll(s, "ORB", "Orb")

	var result []rune
	for i, r := range s {
		if i > 0 && i < len(s)-1 &&
			unicode.IsUpper(r) &&
			(unicode.IsLower(rune(s[i-1])) ||
				(i+1 < len(s) && unicode.IsLower(rune(s[i+1])))) {
			result = append(result, '_')
		}
		result = append(result, unicode.ToLower(r))
	}

	// Clean up any double underscores
	res := string(result)
	for strings.Contains(res, "__") {
		res = strings.ReplaceAll(res, "__", "_")
	}

	return strings.Trim(res, "_")
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
