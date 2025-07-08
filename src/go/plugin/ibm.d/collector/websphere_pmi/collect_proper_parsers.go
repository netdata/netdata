// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Additional parsing functions for proper collection

// Helper function to log parsing failures with detailed context
func (w *WebSpherePMI) logParsingFailure(parserName, statName string, stat *pmiStat, expectedMetrics []string, foundMetrics map[string]bool) {
	// Build list of missing metrics
	var missing []string
	for _, expected := range expectedMetrics {
		if !foundMetrics[expected] {
			missing = append(missing, expected)
		}
	}
	
	if len(missing) == 0 {
		return
	}
	
	// Log detailed failure information
	w.Errorf("ISSUE: failed to parse %s metrics in stat '%s'", parserName, statName)
	w.Errorf("  Missing metrics: %v", missing)
	w.Errorf("  Stat hierarchy: %s", statName)
	
	// Log available CountStatistics
	if len(stat.CountStatistics) > 0 {
		w.Errorf("  Available CountStatistics:")
		for _, cs := range stat.CountStatistics {
			w.Errorf("    - %s (count=%s)", cs.Name, cs.Count)
		}
	}
	
	// Log available TimeStatistics
	if len(stat.TimeStatistics) > 0 {
		w.Errorf("  Available TimeStatistics:")
		for _, ts := range stat.TimeStatistics {
			w.Errorf("    - %s (count=%s, total=%s)", ts.Name, ts.Count, ts.TotalTime)
		}
	}
	
	// Log available RangeStatistics
	if len(stat.RangeStatistics) > 0 {
		w.Errorf("  Available RangeStatistics:")
		for _, rs := range stat.RangeStatistics {
			w.Errorf("    - %s (value=%s)", rs.Name, rs.Current)
		}
	}
	
	// Log available BoundedRangeStatistics
	if len(stat.BoundedRangeStatistics) > 0 {
		w.Errorf("  Available BoundedRangeStatistics:")
		for _, brs := range stat.BoundedRangeStatistics {
			w.Errorf("    - %s (value=%s)", brs.Name, brs.Current)
		}
	}
	
	// Log available AverageStatistics
	if len(stat.AverageStatistics) > 0 {
		w.Errorf("  Available AverageStatistics:")
		for _, as := range stat.AverageStatistics {
			w.Errorf("    - %s (count=%s, total=%s)", as.Name, as.Count, as.Total)
		}
	}
	
	// Log available DoubleStatistics
	if len(stat.DoubleStatistics) > 0 {
		w.Errorf("  Available DoubleStatistics:")
		for _, ds := range stat.DoubleStatistics {
			w.Errorf("    - %s (double=%s)", ds.Name, ds.Double)
		}
	}
	
	// Log sub-stats if any
	if len(stat.SubStats) > 0 {
		w.Errorf("  Available SubStats:")
		for _, sub := range stat.SubStats {
			w.Errorf("    - %s", sub.Name)
		}
	}
}

// parseWebApplicationsContainer handles the Web Applications container
func (w *WebSpherePMI) parseWebApplicationsContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWebAppContainerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("webapp_container", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "LoadedServletCount":
			mx[fmt.Sprintf("webapp_container_%s_loaded_servlets", cleanInst)] = metric.Value
		case "RequestCount":
			mx[fmt.Sprintf("webapp_container_%s_requests", cleanInst)] = metric.Value
		case "ErrorCount":
			mx[fmt.Sprintf("webapp_container_%s_errors", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "webapp_container", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"webapp_container",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "webapp_container", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "webapp_container", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "webapp_container", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "webapp_container", cleanInst, metric)
	}
}

// parseWebApplication handles individual web application
func (w *WebSpherePMI) parseWebApplication(stat *pmiStat, nodeName, serverName string, mx map[string]int64, path []string) {
	appName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, appName)
	
	// Create charts if not exists
	w.ensureWebAppCharts(instance, nodeName, serverName, appName)
	
	// Mark as seen
	w.markInstanceSeen("webapp", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "LoadedServletCount":
			mx[fmt.Sprintf("webapp_%s_loaded_servlets", cleanInst)] = metric.Value
		case "ReloadCount":
			mx[fmt.Sprintf("webapp_%s_reloads", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "webapp", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "application", Value: appName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"webapp",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "webapp", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "webapp", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "webapp", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "webapp", cleanInst, metric)
	}
	
	// Process child components (Servlets, Sessions)
	for _, child := range stat.SubStats {
		switch child.Name {
		case "Servlets":
			w.parseServletsContainer(&child, nodeName, serverName, appName, mx)
		}
	}
}

// parseServletsContainer handles servlets within a web application
func (w *WebSpherePMI) parseServletsContainer(stat *pmiStat, nodeName, serverName, appName string, mx map[string]int64) {
	// Process individual servlets
	for _, servlet := range stat.SubStats {
		w.parseServlet(&servlet, nodeName, serverName, appName, mx)
	}
}

// parseServlet handles individual servlet metrics
func (w *WebSpherePMI) parseServlet(stat *pmiStat, nodeName, serverName, appName string, mx map[string]int64) {
	servletName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s.%s", nodeName, serverName, appName, servletName)
	
	// Create charts if not exists
	w.ensureServletCharts(instance, nodeName, serverName, appName, servletName)
	
	// Mark as seen
	w.markInstanceSeen("servlet", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "RequestCount":
			mx[fmt.Sprintf("servlet_%s_requests", cleanInst)] = metric.Value
		case "ErrorCount":
			mx[fmt.Sprintf("servlet_%s_errors", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "servlet", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "application", Value: appName},
		{Key: "servlet", Value: servletName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		// Process all TimeStatistics with the smart processor
		w.processTimeStatisticWithContext(
			"servlet",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "ConcurrentRequests":
			mx[fmt.Sprintf("servlet_%s_concurrent", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "servlet", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "servlet", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "servlet", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "servlet", cleanInst, metric)
	}
	
}

// parseSessionManagerContainer handles the Servlet Session Manager
func (w *WebSpherePMI) parseSessionManagerContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	// Container-level instance (for global session metrics if they exist at this level)
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Only process container-level session metrics if there are no children
	// (indicating this is a direct session manager, not just a container)
	if len(stat.SubStats) == 0 {
		// This is a direct session manager - process its metrics
		w.parseSessionMetrics(stat, instance, nodeName, serverName, "global", mx)
	} else {
		// Process container-level metrics if any exist
		if len(stat.CountStatistics) > 0 || len(stat.TimeStatistics) > 0 || len(stat.RangeStatistics) > 0 ||
		   len(stat.BoundedRangeStatistics) > 0 || len(stat.AverageStatistics) > 0 || len(stat.DoubleStatistics) > 0 {
			// Container-level session metrics belong in web/containers family
			w.parseContainerSessionMetrics(stat, instance, nodeName, serverName, mx)
		}
	}
	
	// Process per-application session managers (children)
	for _, child := range stat.SubStats {
		if strings.Contains(child.Name, "#") || strings.Contains(child.Name, ".war") {
			// Application-specific session manager (both # and .war formats)
			appInstance := fmt.Sprintf("%s.%s", instance, child.Name)
			w.parseSessionMetrics(&child, appInstance, nodeName, serverName, child.Name, mx)
		}
	}
}

// parseSessionMetrics handles session manager metrics
func (w *WebSpherePMI) parseSessionMetrics(stat *pmiStat, instance, nodeName, serverName, appName string, mx map[string]int64) {
	// Create charts if not exists
	w.ensureSessionCharts(instance, nodeName, serverName, appName)
	
	// Mark as seen
	w.markInstanceSeen("sessions", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CreateCount":
			mx[fmt.Sprintf("sessions_%s_created", cleanInst)] = metric.Value
		case "InvalidateCount":
			mx[fmt.Sprintf("sessions_%s_invalidated", cleanInst)] = metric.Value
		case "TimeoutInvalidationCount":
			mx[fmt.Sprintf("sessions_%s_timeout_invalidated", cleanInst)] = metric.Value
		case "NoRoomForNewSessionCount":
			mx[fmt.Sprintf("sessions_%s_rejected", cleanInst)] = metric.Value
		case "ActivateNonExistSessionCount":
			mx[fmt.Sprintf("sessions_%s_activate_nonexist", cleanInst)] = metric.Value
		case "AffinityBreakCount":
			mx[fmt.Sprintf("sessions_%s_affinity_break", cleanInst)] = metric.Value
		case "CacheDiscardCount":
			mx[fmt.Sprintf("sessions_%s_cache_discard", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "sessions", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "application", Value: appName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		// Process all TimeStatistics with the smart processor
		// Note: SessionObjectSize can appear as both TimeStatistic (app-level) and AverageStatistic (container-level)
		// We process both to get full coverage
		w.processTimeStatisticWithContext(
			"sessions",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "ActiveCount":
			mx[fmt.Sprintf("sessions_%s_active", cleanInst)] = metric.Current
		case "LiveCount":
			mx[fmt.Sprintf("sessions_%s_live", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "sessions", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "sessions", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		// Note: SessionObjectSize at container level is handled in parseContainerSessionMetrics
		w.collectAverageMetric(mx, "sessions", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "sessions", cleanInst, metric)
	}
}

// parseContainerSessionMetrics handles container-level session metrics
// These belong in the web/containers family, not web/sessions
func (w *WebSpherePMI) parseContainerSessionMetrics(stat *pmiStat, instance, nodeName, serverName string, mx map[string]int64) {
	// Create charts if not exists
	w.ensureWebAppContainerSessionCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("webapp_container_sessions", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// For container-level session metrics, use webapp_container prefix
	// This ensures they appear in web/containers family
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "webapp_container_sessions", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "application", Value: "_container_"},  // Add placeholder for consistency with app-level sessions
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		// Process all TimeStatistics with the smart processor
		w.processTimeStatisticWithContext(
			"webapp_container_sessions",
			cleanInst,
			labels,
			metric,
			mx,
			3450+i*10, // Priority in web/containers range
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "webapp_container_sessions", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "webapp_container_sessions", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for i, metric := range avgMetrics {
		switch metric.Name {
		case "SessionObjectSize":
			// Use smart processor with context-aware routing for SessionObjectSize
			w.processAverageStatisticWithContext(
				"webapp_container_sessions",
				cleanInst,
				labels,
				metric,
				mx,
				10+i*10, // Priority offset
				"bytes",  // SessionObjectSize is measured in bytes
			)
		case "ExternalReadSize":
			// Use smart processor with context-aware routing for ExternalReadSize
			w.processAverageStatisticWithContext(
				"webapp_container_sessions",
				cleanInst,
				labels,
				metric,
				mx,
				20+i*10, // Priority offset
				"bytes",  // ExternalReadSize is measured in bytes
			)
		case "ExternalWriteSize":
			// Use smart processor with context-aware routing for ExternalWriteSize
			w.processAverageStatisticWithContext(
				"webapp_container_sessions",
				cleanInst,
				labels,
				metric,
				mx,
				30+i*10, // Priority offset
				"bytes",  // ExternalWriteSize is measured in bytes
			)
		default:
			// Use collection helper for unknown average metrics
			w.collectAverageMetric(mx, "webapp_container_sessions", cleanInst, metric)
		}
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "webapp_container_sessions", cleanInst, metric)
	}
}

// parseCacheComponent handles standalone cache components like "Object Cache" and "Counters"
func (w *WebSpherePMI) parseCacheComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("cache", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "HitsInMemoryCount":
			mx[fmt.Sprintf("cache_%s_memory_hits", cleanInst)] = metric.Value
		case "HitsOnDiskCount":
			mx[fmt.Sprintf("cache_%s_disk_hits", cleanInst)] = metric.Value
		case "MissCount":
			mx[fmt.Sprintf("cache_%s_misses", cleanInst)] = metric.Value
		case "ClientRequestCount":
			mx[fmt.Sprintf("cache_%s_client_requests", cleanInst)] = metric.Value
		case "InMemoryAndDiskCacheEntryCount":
			mx[fmt.Sprintf("cache_%s_total_entries", cleanInst)] = metric.Value
		case "ExplicitInvalidationCount":
			mx[fmt.Sprintf("cache_%s_explicit_invalidations", cleanInst)] = metric.Value
		case "LruInvalidationCount":
			mx[fmt.Sprintf("cache_%s_lru_invalidations", cleanInst)] = metric.Value
		case "TimeoutInvalidationCount":
			mx[fmt.Sprintf("cache_%s_timeout_invalidations", cleanInst)] = metric.Value
		case "RemoteHitCount":
			mx[fmt.Sprintf("cache_%s_remote_hits", cleanInst)] = metric.Value
		case "DistributedRequestCount":
			mx[fmt.Sprintf("cache_%s_distributed_requests", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "cache", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"cache",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "cache", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "cache", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "cache", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "cache", cleanInst, metric)
	}
}

// parseDynamicCacheContainer handles Dynamic Cache metrics
func (w *WebSpherePMI) parseDynamicCacheContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("cache", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "MaxInMemoryCacheEntryCount":
			mx[fmt.Sprintf("cache_%s_max_entries", cleanInst)] = metric.Value
		case "InMemoryCacheEntryCount":
			mx[fmt.Sprintf("cache_%s_in_memory_entries", cleanInst)] = metric.Value
		case "HitsInMemoryCount":
			mx[fmt.Sprintf("cache_%s_memory_hits", cleanInst)] = metric.Value
		case "HitsOnDiskCount":
			mx[fmt.Sprintf("cache_%s_disk_hits", cleanInst)] = metric.Value
		case "MissCount":
			mx[fmt.Sprintf("cache_%s_misses", cleanInst)] = metric.Value
		case "ClientRequestCount":
			mx[fmt.Sprintf("cache_%s_client_requests", cleanInst)] = metric.Value
		case "InMemoryAndDiskCacheEntryCount":
			mx[fmt.Sprintf("cache_%s_total_entries", cleanInst)] = metric.Value
		case "RemoteHitCount":
			mx[fmt.Sprintf("cache_%s_remote_hits", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "cache", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"cache",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "cache", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "cache", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "cache", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "cache", cleanInst, metric)
	}
	
	// Process child cache objects
	for _, child := range stat.SubStats {
		if strings.HasPrefix(child.Name, "Object:") {
			w.parseCacheObject(&child, nodeName, serverName, mx)
		}
	}
}

// parseCacheObject handles individual cache object metrics
// XML Structure: Object: [cache_name] -> [direct metrics] + Object Cache -> [metrics and Counters substat]
func (w *WebSpherePMI) parseCacheObject(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	// cacheName := strings.TrimPrefix(stat.Name, "Object: ") // Not used - aggregating all cache objects
	// Use aggregated instance name for NIDL compliance
	instance := fmt.Sprintf("%s.%s.Object_Cache", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureObjectCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("object_cache", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Process direct CountStatistics at the top level
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "InMemoryCacheEntryCount":
			mx[fmt.Sprintf("object_cache_%s_objects", cleanInst)] = metric.Value
		case "MaxInMemoryCacheEntryCount":
			mx[fmt.Sprintf("object_cache_%s_max_objects", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "object_cache", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"object_cache",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "object_cache", cleanInst, metric)
	}
	
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "object_cache", cleanInst, metric)
	}
	
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "object_cache", cleanInst, metric)
	}
	
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "object_cache", cleanInst, metric)
	}
	
	// Process "Object Cache" sub-stat for additional metrics
	for _, child := range stat.SubStats {
		if child.Name == "Object Cache" {
			// Process direct metrics in Object Cache
			childCountMetrics := w.extractCountStatistics(child.CountStatistics)
			for _, metric := range childCountMetrics {
				// Skip metrics that are handled elsewhere to avoid duplicates
				switch metric.Name {
				case "HitsInMemoryCount", "HitsOnDiskCount", "MissCount", 
				     "InMemoryAndDiskCacheEntryCount", "RemoteHitCount",
				     "ClientRequestCount", "DistributedRequestCount",
				     "ExplicitInvalidationCount", "ExplicitDiskInvalidationCount",
				     "ExplicitMemoryInvalidationCount", "LocalExplicitInvalidationCount",
				     "RemoteExplicitInvalidationCount", "LruInvalidationCount",
				     "TimeoutInvalidationCount", "RemoteCreationCount":
					// These are handled in the Counters sub-stat
					continue
				default:
					w.collectCountMetric(mx, "object_cache", cleanInst, metric)
				}
			}
			
			// Look for "Counters" sub-stat
			for _, counters := range child.SubStats {
				if counters.Name == "Counters" {
					// Process counter metrics
					counterMetrics := w.extractCountStatistics(counters.CountStatistics)
					for _, metric := range counterMetrics {
						switch metric.Name {
						case "HitsInMemoryCount":
							mx[fmt.Sprintf("object_cache_%s_memory_hits", cleanInst)] = metric.Value
						case "HitsOnDiskCount":
							mx[fmt.Sprintf("object_cache_%s_disk_hits", cleanInst)] = metric.Value
						case "MissCount":
							mx[fmt.Sprintf("object_cache_%s_misses", cleanInst)] = metric.Value
						case "InMemoryAndDiskCacheEntryCount":
							mx[fmt.Sprintf("object_cache_%s_total_entries", cleanInst)] = metric.Value
							mx[fmt.Sprintf("object_cache_%s_entries_memory_and_disk", cleanInst)] = metric.Value
						case "RemoteHitCount":
							mx[fmt.Sprintf("object_cache_%s_remote_hits", cleanInst)] = metric.Value
						case "ClientRequestCount":
							mx[fmt.Sprintf("object_cache_%s_client_requests", cleanInst)] = metric.Value
						case "DistributedRequestCount":
							mx[fmt.Sprintf("object_cache_%s_distributed_requests", cleanInst)] = metric.Value
						case "ExplicitInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_explicit_invalidations", cleanInst)] = metric.Value
						case "ExplicitDiskInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_explicit_disk_invalidations", cleanInst)] = metric.Value
						case "ExplicitMemoryInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_explicit_memory_invalidations", cleanInst)] = metric.Value
						case "LocalExplicitInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_local_explicit_invalidations", cleanInst)] = metric.Value
						case "RemoteExplicitInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_remote_explicit_invalidations", cleanInst)] = metric.Value
						case "LruInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_lru_invalidations", cleanInst)] = metric.Value
						case "TimeoutInvalidationCount":
							mx[fmt.Sprintf("object_cache_%s_timeout_invalidations", cleanInst)] = metric.Value
						case "RemoteCreationCount":
							mx[fmt.Sprintf("object_cache_%s_remote_creations", cleanInst)] = metric.Value
						default:
							// For unknown count metrics, use collection helper
							w.collectCountMetric(mx, "object_cache", cleanInst, metric)
						}
					}
					
					// Also process any other statistic types in Counters with smart processor
					counterTimeMetrics := w.extractTimeStatistics(counters.TimeStatistics)
					for j, metric := range counterTimeMetrics {
						w.processTimeStatisticWithContext(
							"object_cache",
							cleanInst,
							labels,
							metric,
							mx,
							200+j*10, // Higher priority offset for counter metrics
						)
					}
					
					counterRangeMetrics := w.extractRangeStatistics(counters.RangeStatistics)
					for _, metric := range counterRangeMetrics {
						w.collectRangeMetric(mx, "object_cache", cleanInst, metric)
					}
					
					counterBoundedMetrics := w.extractBoundedRangeStatistics(counters.BoundedRangeStatistics)
					for _, metric := range counterBoundedMetrics {
						w.collectBoundedRangeMetric(mx, "object_cache", cleanInst, metric)
					}
					
					counterAvgMetrics := w.extractAverageStatistics(counters.AverageStatistics)
					for _, metric := range counterAvgMetrics {
						w.collectAverageMetric(mx, "object_cache", cleanInst, metric)
					}
					
					counterDoubleMetrics := w.extractDoubleStatistics(counters.DoubleStatistics)
					for _, metric := range counterDoubleMetrics {
						w.collectDoubleMetric(mx, "object_cache", cleanInst, metric)
					}
				}
			}
		}
	}
}

// parseORB handles ORB (Object Request Broker) metrics
func (w *WebSpherePMI) parseORB(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureORBCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("orb", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "RequestCount":
			mx[fmt.Sprintf("orb_%s_requests", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "orb", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		// Process all TimeStatistics with the smart processor
		w.processTimeStatisticWithContext(
			"orb",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "ConcurrentRequestCount":
			mx[fmt.Sprintf("orb_%s_concurrent_requests", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "orb", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "orb", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "orb", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "orb", cleanInst, metric)
	}
}

// Chart creation helpers

// Helper to create chart labels with version information
func (w *WebSpherePMI) createChartLabels(labels ...module.Label) []module.Label {
	return append(labels, w.getVersionLabels()...)
}

func (w *WebSpherePMI) ensureWebAppCharts(instance, nodeName, serverName, appName string) {
	chartKey := fmt.Sprintf("webapp_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create web app charts
		charts := webAppChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "application", Value: appName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureServletCharts(instance, nodeName, serverName, appName, servletName string) {
	chartKey := fmt.Sprintf("servlet_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create servlet charts
		charts := servletChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "application", Value: appName},
				module.Label{Key: "servlet", Value: servletName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureSessionCharts(instance, nodeName, serverName, appName string) {
	chartKey := fmt.Sprintf("sessions_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create session charts
		charts := sessionChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "application", Value: appName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureORBCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("orb_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create ORB charts
		charts := orbChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureCacheCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("cache_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create cache charts
		charts := cacheChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureCacheObjectCharts(instance, nodeName, serverName, cacheName string) {
	chartKey := fmt.Sprintf("cache_object_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create cache object charts
		charts := cacheObjectChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "cache", Value: cacheName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureWebAppContainerCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("webapp_container_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create webapp container charts
		charts := webAppContainerChartsTmpl.Copy()
		family, _ := getContextMetadata("webapp_container")
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Fam = family  // Use correct family from context metadata
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureObjectPoolCharts(instance, nodeName, serverName, poolName string) {
	chartKey := fmt.Sprintf("object_pool_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create object pool charts
		charts := objectPoolChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "pool", Value: poolName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureWebAppContainerSessionCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("webapp_container_sessions_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		cleanInst := w.cleanID(instance)
		
		// Get correct family for webapp_container_sessions
		family, _ := getContextMetadata("webapp_container_sessions")
		
		// Create container session stats chart
		chartStats := &module.Chart{
			ID:       fmt.Sprintf("webapp_container_sessions_%s_stats", cleanInst),
			Title:    "Container Session Statistics",
			Units:    "sessions",
			Fam:      family,
			Ctx:      "websphere_pmi.webapp_container_sessions_stats",
			Type:     module.Line,
			Priority: prioWebApps + 100,
			Dims: module.Dims{
				{ID: fmt.Sprintf("webapp_container_sessions_%s_ActiveCount_current", cleanInst), Name: "active_current"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_ActiveCount_high_watermark", cleanInst), Name: "active_high_watermark"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_ActiveCount_low_watermark", cleanInst), Name: "active_low_watermark"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_ActiveCount_mean", cleanInst), Name: "active_mean"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_ActiveCount_integral", cleanInst), Name: "active_integral"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_LiveCount_current", cleanInst), Name: "live_current"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_LiveCount_high_watermark", cleanInst), Name: "live_high_watermark"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_LiveCount_low_watermark", cleanInst), Name: "live_low_watermark"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_LiveCount_mean", cleanInst), Name: "live_mean"},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_LiveCount_integral", cleanInst), Name: "live_integral"},
			},
			Labels: append([]module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}, w.getVersionLabels()...),
		}
		
		// Create container session lifecycle chart
		chartLifecycle := &module.Chart{
			ID:       fmt.Sprintf("webapp_container_sessions_%s_lifecycle", cleanInst),
			Title:    "Container Session Lifecycle",
			Units:    "sessions/s",
			Fam:      family,
			Ctx:      "websphere_pmi.webapp_container_sessions_lifecycle",
			Type:     module.Line,
			Priority: prioWebApps + 101,
			Dims: module.Dims{
				{ID: fmt.Sprintf("webapp_container_sessions_%s_CreateCount", cleanInst), Name: "created", Algo: module.Incremental},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_InvalidateCount", cleanInst), Name: "invalidated", Algo: module.Incremental},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_TimeoutInvalidationCount", cleanInst), Name: "timeout", Algo: module.Incremental},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_NoRoomForNewSessionCount", cleanInst), Name: "rejected", Algo: module.Incremental},
			},
			Labels: append([]module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}, w.getVersionLabels()...),
		}
		
		// Create container session errors chart
		chartErrors := &module.Chart{
			ID:       fmt.Sprintf("webapp_container_sessions_%s_errors", cleanInst),
			Title:    "Container Session Errors",
			Units:    "errors/s",
			Fam:      family,
			Ctx:      "websphere_pmi.webapp_container_sessions_errors",
			Type:     module.Line,
			Priority: prioWebApps + 102,
			Dims: module.Dims{
				{ID: fmt.Sprintf("webapp_container_sessions_%s_ActivateNonExistSessionCount", cleanInst), Name: "activate_nonexist", Algo: module.Incremental},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_AffinityBreakCount", cleanInst), Name: "affinity_break", Algo: module.Incremental},
				{ID: fmt.Sprintf("webapp_container_sessions_%s_CacheDiscardCount", cleanInst), Name: "cache_discard", Algo: module.Incremental},
			},
			Labels: append([]module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}, w.getVersionLabels()...),
		}
		
		// Note: ExternalReadSize and ExternalWriteSize charts are now created dynamically
		// by processAverageStatisticWithContext with smart family routing
		
		// Add charts
		if err := w.charts.Add(chartStats); err != nil {
			w.Warning(err)
		}
		if err := w.charts.Add(chartLifecycle); err != nil {
			w.Warning(err)
		}
		if err := w.charts.Add(chartErrors); err != nil {
			w.Warning(err)
		}
		// Note: chartReadSize and chartWriteSize removed - created dynamically by processAverageStatisticWithContext
	}
}

func (w *WebSpherePMI) ensureObjectCacheCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("object_cache_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create object cache charts
		charts := objectCacheChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// Add chart templates for the new types
var objectPoolChartsTmpl = module.Charts{
	{
		ID:       "object_pool_%s_objects",
		Title:    "Object Pool Objects",
		Units:    "objects",
		Fam:      "performance/pools",
		Ctx:      "websphere_pmi.object_pool_objects",
		Type:     module.Line,
		Priority: prioObjectPools,
		Dims: module.Dims{
			{ID: "object_pool_%s_idle", Name: "idle"},
		},
	},
	{
		ID:       "object_pool_%s_lifecycle",
		Title:    "Object Pool Lifecycle",
		Units:    "objects/s",
		Fam:      "performance/pools",
		Ctx:      "websphere_pmi.object_pool_lifecycle",
		Type:     module.Line,
		Priority: prioObjectPools + 10,
		Dims: module.Dims{
			{ID: "object_pool_%s_created", Name: "created", Algo: module.Incremental},
			{ID: "object_pool_%s_allocated", Name: "allocated", Algo: module.Incremental},
			{ID: "object_pool_%s_returned", Name: "returned", Algo: module.Incremental},
		},
	},
}

var objectCacheChartsTmpl = module.Charts{
	{
		ID:       "object_cache_%s_objects",
		Title:    "Object Cache Objects",
		Units:    "objects",
		Fam:      "performance/caching/object",
		Ctx:      "websphere_pmi.object_cache_objects",
		Type:     module.Line,
		Priority: prioObjectCache,
		Dims: module.Dims{
			{ID: "object_cache_%s_objects", Name: "current"},
			{ID: "object_cache_%s_max_objects", Name: "max"},
			{ID: "object_cache_%s_total_entries", Name: "total_entries"},
		},
	},
	{
		ID:       "object_cache_%s_hits",
		Title:    "Object Cache Hits",
		Units:    "hits/s",
		Fam:      "performance/caching/object",
		Ctx:      "websphere_pmi.object_cache_hits",
		Type:     module.Line,
		Priority: prioObjectCache + 10,
		Dims: module.Dims{
			{ID: "object_cache_%s_memory_hits", Name: "memory_hits", Algo: module.Incremental},
			{ID: "object_cache_%s_disk_hits", Name: "disk_hits", Algo: module.Incremental},
			{ID: "object_cache_%s_misses", Name: "misses", Algo: module.Incremental},
			{ID: "object_cache_%s_remote_hits", Name: "remote_hits", Algo: module.Incremental},
		},
	},
	{
		ID:       "object_cache_%s_requests",
		Title:    "Object Cache Requests",
		Units:    "requests/s",
		Fam:      "performance/caching/object",
		Ctx:      "websphere_pmi.object_cache_requests",
		Type:     module.Line,
		Priority: prioObjectCache + 20,
		Dims: module.Dims{
			{ID: "object_cache_%s_client_requests", Name: "client_requests", Algo: module.Incremental},
			{ID: "object_cache_%s_distributed_requests", Name: "distributed_requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "object_cache_%s_invalidations",
		Title:    "Object Cache Invalidations",
		Units:    "invalidations/s",
		Fam:      "performance/caching/object",
		Ctx:      "websphere_pmi.object_cache_invalidations",
		Type:     module.Line,
		Priority: prioObjectCache + 30,
		Dims: module.Dims{
			{ID: "object_cache_%s_explicit_invalidations", Name: "explicit", Algo: module.Incremental},
			{ID: "object_cache_%s_explicit_disk_invalidations", Name: "explicit_disk", Algo: module.Incremental},
			{ID: "object_cache_%s_explicit_memory_invalidations", Name: "explicit_memory", Algo: module.Incremental},
			{ID: "object_cache_%s_local_explicit_invalidations", Name: "local_explicit", Algo: module.Incremental},
			{ID: "object_cache_%s_remote_explicit_invalidations", Name: "remote_explicit", Algo: module.Incremental},
			{ID: "object_cache_%s_lru_invalidations", Name: "lru", Algo: module.Incremental},
			{ID: "object_cache_%s_timeout_invalidations", Name: "timeout", Algo: module.Incremental},
		},
	},
	{
		ID:       "object_cache_%s_entries_total",
		Title:    "Object Cache Total Entries",
		Units:    "entries",
		Fam:      "performance/caching/object",
		Ctx:      "websphere_pmi.object_cache_entries_total",
		Type:     module.Line,
		Priority: prioObjectCache + 40,
		Dims: module.Dims{
			{ID: "object_cache_%s_entries_memory_and_disk", Name: "memory_and_disk", Algo: module.Incremental},
		},
	},
	{
		ID:       "object_cache_%s_remote_operations",
		Title:    "Object Cache Remote Operations",
		Units:    "operations/s",
		Fam:      "performance/caching/object",
		Ctx:      "websphere_pmi.object_cache_remote_operations",
		Type:     module.Line,
		Priority: prioObjectCache + 50,
		Dims: module.Dims{
			{ID: "object_cache_%s_remote_creations", Name: "remote_creation", Algo: module.Incremental},
		},
	},
}

var webAppContainerChartsTmpl = module.Charts{
	{
		ID:       "webapp_container_%s_servlets",
		Title:    "Web Application Container Servlets",
		Units:    "servlets",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_servlets",
		Type:     module.Line,
		Priority: prioWebApps + 10,
		Dims: module.Dims{
			{ID: "webapp_container_%s_loaded_servlets", Name: "loaded"},
		},
	},
	{
		ID:       "webapp_container_%s_reloads",
		Title:    "Web Application Container Reloads",
		Units:    "reloads/s",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_reloads",
		Type:     module.Line,
		Priority: prioWebApps + 11,
		Dims: module.Dims{
			{ID: "webapp_container_%s_ReloadCount", Name: "reloads", Algo: module.Incremental},
		},
	},
	{
		ID:       "webapp_container_%s_requests",
		Title:    "Web Application Container Requests",
		Units:    "requests/s",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_requests",
		Type:     module.Line,
		Priority: prioWebApps + 30,
		Dims: module.Dims{
			{ID: "webapp_container_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "webapp_container_%s_errors", Name: "errors", Algo: module.Incremental},
			{ID: "webapp_container_%s_URIRequestCount", Name: "uri_requests", Algo: module.Incremental},
		},
	},
	// New charts for discovered metrics
	{
		ID:       "webapp_container_%s_concurrent_requests",
		Title:    "Web Application Container Concurrent Requests",
		Units:    "requests",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_concurrent_requests",
		Type:     module.Line,
		Priority: prioWebApps + 31,
		Dims: module.Dims{
			{ID: "webapp_container_%s_ConcurrentRequests_current", Name: "current"},
			{ID: "webapp_container_%s_ConcurrentRequests_mean", Name: "mean", Div: precision},
			{ID: "webapp_container_%s_URIConcurrentRequests_current", Name: "uri_current"},
			{ID: "webapp_container_%s_URIConcurrentRequests_mean", Name: "uri_mean", Div: precision},
			{ID: "webapp_container_%s_ConcurrentRequests_high_watermark", Name: "high_watermark", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_ConcurrentRequests_low_watermark", Name: "low_watermark", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_ConcurrentRequests_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_URIConcurrentRequests_high_watermark", Name: "uri_high_watermark", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_URIConcurrentRequests_low_watermark", Name: "uri_low_watermark", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_URIConcurrentRequests_integral", Name: "uri_integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	// Portlet-specific charts
	{
		ID:       "webapp_container_%s_portlets",
		Title:    "Web Application Container Portlets",
		Units:    "portlets",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_portlets",
		Type:     module.Line,
		Priority: prioWebApps + 33,
		Dims: module.Dims{
			{ID: "webapp_container_%s_Number_of_loaded_portlets", Name: "loaded"},
		},
	},
	{
		ID:       "webapp_container_%s_portlet_requests",
		Title:    "Web Application Container Portlet Requests",
		Units:    "requests/s",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_portlet_requests",
		Type:     module.Line,
		Priority: prioWebApps + 34,
		Dims: module.Dims{
			{ID: "webapp_container_%s_Number_of_portlet_requests", Name: "requests", Algo: module.Incremental},
			{ID: "webapp_container_%s_Number_of_portlet_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "webapp_container_%s_portlet_concurrent",
		Title:    "Web Application Container Concurrent Portlet Requests",
		Units:    "requests",
		Fam:      "web/containers",
		Ctx:      "websphere_pmi.webapp_container_portlet_concurrent",
		Type:     module.Line,
		Priority: prioWebApps + 35,
		Dims: module.Dims{
			{ID: "webapp_container_%s_Number_of_concurrent_portlet_requests_current", Name: "current"},
			{ID: "webapp_container_%s_Number_of_concurrent_portlet_requests_mean", Name: "mean", Div: precision},
			{ID: "webapp_container_%s_Number_of_concurrent_portlet_requests_high_watermark", Name: "high_watermark", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_Number_of_concurrent_portlet_requests_low_watermark", Name: "low_watermark", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "webapp_container_%s_Number_of_concurrent_portlet_requests_integral", Name: "integral", Div: precision, DimOpts: module.DimOpts{Hidden: true}},
		},
	},
}

var cacheChartsTmpl = module.Charts{
	{
		ID:       "cache_%s_entries",
		Title:    "Dynamic Cache Entries",
		Units:    "entries",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_entries",
		Type:     module.Line,
		Priority: prioDynaCache,
		Dims: module.Dims{
			{ID: "cache_%s_in_memory_entries", Name: "in_memory"},
			{ID: "cache_%s_total_entries", Name: "total"},
			{ID: "cache_%s_max_entries", Name: "max"},
		},
	},
	{
		ID:       "cache_%s_performance",
		Title:    "Dynamic Cache Performance",
		Units:    "operations/s",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_performance",
		Type:     module.Line,
		Priority: prioDynaCache + 10,
		Dims: module.Dims{
			{ID: "cache_%s_memory_hits", Name: "memory_hit", Algo: module.Incremental},
			{ID: "cache_%s_disk_hits", Name: "disk_hit", Algo: module.Incremental},
			{ID: "cache_%s_remote_hits", Name: "remote_hit", Algo: module.Incremental},
			{ID: "cache_%s_misses", Name: "miss", Algo: module.Incremental},
		},
	},
	{
		ID:       "cache_%s_requests",
		Title:    "Dynamic Cache Requests",
		Units:    "requests/s",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_requests",
		Type:     module.Line,
		Priority: prioDynaCache + 10 + 1,
		Dims: module.Dims{
			{ID: "cache_%s_client_requests", Name: "client", Algo: module.Incremental},
			{ID: "cache_%s_distributed_requests", Name: "distributed", Algo: module.Incremental},
		},
	},
	{
		ID:       "cache_%s_invalidations",
		Title:    "Dynamic Cache Invalidations",
		Units:    "invalidations/s",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_invalidations",
		Type:     module.Line,
		Priority: prioDynaCache + 10 + 2,
		Dims: module.Dims{
			{ID: "cache_%s_explicit_invalidations", Name: "explicit", Algo: module.Incremental},
			{ID: "cache_%s_lru_invalidations", Name: "lru", Algo: module.Incremental},
			{ID: "cache_%s_timeout_invalidations", Name: "timeout", Algo: module.Incremental},
		},
	},
	{
		ID:       "cache_%s_detailed_invalidations",
		Title:    "Dynamic Cache Detailed Invalidations",
		Units:    "invalidations/s",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_detailed_invalidations",
		Type:     module.Line,
		Priority: prioDynaCache + 10 + 3,
		Dims: module.Dims{
			{ID: "cache_%s_ExplicitDiskInvalidationCount", Name: "explicit_disk", Algo: module.Incremental},
			{ID: "cache_%s_ExplicitMemoryInvalidationCount", Name: "explicit_memory", Algo: module.Incremental},
			{ID: "cache_%s_LocalExplicitInvalidationCount", Name: "local_explicit", Algo: module.Incremental},
			{ID: "cache_%s_RemoteExplicitInvalidationCount", Name: "remote_explicit", Algo: module.Incremental},
		},
	},
	{
		ID:       "cache_%s_remote_operations",
		Title:    "Dynamic Cache Remote Operations",
		Units:    "operations/s",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_remote_operations",
		Type:     module.Line,
		Priority: prioDynaCache + 10 + 4,
		Dims: module.Dims{
			{ID: "cache_%s_RemoteCreationCount", Name: "remote_creation", Algo: module.Incremental},
		},
	},
}

var cacheObjectChartsTmpl = module.Charts{
	{
		ID:       "cache_object_%s_entries",
		Title:    "Cache Object Entries",
		Units:    "entries",
		Fam:      "performance/caching/dynamic",
		Ctx:      "websphere_pmi.cache_object_entries",
		Type:     module.Line,
		Priority: prioDynaCache + 20,
		Dims: module.Dims{
			{ID: "cache_object_%s_entries", Name: "current"},
			{ID: "cache_object_%s_max_entries", Name: "max"},
		},
	},
}

var webAppChartsTmpl = module.Charts{
	{
		ID:       "webapp_%s_servlets",
		Title:    "Web Application Servlets",
		Units:    "servlets",
		Fam:      "web/applications",
		Ctx:      "websphere_pmi.webapp_servlets",
		Type:     module.Line,
		Priority: prioWebApps,
		Dims: module.Dims{
			{ID: "webapp_%s_loaded_servlets", Name: "loaded"},
		},
	},
	{
		ID:       "webapp_%s_reloads",
		Title:    "Web Application Reloads",
		Units:    "reloads/s",
		Fam:      "web/applications",
		Ctx:      "websphere_pmi.webapp_reloads",
		Type:     module.Line,
		Priority: prioWebApps + 50,
		Dims: module.Dims{
			{ID: "webapp_%s_reloads", Name: "reloads", Algo: module.Incremental},
		},
	},
}

var servletChartsTmpl = module.Charts{
	{
		ID:       "servlet_%s_requests",
		Title:    "Servlet Requests",
		Units:    "requests/s",
		Fam:      "web/servlets/instances",
		Ctx:      "websphere_pmi.servlet_requests",
		Type:     module.Line,
		Priority: prioWebServlets + 100,
		Dims: module.Dims{
			{ID: "servlet_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "servlet_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlet_%s_concurrent",
		Title:    "Servlet Concurrent Requests",
		Units:    "requests",
		Fam:      "web/servlets/instances",
		Ctx:      "websphere_pmi.servlet_concurrent",
		Type:     module.Line,
		Priority: prioWebServlets + 120 + 1,
		Dims: module.Dims{
			{ID: "servlet_%s_concurrent", Name: "concurrent"},
		},
	},
}

var sessionChartsTmpl = module.Charts{
	{
		ID:       "sessions_%s_active",
		Title:    "Active Sessions",
		Units:    "sessions",
		Fam:      "web/sessions/application",
		Ctx:      "websphere_pmi.sessions_active",
		Type:     module.Line,
		Priority: prioWebSessions,
		Dims: module.Dims{
			{ID: "sessions_%s_active", Name: "active"},
			{ID: "sessions_%s_live", Name: "live"},
		},
	},
	{
		ID:       "sessions_%s_lifecycle",
		Title:    "Session Lifecycle",
		Units:    "sessions/s",
		Fam:      "web/sessions/application",
		Ctx:      "websphere_pmi.sessions_lifecycle",
		Type:     module.Line,
		Priority: prioWebSessions + 10,
		Dims: module.Dims{
			{ID: "sessions_%s_created", Name: "created", Algo: module.Incremental},
			{ID: "sessions_%s_invalidated", Name: "invalidated", Algo: module.Incremental},
			{ID: "sessions_%s_timeout_invalidated", Name: "timeout", Algo: module.Incremental},
			{ID: "sessions_%s_rejected", Name: "rejected", Algo: module.Incremental},
		},
	},
	{
		ID:       "sessions_%s_errors",
		Title:    "Session Errors",
		Units:    "errors/s",
		Fam:      "web/sessions/application",
		Ctx:      "websphere_pmi.sessions_errors",
		Type:     module.Line,
		Priority: prioWebSessions + 10 + 1,
		Dims: module.Dims{
			{ID: "sessions_%s_activate_nonexist", Name: "activate_nonexist", Algo: module.Incremental},
			{ID: "sessions_%s_affinity_break", Name: "affinity_break", Algo: module.Incremental},
			{ID: "sessions_%s_cache_discard", Name: "cache_discard", Algo: module.Incremental},
		},
	},
	// New charts for discovered AverageStatistics
	{
		ID:       "sessions_%s_external_read_size",
		Title:    "Session External Read Size",
		Units:    "bytes",
		Fam:      "web/sessions/application",
		Ctx:      "websphere_pmi.sessions_external_read_size",
		Type:     module.Line,
		Priority: prioWebSessions + 10 + 5,
		Dims: module.Dims{
			{ID: "sessions_%s_ExternalReadSize_mean", Name: "mean"},
			{ID: "sessions_%s_ExternalReadSize_total", Name: "total", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalReadSize_count", Name: "count", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalReadSize_min", Name: "min", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalReadSize_max", Name: "max", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalReadSize_sum_of_squares", Name: "sum_of_squares", DimOpts: module.DimOpts{Hidden: true}},
		},
	},
	{
		ID:       "sessions_%s_external_write_size",
		Title:    "Session External Write Size",
		Units:    "bytes",
		Fam:      "web/sessions/application",
		Ctx:      "websphere_pmi.sessions_external_write_size",
		Type:     module.Line,
		Priority: prioWebSessions + 10 + 6,
		Dims: module.Dims{
			{ID: "sessions_%s_ExternalWriteSize_mean", Name: "mean"},
			{ID: "sessions_%s_ExternalWriteSize_total", Name: "total", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalWriteSize_count", Name: "count", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalWriteSize_min", Name: "min", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalWriteSize_max", Name: "max", DimOpts: module.DimOpts{Hidden: true}},
			{ID: "sessions_%s_ExternalWriteSize_sum_of_squares", Name: "sum_of_squares", DimOpts: module.DimOpts{Hidden: true}},
		},
	},
}

// Chart templates for new parsers
var jcaPoolChartsTmpl = module.Charts{
	{
		ID:       "jca_pool_%s_connections",
		Title:    "JCA Connection Pool Connections",
		Units:    "connections",
		Fam:      "connectivity/jca",
		Ctx:      "websphere_pmi.jca_pool_connections",
		Type:     module.Line,
		Priority: prioJCAPools,
		Dims: module.Dims{
			{ID: "jca_pool_%s_free", Name: "free"},
			{ID: "jca_pool_%s_size", Name: "total"},
		},
	},
	{
		ID:       "jca_pool_%s_lifecycle",
		Title:    "JCA Connection Pool Lifecycle",
		Units:    "connections/s",
		Fam:      "connectivity/jca",
		Ctx:      "websphere_pmi.jca_pool_lifecycle",
		Type:     module.Line,
		Priority: prioJCAPools + 60,
		Dims: module.Dims{
			{ID: "jca_pool_%s_created", Name: "created", Algo: module.Incremental},
			{ID: "jca_pool_%s_closed", Name: "closed", Algo: module.Incremental},
			{ID: "jca_pool_%s_allocated", Name: "allocated", Algo: module.Incremental},
			{ID: "jca_pool_%s_returned", Name: "returned", Algo: module.Incremental},
		},
	},
	{
		ID:       "jca_pool_%s_faults",
		Title:    "JCA Connection Pool Faults",
		Units:    "faults/s",
		Fam:      "connectivity/jca",
		Ctx:      "websphere_pmi.jca_pool_faults",
		Type:     module.Line,
		Priority: prioJCAPools + 2,
		Dims: module.Dims{
			{ID: "jca_pool_%s_faults", Name: "faults", Algo: module.Incremental},
		},
	},
	{
		ID:       "jca_pool_%s_managed_connections",
		Title:    "JCA Managed Connections",
		Units:    "connections",
		Fam:      "connectivity/jca",
		Ctx:      "websphere_pmi.jca_pool_managed_connections",
		Type:     module.Line,
		Priority: prioJCAPools + 3,
		Dims: module.Dims{
			{ID: "jca_pool_%s_managed_connections", Name: "managed"},
			{ID: "jca_pool_%s_connection_handles", Name: "handles"},
		},
	},
	{
		ID:       "jca_pool_%s_utilization",
		Title:    "JCA Connection Pool Utilization",
		Units:    "percentage",
		Fam:      "connectivity/jca",
		Ctx:      "websphere_pmi.jca_pool_utilization",
		Type:     module.Line,
		Priority: prioJCAPools + 4,
		Dims: module.Dims{
			{ID: "jca_pool_%s_percent_used", Name: "used"},
			{ID: "jca_pool_%s_percent_maxed", Name: "maxed"},
		},
	},
	{
		ID:       "jca_pool_%s_wait",
		Title:    "JCA Connection Pool Wait",
		Units:    "threads",
		Fam:      "connectivity/jca",
		Ctx:      "websphere_pmi.jca_pool_wait",
		Type:     module.Line,
		Priority: prioJCAPools + 5,
		Dims: module.Dims{
			{ID: "jca_pool_%s_waiting_threads", Name: "waiting"},
		},
	},
}

var enterpriseAppChartsTmpl = module.Charts{
	{
		ID:       "enterprise_app_%s_metrics",
		Title:    "Enterprise Application Metrics",
		Units:    "metrics",
		Fam:      "enterprise/apps",
		Ctx:      "websphere_pmi.enterprise_app_metrics",
		Type:     module.Line,
		Priority: prioEnterpriseApps,
		Dims: module.Dims{
			{ID: "enterprise_app_%s_startup_time", Name: "startup_time"},
			{ID: "enterprise_app_%s_loads", Name: "loads", Algo: module.Incremental},
		},
	},
}

var systemDataChartsTmpl = module.Charts{
	{
		ID:       "system_data_%s_cpu",
		Title:    "System CPU Usage",
		Units:    "percentage",
		Fam:      "system/cpu",
		Ctx:      "websphere_pmi.system_data_cpu",
		Type:     module.Line,
		Priority: prioSystemCPU,
		Dims: module.Dims{
			{ID: "system_data_%s_cpu_usage", Name: "current"},
			// NOTE: CPUUsageSinceServerStarted average is handled by smart processor creating separate charts
		},
	},
	{
		ID:       "system_data_%s_memory",
		Title:    "System Free Memory",
		Units:    "bytes",
		Fam:      "system/memory",
		Ctx:      "websphere_pmi.system_data_memory",
		Type:     module.Line,
		Priority: prioSystemCPU + 10,
		Dims: module.Dims{
			{ID: "system_data_%s_free_memory", Name: "free"},
		},
	},
}

var wlmChartsTmpl = module.Charts{
	{
		ID:       "wlm_%s_requests",
		Title:    "WLM Requests",
		Units:    "requests/s",
		Fam:      "wlm",
		Ctx:      "websphere_pmi.wlm_requests",
		Type:     module.Line,
		Priority: prioWLM,
		Dims: module.Dims{
			{ID: "wlm_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	},
}

var beanManagerChartsTmpl = module.Charts{
	{
		ID:       "bean_manager_%s_beans",
		Title:    "Bean Manager Live Beans",
		Units:    "beans",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.bean_manager_beans",
		Type:     module.Line,
		Priority: prioEJBContainer + 400,
		Dims: module.Dims{
			{ID: "bean_manager_%s_live_beans", Name: "live"},
		},
	},
}

var connectionManagerChartsTmpl = module.Charts{
	{
		ID:       "connection_manager_%s_connections",
		Title:    "Connection Manager Connections",
		Units:    "connections",
		Fam:      "connections/manager",
		Ctx:      "websphere_pmi.connection_manager_connections",
		Type:     module.Line,
		Priority: prioConnectionMgr,
		Dims: module.Dims{
			{ID: "connection_manager_%s_allocated", Name: "allocated"},
		},
	},
}

var jvmSubsystemChartsTmpl = module.Charts{
	{
		ID:       "jvm_subsystem_%s_metrics",
		Title:    "JVM Subsystem Metrics",
		Units:    "metrics",
		Fam:      "system/jvm",
		Ctx:      "websphere_pmi.jvm_subsystem_metrics",
		Type:     module.Line,
		Priority: prioSystemJVM,
		Dims: module.Dims{
			{ID: "jvm_gc_%s_heap_discarded", Name: "gc_heap_discarded"},
			{ID: "jvm_gc_%s_time", Name: "gc_time"},
			{ID: "jvm_memory_%s_allocated", Name: "memory_allocated"},
			{ID: "jvm_thread_%s_active", Name: "thread_active"},
		},
	},
}

var ejbContainerChartsTmpl = module.Charts{
	{
		ID:       "ejb_container_%s_methods",
		Title:    "EJB Container Method Calls",
		Units:    "calls/s",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.ejb_container_methods",
		Type:     module.Line,
		Priority: prioEJBContainer,
		Dims: module.Dims{
			{ID: "ejb_container_%s_method_calls", Name: "method_calls", Algo: module.Incremental},
		},
	},
}

var mdbChartsTmpl = module.Charts{
	{
		ID:       "mdb_%s_messages",
		Title:    "Message Driven Bean Messages",
		Units:    "messages/s",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.mdb_messages",
		Type:     module.Line,
		Priority: prioEJBContainer + 100,
		Dims: module.Dims{
			{ID: "mdb_%s_messages", Name: "messages", Algo: module.Incremental},
		},
	},
}

var sfsbChartsTmpl = module.Charts{
	{
		ID:       "sfsb_%s_instances",
		Title:    "Stateful Session Bean Instances",
		Units:    "instances",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.sfsb_instances",
		Type:     module.Line,
		Priority: prioEJBContainer + 300,
		Dims: module.Dims{
			{ID: "sfsb_%s_live", Name: "live"},
		},
	},
}

var slsbChartsTmpl = module.Charts{
	{
		ID:       "slsb_%s_methods",
		Title:    "Stateless Session Bean Method Calls",
		Units:    "calls/s",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.slsb_methods",
		Type:     module.Line,
		Priority: prioEJBContainer + 200,
		Dims: module.Dims{
			{ID: "slsb_%s_method_calls", Name: "method_calls", Algo: module.Incremental},
		},
	},
}

var entityBeanChartsTmpl = module.Charts{
	{
		ID:       "entity_bean_%s_lifecycle",
		Title:    "Entity Bean Lifecycle",
		Units:    "operations/s",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.entity_bean_lifecycle",
		Type:     module.Line,
		Priority: prioEJBContainer + 500,
		Dims: module.Dims{
			{ID: "entity_bean_%s_activations", Name: "activations", Algo: module.Incremental},
			{ID: "entity_bean_%s_passivations", Name: "passivations", Algo: module.Incremental},
		},
	},
}

var genericEJBChartsTmpl = module.Charts{
	{
		ID:       "generic_ejb_%s_operations",
		Title:    "Generic EJB Operations",
		Units:    "operations/s",
		Fam:      "integration/ejb/management",
		Ctx:      "websphere_pmi.generic_ejb_operations",
		Type:     module.Line,
		Priority: prioEJBContainer + 600,
		Dims: module.Dims{
			{ID: "generic_ejb_%s_invocations", Name: "invocations", Algo: module.Incremental},
			{ID: "generic_ejb_%s_creates", Name: "creates", Algo: module.Incremental},
			{ID: "generic_ejb_%s_removes", Name: "removes", Algo: module.Incremental},
		},
	},
}

var orbChartsTmpl = module.Charts{
	{
		ID:       "orb_%s_requests",
		Title:    "ORB Requests",
		Units:    "requests/s",
		Fam:      "integration/orb",
		Ctx:      "websphere_pmi.orb_requests",
		Type:     module.Line,
		Priority: prioORB,
		Dims: module.Dims{
			{ID: "orb_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "orb_%s_concurrent",
		Title:    "ORB Concurrent Requests",
		Units:    "requests",
		Fam:      "integration/orb",
		Ctx:      "websphere_pmi.orb_concurrent",
		Type:     module.Line,
		Priority: prioORB + 10,
		Dims: module.Dims{
			{ID: "orb_%s_concurrent_requests", Name: "concurrent"},
		},
	},
}

// parseJCAConnectionPool handles JCA Connection Pool metrics
func (w *WebSpherePMI) parseJCAConnectionPool(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	poolName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, poolName)
	
	// Create charts if not exists
	w.ensureJCAPoolCharts(instance, nodeName, serverName, poolName)
	
	// Mark as seen
	w.markInstanceSeen("jca_pool", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CreateCount":
			mx[fmt.Sprintf("jca_pool_%s_created", cleanInst)] = metric.Value
		case "CloseCount":
			mx[fmt.Sprintf("jca_pool_%s_closed", cleanInst)] = metric.Value
		case "AllocateCount":
			mx[fmt.Sprintf("jca_pool_%s_allocated", cleanInst)] = metric.Value
		case "FreedCount":
			mx[fmt.Sprintf("jca_pool_%s_returned", cleanInst)] = metric.Value
		case "FaultCount":
			mx[fmt.Sprintf("jca_pool_%s_faults", cleanInst)] = metric.Value
		case "ManagedConnectionCount":
			mx[fmt.Sprintf("jca_pool_%s_managed_connections", cleanInst)] = metric.Value
		case "ConnectionHandleCount":
			mx[fmt.Sprintf("jca_pool_%s_connection_handles", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "jca_pool", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "pool", Value: poolName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		// Process all TimeStatistics with the smart processor
		w.processTimeStatisticWithContext(
			"jca_pool",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "WaitingThreadCount":
			mx[fmt.Sprintf("jca_pool_%s_waiting_threads", cleanInst)] = metric.Current
		case "PercentUsed":
			mx[fmt.Sprintf("jca_pool_%s_percent_used", cleanInst)] = metric.Current
		case "PercentMaxed":
			mx[fmt.Sprintf("jca_pool_%s_percent_maxed", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "jca_pool", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		switch metric.Name {
		case "FreePoolSize":
			mx[fmt.Sprintf("jca_pool_%s_free", cleanInst)] = metric.Value
		case "PoolSize":
			mx[fmt.Sprintf("jca_pool_%s_size", cleanInst)] = metric.Value
		default:
			// For unknown bounded range metrics, use collection helper
			w.collectBoundedRangeMetric(mx, "jca_pool", cleanInst, metric)
		}
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "jca_pool", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "jca_pool", cleanInst, metric)
	}
}

// parseEnterpriseApplication handles Enterprise Application metrics
func (w *WebSpherePMI) parseEnterpriseApplication(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	appName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, appName)
	
	// Create charts if not exists
	w.ensureEnterpriseAppCharts(instance, nodeName, serverName, appName)
	
	// Mark as seen
	w.markInstanceSeen("enterprise_app", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "StartupTime":
			mx[fmt.Sprintf("enterprise_app_%s_startup_time", cleanInst)] = metric.Value
		case "LoadCount":
			mx[fmt.Sprintf("enterprise_app_%s_loads", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "enterprise_app", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "enterprise_app", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "enterprise_app", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "enterprise_app", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "enterprise_app", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "enterprise_app", cleanInst, metric)
	}
}

// parseSystemData handles System Data metrics
func (w *WebSpherePMI) parseSystemData(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSystemDataCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("system_data", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CPUUsageSinceLastMeasurement":
			mx[fmt.Sprintf("system_data_%s_cpu_usage", cleanInst)] = metric.Value
		case "FreeMemory":
			mx[fmt.Sprintf("system_data_%s_free_memory", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "system_data", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "system_data", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "system_data", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "system_data", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range avgMetrics {
		switch metric.Name {
		case "CPUUsageSinceServerStarted":
			// Use smart processor for CPU usage statistics
			w.processAverageStatistic(
				"websphere_pmi.system_data",
				"system/cpu",
				cleanInst,
				labels,
				metric,
				mx,
				1800+i*10, // Priority offset
				"percentage",  // CPUUsageSinceServerStarted is a percentage
			)
		default:
			// For unknown average metrics, use collection helper
			w.collectAverageMetric(mx, "system_data", cleanInst, metric)
		}
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "system_data", cleanInst, metric)
	}
}

// parseWLM handles Work Load Management metrics
func (w *WebSpherePMI) parseWLM(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWLMCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("wlm", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "RequestCount":
			mx[fmt.Sprintf("wlm_%s_requests", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "wlm", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "wlm", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "wlm", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "wlm", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "wlm", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "wlm", cleanInst, metric)
	}
}

// parseBeanManager handles Bean Manager metrics
func (w *WebSpherePMI) parseBeanManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureBeanManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("bean_manager", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "bean_manager", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "bean_manager", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "LiveCount":
			mx[fmt.Sprintf("bean_manager_%s_live_beans", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "bean_manager", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "bean_manager", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "bean_manager", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "bean_manager", cleanInst, metric)
	}
}

// parseConnectionManager handles Connection Manager metrics
func (w *WebSpherePMI) parseConnectionManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureConnectionManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("connection_manager", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "connection_manager", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "connection_manager", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "AllocatedCount":
			mx[fmt.Sprintf("connection_manager_%s_allocated", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "connection_manager", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "connection_manager", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "connection_manager", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "connection_manager", cleanInst, metric)
	}
}

// parseJVMSubsystem handles JVM subsystem metrics
func (w *WebSpherePMI) parseJVMSubsystem(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	subsystem := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, subsystem)
	
	// Create charts if not exists
	w.ensureJVMSubsystemCharts(instance, nodeName, serverName, subsystem)
	
	// Mark as seen
	w.markInstanceSeen("jvm_subsystem", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch stat.Name {
		case "JVM.GC":
			switch metric.Name {
			case "HeapDiscarded":
				mx[fmt.Sprintf("jvm_gc_%s_heap_discarded", cleanInst)] = metric.Value
			case "GCTime":
				mx[fmt.Sprintf("jvm_gc_%s_time", cleanInst)] = metric.Value
			default:
				// For unknown count metrics, use collection helper
				w.collectCountMetric(mx, "jvm_gc", cleanInst, metric)
			}
		case "JVM.Memory":
			switch metric.Name {
			case "AllocatedMemory":
				mx[fmt.Sprintf("jvm_memory_%s_allocated", cleanInst)] = metric.Value
			default:
				// For unknown count metrics, use collection helper
				w.collectCountMetric(mx, "jvm_memory", cleanInst, metric)
			}
		case "JVM.Thread":
			switch metric.Name {
			case "ActiveThreads":
				mx[fmt.Sprintf("jvm_thread_%s_active", cleanInst)] = metric.Value
			default:
				// For unknown count metrics, use collection helper
				w.collectCountMetric(mx, "jvm_thread", cleanInst, metric)
			}
		default:
			// For unknown subsystems, use generic collection helper
			w.collectCountMetric(mx, "jvm_subsystem", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics with subsystem-specific prefix
		prefix := "jvm_subsystem"
		switch stat.Name {
		case "JVM.GC":
			prefix = "jvm_gc"
		case "JVM.Memory":
			prefix = "jvm_memory"
		case "JVM.Thread":
			prefix = "jvm_thread"
		}
		w.collectTimeMetric(mx, prefix, cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics with subsystem-specific prefix
		prefix := "jvm_subsystem"
		switch stat.Name {
		case "JVM.GC":
			prefix = "jvm_gc"
		case "JVM.Memory":
			prefix = "jvm_memory"
		case "JVM.Thread":
			prefix = "jvm_thread"
		}
		w.collectRangeMetric(mx, prefix, cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics with subsystem-specific prefix
		prefix := "jvm_subsystem"
		switch stat.Name {
		case "JVM.GC":
			prefix = "jvm_gc"
		case "JVM.Memory":
			prefix = "jvm_memory"
		case "JVM.Thread":
			prefix = "jvm_thread"
		}
		w.collectBoundedRangeMetric(mx, prefix, cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics with subsystem-specific prefix
		prefix := "jvm_subsystem"
		switch stat.Name {
		case "JVM.GC":
			prefix = "jvm_gc"
		case "JVM.Memory":
			prefix = "jvm_memory"
		case "JVM.Thread":
			prefix = "jvm_thread"
		}
		w.collectAverageMetric(mx, prefix, cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics with subsystem-specific prefix
		prefix := "jvm_subsystem"
		switch stat.Name {
		case "JVM.GC":
			prefix = "jvm_gc"
		case "JVM.Memory":
			prefix = "jvm_memory"
		case "JVM.Thread":
			prefix = "jvm_thread"
		}
		w.collectDoubleMetric(mx, prefix, cleanInst, metric)
	}
}

// parseEJBContainer handles EJB container metrics
func (w *WebSpherePMI) parseEJBContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureEJBContainerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("ejb_container", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "TotalMethodCalls":
			mx[fmt.Sprintf("ejb_container_%s_method_calls", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "ejb_container", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "ejb_container", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "ejb_container", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "ejb_container", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "ejb_container", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "ejb_container", cleanInst, metric)
	}
}

// parseMDB handles Message Driven Bean metrics
func (w *WebSpherePMI) parseMDB(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureMDBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("mdb", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "MessageCount":
			mx[fmt.Sprintf("mdb_%s_messages", cleanInst)] = metric.Value
		case "MessageBackoutCount":
			mx[fmt.Sprintf("mdb_%s_backouts", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "mdb", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "bean", Value: beanName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"mdb",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "mdb", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "mdb", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "mdb", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "mdb", cleanInst, metric)
	}
}

// parseStatefulSessionBean handles Stateful Session Bean metrics
func (w *WebSpherePMI) parseStatefulSessionBean(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureSFSBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("sfsb", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CreateCount":
			mx[fmt.Sprintf("sfsb_%s_created", cleanInst)] = metric.Value
		case "RemoveCount":
			mx[fmt.Sprintf("sfsb_%s_removed", cleanInst)] = metric.Value
		case "ActivateCount":
			mx[fmt.Sprintf("sfsb_%s_activated", cleanInst)] = metric.Value
		case "PassivateCount":
			mx[fmt.Sprintf("sfsb_%s_passivated", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "sfsb", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "sfsb", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "LiveCount":
			mx[fmt.Sprintf("sfsb_%s_live", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "sfsb", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "sfsb", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "sfsb", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "sfsb", cleanInst, metric)
	}
}

// parseStatelessSessionBean handles Stateless Session Bean metrics
func (w *WebSpherePMI) parseStatelessSessionBean(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureSLSBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("slsb", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "MethodCallCount":
			mx[fmt.Sprintf("slsb_%s_method_calls", cleanInst)] = metric.Value
		case "CreateCount":
			mx[fmt.Sprintf("slsb_%s_created", cleanInst)] = metric.Value
		case "RemoveCount":
			mx[fmt.Sprintf("slsb_%s_removed", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "slsb", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "bean", Value: beanName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"slsb",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "PooledCount":
			mx[fmt.Sprintf("slsb_%s_pooled", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "slsb", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "slsb", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "slsb", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "slsb", cleanInst, metric)
	}
}

// parseEntityBean handles Entity Bean metrics
func (w *WebSpherePMI) parseEntityBean(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureEntityBeanCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("entity_bean", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "ActivateCount":
			mx[fmt.Sprintf("entity_bean_%s_activations", cleanInst)] = metric.Value
		case "PassivateCount":
			mx[fmt.Sprintf("entity_bean_%s_passivations", cleanInst)] = metric.Value
		case "CreateCount":
			mx[fmt.Sprintf("entity_bean_%s_created", cleanInst)] = metric.Value
		case "RemoveCount":
			mx[fmt.Sprintf("entity_bean_%s_removed", cleanInst)] = metric.Value
		case "LoadCount":
			mx[fmt.Sprintf("entity_bean_%s_loaded", cleanInst)] = metric.Value
		case "StoreCount":
			mx[fmt.Sprintf("entity_bean_%s_stored", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "entity_bean", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "entity_bean", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "LiveCount":
			mx[fmt.Sprintf("entity_bean_%s_live", cleanInst)] = metric.Current
		case "PooledCount":
			mx[fmt.Sprintf("entity_bean_%s_pooled", cleanInst)] = metric.Current
		case "ReadyCount":
			mx[fmt.Sprintf("entity_bean_%s_ready", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "entity_bean", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "entity_bean", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "entity_bean", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "entity_bean", cleanInst, metric)
	}
}

// parseIndividualEJB handles individual EJB instances found dynamically
func (w *WebSpherePMI) parseIndividualEJB(stat *pmiStat, nodeName, serverName string, mx map[string]int64, path []string) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureGenericEJBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("generic_ejb", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "MethodCallCount", "InvocationCount":
			mx[fmt.Sprintf("generic_ejb_%s_invocations", cleanInst)] = metric.Value
		case "CreateCount":
			mx[fmt.Sprintf("generic_ejb_%s_creates", cleanInst)] = metric.Value
		case "RemoveCount":
			mx[fmt.Sprintf("generic_ejb_%s_removes", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "generic_ejb", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "bean", Value: beanName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"generic_ejb",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "LiveCount":
			mx[fmt.Sprintf("generic_ejb_%s_live", cleanInst)] = metric.Current
		case "PooledCount":
			mx[fmt.Sprintf("generic_ejb_%s_pooled", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "generic_ejb", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "generic_ejb", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "generic_ejb", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "generic_ejb", cleanInst, metric)
	}
}

// parseGenericStat handles unclassified stats to capture remaining metrics
func (w *WebSpherePMI) parseGenericStat(stat *pmiStat, nodeName, serverName string, mx map[string]int64, path []string) {
	// Only parse if it has actual metrics to avoid empty instances
	if len(stat.CountStatistics) == 0 && len(stat.RangeStatistics) == 0 && 
	   len(stat.BoundedRangeStatistics) == 0 && len(stat.TimeStatistics) == 0 {
		return
	}
	
	statName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, statName)
	
	// Create unique chart ID to avoid conflicts
	chartKey := fmt.Sprintf("generic_stat_%s", w.cleanID(instance))
	
	// Check if we already have this chart
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create dynamic charts based on available metrics
		w.createDynamicGenericChart(instance, nodeName, serverName, statName, stat)
	}
	
	// Mark as seen
	w.markInstanceSeen("generic_stat", instance)
	
	// Process all available metrics generically
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			metricKey := fmt.Sprintf("generic_stat_%s_%s", w.cleanID(instance), w.cleanID(cs.Name))
			mx[metricKey] = val
		}
	}
	
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			metricKey := fmt.Sprintf("generic_stat_%s_%s", w.cleanID(instance), w.cleanID(rs.Name))
			mx[metricKey] = val
		}
	}
	
	for _, brs := range stat.BoundedRangeStatistics {
		if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
			metricKey := fmt.Sprintf("generic_stat_%s_%s", w.cleanID(instance), w.cleanID(brs.Name))
			mx[metricKey] = val
		}
	}
	
	// NOTE: TimeStatistics are now processed by smart processor (processTimeStatisticWithContext)
	// in specific parsers. Generic fallback skips TimeStatistics to avoid duplication.
	// TimeStatistics in generic stats would be extremely rare and likely indicate
	// misconfigured PMI or unknown PMI components that need specific parser implementation.
}

// createDynamicGenericChart creates charts dynamically based on available metrics
func (w *WebSpherePMI) createDynamicGenericChart(instance, nodeName, serverName, statName string, stat *pmiStat) {
	// Create dynamic dimensions based on available metrics
	var dims module.Dims
	
	// Add CountStatistics dimensions
	for _, cs := range stat.CountStatistics {
		dimID := fmt.Sprintf("generic_stat_%s_%s", w.cleanID(instance), w.cleanID(cs.Name))
		dims = append(dims, &module.Dim{
			ID:   dimID,
			Name: w.cleanID(cs.Name),
			Algo: module.Incremental, // Most count stats are incremental
		})
	}
	
	// Add RangeStatistics dimensions
	for _, rs := range stat.RangeStatistics {
		dimID := fmt.Sprintf("generic_stat_%s_%s", w.cleanID(instance), w.cleanID(rs.Name))
		dims = append(dims, &module.Dim{
			ID:   dimID,
			Name: w.cleanID(rs.Name),
		})
	}
	
	// Add BoundedRangeStatistics dimensions
	for _, brs := range stat.BoundedRangeStatistics {
		dimID := fmt.Sprintf("generic_stat_%s_%s", w.cleanID(instance), w.cleanID(brs.Name))
		dims = append(dims, &module.Dim{
			ID:   dimID,
			Name: w.cleanID(brs.Name),
		})
	}
	
	// NOTE: Skip TimeStatistics - they're handled by smart processor in specific parsers
	// to avoid duplication and ensure consistent 3-chart pattern (rate, current, lifetime)
	
	// Only create chart if we have dimensions
	if len(dims) > 0 {
		chart := &module.Chart{
			ID:       fmt.Sprintf("generic_stat_%s_metrics", w.cleanID(instance)),
			Title:    fmt.Sprintf("Generic Metrics: %s", statName),
			Units:    "metrics",
			Fam:      "management/portlets",
			Ctx:      "websphere_pmi.generic_metrics",
			Type:     module.Line,
			Priority: prioMonitoring,
			Dims:     dims,
			Labels: []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "stat", Value: statName},
			},
		}
		
		if err := w.charts.Add(chart); err != nil {
			w.Warning(err)
		}
	}
}

// parseExtensionRegistryStats handles extension registry statistics
func (w *WebSpherePMI) parseExtensionRegistryStats(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureExtensionRegistryCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("extension_registry", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "RequestCount":
			mx[fmt.Sprintf("extension_registry_%s_requests", cleanInst)] = metric.Value
		case "HitCount":
			mx[fmt.Sprintf("extension_registry_%s_hits", cleanInst)] = metric.Value
		case "DisplacementCount":
			mx[fmt.Sprintf("extension_registry_%s_displacements", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "extension_registry", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		w.collectTimeMetric(mx, "extension_registry", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "extension_registry", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "extension_registry", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "extension_registry", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		switch metric.Name {
		case "HitRate":
			mx[fmt.Sprintf("extension_registry_%s_hit_rate", cleanInst)] = metric.Value
		default:
			w.collectDoubleMetric(mx, "extension_registry", cleanInst, metric)
		}
	}
}

// parseSIBJMSAdapter handles Service Integration Bus JMS Resource Adapter metrics
func (w *WebSpherePMI) parseSIBJMSAdapter(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSIBJMSAdapterCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("sib_jms_adapter", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CreateCount":
			mx[fmt.Sprintf("sib_jms_%s_create_count", cleanInst)] = metric.Value
		case "CloseCount":
			mx[fmt.Sprintf("sib_jms_%s_close_count", cleanInst)] = metric.Value
		case "AllocateCount":
			mx[fmt.Sprintf("sib_jms_%s_allocate_count", cleanInst)] = metric.Value
		case "FreedCount":
			mx[fmt.Sprintf("sib_jms_%s_freed_count", cleanInst)] = metric.Value
		case "FaultCount":
			mx[fmt.Sprintf("sib_jms_%s_fault_count", cleanInst)] = metric.Value
		case "ManagedConnectionCount":
			mx[fmt.Sprintf("sib_jms_%s_managed_connections", cleanInst)] = metric.Value
		case "ConnectionHandleCount":
			mx[fmt.Sprintf("sib_jms_%s_connection_handles", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use generic naming
			mx[fmt.Sprintf("sib_jms_%s_%s", cleanInst, w.cleanID(metric.Name))] = metric.Value
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		w.collectTimeMetric(mx, "sib_jms", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "sib_jms", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "sib_jms", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "sib_jms", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "sib_jms", cleanInst, metric)
	}
}

// parseServletsComponent handles standalone Servlets component metrics
func (w *WebSpherePMI) parseServletsComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureServletsComponentCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("servlets_component", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "RequestCount":
			mx[fmt.Sprintf("servlets_component_%s_requests", cleanInst)] = metric.Value
		case "ErrorCount":
			mx[fmt.Sprintf("servlets_component_%s_errors", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use generic naming
			w.collectCountMetric(mx, "servlets_component", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"servlets_component",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "ConcurrentRequests":
			mx[fmt.Sprintf("servlets_component_%s_concurrent_requests", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "servlets_component", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "servlets_component", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "servlets_component", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "servlets_component", cleanInst, metric)
	}
	
	totalExtracted := len(countMetrics) + len(timeMetrics) + len(rangeMetrics) + 
					 len(boundedMetrics) + len(avgMetrics) + len(doubleMetrics)
	w.Debugf("ServletsComponent extracted %d total metrics (%d count, %d time, %d range, %d bounded, %d avg, %d double) for instance %s", 
		totalExtracted, len(countMetrics), len(timeMetrics), len(rangeMetrics), 
		len(boundedMetrics), len(avgMetrics), len(doubleMetrics), instance)
}

// parseWIMComponent handles WebSphere Identity Manager component metrics
func (w *WebSpherePMI) parseWIMComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWIMComponentCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("wim_component", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "Number of portlet requests":
			mx[fmt.Sprintf("wim_%s_portlet_requests", cleanInst)] = metric.Value
		case "Number of portlet errors":
			mx[fmt.Sprintf("wim_%s_portlet_errors", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use generic naming
			mx[fmt.Sprintf("wim_%s_%s", cleanInst, w.cleanID(metric.Name))] = metric.Value
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"wim",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "wim", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "wim", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "wim", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "wim", cleanInst, metric)
	}
}

// parseWLMTaggedComponentManager handles Workload Management Tagged Component Manager metrics
func (w *WebSpherePMI) parseWLMTaggedComponentManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWLMTaggedComponentManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("wlm_tagged_component", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics (none expected)
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		w.collectCountMetric(mx, "wlm_tagged", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"wlm_tagged",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics (none expected)
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "wlm_tagged", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics (none expected)
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "wlm_tagged", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics (none expected)
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "wlm_tagged", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics (none expected)
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "wlm_tagged", cleanInst, metric)
	}
}

// parsePMIWebServiceService handles PMI Web Service Service metrics
func (w *WebSpherePMI) parsePMIWebServiceService(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensurePMIWebServiceServiceCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("pmi_webservice_service", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "RequestReceivedService":
			mx[fmt.Sprintf("pmi_webservice_%s_requests_received", cleanInst)] = metric.Value
		case "RequestDispatchedService":
			mx[fmt.Sprintf("pmi_webservice_%s_requests_dispatched", cleanInst)] = metric.Value
		case "RequestSuccessfulService":
			mx[fmt.Sprintf("pmi_webservice_%s_requests_successful", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "pmi_webservice", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"pmi_webservice",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "pmi_webservice", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "pmi_webservice", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for i, metric := range avgMetrics {
		switch metric.Name {
		case "SizeService":
			// Use smart processor for SizeService
			w.processAverageStatisticWithContext(
				"pmi_webservice",
				cleanInst,
				labels,
				metric,
				mx,
				i*10, // Priority offset
				"bytes",  // SizeService is measured in bytes
			)
		case "RequestSizeService":
			// Use smart processor for RequestSizeService
			w.processAverageStatisticWithContext(
				"pmi_webservice",
				cleanInst,
				labels,
				metric,
				mx,
				20+i*10, // Priority offset
				"bytes",  // RequestSizeService is measured in bytes
			)
		case "ReplySizeService":
			// Use smart processor for ReplySizeService
			w.processAverageStatisticWithContext(
				"pmi_webservice",
				cleanInst,
				labels,
				metric,
				mx,
				40+i*10, // Priority offset
				"bytes",  // ReplySizeService is measured in bytes
			)
		default:
			// Use collection helper for unknown average metrics
			w.collectAverageMetric(mx, "pmi_webservice", cleanInst, metric)
		}
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "pmi_webservice", cleanInst, metric)
	}
}

// parseTCPChannelDCS handles TCP Channel Data Communication Services metrics
func (w *WebSpherePMI) parseTCPChannelDCS(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Check if MaxPoolSize is available (version-specific)
	hasMaxPoolSize := false
	for _, cs := range stat.CountStatistics {
		if cs.Name == "MaxPoolSize" {
			hasMaxPoolSize = true
			break
		}
	}
	
	// Create charts if not exists, with version-specific handling
	w.ensureTCPChannelDCSCharts(instance, nodeName, serverName, hasMaxPoolSize)
	
	// Mark as seen
	w.markInstanceSeen("tcp_channel_dcs", instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "CreateCount":
			mx[fmt.Sprintf("tcp_dcs_%s_create_count", cleanInst)] = metric.Value
		case "DestroyCount":
			mx[fmt.Sprintf("tcp_dcs_%s_destroy_count", cleanInst)] = metric.Value
		case "DeclaredThreadHungCount":
			mx[fmt.Sprintf("tcp_dcs_%s_thread_hung_count", cleanInst)] = metric.Value
		case "ClearedThreadHangCount":
			mx[fmt.Sprintf("tcp_dcs_%s_thread_hang_cleared", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "tcp_dcs", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"tcp_dcs",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.Debugf("TCP DCS RangeStatistic: %s", metric.Name)
		switch metric.Name {
		case "ConcurrentHungThreadCount":
			mx[fmt.Sprintf("tcp_dcs_%s_concurrent_hung_threads", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.Debugf("TCP DCS: Using collection helper for unknown RangeStatistic: %s", metric.Name)
			w.collectRangeMetric(mx, "tcp_dcs", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		switch metric.Name {
		case "ActiveCount":
			mx[fmt.Sprintf("tcp_dcs_%s_active_count", cleanInst)] = metric.Value
		case "PoolSize":
			mx[fmt.Sprintf("tcp_dcs_%s_pool_size", cleanInst)] = metric.Value
		case "PercentMaxed":
			mx[fmt.Sprintf("tcp_dcs_%s_percent_maxed", cleanInst)] = metric.Value
		case "PercentUsed":
			mx[fmt.Sprintf("tcp_dcs_%s_percent_used", cleanInst)] = metric.Value
		default:
			// For unknown bounded range metrics, use collection helper
			w.collectBoundedRangeMetric(mx, "tcp_dcs", cleanInst, metric)
		}
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "tcp_dcs", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "tcp_dcs", cleanInst, metric)
	}
}

// parseDetailsComponent handles Details component metrics (context-sensitive)
func (w *WebSpherePMI) parseDetailsComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64, path []string) {
	// Context-sensitive parsing based on parent component
	parentContext := "unknown"
	if len(path) > 1 {
		parentContext = path[len(path)-2]
	}
	
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, parentContext)
	
	// Create charts if not exists
	w.ensureDetailsComponentCharts(instance, nodeName, serverName, parentContext)
	
	// Mark as seen
	w.markInstanceSeen("details_component", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "Number of loaded portlets":
			mx[fmt.Sprintf("details_%s_loaded_portlets", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "details", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		w.collectTimeMetric(mx, "details", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "details", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "details", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "details", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "details", cleanInst, metric)
	}
}

// parseISCProductDetails handles IBM Support Center Product Details metrics
func (w *WebSpherePMI) parseISCProductDetails(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureISCProductDetailsCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("isc_product_details", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "Number of portlet requests":
			mx[fmt.Sprintf("isc_product_%s_portlet_requests", cleanInst)] = metric.Value
		case "Number of portlet errors":
			mx[fmt.Sprintf("isc_product_%s_portlet_errors", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "isc_product", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"isc_product",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		w.collectRangeMetric(mx, "isc_product", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		w.collectBoundedRangeMetric(mx, "isc_product", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		w.collectAverageMetric(mx, "isc_product", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		w.collectDoubleMetric(mx, "isc_product", cleanInst, metric)
	}
}

// ensureExtensionRegistryCharts creates extension registry charts
func (w *WebSpherePMI) ensureExtensionRegistryCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("extension_registry_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := extensionRegistryChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureSIBJMSAdapterCharts creates SIB JMS adapter charts
func (w *WebSpherePMI) ensureSIBJMSAdapterCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("sib_jms_adapter_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := sibJMSAdapterChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureServletsComponentCharts creates servlets component charts
func (w *WebSpherePMI) ensureServletsComponentCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("servlets_component_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := servletsComponentChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureWIMComponentCharts creates WIM component charts
func (w *WebSpherePMI) ensureWIMComponentCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("wim_component_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := wimComponentChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureWLMTaggedComponentManagerCharts creates WLM tagged component manager charts
func (w *WebSpherePMI) ensureWLMTaggedComponentManagerCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("wlm_tagged_component_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := wlmTaggedComponentChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensurePMIWebServiceServiceCharts creates PMI web service service charts
func (w *WebSpherePMI) ensurePMIWebServiceServiceCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("pmi_webservice_service_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := pmiWebServiceServiceChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureTCPChannelDCSCharts creates TCP channel DCS charts
func (w *WebSpherePMI) ensureTCPChannelDCSCharts(instance, nodeName, serverName string, hasMaxPoolSize bool) {
	chartKey := fmt.Sprintf("tcp_channel_dcs_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := tcpChannelDCSChartsTmpl.Copy()
		
		// Remove pool_limits chart if MaxPoolSize is not available (WebSphere 8.5.5)
		if !hasMaxPoolSize {
			// Find and remove the pool_limits chart
			filtered := module.Charts{}
			for _, chart := range *charts {
				if chart.ID != "tcp_dcs_%s_pool_limits" {
					filtered = append(filtered, chart)
				}
			}
			charts = &filtered
		}
		
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureDetailsComponentCharts creates details component charts
func (w *WebSpherePMI) ensureDetailsComponentCharts(instance, nodeName, serverName, parentContext string) {
	chartKey := fmt.Sprintf("details_component_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := detailsComponentChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "context", Value: parentContext},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ensureISCProductDetailsCharts creates ISC product details charts
func (w *WebSpherePMI) ensureISCProductDetailsCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("isc_product_details_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := iscProductDetailsChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// All priority constants are now defined in charts.go

// Chart creation helpers for new parsers
func (w *WebSpherePMI) ensureJCAPoolCharts(instance, nodeName, serverName, poolName string) {
	chartKey := fmt.Sprintf("jca_pool_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := jcaPoolChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "pool", Value: poolName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureEnterpriseAppCharts(instance, nodeName, serverName, appName string) {
	chartKey := fmt.Sprintf("enterprise_app_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := enterpriseAppChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "application", Value: appName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureSystemDataCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("system_data_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := systemDataChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureWLMCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("wlm_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := wlmChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureBeanManagerCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("bean_manager_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := beanManagerChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureConnectionManagerCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("connection_manager_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := connectionManagerChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureJVMSubsystemCharts(instance, nodeName, serverName, subsystem string) {
	chartKey := fmt.Sprintf("jvm_subsystem_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := jvmSubsystemChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "subsystem", Value: subsystem},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureEJBContainerCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("ejb_container_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := ejbContainerChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureMDBCharts(instance, nodeName, serverName, beanName string) {
	chartKey := fmt.Sprintf("mdb_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := mdbChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "bean", Value: beanName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureSFSBCharts(instance, nodeName, serverName, beanName string) {
	chartKey := fmt.Sprintf("sfsb_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := sfsbChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "bean", Value: beanName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureSLSBCharts(instance, nodeName, serverName, beanName string) {
	chartKey := fmt.Sprintf("slsb_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := slsbChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "bean", Value: beanName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureEntityBeanCharts(instance, nodeName, serverName, beanName string) {
	chartKey := fmt.Sprintf("entity_bean_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := entityBeanChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "bean", Value: beanName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureGenericEJBCharts(instance, nodeName, serverName, beanName string) {
	chartKey := fmt.Sprintf("generic_ejb_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := genericEJBChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "bean", Value: beanName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// ===== SECURITY PARSERS =====

// parseSecurityAuthentication handles Security Authentication metrics
func (w *WebSpherePMI) parseSecurityAuthentication(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSecurityAuthCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("security_auth", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		// Traditional WebSphere detailed metrics
		case "WebAuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_web_auth", cleanInst)] = metric.Value
		case "TAIRequestCount":
			mx[fmt.Sprintf("security_auth_%s_tai_requests", cleanInst)] = metric.Value
		case "IdentityAssertionCount":
			mx[fmt.Sprintf("security_auth_%s_identity_assertions", cleanInst)] = metric.Value
		case "BasicAuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_basic_auth", cleanInst)] = metric.Value
		case "TokenAuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_token_auth", cleanInst)] = metric.Value
		case "ClientCertAuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_cert_auth", cleanInst)] = metric.Value
		case "LTPATokenAuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_ltpa_auth", cleanInst)] = metric.Value
		case "CustomAuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_custom_auth", cleanInst)] = metric.Value
		case "AuthenticationFailureCount":
			mx[fmt.Sprintf("security_auth_%s_failures", cleanInst)] = metric.Value
		// Liberty/generic metrics
		case "AuthenticationCount":
			mx[fmt.Sprintf("security_auth_%s_total_auth", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, skip for now until charts are created
			// w.collectCountMetric(mx, "security_auth", cleanInst, metric)
			w.Debugf("Skipping unknown security_auth count metric: %s", metric.Name)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"security_auth",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "security_auth", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "security_auth", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "security_auth", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "security_auth", cleanInst, metric)
	}
}

// parseSecurityAuthorization handles Security Authorization metrics
func (w *WebSpherePMI) parseSecurityAuthorization(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSecurityAuthzCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("security_authz", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "security_authz", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"security_authz",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "security_authz", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "security_authz", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "security_authz", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "security_authz", cleanInst, metric)
	}
}

// ===== SECURITY CHART TEMPLATES =====

var securityAuthChartsTmpl = module.Charts{
	{
		ID:       "security_auth_%s_events",
		Title:    "Security Authentication Events",
		Units:    "events/s",
		Fam:      "security/authentication/overview",
		Ctx:      "websphere_pmi.security_auth_events",
		Type:     module.Line,
		Priority: prioSecurityAuth,
		Dims: module.Dims{
			// Traditional WebSphere metrics (based on actual XML)
			{ID: "security_auth_%s_web_auth", Name: "web", Algo: module.Incremental},
			{ID: "security_auth_%s_basic_auth", Name: "basic", Algo: module.Incremental},
			{ID: "security_auth_%s_token_auth", Name: "token", Algo: module.Incremental},
			{ID: "security_auth_%s_tai_requests", Name: "tai", Algo: module.Incremental},
			{ID: "security_auth_%s_identity_assertions", Name: "identity", Algo: module.Incremental},
		},
	},
}

var securityAuthzChartsTmpl = module.Charts{
	// Empty - all timing is handled by smart processor
}

// Chart templates for new parsers
var interceptorChartsTmpl = module.Charts{
	// Empty - all timing is handled by smart processor
}

var portletChartsTmpl = module.Charts{
	{
		ID:       "portlet_%s_requests",
		Title:    "Portlet Requests",
		Units:    "requests/s",
		Fam:      "web/portlets/instances",
		Ctx:      "websphere_pmi.portlet_requests",
		Type:     module.Line,
		Priority: prioWebPortlets,
		Dims: module.Dims{
			{ID: "portlet_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "portlet_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "portlet_%s_concurrent",
		Title:    "Portlet Concurrent Requests",
		Units:    "requests",
		Fam:      "web/portlets/instances",
		Ctx:      "websphere_pmi.portlet_concurrent",
		Type:     module.Line,
		Priority: prioWebPortlets + 1,
		Dims: module.Dims{
			{ID: "portlet_%s_concurrent", Name: "concurrent"},
		},
	},
}

var webServiceChartsTmpl = module.Charts{
	{
		ID:       "web_service_%s_services",
		Title:    "Web Service Module Services",
		Units:    "services",
		Fam:      "integration/webservices",
		Ctx:      "websphere_pmi.web_service_services",
		Type:     module.Line,
		Priority: prioWebServices,
		Dims: module.Dims{
			{ID: "web_service_%s_services_loaded", Name: "loaded", Algo: module.Absolute},
		},
	},
}

var urlChartsTmpl = module.Charts{
	{
		ID:       "urls_%s_requests",
		Title:    "URL Requests",
		Units:    "requests/s",
		Fam:      "web/servlets/urls",
		Ctx:      "websphere_pmi.url_requests",
		Type:     module.Line,
		Priority: prioWebServlets + 10,
		Dims: module.Dims{
			{ID: "urls_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "urls_%s_concurrent",
		Title:    "URL Concurrent Requests",
		Units:    "requests",
		Fam:      "web/servlets/urls",
		Ctx:      "websphere_pmi.url_concurrent",
		Type:     module.Line,
		Priority: prioWebServlets + 11,
		Dims: module.Dims{
			{ID: "urls_%s_concurrent", Name: "concurrent"},
		},
	},
}

var servletURLChartsTmpl = module.Charts{
	{
		ID:       "servlet_url_%s_requests",
		Title:    "Servlet URL Requests",
		Units:    "requests/s",
		Fam:      "web/servlets/urls",
		Ctx:      "websphere_pmi.servlet_url_requests",
		Type:     module.Line,
		Priority: prioWebServlets + 20,
		Dims: module.Dims{
			{ID: "servlet_url_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlet_url_%s_concurrent",
		Title:    "Servlet URL Concurrent Requests",
		Units:    "requests",
		Fam:      "web/servlets/urls",
		Ctx:      "websphere_pmi.servlet_url_concurrent",
		Type:     module.Line,
		Priority: prioWebServlets + 21,
		Dims: module.Dims{
			{ID: "servlet_url_%s_concurrent", Name: "concurrent"},
		},
	},
}

var haManagerChartsTmpl = module.Charts{
	{
		ID:       "ha_manager_%s_groups",
		Title:    "HA Manager Groups",
		Units:    "groups",
		Fam:      "availability/hamanager",
		Ctx:      "websphere_pmi.ha_manager_groups",
		Type:     module.Line,
		Priority: prioHAManager,
		Dims: module.Dims{
			{ID: "ha_manager_%s_local_groups", Name: "local_groups"},
		},
	},
	{
		ID:       "ha_manager_%s_bulletin_board",
		Title:    "HA Manager Bulletin Board",
		Units:    "items",
		Fam:      "availability/hamanager",
		Ctx:      "websphere_pmi.ha_manager_bulletin_board",
		Type:     module.Line,
		Priority: prioHAManager + 1,
		Dims: module.Dims{
			{ID: "ha_manager_%s_bulletin_subjects", Name: "subjects"},
			{ID: "ha_manager_%s_bulletin_subscriptions", Name: "subscriptions"},
			{ID: "ha_manager_%s_local_bulletin_subjects", Name: "local_subjects"},
			{ID: "ha_manager_%s_local_bulletin_subscriptions", Name: "local_subscriptions"},
		},
	},
}

// ===== SECURITY CHART CREATION HELPERS =====

func (w *WebSpherePMI) ensureSecurityAuthCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("security_auth_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := securityAuthChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureSecurityAuthzCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("security_authz_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := securityAuthzChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// parseInterceptorContainer handles Interceptors container metrics
func (w *WebSpherePMI) parseInterceptorContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	// Check if the container itself has metrics
	if len(stat.CountStatistics) > 0 || len(stat.TimeStatistics) > 0 || len(stat.RangeStatistics) > 0 || 
	   len(stat.BoundedRangeStatistics) > 0 || len(stat.AverageStatistics) > 0 || len(stat.DoubleStatistics) > 0 {
		// Process container-level metrics using universal helpers
		// instance := fmt.Sprintf("%s.%s", nodeName, serverName)
		// cleanInst := w.cleanID(instance)
		
		// Extract all metric types but skip collection for now until charts are created
		countMetrics := w.extractCountStatistics(stat.CountStatistics)
		for _, metric := range countMetrics {
			// w.collectCountMetric(mx, "interceptor_container", cleanInst, metric)
			w.Debugf("Skipping interceptor_container count metric: %s", metric.Name)
		}
		
		timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
		for _, metric := range timeMetrics {
			// w.collectTimeMetric(mx, "interceptor_container", cleanInst, metric)
			w.Debugf("Skipping interceptor_container time metric: %s", metric.Name)
		}
		
		rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
		for _, metric := range rangeMetrics {
			// w.collectRangeMetric(mx, "interceptor_container", cleanInst, metric)
			w.Debugf("Skipping interceptor_container range metric: %s", metric.Name)
		}
		
		boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
		for _, metric := range boundedMetrics {
			// w.collectBoundedRangeMetric(mx, "interceptor_container", cleanInst, metric)
			w.Debugf("Skipping interceptor_container bounded range metric: %s", metric.Name)
		}
		
		avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
		for _, metric := range avgMetrics {
			// w.collectAverageMetric(mx, "interceptor_container", cleanInst, metric)
			w.Debugf("Skipping interceptor_container average metric: %s", metric.Name)
		}
		
		doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
		for _, metric := range doubleMetrics {
			// w.collectDoubleMetric(mx, "interceptor_container", cleanInst, metric)
			w.Debugf("Skipping interceptor_container double metric: %s", metric.Name)
		}
	}
	
	// Process individual interceptors as children
	for _, interceptor := range stat.SubStats {
		w.parseIndividualInterceptor(&interceptor, nodeName, serverName, mx)
	}
}

// parseIndividualInterceptor handles individual interceptor metrics
func (w *WebSpherePMI) parseIndividualInterceptor(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	interceptorName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, interceptorName)
	
	// Create charts if not exists
	w.ensureInterceptorCharts(instance, nodeName, serverName, interceptorName)
	
	// Mark as seen
	w.markInstanceSeen("interceptor", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "interceptor", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "interceptor", Value: interceptorName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"interceptor",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "interceptor", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "interceptor", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "interceptor", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "interceptor", cleanInst, metric)
	}
}

// parsePortlet handles individual portlet metrics  
func (w *WebSpherePMI) parsePortlet(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	portletName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, portletName)
	
	// Create charts if not exists
	w.ensurePortletCharts(instance, nodeName, serverName, portletName)
	
	// Mark as seen
	w.markInstanceSeen("portlet", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "Number of portlet requests":
			mx[fmt.Sprintf("portlet_%s_requests", cleanInst)] = metric.Value
		case "Number of portlet errors":
			mx[fmt.Sprintf("portlet_%s_errors", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "portlet", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "portlet", Value: portletName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"portlet",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "Number of concurrent portlet requests":
			mx[fmt.Sprintf("portlet_%s_concurrent", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "portlet", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "portlet", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "portlet", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "portlet", cleanInst, metric)
	}
}

// parseWebServiceModule handles web service module metrics
func (w *WebSpherePMI) parseWebServiceModule(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	moduleName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, moduleName)
	
	// Create charts if not exists
	w.ensureWebServiceCharts(instance, nodeName, serverName, moduleName)
	
	// Mark as seen
	w.markInstanceSeen("web_service", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "ServicesLoaded":
			mx[fmt.Sprintf("web_service_%s_services_loaded", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "web_service", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	for _, metric := range timeMetrics {
		// Use collection helper for all time metrics
		w.collectTimeMetric(mx, "web_service", cleanInst, metric)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		// Use collection helper for all range metrics
		w.collectRangeMetric(mx, "web_service", cleanInst, metric)
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "web_service", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "web_service", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "web_service", cleanInst, metric)
	}
}

// parseURLContainer handles URLs container metrics
func (w *WebSpherePMI) parseURLContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureURLCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("urls", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "URIRequestCount":
			mx[fmt.Sprintf("urls_%s_requests", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "urls", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"urls",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "URIConcurrentRequests":
			mx[fmt.Sprintf("urls_%s_concurrent", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "urls", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "urls", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "urls", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "urls", cleanInst, metric)
	}
}

// parseServletURL handles individual servlet URL metrics
func (w *WebSpherePMI) parseServletURL(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	urlPath := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, urlPath)
	
	// Create charts if not exists
	w.ensureServletURLCharts(instance, nodeName, serverName, urlPath)
	
	// Mark as seen
	w.markInstanceSeen("servlet_url", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "URIRequestCount":
			mx[fmt.Sprintf("servlet_url_%s_requests", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "servlet_url", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
		{Key: "url", Value: urlPath},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"servlet_url",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "URIConcurrentRequests":
			mx[fmt.Sprintf("servlet_url_%s_concurrent", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "servlet_url", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// Use collection helper for all bounded range metrics
		w.collectBoundedRangeMetric(mx, "servlet_url", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "servlet_url", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "servlet_url", cleanInst, metric)
	}
}

// parseHAManager handles HA Manager metrics
func (w *WebSpherePMI) parseHAManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureHAManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("ha_manager", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		// Use collection helper for all count metrics
		w.collectCountMetric(mx, "ha_manager", cleanInst, metric)
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"ha_manager",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "LocalGroupCount":
			mx[fmt.Sprintf("ha_manager_%s_local_groups", cleanInst)] = metric.Current
		case "BulletinBoardSubjectCount":
			mx[fmt.Sprintf("ha_manager_%s_bulletin_subjects", cleanInst)] = metric.Current
		case "BulletinBoardSubcriptionCount":
			mx[fmt.Sprintf("ha_manager_%s_bulletin_subscriptions", cleanInst)] = metric.Current
		case "LocalBulletinBoardSubjectCount":
			mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subjects", cleanInst)] = metric.Current
		case "LocalBulletinBoardSubcriptionCount":
			mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subscriptions", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "ha_manager", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		switch metric.Name {
		case "LocalGroupCount":
			mx[fmt.Sprintf("ha_manager_%s_local_groups", cleanInst)] = metric.Value
		case "BulletinBoardSubjectCount":
			mx[fmt.Sprintf("ha_manager_%s_bulletin_subjects", cleanInst)] = metric.Value
		case "BulletinBoardSubcriptionCount":
			mx[fmt.Sprintf("ha_manager_%s_bulletin_subscriptions", cleanInst)] = metric.Value
		case "LocalBulletinBoardSubjectCount":
			mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subjects", cleanInst)] = metric.Value
		case "LocalBulletinBoardSubcriptionCount":
			mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subscriptions", cleanInst)] = metric.Value
		default:
			// For unknown bounded range metrics, use collection helper
			w.collectBoundedRangeMetric(mx, "ha_manager", cleanInst, metric)
		}
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// Use collection helper for all average metrics
		w.collectAverageMetric(mx, "ha_manager", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// Use collection helper for all double metrics
		w.collectDoubleMetric(mx, "ha_manager", cleanInst, metric)
	}
}

// Chart creation functions for new parsers

func (w *WebSpherePMI) ensureInterceptorCharts(instance, nodeName, serverName, interceptorName string) {
	chartKey := fmt.Sprintf("interceptor_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := interceptorChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "interceptor", Value: interceptorName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensurePortletCharts(instance, nodeName, serverName, portletName string) {
	chartKey := fmt.Sprintf("portlet_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := portletChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "portlet", Value: portletName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureWebServiceCharts(instance, nodeName, serverName, moduleName string) {
	chartKey := fmt.Sprintf("web_service_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := webServiceChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "module", Value: moduleName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureURLCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("urls_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := urlChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureServletURLCharts(instance, nodeName, serverName, urlPath string) {
	chartKey := fmt.Sprintf("servlet_url_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := servletURLChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
				module.Label{Key: "url", Value: urlPath},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

func (w *WebSpherePMI) ensureHAManagerCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("ha_manager_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := haManagerChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = w.createChartLabels(
				module.Label{Key: "instance", Value: instance},
				module.Label{Key: "node", Value: nodeName},
				module.Label{Key: "server", Value: serverName},
			)
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// parseObjectCache handles object cache metrics (appears under various parent objects)
func (w *WebSpherePMI) parseObjectCache(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	// Object caches can appear as children of various containers
	// The parent name is important for differentiation
	parentName := ""
	if len(stat.SubStats) > 0 {
		// If it has substats, check if parent name is in the path
		// This function might be called from different contexts
		parentName = "cache" // Default parent context
	}
	
	instance := fmt.Sprintf("%s.%s.%s.%s", nodeName, serverName, parentName, stat.Name)
	
	// Create charts if not exists
	w.ensureObjectCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("object_cache", instance)
	
	// Use universal helpers to extract ALL available metrics
	cleanInst := w.cleanID(instance)
	
	// Extract ALL CountStatistics
	countMetrics := w.extractCountStatistics(stat.CountStatistics)
	for _, metric := range countMetrics {
		switch metric.Name {
		case "ObjectsOnDisk":
			mx[fmt.Sprintf("object_cache_%s_objects_on_disk", cleanInst)] = metric.Value
		case "RefreshCount":
			mx[fmt.Sprintf("object_cache_%s_refresh_count", cleanInst)] = metric.Value
		case "ReferencedObjectCount":
			mx[fmt.Sprintf("object_cache_%s_referenced_objects", cleanInst)] = metric.Value
		case "HitsOnDisk":
			mx[fmt.Sprintf("object_cache_%s_hits_on_disk", cleanInst)] = metric.Value
		case "MissesOnDisk":
			mx[fmt.Sprintf("object_cache_%s_misses_on_disk", cleanInst)] = metric.Value
		case "ExplicitInvalidationsFromDisk":
			mx[fmt.Sprintf("object_cache_%s_explicit_invalidations_from_disk", cleanInst)] = metric.Value
		case "TimeoutInvalidationsFromDisk":
			mx[fmt.Sprintf("object_cache_%s_timeout_invalidations_from_disk", cleanInst)] = metric.Value
		case "ObjectReadFromDisk":
			mx[fmt.Sprintf("object_cache_%s_objects_read_from_disk", cleanInst)] = metric.Value
		case "ObjectWritesToDisk":
			mx[fmt.Sprintf("object_cache_%s_objects_write_to_disk", cleanInst)] = metric.Value
		case "ObjectDeleteFromDisk":
			mx[fmt.Sprintf("object_cache_%s_objects_delete_from_disk", cleanInst)] = metric.Value
		case "PendingRemovalFromDisk":
			mx[fmt.Sprintf("object_cache_%s_pending_removal_from_disk", cleanInst)] = metric.Value
		case "DependentIDsOnDisk":
			mx[fmt.Sprintf("object_cache_%s_dependent_ids_on_disk", cleanInst)] = metric.Value
		case "TemplatesOnDisk":
			mx[fmt.Sprintf("object_cache_%s_templates_on_disk", cleanInst)] = metric.Value
		case "HitsInMemory":
			mx[fmt.Sprintf("object_cache_%s_hits_in_memory", cleanInst)] = metric.Value
		case "LruInvalidationsFromMemory":
			mx[fmt.Sprintf("object_cache_%s_lru_invalidations_from_memory", cleanInst)] = metric.Value
		case "ExplicitInvalidationsFromMemory":
			mx[fmt.Sprintf("object_cache_%s_explicit_invalidations_from_memory", cleanInst)] = metric.Value
		case "TimeoutInvalidationsFromMemory":
			mx[fmt.Sprintf("object_cache_%s_timeout_invalidations_from_memory", cleanInst)] = metric.Value
		case "InMemoryAndDiskCacheEntries":
			mx[fmt.Sprintf("object_cache_%s_entries_memory_and_disk", cleanInst)] = metric.Value
		case "RemoteHitCount":
			mx[fmt.Sprintf("object_cache_%s_remote_hits", cleanInst)] = metric.Value
		case "MissCount":
			mx[fmt.Sprintf("object_cache_%s_misses", cleanInst)] = metric.Value
		case "RemoteInvalidationCount":
			mx[fmt.Sprintf("object_cache_%s_remote_invalidations", cleanInst)] = metric.Value
		case "RemoteUpdateCount":
			mx[fmt.Sprintf("object_cache_%s_remote_updates", cleanInst)] = metric.Value
		case "LocalInvalidationCount":
			mx[fmt.Sprintf("object_cache_%s_local_invalidations", cleanInst)] = metric.Value
		default:
			// For unknown count metrics, use collection helper
			w.collectCountMetric(mx, "object_cache", cleanInst, metric)
		}
	}
	
	// Extract ALL TimeStatistics and process with smart processor
	timeMetrics := w.extractTimeStatistics(stat.TimeStatistics)
	labels := append([]module.Label{
		{Key: "instance", Value: instance},
		{Key: "node", Value: nodeName},
		{Key: "server", Value: serverName},
	}, w.getVersionLabels()...)
	
	for i, metric := range timeMetrics {
		w.processTimeStatisticWithContext(
			"object_cache",
			cleanInst,
			labels,
			metric,
			mx,
			100+i*10, // Offset priority after main charts
		)
	}
	
	// Extract ALL RangeStatistics
	rangeMetrics := w.extractRangeStatistics(stat.RangeStatistics)
	for _, metric := range rangeMetrics {
		switch metric.Name {
		case "ObjectsInCache":
			mx[fmt.Sprintf("object_cache_%s_objects_in_cache", cleanInst)] = metric.Current
		case "RemoteCacheEntries":
			mx[fmt.Sprintf("object_cache_%s_remote_cache_entries", cleanInst)] = metric.Current
		default:
			// For unknown range metrics, use collection helper
			w.collectRangeMetric(mx, "object_cache", cleanInst, metric)
		}
	}
	
	// Extract ALL BoundedRangeStatistics
	boundedMetrics := w.extractBoundedRangeStatistics(stat.BoundedRangeStatistics)
	for _, metric := range boundedMetrics {
		// For unknown bounded metrics, use collection helper
		w.collectBoundedRangeMetric(mx, "object_cache", cleanInst, metric)
	}
	
	// Extract ALL AverageStatistics
	avgMetrics := w.extractAverageStatistics(stat.AverageStatistics)
	for _, metric := range avgMetrics {
		// For unknown average metrics, use collection helper
		w.collectAverageMetric(mx, "object_cache", cleanInst, metric)
	}
	
	// Extract ALL DoubleStatistics
	doubleMetrics := w.extractDoubleStatistics(stat.DoubleStatistics)
	for _, metric := range doubleMetrics {
		// For unknown double metrics, use collection helper
		w.collectDoubleMetric(mx, "object_cache", cleanInst, metric)
	}
}

// ensureSessionObjectSizeTimeChart creates a chart for SessionObjectSize as TimeStatistic if it does not exist
func (w *WebSpherePMI) ensureSessionObjectSizeTimeChart(instance, nodeName, serverName, appName string) {
	chartKey := fmt.Sprintf("session_object_size_time_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		cleanInst := w.cleanID(instance)
		chart := &module.Chart{
			ID:       fmt.Sprintf("sessions_%s_object_size_time", cleanInst),
			Title:    "Session Object Size Total",
			Units:    "bytes",
			Fam:      "web/sessions/application",
			Ctx:      "websphere_pmi.session_object_size_time",
			Type:     module.Line,
			Priority: prioWebSessions + 200, // Lower priority than main session charts
			Dims: module.Dims{
				{ID: fmt.Sprintf("sessions_%s_object_size_total", cleanInst), Name: "total", Algo: module.Incremental},
			},
			Labels: []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "application", Value: appName},
			},
		}
		
		if err := w.charts.Add(chart); err != nil {
			w.Warning(err)
		}
	}
}
