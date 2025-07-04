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
	
	// Track expected vs found metrics
	expectedMetrics := []string{"LoadedServletCount", "RequestCount", "ErrorCount"}
	foundMetrics := make(map[string]bool)
	
	// Container-level metrics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "LoadedServletCount":
				mx[fmt.Sprintf("webapp_container_%s_loaded_servlets", w.cleanID(instance))] = val
				foundMetrics["LoadedServletCount"] = true
			case "RequestCount":
				mx[fmt.Sprintf("webapp_container_%s_requests", w.cleanID(instance))] = val
				foundMetrics["RequestCount"] = true
			case "ErrorCount":
				mx[fmt.Sprintf("webapp_container_%s_errors", w.cleanID(instance))] = val
				foundMetrics["ErrorCount"] = true
			}
		} else {
			w.Debugf("parseWebApplicationsContainer: Failed to parse value for %s: %v", cs.Name, err)
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("WebApplicationsContainer", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseWebApplication handles individual web application
func (w *WebSpherePMI) parseWebApplication(stat *pmiStat, nodeName, serverName string, mx map[string]int64, path []string) {
	appName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, appName)
	
	// Create charts if not exists
	w.ensureWebAppCharts(instance, nodeName, serverName, appName)
	
	// Mark as seen
	w.markInstanceSeen("webapp", instance)
	
	// Define expected metrics using the helper
	mappings := []MetricMapping{
		{MetricName: "LoadedServletCount", CollectionKey: "webapp_" + w.cleanID(instance) + "_loaded_servlets", StatType: "count"},
		{MetricName: "ReloadCount", CollectionKey: "webapp_" + w.cleanID(instance) + "_reloads", StatType: "count"},
		// REMOVED: ServicesLoaded - this metric doesn't exist in WebSphere PMI XML
	}
	
	// Use the helper to collect metrics with comprehensive logging
	w.collectMetricsWithLogging("WebApplication", stat, instance, mx, mappings)
	
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
	
	// Define expected metrics using the helper
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "RequestCount", CollectionKey: fmt.Sprintf("servlet_%s_requests", w.cleanID(instance)), StatType: "count"},
		{MetricName: "ErrorCount", CollectionKey: fmt.Sprintf("servlet_%s_errors", w.cleanID(instance)), StatType: "count"},
		// REMOVED: LoadedServletCount and ReloadCount - these are webapp-level metrics, not servlet-level
		// RangeStatistics
		{MetricName: "ConcurrentRequests", CollectionKey: fmt.Sprintf("servlet_%s_concurrent", w.cleanID(instance)), StatType: "range"},
		// TimeStatistics
		{MetricName: "ServiceTime", CollectionKey: fmt.Sprintf("servlet_%s_service_time_total", w.cleanID(instance)), StatType: "time"},
		{MetricName: "AsyncContext Response Time", CollectionKey: fmt.Sprintf("servlet_%s_async_response_time_total", w.cleanID(instance)), StatType: "time"},
	}
	
	// Use the helper to collect metrics with comprehensive logging
	w.collectMetricsWithLogging("Servlet", stat, instance, mx, mappings)
	
	// Special handling for ServiceTime count (not handled by helper)
	for _, ts := range stat.TimeStatistics {
		if ts.Name == "ServiceTime" {
			if count, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_%s_service_time_count", w.cleanID(instance))] = count
			} else {
				w.Errorf("ISSUE: parseServlet failed to parse ServiceTime count value '%s': %v", ts.Count, err)
			}
		}
		if ts.Name == "AsyncContext Response Time" {
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_%s_async_response_time_total", w.cleanID(instance))] = val
			}
		}
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
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"CreateCount",
		"InvalidateCount", 
		"TimeoutInvalidationCount",
		"NoRoomForNewSessionCount",
		"ActiveCount",
		"LiveCount",
		"ActivateNonExistSessionCount",
		"AffinityBreakCount",
		"CacheDiscardCount",
		"ExternalReadTime",
		"ExternalWriteTime",
		"LifeTime",
		"SessionObjectSize",
		"TimeSinceLastActivated",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "CreateCount":
				mx[fmt.Sprintf("sessions_%s_created", w.cleanID(instance))] = val
				foundMetrics["CreateCount"] = true
			case "InvalidateCount":
				mx[fmt.Sprintf("sessions_%s_invalidated", w.cleanID(instance))] = val
				foundMetrics["InvalidateCount"] = true
			case "TimeoutInvalidationCount":
				mx[fmt.Sprintf("sessions_%s_timeout_invalidated", w.cleanID(instance))] = val
				foundMetrics["TimeoutInvalidationCount"] = true
			case "NoRoomForNewSessionCount":
				mx[fmt.Sprintf("sessions_%s_rejected", w.cleanID(instance))] = val
				foundMetrics["NoRoomForNewSessionCount"] = true
			case "ActivateNonExistSessionCount":
				mx[fmt.Sprintf("sessions_%s_activate_nonexist", w.cleanID(instance))] = val
				foundMetrics["ActivateNonExistSessionCount"] = true
			case "AffinityBreakCount":
				mx[fmt.Sprintf("sessions_%s_affinity_break", w.cleanID(instance))] = val
				foundMetrics["AffinityBreakCount"] = true
			case "CacheDiscardCount":
				mx[fmt.Sprintf("sessions_%s_cache_discard", w.cleanID(instance))] = val
				foundMetrics["CacheDiscardCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseSessionMetrics failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "ActiveCount":
				mx[fmt.Sprintf("sessions_%s_active", w.cleanID(instance))] = val
				foundMetrics["ActiveCount"] = true
			case "LiveCount":
				mx[fmt.Sprintf("sessions_%s_live", w.cleanID(instance))] = val
				foundMetrics["LiveCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseSessionMetrics failed to parse RangeStatistic %s current value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "ExternalReadTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("sessions_%s_external_read_time_total", w.cleanID(instance))] = val
				foundMetrics["ExternalReadTime"] = true
			} else {
				w.Errorf("ISSUE: parseSessionMetrics failed to parse TimeStatistic ExternalReadTime total value '%s': %v", total, err)
			}
		case "ExternalWriteTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("sessions_%s_external_write_time_total", w.cleanID(instance))] = val
				foundMetrics["ExternalWriteTime"] = true
			} else {
				w.Errorf("ISSUE: parseSessionMetrics failed to parse TimeStatistic ExternalWriteTime total value '%s': %v", total, err)
			}
		case "LifeTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("sessions_%s_life_time_total", w.cleanID(instance))] = val
				foundMetrics["LifeTime"] = true
			} else {
				w.Errorf("ISSUE: parseSessionMetrics failed to parse TimeStatistic LifeTime total value '%s': %v", total, err)
			}
		case "SessionObjectSize":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("sessions_%s_object_size_total", w.cleanID(instance))] = val
				foundMetrics["SessionObjectSize"] = true
			} else {
				w.Errorf("ISSUE: parseSessionMetrics failed to parse TimeStatistic SessionObjectSize total value '%s': %v", total, err)
			}
		case "TimeSinceLastActivated":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("sessions_%s_time_since_activated_total", w.cleanID(instance))] = val
				foundMetrics["TimeSinceLastActivated"] = true
			} else {
				w.Errorf("ISSUE: parseSessionMetrics failed to parse TimeStatistic TimeSinceLastActivated total value '%s': %v", total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("SessionMetrics", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseCacheComponent handles standalone cache components like "Object Cache" and "Counters"
func (w *WebSpherePMI) parseCacheComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("cache", instance)
	
	// Define expected metrics for operational cache components
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// Operational cache metrics (hits, misses, requests)
		{MetricName: "HitsInMemoryCount", CollectionKey: "cache_" + cleanInst + "_memory_hits", StatType: "count"},
		{MetricName: "HitsOnDiskCount", CollectionKey: "cache_" + cleanInst + "_disk_hits", StatType: "count"},
		{MetricName: "MissCount", CollectionKey: "cache_" + cleanInst + "_misses", StatType: "count"},
		{MetricName: "ClientRequestCount", CollectionKey: "cache_" + cleanInst + "_client_requests", StatType: "count"},
		{MetricName: "InMemoryAndDiskCacheEntryCount", CollectionKey: "cache_" + cleanInst + "_total_entries", StatType: "count"},
		// Additional cache invalidation metrics these components often have
		{MetricName: "ExplicitInvalidationCount", CollectionKey: "cache_" + cleanInst + "_explicit_invalidations", StatType: "count"},
		{MetricName: "LruInvalidationCount", CollectionKey: "cache_" + cleanInst + "_lru_invalidations", StatType: "count"},
		{MetricName: "TimeoutInvalidationCount", CollectionKey: "cache_" + cleanInst + "_timeout_invalidations", StatType: "count"},
		{MetricName: "RemoteHitCount", CollectionKey: "cache_" + cleanInst + "_remote_hits", StatType: "count"},
		{MetricName: "DistributedRequestCount", CollectionKey: "cache_" + cleanInst + "_distributed_requests", StatType: "count"},
	}
	
	// Use helper to collect metrics with logging
	w.collectMetricsWithLogging("CacheComponent", stat, instance, mx, mappings)
}

// parseDynamicCacheContainer handles Dynamic Cache metrics
func (w *WebSpherePMI) parseDynamicCacheContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("cache", instance)
	
	// Define expected metrics
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// Note: These are typically found in child "Object:" stats, not at container level
		{MetricName: "MaxInMemoryCacheEntryCount", CollectionKey: "cache_" + cleanInst + "_max_entries", StatType: "count"},
		{MetricName: "InMemoryCacheEntryCount", CollectionKey: "cache_" + cleanInst + "_in_memory_entries", StatType: "count"},
		{MetricName: "HitsInMemoryCount", CollectionKey: "cache_" + cleanInst + "_memory_hits", StatType: "count"},
		{MetricName: "HitsOnDiskCount", CollectionKey: "cache_" + cleanInst + "_disk_hits", StatType: "count"},
		{MetricName: "MissCount", CollectionKey: "cache_" + cleanInst + "_misses", StatType: "count"},
		{MetricName: "ClientRequestCount", CollectionKey: "cache_" + cleanInst + "_client_requests", StatType: "count"},
		{MetricName: "InMemoryAndDiskCacheEntryCount", CollectionKey: "cache_" + cleanInst + "_total_entries", StatType: "count"},
	}
	
	// Use helper to collect metrics with logging
	w.collectMetricsWithLogging("DynamicCacheContainer", stat, instance, mx, mappings)
	
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
	cacheName := strings.TrimPrefix(stat.Name, "Object: ")
	// Use aggregated instance name for NIDL compliance
	instance := fmt.Sprintf("%s.%s.Object_Cache", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureObjectCacheCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("object_cache", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"InMemoryCacheEntryCount",
		"MaxInMemoryCacheEntryCount",
		"HitsInMemoryCount",
		"HitsOnDiskCount",
		"MissCount",
		"InMemoryAndDiskCacheEntryCount",
	}
	foundMetrics := make(map[string]bool)
	
	// CRITICAL: Process direct CountStatistics of "Object: [cache_name]" stat first
	// These contain InMemoryCacheEntryCount and MaxInMemoryCacheEntryCount
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "InMemoryCacheEntryCount":
				mx[fmt.Sprintf("object_cache_%s_objects", w.cleanID(instance))] = val
				foundMetrics["InMemoryCacheEntryCount"] = true
			case "MaxInMemoryCacheEntryCount":
				mx[fmt.Sprintf("object_cache_%s_max_objects", w.cleanID(instance))] = val
				foundMetrics["MaxInMemoryCacheEntryCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseCacheObject failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Look for "Object Cache" sub-stat for additional metrics
	objectCacheFound := false
	countersFound := false
	for _, child := range stat.SubStats {
		if child.Name == "Object Cache" {
			objectCacheFound = true
			// Look for "Counters" sub-stat
			for _, counters := range child.SubStats {
				if counters.Name == "Counters" {
					countersFound = true
					// Process counter metrics
					for _, cs := range counters.CountStatistics {
						if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
							switch cs.Name {
							case "HitsInMemoryCount":
								mx[fmt.Sprintf("object_cache_%s_memory_hits", w.cleanID(instance))] = val
								foundMetrics["HitsInMemoryCount"] = true
							case "HitsOnDiskCount":
								mx[fmt.Sprintf("object_cache_%s_disk_hits", w.cleanID(instance))] = val
								foundMetrics["HitsOnDiskCount"] = true
							case "MissCount":
								mx[fmt.Sprintf("object_cache_%s_misses", w.cleanID(instance))] = val
								foundMetrics["MissCount"] = true
							case "InMemoryAndDiskCacheEntryCount":
								mx[fmt.Sprintf("object_cache_%s_total_entries", w.cleanID(instance))] = val
								foundMetrics["InMemoryAndDiskCacheEntryCount"] = true
							}
						} else {
							w.Errorf("ISSUE: parseCacheObject failed to parse CountStatistic %s value '%s' in Counters: %v", cs.Name, cs.Count, err)
						}
					}
				}
			}
			
			if !countersFound {
				w.Errorf("ISSUE: parseCacheObject found 'Object Cache' sub-stat but no 'Counters' sub-stat for cache '%s'", cacheName)
			}
		}
	}
	
	if !objectCacheFound {
		w.Errorf("ISSUE: parseCacheObject did not find 'Object Cache' sub-stat for cache '%s'", cacheName)
	}
	
	// Log any missing metrics
	w.logParsingFailure("CacheObject", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseORB handles ORB (Object Request Broker) metrics
func (w *WebSpherePMI) parseORB(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureORBCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("orb", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"RequestCount",
		"ConcurrentRequestCount",
		"LookupTime",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "RequestCount":
				mx[fmt.Sprintf("orb_%s_requests", w.cleanID(instance))] = val
				foundMetrics["RequestCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseORB failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "ConcurrentRequestCount":
				mx[fmt.Sprintf("orb_%s_concurrent_requests", w.cleanID(instance))] = val
				foundMetrics["ConcurrentRequestCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseORB failed to parse RangeStatistic %s current value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "LookupTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("orb_%s_lookup_time_total", w.cleanID(instance))] = val
				foundMetrics["LookupTime"] = true
			} else {
				w.Errorf("ISSUE: parseORB failed to parse TimeStatistic %s total value '%s': %v", ts.Name, total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("ORB", stat.Name, stat, expectedMetrics, foundMetrics)
}

// Chart creation helpers
func (w *WebSpherePMI) ensureWebAppCharts(instance, nodeName, serverName, appName string) {
	chartKey := fmt.Sprintf("webapp_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		// Create web app charts
		charts := webAppChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "application", Value: appName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "application", Value: appName},
				{Key: "servlet", Value: servletName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "application", Value: appName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "cache", Value: cacheName},
			}
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
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "pool", Value: poolName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
		Fam:      "object_pools",
		Ctx:      "websphere_pmi.object_pool_objects",
		Type:     module.Line,
		Priority: prioObjectPoolObjects,
		Dims: module.Dims{
			{ID: "object_pool_%s_idle", Name: "idle"},
		},
	},
	{
		ID:       "object_pool_%s_lifecycle",
		Title:    "Object Pool Lifecycle",
		Units:    "objects/s",
		Fam:      "object_pools",
		Ctx:      "websphere_pmi.object_pool_lifecycle",
		Type:     module.Line,
		Priority: prioObjectPoolLifecycle,
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
		Fam:      "object_cache",
		Ctx:      "websphere_pmi.object_cache_objects",
		Type:     module.Line,
		Priority: prioObjectCacheObjects,
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
		Fam:      "object_cache",
		Ctx:      "websphere_pmi.object_cache_hits",
		Type:     module.Line,
		Priority: prioObjectCacheHits,
		Dims: module.Dims{
			{ID: "object_cache_%s_memory_hits", Name: "memory_hits", Algo: module.Incremental},
			{ID: "object_cache_%s_disk_hits", Name: "disk_hits", Algo: module.Incremental},
			{ID: "object_cache_%s_misses", Name: "misses", Algo: module.Incremental},
		},
	},
}

var webAppContainerChartsTmpl = module.Charts{
	{
		ID:       "webapp_container_%s_servlets",
		Title:    "Web Application Container Servlets",
		Units:    "servlets",
		Fam:      "webapp_container",
		Ctx:      "websphere_pmi.webapp_container_servlets",
		Type:     module.Line,
		Priority: prioWebAppContainerServlets,
		Dims: module.Dims{
			{ID: "webapp_container_%s_loaded_servlets", Name: "loaded"},
		},
	},
	{
		ID:       "webapp_container_%s_requests",
		Title:    "Web Application Container Requests",
		Units:    "requests/s",
		Fam:      "webapp_container",
		Ctx:      "websphere_pmi.webapp_container_requests",
		Type:     module.Line,
		Priority: prioWebAppContainerRequests,
		Dims: module.Dims{
			{ID: "webapp_container_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "webapp_container_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	},
}

var cacheChartsTmpl = module.Charts{
	{
		ID:       "cache_%s_entries",
		Title:    "Dynamic Cache Entries",
		Units:    "entries",
		Fam:      "cache",
		Ctx:      "websphere_pmi.cache_entries",
		Type:     module.Line,
		Priority: prioCacheEntries,
		Dims: module.Dims{
			{ID: "cache_%s_in_memory_entries", Name: "in_memory"},
			{ID: "cache_%s_total_entries", Name: "total"},
			{ID: "cache_%s_max_entries", Name: "max"},
		},
	},
	{
		ID:       "cache_%s_hits",
		Title:    "Dynamic Cache Hits",
		Units:    "hits/s",
		Fam:      "cache",
		Ctx:      "websphere_pmi.cache_hits",
		Type:     module.Line,
		Priority: prioCacheHits,
		Dims: module.Dims{
			{ID: "cache_%s_memory_hits", Name: "memory", Algo: module.Incremental},
			{ID: "cache_%s_disk_hits", Name: "disk", Algo: module.Incremental},
			{ID: "cache_%s_remote_hits", Name: "remote", Algo: module.Incremental},
			{ID: "cache_%s_misses", Name: "misses", Algo: module.Incremental},
		},
	},
	{
		ID:       "cache_%s_requests",
		Title:    "Dynamic Cache Requests",
		Units:    "requests/s",
		Fam:      "cache",
		Ctx:      "websphere_pmi.cache_requests",
		Type:     module.Line,
		Priority: prioCacheHits + 1,
		Dims: module.Dims{
			{ID: "cache_%s_client_requests", Name: "client", Algo: module.Incremental},
			{ID: "cache_%s_distributed_requests", Name: "distributed", Algo: module.Incremental},
		},
	},
	{
		ID:       "cache_%s_invalidations",
		Title:    "Dynamic Cache Invalidations",
		Units:    "invalidations/s",
		Fam:      "cache",
		Ctx:      "websphere_pmi.cache_invalidations",
		Type:     module.Line,
		Priority: prioCacheHits + 2,
		Dims: module.Dims{
			{ID: "cache_%s_explicit_invalidations", Name: "explicit", Algo: module.Incremental},
			{ID: "cache_%s_lru_invalidations", Name: "lru", Algo: module.Incremental},
			{ID: "cache_%s_timeout_invalidations", Name: "timeout", Algo: module.Incremental},
		},
	},
}

var cacheObjectChartsTmpl = module.Charts{
	{
		ID:       "cache_object_%s_entries",
		Title:    "Cache Object Entries",
		Units:    "entries",
		Fam:      "cache_objects",
		Ctx:      "websphere_pmi.cache_object_entries",
		Type:     module.Line,
		Priority: prioCacheObjectEntries,
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
		Fam:      "webapps",
		Ctx:      "websphere_pmi.webapp_servlets",
		Type:     module.Line,
		Priority: prioWebAppServlets,
		Dims: module.Dims{
			{ID: "webapp_%s_loaded_servlets", Name: "loaded"},
		},
	},
	{
		ID:       "webapp_%s_reloads",
		Title:    "Web Application Reloads",
		Units:    "reloads/s",
		Fam:      "webapps",
		Ctx:      "websphere_pmi.webapp_reloads",
		Type:     module.Line,
		Priority: prioWebAppReloads,
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
		Fam:      "servlets",
		Ctx:      "websphere_pmi.servlet_requests",
		Type:     module.Line,
		Priority: prioServletRequests,
		Dims: module.Dims{
			{ID: "servlet_%s_requests", Name: "requests", Algo: module.Incremental},
			{ID: "servlet_%s_errors", Name: "errors", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlet_%s_response_time",
		Title:    "Servlet Response Time",
		Units:    "milliseconds/s",
		Fam:      "servlets",
		Ctx:      "websphere_pmi.servlet_response_time",
		Type:     module.Line,
		Priority: prioServletResponseTime,
		Dims: module.Dims{
			{ID: "servlet_%s_service_time_total", Name: "service_time", Algo: module.Incremental},
			{ID: "servlet_%s_async_response_time_total", Name: "async_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "servlet_%s_service_count",
		Title:    "Servlet Service Count",
		Units:    "requests",
		Fam:      "servlets",
		Ctx:      "websphere_pmi.servlet_service_count",
		Type:     module.Line,
		Priority: prioServletServiceCount,
		Dims: module.Dims{
			{ID: "servlet_%s_service_time_count", Name: "service_count"},
		},
	},
	{
		ID:       "servlet_%s_concurrent",
		Title:    "Servlet Concurrent Requests",
		Units:    "requests",
		Fam:      "servlets",
		Ctx:      "websphere_pmi.servlet_concurrent",
		Type:     module.Line,
		Priority: prioServletServiceCount + 1,
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
		Fam:      "sessions",
		Ctx:      "websphere_pmi.sessions_active",
		Type:     module.Line,
		Priority: prioSessionsActive,
		Dims: module.Dims{
			{ID: "sessions_%s_active", Name: "active"},
			{ID: "sessions_%s_live", Name: "live"},
		},
	},
	{
		ID:       "sessions_%s_lifecycle",
		Title:    "Session Lifecycle",
		Units:    "sessions/s",
		Fam:      "sessions",
		Ctx:      "websphere_pmi.sessions_lifecycle",
		Type:     module.Line,
		Priority: prioSessionsLifecycle,
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
		Fam:      "sessions",
		Ctx:      "websphere_pmi.sessions_errors",
		Type:     module.Line,
		Priority: prioSessionsLifecycle + 1,
		Dims: module.Dims{
			{ID: "sessions_%s_activate_nonexist", Name: "activate_nonexist", Algo: module.Incremental},
			{ID: "sessions_%s_affinity_break", Name: "affinity_break", Algo: module.Incremental},
			{ID: "sessions_%s_cache_discard", Name: "cache_discard", Algo: module.Incremental},
		},
	},
	{
		ID:       "sessions_%s_external_time",
		Title:    "Session External Storage Time",
		Units:    "milliseconds/s",
		Fam:      "sessions",
		Ctx:      "websphere_pmi.sessions_external_time",
		Type:     module.Line,
		Priority: prioSessionsLifecycle + 2,
		Dims: module.Dims{
			{ID: "sessions_%s_external_read_time_total", Name: "read_time", Algo: module.Incremental},
			{ID: "sessions_%s_external_write_time_total", Name: "write_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "sessions_%s_timing",
		Title:    "Session Timing",
		Units:    "milliseconds/s",
		Fam:      "sessions",
		Ctx:      "websphere_pmi.sessions_timing",
		Type:     module.Line,
		Priority: prioSessionsLifecycle + 3,
		Dims: module.Dims{
			{ID: "sessions_%s_life_time_total", Name: "life_time", Algo: module.Incremental},
			{ID: "sessions_%s_time_since_activated_total", Name: "time_since_activated", Algo: module.Incremental},
		},
	},
	{
		ID:       "sessions_%s_object_size",
		Title:    "Session Object Size",
		Units:    "bytes/s",
		Fam:      "sessions",
		Ctx:      "websphere_pmi.sessions_object_size",
		Type:     module.Line,
		Priority: prioSessionsLifecycle + 4,
		Dims: module.Dims{
			{ID: "sessions_%s_object_size_total", Name: "object_size", Algo: module.Incremental},
		},
	},
}

// Chart templates for new parsers
var jcaPoolChartsTmpl = module.Charts{
	{
		ID:       "jca_pool_%s_connections",
		Title:    "JCA Connection Pool Connections",
		Units:    "connections",
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_connections",
		Type:     module.Line,
		Priority: prioJCAPoolConnections,
		Dims: module.Dims{
			{ID: "jca_pool_%s_free", Name: "free"},
			{ID: "jca_pool_%s_size", Name: "total"},
		},
	},
	{
		ID:       "jca_pool_%s_lifecycle",
		Title:    "JCA Connection Pool Lifecycle",
		Units:    "connections/s",
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_lifecycle",
		Type:     module.Line,
		Priority: prioJCAPoolLifecycle,
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
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_faults",
		Type:     module.Line,
		Priority: prioJCAPoolConnections + 2,
		Dims: module.Dims{
			{ID: "jca_pool_%s_faults", Name: "faults", Algo: module.Incremental},
		},
	},
	{
		ID:       "jca_pool_%s_managed_connections",
		Title:    "JCA Managed Connections",
		Units:    "connections",
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_managed_connections",
		Type:     module.Line,
		Priority: prioJCAPoolConnections + 3,
		Dims: module.Dims{
			{ID: "jca_pool_%s_managed_connections", Name: "managed"},
			{ID: "jca_pool_%s_connection_handles", Name: "handles"},
		},
	},
	{
		ID:       "jca_pool_%s_utilization",
		Title:    "JCA Connection Pool Utilization",
		Units:    "percentage",
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_utilization",
		Type:     module.Line,
		Priority: prioJCAPoolConnections + 4,
		Dims: module.Dims{
			{ID: "jca_pool_%s_percent_used", Name: "used"},
			{ID: "jca_pool_%s_percent_maxed", Name: "maxed"},
		},
	},
	{
		ID:       "jca_pool_%s_wait",
		Title:    "JCA Connection Pool Wait",
		Units:    "threads",
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_wait",
		Type:     module.Line,
		Priority: prioJCAPoolConnections + 5,
		Dims: module.Dims{
			{ID: "jca_pool_%s_waiting_threads", Name: "waiting"},
		},
	},
	{
		ID:       "jca_pool_%s_time",
		Title:    "JCA Connection Pool Time",
		Units:    "milliseconds/s",
		Fam:      "jca_pools",
		Ctx:      "websphere_pmi.jca_pool_time",
		Type:     module.Line,
		Priority: prioJCAPoolConnections + 6,
		Dims: module.Dims{
			{ID: "jca_pool_%s_wait_time_total", Name: "wait_time", Algo: module.Incremental},
			{ID: "jca_pool_%s_use_time_total", Name: "use_time", Algo: module.Incremental},
		},
	},
}

var enterpriseAppChartsTmpl = module.Charts{
	{
		ID:       "enterprise_app_%s_metrics",
		Title:    "Enterprise Application Metrics",
		Units:    "metrics",
		Fam:      "enterprise_apps",
		Ctx:      "websphere_pmi.enterprise_app_metrics",
		Type:     module.Line,
		Priority: prioEnterpriseAppMetrics,
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
		Fam:      "system_data",
		Ctx:      "websphere_pmi.system_data_cpu",
		Type:     module.Line,
		Priority: prioSystemDataMetrics,
		Dims: module.Dims{
			{ID: "system_data_%s_cpu_usage", Name: "current"},
			{ID: "system_data_%s_cpu_average", Name: "average", Div: 100}, // Convert back from precision
		},
	},
	{
		ID:       "system_data_%s_memory",
		Title:    "System Free Memory",
		Units:    "bytes",
		Fam:      "system_data",
		Ctx:      "websphere_pmi.system_data_memory",
		Type:     module.Line,
		Priority: prioSystemDataMetrics + 10,
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
		Priority: prioWLMMetrics,
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
		Fam:      "bean_manager",
		Ctx:      "websphere_pmi.bean_manager_beans",
		Type:     module.Line,
		Priority: prioBeanManagerMetrics,
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
		Fam:      "connection_manager",
		Ctx:      "websphere_pmi.connection_manager_connections",
		Type:     module.Line,
		Priority: prioConnectionManagerMetrics,
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
		Fam:      "jvm_subsystem",
		Ctx:      "websphere_pmi.jvm_subsystem_metrics",
		Type:     module.Line,
		Priority: prioJVMSubsystemMetrics,
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
		Fam:      "ejb_container",
		Ctx:      "websphere_pmi.ejb_container_methods",
		Type:     module.Line,
		Priority: prioEJBContainerMetrics,
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
		Fam:      "mdb",
		Ctx:      "websphere_pmi.mdb_messages",
		Type:     module.Line,
		Priority: prioMDBMetrics,
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
		Fam:      "sfsb",
		Ctx:      "websphere_pmi.sfsb_instances",
		Type:     module.Line,
		Priority: prioSFSBMetrics,
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
		Fam:      "slsb",
		Ctx:      "websphere_pmi.slsb_methods",
		Type:     module.Line,
		Priority: prioSLSBMetrics,
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
		Fam:      "entity_beans",
		Ctx:      "websphere_pmi.entity_bean_lifecycle",
		Type:     module.Line,
		Priority: prioEntityBeanMetrics,
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
		Fam:      "generic_ejb",
		Ctx:      "websphere_pmi.generic_ejb_operations",
		Type:     module.Line,
		Priority: prioGenericEJBMetrics,
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
		Fam:      "orb",
		Ctx:      "websphere_pmi.orb_requests",
		Type:     module.Line,
		Priority: prioORBRequests,
		Dims: module.Dims{
			{ID: "orb_%s_requests", Name: "requests", Algo: module.Incremental},
		},
	},
	{
		ID:       "orb_%s_concurrent",
		Title:    "ORB Concurrent Requests",
		Units:    "requests",
		Fam:      "orb",
		Ctx:      "websphere_pmi.orb_concurrent",
		Type:     module.Line,
		Priority: prioORBConcurrent,
		Dims: module.Dims{
			{ID: "orb_%s_concurrent_requests", Name: "concurrent"},
		},
	},
	{
		ID:       "orb_%s_lookup_time",
		Title:    "ORB Lookup Time",
		Units:    "milliseconds/s",
		Fam:      "orb",
		Ctx:      "websphere_pmi.orb_lookup_time",
		Type:     module.Line,
		Priority: prioORBLookupTime,
		Dims: module.Dims{
			{ID: "orb_%s_lookup_time_total", Name: "lookup_time", Algo: module.Incremental},
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
	
	// Define expected metrics mappings - collect ALL available metrics
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "CreateCount", CollectionKey: fmt.Sprintf("jca_pool_%s_created", w.cleanID(instance)), StatType: "count"},
		{MetricName: "CloseCount", CollectionKey: fmt.Sprintf("jca_pool_%s_closed", w.cleanID(instance)), StatType: "count"},
		{MetricName: "AllocateCount", CollectionKey: fmt.Sprintf("jca_pool_%s_allocated", w.cleanID(instance)), StatType: "count"},
		{MetricName: "FreedCount", CollectionKey: fmt.Sprintf("jca_pool_%s_returned", w.cleanID(instance)), StatType: "count"}, // Map FreedCount to returned
		{MetricName: "FaultCount", CollectionKey: fmt.Sprintf("jca_pool_%s_faults", w.cleanID(instance)), StatType: "count"},
		{MetricName: "ManagedConnectionCount", CollectionKey: fmt.Sprintf("jca_pool_%s_managed_connections", w.cleanID(instance)), StatType: "count"},
		{MetricName: "ConnectionHandleCount", CollectionKey: fmt.Sprintf("jca_pool_%s_connection_handles", w.cleanID(instance)), StatType: "count"},
		
		// BoundedRangeStatistics
		{MetricName: "FreePoolSize", CollectionKey: fmt.Sprintf("jca_pool_%s_free", w.cleanID(instance)), StatType: "bounded_range"},
		{MetricName: "PoolSize", CollectionKey: fmt.Sprintf("jca_pool_%s_size", w.cleanID(instance)), StatType: "bounded_range"},
		
		// RangeStatistics
		{MetricName: "WaitingThreadCount", CollectionKey: fmt.Sprintf("jca_pool_%s_waiting_threads", w.cleanID(instance)), StatType: "range"},
		{MetricName: "PercentUsed", CollectionKey: fmt.Sprintf("jca_pool_%s_percent_used", w.cleanID(instance)), StatType: "range"},
		{MetricName: "PercentMaxed", CollectionKey: fmt.Sprintf("jca_pool_%s_percent_maxed", w.cleanID(instance)), StatType: "range"},
		
		// TimeStatistics
		{MetricName: "WaitTime", CollectionKey: fmt.Sprintf("jca_pool_%s_wait_time_total", w.cleanID(instance)), StatType: "time"},
		{MetricName: "UseTime", CollectionKey: fmt.Sprintf("jca_pool_%s_use_time_total", w.cleanID(instance)), StatType: "time"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("JCAConnectionPool", stat, instance, mx, mappings)
}

// parseEnterpriseApplication handles Enterprise Application metrics
func (w *WebSpherePMI) parseEnterpriseApplication(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	appName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, appName)
	
	// Create charts if not exists
	w.ensureEnterpriseAppCharts(instance, nodeName, serverName, appName)
	
	// Mark as seen
	w.markInstanceSeen("enterprise_app", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "StartupTime", CollectionKey: fmt.Sprintf("enterprise_app_%s_startup_time", w.cleanID(instance)), StatType: "count"},
		{MetricName: "LoadCount", CollectionKey: fmt.Sprintf("enterprise_app_%s_loads", w.cleanID(instance)), StatType: "count"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("EnterpriseApplication", stat, instance, mx, mappings)
}

// parseSystemData handles System Data metrics
func (w *WebSpherePMI) parseSystemData(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSystemDataCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("system_data", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"CPUUsageSinceLastMeasurement",
		"FreeMemory",
		"CPUUsageSinceServerStarted",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "CPUUsageSinceLastMeasurement":
				mx[fmt.Sprintf("system_data_%s_cpu_usage", w.cleanID(instance))] = val
				foundMetrics["CPUUsageSinceLastMeasurement"] = true
			case "FreeMemory":
				mx[fmt.Sprintf("system_data_%s_free_memory", w.cleanID(instance))] = val
				foundMetrics["FreeMemory"] = true
			}
		} else {
			w.Errorf("ISSUE: parseSystemData failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process AverageStatistics (additional CPU metrics)
	for _, as := range stat.AverageStatistics {
		switch as.Name {
		case "CPUUsageSinceServerStarted":
			// Use Mean value for average CPU usage
			if mean := as.Mean; mean != "" {
				if val, err := strconv.ParseFloat(mean, 64); err == nil {
					// Convert to integer with precision
					mx[fmt.Sprintf("system_data_%s_cpu_average", w.cleanID(instance))] = int64(val * 100) // percentage with 2 decimal precision
					foundMetrics["CPUUsageSinceServerStarted"] = true
				} else {
					w.Errorf("ISSUE: parseSystemData failed to parse AverageStatistic CPUUsageSinceServerStarted mean value '%s': %v", mean, err)
				}
			} else {
				w.Errorf("ISSUE: parseSystemData AverageStatistic CPUUsageSinceServerStarted has empty mean value")
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("SystemData", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseWLM handles Work Load Management metrics
func (w *WebSpherePMI) parseWLM(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWLMCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("wlm", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "RequestCount", CollectionKey: fmt.Sprintf("wlm_%s_requests", w.cleanID(instance)), StatType: "count"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("WLM", stat, instance, mx, mappings)
}

// parseBeanManager handles Bean Manager metrics
func (w *WebSpherePMI) parseBeanManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureBeanManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("bean_manager", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// RangeStatistics
		{MetricName: "LiveCount", CollectionKey: fmt.Sprintf("bean_manager_%s_live_beans", w.cleanID(instance)), StatType: "range"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("BeanManager", stat, instance, mx, mappings)
}

// parseConnectionManager handles Connection Manager metrics
func (w *WebSpherePMI) parseConnectionManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureConnectionManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("connection_manager", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// RangeStatistics
		{MetricName: "AllocatedCount", CollectionKey: fmt.Sprintf("connection_manager_%s_allocated", w.cleanID(instance)), StatType: "range"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("ConnectionManager", stat, instance, mx, mappings)
}

// parseJVMSubsystem handles JVM subsystem metrics
func (w *WebSpherePMI) parseJVMSubsystem(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	subsystem := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, subsystem)
	
	// Create charts if not exists
	w.ensureJVMSubsystemCharts(instance, nodeName, serverName, subsystem)
	
	// Mark as seen
	w.markInstanceSeen("jvm_subsystem", instance)
	
	// Track expected vs found metrics based on subsystem
	var expectedMetrics []string
	switch stat.Name {
	case "JVM.GC":
		expectedMetrics = []string{"HeapDiscarded", "GCTime"}
	case "JVM.Memory":
		expectedMetrics = []string{"AllocatedMemory"}
	case "JVM.Thread":
		expectedMetrics = []string{"ActiveThreads"}
	}
	foundMetrics := make(map[string]bool)
	
	// Process JVM subsystem metrics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch stat.Name {
			case "JVM.GC":
				switch cs.Name {
				case "HeapDiscarded":
					mx[fmt.Sprintf("jvm_gc_%s_heap_discarded", w.cleanID(instance))] = val
					foundMetrics["HeapDiscarded"] = true
				case "GCTime":
					mx[fmt.Sprintf("jvm_gc_%s_time", w.cleanID(instance))] = val
					foundMetrics["GCTime"] = true
				}
			case "JVM.Memory":
				switch cs.Name {
				case "AllocatedMemory":
					mx[fmt.Sprintf("jvm_memory_%s_allocated", w.cleanID(instance))] = val
					foundMetrics["AllocatedMemory"] = true
				}
			case "JVM.Thread":
				switch cs.Name {
				case "ActiveThreads":
					mx[fmt.Sprintf("jvm_thread_%s_active", w.cleanID(instance))] = val
					foundMetrics["ActiveThreads"] = true
				}
			}
		} else {
			w.Errorf("ISSUE: parseJVMSubsystem failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("JVMSubsystem", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseEJBContainer handles EJB container metrics
func (w *WebSpherePMI) parseEJBContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureEJBContainerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("ejb_container", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "TotalMethodCalls", CollectionKey: fmt.Sprintf("ejb_container_%s_method_calls", w.cleanID(instance)), StatType: "count"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("EJBContainer", stat, instance, mx, mappings)
}

// parseMDB handles Message Driven Bean metrics
func (w *WebSpherePMI) parseMDB(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureMDBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("mdb", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "MessageCount", CollectionKey: fmt.Sprintf("mdb_%s_messages", w.cleanID(instance)), StatType: "count"},
		{MetricName: "MessageBackoutCount", CollectionKey: fmt.Sprintf("mdb_%s_backouts", w.cleanID(instance)), StatType: "count"},
		// TimeStatistics - similar to other beans
		{MetricName: "ServiceTime", CollectionKey: fmt.Sprintf("mdb_%s_service_time", w.cleanID(instance)), StatType: "time"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("MDB", stat, instance, mx, mappings)
}

// parseStatefulSessionBean handles Stateful Session Bean metrics
func (w *WebSpherePMI) parseStatefulSessionBean(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureSFSBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("sfsb", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// RangeStatistics
		{MetricName: "LiveCount", CollectionKey: fmt.Sprintf("sfsb_%s_live", w.cleanID(instance)), StatType: "range"},
		// CountStatistics - similar to other session beans
		{MetricName: "CreateCount", CollectionKey: fmt.Sprintf("sfsb_%s_created", w.cleanID(instance)), StatType: "count"},
		{MetricName: "RemoveCount", CollectionKey: fmt.Sprintf("sfsb_%s_removed", w.cleanID(instance)), StatType: "count"},
		{MetricName: "ActivateCount", CollectionKey: fmt.Sprintf("sfsb_%s_activated", w.cleanID(instance)), StatType: "count"},
		{MetricName: "PassivateCount", CollectionKey: fmt.Sprintf("sfsb_%s_passivated", w.cleanID(instance)), StatType: "count"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("StatefulSessionBean", stat, instance, mx, mappings)
}

// parseStatelessSessionBean handles Stateless Session Bean metrics
func (w *WebSpherePMI) parseStatelessSessionBean(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureSLSBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("slsb", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "MethodCallCount", CollectionKey: fmt.Sprintf("slsb_%s_method_calls", w.cleanID(instance)), StatType: "count"},
		{MetricName: "CreateCount", CollectionKey: fmt.Sprintf("slsb_%s_created", w.cleanID(instance)), StatType: "count"},
		{MetricName: "RemoveCount", CollectionKey: fmt.Sprintf("slsb_%s_removed", w.cleanID(instance)), StatType: "count"},
		// TimeStatistics
		{MetricName: "ServiceTime", CollectionKey: fmt.Sprintf("slsb_%s_service_time", w.cleanID(instance)), StatType: "time"},
		// RangeStatistics
		{MetricName: "PooledCount", CollectionKey: fmt.Sprintf("slsb_%s_pooled", w.cleanID(instance)), StatType: "range"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("StatelessSessionBean", stat, instance, mx, mappings)
}

// parseEntityBean handles Entity Bean metrics
func (w *WebSpherePMI) parseEntityBean(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureEntityBeanCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("entity_bean", instance)
	
	// Define expected metrics mappings
	mappings := []MetricMapping{
		// CountStatistics
		{MetricName: "ActivateCount", CollectionKey: fmt.Sprintf("entity_bean_%s_activations", w.cleanID(instance)), StatType: "count"},
		{MetricName: "PassivateCount", CollectionKey: fmt.Sprintf("entity_bean_%s_passivations", w.cleanID(instance)), StatType: "count"},
		{MetricName: "CreateCount", CollectionKey: fmt.Sprintf("entity_bean_%s_created", w.cleanID(instance)), StatType: "count"},
		{MetricName: "RemoveCount", CollectionKey: fmt.Sprintf("entity_bean_%s_removed", w.cleanID(instance)), StatType: "count"},
		{MetricName: "LoadCount", CollectionKey: fmt.Sprintf("entity_bean_%s_loaded", w.cleanID(instance)), StatType: "count"},
		{MetricName: "StoreCount", CollectionKey: fmt.Sprintf("entity_bean_%s_stored", w.cleanID(instance)), StatType: "count"},
		// RangeStatistics
		{MetricName: "LiveCount", CollectionKey: fmt.Sprintf("entity_bean_%s_live", w.cleanID(instance)), StatType: "range"},
		{MetricName: "PooledCount", CollectionKey: fmt.Sprintf("entity_bean_%s_pooled", w.cleanID(instance)), StatType: "range"},
		{MetricName: "ReadyCount", CollectionKey: fmt.Sprintf("entity_bean_%s_ready", w.cleanID(instance)), StatType: "range"},
	}
	
	// Use helper for comprehensive error logging
	w.collectMetricsWithLogging("EntityBean", stat, instance, mx, mappings)
}

// parseIndividualEJB handles individual EJB instances found dynamically
func (w *WebSpherePMI) parseIndividualEJB(stat *pmiStat, nodeName, serverName string, mx map[string]int64, path []string) {
	beanName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, beanName)
	
	// Create charts if not exists
	w.ensureGenericEJBCharts(instance, nodeName, serverName, beanName)
	
	// Mark as seen
	w.markInstanceSeen("generic_ejb", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"MethodCallCount", "InvocationCount",
		"CreateCount", "RemoveCount",
		"ServiceTime", "ResponseTime",
		"LiveCount", "PooledCount",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			// Use generic metric names that work for most EJBs
			switch cs.Name {
			case "MethodCallCount", "InvocationCount":
				mx[fmt.Sprintf("generic_ejb_%s_invocations", w.cleanID(instance))] = val
				foundMetrics["MethodCallCount"] = true
				foundMetrics["InvocationCount"] = true
			case "CreateCount":
				mx[fmt.Sprintf("generic_ejb_%s_creates", w.cleanID(instance))] = val
				foundMetrics["CreateCount"] = true
			case "RemoveCount":
				mx[fmt.Sprintf("generic_ejb_%s_removes", w.cleanID(instance))] = val
				foundMetrics["RemoveCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseIndividualEJB failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "ServiceTime", "ResponseTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("generic_ejb_%s_service_time", w.cleanID(instance))] = val
				foundMetrics["ServiceTime"] = true
				foundMetrics["ResponseTime"] = true
			} else {
				w.Errorf("ISSUE: parseIndividualEJB failed to parse TimeStatistic %s total value '%s': %v", ts.Name, total, err)
			}
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "LiveCount":
				mx[fmt.Sprintf("generic_ejb_%s_live", w.cleanID(instance))] = val
				foundMetrics["LiveCount"] = true
			case "PooledCount":
				mx[fmt.Sprintf("generic_ejb_%s_pooled", w.cleanID(instance))] = val
				foundMetrics["PooledCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseIndividualEJB failed to parse RangeStatistic %s current value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("IndividualEJB", stat.Name, stat, expectedMetrics, foundMetrics)
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
	
	for _, ts := range stat.TimeStatistics {
		total := ts.TotalTime
		if total == "" {
			total = ts.Total
		}
		if val, err := strconv.ParseInt(total, 10, 64); err == nil {
			metricKey := fmt.Sprintf("generic_stat_%s_%s_total", w.cleanID(instance), w.cleanID(ts.Name))
			mx[metricKey] = val
		}
	}
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
	
	// Add TimeStatistics dimensions
	for _, ts := range stat.TimeStatistics {
		dimID := fmt.Sprintf("generic_stat_%s_%s_total", w.cleanID(instance), w.cleanID(ts.Name))
		dims = append(dims, &module.Dim{
			ID:   dimID,
			Name: w.cleanID(ts.Name) + "_total",
			Algo: module.Incremental, // Time stats are typically incremental
		})
	}
	
	// Only create chart if we have dimensions
	if len(dims) > 0 {
		chart := &module.Chart{
			ID:       fmt.Sprintf("generic_stat_%s_metrics", w.cleanID(instance)),
			Title:    fmt.Sprintf("Generic Metrics: %s", statName),
			Units:    "metrics",
			Fam:      "generic",
			Ctx:      "websphere_pmi.generic_metrics",
			Type:     module.Line,
			Priority: prioGenericStatMetrics,
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
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// Extension registry metrics (based on actual available metrics)
		{MetricName: "RequestCount", CollectionKey: "extension_registry_" + cleanInst + "_requests", StatType: "count"},
		{MetricName: "HitCount", CollectionKey: "extension_registry_" + cleanInst + "_hits", StatType: "count"},
		{MetricName: "DisplacementCount", CollectionKey: "extension_registry_" + cleanInst + "_displacements", StatType: "count"},
		{MetricName: "HitRate", CollectionKey: "extension_registry_" + cleanInst + "_hit_rate", StatType: "double"},
	}
	
	w.collectMetricsWithLogging("ExtensionRegistry", stat, instance, mx, mappings)
}

// parseSIBJMSAdapter handles Service Integration Bus JMS Resource Adapter metrics
func (w *WebSpherePMI) parseSIBJMSAdapter(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSIBJMSAdapterCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("sib_jms_adapter", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// SIB JMS Resource Adapter metrics (actually JCA connection pool metrics)
		{MetricName: "CreateCount", CollectionKey: "sib_jms_" + cleanInst + "_create_count", StatType: "count"},
		{MetricName: "CloseCount", CollectionKey: "sib_jms_" + cleanInst + "_close_count", StatType: "count"},
		{MetricName: "AllocateCount", CollectionKey: "sib_jms_" + cleanInst + "_allocate_count", StatType: "count"},
		{MetricName: "FreedCount", CollectionKey: "sib_jms_" + cleanInst + "_freed_count", StatType: "count"},
		{MetricName: "FaultCount", CollectionKey: "sib_jms_" + cleanInst + "_fault_count", StatType: "count"},
		{MetricName: "ManagedConnectionCount", CollectionKey: "sib_jms_" + cleanInst + "_managed_connections", StatType: "count"},
		{MetricName: "ConnectionHandleCount", CollectionKey: "sib_jms_" + cleanInst + "_connection_handles", StatType: "count"},
	}
	
	w.collectMetricsWithLogging("SIBJMSAdapter", stat, instance, mx, mappings)
}

// parseServletsComponent handles standalone Servlets component metrics
func (w *WebSpherePMI) parseServletsComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureServletsComponentCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("servlets_component", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// Servlets component metrics (based on actual available metrics)
		{MetricName: "RequestCount", CollectionKey: "servlets_component_" + cleanInst + "_requests", StatType: "count"},
		{MetricName: "ErrorCount", CollectionKey: "servlets_component_" + cleanInst + "_errors", StatType: "count"},
		{MetricName: "ServiceTime", CollectionKey: "servlets_component_" + cleanInst + "_service_time", StatType: "time"},
		{MetricName: "AsyncContext Response Time", CollectionKey: "servlets_component_" + cleanInst + "_async_response_time", StatType: "time"},
		{MetricName: "ConcurrentRequests", CollectionKey: "servlets_component_" + cleanInst + "_concurrent_requests", StatType: "range"},
	}
	
	w.collectMetricsWithLogging("ServletsComponent", stat, instance, mx, mappings)
}

// parseWIMComponent handles WebSphere Identity Manager component metrics
func (w *WebSpherePMI) parseWIMComponent(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWIMComponentCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("wim_component", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// WIM component metrics (actually portlet metrics)
		{MetricName: "Number of portlet requests", CollectionKey: "wim_" + cleanInst + "_portlet_requests", StatType: "count"},
		{MetricName: "Number of portlet errors", CollectionKey: "wim_" + cleanInst + "_portlet_errors", StatType: "count"},
		{MetricName: "Response time of portlet render", CollectionKey: "wim_" + cleanInst + "_render_time", StatType: "time"},
		{MetricName: "Response time of portlet action", CollectionKey: "wim_" + cleanInst + "_action_time", StatType: "time"},
		{MetricName: "Response time of a portlet processEvent request", CollectionKey: "wim_" + cleanInst + "_process_event_time", StatType: "time"},
		{MetricName: "Response time of a portlet serveResource request", CollectionKey: "wim_" + cleanInst + "_serve_resource_time", StatType: "time"},
	}
	
	w.collectMetricsWithLogging("WIMComponent", stat, instance, mx, mappings)
}

// parseWLMTaggedComponentManager handles Workload Management Tagged Component Manager metrics
func (w *WebSpherePMI) parseWLMTaggedComponentManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureWLMTaggedComponentManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("wlm_tagged_component", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// WLM Tagged Component Manager metrics (based on actual available metrics)
		{MetricName: "ProcessingTime", CollectionKey: "wlm_tagged_" + cleanInst + "_processing_time", StatType: "time"},
	}
	
	w.collectMetricsWithLogging("WLMTaggedComponentManager", stat, instance, mx, mappings)
}

// parsePMIWebServiceService handles PMI Web Service Service metrics
func (w *WebSpherePMI) parsePMIWebServiceService(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensurePMIWebServiceServiceCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("pmi_webservice_service", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// PMI Web Service Service metrics (based on actual available metrics)
		{MetricName: "RequestReceivedService", CollectionKey: "pmi_webservice_" + cleanInst + "_requests_received", StatType: "count"},
		{MetricName: "RequestDispatchedService", CollectionKey: "pmi_webservice_" + cleanInst + "_requests_dispatched", StatType: "count"},
		{MetricName: "RequestSuccessfulService", CollectionKey: "pmi_webservice_" + cleanInst + "_requests_successful", StatType: "count"},
		{MetricName: "ResponseTimeService", CollectionKey: "pmi_webservice_" + cleanInst + "_response_time", StatType: "time"},
		{MetricName: "RequestResponseService", CollectionKey: "pmi_webservice_" + cleanInst + "_request_response_time", StatType: "time"},
	}
	
	w.collectMetricsWithLogging("PMIWebServiceService", stat, instance, mx, mappings)
}

// parseTCPChannelDCS handles TCP Channel Data Communication Services metrics
func (w *WebSpherePMI) parseTCPChannelDCS(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureTCPChannelDCSCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("tcp_channel_dcs", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// TCP Channel DCS metrics (based on actual available metrics)
		{MetricName: "CreateCount", CollectionKey: "tcp_dcs_" + cleanInst + "_create_count", StatType: "count"},
		{MetricName: "DestroyCount", CollectionKey: "tcp_dcs_" + cleanInst + "_destroy_count", StatType: "count"},
		{MetricName: "DeclaredThreadHungCount", CollectionKey: "tcp_dcs_" + cleanInst + "_thread_hung_count", StatType: "count"},
		{MetricName: "ClearedThreadHangCount", CollectionKey: "tcp_dcs_" + cleanInst + "_thread_hang_cleared", StatType: "count"},
		{MetricName: "ActiveTime", CollectionKey: "tcp_dcs_" + cleanInst + "_active_time", StatType: "time"},
		{MetricName: "ConcurrentHungThreadCount", CollectionKey: "tcp_dcs_" + cleanInst + "_concurrent_hung_threads", StatType: "range"},
		{MetricName: "ActiveCount", CollectionKey: "tcp_dcs_" + cleanInst + "_active_count", StatType: "bounded_range"},
		{MetricName: "PoolSize", CollectionKey: "tcp_dcs_" + cleanInst + "_pool_size", StatType: "bounded_range"},
		{MetricName: "PercentMaxed", CollectionKey: "tcp_dcs_" + cleanInst + "_percent_maxed", StatType: "bounded_range"},
		{MetricName: "PercentUsed", CollectionKey: "tcp_dcs_" + cleanInst + "_percent_used", StatType: "bounded_range"},
	}
	
	w.collectMetricsWithLogging("TCPChannelDCS", stat, instance, mx, mappings)
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
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// Details component metrics (actually portlet metrics)
		{MetricName: "Number of loaded portlets", CollectionKey: "details_" + cleanInst + "_loaded_portlets", StatType: "count"},
	}
	
	w.collectMetricsWithLogging("DetailsComponent", stat, instance, mx, mappings)
}

// parseISCProductDetails handles IBM Support Center Product Details metrics
func (w *WebSpherePMI) parseISCProductDetails(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureISCProductDetailsCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("isc_product_details", instance)
	
	cleanInst := w.cleanID(instance)
	mappings := []MetricMapping{
		// ISC Product Details metrics (actually portlet metrics)
		{MetricName: "Number of portlet requests", CollectionKey: "isc_product_" + cleanInst + "_portlet_requests", StatType: "count"},
		{MetricName: "Number of portlet errors", CollectionKey: "isc_product_" + cleanInst + "_portlet_errors", StatType: "count"},
		{MetricName: "Response time of portlet render", CollectionKey: "isc_product_" + cleanInst + "_render_time", StatType: "time"},
		{MetricName: "Response time of portlet action", CollectionKey: "isc_product_" + cleanInst + "_action_time", StatType: "time"},
		{MetricName: "Response time of a portlet processEvent request", CollectionKey: "isc_product_" + cleanInst + "_process_event_time", StatType: "time"},
		{MetricName: "Response time of a portlet serveResource request", CollectionKey: "isc_product_" + cleanInst + "_serve_resource_time", StatType: "time"},
	}
	
	w.collectMetricsWithLogging("ISCProductDetails", stat, instance, mx, mappings)
}

// ensureExtensionRegistryCharts creates extension registry charts
func (w *WebSpherePMI) ensureExtensionRegistryCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("extension_registry_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := extensionRegistryChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
func (w *WebSpherePMI) ensureTCPChannelDCSCharts(instance, nodeName, serverName string) {
	chartKey := fmt.Sprintf("tcp_channel_dcs_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := tcpChannelDCSChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "context", Value: parentContext},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}

// Priority constants for new charts (non-duplicated ones)
const (
	prioWebAppContainerServlets  = 2350
	prioWebAppContainerRequests  = 2360
	prioWebAppServlets          = 2400
	prioWebAppReloads           = 2410
	prioServletRequests         = 2500
	prioServletResponseTime     = 2510
	prioServletServiceCount     = 2520
	prioSessionsActive          = 2600
	prioSessionsLifecycle       = 2610
	prioORBRequests             = 2700
	prioORBConcurrent           = 2710
	prioORBLookupTime           = 2720
	prioCacheEntries            = 2800
	prioCacheHits               = 2810
	prioCacheObjectEntries      = 2900
	prioJCAPoolConnections      = 3200
	prioJCAPoolLifecycle        = 3210
	prioEnterpriseAppMetrics    = 3300
	prioSystemDataMetrics       = 3400
	prioWLMMetrics             = 3500
	prioBeanManagerMetrics      = 3600
	prioConnectionManagerMetrics = 3700
	prioJVMSubsystemMetrics     = 3800
	prioEJBContainerMetrics     = 3900
	prioMDBMetrics             = 4000
	prioSFSBMetrics            = 4100
	prioSLSBMetrics            = 4200
	prioEntityBeanMetrics      = 4300
	prioGenericEJBMetrics      = 4400
	prioSecurityAuthFailures   = 5010
	prioGenericStatMetrics     = 9000  // Low priority for catch-all
)

// Chart creation helpers for new parsers
func (w *WebSpherePMI) ensureJCAPoolCharts(instance, nodeName, serverName, poolName string) {
	chartKey := fmt.Sprintf("jca_pool_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := jcaPoolChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "pool", Value: poolName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "application", Value: appName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "subsystem", Value: subsystem},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "bean", Value: beanName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "bean", Value: beanName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "bean", Value: beanName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "bean", Value: beanName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "bean", Value: beanName},
			}
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
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"WebAuthenticationCount",
		"TAIRequestCount",
		"IdentityAssertionCount",
		"BasicAuthenticationCount",
		"TokenAuthenticationCount",
		"WebAuthenticationTime",
		"TAIRequestTime",
		"IdentityAssertionTime",
		"BasicAuthenticationTime",
		"TokenAuthenticationTime",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics (authentication events)
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			// Traditional WebSphere detailed metrics
			case "WebAuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_web_auth", w.cleanID(instance))] = val
				foundMetrics["WebAuthenticationCount"] = true
			case "TAIRequestCount":
				mx[fmt.Sprintf("security_auth_%s_tai_requests", w.cleanID(instance))] = val
				foundMetrics["TAIRequestCount"] = true
			case "IdentityAssertionCount":
				mx[fmt.Sprintf("security_auth_%s_identity_assertions", w.cleanID(instance))] = val
				foundMetrics["IdentityAssertionCount"] = true
			case "BasicAuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_basic_auth", w.cleanID(instance))] = val
				foundMetrics["BasicAuthenticationCount"] = true
			case "TokenAuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_token_auth", w.cleanID(instance))] = val
				foundMetrics["TokenAuthenticationCount"] = true
			case "ClientCertAuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_cert_auth", w.cleanID(instance))] = val
			case "LTPATokenAuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_ltpa_auth", w.cleanID(instance))] = val
			case "CustomAuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_custom_auth", w.cleanID(instance))] = val
			case "AuthenticationFailureCount":
				mx[fmt.Sprintf("security_auth_%s_failures", w.cleanID(instance))] = val
			// Liberty/generic metrics
			case "AuthenticationCount":
				mx[fmt.Sprintf("security_auth_%s_total_auth", w.cleanID(instance))] = val
			}
		} else {
			w.Errorf("ISSUE: parseSecurityAuthentication failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process TimeStatistics (authentication timing)
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		// Traditional WebSphere detailed metrics
		case "WebAuthenticationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_auth_%s_web_auth_time_total", w.cleanID(instance))] = val
				foundMetrics["WebAuthenticationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthentication failed to parse TimeStatistic WebAuthenticationTime total value '%s': %v", total, err)
			}
		case "TAIRequestTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_auth_%s_tai_time_total", w.cleanID(instance))] = val
				foundMetrics["TAIRequestTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthentication failed to parse TimeStatistic TAIRequestTime total value '%s': %v", total, err)
			}
		case "IdentityAssertionTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_auth_%s_identity_time_total", w.cleanID(instance))] = val
				foundMetrics["IdentityAssertionTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthentication failed to parse TimeStatistic IdentityAssertionTime total value '%s': %v", total, err)
			}
		case "BasicAuthenticationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_auth_%s_basic_auth_time_total", w.cleanID(instance))] = val
				foundMetrics["BasicAuthenticationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthentication failed to parse TimeStatistic BasicAuthenticationTime total value '%s': %v", total, err)
			}
		case "TokenAuthenticationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_auth_%s_token_auth_time_total", w.cleanID(instance))] = val
				foundMetrics["TokenAuthenticationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthentication failed to parse TimeStatistic TokenAuthenticationTime total value '%s': %v", total, err)
			}
		// Liberty/generic metrics
		case "AuthenticationResponseTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_auth_%s_response_time_total", w.cleanID(instance))] = val
			} else {
				w.Errorf("ISSUE: parseSecurityAuthentication failed to parse TimeStatistic AuthenticationResponseTime total value '%s': %v", total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("SecurityAuthentication", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseSecurityAuthorization handles Security Authorization metrics
func (w *WebSpherePMI) parseSecurityAuthorization(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureSecurityAuthzCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("security_authz", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"WebAuthorizationTime",
		"EJBAuthorizationTime",
	}
	foundMetrics := make(map[string]bool)
	
	// Process TimeStatistics (authorization timing)
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "WebAuthorizationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_web_authz_time_total", w.cleanID(instance))] = val
				foundMetrics["WebAuthorizationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic WebAuthorizationTime total value '%s': %v", total, err)
			}
			if count, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_web_authz_count", w.cleanID(instance))] = count
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic WebAuthorizationTime count value '%s': %v", ts.Count, err)
			}
		case "EJBAuthorizationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_ejb_authz_time_total", w.cleanID(instance))] = val
				foundMetrics["EJBAuthorizationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic EJBAuthorizationTime total value '%s': %v", total, err)
			}
			if count, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_ejb_authz_count", w.cleanID(instance))] = count
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic EJBAuthorizationTime count value '%s': %v", ts.Count, err)
			}
		case "AdminAuthorizationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_admin_authz_time_total", w.cleanID(instance))] = val
				foundMetrics["AdminAuthorizationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic AdminAuthorizationTime total value '%s': %v", total, err)
			}
			if count, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_admin_authz_count", w.cleanID(instance))] = count
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic AdminAuthorizationTime count value '%s': %v", ts.Count, err)
			}
		case "JACCAuthorizationTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_jacc_authz_time_total", w.cleanID(instance))] = val
				foundMetrics["JACCAuthorizationTime"] = true
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic JACCAuthorizationTime total value '%s': %v", total, err)
			}
			if count, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("security_authz_%s_jacc_authz_count", w.cleanID(instance))] = count
			} else {
				w.Errorf("ISSUE: parseSecurityAuthorization failed to parse TimeStatistic JACCAuthorizationTime count value '%s': %v", ts.Count, err)
			}
		}
	}
	
	// Add the missing metrics to expected list if found
	if _, ok := foundMetrics["AdminAuthorizationTime"]; ok || len(stat.TimeStatistics) > 2 {
		expectedMetrics = append(expectedMetrics, "AdminAuthorizationTime")
	}
	if _, ok := foundMetrics["JACCAuthorizationTime"]; ok || len(stat.TimeStatistics) > 3 {
		expectedMetrics = append(expectedMetrics, "JACCAuthorizationTime")
	}
	
	// Log any missing metrics
	w.logParsingFailure("SecurityAuthorization", stat.Name, stat, expectedMetrics, foundMetrics)
}

// ===== SECURITY CHART TEMPLATES =====

var securityAuthChartsTmpl = module.Charts{
	{
		ID:       "security_auth_%s_events",
		Title:    "Security Authentication Events",
		Units:    "events/s",
		Fam:      "security",
		Ctx:      "websphere_pmi.security_auth_events",
		Type:     module.Line,
		Priority: prioSecurityAuthEvents,
		Dims: module.Dims{
			// Traditional WebSphere metrics (based on actual XML)
			{ID: "security_auth_%s_web_auth", Name: "web", Algo: module.Incremental},
			{ID: "security_auth_%s_basic_auth", Name: "basic", Algo: module.Incremental},
			{ID: "security_auth_%s_token_auth", Name: "token", Algo: module.Incremental},
			{ID: "security_auth_%s_tai_requests", Name: "tai", Algo: module.Incremental},
			{ID: "security_auth_%s_identity_assertions", Name: "identity", Algo: module.Incremental},
		},
	},
	{
		ID:       "security_auth_%s_timing",
		Title:    "Security Authentication Response Time",
		Units:    "milliseconds/s",
		Fam:      "security",
		Ctx:      "websphere_pmi.security_auth_timing",
		Type:     module.Line,
		Priority: prioSecurityAuthTiming,
		Dims: module.Dims{
			// Traditional WebSphere timing metrics (based on actual XML)
			{ID: "security_auth_%s_web_auth_time_total", Name: "web", Algo: module.Incremental},
			{ID: "security_auth_%s_basic_auth_time_total", Name: "basic", Algo: module.Incremental},
			{ID: "security_auth_%s_token_auth_time_total", Name: "token", Algo: module.Incremental},
			{ID: "security_auth_%s_tai_time_total", Name: "tai", Algo: module.Incremental},
			{ID: "security_auth_%s_identity_time_total", Name: "identity", Algo: module.Incremental},
		},
	},
}

var securityAuthzChartsTmpl = module.Charts{
	{
		ID:       "security_authz_%s_events",
		Title:    "Security Authorization Events",
		Units:    "events/s",
		Fam:      "security",
		Ctx:      "websphere_pmi.security_authz_events",
		Type:     module.Line,
		Priority: prioSecurityAuthzEvents,
		Dims: module.Dims{
			{ID: "security_authz_%s_web_authz_count", Name: "web", Algo: module.Incremental},
			{ID: "security_authz_%s_ejb_authz_count", Name: "ejb", Algo: module.Incremental},
			{ID: "security_authz_%s_admin_authz_count", Name: "admin", Algo: module.Incremental},
			{ID: "security_authz_%s_jacc_authz_count", Name: "jacc", Algo: module.Incremental},
		},
	},
	{
		ID:       "security_authz_%s_timing",
		Title:    "Security Authorization Response Time",
		Units:    "milliseconds/s",
		Fam:      "security",
		Ctx:      "websphere_pmi.security_authz_timing",
		Type:     module.Line,
		Priority: prioSecurityAuthzTiming,
		Dims: module.Dims{
			{ID: "security_authz_%s_web_authz_time_total", Name: "web", Algo: module.Incremental},
			{ID: "security_authz_%s_ejb_authz_time_total", Name: "ejb", Algo: module.Incremental},
			{ID: "security_authz_%s_admin_authz_time_total", Name: "admin", Algo: module.Incremental},
			{ID: "security_authz_%s_jacc_authz_time_total", Name: "jacc", Algo: module.Incremental},
		},
	},
}

// Chart templates for new parsers
var interceptorChartsTmpl = module.Charts{
	{
		ID:       "interceptor_%s_processing_time",
		Title:    "Interceptor Processing Time",
		Units:    "milliseconds/s",
		Fam:      "interceptors",
		Ctx:      "websphere_pmi.interceptor_processing_time",
		Type:     module.Line,
		Priority: prioORBInterceptors,
		Dims: module.Dims{
			{ID: "interceptor_%s_processing_time_total", Name: "processing_time", Algo: module.Incremental},
		},
	},
	{
		ID:       "interceptor_%s_processing_count",
		Title:    "Interceptor Processing Count",
		Units:    "operations/s",
		Fam:      "interceptors",
		Ctx:      "websphere_pmi.interceptor_processing_count",
		Type:     module.Line,
		Priority: prioORBInterceptors + 1,
		Dims: module.Dims{
			{ID: "interceptor_%s_processing_count", Name: "operations", Algo: module.Incremental},
		},
	},
}

var portletChartsTmpl = module.Charts{
	{
		ID:       "portlet_%s_requests",
		Title:    "Portlet Requests",
		Units:    "requests/s",
		Fam:      "portlets",
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
		Fam:      "portlets",
		Ctx:      "websphere_pmi.portlet_concurrent",
		Type:     module.Line,
		Priority: prioWebPortlets + 1,
		Dims: module.Dims{
			{ID: "portlet_%s_concurrent", Name: "concurrent"},
		},
	},
	{
		ID:       "portlet_%s_response_time",
		Title:    "Portlet Response Time",
		Units:    "milliseconds/s",
		Fam:      "portlets",
		Ctx:      "websphere_pmi.portlet_response_time",
		Type:     module.Line,
		Priority: prioWebPortlets + 2,
		Dims: module.Dims{
			{ID: "portlet_%s_render_time_total", Name: "render", Algo: module.Incremental},
			{ID: "portlet_%s_action_time_total", Name: "action", Algo: module.Incremental},
			{ID: "portlet_%s_event_time_total", Name: "event", Algo: module.Incremental},
			{ID: "portlet_%s_resource_time_total", Name: "resource", Algo: module.Incremental},
		},
	},
}

var webServiceChartsTmpl = module.Charts{
	{
		ID:       "web_service_%s_services",
		Title:    "Web Service Module Services",
		Units:    "services",
		Fam:      "web_services",
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
		Fam:      "urls",
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
		Fam:      "urls",
		Ctx:      "websphere_pmi.url_concurrent",
		Type:     module.Line,
		Priority: prioWebServlets + 11,
		Dims: module.Dims{
			{ID: "urls_%s_concurrent", Name: "concurrent"},
		},
	},
	{
		ID:       "urls_%s_response_time",
		Title:    "URL Response Time",
		Units:    "milliseconds/s",
		Fam:      "urls",
		Ctx:      "websphere_pmi.url_response_time",
		Type:     module.Line,
		Priority: prioWebServlets + 12,
		Dims: module.Dims{
			{ID: "urls_%s_service_time_total", Name: "service", Algo: module.Incremental},
			{ID: "urls_%s_async_time_total", Name: "async", Algo: module.Incremental},
		},
	},
}

var servletURLChartsTmpl = module.Charts{
	{
		ID:       "servlet_url_%s_requests",
		Title:    "Servlet URL Requests",
		Units:    "requests/s",
		Fam:      "servlet_urls",
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
		Fam:      "servlet_urls",
		Ctx:      "websphere_pmi.servlet_url_concurrent",
		Type:     module.Line,
		Priority: prioWebServlets + 21,
		Dims: module.Dims{
			{ID: "servlet_url_%s_concurrent", Name: "concurrent"},
		},
	},
	{
		ID:       "servlet_url_%s_response_time",
		Title:    "Servlet URL Response Time",
		Units:    "milliseconds/s",
		Fam:      "servlet_urls",
		Ctx:      "websphere_pmi.servlet_url_response_time",
		Type:     module.Line,
		Priority: prioWebServlets + 22,
		Dims: module.Dims{
			{ID: "servlet_url_%s_service_time_total", Name: "service", Algo: module.Incremental},
			{ID: "servlet_url_%s_async_time_total", Name: "async", Algo: module.Incremental},
		},
	},
}

var haManagerChartsTmpl = module.Charts{
	{
		ID:       "ha_manager_%s_groups",
		Title:    "HA Manager Groups",
		Units:    "groups",
		Fam:      "ha_manager",
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
		Fam:      "ha_manager",
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
	{
		ID:       "ha_manager_%s_rebuild_time",
		Title:    "HA Manager Rebuild Time",
		Units:    "milliseconds/s",
		Fam:      "ha_manager",
		Ctx:      "websphere_pmi.ha_manager_rebuild_time",
		Type:     module.Line,
		Priority: prioHAManager + 2,
		Dims: module.Dims{
			{ID: "ha_manager_%s_group_rebuild_time_total", Name: "group_rebuild", Algo: module.Incremental},
			{ID: "ha_manager_%s_bulletin_rebuild_time_total", Name: "bulletin_rebuild", Algo: module.Incremental},
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
	
	// Track expected vs found metrics
	expectedMetrics := []string{"ProcessingTime"}
	foundMetrics := make(map[string]bool)
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "ProcessingTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("interceptor_%s_processing_time_total", w.cleanID(instance))] = val
				foundMetrics["ProcessingTime"] = true
			} else {
				w.Errorf("ISSUE: parseInterceptor failed to parse TimeStatistic ProcessingTime total value '%s': %v", total, err)
			}
			if count, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("interceptor_%s_processing_count", w.cleanID(instance))] = count
			} else {
				w.Errorf("ISSUE: parseInterceptor failed to parse TimeStatistic ProcessingTime count value '%s': %v", ts.Count, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("Interceptor", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parsePortlet handles individual portlet metrics  
func (w *WebSpherePMI) parsePortlet(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	portletName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, portletName)
	
	// Create charts if not exists
	w.ensurePortletCharts(instance, nodeName, serverName, portletName)
	
	// Mark as seen
	w.markInstanceSeen("portlet", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"Number of portlet requests",
		"Number of portlet errors",
		"Number of concurrent portlet requests",
		"Response time of portlet render",
		"Response time of portlet action",
		"Response time of a portlet processEvent request",
		"Response time of a portlet serveResource request",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "Number of portlet requests":
				mx[fmt.Sprintf("portlet_%s_requests", w.cleanID(instance))] = val
				foundMetrics["Number of portlet requests"] = true
			case "Number of portlet errors":
				mx[fmt.Sprintf("portlet_%s_errors", w.cleanID(instance))] = val
				foundMetrics["Number of portlet errors"] = true
			}
		} else {
			w.Errorf("ISSUE: parsePortlet failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "Number of concurrent portlet requests":
				mx[fmt.Sprintf("portlet_%s_concurrent", w.cleanID(instance))] = val
				foundMetrics["Number of concurrent portlet requests"] = true
			}
		} else {
			w.Errorf("ISSUE: parsePortlet failed to parse RangeStatistic %s value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "Response time of portlet render":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("portlet_%s_render_time_total", w.cleanID(instance))] = val
				foundMetrics["Response time of portlet render"] = true
			} else {
				w.Errorf("ISSUE: parsePortlet failed to parse TimeStatistic 'Response time of portlet render' total value '%s': %v", total, err)
			}
		case "Response time of portlet action":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("portlet_%s_action_time_total", w.cleanID(instance))] = val
				foundMetrics["Response time of portlet action"] = true
			} else {
				w.Errorf("ISSUE: parsePortlet failed to parse TimeStatistic 'Response time of portlet action' total value '%s': %v", total, err)
			}
		case "Response time of a portlet processEvent request":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("portlet_%s_event_time_total", w.cleanID(instance))] = val
				foundMetrics["Response time of a portlet processEvent request"] = true
			} else {
				w.Errorf("ISSUE: parsePortlet failed to parse TimeStatistic 'Response time of a portlet processEvent request' total value '%s': %v", total, err)
			}
		case "Response time of a portlet serveResource request":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("portlet_%s_resource_time_total", w.cleanID(instance))] = val
				foundMetrics["Response time of a portlet serveResource request"] = true
			} else {
				w.Errorf("ISSUE: parsePortlet failed to parse TimeStatistic 'Response time of a portlet serveResource request' total value '%s': %v", total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("Portlet", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseWebServiceModule handles web service module metrics
func (w *WebSpherePMI) parseWebServiceModule(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	moduleName := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, moduleName)
	
	// Create charts if not exists
	w.ensureWebServiceCharts(instance, nodeName, serverName, moduleName)
	
	// Mark as seen
	w.markInstanceSeen("web_service", instance)
	
	// Track expected vs found metrics  
	// Note: Web service module .war files only contain ServicesLoaded metric
	// The other metrics (RequestReceivedService, ResponseTimeService, etc.) are for actual web service operations
	expectedMetrics := []string{
		"ServicesLoaded",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "ServicesLoaded":
				mx[fmt.Sprintf("web_service_%s_services_loaded", w.cleanID(instance))] = val
				foundMetrics["ServicesLoaded"] = true
			}
		} else {
			w.Errorf("ISSUE: parseWebServiceModule failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("WebServiceModule", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseURLContainer handles URLs container metrics
func (w *WebSpherePMI) parseURLContainer(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureURLCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("urls", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"URIRequestCount",
		"URIConcurrentRequests",
		"URIServiceTime",
		"URL AsyncContext Response Time",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "URIRequestCount":
				mx[fmt.Sprintf("urls_%s_requests", w.cleanID(instance))] = val
				foundMetrics["URIRequestCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseURLContainer failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "URIConcurrentRequests":
				mx[fmt.Sprintf("urls_%s_concurrent", w.cleanID(instance))] = val
				foundMetrics["URIConcurrentRequests"] = true
			}
		} else {
			w.Errorf("ISSUE: parseURLContainer failed to parse RangeStatistic %s value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "URIServiceTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("urls_%s_service_time_total", w.cleanID(instance))] = val
				foundMetrics["URIServiceTime"] = true
			} else {
				w.Errorf("ISSUE: parseURLContainer failed to parse TimeStatistic URIServiceTime total value '%s': %v", total, err)
			}
		case "URL AsyncContext Response Time":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("urls_%s_async_time_total", w.cleanID(instance))] = val
				foundMetrics["URL AsyncContext Response Time"] = true
			} else {
				w.Errorf("ISSUE: parseURLContainer failed to parse TimeStatistic 'URL AsyncContext Response Time' total value '%s': %v", total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("URLContainer", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseServletURL handles individual servlet URL metrics
func (w *WebSpherePMI) parseServletURL(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	urlPath := stat.Name
	instance := fmt.Sprintf("%s.%s.%s", nodeName, serverName, urlPath)
	
	// Create charts if not exists
	w.ensureServletURLCharts(instance, nodeName, serverName, urlPath)
	
	// Mark as seen
	w.markInstanceSeen("servlet_url", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"URIRequestCount",
		"URIConcurrentRequests",
		"URIServiceTime",
		"URL AsyncContext Response Time",
	}
	foundMetrics := make(map[string]bool)
	
	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			switch cs.Name {
			case "URIRequestCount":
				mx[fmt.Sprintf("servlet_url_%s_requests", w.cleanID(instance))] = val
				foundMetrics["URIRequestCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseServletURL failed to parse CountStatistic %s value '%s': %v", cs.Name, cs.Count, err)
		}
	}
	
	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "URIConcurrentRequests":
				mx[fmt.Sprintf("servlet_url_%s_concurrent", w.cleanID(instance))] = val
				foundMetrics["URIConcurrentRequests"] = true
			}
		} else {
			w.Errorf("ISSUE: parseServletURL failed to parse RangeStatistic %s value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "URIServiceTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_url_%s_service_time_total", w.cleanID(instance))] = val
				foundMetrics["URIServiceTime"] = true
			} else {
				w.Errorf("ISSUE: parseServletURL failed to parse TimeStatistic URIServiceTime total value '%s': %v", total, err)
			}
		case "URL AsyncContext Response Time":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_url_%s_async_time_total", w.cleanID(instance))] = val
				foundMetrics["URL AsyncContext Response Time"] = true
			} else {
				w.Errorf("ISSUE: parseServletURL failed to parse TimeStatistic 'URL AsyncContext Response Time' total value '%s': %v", total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("ServletURL", stat.Name, stat, expectedMetrics, foundMetrics)
}

// parseHAManager handles HA Manager metrics
func (w *WebSpherePMI) parseHAManager(stat *pmiStat, nodeName, serverName string, mx map[string]int64) {
	instance := fmt.Sprintf("%s.%s", nodeName, serverName)
	
	// Create charts if not exists
	w.ensureHAManagerCharts(instance, nodeName, serverName)
	
	// Mark as seen
	w.markInstanceSeen("ha_manager", instance)
	
	// Track expected vs found metrics
	expectedMetrics := []string{
		"LocalGroupCount",
		"BulletinBoardSubjectCount", 
		"BulletinBoardSubcriptionCount",
		"LocalBulletinBoardSubjectCount",
		"LocalBulletinBoardSubcriptionCount",
		"GroupStateRebuildTime",
		"BulletinBoardRebuildTime",
	}
	foundMetrics := make(map[string]bool)
	
	// Process RangeStatistics (if any)
	for _, rs := range stat.RangeStatistics {
		if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			switch rs.Name {
			case "LocalGroupCount":
				mx[fmt.Sprintf("ha_manager_%s_local_groups", w.cleanID(instance))] = val
				foundMetrics["LocalGroupCount"] = true
			case "BulletinBoardSubjectCount":
				mx[fmt.Sprintf("ha_manager_%s_bulletin_subjects", w.cleanID(instance))] = val
				foundMetrics["BulletinBoardSubjectCount"] = true
			case "BulletinBoardSubcriptionCount":
				mx[fmt.Sprintf("ha_manager_%s_bulletin_subscriptions", w.cleanID(instance))] = val
				foundMetrics["BulletinBoardSubcriptionCount"] = true
			case "LocalBulletinBoardSubjectCount":
				mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subjects", w.cleanID(instance))] = val
				foundMetrics["LocalBulletinBoardSubjectCount"] = true
			case "LocalBulletinBoardSubcriptionCount":
				mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subscriptions", w.cleanID(instance))] = val
				foundMetrics["LocalBulletinBoardSubcriptionCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseHAManager failed to parse RangeStatistic %s value '%s': %v", rs.Name, rs.Current, err)
		}
	}
	
	// HAManager uses BoundedRangeStatistics! Check Current field (maps to value attribute in XML)
	for _, brs := range stat.BoundedRangeStatistics {
		if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
			switch brs.Name {
			case "LocalGroupCount":
				mx[fmt.Sprintf("ha_manager_%s_local_groups", w.cleanID(instance))] = val
				foundMetrics["LocalGroupCount"] = true
			case "BulletinBoardSubjectCount":
				mx[fmt.Sprintf("ha_manager_%s_bulletin_subjects", w.cleanID(instance))] = val
				foundMetrics["BulletinBoardSubjectCount"] = true
			case "BulletinBoardSubcriptionCount":
				mx[fmt.Sprintf("ha_manager_%s_bulletin_subscriptions", w.cleanID(instance))] = val
				foundMetrics["BulletinBoardSubcriptionCount"] = true
			case "LocalBulletinBoardSubjectCount":
				mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subjects", w.cleanID(instance))] = val
				foundMetrics["LocalBulletinBoardSubjectCount"] = true
			case "LocalBulletinBoardSubcriptionCount":
				mx[fmt.Sprintf("ha_manager_%s_local_bulletin_subscriptions", w.cleanID(instance))] = val
				foundMetrics["LocalBulletinBoardSubcriptionCount"] = true
			}
		} else {
			w.Errorf("ISSUE: parseHAManager failed to parse BoundedRangeStatistic %s value '%s': %v", brs.Name, brs.Current, err)
		}
	}
	
	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		switch ts.Name {
		case "GroupStateRebuildTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("ha_manager_%s_group_rebuild_time_total", w.cleanID(instance))] = val
				foundMetrics["GroupStateRebuildTime"] = true
			} else {
				w.Errorf("ISSUE: parseHAManager failed to parse TimeStatistic %s total value '%s': %v", ts.Name, total, err)
			}
		case "BulletinBoardRebuildTime":
			total := ts.TotalTime
			if total == "" {
				total = ts.Total
			}
			if val, err := strconv.ParseInt(total, 10, 64); err == nil {
				mx[fmt.Sprintf("ha_manager_%s_bulletin_rebuild_time_total", w.cleanID(instance))] = val
				foundMetrics["BulletinBoardRebuildTime"] = true
			} else {
				w.Errorf("ISSUE: parseHAManager failed to parse TimeStatistic %s total value '%s': %v", ts.Name, total, err)
			}
		}
	}
	
	// Log any missing metrics
	w.logParsingFailure("HAManager", stat.Name, stat, expectedMetrics, foundMetrics)
}

// Chart creation functions for new parsers

func (w *WebSpherePMI) ensureInterceptorCharts(instance, nodeName, serverName, interceptorName string) {
	chartKey := fmt.Sprintf("interceptor_%s", instance)
	if _, exists := w.collectedInstances[chartKey]; !exists {
		w.collectedInstances[chartKey] = true
		
		charts := interceptorChartsTmpl.Copy()
		for _, chart := range *charts {
			chart.ID = fmt.Sprintf(chart.ID, w.cleanID(instance))
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "interceptor", Value: interceptorName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "portlet", Value: portletName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "module", Value: moduleName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
				{Key: "url", Value: urlPath},
			}
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
			chart.Labels = []module.Label{
				{Key: "node", Value: nodeName},
				{Key: "server", Value: serverName},
			}
			for _, dim := range chart.Dims {
				dim.ID = fmt.Sprintf(dim.ID, w.cleanID(instance))
			}
		}
		
		if err := w.charts.Add(*charts...); err != nil {
			w.Warning(err)
		}
	}
}
