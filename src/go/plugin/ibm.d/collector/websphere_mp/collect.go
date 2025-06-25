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
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

const precision = 1000 // Precision multiplier for floating-point values

// Prometheus metric line pattern
var promMetricPattern = regexp.MustCompile(`^([a-zA-Z_:][a-zA-Z0-9_:]*(?:\{[^}]*\})?)?\s+([+-]?[0-9]*\.?[0-9]+(?:[eE][+-]?[0-9]+)?)\s*$`)

func (w *WebSphereMicroProfile) collect(ctx context.Context) (map[string]int64, error) {
	if w.charts == nil {
		w.initCharts()
	}

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

	return mx, nil
}

func (w *WebSphereMicroProfile) collectMicroProfileMetrics(ctx context.Context) (map[string]float64, error) {
	u, err := url.Parse(w.HTTPConfig.RequestConfig.URL)
	if err != nil {
		return nil, err
	}

	u.Path = w.MetricsEndpoint
	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil)
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
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("unexpected status code: %d, body: %s", resp.StatusCode, string(body))
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
			w.Debugf("failed to parse metric value '%s': %v", valueStr, err)
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
	jvmCount := 0
	restCount := 0
	mpCount := 0
	customCount := 0

	for metricName, value := range metrics {
		// Convert float to int64 with precision
		intValue := int64(value * precision)
		
		// Categorize and process metrics
		if w.CollectJVMMetrics && w.jvmPattern.MatchString(metricName) {
			w.processJVMMetric(mx, metricName, intValue)
		} else if w.CollectRESTMetrics && w.restPattern.MatchString(metricName) {
			if w.MaxRESTEndpoints == 0 || restCount < w.MaxRESTEndpoints {
				if w.restSelector == nil || w.restSelector.MatchString(metricName) {
					w.processRESTMetric(mx, metricName, intValue)
					restCount++
				}
			}
		} else if w.CollectMPMetrics && w.mpPattern.MatchString(metricName) {
			w.processMPMetric(mx, metricName, intValue)
			mpCount++
		} else if w.CollectCustomMetrics && w.customPattern.MatchString(metricName) {
			if w.MaxCustomMetrics == 0 || customCount < w.MaxCustomMetrics {
				if w.customSelector == nil || w.customSelector.MatchString(metricName) {
					w.processCustomMetric(mx, metricName, intValue)
					customCount++
				}
			}
		}
	}
}

func (w *WebSphereMicroProfile) processJVMMetric(mx map[string]int64, metricName string, value int64) {
	// Handle JVM metrics
	cleanedName := cleanName(metricName)
	mx[cleanedName] = value
	
	// Track for dynamic chart creation
	w.seenMetrics[cleanedName] = true
	
	if !w.collectedMetrics[cleanedName] {
		w.collectedMetrics[cleanedName] = true
		// Add JVM charts dynamically if needed
		if charts := w.createJVMChart(cleanedName, metricName); charts != nil {
			if err := w.charts.Add(*charts...); err != nil {
				w.Warning(err)
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
				w.Warning(err)
			}
		}
	}
}

func (w *WebSphereMicroProfile) processMPMetric(mx map[string]int64, metricName string, value int64) {
	// Handle MicroProfile-specific metrics
	cleanedName := cleanName(metricName)
	mx[cleanedName] = value
	
	// Track for dynamic chart creation
	w.seenMetrics[cleanedName] = true
	
	if !w.collectedMetrics[cleanedName] {
		w.collectedMetrics[cleanedName] = true
		// Add MicroProfile charts dynamically
		if charts := w.createMPChart(cleanedName, metricName); charts != nil {
			if err := w.charts.Add(*charts...); err != nil {
				w.Warning(err)
			}
		}
	}
}

func (w *WebSphereMicroProfile) processCustomMetric(mx map[string]int64, metricName string, value int64) {
	// Handle custom application metrics
	cleanedName := cleanName(metricName)
	mx[cleanedName] = value
	
	// Track for dynamic chart creation
	w.seenMetrics[cleanedName] = true
	
	if !w.collectedMetrics[cleanedName] {
		w.collectedMetrics[cleanedName] = true
		// Add custom charts dynamically
		if charts := w.createCustomChart(cleanedName, metricName); charts != nil {
			if err := w.charts.Add(*charts...); err != nil {
				w.Warning(err)
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