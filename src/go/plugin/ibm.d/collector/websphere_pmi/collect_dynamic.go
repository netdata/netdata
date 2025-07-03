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
		strings.Contains(nameLower, "hamanager") ||
		// Individual thread pool names from WebSphere
		nameLower == "default" || nameLower == "ariesthreadpool" || nameLower == "hamanager.thread.pool" ||
		nameLower == "message listener" || nameLower == "object request broker" ||
		strings.Contains(nameLower, "sibfap") || strings.Contains(nameLower, "soapconnector") ||
		strings.Contains(nameLower, "tcpchannel") || strings.Contains(nameLower, "wmqjca") {
		return "threading"
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
		extracted := w.extractStatValues(mx, chartID, stat)
		w.Debugf("Server extensions - extracted %d metrics for %s", extracted, instanceName)
	}

	// Server root metrics
	if stat.Name == "server" {
		w.ensureChartExists("websphere_pmi.server.status", "Server Status", "status", "line", "server", 70001,
			[]string{"status"}, instanceName, map[string]string{
				"node":   nodeName,
				"server": serverName,
			})

		// Set server as up (value=1) when we receive data
		// Chart ID is "server.status_{instance}" (without websphere_pmi prefix)
		chartID := fmt.Sprintf("server.status_%s", w.sanitizeForChartID(instanceName))
		mx[fmt.Sprintf("%s_status", chartID)] = 1
		w.Debugf("Server status - set status=1 for %s", instanceName)

		// Extract any other metrics from server stat if needed
		if len(stat.CountStatistics) > 0 || len(stat.BoundedRangeStatistics) > 0 {
			extracted := w.extractStatValues(mx, chartID, stat)
			w.Debugf("Server overview - extracted %d metrics for %s", extracted, instanceName)
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
		extracted := w.extractStatValues(mx, chartID, stat)
		w.Debugf("Web application '%s' - extracted %d session metrics", appName, extracted)
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
			// Set zeros for missing metrics so they show up in charts
			mx[fmt.Sprintf("%s_active", chartID)] = 0
			mx[fmt.Sprintf("%s_pool_size", chartID)] = 0
			mx[fmt.Sprintf("%s_maximum_size", chartID)] = 0
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
		[]string{"hits", "misses", "creates", "removes"}, instanceName, map[string]string{
			"node":   nodeName,
			"server": serverName,
		})

	chartID := fmt.Sprintf("caching.dynacache_%s", w.sanitizeForChartID(instanceName))
	w.extractCacheMetrics(mx, chartID, stat)
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
	extracted := w.extractStatValues(mx, chartID, stat)
	w.Debugf("System Data - extracted %d metrics for %s", extracted, instanceName)
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
	extracted := w.extractStatValues(mx, chartID, stat)
	w.Debugf("Other metrics (%s) - extracted %d metrics for %s", stat.Name, extracted, instanceName)
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
		if ts.Total != "" {
			if val, err := strconv.ParseInt(ts.Total, 10, 64); err == nil {
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
			// Skip unmapped metrics for now to keep dimensions consistent
			continue
		}

		if brs.Current != "" {
			if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
				// Metric key must match dimension ID: chartID_dimensionName
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Thread pool metric: %s = %d (from %s)", metricKey, val, brs.Name)
			}
		}
	}

	// Also check CountStatistics for MaxPoolSize
	for _, cs := range stat.CountStatistics {
		if cs.Name == "MaxPoolSize" && cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_maximum_size", chartID)
				mx[metricKey] = val
				w.Debugf("Thread pool metric: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}

	// If we didn't find maximum_size, look for it in BoundedRangeStatistics
	if _, hasMax := mx[fmt.Sprintf("%s_maximum_size", chartID)]; !hasMax {
		// Try to find MaxPoolSize or UpperBound in BoundedRangeStatistics
		for _, brs := range stat.BoundedRangeStatistics {
			if brs.Name == "MaxPoolSize" && brs.UpperBound != "" {
				if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
					metricKey := fmt.Sprintf("%s_maximum_size", chartID)
					mx[metricKey] = val
					w.Debugf("Thread pool metric: %s = %d (from %s.UpperBound)", metricKey, val, brs.Name)
				}
			} else if brs.Name == "PoolSize" && brs.UpperBound != "" && !hasMax {
				// Use PoolSize UpperBound as maximum if available
				if val, err := strconv.ParseInt(brs.UpperBound, 10, 64); err == nil {
					metricKey := fmt.Sprintf("%s_maximum_size", chartID)
					mx[metricKey] = val
					w.Debugf("Thread pool metric: %s = %d (from %s.UpperBound)", metricKey, val, brs.Name)
				}
			}
		}
	}
}

// extractSecurityAuthMetrics extracts authentication metrics with proper dimension mapping
func (w *WebSpherePMI) extractSecurityAuthMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI security statistic names to our dimension names
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		nameLower := strings.ToLower(cs.Name)

		if strings.Contains(nameLower, "success") {
			dimensionName = "successful"
		} else if strings.Contains(nameLower, "fail") {
			dimensionName = "failed"
		} else if strings.Contains(nameLower, "active") || strings.Contains(nameLower, "subject") {
			dimensionName = "active_subjects"
		} else {
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Security auth metric: %s = %d (from %s)", metricKey, val, cs.Name)
			}
		}
	}
}

// extractSecurityAuthzMetrics extracts authorization metrics with proper dimension mapping
func (w *WebSpherePMI) extractSecurityAuthzMetrics(mx map[string]int64, chartID string, stat pmiStat) {
	// Map WebSphere PMI security statistic names to our dimension names
	for _, cs := range stat.CountStatistics {
		var dimensionName string
		nameLower := strings.ToLower(cs.Name)

		if strings.Contains(nameLower, "grant") || strings.Contains(nameLower, "permit") {
			dimensionName = "granted"
		} else if strings.Contains(nameLower, "deny") || strings.Contains(nameLower, "denied") {
			dimensionName = "denied"
		} else {
			continue
		}

		if cs.Count != "" {
			if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
				metricKey := fmt.Sprintf("%s_%s", chartID, dimensionName)
				mx[metricKey] = val
				w.Debugf("Security authz metric: %s = %d (from %s)", metricKey, val, cs.Name)
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
