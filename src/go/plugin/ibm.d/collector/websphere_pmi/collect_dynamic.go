// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"context"
	"fmt"
	"strconv"
	"strings"
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

	switch category {
	case "server":
		w.collectServerOverviewMetrics(mx, nodeName, serverName, path, stat)
	case "jvm":
		w.collectJVMMetrics(mx, nodeName, serverName, path, stat)
	case "web":
		w.collectWebMetrics(mx, nodeName, serverName, path, stat)
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
	if strings.Contains(nameLower, "jvm runtime") || strings.Contains(nameLower, "object pool") {
		return "jvm"
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
	if strings.Contains(nameLower, "hamanager") {
		return "other"
	}

	// Transaction metrics
	if strings.Contains(nameLower, "transaction manager") {
		return "transactions"
	}

	// Caching metrics
	if strings.Contains(nameLower, "dynamic caching") {
		return "caching"
	}

	// Messaging/ORB metrics
	if strings.Contains(nameLower, "orb") || strings.Contains(nameLower, "interceptors") ||
		strings.Contains(nameLower, "sib") || strings.Contains(nameLower, "jms") {
		return "messaging"
	}

	// System data
	if strings.Contains(nameLower, "system data") {
		return "system"
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
func (w *WebSpherePMI) routeJVMMetrics(mx map[string]int64, tempMx map[string]int64, memoryChartID, runtimeChartID string) {
	for key, value := range tempMx {
		// Remove the "temp_" prefix to get the metric name
		metricName := strings.TrimPrefix(key, "temp_")

		// Route to memory chart
		if strings.Contains(metricName, "HeapSize") || strings.Contains(metricName, "Memory") {
			mx[fmt.Sprintf("%s_%s", memoryChartID, metricName)] = value
		} else if strings.Contains(metricName, "ProcessCpuUsage") || strings.Contains(metricName, "UpTime") {
			// Route to runtime chart
			mx[fmt.Sprintf("%s_%s", runtimeChartID, metricName)] = value
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

		// CPU and uptime
		cpuDimensions := []string{"ProcessCpuUsage", "UpTime"}
		w.ensureChartExists("websphere_pmi.jvm.runtime", "JVM Runtime", "value", "line", "jvm/runtime", 70101,
			cpuDimensions, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		// Extract for both charts
		memoryChartID := fmt.Sprintf("jvm.memory_%s", w.sanitizeForChartID(instanceName))
		runtimeChartID := fmt.Sprintf("jvm.runtime_%s", w.sanitizeForChartID(instanceName))

		// Extract all metrics first
		tempMx := make(map[string]int64)
		extracted := w.extractStatValues(tempMx, "temp", stat)

		// Route metrics to appropriate charts
		w.routeJVMMetrics(mx, tempMx, memoryChartID, runtimeChartID)
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
	w.Debugf("Web metrics processing: path='%s', stat.Name='%s'", path, stat.Name)

	// Check if this is a session metric for a specific web application
	// Pattern: isclite#isclite.war, perfServletApp#perfServletApp.war
	if strings.Contains(stat.Name, "#") && strings.Contains(stat.Name, ".war") {
		appName := stat.Name // Use the full application identifier
		instanceName := fmt.Sprintf("%s.%s.%s", nodeName, serverName, w.sanitizeForMetricName(appName))

		w.Debugf("Found web application session metrics: app='%s'", appName)

		// Create session management charts for this application
		w.ensureChartExists("websphere_pmi.web.sessions", "Web Application Sessions", "sessions", "line", "web/sessions", 70500,
			[]string{"active", "created", "invalidated", "lifetime"}, instanceName, map[string]string{
				"node":        nodeName,
				"server":      serverName,
				"application": appName,
			})

		chartID := fmt.Sprintf("web.sessions_%s", w.sanitizeForChartID(instanceName))
		w.extractWebSessionMetrics(mx, chartID, stat)
		w.Debugf("Web application '%s' - extracted session metrics", appName)
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

		// Create chart with shared dimensions for all thread pool instances
		w.ensureChartExists("websphere_pmi.threading.pools", "Thread Pool Usage", "threads", "stacked", "threading/pools", 70800,
			[]string{"active", "pool_size", "maximum_size"}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
				"pool":   poolName,
			})

		// Chart ID is "threading.pools_{instance}" (without websphere_pmi prefix)
		chartID := fmt.Sprintf("threading.pools_%s", w.sanitizeForChartID(instanceName))

		// Extract thread pool metrics directly into mx with proper dimension IDs
		w.extractThreadPoolMetrics(mx, chartID, stat)
		w.Debugf("Thread pool '%s' - extracted metrics for chart %s", poolName, chartID)

		// Debug: log what's in mx for this chart
		foundMetrics := false
		for k, v := range mx {
			if strings.HasPrefix(k, chartID) {
				w.Debugf("Thread pool mx[%s] = %d", k, v)
				foundMetrics = true
			}
		}

		if !foundMetrics {
			w.Debugf("Thread pool '%s' - no metrics found in stat (BoundedRangeStatistics=%d, CountStatistics=%d)",
				poolName, len(stat.BoundedRangeStatistics), len(stat.CountStatistics))
			// The extractThreadPoolMetrics function will handle setting default values
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

// collectTransactionMetrics handles transaction metrics
func (w *WebSpherePMI) collectTransactionMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)

	w.ensureChartExists("websphere_pmi.system.transactions", "Transactions", "transactions/s", "stacked", "system", 70900,
		[]string{"committed", "rolled_back", "active"}, instanceName, map[string]string{
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
		w.ensureChartExists("websphere_pmi.security.authentication", "Authentication Events", "events/s", "stacked", "security", 71000,
			[]string{"successful", "failed", "active_subjects"}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		chartID := fmt.Sprintf("security.authentication_%s", w.sanitizeForChartID(instanceName))
		w.extractSecurityAuthMetrics(mx, chartID, stat)
	}

	// Authorization metrics
	if strings.Contains(pathLower, "authz") || strings.Contains(pathLower, "authorization") {
		w.ensureChartExists("websphere_pmi.security.authorization", "Authorization Events", "events/s", "stacked", "security", 71001,
			[]string{"granted", "denied"}, instanceName, map[string]string{
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

// collectCacheMetrics handles cache-related metrics
func (w *WebSpherePMI) collectCacheMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)

	w.ensureChartExists("websphere_pmi.caching.dynacache", "Dynamic Cache", "operations/s", "stacked", "caching", 71300,
		[]string{"creates", "removes"}, instanceName, map[string]string{
			"node":   nodeName,
			"server": serverName,
		})

	chartID := fmt.Sprintf("caching.dynacache_%s", w.sanitizeForChartID(instanceName))
	w.extractDynaCacheMetrics(mx, chartID, stat)
}

// collectSystemMetrics handles system-level metrics
func (w *WebSpherePMI) collectSystemMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
	instanceName := fmt.Sprintf("%s.%s", nodeName, serverName)

	w.ensureChartExists("websphere_pmi.system.data", "System Data", "value", "line", "system", 70900,
		[]string{"value"}, instanceName, map[string]string{
			"node":   nodeName,
			"server": serverName,
		})

	chartID := fmt.Sprintf("system.data_%s", w.sanitizeForChartID(instanceName))
	w.extractSystemDataMetrics(mx, chartID, stat)
	w.Debugf("System Data - extracted metrics for %s", instanceName)
}

// collectGenericMetrics handles unrecognized metric categories
func (w *WebSpherePMI) collectGenericMetrics(mx map[string]int64, nodeName, serverName, path string, stat pmiStat) {
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
	w.Debugf("Thread pool extracting from stat with %d BoundedRangeStatistics, %d CountStatistics",
		len(stat.BoundedRangeStatistics), len(stat.CountStatistics))

	foundMetrics := make(map[string]bool)

	// Map WebSphere PMI statistic names to our standard dimension names
	for _, brs := range stat.BoundedRangeStatistics {
		var dimensionName string
		switch brs.Name {
		case "ActiveCount":
			dimensionName = "active"
		case "PoolSize":
			dimensionName = "pool_size"
		case "MaxPoolSize":
			dimensionName = "maximum_size"
		default:
			// Log what we're skipping to help debug
			w.Debugf("Thread pool: skipping BoundedRangeStatistic '%s' (current: %s, upper: %s)", brs.Name, brs.Current, brs.UpperBound)
			continue
		}

		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				// Metric key must match dimension ID: chartID_dimensionName
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				foundMetrics[dimensionName] = true
				w.Debugf("Thread pool metric: %s = %d (from BoundedRangeStatistic %s)", metricKey, val, brs.Name)
			}
		}
	}

	// Also check CountStatistics for MaxPoolSize
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		switch cs.Name {
		case "ActiveCount":
			dimensionName = "active"
		case "PoolSize":
			dimensionName = "pool_size"
		case "MaxPoolSize":
			dimensionName = "maximum_size"
		default:
			// Log what we're skipping to help debug
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

	// If we didn't find maximum_size, look for it in BoundedRangeStatistics UpperBound
	if !foundMetrics["maximum_size"] {
		// Try to find MaxPoolSize or UpperBound in BoundedRangeStatistics
		for _, brs := range stat.BoundedRangeStatistics {
			if brs.Name == "MaxPoolSize" && brs.UpperBound != "" {
				if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
					metricKey := fmt.Sprintf("%s_maximum_size", chartID)
					mx[metricKey] = val
					foundMetrics["maximum_size"] = true
					w.Debugf("Thread pool metric: %s = %d (from %s.UpperBound)", metricKey, val, brs.Name)
					break
				}
			} else if brs.Name == "PoolSize" && brs.UpperBound != "" && !foundMetrics["maximum_size"] {
				// Use PoolSize UpperBound as maximum if available
				if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
					metricKey := fmt.Sprintf("%s_maximum_size", chartID)
					mx[metricKey] = val
					foundMetrics["maximum_size"] = true
					w.Debugf("Thread pool metric: %s = %d (from %s.UpperBound)", metricKey, val, brs.Name)
					break
				}
			}
		}
	}

	// Note: Do not provide default zero values - let the framework detect missing metrics
	// This allows proper identification of data collection issues
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
func (w *WebSpherePMI) extractSystemDataMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// System data typically has a single value metric
	// Try to extract from various statistic types
	found := false

	// Check CountStatistics
	for _, cs := range stat.CountStatistics {
		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_value", chartID)
				mx[metricKey] = val
				w.Debugf("System data metric: %s = %d (from CountStatistic %s)", metricKey, val, cs.Name)
				found = true
				break
			}
		}
	}

	// If not found in CountStatistics, check RangeStatistics
	if !found {
		for _, rs := range stat.RangeStatistics {
			if rs.Current != "" {
				if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
					metricKey := fmt.Sprintf("%s_value", chartID)
					mx[metricKey] = val
					w.Debugf("System data metric: %s = %d (from RangeStatistic %s)", metricKey, val, rs.Name)
					found = true
					break
				}
			}
		}
	}

	// If not found, check Value field directly
	if !found && stat.Value != nil && stat.Value.Value != "" {
		if val, err := strconv.ParseInt(stat.Value.Value, 10, 64); err == nil {
			metricKey := fmt.Sprintf("%s_value", chartID)
			mx[metricKey] = val
			w.Debugf("System data metric: %s = %d (from Value field)", metricKey, val)
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
