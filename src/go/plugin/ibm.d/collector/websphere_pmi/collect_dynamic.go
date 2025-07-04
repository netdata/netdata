// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// precision for floating point metric conversion
const precision = 1000

// collectDynamic implements direct NIDL-compliant collection without flattener/correlator
func (w *WebSpherePMI) collectDynamic(ctx context.Context, stats *pmiStatsResponse) map[string]int64 {
	mx := make(map[string]int64)

	// Track what we see in this collection cycle
	w.resetSeenTracking()

	// Process each node/server hierarchy
	for _, node := range stats.Nodes {
		for _, server := range node.Servers {
			w.collectServerMetrics(mx, node.Name, server.Name, server.Stats)
		}
	}

	// Process direct stats (for Liberty and other variants)
	if len(stats.Stats) > 0 {
		w.collectServerMetrics(mx, "local", "server", stats.Stats)
	}

	// Clean up charts for instances that are no longer present
	w.cleanupAbsentInstances()

	return mx
}

// resetSeenTracking resets the tracking for what we've seen this cycle
func (w *WebSpherePMI) resetSeenTracking() {
	if w.dynamicCollector == nil {
		w.dynamicCollector = NewDynamicCollector()
	}
	w.dynamicCollector.ResetSeen()
}

// collectServerMetrics processes stats for a specific server instance
func (w *WebSpherePMI) collectServerMetrics(mx map[string]int64, nodeName, serverName string, stats []pmiStat) {
	for _, stat := range stats {
		w.processStatEntry(mx, nodeName, serverName, "", stat)
	}
}

// processStatEntry recursively processes a PMI stat entry and its children
func (w *WebSpherePMI) processStatEntry(mx map[string]int64, nodeName, serverName, parentPath string, stat pmiStat) {
	currentPath := parentPath
	if currentPath != "" && stat.Name != "" {
		currentPath = currentPath + "." + stat.Name
	} else if stat.Name != "" {
		currentPath = stat.Name
	}

	// Check if this stat has sub-stats (indicating it's a container)
	if len(stat.SubStats) > 0 {
		// This is a container stat - process each sub-stat as individual instances
		w.Debugf("Processing container stat '%s' with %d sub-stats", stat.Name, len(stat.SubStats))

		for _, subStat := range stat.SubStats {
			// Process sub-stat recursively
			w.processStatEntry(mx, nodeName, serverName, currentPath, subStat)
		}

		// Also check if the container itself has metrics (some do)
		if w.hasDirectMetrics(stat) {
			w.extractMetricsFromStat(mx, nodeName, serverName, currentPath, stat)
		}
	} else {
		// This is a leaf stat with actual metrics - extract them
		w.extractMetricsFromStat(mx, nodeName, serverName, currentPath, stat)
	}
}

// hasDirectMetrics checks if a stat has direct metric values (not just sub-stats)
func (w *WebSpherePMI) hasDirectMetrics(stat pmiStat) bool {
	return len(stat.CountStatistics) > 0 ||
		len(stat.TimeStatistics) > 0 ||
		len(stat.RangeStatistics) > 0 ||
		len(stat.BoundedRangeStatistics) > 0 ||
		len(stat.DoubleStatistics) > 0 ||
		(stat.Value != nil && stat.Value.Value != "")
}

// extractMetricsFromStat extracts metrics from a single PMI stat entry
func (w *WebSpherePMI) extractMetricsFromStat(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// Determine the category based on both stat name and path context
	category := w.categorizeStatByContext(stat.Name, path)

	// Debug: log categorization
	w.Debugf("Extracting metrics: path='%s', stat.Name='%s', category='%s'", path, stat.Name, category)
	
	// Additional debug for missing categories
	if stat.Name == "pmiWebServiceModule" || stat.Name == "Object Pool" {
		w.Debugf("IMPORTANT: Found %s with %d sub-stats, categorized as '%s'", 
			stat.Name, len(stat.SubStats), category)
	}

	switch category {
	case "skip":
		// Skip this stat - it's a container that shouldn't be collected
		w.Debugf("Skipping container stat '%s'", stat.Name)
		return
	case "server":
		w.collectServerOverviewMetrics(mx, nodeName, serverName, path, stat)
	case "jvm":
		w.collectJVMMetrics(mx, nodeName, serverName, path, stat)
	case "web":
		w.collectWebMetrics(mx, nodeName, serverName, path, stat)
	case "portlet":
		w.collectPortletApplicationMetrics(mx, nodeName, serverName, path, stat)
	case "connections":
		w.collectConnectionPoolMetrics(mx, nodeName, serverName, path, stat)
	case "threading":
		w.collectThreadPoolMetrics(mx, nodeName, serverName, path, stat)
	case "transactions":
		w.collectTransactionMetrics(mx, nodeName, serverName, path, stat)
	case "security":
		w.collectSecurityMetrics(mx, nodeName, serverName, path, stat)
	case "messaging":
		w.collectMessagingMetrics(mx, nodeName, serverName, path, stat)
	case "caching":
		w.collectCacheMetrics(mx, nodeName, serverName, path, stat)
	case "hamanager":
		w.collectHAManagerMetrics(mx, nodeName, serverName, path, stat)
	case "webservice":
		w.collectWebServiceMetrics(mx, nodeName, serverName, path, stat)
	case "objectpool":
		w.collectObjectPoolMetrics(mx, nodeName, serverName, path, stat)
	case "system":
		w.collectSystemMetrics(mx, nodeName, serverName, path, stat)
	default:
		// Generic processing for unrecognized categories
		w.Debugf("Using generic processing for stat '%s' (category: %s)", stat.Name, category)
		w.collectGenericMetrics(mx, nodeName, serverName, path, stat)
	}
}

// categorizeStatByContext determines the category based on both stat name and path context
func (w *WebSpherePMI) categorizeStatByContext(statName, path string) string {
	pathLower := strings.ToLower(path)

	// Check path context first for nested stats
	if strings.Contains(pathLower, "thread pools") {
		return "threading"
	}
	if strings.Contains(pathLower, "servlet session manager") ||
		(strings.Contains(statName, "#") && strings.Contains(statName, ".war")) {
		return "web"
	}
	if strings.Contains(pathLower, "jdbc connection pools") {
		return "connections"
	}
	if strings.Contains(pathLower, "jca connection pools") {
		return "connections"
	}

	// Then fall back to name-based categorization
	return w.categorizeStatName(statName)
}

// categorizeStatName determines the category based on the PMI stat name and path context
func (w *WebSpherePMI) categorizeStatName(statName string) string {
	nameLower := strings.ToLower(statName)

	// Server overview metrics
	if statName == "server" || strings.Contains(nameLower, "extensionregistrystats") {
		return "server"
	}

	// Security metrics
	if strings.Contains(nameLower, "security authentication") ||
		strings.Contains(nameLower, "security authorization") {
		return "security"
	}

	// JVM metrics
	if strings.Contains(nameLower, "jvm runtime") {
		return "jvm"
	}
	
	// Object Pool metrics (separate from JVM)
	if strings.Contains(nameLower, "object pool") {
		return "objectpool"
	}

	// Portlet Application metrics (specific handling)
	if statName == "Portlet Application" {
		return "portlet"
	}

	// Web container metrics - now includes individual applications
	if strings.Contains(nameLower, "web applications") || strings.Contains(nameLower, "webcontainer") ||
		strings.Contains(nameLower, "servlets") || strings.Contains(nameLower, "servlet session manager") ||
		strings.Contains(nameLower, "urls") || strings.Contains(nameLower, "portlet") ||
		(strings.Contains(statName, "#") && strings.Contains(statName, ".war")) {
		return "web"
	}

	// Connection pool metrics
	if strings.Contains(nameLower, "jdbc connection pools") || strings.Contains(nameLower, "jca connection pools") {
		return "connections"
	}

	// Thread pool metrics - now includes individual named pools
	if strings.Contains(nameLower, "thread pools") || strings.Contains(nameLower, "threadpool") ||
		// Individual thread pool names from WebSphere
		nameLower == "default" || nameLower == "ariesthreadpool" || nameLower == "hamanager.thread.pool" ||
		nameLower == "message listener" || nameLower == "object request broker" ||
		strings.Contains(nameLower, "sibfap") || strings.Contains(nameLower, "soapconnector") ||
		strings.Contains(nameLower, "tcpchannel") || strings.Contains(nameLower, "wmqjca") {
		return "threading"
	}

	// HAManager metrics (not a thread pool)
	if strings.Contains(nameLower, "hamanager") && !strings.Contains(nameLower, ".thread.pool") {
		return "hamanager"
	}

	// Transaction metrics
	if strings.Contains(nameLower, "transaction manager") {
		return "transactions"
	}

	// Caching metrics
	if strings.Contains(nameLower, "dynamic caching") || 
		strings.HasPrefix(nameLower, "object:") {
		return "caching"
	}

	// Messaging/ORB metrics
	if strings.Contains(nameLower, "orb") || strings.Contains(nameLower, "interceptors") ||
		strings.Contains(nameLower, "sib") || strings.Contains(nameLower, "jms") {
		return "messaging"
	}

	// Web service metrics
	if strings.Contains(nameLower, "pmiwebservicemodule") {
		return "webservice"
	}

	// System data
	if strings.Contains(nameLower, "system data") {
		return "system"
	}

	// Skip known container names that shouldn't be collected
	if nameLower == "counters" || nameLower == "object cache" {
		return "skip"
	}

	return "other"
}

// collectServerOverviewMetrics handles server-level overview metrics
func (w *WebSpherePMI) collectServerOverviewMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	statNameLower := strings.ToLower(stat.Name)

	// Extension Registry metrics
	if strings.Contains(statNameLower, "extensionregistrystats") {
		w.ensureChartExists("websphere_pmi.server.extensions", "Server Extensions", "requests", "stacked", "server", 70000,
			[]string{"requests", "hits", "hit_rate"}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		// Chart ID is "server.extensions_{instance}" (without websphere_pmi prefix)
		chartID := fmt.Sprintf("server.extensions_%s", w.sanitizeForChartID(instanceName))
		// Use specialized extraction for server extensions
		w.extractServerExtensionMetrics(mx, chartID, stat)
		w.Debugf("Server extensions - extracted metrics for %s", instanceName)
	}

	// Server root metrics - only extract actual PMI data if available
	if stat.Name == "server" {
		// Only extract actual metrics if they exist - don't create synthetic status charts
		if len(stat.CountStatistics) > 0 || len(stat.BoundedRangeStatistics) > 0 {
			// We could create a chart for actual server metrics here if WebSphere provides them
			// For now, just log that we found server-level stats but don't extract them
			// since they would need proper context and dimension mapping
			w.Debugf("Server root stat found with %d CountStatistics, %d BoundedRangeStatistics - not extracted without proper mapping",
				len(stat.CountStatistics), len(stat.BoundedRangeStatistics))
		}
	}
}

// routeJVMMetrics routes JVM metrics to appropriate charts based on metric names
func (w *WebSpherePMI) routeJVMMetrics(mx map[string]int64, tempMx map[string]int64, memoryChartID, cpuChartID, uptimeChartID string) {
	for key, value := range tempMx {
		// Remove the "temp_" prefix to get the metric name
		metricName := strings.TrimPrefix(key, "temp_")

		// Route to appropriate chart
		if strings.Contains(metricName, "HeapSize") || strings.Contains(metricName, "Memory") {
			mx[fmt.Sprintf("%s_%s", memoryChartID, metricName)] = value
		} else if strings.Contains(metricName, "ProcessCpuUsage") {
			// Route to CPU chart with correct dimension name
			mx[fmt.Sprintf("%s_cpu", cpuChartID)] = value
		} else if strings.Contains(metricName, "UpTime") {
			// Route to uptime chart with correct dimension name
			mx[fmt.Sprintf("%s_uptime", uptimeChartID)] = value
		}
	}
}

// collectJVMMetrics handles JVM-related metrics
func (w *WebSpherePMI) collectJVMMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	statNameLower := strings.ToLower(stat.Name)

	// JVM Runtime metrics
	if strings.Contains(statNameLower, "jvm runtime") {
		// Create charts based on the actual metrics present
		heapDimensions := []string{"HeapSize_current", "FreeMemory", "UsedMemory"}
		w.ensureChartExists("websphere_pmi.jvm.memory", "JVM Memory", "KB", "stacked", "jvm/memory", 70100,
			heapDimensions, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		// CPU usage
		cpuDimensions := []string{"cpu"}
		w.ensureChartExists("websphere_pmi.jvm_process_cpu", "JVM Process CPU Usage", "percentage", "line", "jvm/cpu", 70101,
			cpuDimensions, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		// Uptime
		uptimeDimensions := []string{"uptime"}
		w.ensureChartExists("websphere_pmi.jvm_uptime", "JVM Uptime", "seconds", "line", "jvm/uptime", 70103,
			uptimeDimensions, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		// Extract for all charts
		memoryChartID := fmt.Sprintf("jvm.memory_%s", w.sanitizeForChartID(instanceName))
		cpuChartID := fmt.Sprintf("jvm_process_cpu_%s", w.sanitizeForChartID(instanceName))
		uptimeChartID := fmt.Sprintf("jvm_uptime_%s", w.sanitizeForChartID(instanceName))

		// Extract all metrics first
		tempMx := make(map[string]int64)
		extracted := w.extractStatValues(tempMx, "temp", stat)

		// Route metrics to appropriate charts
		w.routeJVMMetrics(mx, tempMx, memoryChartID, cpuChartID, uptimeChartID)
		w.Debugf("JVM Runtime - extracted %d metrics for %s", extracted, instanceName)
	}

	// Object Pool metrics
	if strings.Contains(statNameLower, "object pool") {
		// Object pool dimensions based on actual XML
		poolDimensions := []string{"ObjectsCreatedCount", "ObjectsAllocatedCount_current", "ObjectsReturnedCount_current", "IdleObjectsSize_current"}
		w.ensureChartExists("websphere_pmi.jvm.object_pools", "JVM Object Pools", "objects", "line", "jvm/object_pools", 70102,
			poolDimensions, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		chartID := fmt.Sprintf("jvm.object_pools_%s", w.sanitizeForChartID(instanceName))
		extracted := w.extractStatValues(mx, chartID, stat)
		w.Debugf("JVM Object Pool - extracted %d metrics for %s", extracted, instanceName)
	}
}

// collectWebMetrics handles web container metrics
func (w *WebSpherePMI) collectWebMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// Debug: log what we're processing
	w.Debugf("Web metrics processing: path='%s', stat.Name='%s', subStats=%d", path, stat.Name, len(stat.SubStats))
	
	// Special debugging for ISCAdminPortlet
	if strings.Contains(stat.Name, "ISCAdminPortlet") {
		w.Debugf("SPECIAL: ISCAdminPortlet entry - %d CountStats, %d SubStats", len(stat.CountStatistics), len(stat.SubStats))
		if len(stat.SubStats) > 0 {
			w.Debugf("SPECIAL: ISCAdminPortlet SubStats:")
			for i, sub := range stat.SubStats {
				w.Debugf("  [%d] %s", i, sub.Name)
			}
		}
	}

	// Check if this is a session metric for a specific web application
	// Pattern: isclite#isclite.war, perfServletApp#perfServletApp.war
	if strings.Contains(stat.Name, "#") && strings.Contains(stat.Name, ".war") {
		appName := stat.Name // Use the full application identifier
		instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName))
		instanceLabels := map[string]string{
			"node":        nodeName,
			"server":      serverName,
			"application": appName,
		}

		w.Debugf("Found web application session metrics: app='%s'", appName)
		
		// Extract servlet container metrics at the web application level
		w.collectServletContainerMetrics(mx, nodeName, serverName, appName, stat)

		// Chart 1: Session Lifecycle (creation, invalidation rates)
		w.ensureChartExistsWithDims("websphere_pmi.web.sessions_lifecycle", "Web Application Session Lifecycle", "sessions/s", "line", "web/sessions", 70500,
			[]DimensionConfig{
				{Name: "created", Algo: module.Incremental},
				{Name: "invalidated", Algo: module.Incremental},
				{Name: "timeout_invalidated", Algo: module.Incremental},
			}, instanceName, instanceLabels)

		// Chart 2: Active Sessions (current counts)
		w.ensureChartExists("websphere_pmi.web.sessions_active", "Web Application Active Sessions", "sessions", "line", "web/sessions", 70501,
			[]string{"active", "live"}, instanceName, instanceLabels)

		// Chart 3: Session Times (lifetimes and activation times)
		w.ensureChartExists("websphere_pmi.web.sessions_time", "Web Application Session Times", "milliseconds", "line", "web/sessions", 70502,
			[]string{"lifetime", "time_since_activated"}, instanceName, instanceLabels)

		// Chart 4: Cache Management (cache-related issues)
		w.ensureChartExists("websphere_pmi.web.sessions_cache", "Web Application Session Cache", "events", "line", "web/sessions", 70503,
			[]string{"cache_discarded", "no_room_for_new"}, instanceName, instanceLabels)

		// Chart 5: External Storage Performance
		w.ensureChartExists("websphere_pmi.web.sessions_external_time", "Web Application Session External Storage Time", "milliseconds", "line", "web/sessions", 70504,
			[]string{"external_read_time", "external_write_time"}, instanceName, instanceLabels)

		// Chart 6: External Storage Size
		w.ensureChartExists("websphere_pmi.web.sessions_external_size", "Web Application Session External Storage Size", "bytes", "line", "web/sessions", 70505,
			[]string{"external_read_size", "external_write_size"}, instanceName, instanceLabels)

		// Chart 7: Session Health (affinity breaks, non-existent activations, object size)
		w.ensureChartExists("websphere_pmi.web.sessions_health", "Web Application Session Health", "events", "line", "web/sessions", 70506,
			[]string{"affinity_breaks", "activate_nonexist"}, instanceName, instanceLabels)

		// Chart 8: Session Object Size
		w.ensureChartExists("websphere_pmi.web.sessions_object_size", "Web Application Session Object Size", "bytes", "line", "web/sessions", 70507,
			[]string{"object_size"}, instanceName, instanceLabels)

		// Chart IDs for each chart (without websphere_pmi prefix)
		lifecycleChartID := fmt.Sprintf("web.sessions_lifecycle_%s", w.sanitizeForChartID(instanceName))
		activeChartID := fmt.Sprintf("web.sessions_active_%s", w.sanitizeForChartID(instanceName))
		timeChartID := fmt.Sprintf("web.sessions_time_%s", w.sanitizeForChartID(instanceName))
		cacheChartID := fmt.Sprintf("web.sessions_cache_%s", w.sanitizeForChartID(instanceName))
		externalTimeChartID := fmt.Sprintf("web.sessions_external_time_%s", w.sanitizeForChartID(instanceName))
		externalSizeChartID := fmt.Sprintf("web.sessions_external_size_%s", w.sanitizeForChartID(instanceName))
		healthChartID := fmt.Sprintf("web.sessions_health_%s", w.sanitizeForChartID(instanceName))
		objectSizeChartID := fmt.Sprintf("web.sessions_object_size_%s", w.sanitizeForChartID(instanceName))

		// Extract all session metrics into mx with proper dimension IDs
		w.extractWebSessionMetricsToCharts(mx, lifecycleChartID, activeChartID, timeChartID, cacheChartID,
			externalTimeChartID, externalSizeChartID, healthChartID, objectSizeChartID, stat)

		w.Debugf("Web application '%s' - extracted session metrics for 8 charts", appName)
		
		// Also check for portlet metrics in this web application
		w.collectPortletMetrics(mx, nodeName, serverName, appName, stat)
	}

	// Check for servlet-specific metrics (if path indicates servlet)
	pathLower := strings.ToLower(path)
	if strings.Contains(pathLower, "servlet") {
		servletName := w.extractServletName(path)
		if servletName != "" && w.shouldCollectServlet(servletName) {
			instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, servletName)

			w.ensureChartExists("websphere_pmi.web.servlet.requests", "Servlet Requests", "requests/s", "stacked", "web/servlets", 70400,
				[]string{"requests", "errors"}, instanceName, map[string]string{
					"node":    nodeName,
					"server":  serverName,
					"servlet": servletName,
				})

			chartID := fmt.Sprintf("web.servlet.requests_%s", w.sanitizeForChartID(instanceName))
			extracted := w.extractStatValues(mx, chartID, stat)
			w.Debugf("Servlet '%s' - extracted %d metrics", servletName, extracted)
		}
	}

	// Process sub-stats to find URLs and Servlets
	for _, subStat := range stat.SubStats {
		// Check if this is a "Servlets" container
		if subStat.Name == "Servlets" {
			// Extract app name from the stat name if it contains #
			appName := ""
			if strings.Contains(stat.Name, "#") && strings.Contains(stat.Name, ".war") {
				appName = stat.Name
			}
			w.processServletsContainer(mx, nodeName, serverName, appName, subStat)
		} else if subStat.Name == "URLs" {
			// If we encounter a URLs container directly, process it
			// This would be unusual - URLs should be under servlets
			w.Debugf("Found URLs container directly under %s", stat.Name)
		}
	}
}

// collectPortletMetrics handles portlet metrics for a web application
func (w *WebSpherePMI) collectPortletMetrics(mx map[string]int64, nodeName, serverName, appName string, stat pmiStat) {
	// Use recursive search to find portlet metrics anywhere in the stat hierarchy
	w.findAndProcessPortlets(mx, nodeName, serverName, appName, stat)
}

// findAndProcessPortlets recursively searches for portlet metrics in the stat hierarchy
func (w *WebSpherePMI) findAndProcessPortlets(mx map[string]int64, nodeName, serverName, appName string, stat pmiStat) {
	// Check if this stat itself is a portlet
	if w.isIndividualPortletStat(stat) {
		w.collectIndividualPortletMetrics(mx, nodeName, serverName, appName, stat.Name, stat)
		return
	}
	
	// Check if this stat contains portlet metrics directly
	if w.hasPortletMetrics(stat) {
		w.collectIndividualPortletMetrics(mx, nodeName, serverName, appName, stat.Name, stat)
	}
	
	// Recursively search sub-stats
	for _, subStat := range stat.SubStats {
		w.findAndProcessPortlets(mx, nodeName, serverName, appName, subStat)
	}
}

// isIndividualPortletStat checks if a stat represents an individual portlet
func (w *WebSpherePMI) isIndividualPortletStat(stat pmiStat) bool {
	// A portlet stat should have specific portlet metrics
	for _, cs := range stat.CountStatistics {
		if cs.Name == "Number of portlet requests" || cs.Name == "Number of portlet errors" {
			return true
		}
	}
	
	for _, rs := range stat.RangeStatistics {
		if rs.Name == "Number of concurrent portlet requests" {
			return true
		}
	}
	
	for _, ts := range stat.TimeStatistics {
		if strings.Contains(ts.Name, "portlet") {
			return true
		}
	}
	
	return false
}

// hasPortletMetrics checks if a stat contains any portlet-related metrics
func (w *WebSpherePMI) hasPortletMetrics(stat pmiStat) bool {
	return w.isIndividualPortletStat(stat)
}

// collectIndividualPortletMetrics handles metrics for a single portlet
func (w *WebSpherePMI) collectIndividualPortletMetrics(mx map[string]int64, nodeName, serverName, appName, portletName string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName), w.sanitizeForMetricName(portletName))
	instanceLabels := map[string]string{
		"node":        nodeName,
		"server":      serverName,
		"application": appName,
		"portlet":     portletName,
	}

	w.Debugf("Processing portlet '%s' in application '%s'", portletName, appName)

	// Chart 1: Portlet Requests 
	w.ensureChartExistsWithDims("websphere_pmi.web.portlet_requests", "Portlet Requests", "requests/s", "line", "web/portlets", 70600,
		[]DimensionConfig{
			{Name: "requests", Algo: module.Incremental},
			{Name: "errors", Algo: module.Incremental},
		}, instanceName, instanceLabels)

	// Chart 2: Portlet Concurrent Requests
	w.ensureChartExists("websphere_pmi.web.portlet_concurrent", "Portlet Concurrent Requests", "requests", "line", "web/portlets", 70601,
		[]string{"concurrent_requests"}, instanceName, instanceLabels)

	// Chart 3: Portlet Response Times
	w.ensureChartExists("websphere_pmi.web.portlet_response_time", "Portlet Response Times", "milliseconds", "line", "web/portlets", 70602,
		[]string{"render_time", "action_time", "event_time", "resource_time"}, instanceName, instanceLabels)

	// Chart IDs for each chart (without websphere_pmi prefix)
	requestsChartID := fmt.Sprintf("web.portlet_requests_%s", w.sanitizeForChartID(instanceName))
	concurrentChartID := fmt.Sprintf("web.portlet_concurrent_%s", w.sanitizeForChartID(instanceName))
	responseTimeChartID := fmt.Sprintf("web.portlet_response_time_%s", w.sanitizeForChartID(instanceName))

	// Extract all portlet metrics into mx with proper dimension IDs
	w.extractPortletMetricsToCharts(mx, requestsChartID, concurrentChartID, responseTimeChartID, stat)

	w.Debugf("Portlet '%s' in application '%s' - extracted metrics for 3 charts", portletName, appName)
}

// extractPortletMetricsToCharts extracts portlet metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractPortletMetricsToCharts(mx map[string]int64, requestsChartID, concurrentChartID, responseTimeChartID string, stat pmiStat) {
	// Extract from CountStatistics (Number of portlet requests, Number of portlet errors)
	for _, cs := range stat.CountStatistics {
		var chartID, dimensionName string
		switch cs.Name {
		case "Number of portlet requests":
			chartID = requestsChartID
			dimensionName = "requests"
		case "Number of portlet errors":
			chartID = requestsChartID
			dimensionName = "errors"
		default:
			w.Debugf("Portlet: skipping CountStatistic '%s' (value: %s)", cs.Name, cs.Count)
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Portlet metric: %s = %d (from CountStatistic %s)", metricKey, val, cs.Name)
			}
		}
	}

	// Extract from RangeStatistics (Number of concurrent portlet requests)
	for _, rs := range stat.RangeStatistics {
		var chartID, dimensionName string
		switch rs.Name {
		case "Number of concurrent portlet requests":
			chartID = concurrentChartID
			dimensionName = "concurrent_requests"
		default:
			w.Debugf("Portlet: skipping RangeStatistic '%s' (value: %s)", rs.Name, rs.Current)
			continue
		}

		if rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Portlet metric: %s = %d (from RangeStatistic %s)", metricKey, val, rs.Name)
			}
		}
	}

	// Extract from TimeStatistics (Response times for different portlet operations)
	for _, ts := range stat.TimeStatistics {
		var chartID, dimensionName string
		switch ts.Name {
		case "Response time of portlet render":
			chartID = responseTimeChartID
			dimensionName = "render_time"
		case "Response time of portlet action":
			chartID = responseTimeChartID
			dimensionName = "action_time"
		case "Response time of a portlet processEvent request":
			chartID = responseTimeChartID
			dimensionName = "event_time"
		case "Response time of a portlet serveResource request":
			chartID = responseTimeChartID
			dimensionName = "resource_time"
		default:
			w.Debugf("Portlet: skipping TimeStatistic '%s'", ts.Name)
			continue
		}

		// Use totalTime for lifetime total response time
		totalTimeStr := ts.TotalTime
		if totalTimeStr == "" {
			totalTimeStr = ts.Total // Fallback for other versions
		}

		if totalTimeStr != "" {
			if val, err := strconv.ParseInt(totalTimeStr, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Portlet metric: %s = %d (from TimeStatistic %s.totalTime)", metricKey, val, ts.Name)
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
}

// collectPortletApplicationMetrics handles direct Portlet Application stats
func (w *WebSpherePMI) collectPortletApplicationMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	w.Debugf("Direct Portlet Application processing: path='%s', SubStats=%d", path, len(stat.SubStats))
	
	// Extract the web application name from the path context
	// The path should contain the parent web application information
	appName := w.extractPortletAppNameFromPath(path)
	if appName == "" {
		w.Debugf("Could not extract application name from path: %s", path)
		return
	}
	
	w.Debugf("Processing Portlet Application for app: %s", appName)
	
	// Look for "Portlets" sub-stat
	for _, subStat := range stat.SubStats {
		if subStat.Name == "Portlets" {
			w.Debugf("Found Portlets container in app '%s' with %d portlets", appName, len(subStat.SubStats))
			
			// Process each individual portlet
			for _, portletStat := range subStat.SubStats {
				if portletStat.Name != "" {
					w.collectIndividualPortletMetrics(mx, nodeName, serverName, appName, portletStat.Name, portletStat)
				}
			}
		}
	}
}

// extractPortletAppNameFromPath extracts the web application name from the path context
func (w *WebSpherePMI) extractPortletAppNameFromPath(path string) string {
	// The path structure should contain the parent application context
	// We need to look for patterns that indicate the parent web application
	
	// Try to extract from various path patterns
	pathLower := strings.ToLower(path)
	
	// Look for common application patterns in the path
	if strings.Contains(pathLower, "isclite") {
		if strings.Contains(pathLower, "iscadminportlet") {
			return "isclite#ISCAdminPortlet.war"
		} else if strings.Contains(pathLower, "wimportlet") {
			return "isclite#WIMPortlet.war"
		} else if strings.Contains(pathLower, "wasportlet") {
			return "isclite#wasportlet.war"
		}
	}
	
	// If we can't extract from path, we might need to get it from context
	// For now, return empty string to indicate we couldn't determine the app
	return ""
}

// collectConnectionPoolMetrics handles connection pool metrics
func (w *WebSpherePMI) collectConnectionPoolMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	pathLower := strings.ToLower(path)

	// JDBC pool metrics
	if strings.Contains(pathLower, "jdbc") || strings.Contains(pathLower, "datasource") {
		poolName := w.extractPoolName(path)
		if poolName != "" && w.shouldCollectPool(poolName) {
			instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)

			w.ensureChartExists("websphere_pmi.connections.jdbc", "JDBC Connection Pool", "connections", "stacked", "connections/jdbc", 70600,
				[]string{"active", "free", "total"}, instanceName, map[string]string{
					"node":   nodeName,
					"server": serverName,
					"pool":   poolName,
				})

			chartID := fmt.Sprintf("connections.jdbc_%s", w.sanitizeForChartID(instanceName))
			w.extractConnectionPoolMetrics(mx, chartID, stat)
		}
	}

	// JCA pool metrics
	if strings.Contains(pathLower, "jca") {
		poolName := w.extractPoolName(path)
		if poolName != "" && w.shouldCollectPool(poolName) {
			instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)

			w.ensureChartExists("websphere_pmi.connections.jca", "JCA Connection Pool", "connections", "stacked", "connections/jca", 70700,
				[]string{"active", "free", "total"}, instanceName, map[string]string{
					"node":   nodeName,
					"server": serverName,
					"pool":   poolName,
				})

			chartID := fmt.Sprintf("connections.jca_%s", w.sanitizeForChartID(instanceName))
			w.extractConnectionPoolMetrics(mx, chartID, stat)
		}
	}
}

// collectThreadPoolMetrics handles thread pool metrics
func (w *WebSpherePMI) collectThreadPoolMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// For thread pools, the stat name IS the pool name when it's a leaf
	poolName := stat.Name

	// Debug: log the path and pool name
	w.Debugf("Thread pool processing: path='%s', stat.Name='%s', poolName='%s'", path, stat.Name, poolName)

	// Skip the parent "Thread Pools" container - it's not an actual pool
	if poolName == "Thread Pools" {
		w.Debugf("Skipping parent container 'Thread Pools'")
		return
	}

	if poolName != "" && w.shouldCollectThreadPool(poolName) {
		instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)
		instanceLabels := map[string]string{
			"node":   nodeName,
			"server": serverName,
			"pool":   poolName,
		}

		// Chart 1: Thread Pool Usage (current active threads vs capacity)
		w.ensureChartExists("websphere_pmi.threading.pools_usage", "Thread Pool Usage", "threads", "stacked", "threading/pools", 70800,
			[]string{"active", "pool_size", "maximum_size"}, instanceName, instanceLabels)

		// Chart 2: Thread Pool Lifecycle (creation and destruction rates)
		w.ensureChartExistsWithDims("websphere_pmi.threading.pools_lifecycle", "Thread Pool Lifecycle", "threads/s", "line", "threading/pools", 70801,
			[]DimensionConfig{
				{Name: "created", Algo: module.Incremental},
				{Name: "destroyed", Algo: module.Incremental},
			}, instanceName, instanceLabels)

		// Chart 3: Thread Pool Health (hung thread monitoring)
		w.ensureChartExists("websphere_pmi.threading.pools_health", "Thread Pool Health", "threads", "line", "threading/pools", 70802,
			[]string{"declared_hung", "cleared_hung", "concurrent_hung"}, instanceName, instanceLabels)

		// Chart 4: Thread Pool Performance (utilization and efficiency)
		w.ensureChartExists("websphere_pmi.threading.pools_performance", "Thread Pool Performance", "percent", "line", "threading/pools", 70803,
			[]string{"percent_used", "percent_maxed"}, instanceName, instanceLabels)

		// Chart 5: Thread Pool Active Time
		w.ensureChartExists("websphere_pmi.threading.pools_time", "Thread Pool Active Time", "milliseconds", "line", "threading/pools", 70804,
			[]string{"active_time"}, instanceName, instanceLabels)

		// Chart IDs for each chart (without websphere_pmi prefix)
		usageChartID := fmt.Sprintf("threading.pools_usage_%s", w.sanitizeForChartID(instanceName))
		lifecycleChartID := fmt.Sprintf("threading.pools_lifecycle_%s", w.sanitizeForChartID(instanceName))
		healthChartID := fmt.Sprintf("threading.pools_health_%s", w.sanitizeForChartID(instanceName))
		performanceChartID := fmt.Sprintf("threading.pools_performance_%s", w.sanitizeForChartID(instanceName))
		timeChartID := fmt.Sprintf("threading.pools_time_%s", w.sanitizeForChartID(instanceName))

		// Extract all thread pool metrics into mx with proper dimension IDs
		// Note: extractThreadPoolMetrics now handles all metrics and maps them to the right chartIDs
		w.extractThreadPoolMetricsToCharts(mx, usageChartID, lifecycleChartID, healthChartID, performanceChartID, timeChartID, stat)

		w.Debugf("Thread pool '%s' - extracted metrics for charts: usage=%s, lifecycle=%s, health=%s, performance=%s, time=%s",
			poolName, usageChartID, lifecycleChartID, healthChartID, performanceChartID, timeChartID)

		// Debug: log what's in mx for these charts
		foundMetrics := false
		chartPrefixes := []string{usageChartID, lifecycleChartID, healthChartID, performanceChartID, timeChartID}
		for _, prefix := range chartPrefixes {
			for k, v := range mx {
				if strings.HasPrefix(k, prefix) {
					w.Debugf("Thread pool mx[%s] = %d", k, v)
					foundMetrics = true
				}
			}
		}

		if !foundMetrics {
			w.Debugf("Thread pool '%s' - no metrics found in stat (BoundedRangeStatistics=%d, CountStatistics=%d, TimeStatistics=%d, RangeStatistics=%d)",
				poolName, len(stat.BoundedRangeStatistics), len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.RangeStatistics))
		}
	} else {
		w.Debugf("Thread pool '%s' skipped (poolName empty or filtered)", poolName)
	}
}

// copyThreadPoolMetricsToMain is no longer needed since we extract directly with proper keys

// extractConnectionPoolMetrics extracts connection pool metrics with proper dimension mapping
func (w *WebSpherePMI) extractConnectionPoolMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI statistic names to our standard dimension names
	for _, brs := range stat.BoundedRangeStatistics {
		var dimensionName string
		switch brs.Name {
		case "ActiveCount":
			dimensionName = "active"
		case "FreePoolSize":
			dimensionName = "free"
		case "PoolSize":
			// PoolSize usually means total size
			dimensionName = "total"
		default:
			// Try to map based on name patterns
			nameLower := strings.ToLower(brs.Name)
			if strings.Contains(nameLower, "active") {
				dimensionName = "active"
			} else if strings.Contains(nameLower, "free") {
				dimensionName = "free"
			} else if strings.Contains(nameLower, "size") && !strings.Contains(nameLower, "free") {
				dimensionName = "total"
			} else {
				continue
			}
		}

		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Connection pool metric: %s = %d (from %s)", metricKey, val, brs.Name)
			}
		}
	}

	// Also check CountStatistics
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		nameLower := strings.ToLower(cs.Name)

		if strings.Contains(nameLower, "active") {
			dimensionName = "active"
		} else if strings.Contains(nameLower, "free") {
			dimensionName = "free"
		} else if strings.Contains(nameLower, "size") || strings.Contains(nameLower, "total") {
			dimensionName = "total"
		} else {
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Connection pool metric: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}
}

// processServletsContainer processes the "Servlets" container to find individual servlets and their URLs
func (w *WebSpherePMI) processServletsContainer(mx map[string]int64, nodeName, serverName, appName string, stat pmiStat) {
	w.Debugf("Processing Servlets container for app '%s' with %d sub-stats", appName, len(stat.SubStats))
	
	// Note: LoadedServletCount and ReloadCount are at the web application level,
	// not inside the Servlets container - they're collected in collectWebMetrics
	
	// Each sub-stat should be an individual servlet
	for _, servletStat := range stat.SubStats {
		servletName := servletStat.Name
		w.Debugf("Found servlet '%s' in app '%s'", servletName, appName)
		
		// Process this servlet's metrics
		w.collectServletMetrics(mx, nodeName, serverName, appName, servletName, servletStat)
		
		// Look for URLs sub-stat within this servlet
		for _, subStat := range servletStat.SubStats {
			if subStat.Name == "URLs" {
				w.processURLsContainer(mx, nodeName, serverName, appName, servletName, subStat)
			}
		}
	}
}

// collectServletContainerMetrics collects container-level servlet metrics from web application level
// These metrics (LoadedServletCount, ReloadCount) are at the web application level, not inside Servlets
func (w *WebSpherePMI) collectServletContainerMetrics(mx map[string]int64, nodeName, serverName, appName string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName))
	instanceLabels := map[string]string{
		"node":        nodeName,
		"server":      serverName,
		"application": appName,
	}

	w.Debugf("Collecting servlet container metrics for app: %s from web application level", appName)

	// Chart 1: Servlet Container State (gauge - current loaded servlets)
	w.ensureChartExistsWithDims("websphere_pmi.web.servlet_container_state", "Servlet Container State", "servlets", "line", "web/servlets/state", 70415,
		[]DimensionConfig{{Name: "loaded_servlets", Algo: module.Absolute}}, instanceName, instanceLabels)

	// Chart 2: Servlet Container Reloads (incremental counter)
	w.ensureChartExistsWithDims("websphere_pmi.web.servlet_container_reloads", "Servlet Container Reloads", "reloads/s", "line", "web/servlets/reloads", 70416,
		[]DimensionConfig{{Name: "reload_count", Algo: module.Incremental}}, instanceName, instanceLabels)

	stateChartID := fmt.Sprintf("web.servlet_container_state_%s", w.sanitizeForChartID(instanceName))
	reloadsChartID := fmt.Sprintf("web.servlet_container_reloads_%s", w.sanitizeForChartID(instanceName))

	// Extract container-level metrics
	for _, cs := range stat.CountStatistics {
		var metricKey string
		
		switch cs.Name {
		case "LoadedServletCount":
			metricKey = fmt.Sprintf("%s_loaded_servlets", stateChartID)
		case "ReloadCount":
			metricKey = fmt.Sprintf("%s_reload_count", reloadsChartID)
		default:
			continue
		}
		
		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				mx[metricKey] = val
				w.Debugf("Servlet container metric at web app level: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}
}

// processURLsContainer processes the "URLs" container to find individual URL metrics
func (w *WebSpherePMI) processURLsContainer(mx map[string]int64, nodeName, serverName, appName, servletName string, stat pmiStat) {
	w.Debugf("Processing URLs container for servlet '%s' with %d URLs", servletName, len(stat.SubStats))
	
	// Each sub-stat should be an individual URL
	for _, urlStat := range stat.SubStats {
		urlPath := urlStat.Name
		w.collectURLMetrics(mx, nodeName, serverName, appName, servletName, urlPath, urlStat)
	}
}

// collectServletMetrics collects metrics for an individual servlet
func (w *WebSpherePMI) collectServletMetrics(mx map[string]int64, nodeName, serverName, appName, servletName string, stat pmiStat) {
	// Create a clean servlet identifier
	cleanServletName := w.sanitizeForMetricName(servletName)
	instanceName := fmt.Sprintf("%s.%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName), cleanServletName)
	instanceLabels := map[string]string{
		"node":        nodeName,
		"server":      serverName,
		"application": appName,
		"servlet":     servletName,
	}

	// Check if this servlet has meaningful metrics (not just sub-stats)
	if !w.hasDirectMetrics(stat) {
		w.Debugf("Servlet '%s' has no direct metrics, only sub-stats", servletName)
		return
	}

	w.Debugf("Collecting metrics for servlet: %s", servletName)

	// Chart 1: Servlet Request Rate (incremental counter)
	w.ensureChartExistsWithDims("websphere_pmi.web.servlet_request_rate", "Servlet Request Rate", "requests/s", "line", "web/servlets/requests", 70410,
		[]DimensionConfig{{Name: "request_count", Algo: module.Incremental}}, instanceName, instanceLabels)

	// Chart 2: Servlet Error Rate (incremental counter)
	w.ensureChartExistsWithDims("websphere_pmi.web.servlet_error_rate", "Servlet Error Rate", "errors/s", "line", "web/servlets/errors", 70411,
		[]DimensionConfig{{Name: "error_count", Algo: module.Incremental}}, instanceName, instanceLabels)

	// Chart 3: Servlet Concurrent Requests (gauge)
	w.ensureChartExistsWithDims("websphere_pmi.web.servlet_concurrent", "Servlet Concurrent Requests", "requests", "line", "web/servlets/concurrent", 70412,
		[]DimensionConfig{{Name: "concurrent_requests", Algo: module.Absolute}}, instanceName, instanceLabels)

	// Chart 4: Servlet Response Times (keep as is)
	w.ensureChartExists("websphere_pmi.web.servlet_response_time", "Servlet Response Time", "milliseconds", "line", "web/servlets/response", 70413,
		[]string{"service_time", "async_response_time"}, instanceName, instanceLabels)

	// Chart IDs for metrics extraction
	requestRateChartID := fmt.Sprintf("web.servlet_request_rate_%s", w.sanitizeForChartID(instanceName))
	errorRateChartID := fmt.Sprintf("web.servlet_error_rate_%s", w.sanitizeForChartID(instanceName))
	concurrentChartID := fmt.Sprintf("web.servlet_concurrent_%s", w.sanitizeForChartID(instanceName))
	responseTimeChartID := fmt.Sprintf("web.servlet_response_time_%s", w.sanitizeForChartID(instanceName))

	// Extract servlet metrics
	w.extractServletMetricsToCharts(mx, requestRateChartID, errorRateChartID, concurrentChartID, responseTimeChartID, stat)
}

// collectURLMetrics collects metrics for an individual URL
func (w *WebSpherePMI) collectURLMetrics(mx map[string]int64, nodeName, serverName, appName, servletName, urlPath string, stat pmiStat) {
	// Create a clean URL identifier
	cleanURL := w.sanitizeForMetricName(urlPath)
	instanceName := fmt.Sprintf("%s.%s.%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName), w.sanitizeForMetricName(servletName), cleanURL)
	instanceLabels := map[string]string{
		"node":        nodeName,
		"server":      serverName,
		"application": appName,
		"servlet":     servletName,
		"url":         urlPath,
	}

	w.Debugf("Collecting metrics for URL: %s", urlPath)

	// Chart 1: URL Request Rate (incremental counter)
	w.ensureChartExistsWithDims("websphere_pmi.web.url_request_rate", "URL Request Rate", "requests/s", "line", "web/urls/requests", 70420,
		[]DimensionConfig{{Name: "request_count", Algo: module.Incremental}}, instanceName, instanceLabels)

	// Chart 2: URL Concurrent Requests (gauge)
	w.ensureChartExistsWithDims("websphere_pmi.web.url_concurrent", "URL Concurrent Requests", "requests", "line", "web/urls/concurrent", 70421,
		[]DimensionConfig{{Name: "concurrent_requests", Algo: module.Absolute}}, instanceName, instanceLabels)

	// Chart 3: URL Response Time (keep as is)
	w.ensureChartExists("websphere_pmi.web.url_response_time", "URL Response Time", "milliseconds", "line", "web/urls/response", 70422,
		[]string{"service_time", "async_response_time"}, instanceName, instanceLabels)

	// Chart IDs for metrics extraction
	requestRateChartID := fmt.Sprintf("web.url_request_rate_%s", w.sanitizeForChartID(instanceName))
	concurrentChartID := fmt.Sprintf("web.url_concurrent_%s", w.sanitizeForChartID(instanceName))
	responseTimeChartID := fmt.Sprintf("web.url_response_time_%s", w.sanitizeForChartID(instanceName))

	// Extract URL metrics
	w.extractURLMetricsToCharts(mx, requestRateChartID, concurrentChartID, responseTimeChartID, stat)
}

// collectTransactionMetrics handles transaction metrics
func (w *WebSpherePMI) collectTransactionMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)

	w.ensureChartExistsWithDims("websphere_pmi.system.transactions", "Transactions", "transactions/s", "stacked", "system", 70900,
		[]DimensionConfig{
			{Name: "committed", Algo: module.Incremental},
			{Name: "rolled_back", Algo: module.Incremental},
			{Name: "active", Algo: module.Incremental},
		}, instanceName, map[string]string{
			"node":   nodeName,
			"server": serverName,
		})

	chartID := fmt.Sprintf("system.transactions_%s", w.sanitizeForChartID(instanceName))

	// Extract transaction metrics with proper dimension mapping
	w.extractTransactionMetrics(mx, chartID, stat)
}

// collectSecurityMetrics handles security-related metrics
func (w *WebSpherePMI) collectSecurityMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	pathLower := strings.ToLower(path)

	// Authentication metrics
	if strings.Contains(pathLower, "auth") {
		w.ensureChartExistsWithDims("websphere_pmi.security.authentication", "Authentication Events", "events/s", "stacked", "security", 71000,
			[]DimensionConfig{
				{Name: "successful", Algo: module.Incremental},
				{Name: "failed", Algo: module.Incremental},
				{Name: "active_subjects", Algo: module.Incremental},
			}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		chartID := fmt.Sprintf("security.authentication_%s", w.sanitizeForChartID(instanceName))
		w.extractSecurityAuthMetrics(mx, chartID, stat)
	}

	// Authorization metrics
	if strings.Contains(pathLower, "authz") || strings.Contains(pathLower, "authorization") {
		w.ensureChartExistsWithDims("websphere_pmi.security.authorization", "Authorization Events", "events/s", "stacked", "security", 71001,
			[]DimensionConfig{
				{Name: "granted", Algo: module.Incremental},
				{Name: "denied", Algo: module.Incremental},
			}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		chartID := fmt.Sprintf("security.authorization_%s", w.sanitizeForChartID(instanceName))
		w.extractSecurityAuthzMetrics(mx, chartID, stat)
	}
}

// collectMessagingMetrics handles messaging metrics
func (w *WebSpherePMI) collectMessagingMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	pathLower := strings.ToLower(path)
	statNameLower := strings.ToLower(stat.Name)

	// ORB Interceptor metrics
	if stat.Name == "ORB" || stat.Name == "Interceptors" || strings.Contains(statNameLower, "interceptor") {
		w.collectORBInterceptorMetrics(mx, nodeName, serverName, path, stat)
		return
	}

	// JMS destination metrics
	if strings.Contains(pathLower, "jms") {
		destName := w.extractJMSDestinationName(path)
		if destName != "" && w.shouldCollectJMSDestination(destName) {
			instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, destName)

			w.ensureChartExists("websphere_pmi.messaging.jms", "JMS Destinations", "messages/s", "stacked", "messaging/jms", 71100,
				[]string{"messages_sent", "messages_received", "pending"}, instanceName, map[string]string{
					"node":        nodeName,
					"server":      serverName,
					"destination": destName,
				})

			chartID := fmt.Sprintf("messaging.jms_%s", w.sanitizeForChartID(instanceName))
			w.extractStatValues(mx, chartID, stat)
		}
	}

	// SIB messaging metrics
	if strings.Contains(pathLower, "sib") {
		instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)

		w.ensureChartExists("websphere_pmi.messaging.sib", "SIB Messaging", "messages/s", "stacked", "messaging/sib", 71200,
			[]string{"messages_sent", "messages_received"}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		chartID := fmt.Sprintf("messaging.sib_%s", w.sanitizeForChartID(instanceName))
		w.extractStatValues(mx, chartID, stat)
	}
}

// collectORBInterceptorMetrics handles ORB interceptor processing time metrics
func (w *WebSpherePMI) collectORBInterceptorMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {

	// Check if this is the ORB container
	if stat.Name == "ORB" {
		w.Debugf("Found ORB container with %d sub-stats", len(stat.SubStats))
		
		// Look for Interceptors sub-stat - ORB itself has no direct metrics
		for _, subStat := range stat.SubStats {
			if subStat.Name == "Interceptors" {
				w.collectORBInterceptorMetrics(mx, nodeName, serverName, path, subStat)
			}
		}
		return
	}

	// Check if this is the Interceptors container
	if stat.Name == "Interceptors" {
		w.Debugf("Found ORB Interceptors container with %d sub-stats", len(stat.SubStats))

		// Process each interceptor individually as separate instances
		interceptorCount := 0
		for _, interceptorStat := range stat.SubStats {
			dimensionID := w.getInterceptorDimensionID(interceptorStat.Name)
			if dimensionID != "" {
				// Check if this interceptor has ProcessingTime metrics
				hasProcessingTime := false
				for _, ts := range interceptorStat.TimeStatistics {
					if ts.Name == "ProcessingTime" {
						hasProcessingTime = true
						break
					}
				}
				
				if hasProcessingTime {
					// Create individual chart/instance for each interceptor
					interceptorInstanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, dimensionID)
					interceptorLabels := map[string]string{
						"node":        nodeName,
						"server":      serverName,
						"interceptor": interceptorStat.Name,
					}

					w.ensureChartExists("websphere_pmi.orb.interceptor_processing_time", "ORB Interceptor Processing Time", "milliseconds", "line", "messaging", 71300,
						[]string{"processing_time"}, interceptorInstanceName, interceptorLabels)

					chartID := fmt.Sprintf("orb.interceptor_processing_time_%s", w.sanitizeForChartID(interceptorInstanceName))
					w.extractORBInterceptorMetrics(mx, chartID, interceptorStat)
					interceptorCount++
				}
			}
		}

		w.Debugf("ORB Interceptors - processed %d interceptors with metrics", interceptorCount)
		return
	}

	// Check if this is an individual interceptor (contains "interceptor" in name)
	if strings.Contains(strings.ToLower(stat.Name), "interceptor") {
		dimensionID := w.getInterceptorDimensionID(stat.Name)
		if dimensionID != "" {
			// Check if this interceptor has ProcessingTime metrics
			hasProcessingTime := false
			for _, ts := range stat.TimeStatistics {
				if ts.Name == "ProcessingTime" {
					hasProcessingTime = true
					break
				}
			}
			
			if hasProcessingTime {
				// Create individual chart/instance for this interceptor
				interceptorInstanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, dimensionID)
				interceptorLabels := map[string]string{
					"node":        nodeName,
					"server":      serverName,
					"interceptor": stat.Name,
				}

				w.ensureChartExists("websphere_pmi.orb.interceptor_processing_time", "ORB Interceptor Processing Time", "milliseconds", "line", "messaging", 71300,
					[]string{"processing_time"}, interceptorInstanceName, interceptorLabels)

				chartID := fmt.Sprintf("orb.interceptor_processing_time_%s", w.sanitizeForChartID(interceptorInstanceName))
				w.extractORBInterceptorMetrics(mx, chartID, stat)
			}
		}
	}
}

// getInterceptorDimensionID maps interceptor names to dimension IDs
func (w *WebSpherePMI) getInterceptorDimensionID(interceptorName string) string {
	switch interceptorName {
	case "PMIServerRequestInterceptor":
		return "pmi_server"
	case "SecurityIORInterceptor":
		return "security_ior"
	case "TxIORInterceptor":
		return "tx_ior"
	case "WLMClientRequestInterceptor":
		return "wlm_client"
	case "WLMServerRequestInterceptor":
		return "wlm_server"
	case "com.ibm.ISecurityLocalObjectBaseL13Impl.CSIClientRI":
		return "security_csi_client"
	case "com.ibm.ISecurityLocalObjectBaseL13Impl.CSIServerRI":
		return "security_csi_server"
	case "com.ibm.ws.Transaction.JTS.TxClientInterceptor":
		return "tx_jts_client"
	case "com.ibm.ws.Transaction.JTS.TxServerInterceptor":
		return "tx_jts_server"
	case "com.ibm.ws.activity.remote.cos.ActivityIORInterceptor":
		return "activity_ior"
	case "com.ibm.ws.activity.remote.cos.ActivityServiceClientInterceptor":
		return "activity_client"
	case "com.ibm.ws.activity.remote.cos.ActivityServiceServerInterceptor":
		return "activity_server"
	case "com.ibm.ws.runtime.workloadcontroller.OrbWorkloadRequestInterceptor":
		return "workload_controller"
	case "com.ibm.debug.DebugPortableInterceptor":
		return "debug"
	case "com.ibm.ejs.ras.RasContextSupport":
		return "ras_context"
	case "WLMTaggedComponentManager":
		return "wlm_tagged_component"
	default:
		// For any unknown interceptor, create a sanitized ID from the name
		if strings.Contains(interceptorName, "Interceptor") || strings.Contains(interceptorName, "Manager") {
			// Sanitize the name to create a valid dimension ID
			sanitized := w.sanitizeForMetricName(interceptorName)
			w.Debugf("Creating dynamic interceptor ID for: %s -> %s", interceptorName, sanitized)
			return sanitized
		}
		return ""
	}
}

// collectCacheMetrics handles cache-related metrics
func (w *WebSpherePMI) collectCacheMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// Check if this is a cache object (e.g., "Object: ws/com.ibm.workplace/ExtensionRegistryCache")
	if strings.HasPrefix(stat.Name, "Object: ") {
		// Extract cache name from "Object: ws/com.ibm.workplace/ExtensionRegistryCache"
		cacheName := strings.TrimPrefix(stat.Name, "Object: ")
		cacheName = strings.ReplaceAll(cacheName, "/", "_") // Clean for metric names
		
		instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(cacheName))
		instanceLabels := map[string]string{
			"node":   nodeName,
			"server": serverName,
			"cache":  cacheName,
		}

		w.Debugf("Found cache object: cache='%s'", cacheName)

		// Chart 1: Cache Hit Rates
		w.ensureChartExistsWithDims("websphere_pmi.caching.hit_rates", "Dynamic Cache Hit Rates", "hits/s", "stacked", "caching", 71300,
			[]DimensionConfig{
				{Name: "memory_hits", Algo: module.Incremental},
				{Name: "disk_hits", Algo: module.Incremental},
				{Name: "misses", Algo: module.Incremental},
			}, instanceName, instanceLabels)

		// Chart 2: Cache Requests  
		w.ensureChartExistsWithDims("websphere_pmi.caching.requests", "Dynamic Cache Requests", "requests/s", "line", "caching", 71301,
			[]DimensionConfig{
				{Name: "client_requests", Algo: module.Incremental},
				{Name: "distributed_requests", Algo: module.Incremental},
			}, instanceName, instanceLabels)

		// Chart 3: Cache Entries
		w.ensureChartExists("websphere_pmi.caching.entries", "Dynamic Cache Entries", "entries", "line", "caching", 71302,
			[]string{"memory_entries", "total_entries", "max_memory_entries"}, instanceName, instanceLabels)

		// Chart 4: Cache Invalidations
		w.ensureChartExistsWithDims("websphere_pmi.caching.invalidations", "Dynamic Cache Invalidations", "invalidations/s", "stacked", "caching", 71303,
			[]DimensionConfig{
				{Name: "explicit", Algo: module.Incremental},
				{Name: "lru", Algo: module.Incremental},
				{Name: "timeout", Algo: module.Incremental},
				{Name: "memory_explicit", Algo: module.Incremental},
				{Name: "disk_explicit", Algo: module.Incremental},
				{Name: "local_explicit", Algo: module.Incremental},
			}, instanceName, instanceLabels)

		// Chart 5: Cache Remote Operations
		w.ensureChartExistsWithDims("websphere_pmi.caching.remote", "Dynamic Cache Remote Operations", "operations/s", "line", "caching", 71304,
			[]DimensionConfig{
				{Name: "remote_creations", Algo: module.Incremental},
				{Name: "remote_hits", Algo: module.Incremental},
				{Name: "remote_invalidations", Algo: module.Incremental},
			}, instanceName, instanceLabels)

		// Chart IDs for each chart (without websphere_pmi prefix)
		hitRatesChartID := fmt.Sprintf("caching.hit_rates_%s", w.sanitizeForChartID(instanceName))
		requestsChartID := fmt.Sprintf("caching.requests_%s", w.sanitizeForChartID(instanceName))
		entriesChartID := fmt.Sprintf("caching.entries_%s", w.sanitizeForChartID(instanceName))
		invalidationsChartID := fmt.Sprintf("caching.invalidations_%s", w.sanitizeForChartID(instanceName))
		remoteChartID := fmt.Sprintf("caching.remote_%s", w.sanitizeForChartID(instanceName))

		// Extract all cache metrics into mx with proper dimension IDs
		w.extractDynaCacheMetricsToCharts(mx, hitRatesChartID, requestsChartID, entriesChartID, invalidationsChartID, remoteChartID, stat)
		
		w.Debugf("Cache object '%s' - extracted metrics for 5 charts", cacheName)
	} else if stat.Name == "Dynamic Caching" {
		// This is the parent container
		w.Debugf("Processing Dynamic Caching container with %d sub-stats", len(stat.SubStats))
		
		// Skip top-level Dynamic Caching metrics - they mix different metric types
		// We collect proper per-cache metrics below instead
		
		// Then process sub-stats for individual cache objects
		for _, subStat := range stat.SubStats {
			if strings.HasPrefix(subStat.Name, "Object: ") {
				w.collectCacheMetrics(mx, nodeName, serverName, path, subStat)
			}
		}
	}
}

// collectHAManagerMetrics handles HAManager high availability metrics  
func (w *WebSpherePMI) collectHAManagerMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	instanceLabels := map[string]string{
		"node":   nodeName,
		"server": serverName,
	}

	// Check if this is HAManagerMBean (core HA metrics)
	if stat.Name == "HAManagerMBean" {
		w.Debugf("Found HAManagerMBean with %d CountStats, %d TimeStats, %d BoundedRangeStats",
			len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.BoundedRangeStatistics))

		// Chart 1: HA Groups
		w.ensureChartExists("websphere_pmi.hamanager.groups", "HAManager Groups", "groups", "line", "hamanager", 72100,
			[]string{"local_groups", "bulletin_board_subjects", "bulletin_board_subscriptions", "local_bulletin_board_subjects", "local_bulletin_board_subscriptions"}, instanceName, instanceLabels)

		// Chart 2: HA Rebuild Times
		w.ensureChartExists("websphere_pmi.hamanager.rebuild_times", "HAManager Rebuild Times", "milliseconds", "line", "hamanager", 72101,
			[]string{"group_state_rebuild_time", "bulletin_board_rebuild_time"}, instanceName, instanceLabels)

		// Chart IDs for metrics extraction
		groupsChartID := fmt.Sprintf("hamanager.groups_%s", w.sanitizeForChartID(instanceName))
		rebuildTimesChartID := fmt.Sprintf("hamanager.rebuild_times_%s", w.sanitizeForChartID(instanceName))

		// Extract HAManager metrics into mx with proper dimension IDs
		w.extractHAManagerMetricsToCharts(mx, groupsChartID, rebuildTimesChartID, stat)

		w.Debugf("HAManager core - extracted metrics for 2 charts")
	} else if stat.Name == "HAManager" {
		// This is the parent container - process sub-stats for HAManagerMBean
		w.Debugf("Processing HAManager container with %d sub-stats", len(stat.SubStats))
		for _, subStat := range stat.SubStats {
			if subStat.Name == "HAManagerMBean" {
				w.collectHAManagerMetrics(mx, nodeName, serverName, path, subStat)
			}
		}
	}
}

// collectWebServiceMetrics handles WebSphere web service PMI metrics
func (w *WebSpherePMI) collectWebServiceMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// Check if this is the pmiWebServiceModule container
	if stat.Name == "pmiWebServiceModule" {
		w.Debugf("Found pmiWebServiceModule container with %d sub-stats", len(stat.SubStats))

		// Process each web service application (*.war)
		webServiceCount := 0
		for _, appStat := range stat.SubStats {
			if strings.HasSuffix(appStat.Name, ".war") {
				w.collectWebServiceApplication(mx, nodeName, serverName, path, appStat)
				webServiceCount++
			}
		}

		w.Debugf("Web Services - processed %d applications", webServiceCount)
		return
	}

	// Check if this is a web service application (*.war)
	if strings.HasSuffix(stat.Name, ".war") {
		w.collectWebServiceApplication(mx, nodeName, serverName, path, stat)
	}
}

// collectWebServiceApplication handles individual web service application metrics
func (w *WebSpherePMI) collectWebServiceApplication(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// Extract application name from "applicationName.warName.war"
	appName := stat.Name
	// Clean app name for instance naming
	cleanAppName := strings.ReplaceAll(appName, ".war", "")
	cleanAppName = strings.ReplaceAll(cleanAppName, ".", "_")

	instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, cleanAppName)
	instanceLabels := map[string]string{
		"node":        nodeName,
		"server":      serverName,
		"application": cleanAppName,
		"war_file":    appName,
	}

	w.Debugf("Processing web service application: %s", appName)

	// Chart 1: Request Counts
	w.ensureChartExistsWithDims("websphere_pmi.webservice.requests", "Web Service Requests", "requests/s", "stacked", "webservice", 73100,
		[]DimensionConfig{
			{Name: "received", Algo: module.Incremental},
			{Name: "dispatched", Algo: module.Incremental},
			{Name: "successful", Algo: module.Incremental},
		}, instanceName, instanceLabels)

	// Chart 2: Response Times
	w.ensureChartExists("websphere_pmi.webservice.response_times", "Web Service Response Times", "milliseconds", "line", "webservice", 73101,
		[]string{"response_time", "request_response_time", "dispatch_response_time", "reply_response_time"}, instanceName, instanceLabels)

	// Chart 3: Message Sizes
	w.ensureChartExists("websphere_pmi.webservice.message_sizes", "Web Service Message Sizes", "bytes", "line", "webservice", 73102,
		[]string{"request_size", "reply_size", "avg_size"}, instanceName, instanceLabels)

	// Chart 4: Services Loaded
	w.ensureChartExists("websphere_pmi.webservice.services", "Web Service Services", "services", "line", "webservice", 73103,
		[]string{"services_loaded"}, instanceName, instanceLabels)

	// Chart IDs for metrics extraction
	requestsChartID := fmt.Sprintf("webservice.requests_%s", w.sanitizeForChartID(instanceName))
	responseTimesChartID := fmt.Sprintf("webservice.response_times_%s", w.sanitizeForChartID(instanceName))
	messageSizesChartID := fmt.Sprintf("webservice.message_sizes_%s", w.sanitizeForChartID(instanceName))
	servicesChartID := fmt.Sprintf("webservice.services_%s", w.sanitizeForChartID(instanceName))

	// Extract web service metrics into mx with proper dimension IDs
	w.extractWebServiceMetricsToCharts(mx, requestsChartID, responseTimesChartID, messageSizesChartID, servicesChartID, stat)

	w.Debugf("Web service application '%s' - extracted metrics for 4 charts", appName)
}

// collectObjectPoolMetrics handles Object Pool metrics
func (w *WebSpherePMI) collectObjectPoolMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// Check if this is the Object Pool container
	if stat.Name == "Object Pool" {
		w.Debugf("Found Object Pool container with %d sub-stats", len(stat.SubStats))
		
		// Extract any container-level metrics
		if len(stat.CountStatistics) > 0 || len(stat.BoundedRangeStatistics) > 0 {
			// Create container-level charts
			instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
			instanceLabels := map[string]string{
				"node":   nodeName,
				"server": serverName,
			}
			
			// Chart 1: Object Pool Total Operations (rates)
			w.ensureChartExistsWithDims("websphere_pmi.objectpool.total_operations", "Object Pool Total Operations", "operations/s", "line", "objectpool", 71000,
				[]DimensionConfig{
					{Name: "objects_created", Algo: module.Incremental},
					{Name: "objects_allocated", Algo: module.Incremental},
					{Name: "objects_returned", Algo: module.Incremental},
				}, instanceName, instanceLabels)
			
			// Chart 2: Object Pool Total State (gauge)
			w.ensureChartExists("websphere_pmi.objectpool.total_state", "Object Pool Total State", "objects", "line", "objectpool", 71001,
				[]string{"idle_objects"}, instanceName, instanceLabels)
			
			operationsChartID := fmt.Sprintf("objectpool.total_operations_%s", w.sanitizeForChartID(instanceName))
			stateChartID := fmt.Sprintf("objectpool.total_state_%s", w.sanitizeForChartID(instanceName))
			w.extractObjectPoolMetricsToCharts(mx, operationsChartID, stateChartID, stat)
		}
		
		// Process each object pool
		for _, poolStat := range stat.SubStats {
			if strings.HasPrefix(poolStat.Name, "ObjectPool_") {
				w.collectIndividualObjectPool(mx, nodeName, serverName, poolStat)
			}
		}
		
		return
	}
	
	// Handle individual object pool
	if strings.HasPrefix(stat.Name, "ObjectPool_") {
		w.collectIndividualObjectPool(mx, nodeName, serverName, stat)
	}
}

// collectIndividualObjectPool handles metrics for a specific object pool
func (w *WebSpherePMI) collectIndividualObjectPool(mx map[string]int64, nodeName, serverName string, stat pmiStat) {
	// Extract pool name - remove ObjectPool_ prefix
	poolName := strings.TrimPrefix(stat.Name, "ObjectPool_")
	cleanPoolName := w.sanitizeForMetricName(poolName)
	
	instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, cleanPoolName)
	instanceLabels := map[string]string{
		"node":   nodeName,
		"server": serverName,
		"pool":   poolName,
	}
	
	w.Debugf("Processing object pool: %s", poolName)
	
	// Chart 1: Object Pool Operations
	w.ensureChartExists("websphere_pmi.objectpool.operations", "Object Pool Operations", "operations", "line", "objectpool", 71001,
		[]string{"objects_created", "objects_allocated", "objects_returned"}, instanceName, instanceLabels)
	
	// Chart 2: Object Pool State
	w.ensureChartExists("websphere_pmi.objectpool.state", "Object Pool State", "objects", "line", "objectpool", 71001,
		[]string{"idle_objects"}, instanceName, instanceLabels)
	
	// Chart IDs
	operationsChartID := fmt.Sprintf("objectpool.operations_%s", w.sanitizeForChartID(instanceName))
	stateChartID := fmt.Sprintf("objectpool.state_%s", w.sanitizeForChartID(instanceName))
	
	// Extract metrics
	w.extractObjectPoolMetricsToCharts(mx, operationsChartID, stateChartID, stat)
	
	w.Debugf("Object pool '%s' - extracted metrics", poolName)
}

// collectSystemMetrics handles system-level metrics
func (w *WebSpherePMI) collectSystemMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)
	instanceLabels := map[string]string{
		"node":   nodeName,
		"server": serverName,
	}

	// Chart 1: CPU Usage (percentage)
	w.ensureChartExists("websphere_pmi.system.cpu", "System CPU Usage", "percentage", "line", "system", 70900,
		[]string{"usage"}, instanceName, instanceLabels)

	// Chart 2: Free Memory (bytes)
	w.ensureChartExists("websphere_pmi.system.memory", "System Free Memory", "bytes", "area", "system", 70901,
		[]string{"free"}, instanceName, instanceLabels)

	cpuChartID := fmt.Sprintf("system.cpu_%s", w.sanitizeForChartID(instanceName))
	memoryChartID := fmt.Sprintf("system.memory_%s", w.sanitizeForChartID(instanceName))
	w.extractSystemDataMetricsProper(mx, cpuChartID, memoryChartID, stat)
	w.Debugf("System Data - extracted CPU and memory metrics for %s", instanceName)
}

// collectGenericMetrics handles unrecognized metric categories
func (w *WebSpherePMI) collectGenericMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	// First check if this stat has any actual metrics to collect
	hasMetrics := false
	
	// Check for any statistics with values
	if len(stat.CountStatistics) > 0 && stat.CountStatistics[0].Count != "" {
		if _, err := strconv.ParseInt(stat.CountStatistics[0].Count, 10, 64); err == nil {
			hasMetrics = true
		}
	}
	if !hasMetrics && len(stat.RangeStatistics) > 0 && stat.RangeStatistics[0].Current != "" {
		if _, err := strconv.ParseInt(stat.RangeStatistics[0].Current, 10, 64); err == nil {
			hasMetrics = true
		}
	}
	if !hasMetrics && len(stat.BoundedRangeStatistics) > 0 && stat.BoundedRangeStatistics[0].Current != "" {
		if _, err := strconv.ParseInt(stat.BoundedRangeStatistics[0].Current, 10, 64); err == nil {
			hasMetrics = true
		}
	}
	if !hasMetrics && stat.Value != nil && stat.Value.Value != "" {
		if _, err := strconv.ParseInt(stat.Value.Value, 10, 64); err == nil {
			hasMetrics = true
		}
	}
	
	// Only create chart and collect metrics if there's actual data
	if !hasMetrics {
		w.Debugf("Skipping other metric '%s' - no values found", stat.Name)
		return
	}
	
	// For metrics that don't fit established categories, create generic monitoring charts
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)

	w.ensureChartExists("websphere_pmi.monitoring.other", "Other Metrics", "value", "line", "monitoring", 79000,
		[]string{"value"}, instanceName, map[string]string{
			"node":        nodeName,
			"server":      serverName,
			"metric_path": stat.Name,
		})

	chartID := fmt.Sprintf("monitoring.other_%s", w.sanitizeForChartID(instanceName))
	// For generic metrics, we need to ensure we create a "value" dimension
	w.extractGenericMetrics(mx, chartID, stat)
	w.Debugf("Other metrics (%s) - extracted metrics for %s", stat.Name, instanceName)
}

// Helper functions for name extraction and validation
func (w *WebSpherePMI) extractServletName(path string) string {
	// Extract servlet name from path like "webContainer.servlet.myapp.MyServlet"
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if strings.ToLower(part) == "servlet" && i+1 < len(parts) {
			return strings.Join(parts[i+1:], ".")
		}
	}
	return ""
}

func (w *WebSpherePMI) extractApplicationName(path string) string {
	// Extract application name from session paths
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if strings.ToLower(part) == "session" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (w *WebSpherePMI) extractPoolName(path string) string {
	// Extract pool name from connection pool paths
	parts := strings.Split(path, ".")
	for i, part := range parts {
		partLower := strings.ToLower(part)
		if (partLower == "connectionpool" || partLower == "datasource") && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (w *WebSpherePMI) extractThreadPoolName(path string) string {
	// Extract thread pool name from thread pool paths
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if strings.ToLower(part) == "threadpool" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (w *WebSpherePMI) extractJMSDestinationName(path string) string {
	// Extract JMS destination name
	parts := strings.Split(path, ".")
	for i, part := range parts {
		if strings.ToLower(part) == "jms" && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	return ""
}

func (w *WebSpherePMI) sanitizeForMetricName(input string) string {
	// Replace invalid characters for metric names
	return strings.ReplaceAll(strings.ReplaceAll(input, ".", "_"), " ", "_")
}

// Validation functions using configured selectors
func (w *WebSpherePMI) shouldCollectServlet(name string) bool {
	if w.servletSelector == nil {
		return true
	}
	return w.servletSelector.MatchString(name)
}

func (w *WebSpherePMI) shouldCollectApplication(name string) bool {
	if w.appSelector == nil {
		return true
	}
	return w.appSelector.MatchString(name)
}

func (w *WebSpherePMI) shouldCollectPool(name string) bool {
	if w.poolSelector == nil {
		return true
	}
	return w.poolSelector.MatchString(name)
}

func (w *WebSpherePMI) shouldCollectThreadPool(name string) bool {
	// Thread pools don't have a separate selector, use pool selector
	return w.shouldCollectPool(name)
}

func (w *WebSpherePMI) shouldCollectJMSDestination(name string) bool {
	if w.jmsSelector == nil {
		return true
	}
	return w.jmsSelector.MatchString(name)
}

// extractStatValues extracts numeric values from PMI stat entries
// The chartID parameter should be the chart ID without dimension suffix
func (w *WebSpherePMI) extractStatValues(mx map[string]int64, chartID string, stat pmiStat) int {
	extractedCount := 0

	// Extract from Value field
	if stat.Value != nil && stat.Value.Value != "" {
		if val, err := strconv.ParseInt(stat.Value.Value, 10, 64); err == nil {
			// Use the stat name as dimension name, or "value" if no name
			dimName := "value"
			if stat.Name != "" {
				dimName = w.sanitizeForMetricName(stat.Name)
			}
			metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
			mx[metricKey] = val
			extractedCount++
			w.Debugf("Extracted value metric: %s = %d", metricKey, val)
		}
	}

	// Extract from CountStatistics
	for _, cs := range stat.CountStatistics {
		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				dimName := w.sanitizeForMetricName(cs.Name)
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = val
				extractedCount++
				w.Debugf("Extracted count metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract from TimeStatistics
	for _, ts := range stat.TimeStatistics {
		baseName := w.sanitizeForMetricName(ts.Name)
		if ts.Count != "" {
			if val, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				dimName := baseName + "_count"
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = val
				extractedCount++
				w.Debugf("Extracted time count metric: %s = %d", metricKey, val)
			}
		}
		// Handle total/totalTime (WebSphere uses totalTime)
		totalStr := ts.TotalTime
		if totalStr == "" {
			totalStr = ts.Total
		}
		if totalStr != "" {
			if val, err := strconv.ParseInt(totalStr, 10, 64); err == nil {
				dimName := baseName + "_total"
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = val
				extractedCount++
				w.Debugf("Extracted time total metric: %s = %d", metricKey, val)
			}
		}
		if ts.Mean != "" {
			if val, err := strconv.ParseFloat(ts.Mean, 64); err == nil {
				dimName := baseName + "_mean"
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = int64(val * 1000) // Convert to microseconds for precision
				extractedCount++
				w.Debugf("Extracted time mean metric: %s = %d", metricKey, int64(val*1000))
			}
		}
	}

	// Extract from RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				dimName := w.sanitizeForMetricName(rs.Name) + "_current"
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = val
				extractedCount++
				w.Debugf("Extracted range metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract from BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				dimName := w.sanitizeForMetricName(brs.Name) + "_current"
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = val
				extractedCount++
				w.Debugf("Extracted bounded range metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract from DoubleStatistics
	for _, ds := range stat.DoubleStatistics {
		if ds.Double != "" {
			if val, err := strconv.ParseFloat(ds.Double, 64); err == nil {
				dimName := w.sanitizeForMetricName(ds.Name)
				metricKey := fmt.Sprintf("%s_%s", chartID, dimName)
				mx[metricKey] = int64(val * 1000) // Convert to thousandths for precision
				extractedCount++
				w.Debugf("Extracted double metric: %s = %d", metricKey, int64(val*1000))
			}
		}
	}

	return extractedCount
}

// extractThreadPoolMetrics extracts thread pool metrics with proper dimension IDs
func (w *WebSpherePMI) extractThreadPoolMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Debug what we have available
	w.Debugf("Thread pool extracting from stat with %d BoundedRangeStatistics, %d CountStatistics, %d TimeStatistics, %d RangeStatistics",
		len(stat.BoundedRangeStatistics), len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.RangeStatistics))

	foundMetrics := make(map[string]bool)

	// Extract from CountStatistics (CreateCount, DestroyCount, DeclaredThreadHungCount, ClearedThreadHangCount, MaxPoolSize)
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "CreateCount":
			dimensionName = "created"
		case "DestroyCount":
			dimensionName = "destroyed"
		case "DeclaredThreadHungCount":
			dimensionName = "declared_hung"
		case "ClearedThreadHangCount":
			dimensionName = "cleared_hung"
		case "MaxPoolSize":
			dimensionName = "maximum_size"
		case "ActiveCount":
			dimensionName = "active" // Fallback if not in BoundedRange
		case "PoolSize":
			dimensionName = "pool_size" // Fallback if not in BoundedRange
		default:
			w.Debugf("Thread pool: skipping CountStatistic '%s' (value: %s)", cs.Name, cs.Count)
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from CountStatistic %s)", metricKey, val, cs.Name)
			}
		}
	}

	// Extract from BoundedRangeStatistics (ActiveCount, PoolSize, PercentMaxed, PercentUsed)
	for _, brs := range stat.BoundedRangeStatistics {
		var dimensionName string
		switch brs.Name {
		case "ActiveCount":
			dimensionName = "active"
		case "PoolSize":
			dimensionName = "pool_size"
		case "PercentMaxed":
			dimensionName = "percent_maxed"
		case "PercentUsed":
			dimensionName = "percent_used"
		default:
			w.Debugf("Thread pool: skipping BoundedRangeStatistic '%s' (current: %s)", brs.Name, brs.Current)
			continue
		}

		// Use Current value (current state)
		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from BoundedRangeStatistic %s)", metricKey, val, brs.Name)
			}
		}

		// For WebSphere 8.5.5: Extract maximum_size from PoolSize UpperBound if MaxPoolSize CountStatistic not found
		if brs.Name == "PoolSize" && !foundMetrics["maximum_size"] && brs.UpperBound != "" {
			if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_maximum_size", chartID)
				mx[metricKey] = val
				foundMetrics["maximum_size"] = true
				w.Debugf("Thread pool metric: %s = %d (from BoundedRangeStatistic %s.UpperBound)", metricKey, val, brs.Name)
			}
		}
	}

	// Extract from RangeStatistics (ConcurrentHungThreadCount)
	for _, rs := range stat.RangeStatistics {
		var dimensionName string
		switch rs.Name {
		case "ConcurrentHungThreadCount":
			dimensionName = "concurrent_hung"
		default:
			w.Debugf("Thread pool: skipping RangeStatistic '%s' (value: %s)", rs.Name, rs.Current)
			continue
		}

		if rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from RangeStatistic %s)", metricKey, val, rs.Name)
			}
		}
	}

	// Extract from TimeStatistics (ActiveTime)
	for _, ts := range stat.TimeStatistics {
		var dimensionName string
		switch ts.Name {
		case "ActiveTime":
			dimensionName = "active_time"
		default:
			w.Debugf("Thread pool: skipping TimeStatistic '%s'", ts.Name)
			continue
		}

		// Use totalTime for lifetime total, or count for number of operations
		totalTimeStr := ts.TotalTime
		if totalTimeStr == "" {
			totalTimeStr = ts.Total // Fallback for other versions
		}

		if totalTimeStr != "" {
			if val, err := strconv.ParseInt(totalTimeStr, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from TimeStatistic %s.totalTime)", metricKey, val, ts.Name)
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
}

// extractThreadPoolMetricsToCharts extracts thread pool metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractThreadPoolMetricsToCharts(mx map[string]int64, usageChartID, lifecycleChartID, healthChartID, performanceChartID, timeChartID string, stat pmiStat) {
	// Debug what we have available
	w.Debugf("Thread pool extracting to charts from stat with %d BoundedRangeStatistics, %d CountStatistics, %d TimeStatistics, %d RangeStatistics",
		len(stat.BoundedRangeStatistics), len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.RangeStatistics))

	foundMetrics := make(map[string]bool)

	// Extract from CountStatistics (CreateCount, DestroyCount, DeclaredThreadHungCount, ClearedThreadHangCount, MaxPoolSize)
	for _, cs := range stat.CountStatistics {
		var chartID, dimensionName string
		switch cs.Name {
		case "CreateCount":
			chartID = lifecycleChartID
			dimensionName = "created"
		case "DestroyCount":
			chartID = lifecycleChartID
			dimensionName = "destroyed"
		case "DeclaredThreadHungCount":
			chartID = healthChartID
			dimensionName = "declared_hung"
		case "ClearedThreadHangCount":
			chartID = healthChartID
			dimensionName = "cleared_hung"
		case "MaxPoolSize":
			chartID = usageChartID
			dimensionName = "maximum_size"
		case "ActiveCount":
			chartID = usageChartID
			dimensionName = "active" // Fallback if not in BoundedRange
		case "PoolSize":
			chartID = usageChartID
			dimensionName = "pool_size" // Fallback if not in BoundedRange
		default:
			w.Debugf("Thread pool: skipping CountStatistic '%s' (value: %s)", cs.Name, cs.Count)
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from CountStatistic %s)", metricKey, val, cs.Name)
			}
		}
	}

	// Extract from BoundedRangeStatistics (ActiveCount, PoolSize, PercentMaxed, PercentUsed)
	for _, brs := range stat.BoundedRangeStatistics {
		var chartID, dimensionName string
		switch brs.Name {
		case "ActiveCount":
			chartID = usageChartID
			dimensionName = "active"
		case "PoolSize":
			chartID = usageChartID
			dimensionName = "pool_size"
		case "PercentMaxed":
			chartID = performanceChartID
			dimensionName = "percent_maxed"
		case "PercentUsed":
			chartID = performanceChartID
			dimensionName = "percent_used"
		default:
			w.Debugf("Thread pool: skipping BoundedRangeStatistic '%s' (current: %s)", brs.Name, brs.Current)
			continue
		}

		// Use Current value (current state)
		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from BoundedRangeStatistic %s)", metricKey, val, brs.Name)
			}
		}

		// For WebSphere 8.5.5: Extract maximum_size from PoolSize UpperBound if MaxPoolSize CountStatistic not found
		if brs.Name == "PoolSize" && !foundMetrics["maximum_size"] && brs.UpperBound != "" {
			if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_maximum_size", usageChartID)
				mx[metricKey] = val
				foundMetrics["maximum_size"] = true
				w.Debugf("Thread pool metric: %s = %d (from BoundedRangeStatistic %s.UpperBound)", metricKey, val, brs.Name)
			}
		}
	}

	// Extract from RangeStatistics (ConcurrentHungThreadCount)
	for _, rs := range stat.RangeStatistics {
		var chartID, dimensionName string
		switch rs.Name {
		case "ConcurrentHungThreadCount":
			chartID = healthChartID
			dimensionName = "concurrent_hung"
		default:
			w.Debugf("Thread pool: skipping RangeStatistic '%s' (value: %s)", rs.Name, rs.Current)
			continue
		}

		if rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from RangeStatistic %s)", metricKey, val, rs.Name)
			}
		}
	}

	// Extract from TimeStatistics (ActiveTime)
	for _, ts := range stat.TimeStatistics {
		var chartID, dimensionName string
		switch ts.Name {
		case "ActiveTime":
			chartID = timeChartID
			dimensionName = "active_time"
		default:
			w.Debugf("Thread pool: skipping TimeStatistic '%s'", ts.Name)
			continue
		}

		// Use totalTime for lifetime total, or count for number of operations
		totalTimeStr := ts.TotalTime
		if totalTimeStr == "" {
			totalTimeStr = ts.Total // Fallback for other versions
		}

		if totalTimeStr != "" {
			if val, err := strconv.ParseInt(totalTimeStr, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from TimeStatistic %s.totalTime)", metricKey, val, ts.Name)
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
}

// extractWebSessionMetricsToCharts extracts web session metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractWebSessionMetricsToCharts(mx map[string]int64, lifecycleChartID, activeChartID, timeChartID, cacheChartID, externalTimeChartID, externalSizeChartID, healthChartID, objectSizeChartID string, stat pmiStat) {
	// Debug what we have available
	w.Debugf("Web session extracting to charts from stat with %d CountStatistics, %d TimeStatistics, %d RangeStatistics, %d AverageStatistics",
		len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.RangeStatistics), len(stat.AverageStatistics))

	foundMetrics := make(map[string]bool)

	// Extract from CountStatistics (CreateCount, InvalidateCount, NoRoomForNewSessionCount, CacheDiscardCount, AffinityBreakCount, TimeoutInvalidationCount, ActivateNonExistSessionCount)
	for _, cs := range stat.CountStatistics {
		var chartID, dimensionName string
		switch cs.Name {
		case "CreateCount":
			chartID = lifecycleChartID
			dimensionName = "created"
		case "InvalidateCount":
			chartID = lifecycleChartID
			dimensionName = "invalidated"
		case "TimeoutInvalidationCount":
			chartID = lifecycleChartID
			dimensionName = "timeout_invalidated"
		case "NoRoomForNewSessionCount":
			chartID = cacheChartID
			dimensionName = "no_room_for_new"
		case "CacheDiscardCount":
			chartID = cacheChartID
			dimensionName = "cache_discarded"
		case "AffinityBreakCount":
			chartID = healthChartID
			dimensionName = "affinity_breaks"
		case "ActivateNonExistSessionCount":
			chartID = healthChartID
			dimensionName = "activate_nonexist"
		default:
			w.Debugf("Web session: skipping CountStatistic '%s' (value: %s)", cs.Name, cs.Count)
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Web session metric: %s = %d (from CountStatistic %s)", metricKey, val, cs.Name)
			}
		}
	}

	// Extract from RangeStatistics (ActiveCount, LiveCount)
	for _, rs := range stat.RangeStatistics {
		var chartID, dimensionName string
		switch rs.Name {
		case "ActiveCount":
			chartID = activeChartID
			dimensionName = "active"
		case "LiveCount":
			chartID = activeChartID
			dimensionName = "live"
		default:
			w.Debugf("Web session: skipping RangeStatistic '%s' (value: %s)", rs.Name, rs.Current)
			continue
		}

		if rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Web session metric: %s = %d (from RangeStatistic %s)", metricKey, val, rs.Name)
			}
		}
	}

	// Extract from TimeStatistics (LifeTime, ExternalReadTime, ExternalWriteTime, TimeSinceLastActivated, SessionObjectSize)
	for _, ts := range stat.TimeStatistics {
		var chartID, dimensionName string
		switch ts.Name {
		case "LifeTime":
			chartID = timeChartID
			dimensionName = "lifetime"
		case "ExternalReadTime":
			chartID = externalTimeChartID
			dimensionName = "external_read_time"
		case "ExternalWriteTime":
			chartID = externalTimeChartID
			dimensionName = "external_write_time"
		case "TimeSinceLastActivated":
			chartID = timeChartID
			dimensionName = "time_since_activated"
		case "SessionObjectSize":
			chartID = objectSizeChartID
			dimensionName = "object_size"
		default:
			w.Debugf("Web session: skipping TimeStatistic '%s'", ts.Name)
			continue
		}

		// Use totalTime for lifetime total
		totalTimeStr := ts.TotalTime
		if totalTimeStr == "" {
			totalTimeStr = ts.Total // Fallback for other versions
		}

		if totalTimeStr != "" {
			if val, err := strconv.ParseInt(totalTimeStr, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Web session metric: %s = %d (from TimeStatistic %s.totalTime)", metricKey, val, ts.Name)
			}
		}
	}

	// Extract from AverageStatistics (ExternalReadSize, ExternalWriteSize)
	for _, as := range stat.AverageStatistics {
		var chartID, dimensionName string
		switch as.Name {
		case "ExternalReadSize":
			chartID = externalSizeChartID
			dimensionName = "external_read_size"
		case "ExternalWriteSize":
			chartID = externalSizeChartID
			dimensionName = "external_write_size"
		default:
			w.Debugf("Web session: skipping AverageStatistic '%s'", as.Name)
			continue
		}

		// Use total for cumulative size
		if as.Total != "" {
			if val, err := strconv.ParseInt(as.Total, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Web session metric: %s = %d (from AverageStatistic %s.total)", metricKey, val, as.Name)
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
}

// extractDynaCacheMetricsToCharts extracts dynamic cache metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractDynaCacheMetricsToCharts(mx map[string]int64, hitRatesChartID, requestsChartID, entriesChartID, invalidationsChartID, remoteChartID string, stat pmiStat) {
	// Debug what we have available
	w.Debugf("Dynamic cache extracting to charts from stat with %d CountStatistics",
		len(stat.CountStatistics))

	foundMetrics := make(map[string]bool)

	// The cache metrics are nested: we need to look in SubStats for "Object Cache" → "Counters"
	// Let's traverse the hierarchy: Object → Object Cache → Counters
	for _, subStat := range stat.SubStats {
		if subStat.Name == "Object Cache" {
			// Found Object Cache level, now look for Counters
			for _, counterStat := range subStat.SubStats {
				if counterStat.Name == "Counters" {
					// Extract all CountStatistics from Counters
					w.extractCacheCountersToCharts(mx, hitRatesChartID, requestsChartID, entriesChartID, invalidationsChartID, remoteChartID, counterStat, &foundMetrics)
				}
			}
			// Also extract any direct CountStatistics from Object Cache level
			w.extractCacheCountersToCharts(mx, hitRatesChartID, requestsChartID, entriesChartID, invalidationsChartID, remoteChartID, subStat, &foundMetrics)
		}
	}

	// Also extract direct CountStatistics from the Object level (InMemoryCacheEntryCount, MaxInMemoryCacheEntryCount)
	w.extractCacheCountersToCharts(mx, hitRatesChartID, requestsChartID, entriesChartID, invalidationsChartID, remoteChartID, stat, &foundMetrics)

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
}

// extractCacheCountersToCharts extracts cache counter metrics to appropriate charts
func (w *WebSpherePMI) extractCacheCountersToCharts(mx map[string]int64, hitRatesChartID, requestsChartID, entriesChartID, invalidationsChartID, remoteChartID string, stat pmiStat, foundMetrics *map[string]bool) {
	for _, cs := range stat.CountStatistics {
		var chartID, dimensionName string
		switch cs.Name {
		// Hit Rates Chart
		case "HitsInMemoryCount":
			chartID = hitRatesChartID
			dimensionName = "memory_hits"
		case "HitsOnDiskCount":
			chartID = hitRatesChartID
			dimensionName = "disk_hits"
		case "MissCount":
			chartID = hitRatesChartID
			dimensionName = "misses"
		// Requests Chart
		case "ClientRequestCount":
			chartID = requestsChartID
			dimensionName = "client_requests"
		case "DistributedRequestCount":
			chartID = requestsChartID
			dimensionName = "distributed_requests"
		case "RemoteHitCount":
			chartID = remoteChartID
			dimensionName = "remote_hits"
		// Entries Chart
		case "InMemoryCacheEntryCount":
			chartID = entriesChartID
			dimensionName = "memory_entries"
		case "InMemoryAndDiskCacheEntryCount":
			chartID = entriesChartID
			dimensionName = "total_entries"
		case "MaxInMemoryCacheEntryCount":
			chartID = entriesChartID
			dimensionName = "max_memory_entries"
		// Invalidations Chart
		case "ExplicitInvalidationCount":
			chartID = invalidationsChartID
			dimensionName = "explicit"
		case "LruInvalidationCount":
			chartID = invalidationsChartID
			dimensionName = "lru"
		case "TimeoutInvalidationCount":
			chartID = invalidationsChartID
			dimensionName = "timeout"
		case "ExplicitMemoryInvalidationCount":
			chartID = invalidationsChartID
			dimensionName = "memory_explicit"
		case "ExplicitDiskInvalidationCount":
			chartID = invalidationsChartID
			dimensionName = "disk_explicit"
		case "LocalExplicitInvalidationCount":
			chartID = invalidationsChartID
			dimensionName = "local_explicit"
		case "RemoteExplicitInvalidationCount":
			chartID = remoteChartID
			dimensionName = "remote_invalidations"
		// Remote Operations Chart
		case "RemoteCreationCount":
			chartID = remoteChartID
			dimensionName = "remote_creations"
		default:
			w.Debugf("Dynamic cache: skipping CountStatistic '%s' (value: %s)", cs.Name, cs.Count)
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				(*foundMetrics)[dimensionName] = true
				w.Debugf("Dynamic cache metric: %s = %d (from CountStatistic %s)", metricKey, val, cs.Name)
			}
		}
	}
}

// extractCacheMetrics extracts cache metrics with proper dimension mapping
func (w *WebSpherePMI) extractCacheMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI cache statistic names to our dimension names
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		nameLower := strings.ToLower(cs.Name)

		if strings.Contains(nameLower, "hit") && !strings.Contains(nameLower, "miss") {
			dimensionName = "hits"
		} else if strings.Contains(nameLower, "miss") {
			dimensionName = "misses"
		} else if strings.Contains(nameLower, "create") || strings.Contains(nameLower, "insert") {
			dimensionName = "creates"
		} else if strings.Contains(nameLower, "remove") || strings.Contains(nameLower, "evict") {
			dimensionName = "removes"
		} else {
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Cache metric: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}
}

// extractTransactionMetrics extracts transaction metrics with proper dimension mapping
func (w *WebSpherePMI) extractTransactionMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI transaction statistic names to our dimension names
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "CommittedCount", "LocalCommittedCount":
			dimensionName = "committed"
		case "RolledbackCount", "LocalRolledbackCount":
			dimensionName = "rolled_back"
		case "ActiveCount", "LocalActiveCount":
			dimensionName = "active"
		default:
			// Skip other transaction metrics for now
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				// Only update if we don't have a value yet (prefer non-local values)
				if _, exists := mx[metricKey]; !exists || strings.HasPrefix(cs.Name, "Local") {
					mx[metricKey] = val
					w.Debugf("Transaction metric: %s = %d (from %s)", metricKey, val, cs.Name)
				}
			}
		}
	}
}

// extractServerExtensionMetrics extracts server extension metrics with proper dimension mapping
func (w *WebSpherePMI) extractServerExtensionMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI statistic names to our dimension names
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "RequestCount":
			dimensionName = "requests"
		case "HitCount":
			dimensionName = "hits"
		case "HitRate":
			dimensionName = "hit_rate"
		case "DisplacementCount":
			dimensionName = "requests" // If no RequestCount, try DisplacementCount
		default:
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Server extension metric: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}

	// Also check BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		var dimensionName string
		switch brs.Name {
		case "RequestCount":
			dimensionName = "requests"
		case "HitCount":
			dimensionName = "hits"
		case "HitRate":
			dimensionName = "hit_rate"
		default:
			continue
		}

		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Server extension metric: %s = %d (from %s)", metricKey, val, brs.Name)
			}
		}
	}

	// Also check DoubleStatistics for HitRate
	for _, ds := range stat.DoubleStatistics {
		if ds.Name == "HitRate" && ds.Double != "" {
			if val, err := strconv.ParseFloat(ds.Double, 64); err == nil {
				metricKey := fmt.Sprintf("%s_hit_rate", chartID)
				mx[metricKey] = int64(val * 1000) // Convert to permille for precision
				w.Debugf("Server extension metric: %s = %d (from %s)", metricKey, int64(val*1000), ds.Name)
			}
		}
	}
}

// extractSecurityAuthMetrics extracts security authentication metrics with proper dimension mapping
func (w *WebSpherePMI) extractSecurityAuthMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Extract all authentication-related count statistics
	// WebSphere provides various authentication counts like WebAuthenticationCount, BasicAuthenticationCount, etc.
	// We'll sum related counts for our simplified dimensions

	var successfulCount, failedCount int64

	for _, cs := range stat.CountStatistics {
		if cs.Count == "" {
			continue
		}

		val, err := strconv.ParseInt(cs.Count, 10, 64)
		if err != nil {
			continue
		}

		// Aggregate authentication counts
		// Most authentication counts represent successful attempts
		// Failed attempts usually have "Failed" in the name
		if strings.Contains(cs.Name, "Failed") {
			failedCount += val
		} else if strings.Contains(cs.Name, "AuthenticationCount") ||
			strings.Contains(cs.Name, "IdentityAssertionCount") ||
			strings.Contains(cs.Name, "TAIRequestCount") {
			successfulCount += val
		}

		// Don't extract individual metrics - only use aggregated values
		// to match the chart dimensions
	}

	// Set aggregated values for chart dimensions
	mx[fmt.Sprintf("%s_successful", chartID)] = successfulCount
	mx[fmt.Sprintf("%s_failed", chartID)] = failedCount

	// For active subjects, look for a specific metric or use 0
	mx[fmt.Sprintf("%s_active_subjects", chartID)] = 0 // Default to 0 if not found

	// Also check BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		var dimensionName string
		switch brs.Name {
		case "ActiveSubjects", "ActiveSubjectCount":
			dimensionName = "active_subjects"
		default:
			continue
		}

		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Security auth metric: %s = %d (from %s)", metricKey, val, brs.Name)
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of whether security features are disabled vs. data collection issues
}

// extractSecurityAuthzMetrics extracts security authorization metrics with proper dimension mapping
func (w *WebSpherePMI) extractSecurityAuthzMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// WebSphere provides TimeStatistics for authorization, not CountStatistics
	// We'll extract the count from TimeStatistics which represents the number of authorization checks

	var grantedCount, deniedCount int64

	// Process TimeStatistics for authorization metrics
	for _, ts := range stat.TimeStatistics {
		if ts.Count == "" {
			continue
		}

		count, err := strconv.ParseInt(ts.Count, 10, 64)
		if err != nil {
			continue
		}

		// All authorization time statistics represent granted authorizations
		// (denied authorizations would typically throw exceptions and not be counted here)
		if strings.Contains(ts.Name, "AuthorizationTime") {
			grantedCount += count
		}

		// Don't extract individual metrics - only use aggregated values
		// to match the chart dimensions
	}

	// Set values for chart dimensions
	mx[fmt.Sprintf("%s_granted", chartID)] = grantedCount
	mx[fmt.Sprintf("%s_denied", chartID)] = deniedCount // Usually 0 as denials throw exceptions
}

// extractWebSessionMetrics extracts web session metrics with proper dimension mapping
func (w *WebSpherePMI) extractWebSessionMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI statistic names to our dimension names
	// WebSphere uses: CreateCount, InvalidateCount for CountStatistics
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
		case "CreateCount":
			dimensionName = "created"
		case "InvalidateCount":
			dimensionName = "invalidated"
		default:
			continue
		}

		metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
		mx[metricKey] = val
		w.Debugf("Web session metric: %s = %d", metricKey, val)
	}

	// Check RangeStatistics for active sessions
	for _, rs := range stat.RangeStatistics {
		if (rs.Name == "ActiveCount" || rs.Name == "LiveCount") && rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_active", chartID)
				mx[metricKey] = val
				w.Debugf("Web session active metric: %s = %d", metricKey, val)
				break // Only use first match (ActiveCount preferred over LiveCount)
			}
		}
	}

	// Check TimeStatistics for lifetime
	for _, ts := range stat.TimeStatistics {
		if ts.Name == "LifeTime" {
			// Get total time - WebSphere uses "totalTime" attribute
			totalStr := ts.TotalTime
			if totalStr == "" {
				totalStr = ts.Total // Fallback for other versions
			}

			// Calculate average lifetime when sessions exist
			if ts.Count != "" && totalStr != "" {
				count, err1 := strconv.ParseInt(ts.Count, 10, 64)
				total, err2 := strconv.ParseInt(totalStr, 10, 64)

				if err1 == nil && err2 == nil && count > 0 {
					// Calculate average lifetime in milliseconds
					avgLifetime := total / count
					metricKey := fmt.Sprintf("%s_lifetime", chartID)
					mx[metricKey] = avgLifetime
					w.Debugf("Web session lifetime metric: %s = %d ms (total=%d, count=%d)",
						metricKey, avgLifetime, total, count)
				} else if err1 == nil && count == 0 {
					// No sessions - set lifetime to 0
					metricKey := fmt.Sprintf("%s_lifetime", chartID)
					mx[metricKey] = 0
					w.Debugf("Web session lifetime metric: %s = 0 (no sessions)", metricKey)
				}
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
}

// extractSystemDataMetrics extracts system data metrics with proper dimension mapping
func (w *WebSpherePMI) extractSystemDataMetricsProper(mx map[string]int64, cpuChartID, memoryChartID string, stat pmiStat) {
	// Extract specific System Data metrics based on what we found in the PMI output
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		switch cs.Name {
		case "FreeMemory":
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_free", memoryChartID)
				mx[metricKey] = val
				w.Debugf("System memory metric: %s = %d bytes", metricKey, val)
			}
		}
	}
	
	// Process AverageStatistics for CPU
	for _, as := range stat.AverageStatistics {
		switch as.Name {
		case "CPUUsageSinceServerStarted":
			// Use the mean value for average CPU usage
			if val, err := strconv.ParseFloat(as.Mean, 64); err == nil {
				// Convert to integer percentage (multiply by precision for decimal preservation)
				metricKey := fmt.Sprintf("%s_usage", cpuChartID)
				mx[metricKey] = int64(val * float64(precision))
				w.Debugf("System CPU metric: %s = %.2f%% (stored as %d)", metricKey, val, mx[metricKey])
			}
		}
	}
}

// extractDynaCacheMetrics extracts dynamic cache metrics with proper dimension mapping
func (w *WebSpherePMI) extractDynaCacheMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI statistic names to our dimension names
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "RemoteCreationCount", "ObjectsCreatedCount", "CreateCount":
			dimensionName = "creates"
		case "RemoteRemovalCount", "LruInvalidationCount", "RemoveCount", "ExplicitInvalidationCount":
			dimensionName = "removes"
		default:
			// Look for patterns in the name
			nameLower := strings.ToLower(cs.Name)
			if strings.Contains(nameLower, "creat") {
				dimensionName = "creates"
			} else if strings.Contains(nameLower, "remov") || strings.Contains(nameLower, "invalidat") {
				dimensionName = "removes"
			} else {
				continue
			}
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				// Add to existing value (multiple remove types)
				if dimensionName == "removes" {
					mx[metricKey] += val
				} else {
					mx[metricKey] = val
				}
				w.Debugf("DynaCache metric: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}
}

// extractGenericMetrics extracts metrics for the generic monitoring.other context
func (w *WebSpherePMI) extractGenericMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// For generic metrics, we collect the first available value and map it to "value" dimension
	metricKey := fmt.Sprintf("%s_value", chartID)

	// Try CountStatistics first
	if len(stat.CountStatistics) > 0 && stat.CountStatistics[0].Count != "" {
		if val, err := strconv.ParseInt(stat.CountStatistics[0].Count, 10, 64); err == nil {
			mx[metricKey] = val
			w.Debugf("Generic metric: %s = %d (from CountStatistic %s)", metricKey, val, stat.CountStatistics[0].Name)
			return
		}
	}

	// Try RangeStatistics
	if len(stat.RangeStatistics) > 0 && stat.RangeStatistics[0].Current != "" {
		if val, err := strconv.ParseInt(stat.RangeStatistics[0].Current, 10, 64); err == nil {
			mx[metricKey] = val
			w.Debugf("Generic metric: %s = %d (from RangeStatistic %s)", metricKey, val, stat.RangeStatistics[0].Name)
			return
		}
	}

	// Try BoundedRangeStatistics
	if len(stat.BoundedRangeStatistics) > 0 && stat.BoundedRangeStatistics[0].Current != "" {
		if val, err := strconv.ParseInt(stat.BoundedRangeStatistics[0].Current, 10, 64); err == nil {
			mx[metricKey] = val
			w.Debugf("Generic metric: %s = %d (from BoundedRangeStatistic %s)", metricKey, val, stat.BoundedRangeStatistics[0].Name)
			return
		}
	}

	// Try Value field
	if stat.Value != nil && stat.Value.Value != "" {
		if val, err := strconv.ParseInt(stat.Value.Value, 10, 64); err == nil {
			mx[metricKey] = val
			w.Debugf("Generic metric: %s = %d (from Value field)", metricKey, val)
			return
		}
	}

	// Note: Do not set default zero value - let the framework detect missing metrics
	w.Debugf("Generic metric: %s - no value found in stat", metricKey)
}

// extractHAManagerMetricsToCharts extracts HAManager metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractHAManagerMetricsToCharts(mx map[string]int64, groupsChartID, rebuildTimesChartID string, stat pmiStat) {
	w.Debugf("HAManager extracting to charts from stat with %d CountStats, %d TimeStats, %d BoundedRangeStats",
		len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.BoundedRangeStatistics))

	// Extract BoundedRangeStatistics for group counts
	for _, brs := range stat.BoundedRangeStatistics {
		var metricKey string
		
		switch brs.Name {
		case "LocalGroupCount":
			metricKey = fmt.Sprintf("%s_local_groups", groupsChartID)
		case "BulletinBoardSubjectCount":
			metricKey = fmt.Sprintf("%s_bulletin_board_subjects", groupsChartID)
		case "BulletinBoardSubcriptionCount": // Note: spelling in XML is "Subcription" not "Subscription"
			metricKey = fmt.Sprintf("%s_bulletin_board_subscriptions", groupsChartID)
		case "LocalBulletinBoardSubjectCount":
			metricKey = fmt.Sprintf("%s_local_bulletin_board_subjects", groupsChartID)
		case "LocalBulletinBoardSubcriptionCount": // Note: spelling in XML is "Subcription" not "Subscription"
			metricKey = fmt.Sprintf("%s_local_bulletin_board_subscriptions", groupsChartID)
		}
		
		if metricKey != "" && brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				mx[metricKey] = val
				w.Debugf("HAManager group metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract TimeStatistics for rebuild times
	for _, ts := range stat.TimeStatistics {
		var metricKey string
		
		switch ts.Name {
		case "GroupStateRebuildTime":
			metricKey = fmt.Sprintf("%s_group_state_rebuild_time", rebuildTimesChartID)
		case "BulletinBoardRebuildTime":
			metricKey = fmt.Sprintf("%s_bulletin_board_rebuild_time", rebuildTimesChartID)
		}
		
		if metricKey != "" {
			// Get total time - WebSphere uses "totalTime" attribute
			totalStr := ts.TotalTime
			if totalStr == "" {
				totalStr = ts.Total // Fallback for other versions
			}

			// Calculate average rebuild time when rebuilds have occurred
			if ts.Count != "" && totalStr != "" {
				count, err1 := strconv.ParseInt(ts.Count, 10, 64)
				total, err2 := strconv.ParseInt(totalStr, 10, 64)

				if err1 == nil && err2 == nil && count > 0 {
					// Calculate average rebuild time in milliseconds
					avgRebuildTime := total / count
					mx[metricKey] = avgRebuildTime
					w.Debugf("HAManager rebuild time metric: %s = %d ms (total=%d, count=%d)",
						metricKey, avgRebuildTime, total, count)
				} else if err1 == nil && count == 0 {
					// No rebuilds - set time to 0
					mx[metricKey] = 0
					w.Debugf("HAManager rebuild time metric: %s = 0 (no rebuilds)", metricKey)
				}
			}
		}
	}
}

// extractORBInterceptorMetrics extracts processing time metrics from individual ORB interceptors
func (w *WebSpherePMI) extractORBInterceptorMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Check if this interceptor has ProcessingTime TimeStatistic
	for _, ts := range stat.TimeStatistics {
		if ts.Name == "ProcessingTime" {
			// Use "processing_time" as the dimension name since each interceptor has its own chart
			metricKey := fmt.Sprintf("%s_processing_time", chartID)

			// Get total time - WebSphere uses "totalTime" attribute
			totalStr := ts.TotalTime
			if totalStr == "" {
				totalStr = ts.Total // Fallback for other versions
			}

			// Calculate average processing time when requests have been processed
			if ts.Count != "" && totalStr != "" {
				count, err1 := strconv.ParseInt(ts.Count, 10, 64)
				total, err2 := strconv.ParseInt(totalStr, 10, 64)

				if err1 == nil && err2 == nil && count > 0 {
					// Calculate average processing time in milliseconds
					avgProcessingTime := total / count
					mx[metricKey] = avgProcessingTime
					w.Debugf("ORB interceptor metric: %s (%s) = %d ms (total=%d, count=%d)",
						metricKey, stat.Name, avgProcessingTime, total, count)
				} else if err1 == nil && count == 0 {
					// No requests processed - set time to 0
					mx[metricKey] = 0
					w.Debugf("ORB interceptor metric: %s (%s) = 0 (no requests)", metricKey, stat.Name)
				}
			}
			break // Only one ProcessingTime per interceptor
		}
	}
}

// extractWebServiceMetricsToCharts extracts web service metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractWebServiceMetricsToCharts(mx map[string]int64, requestsChartID, responseTimesChartID, messageSizesChartID, servicesChartID string, stat pmiStat) {
	w.Debugf("Web service extracting to charts from stat with %d CountStats, %d TimeStats, %d AverageStats",
		len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.AverageStatistics))

	// First, extract application-level ServicesLoaded metric
	for _, cs := range stat.CountStatistics {
		if cs.Name == "ServicesLoaded" && cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_services_loaded", servicesChartID)
				mx[metricKey] = val
				w.Debugf("Web service services metric: %s = %d", metricKey, val)
			}
		}
	}

	// Look for pmiWebServiceService sub-stat for detailed metrics
	for _, subStat := range stat.SubStats {
		if subStat.Name == "pmiWebServiceService" {
			w.extractWebServiceServiceMetrics(mx, requestsChartID, responseTimesChartID, messageSizesChartID, subStat)
			break
		}
	}
}

// extractWebServiceServiceMetrics extracts metrics from pmiWebServiceService block
func (w *WebSpherePMI) extractWebServiceServiceMetrics(mx map[string]int64, requestsChartID, responseTimesChartID, messageSizesChartID string, stat pmiStat) {
	// Extract CountStatistics for request counts
	for _, cs := range stat.CountStatistics {
		var metricKey string
		
		switch cs.Name {
		case "RequestReceivedService":
			metricKey = fmt.Sprintf("%s_received", requestsChartID)
		case "RequestDispatchedService":
			metricKey = fmt.Sprintf("%s_dispatched", requestsChartID)
		case "RequestSuccessfulService":
			metricKey = fmt.Sprintf("%s_successful", requestsChartID)
		}
		
		if metricKey != "" && cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				mx[metricKey] = val
				w.Debugf("Web service request metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract TimeStatistics for response times
	for _, ts := range stat.TimeStatistics {
		var metricKey string
		
		switch ts.Name {
		case "ResponseTimeService":
			metricKey = fmt.Sprintf("%s_response_time", responseTimesChartID)
		case "RequestResponseService":
			metricKey = fmt.Sprintf("%s_request_response_time", responseTimesChartID)
		case "DispatchResponseService":
			metricKey = fmt.Sprintf("%s_dispatch_response_time", responseTimesChartID)
		case "ReplyResponseService":
			metricKey = fmt.Sprintf("%s_reply_response_time", responseTimesChartID)
		}
		
		if metricKey != "" {
			// Get total time - WebSphere uses "totalTime" attribute
			totalStr := ts.TotalTime
			if totalStr == "" {
				totalStr = ts.Total // Fallback for other versions
			}

			// Calculate average response time when requests have been processed
			if ts.Count != "" && totalStr != "" {
				count, err1 := strconv.ParseInt(ts.Count, 10, 64)
				total, err2 := strconv.ParseInt(totalStr, 10, 64)

				if err1 == nil && err2 == nil && count > 0 {
					// Calculate average response time in milliseconds
					avgResponseTime := total / count
					mx[metricKey] = avgResponseTime
					w.Debugf("Web service response time metric: %s = %d ms (total=%d, count=%d)",
						metricKey, avgResponseTime, total, count)
				} else if err1 == nil && count == 0 {
					// No requests processed - set time to 0
					mx[metricKey] = 0
					w.Debugf("Web service response time metric: %s = 0 (no requests)", metricKey)
				}
			}
		}
	}

	// Extract AverageStatistics for message sizes
	for _, as := range stat.AverageStatistics {
		var metricKey string
		
		switch as.Name {
		case "RequestSizeService":
			metricKey = fmt.Sprintf("%s_request_size", messageSizesChartID)
		case "ReplySizeService":
			metricKey = fmt.Sprintf("%s_reply_size", messageSizesChartID)
		case "SizeService":
			metricKey = fmt.Sprintf("%s_avg_size", messageSizesChartID)
		}
		
		if metricKey != "" && as.Mean != "" {
			// Use mean value for average size metrics (already calculated by WebSphere)
			if val, err := strconv.ParseFloat(as.Mean, 64); err == nil {
				// Convert to int64 (bytes)
				mx[metricKey] = int64(val)
				w.Debugf("Web service size metric: %s = %d bytes", metricKey, int64(val))
			}
		}
	}
}

// extractTopLevelCacheMetrics extracts top-level Dynamic Caching metrics
func (w *WebSpherePMI) extractTopLevelCacheMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Extract CountStatistics
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "HitsInMemoryCount":
			dimensionName = "hits_in_memory"
		case "HitsOnDiskCount":
			dimensionName = "hits_on_disk"
		case "ExplicitInvalidationCount":
			dimensionName = "explicit_invalidations"
		case "LruInvalidationCount":
			dimensionName = "lru_invalidations"
		case "TimeoutInvalidationCount":
			dimensionName = "timeout_invalidations"
		case "InMemoryAndDiskCacheEntryCount":
			dimensionName = "memory_and_disk_entries"
		case "RemoteHitCount":
			dimensionName = "remote_hits"
		case "MissCount":
			dimensionName = "misses"
		case "ClientRequestCount":
			dimensionName = "client_requests"
		case "DistributedRequestCount":
			dimensionName = "distributed_requests"
		case "ExplicitMemoryInvalidationCount":
			dimensionName = "memory_explicit_invalidations"
		case "ExplicitDiskInvalidationCount":
			dimensionName = "disk_explicit_invalidations"
		case "RemoteCreationCount":
			dimensionName = "remote_creations"
		case "RemoteExplicitInvalidationCount":
			dimensionName = "remote_explicit_invalidations"
		case "LocalExplicitInvalidationCount":
			dimensionName = "local_explicit_invalidations"
		default:
			w.Debugf("Unknown cache metric: %s", cs.Name)
			continue
		}
		
		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Top-level cache metric: %s = %d", metricKey, val)
			}
		}
	}
	
	// Note: No BoundedRangeStatistics at top-level Dynamic Caching in our test environment
}

// extractORBOverviewMetrics extracts ORB-level metrics

// extractObjectPoolMetrics extracts object pool metrics from container level
func (w *WebSpherePMI) extractObjectPoolMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Extract CountStatistics
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "ObjectsCreatedCount":
			dimensionName = "objects_created"
		default:
			continue
		}
		
		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
			}
		}
	}
	
	// Extract BoundedRangeStatistics
	for _, rs := range stat.BoundedRangeStatistics {
		var dimensionName string
		switch rs.Name {
		case "ObjectsAllocatedCount":
			dimensionName = "objects_allocated"
		case "ObjectsReturnedCount":
			dimensionName = "objects_returned"
		case "IdleObjectsSize":
			dimensionName = "idle_objects"
		default:
			continue
		}
		
		if rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
			}
		}
	}
}

// extractObjectPoolMetricsToCharts extracts object pool metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractObjectPoolMetricsToCharts(mx map[string]int64, operationsChartID, stateChartID string, stat pmiStat) {
	// Extract CountStatistics for operations
	for _, cs := range stat.CountStatistics {
		var metricKey string
		switch cs.Name {
		case "ObjectsCreatedCount":
			metricKey = fmt.Sprintf("%s_objects_created", operationsChartID)
		default:
			continue
		}
		
		if metricKey != "" && cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				mx[metricKey] = val
				w.Debugf("Object pool metric: %s = %d", metricKey, val)
			}
		}
	}
	
	// Extract BoundedRangeStatistics
	for _, rs := range stat.BoundedRangeStatistics {
		var metricKey string
		
		switch rs.Name {
		case "ObjectsAllocatedCount":
			metricKey = fmt.Sprintf("%s_objects_allocated", operationsChartID)
		case "ObjectsReturnedCount":
			metricKey = fmt.Sprintf("%s_objects_returned", operationsChartID)
		case "IdleObjectsSize":
			metricKey = fmt.Sprintf("%s_idle_objects", stateChartID)
		default:
			continue
		}
		
		if metricKey != "" && rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				mx[metricKey] = val
				w.Debugf("Object pool metric: %s = %d (from %s)", metricKey, val, rs.Name)
			}
		}
	}
}

// extractServletMetricsToCharts extracts servlet metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractServletMetricsToCharts(mx map[string]int64, requestRateChartID, errorRateChartID, concurrentChartID, responseTimeChartID string, stat pmiStat) {
	w.Debugf("Servlet extracting to charts from stat with %d CountStats, %d TimeStats, %d RangeStats",
		len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.RangeStatistics))

	// Extract CountStatistics for request counts and errors
	for _, cs := range stat.CountStatistics {
		var metricKey string
		
		switch cs.Name {
		case "RequestCount":
			metricKey = fmt.Sprintf("%s_request_count", requestRateChartID)
		case "ErrorCount":
			metricKey = fmt.Sprintf("%s_error_count", errorRateChartID)
		default:
			continue
		}
		
		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				mx[metricKey] = val
				w.Debugf("Servlet count metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract RangeStatistics for concurrent requests
	for _, rs := range stat.RangeStatistics {
		if rs.Name == "ConcurrentRequests" && rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_concurrent_requests", concurrentChartID)
				mx[metricKey] = val
				w.Debugf("Servlet concurrent requests metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract TimeStatistics for response times
	for _, ts := range stat.TimeStatistics {
		var metricKey string
		
		switch ts.Name {
		case "ServiceTime":
			metricKey = fmt.Sprintf("%s_service_time", responseTimeChartID)
		case "AsyncContext Response Time":
			metricKey = fmt.Sprintf("%s_async_response_time", responseTimeChartID)
		default:
			continue
		}

		// Get total time - WebSphere uses "totalTime" attribute
		totalStr := ts.TotalTime
		if totalStr == "" {
			totalStr = ts.Total // Fallback for other versions
		}

		// Calculate average response time when requests have been processed
		if ts.Count != "" && totalStr != "" {
			count, err1 := strconv.ParseInt(ts.Count, 10, 64)
			total, err2 := strconv.ParseInt(totalStr, 10, 64)

			if err1 == nil && err2 == nil && count > 0 {
				// Calculate average response time in milliseconds
				avgResponseTime := total / count
				mx[metricKey] = avgResponseTime
				w.Debugf("Servlet time metric: %s = %d ms (total=%d, count=%d)",
					metricKey, avgResponseTime, total, count)
			} else if err1 == nil && count == 0 {
				// No requests processed - set time to 0
				mx[metricKey] = 0
				w.Debugf("Servlet time metric: %s = 0 (no requests)", metricKey)
			}
		}
	}
}

// extractURLMetricsToCharts extracts URL metrics and distributes them to appropriate charts
func (w *WebSpherePMI) extractURLMetricsToCharts(mx map[string]int64, requestRateChartID, concurrentChartID, responseTimeChartID string, stat pmiStat) {
	w.Debugf("URL extracting to charts from stat with %d CountStats, %d TimeStats, %d RangeStats",
		len(stat.CountStatistics), len(stat.TimeStatistics), len(stat.RangeStatistics))

	// Extract CountStatistics for request counts
	for _, cs := range stat.CountStatistics {
		if cs.Name == "URIRequestCount" && cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_request_count", requestRateChartID)
				mx[metricKey] = val
				w.Debugf("URL request count metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract RangeStatistics for concurrent requests
	for _, rs := range stat.RangeStatistics {
		if rs.Name == "URIConcurrentRequests" && rs.Current != "" {
			if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_concurrent_requests", concurrentChartID)
				mx[metricKey] = val
				w.Debugf("URL concurrent requests metric: %s = %d", metricKey, val)
			}
		}
	}

	// Extract TimeStatistics for response times
	for _, ts := range stat.TimeStatistics {
		var metricKey string
		
		switch ts.Name {
		case "URIServiceTime":
			metricKey = fmt.Sprintf("%s_service_time", responseTimeChartID)
		case "URL AsyncContext Response Time":
			metricKey = fmt.Sprintf("%s_async_response_time", responseTimeChartID)
		default:
			continue
		}

		// Get total time - WebSphere uses "totalTime" attribute
		totalStr := ts.TotalTime
		if totalStr == "" {
			totalStr = ts.Total // Fallback for other versions
		}

		// Calculate average response time when requests have been processed
		if ts.Count != "" && totalStr != "" {
			count, err1 := strconv.ParseInt(ts.Count, 10, 64)
			total, err2 := strconv.ParseInt(totalStr, 10, 64)

			if err1 == nil && err2 == nil && count > 0 {
				// Calculate average response time in milliseconds
				avgResponseTime := total / count
				mx[metricKey] = avgResponseTime
				w.Debugf("URL response time metric: %s = %d ms (total=%d, count=%d)",
					metricKey, avgResponseTime, total, count)
			} else if err1 == nil && count == 0 {
				// No requests processed - set time to 0
				mx[metricKey] = 0
				w.Debugf("URL response time metric: %s = 0 (no requests)", metricKey)
			}
		}
	}
}

// processStat is a legacy method for test compatibility
// Sets the path field recursively for testing path building logic
func (w *WebSpherePMI) processStat(stat *pmiStat, path string, mx map[string]int64) {
	if path == "" {
		stat.Path = stat.Name
	} else {
		if stat.Name != "" {
			stat.Path = path + "/" + stat.Name
		} else {
			stat.Path = path
		}
	}

	// Process sub-stats recursively
	for i := range stat.SubStats {
		w.processStat(&stat.SubStats[i], stat.Path, mx)
	}
}
