// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere_mp

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strconv"
	"strings"
)

const precision = 1000 // Precision multiplier for floating-point values

// Prometheus metric line pattern
var promMetricPattern = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^}]*\})?)?\s+([+-]?[0-9]*\.?[0-9]+(?:[eE][+-]?[0-9]+)?)\s*$`)

func (w *WebSphereMicroProfile) collect(ctx context.Context) (map[string]int64, error) {

	// Collect MicroProfile metrics
	metrics, err := w.collectMicroProfileMetrics(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to collect MicroProfile metrics: %w", err)
	}

	mx := make(map[string]int64)

	// Process collected metrics
	w.processMetrics(mx, metrics)

	// Update seen instances for lifecycle management
	w.updateSeenInstances()

	// Log collection summary on first successful collection or significant changes
	metricCount := len(mx)
	if metricCount > 0 && (len(w.collectedMetrics) == 1 || metricCount != w.lastMetricCount) {
		w.Infof("collected %d metrics from %s", metricCount, w.MetricsEndpoint)
		w.lastMetricCount = metricCount
	}

	return mx, nil
}

func (w *WebSphereMicroProfile) collectMicroProfileMetrics(ctx context.Context) (map[string]float64, error) {
	req, err := http.NewRequestWithContext(ctx, "GET", w.metricsURL, nil)
	if err != nil {
		return nil, err
	}

	// Set request headers for Prometheus format
	req.Header.Set("Accept", "text/plain")
	if w.HTTPConfig.RequestConfig.Username != "" || w.HTTPConfig.RequestConfig.Password != "" {
		req.SetBasicAuth(w.HTTPConfig.RequestConfig.Username, w.HTTPConfig.RequestConfig.Password)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("HTTP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		if resp.StatusCode == http.StatusUnauthorized {
			return nil, fmt.Errorf("authentication failed (401): verify username and password")
		} else if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("metrics endpoint not found (404): verify endpoint path '%s' and that MicroProfile Metrics is enabled", w.MetricsEndpoint)
		}
		return nil, fmt.Errorf("unexpected status code %d: %s", resp.StatusCode, string(body))
	}

	// Parse Prometheus format metrics
	metrics, err := w.parsePrometheusMetrics(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to parse metrics: %w", err)
	}

	// Set server type if not already known
	if w.serverType == "" {
		w.serverType = "Liberty MicroProfile"
		w.Debugf("detected WebSphere Liberty with MicroProfile Metrics")
	}

	// Debug: log first few metrics to see what we're getting
	count := 0
	for name, value := range metrics {
		if count < 10 {
			w.Debugf("metric: %s = %f", name, value)
			count++
		}
	}

	return metrics, nil
}

func (w *WebSphereMicroProfile) parsePrometheusMetrics(reader io.Reader) (map[string]float64, error) {
	metrics := make(map[string]float64)
	scanner := bufio.NewScanner(reader)

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// Skip comments and empty lines
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Parse metric line
		matches := promMetricPattern.FindStringSubmatch(line)
		if len(matches) != 3 {
			continue
		}

		metricName := matches[1]
		valueStr := matches[2]

		// Parse value
		value, err := strconv.ParseFloat(valueStr, 64)
		if err != nil {
			w.Debugf("failed to parse metric '%s' value '%s': %v", metricName, valueStr, err)
			continue
		}

		// Clean metric name for processing
		cleanedName := w.cleanMetricName(metricName)
		if cleanedName != "" {
			metrics[cleanedName] = value
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	return metrics, nil
}

func (w *WebSphereMicroProfile) cleanMetricName(metricName string) string {
	// Remove labels from metric name (everything after {)
	if idx := strings.Index(metricName, "{"); idx != -1 {
		baseMetric := metricName[:idx]
		labels := metricName[idx:]

		// Extract meaningful labels for instance identification
		return w.processMetricWithLabels(baseMetric, labels)
	}

	return metricName
}

func (w *WebSphereMicroProfile) processMetricWithLabels(baseMetric, labels string) string {
	// Extract common labels for creating unique metric names
	// For REST metrics, extract method and endpoint
	if strings.HasPrefix(baseMetric, "REST_request") {
		method := w.extractLabel(labels, "method")
		endpoint := w.extractLabel(labels, "endpoint")
		if method != "" && endpoint != "" {
			return fmt.Sprintf("%s_%s_%s", baseMetric, cleanName(method), cleanName(endpoint))
		}
	}

	// For application metrics, extract app name
	if strings.HasPrefix(baseMetric, "application_") {
		app := w.extractLabel(labels, "app")
		if app != "" {
			return fmt.Sprintf("%s_%s", baseMetric, cleanName(app))
		}
	}

	return baseMetric
}

func (w *WebSphereMicroProfile) extractLabel(labels, key string) string {
	// Simple label extraction from {key="value",other="value"}
	pattern := fmt.Sprintf(`%s="([^"]*)"`, key)
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(labels)
	if len(matches) >= 2 {
		return matches[1]
	}
	return ""
}

func (w *WebSphereMicroProfile) processMetrics(mx map[string]int64, metrics map[string]float64) {
	restCount := 0

	for metricName, value := range metrics {
		// Convert float to int64 with appropriate handling
		var intValue int64

		// Bytes and counts are already integers - no precision needed
		if strings.Contains(metricName, "_bytes") ||
			strings.Contains(metricName, "_count") ||
			strings.Contains(metricName, "_total") ||
			strings.Contains(metricName, "Count") ||
			strings.Contains(metricName, "Size") ||
			strings.Contains(metricName, "activeThreads") ||
			strings.Contains(metricName, "size") {
			intValue = int64(value)
		} else {
			// Apply precision for floating-point values (percentages, seconds, etc.)
			intValue = int64(value * precision)
		}

		// Check for vendor-specific metrics first (servlet, session, threadpool)
		if strings.HasPrefix(metricName, "servlet_") ||
			strings.HasPrefix(metricName, "session_") ||
			strings.HasPrefix(metricName, "threadpool_") {
			w.processVendorMetric(mx, metricName, intValue)
		} else if w.CollectJVMMetrics && w.jvmPattern.MatchString(metricName) {
			w.processJVMMetric(mx, metricName, intValue)
		} else if w.CollectRESTMetrics && w.restPattern.MatchString(metricName) {
			if w.MaxRESTEndpoints == 0 || restCount < w.MaxRESTEndpoints {
				if w.restSelector == nil || w.restSelector.MatchString(metricName) {
					w.processRESTMetric(mx, metricName, intValue)
					restCount++
				}
			}
		} else {
			// Everything else goes to "other" family
			w.processOtherMetric(mx, metricName, intValue)
		}
	}
}

func (w *WebSphereMicroProfile) processJVMMetric(mx map[string]int64, metricName string, value int64) {
	// Create JVM charts dynamically when JVM metrics are first discovered
	if !w.jvmChartsCreated {
		w.jvmChartsCreated = true
		charts := newJVMCharts()
		if err := w.charts.Add(*charts...); err != nil {
			w.Warningf("failed to add JVM charts: %v", err)
		} else {
			w.Debugf("created JVM charts dynamically")
		}
	}

	// Map Liberty MicroProfile metrics to base chart dimensions
	// Based on actual metric names observed in Liberty MicroProfile endpoints
	switch metricName {
	// JVM uptime
	case "jvm_uptime_seconds":
		mx["jvm_uptime_seconds"] = value
		return
	case "classloader_loadedClasses_total":
		mx["jvm_classes_loaded"] = value
		return
	case "classloader_loadedClasses_count":
		mx["jvm_classes_loaded"] = value
		return
	case "classloader_unloadedClasses_total":
		mx["jvm_classes_unloaded"] = value
		return
	case "thread_count":
		mx["jvm_thread_count"] = value
		// Store for calculating "other" threads later
		if daemonCount, exists := mx["jvm_thread_daemon_count"]; exists {
			mx["jvm_thread_other_count"] = value - daemonCount
		}
		return
	case "thread_daemon_count":
		mx["jvm_thread_daemon_count"] = value
		// Calculate "other" threads if we have total count
		if totalCount, exists := mx["jvm_thread_count"]; exists {
			mx["jvm_thread_other_count"] = totalCount - value
		}
		return
	case "thread_max_count":
		mx["jvm_thread_max_count"] = value
		return
	case "memory_usedHeap_bytes":
		mx["jvm_memory_heap_used"] = value
		// Calculate free memory if we have committed memory
		if committed, exists := mx["jvm_memory_heap_committed"]; exists {
			mx["jvm_memory_heap_free"] = committed - value
		}
		return
	case "memory_committedHeap_bytes":
		mx["jvm_memory_heap_committed"] = value
		// Calculate free memory if we have used memory
		if used, exists := mx["jvm_memory_heap_used"]; exists {
			mx["jvm_memory_heap_free"] = value - used
		}
		return
	case "memory_maxHeap_bytes":
		mx["jvm_memory_heap_max"] = value
		return
	case "gc_total":
		mx["jvm_gc_collections_total"] = value
		return
	case "gc_time_seconds":
		mx["gc_time_seconds"] = value
		return
	case "gc_time_per_cycle_seconds":
		mx["gc_time_per_cycle_seconds"] = value
		return
	// CPU metrics
	case "cpu_availableProcessors":
		mx["cpu_availableProcessors"] = value
		return
	case "cpu_processCpuLoad_percent":
		mx["cpu_processCpuLoad_percent"] = value
		return
	case "cpu_processCpuTime_seconds":
		mx["cpu_processCpuTime_seconds"] = value
		return
	case "cpu_processCpuUtilization_percent":
		mx["cpu_processCpuUtilization_percent"] = value
		return
	case "cpu_systemLoadAverage":
		mx["cpu_systemLoadAverage"] = value
		return
	// Memory utilization
	case "memory_heapUtilization_percent":
		mx["memory_heapUtilization_percent"] = value
		return
	// Non-heap memory metrics
	case "memory_usedNonHeap_bytes":
		mx["memory_usedNonHeap_bytes"] = value
		return
	case "memory_committedNonHeap_bytes":
		mx["memory_committedNonHeap_bytes"] = value
		return
	case "memory_maxNonHeap_bytes":
		mx["memory_maxNonHeap_bytes"] = value
		return
	}

	// Handle other JVM metrics dynamically
	cleanedName := cleanName(metricName)
	mx[cleanedName] = value

	// Track for dynamic chart creation
	w.seenMetrics[cleanedName] = true

	if !w.collectedMetrics[cleanedName] {
		w.collectedMetrics[cleanedName] = true
		// Add JVM charts dynamically if needed
		if charts := w.createJVMChart(cleanedName, metricName); charts != nil {
			if err := w.charts.Add(*charts...); err != nil {
				w.Warningf("failed to add JVM chart for metric '%s': %v", metricName, err)
			}
		}
	}
}

func (w *WebSphereMicroProfile) processRESTMetric(mx map[string]int64, metricName string, value int64) {
	// Handle REST metrics
	cleanedName := cleanName(metricName)
	mx[cleanedName] = value

	// Track for dynamic chart creation
	w.seenMetrics[cleanedName] = true

	if !w.collectedMetrics[cleanedName] {
		w.collectedMetrics[cleanedName] = true
		// Add REST charts dynamically
		if charts := w.createRESTChart(cleanedName, metricName); charts != nil {
			if err := w.charts.Add(*charts...); err != nil {
				w.Warningf("failed to add REST chart for metric '%s': %v", metricName, err)
			}
		}
	}
}

func (w *WebSphereMicroProfile) processVendorMetric(mx map[string]int64, metricName string, value int64) {
	// Create charts dynamically when vendor-specific metrics are first discovered

	// Check if this is a threadpool metric and create charts if needed
	if strings.HasPrefix(metricName, "threadpool_") && !w.threadPoolChartsCreated {
		w.threadPoolChartsCreated = true
		charts := newThreadPoolCharts()
		if err := w.charts.Add(*charts...); err != nil {
			w.Warningf("failed to add threadpool charts: %v", err)
		} else {
			w.Debugf("created threadpool charts dynamically")
		}
	}

	// Check if this is a servlet metric and create charts if needed
	if strings.HasPrefix(metricName, "servlet_") && !w.servletChartsCreated {
		w.servletChartsCreated = true
		charts := newServletCharts()
		if err := w.charts.Add(*charts...); err != nil {
			w.Warningf("failed to add servlet charts: %v", err)
		} else {
			w.Debugf("created servlet charts dynamically")
		}
	}

	// Check if this is a session metric and create charts if needed
	if strings.HasPrefix(metricName, "session_") && !w.sessionChartsCreated {
		w.sessionChartsCreated = true
		charts := newSessionCharts()
		if err := w.charts.Add(*charts...); err != nil {
			w.Warningf("failed to add session charts: %v", err)
		} else {
			w.Debugf("created session charts dynamically")
		}
	}

	// Handle vendor-specific metrics (servlet, session, threadpool)
	switch metricName {
	// Threadpool metrics
	case "threadpool_activeThreads":
		mx["threadpool_activeThreads"] = value
		// Calculate idle threads if we have pool size
		if poolSize, exists := mx["threadpool_size"]; exists {
			mx["threadpool_idle"] = poolSize - value
		}
		return
	case "threadpool_size":
		mx["threadpool_size"] = value
		// Calculate idle threads if we have active count
		if activeThreads, exists := mx["threadpool_activeThreads"]; exists {
			mx["threadpool_idle"] = value - activeThreads
		}
		return
	// Servlet metrics
	case "servlet_request_total":
		mx["servlet_request_total"] = value
		return
	case "servlet_request_elapsedTime_per_request_seconds":
		mx["servlet_request_elapsedTime_per_request_seconds"] = value
		return
	case "servlet_responseTime_total_seconds":
		mx["servlet_responseTime_total_seconds"] = value
		return
	// Session metrics
	case "session_activeSessions":
		mx["session_activeSessions"] = value
		return
	case "session_create_total":
		mx["session_create_total"] = value
		return
	case "session_invalidated_total":
		mx["session_invalidated_total"] = value
		return
	case "session_invalidatedbyTimeout_total":
		mx["session_invalidatedbyTimeout_total"] = value
		return
	case "session_liveSessions":
		mx["session_liveSessions"] = value
		return
	}
}

func (w *WebSphereMicroProfile) processOtherMetric(mx map[string]int64, metricName string, value int64) {
	// Handle any other metrics that don't fit predefined categories
	cleanedName := cleanName(metricName)
	mx[cleanedName] = value

	// Track for dynamic chart creation
	w.seenMetrics[cleanedName] = true

	if !w.collectedMetrics[cleanedName] {
		w.collectedMetrics[cleanedName] = true
		// Add chart dynamically
		if charts := w.createOtherChart(cleanedName, metricName); charts != nil {
			if err := w.charts.Add(*charts...); err != nil {
				w.Warningf("failed to add chart for other metric '%s': %v", metricName, err)
			}
		}
	}
}

func (w *WebSphereMicroProfile) updateSeenInstances() {
	// Clean up metrics that are no longer present
	for metric := range w.collectedMetrics {
		if !w.seenMetrics[metric] {
			delete(w.collectedMetrics, metric)
			// Remove charts for this metric
			for _, chart := range *w.charts {
				if strings.Contains(chart.ID, metric) {
					chart.MarkRemove()
					chart.MarkNotCreated()
				}
			}
		}
	}

	// Reset seen metrics for next collection
	w.seenMetrics = make(map[string]bool)
}
