// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_jmx

import (
	"context"
	"fmt"
	"strings"
	"time"
)

const precision = 1000 // Precision multiplier for floating-point values

func (w *WebSphereJMX) collect(ctx context.Context) (map[string]int64, error) {
	if w.charts == nil {
		w.initCharts()
	}

	// Reset seen maps for dynamic instance management
	for k := range w.seenApps {
		delete(w.seenApps, k)
	}
	for k := range w.seenPools {
		delete(w.seenPools, k)
	}
	for k := range w.seenJDBCPools {
		delete(w.seenJDBCPools, k)
	}
	for k := range w.seenJCAPools {
		delete(w.seenJCAPools, k)
	}
	for k := range w.seenJMS {
		delete(w.seenJMS, k)
	}
	for k := range w.seenServlets {
		delete(w.seenServlets, k)
	}
	for k := range w.seenEJBs {
		delete(w.seenEJBs, k)
	}

	mx := make(map[string]int64)

	// Collect different metric types based on configuration
	collectCtx, cancel := context.WithTimeout(ctx, time.Duration(w.JMXTimeout))
	defer cancel()

	// Always collect JVM metrics
	if w.CollectJVMMetrics {
		if err := w.collectJVMMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect JVM metrics: %v", err)
		}
	}

	// Collect thread pool metrics
	if w.CollectThreadPoolMetrics {
		if err := w.collectThreadPoolMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect thread pool metrics: %v", err)
		}
	}

	// Collect JDBC metrics
	if w.CollectJDBCMetrics {
		if err := w.collectJDBCMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect JDBC metrics: %v", err)
		}
	}

	// Collect JCA metrics
	if w.CollectJCAMetrics {
		if err := w.collectJCAMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect JCA metrics: %v", err)
		}
	}

	// Collect JMS metrics
	if w.CollectJMSMetrics {
		if err := w.collectJMSMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect JMS metrics: %v", err)
		}
	}

	// Collect application metrics
	if w.CollectWebAppMetrics {
		if err := w.collectApplicationMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect application metrics: %v", err)
		}
	}

	// Collect cluster metrics (only on deployment managers)
	if w.CollectClusterMetrics && w.ServerType == "dmgr" {
		if err := w.collectClusterMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect cluster metrics: %v", err)
		}
	}

	// Collect APM metrics if enabled
	if w.CollectServletMetrics {
		if err := w.collectServletMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect servlet metrics: %v", err)
		}
	}

	if w.CollectEJBMetrics {
		if err := w.collectEJBMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect EJB metrics: %v", err)
		}
	}

	if w.CollectJDBCAdvanced {
		if err := w.collectJDBCAdvancedMetrics(collectCtx, mx); err != nil {
			w.Warningf("failed to collect advanced JDBC metrics: %v", err)
		}
	}

	// Update lifecycle for dynamic instances
	w.updateInstanceLifecycle()
	w.updateAPMInstanceLifecycle()

	return mx, nil
}

func (w *WebSphereJMX) collectJVMMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command: "SCRAPE",
		Target:  "JVM",
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("JVM metrics collection failed: %s", resp.Message)
	}

	data := resp.Data

	// Heap memory metrics
	if heap, ok := data["heap"].(map[string]interface{}); ok {
		mx["jvm_heap_used"] = int64(getFloat(heap, "used"))
		mx["jvm_heap_committed"] = int64(getFloat(heap, "committed"))
		mx["jvm_heap_max"] = int64(getFloat(heap, "max"))

		if max := mx["jvm_heap_max"]; max > 0 {
			mx["jvm_heap_usage_percent"] = (mx["jvm_heap_used"] * 100 * precision) / max
		}
	}

	// Non-heap memory
	if nonheap, ok := data["nonheap"].(map[string]interface{}); ok {
		mx["jvm_nonheap_used"] = int64(getFloat(nonheap, "used"))
		mx["jvm_nonheap_committed"] = int64(getFloat(nonheap, "committed"))
	}

	// GC metrics
	if gc, ok := data["gc"].(map[string]interface{}); ok {
		mx["jvm_gc_count"] = int64(getFloat(gc, "count"))
		mx["jvm_gc_time"] = int64(getFloat(gc, "time"))
	}

	// Thread metrics
	if threads, ok := data["threads"].(map[string]interface{}); ok {
		mx["jvm_thread_count"] = int64(getFloat(threads, "count"))
		mx["jvm_thread_daemon"] = int64(getFloat(threads, "daemon"))
		mx["jvm_thread_peak"] = int64(getFloat(threads, "peak"))
		mx["jvm_thread_started"] = int64(getFloat(threads, "totalStarted"))
	}

	// Class loading
	if classes, ok := data["classes"].(map[string]interface{}); ok {
		mx["jvm_classes_loaded"] = int64(getFloat(classes, "loaded"))
		mx["jvm_classes_unloaded"] = int64(getFloat(classes, "unloaded"))
	}

	// Process CPU usage
	if cpu, ok := data["cpu"].(map[string]interface{}); ok {
		mx["jvm_process_cpu_usage"] = int64(getFloat(cpu, "processCpuUsage") * precision)
	}

	// JVM uptime
	if uptime, ok := data["uptime"].(float64); ok {
		mx["jvm_uptime"] = int64(uptime)
	}

	return nil
}

func (w *WebSphereJMX) collectThreadPoolMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "THREADPOOLS",
		MaxItems: w.MaxThreadPools,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("thread pool metrics collection failed: %s", resp.Message)
	}

	pools, ok := resp.Data["threadPools"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, poolData := range pools {
		pool, ok := poolData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := pool["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.poolSelector != nil && !w.poolSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxThreadPools > 0 && collected >= w.MaxThreadPools {
			w.Debugf("reached max thread pools limit (%d)", w.MaxThreadPools)
			break
		}

		// Mark as seen
		w.seenPools[name] = true

		// Create charts if new pool
		if !w.collectedPools[name] {
			w.collectedPools[name] = true
			if err := w.charts.Add(*w.newThreadPoolCharts(name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		poolID := cleanName(name)
		mx[fmt.Sprintf("threadpool_%s_size", poolID)] = int64(getFloat(pool, "poolSize"))
		mx[fmt.Sprintf("threadpool_%s_active", poolID)] = int64(getFloat(pool, "activeCount"))
		mx[fmt.Sprintf("threadpool_%s_max", poolID)] = int64(getFloat(pool, "maximumPoolSize"))

		collected++
	}

	return nil
}

func (w *WebSphereJMX) collectJDBCMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "JDBC",
		MaxItems: w.MaxJDBCPools,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("JDBC metrics collection failed: %s", resp.Message)
	}

	pools, ok := resp.Data["jdbcPools"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, poolData := range pools {
		pool, ok := poolData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := pool["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.poolSelector != nil && !w.poolSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxJDBCPools > 0 && collected >= w.MaxJDBCPools {
			w.Debugf("reached max JDBC pools limit (%d)", w.MaxJDBCPools)
			break
		}

		// Mark as seen
		w.seenJDBCPools[name] = true

		// Create charts if new pool
		if !w.collectedJDBCPools[name] {
			w.collectedJDBCPools[name] = true
			if err := w.charts.Add(*w.newJDBCPoolCharts(name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		poolID := cleanName(name)
		mx[fmt.Sprintf("jdbc_%s_size", poolID)] = int64(getFloat(pool, "poolSize"))
		mx[fmt.Sprintf("jdbc_%s_active", poolID)] = int64(getFloat(pool, "numConnectionsUsed"))
		mx[fmt.Sprintf("jdbc_%s_free", poolID)] = int64(getFloat(pool, "numConnectionsFree"))
		mx[fmt.Sprintf("jdbc_%s_wait_time", poolID)] = int64(getFloat(pool, "avgWaitTime") * precision)
		mx[fmt.Sprintf("jdbc_%s_use_time", poolID)] = int64(getFloat(pool, "avgInUseTime") * precision)
		mx[fmt.Sprintf("jdbc_%s_total_created", poolID)] = int64(getFloat(pool, "numConnectionsCreated"))
		mx[fmt.Sprintf("jdbc_%s_total_destroyed", poolID)] = int64(getFloat(pool, "numConnectionsDestroyed"))
		mx[fmt.Sprintf("jdbc_%s_waiting_threads", poolID)] = int64(getFloat(pool, "waitingThreadCount"))

		collected++
	}

	return nil
}

func (w *WebSphereJMX) collectJCAMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "JCA",
		MaxItems: w.MaxJCAPools,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("JCA metrics collection failed: %s", resp.Message)
	}

	pools, ok := resp.Data["jcaPools"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, poolData := range pools {
		pool, ok := poolData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := pool["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.poolSelector != nil && !w.poolSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxJCAPools > 0 && collected >= w.MaxJCAPools {
			w.Debugf("reached max JCA pools limit (%d)", w.MaxJCAPools)
			break
		}

		// Mark as seen
		w.seenJCAPools[name] = true

		// Create charts if new pool
		if !w.collectedJCAPools[name] {
			w.collectedJCAPools[name] = true
			if err := w.charts.Add(*w.newJCAPoolCharts(name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics (similar to JDBC pools)
		poolID := cleanName(name)
		mx[fmt.Sprintf("jca_%s_size", poolID)] = int64(getFloat(pool, "poolSize"))
		mx[fmt.Sprintf("jca_%s_active", poolID)] = int64(getFloat(pool, "numConnectionsUsed"))
		mx[fmt.Sprintf("jca_%s_free", poolID)] = int64(getFloat(pool, "numConnectionsFree"))
		mx[fmt.Sprintf("jca_%s_wait_time", poolID)] = int64(getFloat(pool, "avgWaitTime") * precision)
		mx[fmt.Sprintf("jca_%s_use_time", poolID)] = int64(getFloat(pool, "avgInUseTime") * precision)
		mx[fmt.Sprintf("jca_%s_total_created", poolID)] = int64(getFloat(pool, "numConnectionsCreated"))
		mx[fmt.Sprintf("jca_%s_total_destroyed", poolID)] = int64(getFloat(pool, "numConnectionsDestroyed"))
		mx[fmt.Sprintf("jca_%s_waiting_threads", poolID)] = int64(getFloat(pool, "waitingThreadCount"))

		collected++
	}

	return nil
}

func (w *WebSphereJMX) collectJMSMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "JMS",
		MaxItems: w.MaxJMSDestinations,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("JMS metrics collection failed: %s", resp.Message)
	}

	destinations, ok := resp.Data["jmsDestinations"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, destData := range destinations {
		dest, ok := destData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := dest["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.jmsSelector != nil && !w.jmsSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxJMSDestinations > 0 && collected >= w.MaxJMSDestinations {
			w.Debugf("reached max JMS destinations limit (%d)", w.MaxJMSDestinations)
			break
		}

		// Mark as seen
		w.seenJMS[name] = true

		// Create charts if new destination
		if !w.collectedJMS[name] {
			w.collectedJMS[name] = true
			destType := "queue"
			if dtype, ok := dest["type"].(string); ok && strings.ToLower(dtype) == "topic" {
				destType = "topic"
			}
			if err := w.charts.Add(*w.newJMSDestinationCharts(name, destType)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		destID := cleanName(name)
		mx[fmt.Sprintf("jms_%s_messages_current", destID)] = int64(getFloat(dest, "messagesCurrentCount"))
		mx[fmt.Sprintf("jms_%s_messages_pending", destID)] = int64(getFloat(dest, "messagesPendingCount"))
		mx[fmt.Sprintf("jms_%s_messages_total", destID)] = int64(getFloat(dest, "messagesAddedCount"))
		mx[fmt.Sprintf("jms_%s_consumers", destID)] = int64(getFloat(dest, "consumerCount"))

		collected++
	}

	return nil
}

func (w *WebSphereJMX) collectApplicationMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "APPLICATIONS",
		MaxItems: w.MaxApplications,
		CollectOptions: map[string]bool{
			"sessions":     w.CollectSessionMetrics,
			"transactions": w.CollectTransactionMetrics,
		},
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("application metrics collection failed: %s", resp.Message)
	}

	apps, ok := resp.Data["applications"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, appData := range apps {
		app, ok := appData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := app["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.appSelector != nil && !w.appSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxApplications > 0 && collected >= w.MaxApplications {
			w.Debugf("reached max applications limit (%d)", w.MaxApplications)
			break
		}

		// Mark as seen
		w.seenApps[name] = true

		// Create charts if new app
		if !w.collectedApps[name] {
			w.collectedApps[name] = true
			if err := w.charts.Add(*w.newApplicationCharts(name, w.CollectSessionMetrics, w.CollectTransactionMetrics)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		appID := cleanName(name)
		mx[fmt.Sprintf("app_%s_requests", appID)] = int64(getFloat(app, "requestCount"))
		mx[fmt.Sprintf("app_%s_errors", appID)] = int64(getFloat(app, "errorCount"))
		mx[fmt.Sprintf("app_%s_response_time", appID)] = int64(getFloat(app, "avgResponseTime") * precision)
		mx[fmt.Sprintf("app_%s_max_response_time", appID)] = int64(getFloat(app, "maxResponseTime") * precision)

		// Session metrics
		if w.CollectSessionMetrics {
			mx[fmt.Sprintf("app_%s_sessions_active", appID)] = int64(getFloat(app, "activeSessions"))
			mx[fmt.Sprintf("app_%s_sessions_live", appID)] = int64(getFloat(app, "liveSessions"))
			mx[fmt.Sprintf("app_%s_sessions_invalidated", appID)] = int64(getFloat(app, "invalidatedSessions"))
		}

		// Transaction metrics
		if w.CollectTransactionMetrics {
			mx[fmt.Sprintf("app_%s_transactions_active", appID)] = int64(getFloat(app, "activeTransactions"))
			mx[fmt.Sprintf("app_%s_transactions_committed", appID)] = int64(getFloat(app, "committedTransactions"))
			mx[fmt.Sprintf("app_%s_transactions_rolledback", appID)] = int64(getFloat(app, "rolledbackTransactions"))
			mx[fmt.Sprintf("app_%s_transactions_timeout", appID)] = int64(getFloat(app, "timedoutTransactions"))
		}

		collected++
	}

	return nil
}

func (w *WebSphereJMX) updateInstanceLifecycle() {
	// Remove applications that are no longer present
	for app := range w.collectedApps {
		if !w.seenApps[app] {
			delete(w.collectedApps, app)
			w.removeApplicationCharts(app)
		}
	}

	// Remove thread pools that are no longer present
	for pool := range w.collectedPools {
		if !w.seenPools[pool] {
			delete(w.collectedPools, pool)
			w.removeThreadPoolCharts(pool)
		}
	}

	// Remove JDBC pools that are no longer present
	for pool := range w.collectedJDBCPools {
		if !w.seenJDBCPools[pool] {
			delete(w.collectedJDBCPools, pool)
			w.removeJDBCPoolCharts(pool)
		}
	}

	// Remove JCA pools that are no longer present
	for pool := range w.collectedJCAPools {
		if !w.seenJCAPools[pool] {
			delete(w.collectedJCAPools, pool)
			w.removeJCAPoolCharts(pool)
		}
	}

	// Remove JMS destinations that are no longer present
	for dest := range w.collectedJMS {
		if !w.seenJMS[dest] {
			delete(w.collectedJMS, dest)
			w.removeJMSDestinationCharts(dest)
		}
	}
}

func (w *WebSphereJMX) removeApplicationCharts(app string) {
	appID := cleanName(app)
	prefix := fmt.Sprintf("app_%s_", appID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (w *WebSphereJMX) removeThreadPoolCharts(pool string) {
	poolID := cleanName(pool)
	prefix := fmt.Sprintf("threadpool_%s_", poolID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (w *WebSphereJMX) removeJDBCPoolCharts(pool string) {
	poolID := cleanName(pool)
	prefix := fmt.Sprintf("jdbc_%s_", poolID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (w *WebSphereJMX) removeJMSDestinationCharts(dest string) {
	destID := cleanName(dest)
	prefix := fmt.Sprintf("jms_%s_", destID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (w *WebSphereJMX) removeJCAPoolCharts(pool string) {
	poolID := cleanName(pool)
	prefix := fmt.Sprintf("jca_%s_", poolID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (w *WebSphereJMX) collectClusterMetrics(ctx context.Context, mx map[string]int64) error {
	// Cluster state metrics
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command: "SCRAPE",
		Target:  "CLUSTER",
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("cluster metrics collection failed: %s", resp.Message)
	}

	// Access cluster data from resp.Data
	if clusterData, ok := resp.Data["cluster"].(map[string]interface{}); ok {
		// Cluster state: 0=stopped, 1=partial, 2=running
		if state, ok := clusterData["state"].(string); ok {
			switch state {
			case "RUNNING":
				mx["cluster_state"] = 2
			case "PARTIAL":
				mx["cluster_state"] = 1
			case "STOPPED":
				mx["cluster_state"] = 0
			default:
				mx["cluster_state"] = -1
			}
		}

		mx["cluster_target_members"] = int64(getFloat(clusterData, "targetMemberCount"))
		mx["cluster_running_members"] = int64(getFloat(clusterData, "runningMemberCount"))
		mx["cluster_wlm_enabled"] = boolToInt(clusterData["wlmEnabled"])
		mx["cluster_session_affinity"] = boolToInt(clusterData["sessionAffinity"])
	}

	// HAManager metrics
	haResp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command: "SCRAPE",
		Target:  "HAMANAGER",
	})
	if err == nil && haResp.Status == "OK" {
		if haData, ok := haResp.Data["hamanager"].(map[string]interface{}); ok {
			mx["ha_core_group_size"] = int64(getFloat(haData, "coreGroupSize"))
			mx["ha_coordinator"] = boolToInt(haData["isCoordinator"])
			mx["ha_bulletins_sent"] = int64(getFloat(haData, "bulletinsSent"))
		}
	}

	// Dynamic cluster metrics
	dynResp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command: "SCRAPE",
		Target:  "DYNAMIC_CLUSTER",
	})
	if err == nil && dynResp.Status == "OK" {
		if dynData, ok := dynResp.Data["dynamicCluster"].(map[string]interface{}); ok {
			mx["dynamic_cluster_min_instances"] = int64(getFloat(dynData, "minInstances"))
			mx["dynamic_cluster_max_instances"] = int64(getFloat(dynData, "maxInstances"))
			mx["dynamic_cluster_target_instances"] = int64(getFloat(dynData, "targetInstances"))
		}
	}

	// Replication domain metrics
	replResp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command: "SCRAPE",
		Target:  "REPLICATION",
	})
	if err == nil && replResp.Status == "OK" {
		if replData, ok := replResp.Data["replication"].(map[string]interface{}); ok {
			mx["replication_bytes_sent"] = int64(getFloat(replData, "bytesSent"))
			mx["replication_bytes_received"] = int64(getFloat(replData, "bytesReceived"))
			mx["replication_sync_failures"] = int64(getFloat(replData, "syncFailures"))
			mx["replication_async_queue_depth"] = int64(getFloat(replData, "asyncQueueDepth"))
		}
	}

	return nil
}

// Helper functions

func getFloat(m map[string]interface{}, key string) float64 {
	if v, ok := m[key]; ok {
		switch val := v.(type) {
		case float64:
			return val
		case float32:
			return float64(val)
		case int:
			return float64(val)
		case int64:
			return float64(val)
		case string:
			// Try to parse string as float
			var f float64
			fmt.Sscanf(val, "%f", &f)
			return f
		}
	}
	return 0
}

func boolToInt(v interface{}) int64 {
	if b, ok := v.(bool); ok && b {
		return 1
	}
	return 0
}

func cleanName(name string) string {
	r := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
		",", "_",
		"(", "_",
		")", "_",
		"[", "_",
		"]", "_",
	)
	return strings.ToLower(r.Replace(name))
}

// APM Collection Methods

func (w *WebSphereJMX) collectServletMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "SERVLET_METRICS",
		MaxItems: w.MaxServlets,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("servlet metrics collection failed: %s", resp.Message)
	}

	servlets, ok := resp.Data["servletMetrics"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, servletData := range servlets {
		servlet, ok := servletData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := servlet["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.servletSelector != nil && !w.servletSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxServlets > 0 && collected >= w.MaxServlets {
			w.Debugf("reached max servlets limit (%d)", w.MaxServlets)
			break
		}

		// Mark as seen
		w.seenServlets[name] = true

		// Create charts if new servlet
		if !w.collectedServlets[name] {
			w.collectedServlets[name] = true
			if err := w.charts.Add(*w.newServletCharts(name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		servletID := cleanName(name)
		mx[fmt.Sprintf("servlet_%s_requests", servletID)] = int64(getFloat(servlet, "requestCount"))
		mx[fmt.Sprintf("servlet_%s_errors", servletID)] = int64(getFloat(servlet, "errorCount"))
		mx[fmt.Sprintf("servlet_%s_response_time_avg", servletID)] = int64(getFloat(servlet, "avgResponseTime") * precision)
		mx[fmt.Sprintf("servlet_%s_response_time_max", servletID)] = int64(getFloat(servlet, "maxResponseTime") * precision)
		mx[fmt.Sprintf("servlet_%s_concurrent_requests", servletID)] = int64(getFloat(servlet, "concurrentRequests"))

		collected++
	}

	return nil
}

func (w *WebSphereJMX) collectEJBMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "EJB_METRICS",
		MaxItems: w.MaxEJBs,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("EJB metrics collection failed: %s", resp.Message)
	}

	ejbs, ok := resp.Data["ejbMetrics"].([]interface{})
	if !ok {
		return nil
	}

	collected := 0
	for _, ejbData := range ejbs {
		ejb, ok := ejbData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := ejb["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Apply selector if configured
		if w.ejbSelector != nil && !w.ejbSelector.MatchString(name) {
			continue
		}

		// Check cardinality limit
		if w.MaxEJBs > 0 && collected >= w.MaxEJBs {
			w.Debugf("reached max EJBs limit (%d)", w.MaxEJBs)
			break
		}

		// Mark as seen
		w.seenEJBs[name] = true

		// Create charts if new EJB
		if !w.collectedEJBs[name] {
			w.collectedEJBs[name] = true
			if err := w.charts.Add(*w.newEJBCharts(name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		ejbID := cleanName(name)
		mx[fmt.Sprintf("ejb_%s_invocations", ejbID)] = int64(getFloat(ejb, "invocationCount"))
		mx[fmt.Sprintf("ejb_%s_response_time_avg", ejbID)] = int64(getFloat(ejb, "avgResponseTime") * precision)
		mx[fmt.Sprintf("ejb_%s_response_time_max", ejbID)] = int64(getFloat(ejb, "maxResponseTime") * precision)
		mx[fmt.Sprintf("ejb_%s_pool_size", ejbID)] = int64(getFloat(ejb, "poolSize"))
		mx[fmt.Sprintf("ejb_%s_pool_available", ejbID)] = int64(getFloat(ejb, "poolAvailable"))

		collected++
	}

	return nil
}

func (w *WebSphereJMX) collectJDBCAdvancedMetrics(ctx context.Context, mx map[string]int64) error {
	resp, err := w.jmxHelper.sendCommand(ctx, jmxCommand{
		Command:  "SCRAPE",
		Target:   "JDBC_ADVANCED",
		MaxItems: w.MaxJDBCPools,
	})
	if err != nil {
		return err
	}

	if resp.Status != "OK" {
		return fmt.Errorf("advanced JDBC metrics collection failed: %s", resp.Message)
	}

	pools, ok := resp.Data["jdbcAdvanced"].([]interface{})
	if !ok {
		return nil
	}

	for _, poolData := range pools {
		pool, ok := poolData.(map[string]interface{})
		if !ok {
			continue
		}

		name, ok := pool["name"].(string)
		if !ok || name == "" {
			continue
		}

		// Only collect for pools we're already tracking
		if !w.collectedJDBCPools[name] {
			continue
		}

		// Collect advanced metrics
		poolID := cleanName(name)
		
		// Time breakdown: query time vs connection hold time
		mx[fmt.Sprintf("jdbc_%s_query_time", poolID)] = int64(getFloat(pool, "avgQueryTime") * precision)
		mx[fmt.Sprintf("jdbc_%s_connection_hold_time", poolID)] = int64(getFloat(pool, "avgConnectionHoldTime") * precision)
		
		// Statement cache metrics
		mx[fmt.Sprintf("jdbc_%s_stmt_cache_hits", poolID)] = int64(getFloat(pool, "statementCacheHits"))
		mx[fmt.Sprintf("jdbc_%s_stmt_cache_misses", poolID)] = int64(getFloat(pool, "statementCacheMisses"))
		mx[fmt.Sprintf("jdbc_%s_stmt_cache_size", poolID)] = int64(getFloat(pool, "statementCacheSize"))
		
		// Connection reuse
		mx[fmt.Sprintf("jdbc_%s_connection_reuse_count", poolID)] = int64(getFloat(pool, "connectionReuseCount"))
	}

	return nil
}

func (w *WebSphereJMX) updateAPMInstanceLifecycle() {
	// Remove servlets that are no longer present
	for servlet := range w.collectedServlets {
		if !w.seenServlets[servlet] {
			delete(w.collectedServlets, servlet)
			w.removeServletCharts(servlet)
		}
	}

	// Remove EJBs that are no longer present
	for ejb := range w.collectedEJBs {
		if !w.seenEJBs[ejb] {
			delete(w.collectedEJBs, ejb)
			w.removeEJBCharts(ejb)
		}
	}
}

func (w *WebSphereJMX) removeServletCharts(servlet string) {
	servletID := cleanName(servlet)
	prefix := fmt.Sprintf("servlet_%s_", servletID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}

func (w *WebSphereJMX) removeEJBCharts(ejb string) {
	ejbID := cleanName(ejb)
	prefix := fmt.Sprintf("ejb_%s_", ejbID)
	for _, chart := range *w.charts {
		if strings.HasPrefix(chart.ID, prefix) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
