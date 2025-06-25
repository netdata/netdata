// SPDX-License-Identifier: GPL-3.0-or-later

//go:build cgo
// +build cgo

package websphere

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
)

const precision = 1000 // Precision multiplier for floating-point values

// Liberty metrics structure based on /ibm/api/metrics endpoint
type libertyMetrics struct {
	Heap struct {
		Used      int64 `json:"used"`
		Committed int64 `json:"committed"`
		Max       int64 `json:"max"`
	} `json:"heap"`

	GC struct {
		Count int64 `json:"count"`
		Time  int64 `json:"time"`
	} `json:"gc"`

	Thread struct {
		Count  int64 `json:"count"`
		Daemon int64 `json:"daemon"`
		Peak   int64 `json:"peak"`
		Live   int64 `json:"totalStarted"`
	} `json:"thread"`

	Classes struct {
		Loaded   int64 `json:"loaded"`
		Unloaded int64 `json:"unloaded"`
	} `json:"classes"`

	Session struct {
		Active      int64 `json:"activeSessions"`
		Live        int64 `json:"liveSessions"`
		Invalidated int64 `json:"invalidatedSessions"`
		Create      int64 `json:"create"`
	} `json:"session"`

	Request struct {
		Total int64 `json:"count"`
	} `json:"request"`

	ThreadPool []struct {
		Name   string `json:"name"`
		Size   int64  `json:"size"`
		Active int64  `json:"activeThreads"`
		Hung   int64  `json:"hungThreads"`
		Max    int64  `json:"maximumSize"`
	} `json:"threadpool"`

	ConnectionPool []struct {
		Name      string  `json:"name"`
		Size      int64   `json:"size"`
		Free      int64   `json:"freeConnections"`
		Max       int64   `json:"maxConnections"`
		WaitTime  float64 `json:"waitTime"`
		Timeouts  int64   `json:"connectionTimeout"`
		InUse     int64   `json:"inUseConnections"`
		Destroyed int64   `json:"destroyedCount"`
	} `json:"connectionpool"`

	Application []struct {
		Name         string  `json:"name"`
		Requests     int64   `json:"requestCount"`
		ResponseTime float64 `json:"responseTime"`
		Errors       int64   `json:"errorCount"`
	} `json:"application"`
}

func (w *WebSphere) collect(ctx context.Context) (map[string]int64, error) {
	if w.charts == nil {
		w.initCharts()
	}

	// Try Liberty metrics endpoint first
	metrics, err := w.collectLibertyMetrics(ctx)
	if err != nil {
		// In the future, we could fall back to JMX or other methods
		return nil, fmt.Errorf("failed to collect metrics: %w", err)
	}

	mx := make(map[string]int64)

	// Collect JVM metrics
	if w.CollectJVMMetrics {
		w.collectJVM(mx, metrics)
	}

	// Collect web container metrics
	w.collectWebContainer(mx, metrics)

	// Collect thread pool metrics
	if w.CollectThreadPoolMetrics {
		w.collectThreadPools(mx, metrics)
	}

	// Collect connection pool metrics
	if w.CollectConnectionPoolMetrics {
		w.collectConnectionPools(mx, metrics)
	}

	// Collect application metrics
	if w.CollectWebAppMetrics {
		w.collectApplications(mx, metrics)
	}

	// Update seen instances for lifecycle management
	w.updateSeenInstances()

	return mx, nil
}

func (w *WebSphere) collectLibertyMetrics(ctx context.Context) (*libertyMetrics, error) {
	u, err := url.Parse(w.HTTPConfig.RequestConfig.URL)
	if err != nil {
		return nil, err
	}

	u.Path = w.MetricsEndpoint
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
	if err != nil {
		return nil, err
	}

	// Set request headers
	req.Header.Set("Accept", "application/json")
	if w.HTTPConfig.RequestConfig.Username != "" || w.HTTPConfig.RequestConfig.Password != "" {
		req.SetBasicAuth(w.HTTPConfig.RequestConfig.Username, w.HTTPConfig.RequestConfig.Password)
	}

	resp, err := w.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
	}

	var metrics libertyMetrics
	if err := json.NewDecoder(resp.Body).Decode(&metrics); err != nil {
		return nil, fmt.Errorf("failed to decode metrics: %w", err)
	}

	// Set server type if not already known
	if w.serverType == "" {
		w.serverType = "Liberty"
		w.Debugf("detected WebSphere Liberty server")
	}

	return &metrics, nil
}

func (w *WebSphere) collectJVM(mx map[string]int64, m *libertyMetrics) {
	// Heap metrics in bytes
	mx["jvm_heap_used"] = m.Heap.Used
	mx["jvm_heap_committed"] = m.Heap.Committed
	mx["jvm_heap_max"] = m.Heap.Max

	// GC metrics
	mx["jvm_gc_count"] = m.GC.Count
	mx["jvm_gc_time"] = m.GC.Time

	// Thread metrics
	mx["jvm_thread_count"] = m.Thread.Count
	mx["jvm_thread_daemon"] = m.Thread.Daemon
	mx["jvm_thread_peak"] = m.Thread.Peak

	// Class loading metrics
	mx["jvm_classes_loaded"] = m.Classes.Loaded
	mx["jvm_classes_unloaded"] = m.Classes.Unloaded
}

func (w *WebSphere) collectWebContainer(mx map[string]int64, m *libertyMetrics) {
	// Session metrics
	if w.CollectSessionMetrics {
		mx["web_sessions_active"] = m.Session.Active
		mx["web_sessions_live"] = m.Session.Live
		mx["web_sessions_invalidated"] = m.Session.Invalidated
	}

	// Request metrics
	mx["web_requests_total"] = m.Request.Total

	// Note: HTTP error breakdown (4xx, 5xx) not available via REST API
	// Would require access log parsing or MicroProfile Metrics
}

func (w *WebSphere) collectThreadPools(mx map[string]int64, m *libertyMetrics) {
	collected := 0

	for _, pool := range m.ThreadPool {
		// Apply filtering if configured
		if w.CollectPoolsMatching != "" && !strings.Contains(strings.ToLower(pool.Name), strings.ToLower(w.CollectPoolsMatching)) {
			continue
		}

		// Check cardinality limit
		if w.MaxThreadPools > 0 && collected >= w.MaxThreadPools {
			break
		}

		poolID := cleanName(pool.Name)

		// Track this pool
		w.seenPools[pool.Name] = true

		// Create charts if this is a new pool
		if !w.collectedPools[pool.Name] {
			w.collectedPools[pool.Name] = true
			if err := w.charts.Add(*newThreadPoolCharts(pool.Name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		mx[fmt.Sprintf("threadpool_%s_size", poolID)] = pool.Size
		mx[fmt.Sprintf("threadpool_%s_active", poolID)] = pool.Active
		mx[fmt.Sprintf("threadpool_%s_hung", poolID)] = pool.Hung
		mx[fmt.Sprintf("threadpool_%s_max", poolID)] = pool.Max

		collected++
	}
}

func (w *WebSphere) collectConnectionPools(mx map[string]int64, m *libertyMetrics) {
	collected := 0

	for _, pool := range m.ConnectionPool {
		// Apply filtering if configured
		if w.CollectPoolsMatching != "" && !strings.Contains(strings.ToLower(pool.Name), strings.ToLower(w.CollectPoolsMatching)) {
			continue
		}

		// Check cardinality limit
		if w.MaxConnectionPools > 0 && collected >= w.MaxConnectionPools {
			break
		}

		poolID := cleanName(pool.Name)

		// Track this pool
		w.seenPools[pool.Name] = true

		// Create charts if this is a new pool
		if !w.collectedPools[pool.Name] {
			w.collectedPools[pool.Name] = true
			if err := w.charts.Add(*newConnectionPoolCharts(pool.Name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		mx[fmt.Sprintf("connpool_%s_size", poolID)] = pool.Size
		mx[fmt.Sprintf("connpool_%s_free", poolID)] = pool.Free
		mx[fmt.Sprintf("connpool_%s_max", poolID)] = pool.Max
		mx[fmt.Sprintf("connpool_%s_wait_time_avg", poolID)] = int64(pool.WaitTime * precision)
		mx[fmt.Sprintf("connpool_%s_timeouts", poolID)] = pool.Timeouts

		collected++
	}
}

func (w *WebSphere) collectApplications(mx map[string]int64, m *libertyMetrics) {
	collected := 0

	for _, app := range m.Application {
		// Apply selector if configured
		if w.appSelector != nil && !w.appSelector.MatchString(app.Name) {
			continue
		}

		// Check cardinality limit
		if w.MaxApplications > 0 && collected >= w.MaxApplications {
			break
		}

		appID := cleanName(app.Name)

		// Track this application
		w.seenApps[app.Name] = true

		// Create charts if this is a new application
		if !w.collectedApps[app.Name] {
			w.collectedApps[app.Name] = true
			if err := w.charts.Add(*newApplicationCharts(app.Name)...); err != nil {
				w.Warning(err)
			}
		}

		// Collect metrics
		mx[fmt.Sprintf("app_%s_requests", appID)] = app.Requests
		mx[fmt.Sprintf("app_%s_response_time_avg", appID)] = int64(app.ResponseTime * precision)
		mx[fmt.Sprintf("app_%s_errors", appID)] = app.Errors

		collected++
	}
}

func (w *WebSphere) updateSeenInstances() {
	// Clean up apps that are no longer present
	for app := range w.collectedApps {
		if !w.seenApps[app] {
			delete(w.collectedApps, app)
			// Remove charts for this app
			appID := cleanName(app)
			prefix := fmt.Sprintf("app_%s_", appID)
			for _, chart := range *w.charts {
				if strings.HasPrefix(chart.ID, prefix) {
					chart.MarkRemove()
					chart.MarkNotCreated()
				}
			}
		}
	}

	// Clean up pools that are no longer present
	for pool := range w.collectedPools {
		if !w.seenPools[pool] {
			delete(w.collectedPools, pool)
			// Remove charts for this pool
			poolID := cleanName(pool)
			// Remove thread pool charts
			threadPrefix := fmt.Sprintf("threadpool_%s_", poolID)
			connPrefix := fmt.Sprintf("connpool_%s_", poolID)
			for _, chart := range *w.charts {
				if strings.HasPrefix(chart.ID, threadPrefix) || strings.HasPrefix(chart.ID, connPrefix) {
					chart.MarkRemove()
					chart.MarkNotCreated()
				}
			}
		}
	}

	// Reset seen maps for next collection
	w.seenApps = make(map[string]bool)
	w.seenPools = make(map[string]bool)
}
