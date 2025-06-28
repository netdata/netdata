// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
	"strings"
)

// processJVMStat processes JVM runtime statistics
func (w *WebSpherePMI) processJVMStat(stat *pmiStat, mx map[string]int64) {
	w.Debugf("processing JVM stat: %s (type: %s)", stat.Name, stat.Type)
	
	switch stat.Name {
	case "HeapSize":
		if stat.BoundedRangeStatistic != nil {
			// HeapSize is in KILOBYTES, convert to bytes
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.Current, 10, 64); err == nil {
				mx["jvm_heap_used"] = v * 1024 // Convert KB to bytes
			} else if vf, err := strconv.ParseFloat(stat.BoundedRangeStatistic.Current, 64); err == nil {
				// Handle scientific notation
				mx["jvm_heap_used"] = int64(vf * 1024)
			}
			
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.UpperBound, 10, 64); err == nil {
				mx["jvm_heap_max"] = v * 1024 // Convert KB to bytes
				mx["jvm_heap_committed"] = v * 1024 // PMI doesn't distinguish committed
			} else if vf, err := strconv.ParseFloat(stat.BoundedRangeStatistic.UpperBound, 64); err == nil {
				// Handle scientific notation
				mx["jvm_heap_max"] = int64(vf * 1024)
				mx["jvm_heap_committed"] = int64(vf * 1024)
			}

			// Calculate free memory (committed - used)
			if used, ok := mx["jvm_heap_used"]; ok {
				if committed, ok := mx["jvm_heap_committed"]; ok {
					mx["jvm_heap_free"] = committed - used
				}
			}

			// Calculate usage percentage
			if used, ok := mx["jvm_heap_used"]; ok {
				if max, ok := mx["jvm_heap_max"]; ok && max > 0 {
					mx["jvm_heap_usage_percent"] = (used * precision * 100) / max
				}
			}
		}

	case "UsedMemory":
		if stat.CountStatistic != nil {
			// UsedMemory is in KILOBYTES, convert to bytes
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_heap_used"] = v * 1024
			}
		}

	case "FreeMemory":
		if stat.CountStatistic != nil {
			// FreeMemory is in KILOBYTES, convert to bytes
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				free := v * 1024
				mx["jvm_heap_free"] = free
				if used, ok := mx["jvm_heap_used"]; ok {
					mx["jvm_heap_max"] = used + free
					mx["jvm_heap_committed"] = used + free
				}
			}
		}

	case "ProcessCpuUsage":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseFloat(stat.CountStatistic.Count, 64); err == nil {
				mx["jvm_process_cpu_percent"] = int64(v * precision)
			}
		}

	case "UpTime":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_uptime"] = v // Already in seconds, no precision needed
			}
		}

	case "LiveThreadCount", "ThreadCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_threads_live"] = v
				// Calculate other threads if we have daemon count
				if daemon, ok := mx["jvm_threads_daemon"]; ok {
					mx["jvm_threads_other"] = v - daemon
				}
			}
		}

	case "DaemonThreadCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_threads_daemon"] = v
				// Calculate other threads if we have live count
				if live, ok := mx["jvm_threads_live"]; ok {
					mx["jvm_threads_other"] = live - v
				}
			}
		}

	case "GCCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_gc_count"] = v
			}
		}

	case "GCTime":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_gc_time"] = v
			}
		}

	case "LoadedClassCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_classes_loaded"] = v
			}
		}

	case "UnloadedClassCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx["jvm_classes_unloaded"] = v
			}
		}
	}
}

// processThreadPoolStat processes thread pool statistics
func (w *WebSpherePMI) processThreadPoolStat(stat *pmiStat, mx map[string]int64) {
	// Extract pool name from path
	poolName := w.extractPoolName(stat.Path, "threadPoolModule")
	if poolName == "" {
		return
	}

	// Check selector
	if w.poolSelector != nil && !w.poolSelector.MatchString(poolName) {
		return
	}

	// Check cardinality limit
	if w.MaxThreadPools > 0 && !w.collectedPools[poolName] && len(w.collectedPools) >= w.MaxThreadPools {
		w.logOnce("max_threadpools", "reached max_threadpools limit (%d), ignoring thread pool: %s", w.MaxThreadPools, poolName)
		return
	}

	poolID := cleanID(poolName)

	// Mark as seen
	w.seenPools[poolName] = true

	// Add charts if this is a new pool
	if !w.collectedPools[poolName] {
		w.Debugf("discovered new thread pool: %s", poolName)
		w.addThreadPoolCharts(poolName)
	}

	switch stat.Name {
	case "PoolSize":
		if stat.BoundedRangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("threadpool_%s_size", poolID)] = v
			}
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.UpperBound, 10, 64); err == nil {
				mx[fmt.Sprintf("threadpool_%s_max_size", poolID)] = v
			}
		}

	case "ActiveCount":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("threadpool_%s_active", poolID)] = v
			}
		}

	case "HungThreadCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("threadpool_%s_hung", poolID)] = v
			}
		}
	}
}

// processJDBCPoolStat processes JDBC connection pool statistics
func (w *WebSpherePMI) processJDBCPoolStat(stat *pmiStat, mx map[string]int64) {
	// Extract datasource name from path
	dsName := w.extractDatasourceName(stat.Path)
	if dsName == "" {
		return
	}

	// Check selector (uses pool selector for consistency)
	if w.poolSelector != nil && !w.poolSelector.MatchString(dsName) {
		return
	}

	// Check cardinality limit
	if w.MaxJDBCPools > 0 && !w.collectedJDBCPools[dsName] && len(w.collectedJDBCPools) >= w.MaxJDBCPools {
		w.logOnce("max_jdbc_pools", "reached max_jdbc_pools limit (%d), ignoring JDBC pool: %s", w.MaxJDBCPools, dsName)
		return
	}

	dsID := cleanID(dsName)

	// Mark as seen
	w.seenJDBCPools[dsName] = true

	// Add charts if this is a new datasource
	if !w.collectedJDBCPools[dsName] {
		w.Debugf("discovered new JDBC pool: %s", dsName)
		w.addJDBCPoolCharts(dsName)
	}

	switch stat.Name {
	case "PoolSize":
		if stat.BoundedRangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_size", dsID)] = v
			}
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.UpperBound, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_max_size", dsID)] = v
			}
		}

	case "AllocateCount", "ActiveCount":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_active", dsID)] = v
			}
		}

	case "FreePoolSize":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_free", dsID)] = v
			}
		}

	case "WaitTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_wait_time_avg", dsID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_wait_time_max", dsID)] = v
			}
		}

	case "UseTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_use_time_avg", dsID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_use_time_max", dsID)] = v
			}
		}

	case "CreateCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_created", dsID)] = v
			}
		}

	case "DestroyCount", "CloseCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_destroyed", dsID)] = v
			}
		}

	case "WaitingThreadCount":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jdbc_%s_waiting_threads", dsID)] = v
			}
		}
	}
}

// processJCAPoolStat processes JCA connection pool statistics
func (w *WebSpherePMI) processJCAPoolStat(stat *pmiStat, mx map[string]int64) {
	// Extract JCA resource name from path
	jcaName := w.extractJCAResourceName(stat.Path)
	if jcaName == "" {
		return
	}

	// Check selector (uses pool selector for consistency)
	if w.poolSelector != nil && !w.poolSelector.MatchString(jcaName) {
		return
	}

	// Check cardinality limit
	if w.MaxJCAPools > 0 && !w.collectedJCAPools[jcaName] && len(w.collectedJCAPools) >= w.MaxJCAPools {
		w.logOnce("max_jca_pools", "reached max_jca_pools limit (%d), ignoring JCA pool: %s", w.MaxJCAPools, jcaName)
		return
	}

	jcaID := cleanID(jcaName)

	// Mark as seen
	w.seenJCAPools[jcaName] = true

	// Add charts if this is a new datasource
	if !w.collectedJCAPools[jcaName] {
		w.Debugf("discovered new JCA pool: %s", jcaName)
		w.addJCAPoolCharts(jcaName)
	}

	switch stat.Name {
	case "PoolSize":
		if stat.BoundedRangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_size", jcaID)] = v
			}
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.UpperBound, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_max_size", jcaID)] = v
			}
		}

	case "AllocateCount", "ActiveCount":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_active", jcaID)] = v
			}
		}

	case "FreePoolSize":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_free", jcaID)] = v
			}
		}

	case "WaitTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_wait_time_avg", jcaID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_wait_time_max", jcaID)] = v
			}
		}

	case "UseTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_use_time_avg", jcaID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_use_time_max", jcaID)] = v
			}
		}

	case "CreateCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_created", jcaID)] = v
			}
		}

	case "DestroyCount", "CloseCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_destroyed", jcaID)] = v
			}
		}

	case "WaitingThreadCount":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("jca_%s_waiting_threads", jcaID)] = v
			}
		}
	}
}

// processWebAppStat processes web application statistics
func (w *WebSpherePMI) processWebAppStat(stat *pmiStat, mx map[string]int64) {
	// Extract application name from path
	appName := w.extractAppName(stat.Path)
	if appName == "" {
		return
	}

	// Check selector
	if w.appSelector != nil && !w.appSelector.MatchString(appName) {
		return
	}

	// Check cardinality limit
	if w.MaxApplications > 0 && !w.collectedApps[appName] && len(w.collectedApps) >= w.MaxApplications {
		return
	}

	appID := cleanID(appName)

	// Mark as seen
	w.seenApps[appName] = true

	// Add charts if this is a new app
	if !w.collectedApps[appName] {
		w.addAppCharts(appName)
	}

	switch stat.Name {
	case "RequestCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("app_%s_requests", appID)] = v
			}
		}

	case "ServiceTime", "ResponseTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("app_%s_response_time_avg", appID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("app_%s_response_time_max", appID)] = v
			}
		}

	case "ActiveSessions":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("app_%s_sessions_active", appID)] = v
			}
		}

	case "LiveSessions":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("app_%s_sessions_live", appID)] = v
			}
		}
	}
}

// processServletStat processes servlet statistics
func (w *WebSpherePMI) processServletStat(stat *pmiStat, mx map[string]int64) {
	// Extract servlet name from path
	servletName := w.extractServletName(stat.Path)
	if servletName == "" {
		return
	}

	// Check selector
	if w.servletSelector != nil && !w.servletSelector.MatchString(servletName) {
		return
	}

	// Check cardinality limit
	if w.MaxServlets > 0 && !w.collectedServlets[servletName] && len(w.collectedServlets) >= w.MaxServlets {
		return
	}

	servletID := cleanID(servletName)

	// Mark as seen
	w.seenServlets[servletName] = true

	// Add charts if this is a new servlet
	if !w.collectedServlets[servletName] {
		w.addServletCharts(servletName)
	}

	switch stat.Name {
	case "RequestCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_%s_requests", servletID)] = v
			}
		}

	case "ErrorCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_%s_errors", servletID)] = v
			}
		}

	case "ServiceTime", "ResponseTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("servlet_%s_response_time_avg", servletID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("servlet_%s_response_time_max", servletID)] = v
			}
		}
	}
}

// processEJBStat processes EJB statistics
func (w *WebSpherePMI) processEJBStat(stat *pmiStat, mx map[string]int64) {
	// Extract EJB name from path
	ejbName := w.extractEJBName(stat.Path)
	if ejbName == "" {
		return
	}

	// Check selector
	if w.ejbSelector != nil && !w.ejbSelector.MatchString(ejbName) {
		return
	}

	// Check cardinality limit
	if w.MaxEJBs > 0 && !w.collectedEJBs[ejbName] && len(w.collectedEJBs) >= w.MaxEJBs {
		return
	}

	ejbID := cleanID(ejbName)

	// Mark as seen
	w.seenEJBs[ejbName] = true

	// Add charts if this is a new EJB
	if !w.collectedEJBs[ejbName] {
		w.addEJBCharts(ejbName)
	}

	switch stat.Name {
	case "MethodCallCount":
		if stat.CountStatistic != nil {
			if v, err := strconv.ParseInt(stat.CountStatistic.Count, 10, 64); err == nil {
				mx[fmt.Sprintf("ejb_%s_invocations", ejbID)] = v
			}
		}

	case "MethodResponseTime":
		if stat.TimeStatistic != nil {
			if v, err := strconv.ParseFloat(stat.TimeStatistic.Mean, 64); err == nil {
				mx[fmt.Sprintf("ejb_%s_response_time_avg", ejbID)] = int64(v * precision)
			}
			if v, err := strconv.ParseInt(stat.TimeStatistic.Max, 10, 64); err == nil {
				mx[fmt.Sprintf("ejb_%s_response_time_max", ejbID)] = v
			}
		}

	case "PoolSize":
		if stat.BoundedRangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.BoundedRangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("ejb_%s_pool_size", ejbID)] = v
			}
		}

	case "ActiveBeanCount":
		if stat.RangeStatistic != nil {
			if v, err := strconv.ParseInt(stat.RangeStatistic.Current, 10, 64); err == nil {
				mx[fmt.Sprintf("ejb_%s_pool_active", ejbID)] = v
			}
		}
	}
}

// Helper functions to extract names from PMI paths

func (w *WebSpherePMI) extractPoolName(path, module string) string {
	// Path format varies by version:
	// Traditional: server/threadPoolModule/WebContainer
	// Liberty: server/executor/default
	parts := strings.Split(path, "/")
	
	// Handle Liberty-specific paths
	if w.wasEdition == "liberty" && module == "threadPoolModule" {
		// Liberty uses executor instead of threadPoolModule
		for i, part := range parts {
			if part == "executor" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	
	// Traditional path handling
	for i, part := range parts {
		if part == module && i+1 < len(parts) {
			return parts[i+1]
		}
	}
	w.Debugf("failed to extract pool name from path: %s (module: %s, edition: %s)", path, module, w.wasEdition)
	return ""
}

func (w *WebSpherePMI) extractDatasourceName(path string) string {
	// Path format: server/connectionPoolModule/jdbc/myDataSource
	if strings.Contains(path, "connectionPoolModule") && strings.Contains(path, "jdbc") {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if part == "jdbc" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	w.Debugf("failed to extract datasource name from path: %s", path)
	return ""
}

func (w *WebSpherePMI) extractJCAResourceName(path string) string {
	// Path format: server/j2cModule/jca/myJCAResource
	if strings.Contains(path, "j2cModule") && strings.Contains(path, "jca") {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if part == "jca" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	w.Debugf("failed to extract JCA resource name from path: %s", path)
	return ""
}

func (w *WebSpherePMI) extractAppName(path string) string {
	// Path format: server/webAppModule/myApp.war
	if strings.Contains(path, "webAppModule") {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if part == "webAppModule" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	w.Debugf("failed to extract app name from path: %s", path)
	return ""
}

func (w *WebSpherePMI) extractServletName(path string) string {
	// Path format: server/webAppModule/myApp.war/servletModule/MyServlet
	if strings.Contains(path, "servletModule") {
		parts := strings.Split(path, "/")
		for i, part := range parts {
			if part == "servletModule" && i+1 < len(parts) {
				return parts[i+1]
			}
		}
	}
	w.Debugf("failed to extract servlet name from path: %s", path)
	return ""
}

func (w *WebSpherePMI) extractEJBName(path string) string {
	// Path format: server/ejbModule/myEJB.jar/MyBean
	if strings.Contains(path, "ejbModule") {
		parts := strings.Split(path, "/")
		if len(parts) >= 2 {
			// Return the last part as the EJB name
			return parts[len(parts)-1]
		}
	}
	w.Debugf("failed to extract EJB name from path: %s", path)
	return ""
}
