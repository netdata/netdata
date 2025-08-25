// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// MetricMapping defines the mapping between metric names and their collection keys
type MetricMapping struct {
	MetricName    string
	CollectionKey string
	StatType      string // "count", "time", "range", "bounded_range", "average", "double"
}

// collectMetricsWithLogging is a generic function to collect metrics with comprehensive failure logging
func (w *WebSpherePMI) collectMetricsWithLogging(
	parserName string,
	stat *pmiStat,
	instance string,
	mx map[string]int64,
	mappings []MetricMapping,
) {
	// Track what we expect vs what we find
	expectedMetrics := make(map[string]string) // metric name -> stat type
	foundMetrics := make(map[string]bool)

	// Build expected metrics map
	for _, mapping := range mappings {
		expectedMetrics[mapping.MetricName] = mapping.StatType
	}

	// Process CountStatistics
	for _, cs := range stat.CountStatistics {
		for _, mapping := range mappings {
			if mapping.StatType == "count" && cs.Name == mapping.MetricName {
				if val, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
					mx[mapping.CollectionKey] = val
					foundMetrics[mapping.MetricName] = true
				} else {
					w.Errorf("ISSUE: %s failed to parse CountStatistic %s value '%s': %v",
						parserName, cs.Name, cs.Count, err)
				}
			}
		}
	}

	// Process TimeStatistics
	for _, ts := range stat.TimeStatistics {
		for _, mapping := range mappings {
			if mapping.StatType == "time" && ts.Name == mapping.MetricName {
				total := ts.TotalTime
				if total == "" {
					total = ts.Total
				}
				if val, err := strconv.ParseInt(total, 10, 64); err == nil {
					mx[mapping.CollectionKey] = val
					foundMetrics[mapping.MetricName] = true
				} else {
					w.Errorf("ISSUE: %s failed to parse TimeStatistic %s total value '%s': %v",
						parserName, ts.Name, total, err)
				}
			}
		}
	}

	// Process RangeStatistics
	for _, rs := range stat.RangeStatistics {
		for _, mapping := range mappings {
			if mapping.StatType == "range" && rs.Name == mapping.MetricName {
				if val, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
					mx[mapping.CollectionKey] = val
					foundMetrics[mapping.MetricName] = true
				} else {
					w.Errorf("ISSUE: %s failed to parse RangeStatistic %s current value '%s': %v",
						parserName, rs.Name, rs.Current, err)
				}
			}
		}
	}

	// Process BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		for _, mapping := range mappings {
			if mapping.StatType == "bounded_range" && brs.Name == mapping.MetricName {
				if val, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
					mx[mapping.CollectionKey] = val
					foundMetrics[mapping.MetricName] = true
				} else {
					w.Errorf("ISSUE: %s failed to parse BoundedRangeStatistic %s current value '%s': %v",
						parserName, brs.Name, brs.Current, err)
				}
			}
		}
	}

	// Process AverageStatistics
	for _, as := range stat.AverageStatistics {
		for _, mapping := range mappings {
			if mapping.StatType == "average" && as.Name == mapping.MetricName {
				if val, err := strconv.ParseInt(as.Mean, 10, 64); err == nil {
					mx[mapping.CollectionKey] = val
					foundMetrics[mapping.MetricName] = true
				} else {
					w.Errorf("ISSUE: %s failed to parse AverageStatistic %s mean value '%s': %v",
						parserName, as.Name, as.Mean, err)
				}
			}
		}
	}

	// Process DoubleStatistics
	for _, ds := range stat.DoubleStatistics {
		for _, mapping := range mappings {
			if mapping.StatType == "double" && ds.Name == mapping.MetricName {
				if val, err := strconv.ParseFloat(ds.Double, 64); err == nil {
					mx[mapping.CollectionKey] = int64(val)
					foundMetrics[mapping.MetricName] = true
				} else {
					w.Errorf("ISSUE: %s failed to parse DoubleStatistic %s double value '%s': %v",
						parserName, ds.Name, ds.Double, err)
				}
			}
		}
	}

	// Log detailed failure for missing metrics
	var missing []string
	for metricName, statType := range expectedMetrics {
		if !foundMetrics[metricName] {
			missing = append(missing, fmt.Sprintf("%s(%s)", metricName, statType))
		}
	}

	if len(missing) > 0 {
		w.Errorf("ISSUE: %s failed to find expected metrics in stat '%s'", parserName, stat.Name)
		w.Errorf("  Missing metrics: %v", missing)

		// Log what's actually available
		if len(stat.CountStatistics) > 0 {
			w.Errorf("  Available CountStatistics:")
			for _, cs := range stat.CountStatistics {
				w.Errorf("    - %s (count=%s)", cs.Name, cs.Count)
			}
		}

		if len(stat.TimeStatistics) > 0 {
			w.Errorf("  Available TimeStatistics:")
			for _, ts := range stat.TimeStatistics {
				total := ts.TotalTime
				if total == "" {
					total = ts.Total
				}
				w.Errorf("    - %s (count=%s, total=%s)", ts.Name, ts.Count, total)
			}
		}

		if len(stat.RangeStatistics) > 0 {
			w.Errorf("  Available RangeStatistics:")
			for _, rs := range stat.RangeStatistics {
				w.Errorf("    - %s (current=%s)", rs.Name, rs.Current)
			}
		}

		if len(stat.BoundedRangeStatistics) > 0 {
			w.Errorf("  Available BoundedRangeStatistics:")
			for _, brs := range stat.BoundedRangeStatistics {
				w.Errorf("    - %s (current=%s)", brs.Name, brs.Current)
			}
		}

		if len(stat.AverageStatistics) > 0 {
			w.Errorf("  Available AverageStatistics:")
			for _, as := range stat.AverageStatistics {
				w.Errorf("    - %s (count=%s, mean=%s)", as.Name, as.Count, as.Mean)
			}
		}

		if len(stat.DoubleStatistics) > 0 {
			w.Errorf("  Available DoubleStatistics:")
			for _, ds := range stat.DoubleStatistics {
				w.Errorf("    - %s (double=%s)", ds.Name, ds.Double)
			}
		}

		if len(stat.SubStats) > 0 {
			w.Errorf("  Available SubStats:")
			for _, sub := range stat.SubStats {
				w.Errorf("    - %s", sub.Name)
			}
		}
	}
}

// =============================================================================
// UNIVERSAL METRIC EXTRACTION HELPERS
// =============================================================================
// These 6 helpers extract ALL metrics from each statistic type and return structs.
// Callers control all naming and chart organization decisions.

// Note: precision constant is defined in collect_dynamic.go

// Data structures for extracted metrics
type ExtractedCountMetric struct {
	Name  string
	Value int64
	Unit  string
}

type ExtractedTimeMetric struct {
	Name  string
	Count int64
	Total int64
	Min   int64
	Max   int64
	Mean  int64 // calculated if possible
	Unit  string
}

type ExtractedRangeMetric struct {
	Name          string
	Current       int64
	HighWaterMark int64
	LowWaterMark  int64
	Integral      int64
	Mean          int64
	Unit          string
}

type ExtractedBoundedRangeMetric struct {
	Name          string
	Value         int64
	UpperBound    int64
	LowerBound    int64
	HighWaterMark int64
	LowWaterMark  int64
	Mean          int64
	Integral      int64
	Unit          string
}

type ExtractedAverageMetric struct {
	Name         string
	Count        int64
	Total        int64
	Mean         int64
	Min          int64
	Max          int64
	SumOfSquares int64
	Unit         string
}

type ExtractedDoubleMetric struct {
	Name  string
	Value int64 // with precision applied
	Unit  string
}

// parseIntSafe safely converts string to int64, returning 0 on error
func parseIntSafe(s string) int64 {
	if val, err := strconv.ParseInt(s, 10, 64); err == nil {
		return val
	}
	return 0
}

// parseFloatSafe safely converts string to float64, returning 0.0 on error
func parseFloatSafe(s string) float64 {
	if val, err := strconv.ParseFloat(s, 64); err == nil {
		return val
	}
	return 0.0
}

// =============================================================================
// UNIT CONVERSION
// =============================================================================

// convertUnit converts a value from one unit to another
func convertUnit(value int64, fromUnit, toUnit string) int64 {
	// Normalize units to uppercase for comparison
	from := strings.ToUpper(strings.TrimSpace(fromUnit))
	to := strings.ToUpper(strings.TrimSpace(toUnit))

	// If units are the same, no conversion needed
	if from == to {
		return value
	}

	// Handle time conversions
	if (from == "MILLISECOND" || from == "MILLISECONDS" || from == "MS") && to == "SECONDS" {
		return value / 1000
	}
	if from == "SECOND" && (to == "MILLISECONDS" || to == "MILLISECOND" || to == "MS") {
		return value * 1000
	}

	// Handle memory conversions
	if from == "KILOBYTE" && to == "BYTES" {
		return value * 1024
	}
	if from == "BYTE" && to == "KILOBYTES" {
		return value / 1024
	}

	// No conversion available or needed
	return value
}

// isMemoryMetric checks if a metric name indicates it's a memory-related metric
func isMemoryMetric(name string) bool {
	// Normalize name to lowercase for comparison
	lower := strings.ToLower(name)

	// Check for common memory-related keywords
	return strings.Contains(lower, "memory") ||
		strings.Contains(lower, "heap") ||
		strings.Contains(lower, "heapsize") ||
		strings.Contains(lower, "freememory") ||
		strings.Contains(lower, "usedmemory") ||
		strings.Contains(lower, "allocatedmemory") ||
		strings.Contains(lower, "committedmemory") ||
		strings.Contains(lower, "totalmemory")
}

// calculateWeightedAverage calculates the weighted average for an integral metric
func (w *WebSpherePMI) calculateWeightedAverage(key string, currentIntegral int64) int64 {
	currentTime := time.Now().Unix()

	// Get previous value from cache
	if prev, exists := w.integralCache[key]; exists {
		deltaIntegral := currentIntegral - prev.Value
		deltaTime := currentTime - prev.Timestamp

		// Only calculate if we have a positive time delta and no counter reset
		if deltaTime > 0 && deltaIntegral >= 0 {
			// Calculate weighted average with precision
			// Formula: (deltaIntegral * precision) / deltaTime
			return (deltaIntegral * precision) / deltaTime
		}
	}

	// Update cache with current values
	w.integralCache[key] = &integralCacheEntry{
		Value:     currentIntegral,
		Timestamp: currentTime,
	}

	// Return 0 for first iteration or if counter reset
	return 0
}

// extractCountStatistics extracts ALL CountStatistics and returns structured data
func (w *WebSpherePMI) extractCountStatistics(stats []countStat) []ExtractedCountMetric {
	var results []ExtractedCountMetric

	for _, cs := range stats {
		results = append(results, ExtractedCountMetric{
			Name:  cs.Name,
			Value: parseIntSafe(cs.Count),
			Unit:  cs.Unit,
		})
	}

	w.Debugf("Extracted %d CountStatistics", len(results))
	return results
}

// extractTimeStatistics extracts ALL TimeStatistics and returns structured data
func (w *WebSpherePMI) extractTimeStatistics(stats []timeStat) []ExtractedTimeMetric {
	var results []ExtractedTimeMetric

	for _, ts := range stats {
		// Extract total time (try TotalTime first, then Total)
		total := ts.TotalTime
		if total == "" {
			total = ts.Total
		}

		count := parseIntSafe(ts.Count)
		totalVal := parseIntSafe(total)

		// Calculate mean if possible
		var mean int64
		if count > 0 && totalVal > 0 {
			mean = totalVal / count
		}

		results = append(results, ExtractedTimeMetric{
			Name:  ts.Name,
			Count: count,
			Total: totalVal,
			Min:   parseIntSafe(ts.Min),
			Max:   parseIntSafe(ts.Max),
			Mean:  mean,
			Unit:  ts.Unit,
		})
	}

	w.Debugf("Extracted %d TimeStatistics", len(results))
	return results
}

// extractRangeStatistics extracts ALL RangeStatistics and returns structured data
func (w *WebSpherePMI) extractRangeStatistics(stats []rangeStat) []ExtractedRangeMetric {
	var results []ExtractedRangeMetric

	for _, rs := range stats {
		results = append(results, ExtractedRangeMetric{
			Name:          rs.Name,
			Current:       parseIntSafe(rs.Current),
			HighWaterMark: parseIntSafe(rs.HighWaterMark),
			LowWaterMark:  parseIntSafe(rs.LowWaterMark),
			Integral:      int64(parseFloatSafe(rs.Integral) * precision),
			Mean:          int64(parseFloatSafe(rs.Mean) * precision),
			Unit:          rs.Unit,
		})
	}

	w.Debugf("Extracted %d RangeStatistics", len(results))
	return results
}

// extractBoundedRangeStatistics extracts ALL BoundedRangeStatistics and returns structured data
func (w *WebSpherePMI) extractBoundedRangeStatistics(stats []boundedRangeStat) []ExtractedBoundedRangeMetric {
	var results []ExtractedBoundedRangeMetric

	for _, brs := range stats {
		results = append(results, ExtractedBoundedRangeMetric{
			Name:          brs.Name,
			Value:         parseIntSafe(brs.Current),
			UpperBound:    parseIntSafe(brs.UpperBound),
			LowerBound:    parseIntSafe(brs.LowerBound),
			HighWaterMark: parseIntSafe(brs.HighWaterMark),
			LowWaterMark:  parseIntSafe(brs.LowWaterMark),
			Mean:          int64(parseFloatSafe(brs.Mean) * precision),
			Integral:      int64(parseFloatSafe(brs.Integral) * precision),
			Unit:          brs.Unit,
		})
	}

	w.Debugf("Extracted %d BoundedRangeStatistics", len(results))
	return results
}

// extractAverageStatistics extracts ALL AverageStatistics and returns structured data
func (w *WebSpherePMI) extractAverageStatistics(stats []averageStat) []ExtractedAverageMetric {
	var results []ExtractedAverageMetric

	for _, as := range stats {
		results = append(results, ExtractedAverageMetric{
			Name:         as.Name,
			Count:        parseIntSafe(as.Count),
			Total:        parseIntSafe(as.Total),
			Mean:         int64(parseFloatSafe(as.Mean) * precision),
			Min:          parseIntSafe(as.Min),
			Max:          parseIntSafe(as.Max),
			SumOfSquares: int64(parseFloatSafe(as.SumOfSquares) * precision),
			Unit:         as.Unit,
		})
	}

	w.Debugf("Extracted %d AverageStatistics", len(results))
	return results
}

// extractDoubleStatistics extracts ALL DoubleStatistics and returns structured data
func (w *WebSpherePMI) extractDoubleStatistics(stats []doubleStat) []ExtractedDoubleMetric {
	var results []ExtractedDoubleMetric

	for _, ds := range stats {
		results = append(results, ExtractedDoubleMetric{
			Name:  ds.Name,
			Value: int64(parseFloatSafe(ds.Double) * precision),
			Unit:  ds.Unit,
		})
	}

	w.Debugf("Extracted %d DoubleStatistics", len(results))
	return results
}

// Note: Callers should use the individual extraction functions directly
// to maintain full control over naming and chart organization.

// =============================================================================
// METRIC COLLECTION HELPERS
// =============================================================================
// These helpers collect extracted metrics into the mx map with proper naming.

// collectCountMetric collects a single count metric into the mx map
func (w *WebSpherePMI) collectCountMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedCountMetric) {
	// Apply unit conversion based on the metric name and unit
	value := metric.Value

	// Check if this is a memory-related metric that needs conversion to bytes
	if isMemoryMetric(metric.Name) && metric.Unit != "" {
		value = convertUnit(value, metric.Unit, "BYTES")
	}
	// Time metrics are handled by chart definitions (Mul/Div)
	// Other metrics typically don't need conversion

	mx[fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))] = value
}

// collectTimeMetric collects all time statistic dimensions into the mx map
func (w *WebSpherePMI) collectTimeMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedTimeMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_count"] = metric.Count
	mx[base+"_total"] = metric.Total
	mx[base+"_min"] = metric.Min
	mx[base+"_max"] = metric.Max
	mx[base+"_mean"] = metric.Mean // Always send, even if 0
}

// collectRangeMetric collects all range statistic dimensions into the mx map
func (w *WebSpherePMI) collectRangeMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedRangeMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_current"] = metric.Current
	mx[base+"_high_watermark"] = metric.HighWaterMark
	mx[base+"_low_watermark"] = metric.LowWaterMark
	mx[base+"_integral"] = metric.Integral // Always send, even if 0
	mx[base+"_mean"] = metric.Mean         // Always send, even if 0

	// Calculate weighted average for any metric with integral value
	// This allows RangeStatistics with integral values to have weighted averages,
	// fixing missing weighted averages for ConcurrentHungThreadCount and other metrics
	// Always send the weighted average, even if 0, to avoid data gaps
	metricName := w.cleanID(metric.Name)
	cacheKey := fmt.Sprintf("%s_%s_%s_integral", prefix, cleanInst, metricName)
	weightedAvg := w.calculateWeightedAverage(cacheKey, metric.Integral)
	weightedAvgKey := base + "_weighted_avg"
	mx[weightedAvgKey] = weightedAvg
	if metric.Integral > 0 {
		w.Debugf("RangeStatistic weighted avg: key=%s, value=%d, metric=%s, integral=%d",
			weightedAvgKey, weightedAvg, metric.Name, metric.Integral)
	}
}

// collectBoundedRangeMetric collects all bounded range statistic dimensions into the mx map
func (w *WebSpherePMI) collectBoundedRangeMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedBoundedRangeMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_value"] = metric.Value
	mx[base+"_upper_bound"] = metric.UpperBound
	mx[base+"_lower_bound"] = metric.LowerBound
	mx[base+"_high_watermark"] = metric.HighWaterMark
	mx[base+"_low_watermark"] = metric.LowWaterMark
	mx[base+"_mean"] = metric.Mean // Always send, even if 0

	// Special handling for webapp_container_sessions metrics
	if prefix == "webapp_container_sessions" && (metric.Name == "ActiveCount" || metric.Name == "LiveCount") {
		// Skip integral dimension for ActiveCount and LiveCount
		// These use the split chart design and don't need weighted averages
		return
	}

	mx[base+"_integral"] = metric.Integral // Always send, even if 0

	// Calculate weighted average for any metric with integral value
	// This allows all BoundedRangeStatistics with integral values to have weighted averages
	// regardless of metric name, fixing missing weighted averages for PercentUsed, PercentMaxed,
	// ConcurrentHungThreadCount, HeapSize, and any future metrics
	// Always send the weighted average, even if 0, to avoid data gaps
	metricName := w.cleanID(metric.Name)
	cacheKey := fmt.Sprintf("%s_%s_%s_integral", prefix, cleanInst, metricName)
	weightedAvg := w.calculateWeightedAverage(cacheKey, metric.Integral)
	weightedAvgKey := base + "_weighted_avg"
	mx[weightedAvgKey] = weightedAvg
	if metric.Integral > 0 {
		w.Debugf("BoundedRangeStatistic weighted avg: key=%s, value=%d, metric=%s, integral=%d",
			weightedAvgKey, weightedAvg, metric.Name, metric.Integral)
	}
}

// collectAverageMetric collects all average statistic dimensions into the mx map
// This function serves as a fallback for unknown/rare AverageStatistics.
// Known metrics should be routed to processAverageStatistic for smart processing.
func (w *WebSpherePMI) collectAverageMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedAverageMetric) {
	// NOTE: SessionObjectSize is handled by processAverageStatistic in parseContainerSessionMetrics
	// Add routing for other known AverageStatistics as we discover them in production

	// Default fallback: collect all 6 dimensions for unknown metrics
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_count"] = metric.Count
	mx[base+"_total"] = metric.Total
	mx[base+"_mean"] = metric.Mean
	mx[base+"_min"] = metric.Min
	mx[base+"_max"] = metric.Max
	mx[base+"_sum_of_squares"] = metric.SumOfSquares // Always send, even if 0
}

// collectDoubleMetric collects a single double metric into the mx map
func (w *WebSpherePMI) collectDoubleMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedDoubleMetric) {
	mx[fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))] = metric.Value
}

// =============================================================================
// TIME STATISTIC PROCESSOR
// =============================================================================
// Smart processor for TimeStatistics that creates rate, current latency, and lifetime charts

// getTitlePrefix determines the appropriate title prefix based on context
func getTitlePrefix(context string) string {
	// Remove websphere_pmi. prefix to get the base context
	baseContext := strings.TrimPrefix(context, "websphere_pmi.")

	switch baseContext {
	// JDBC contexts
	case "jdbc", "jdbc_pool":
		return "JDBC "

	// JCA contexts
	case "jca", "jca_pool":
		return "JCA "

	// Thread pool contexts
	case "thread_pool":
		return "Thread Pool "

	// Servlet contexts
	case "servlet", "servlets_component":
		return "Servlet "

	// Web container contexts
	case "webapp_container", "webapp_container_servlets":
		return "Web Container "

	// Session contexts
	case "sessions":
		return "Session "
	case "webapp_container_sessions":
		return "Container Session "

	// Portlet contexts
	case "portlet":
		return "Portlet "

	// Transaction manager contexts
	case "transaction_manager":
		return "Transaction "

	// Security contexts
	case "security_auth", "security_authz":
		return "Security "

	// ORB contexts
	case "orb":
		return "ORB "

	// Web service contexts
	case "pmi_webservice", "web_service":
		return "Web Service "

	// SIB/JMS contexts
	case "sib_jms":
		return "JMS "

	// TCP channel contexts
	case "tcp_dcs":
		return "TCP "

	// Cache contexts
	case "dyna_cache", "object_cache":
		return "Cache "

	// Object pool contexts
	case "object_pool":
		return "Object Pool "

	// HA Manager contexts
	case "ha_manager":
		return "HA Manager "

	// Interceptor contexts
	case "interceptor":
		return "Interceptor "

	// WLM contexts
	case "wlm_tagged":
		return "WLM "

	// EJB contexts
	case "ejb_container":
		return "EJB Container "
	case "generic_ejb":
		return "EJB "
	case "slsb":
		return "Stateless EJB "
	case "sfsb":
		return "Stateful EJB "
	case "entity_bean":
		return "Entity Bean "
	case "bean_manager":
		return "Bean Manager "
	case "mdb":
		return "Message Driven Bean "

	// System/JVM contexts - typically don't need prefix as they're clear
	case "jvm_runtime", "system_data":
		return ""

	// URL contexts - already have good prefixes
	case "servlet_url", "urls":
		return ""

	default:
		// For unknown contexts, return empty string
		return ""
	}
}

// processTimeStatistic handles all aspects of a TimeStatistic metric
func (w *WebSpherePMI) processTimeStatistic(
	context string, // Chart context (e.g., "websphere_pmi.servlet")
	family string, // Chart family (e.g., "web/servlets")
	instance string, // Clean instance name
	labels []module.Label, // Chart labels
	metric ExtractedTimeMetric,
	mx map[string]int64,
	priority int, // Base priority for charts
) {
	// Create cache key for delta calculations
	cacheKey := fmt.Sprintf("%s_%s_%s", context, instance, metric.Name)

	// Extract component prefix from context (e.g., "websphere_pmi.servlet" -> "servlet")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")

	// Note: Rate charts removed as they are redundant with existing CountStatistic counters

	// 1. Always collect lifetime statistics (milliseconds from WebSphere PMI)
	mx[fmt.Sprintf("%s_%s_%s_min", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Min
	mx[fmt.Sprintf("%s_%s_%s_max", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Max
	mx[fmt.Sprintf("%s_%s_%s_mean", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Mean

	// 2. Calculate current latency if we have previous data
	if prev, exists := w.timeStatCache[cacheKey]; exists {
		deltaCount := metric.Count - prev.Count
		deltaTotal := metric.Total - prev.Total

		// Only skip if counter reset detected, otherwise send the data (including zeros)
		if metric.Count >= prev.Count && metric.Total >= prev.Total {
			var currentLatency int64
			if deltaCount > 0 {
				// Use precision to preserve fractional milliseconds in delta calculations
				currentLatency = (deltaTotal * precision) / deltaCount
			} else {
				currentLatency = 0 // No new operations = 0 latency
			}
			mx[fmt.Sprintf("%s_%s_%s_current", componentPrefix, instance, w.cleanID(metric.Name))] = currentLatency
		}
	}

	// 3. Update cache for next iteration
	w.timeStatCache[cacheKey] = &timeStatCacheEntry{
		Count: metric.Count,
		Total: metric.Total,
	}

	// 4. Ensure charts exist
	w.ensureTimeStatCharts(context, family, instance, labels, metric.Name, priority)
}

// ensureTimeStatCharts creates the 2 charts for a TimeStatistic if they don't exist
func (w *WebSpherePMI) ensureTimeStatCharts(
	context string,
	family string,
	instance string,
	labels []module.Label,
	metricName string,
	priority int,
) {
	cleanMetric := w.cleanID(metricName)
	baseChartID := fmt.Sprintf("%s_%s_%s", cleanMetricName(context), instance, cleanMetric)

	// Check if we already created charts for this metric
	chartKey := fmt.Sprintf("timestat_%s", baseChartID)
	if _, exists := w.collectedInstances[chartKey]; exists {
		return
	}
	w.collectedInstances[chartKey] = true

	// Make contexts unique per metric to avoid conflicts (use snake_case)
	snakeCaseMetric := strings.ToLower(regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(metricName, `${1}_${2}`))
	uniqueContext := fmt.Sprintf("%s_%s", context, w.cleanID(snakeCaseMetric))

	// Extract component prefix from context (e.g., "websphere_pmi.servlet" -> "servlet")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")

	// Get title prefix based on context
	titlePrefix := getTitlePrefix(context)

	// Note: Rate chart removed as redundant with existing CountStatistic counters

	// Chart 1: Average Latency (this iteration)
	chartCurrent := &module.Chart{
		ID:       baseChartID + "_current_latency",
		Title:    fmt.Sprintf("%s%s Average Latency", titlePrefix, metricName),
		Units:    "milliseconds",
		Fam:      family,
		Ctx:      uniqueContext + "_current_latency",
		Type:     module.Line,
		Priority: priority + 1,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_current", componentPrefix, instance, cleanMetric), Name: "average", Div: precision},
		},
		Labels: labels,
	}

	// Chart 2: Lifetime Latency Statistics
	chartLifetime := &module.Chart{
		ID:       baseChartID + "_lifetime_latency",
		Title:    fmt.Sprintf("%s%s Lifetime Latency", titlePrefix, metricName),
		Units:    "milliseconds",
		Fam:      family,
		Ctx:      uniqueContext + "_lifetime_latency",
		Type:     module.Line,
		Priority: priority + 2,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_min", componentPrefix, instance, cleanMetric), Name: "min"},
			{ID: fmt.Sprintf("%s_%s_%s_mean", componentPrefix, instance, cleanMetric), Name: "mean"},
			{ID: fmt.Sprintf("%s_%s_%s_max", componentPrefix, instance, cleanMetric), Name: "max"},
		},
		Labels: labels,
	}

	// Add both charts
	if err := w.Charts().Add(chartCurrent); err != nil {
		w.Warning(err)
	}
	if err := w.Charts().Add(chartLifetime); err != nil {
		w.Warning(err)
	}
}

// cleanMetricName removes the "websphere_pmi." prefix if present
func cleanMetricName(name string) string {
	if strings.HasPrefix(name, "websphere_pmi.") {
		return strings.TrimPrefix(name, "websphere_pmi.")
	}
	return name
}

// getContextMetadata returns the appropriate family and priority for a given context
func getContextMetadata(context string) (family string, basePriority int) {
	switch context {
	// System (1000-1999) - Most critical for health monitoring
	case "jvm_runtime":
		return "system/cpu", 1000
	case "system_data":
		return "system/cpu", 1000 // CPU and memory are at same level
	case "thread_pool":
		return "system/threads", 1300

	// Web Container (3000-4999) - Core business logic
	case "webapp":
		return "web/applications", 3300
	case "webapp_container":
		return "web/containers", 3310
	case "webapp_container_servlets":
		return "web/containers", 3310
	case "webapp_container_sessions":
		// This gets routed by getSessionMetricFamily()
		return "web/sessions/container", 3400
	case "webapp_container_portlets", "webapp_container_portlet_requests", "webapp_container_portlet_concurrent":
		return "web/containers", 3333
	case "servlet":
		return "web/servlets/instances", 3200
	case "servlets_component":
		return "web/servlets/components", 3100
	case "servlet_url":
		return "web/servlets/urls", 3120
	case "urls":
		return "web/servlets/container", 3110
	case "sessions":
		// This gets routed by getSessionMetricFamily()
		return "web/sessions/application", 3200
	case "portlet":
		return "web/portlets/instances", 3500
	case "isc_product":
		return "management/isc", 10100
	case "details_component":
		return "management/details", 10100
	case "wim":
		return "management/wim", 10100

	// Connectivity (2000-2999) - Critical for application functionality
	case "jdbc":
		return "connectivity/jdbc", 2000
	case "jca_container":
		return "connectivity/jca/pools", 2200
	case "jca_pool":
		return "connectivity/jca/connections", 2200
	case "sib_jms":
		return "connectivity/jms", 2300
	case "connection_manager":
		return "connectivity/pools", 2300
	case "tcp_dcs":
		return "connectivity/tcp", 2600

	// Transactions (4000-4999) - Important for data integrity
	case "transaction_manager":
		return "transactions", 4000

	// Security (7000-7999) - Authentication and authorization
	case "security_auth":
		return "security/authentication", 7000
	case "security_authz":
		return "security/authorization", 7100
	case "interceptor":
		return "security/interceptors", 7500

	// Integration (6000-6999) - Web services and integration
	case "pmi_webservice":
		return "integration/web_services", 6100
	case "web_service":
		return "integration/web_services", 6100
	case "orb":
		return "integration/orb", 6000
	case "wlm", "wlm_tagged":
		return "integration/wlm", 7500
	case "mdb":
		return "integration/messaging/mdb", 6100
	case "ejb_container":
		return "integration/ejb/container", 6200
	case "bean_manager":
		return "integration/ejb/management", 6200
	case "slsb":
		return "integration/ejb/stateless", 6300
	case "sfsb":
		return "integration/ejb/stateful", 6400
	case "entity_bean":
		return "integration/ejb/entity", 6500
	case "generic_ejb":
		return "integration/ejb/generic", 6600
	case "ejb_method":
		return "middleware/ejb/methods", 6250
	// Note: EJB subfamilies (lifecycle, persistence, session) are handled by getEJBMetricFamily

	// Performance (5000-5999) - Caching and optimization
	case "cache":
		return "performance/dyna_cache", 5200
	case "object_cache":
		return "performance/object_cache", 5300
	case "object_pool":
		return "performance/object_pool", 5100

	// Availability (8000-8999) - HA and clustering
	case "ha_manager":
		return "availability/ha_manager", 8000
	case "enterprise_app":
		return "availability/applications", 8100

	// Management (9000-9999) - Monitoring and diagnostics
	case "extension_registry":
		return "management/registry", 8100
	case "generic_metrics":
		return "management/portlets", 79000

	default:
		// For any unknown context, route to management
		return "management/other", 79000
	}
}

// getSessionMetricFamily determines the appropriate family for session and container metrics based on metric type
func getSessionMetricFamily(contextName, metricName string) string {
	// For webapp_container context, route by metric type to reduce family sizes
	if contextName == "webapp_container" {
		metricLower := strings.ToLower(metricName)
		if strings.Contains(metricLower, "portlet") {
			return "web/portlets/container" // Container-level portlet metrics (aggregated, no portlet label)
		} else if strings.Contains(metricLower, "service") || strings.Contains(metricLower, "response") ||
			strings.Contains(metricLower, "async") {
			return "web/containers/performance" // Performance metrics (ServiceTime, ResponseTime, AsyncContext)
		} else {
			return "web/containers/overview" // Other webapp container metrics
		}
	} else if contextName == "sessions" {
		// All session metrics go to web/sessions/application
		return "web/sessions/application"
	} else if contextName == "webapp_container_sessions" {
		// Split container session metrics to reduce contexts per family
		metricLower := strings.ToLower(metricName)
		if strings.Contains(metricLower, "sessionobjectsize") || strings.Contains(metricLower, "lifetime") {
			return "web/sessions_container/sizes" // Size and lifetime metrics
		} else if strings.Contains(metricLower, "external") {
			return "web/sessions_container/external" // External read/write metrics
		} else if strings.Contains(metricLower, "create") || strings.Contains(metricLower, "invalidate") ||
			strings.Contains(metricLower, "activate") || strings.Contains(metricLower, "time") {
			return "web/sessions_container/lifecycle" // Lifecycle timing metrics
		} else if strings.Contains(metricLower, "active") || strings.Contains(metricLower, "live") ||
			strings.Contains(metricLower, "affinity") || strings.Contains(metricLower, "nopersist") {
			return "web/sessions_container/status" // Status and count metrics
		} else {
			return "web/sessions_container/other" // Any other container session metrics
		}
	}

	// For non-session contexts, use standard family routing
	family, _ := getContextMetadata(contextName)
	return family
}

// getSecurityMetricFamily determines the appropriate family for security metrics
func getSecurityMetricFamily(contextName, metricName string) string {
	metricLower := strings.ToLower(metricName)

	// Security authentication metrics - consolidate into appropriate subcategories
	if contextName == "security_auth" {
		if strings.Contains(metricLower, "jaas") {
			return "security/authentication/jaas" // All JAAS authentication methods
		} else if strings.Contains(metricLower, "web") || strings.Contains(metricLower, "basic") || strings.Contains(metricLower, "token") {
			return "security/authentication/web" // Web-related authentication (web, basic, token)
		} else if strings.Contains(metricLower, "identity") || strings.Contains(metricLower, "credential") || strings.Contains(metricLower, "tai") {
			return "security/authentication/tai" // TAI and credential management
		} else if strings.Contains(metricLower, "rmi") {
			return "security/authentication/rmi" // RMI authentication
		} else {
			return "security/authentication/overview" // Any other authentication metrics
		}
	} else if contextName == "security_authz" {
		// Security authorization metrics - keep simple
		if strings.Contains(metricLower, "web") || strings.Contains(metricLower, "admin") {
			return "security/authorization/web" // Web and admin authorization
		} else if strings.Contains(metricLower, "ejb") || strings.Contains(metricLower, "jacc") {
			return "security/authorization/ejb" // EJB and JACC authorization
		} else {
			return "security/authorization/overview" // Any other authorization metrics
		}
	}

	// Fallback to standard routing
	family, _ := getContextMetadata(contextName)
	return family
}

// getWebServiceMetricFamily determines the appropriate family for webservice metrics
func getWebServiceMetricFamily(contextName, metricName string) string {
	metricLower := strings.ToLower(metricName)

	// Split webservice metrics by operation type
	if contextName == "pmi_webservice" {
		if strings.Contains(metricLower, "request") && !strings.Contains(metricLower, "tairequest") {
			return "integration/webservices/request" // Request-related metrics
		} else if strings.Contains(metricLower, "reply") {
			return "integration/webservices/reply" // Reply-related metrics
		} else if strings.Contains(metricLower, "dispatch") {
			return "integration/webservices/dispatch" // Dispatch-related metrics
		} else if strings.Contains(metricLower, "size") || strings.Contains(metricLower, "total") || strings.Contains(metricLower, "response") {
			return "integration/webservices/performance" // Size and performance metrics
		} else {
			return "integration/webservices/overview" // Any other webservice metrics
		}
	}

	// Fallback to standard routing
	family, _ := getContextMetadata(contextName)
	return family
}

// getTransactionMetricFamily determines the appropriate family for transaction metrics
func getTransactionMetricFamily(contextName, metricName string) string {
	metricLower := strings.ToLower(metricName)

	// Split transaction metrics by scope and operation
	if contextName == "transaction_manager" {
		if strings.Contains(metricLower, "global") {
			return "transactions/global" // Global transaction metrics
		} else if strings.Contains(metricLower, "local") {
			return "transactions/local" // Local transaction metrics
		} else {
			return "transactions/overview" // Overview and status metrics
		}
	}

	// Fallback to standard routing
	family, _ := getContextMetadata(contextName)
	return family
}

// getEJBTimeMetricPriorityOffset returns a consistent priority offset for EJB time metrics
// This ensures the same metric type always gets the same priority across all EJB instances
func getEJBTimeMetricPriorityOffset(metricName string) int {
	metricLower := strings.ToLower(metricName)

	// Assign consistent offsets based on metric type
	switch {
	case strings.Contains(metricLower, "methodresponsetime"):
		return 100
	case strings.Contains(metricLower, "createtime"):
		return 110
	case strings.Contains(metricLower, "removetime"):
		return 120
	case strings.Contains(metricLower, "activationtime"):
		return 130
	case strings.Contains(metricLower, "passivationtime"):
		return 140
	case strings.Contains(metricLower, "loadtime"):
		return 150
	case strings.Contains(metricLower, "storetime"):
		return 160
	case strings.Contains(metricLower, "locktime"):
		return 170
	case strings.Contains(metricLower, "waittime") && !strings.Contains(metricLower, "asyncwaittime"):
		return 180
	case strings.Contains(metricLower, "asyncwaittime"):
		return 190
	default:
		// For unknown metrics, use a hash of the name to get a consistent offset
		// This ensures the same metric always gets the same priority
		hash := 0
		for _, r := range metricName {
			hash = (hash*31 + int(r)) % 100
		}
		return 200 + hash
	}
}

// getEJBMetricFamily determines the appropriate family for EJB metrics
func getEJBMetricFamily(contextName, metricName string) string {
	// For generic_ejb context, all metrics should go to the same family
	// to avoid label inconsistencies since not all beans have the same metrics
	if contextName == "generic_ejb" {
		return "integration/ejb/beans"
	}

	// For EJB container metrics, route to appropriate container families
	if contextName == "ejb_container" {
		metricLower := strings.ToLower(metricName)

		// Session-specific container metrics
		if strings.Contains(metricLower, "session") ||
			strings.Contains(metricLower, "external") {
			return "integration/ejb_container/session"
		}

		// Persistence-specific container metrics
		if strings.Contains(metricLower, "activation") ||
			strings.Contains(metricLower, "passivation") ||
			strings.Contains(metricLower, "load") ||
			strings.Contains(metricLower, "store") ||
			strings.Contains(metricLower, "lock") ||
			strings.Contains(metricLower, "concurrent") {
			return "integration/ejb_container/persistence"
		}

		// Default container metrics
		return "integration/ejb_container/lifecycle"
	}

	// Fallback to standard routing for other EJB contexts
	family, _ := getContextMetadata(contextName)
	return family
}

// getEJBMetricFamilyWithInstance determines the appropriate family for EJB metrics
// considering both metric name and instance/bean information
func getEJBMetricFamilyWithInstance(contextName, metricName, instance string) string {
	// Simply use the context-based routing which handles the separation properly
	return getEJBMetricFamily(contextName, metricName)
}

// processTimeStatisticWithContext is a convenience wrapper that handles context-based metadata
func (w *WebSpherePMI) processTimeStatisticWithContext(
	contextName string,
	instance string,
	labels []module.Label,
	metric ExtractedTimeMetric,
	mx map[string]int64,
	priorityOffset int,
) {
	// Use smart family routing for session, security, webservice, transaction, and EJB metrics
	var family string
	if strings.HasPrefix(contextName, "security_") {
		family = getSecurityMetricFamily(contextName, metric.Name)
	} else if contextName == "pmi_webservice" {
		family = getWebServiceMetricFamily(contextName, metric.Name)
	} else if contextName == "transaction_manager" {
		family = getTransactionMetricFamily(contextName, metric.Name)
	} else if contextName == "generic_ejb" {
		family = getEJBMetricFamilyWithInstance(contextName, metric.Name, instance)
	} else if contextName == "portlet" {
		// For regular portlet contexts, use the standard family mapping
		family, _ = getContextMetadata(contextName)
	} else if contextName == "isc_product" || contextName == "wim" {
		// For ISC and WIM, always use their specific families
		family, _ = getContextMetadata(contextName)
	} else {
		family = getSessionMetricFamily(contextName, metric.Name)
	}
	_, basePriority := getContextMetadata(contextName) // Still get priority from context
	context := fmt.Sprintf("websphere_pmi.%s", contextName)

	w.processTimeStatistic(
		context,
		family,
		instance,
		labels,
		metric,
		mx,
		basePriority+priorityOffset,
	)
}

// =============================================================================
// AVERAGE STATISTIC PROCESSOR
// =============================================================================
// Smart processor for AverageStatistics that creates rate, current avg, current stddev, and lifetime charts

// processAverageStatistic handles all aspects of an AverageStatistic metric
func (w *WebSpherePMI) processAverageStatistic(
	context string, // Chart context (e.g., "websphere_pmi.session")
	family string, // Chart family (e.g., "web/sessions")
	instance string, // Clean instance name
	labels []module.Label, // Chart labels
	metric ExtractedAverageMetric,
	mx map[string]int64,
	priority int, // Base priority for charts
	units string, // Units for the metric (e.g., "bytes", "milliseconds", "count")
) {
	// Create cache key for delta calculations
	cacheKey := fmt.Sprintf("%s_%s_%s", context, instance, metric.Name)

	// Extract component prefix from context (e.g., "websphere_pmi.session" -> "session")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")

	// Note: Rate charts removed as they are redundant with existing CountStatistic counters

	// 1. Always collect lifetime statistics
	mx[fmt.Sprintf("%s_%s_%s_lifetime_min", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Min
	mx[fmt.Sprintf("%s_%s_%s_lifetime_max", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Max
	mx[fmt.Sprintf("%s_%s_%s_lifetime_mean", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Mean

	// 2. Calculate current average and stddev if we have previous data
	if prev, exists := w.avgStatCache[cacheKey]; exists {
		deltaCount := metric.Count - prev.Count
		deltaTotal := metric.Total - prev.Total
		deltaSumOfSquares := metric.SumOfSquares - prev.SumOfSquares

		// Only process if no counter reset detected
		if metric.Count >= prev.Count && metric.Total >= prev.Total && metric.SumOfSquares >= prev.SumOfSquares {
			var currentAvg int64
			if deltaCount > 0 {
				currentAvg = deltaTotal / deltaCount
			} else {
				currentAvg = 0 // No new operations = 0 average
			}
			mx[fmt.Sprintf("%s_%s_%s_current_avg", componentPrefix, instance, w.cleanID(metric.Name))] = currentAvg

			// Calculate current standard deviation
			var currentStdDev int64
			if deltaCount > 1 && deltaSumOfSquares > 0 {
				// variance = (sum_of_squares/count) - mean²
				// Note: values are already multiplied by precision
				meanSquared := (currentAvg * currentAvg) / precision
				variance := (deltaSumOfSquares / deltaCount) - meanSquared

				if variance > 0 {
					// Simple integer square root approximation
					currentStdDev = w.intSqrt(variance)
				} else {
					currentStdDev = 0
				}
			} else {
				currentStdDev = 0 // Not enough data for stddev
			}
			mx[fmt.Sprintf("%s_%s_%s_current_stddev", componentPrefix, instance, w.cleanID(metric.Name))] = currentStdDev
		}
	} else {
		// First collection - set current values to 0
		mx[fmt.Sprintf("%s_%s_%s_current_avg", componentPrefix, instance, w.cleanID(metric.Name))] = 0
		mx[fmt.Sprintf("%s_%s_%s_current_stddev", componentPrefix, instance, w.cleanID(metric.Name))] = 0
	}

	// 3. Update cache for next iteration
	w.avgStatCache[cacheKey] = &avgStatCacheEntry{
		Count:        metric.Count,
		Total:        metric.Total,
		SumOfSquares: metric.SumOfSquares,
	}

	// 4. Ensure charts exist
	w.ensureAvgStatCharts(context, family, instance, labels, metric.Name, priority, units)
}

// intSqrt calculates integer square root using Newton's method
func (w *WebSpherePMI) intSqrt(x int64) int64 {
	if x <= 0 {
		return 0
	}

	// Initial guess
	guess := x / 2
	if guess == 0 {
		guess = 1
	}

	// Newton's method: next = (guess + x/guess) / 2
	for i := 0; i < 10; i++ { // Limit iterations
		next := (guess + x/guess) / 2
		if next == guess || next == guess-1 || next == guess+1 {
			return next
		}
		guess = next
	}

	return guess
}

// ensureAvgStatCharts creates the 3 charts for an AverageStatistic if they don't exist
func (w *WebSpherePMI) ensureAvgStatCharts(
	context string,
	family string,
	instance string,
	labels []module.Label,
	metricName string,
	priority int,
	units string,
) {
	cleanMetric := w.cleanID(metricName)
	baseChartID := fmt.Sprintf("%s_%s_%s", cleanMetricName(context), instance, cleanMetric)

	// Check if we already created charts for this metric
	chartKey := fmt.Sprintf("avgstat_%s", baseChartID)
	if _, exists := w.collectedInstances[chartKey]; exists {
		return
	}
	w.collectedInstances[chartKey] = true

	// Make contexts unique per metric to avoid conflicts (use snake_case)
	snakeCaseMetric := strings.ToLower(regexp.MustCompile(`([a-z0-9])([A-Z])`).ReplaceAllString(metricName, `${1}_${2}`))
	uniqueContext := fmt.Sprintf("%s_%s", context, w.cleanID(snakeCaseMetric))

	// Extract component prefix from context (e.g., "websphere_pmi.session" -> "session")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")

	// Get title prefix based on context
	titlePrefix := getTitlePrefix(context)

	// Note: Rate chart removed as redundant with existing CountStatistic counters

	// Chart 1: Current Average (this iteration)
	chartCurrentAvg := &module.Chart{
		ID:       baseChartID + "_current_avg",
		Title:    fmt.Sprintf("%s%s Current Average", titlePrefix, metricName),
		Units:    units,
		Fam:      family,
		Ctx:      uniqueContext + "_current_avg",
		Type:     module.Line,
		Priority: priority + 1,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_current_avg", componentPrefix, instance, cleanMetric), Name: "average", Div: precision},
		},
		Labels: labels,
	}

	// Chart 2: Current Standard Deviation (this iteration)
	chartCurrentStdDev := &module.Chart{
		ID:       baseChartID + "_current_stddev",
		Title:    fmt.Sprintf("%s%s Current Standard Deviation", titlePrefix, metricName),
		Units:    units,
		Fam:      family,
		Ctx:      uniqueContext + "_current_stddev",
		Type:     module.Line,
		Priority: priority + 2,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_current_stddev", componentPrefix, instance, cleanMetric), Name: "stddev", Div: precision},
		},
		Labels: labels,
	}

	// Chart 3: Lifetime Statistics
	chartLifetime := &module.Chart{
		ID:       baseChartID + "_lifetime",
		Title:    fmt.Sprintf("%s%s Lifetime Statistics", titlePrefix, metricName),
		Units:    units,
		Fam:      family,
		Ctx:      uniqueContext + "_lifetime",
		Type:     module.Line,
		Priority: priority + 3,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_lifetime_min", componentPrefix, instance, cleanMetric), Name: "min", Div: precision},
			{ID: fmt.Sprintf("%s_%s_%s_lifetime_mean", componentPrefix, instance, cleanMetric), Name: "mean", Div: precision},
			{ID: fmt.Sprintf("%s_%s_%s_lifetime_max", componentPrefix, instance, cleanMetric), Name: "max", Div: precision},
		},
		Labels: labels,
	}

	// Add all three charts
	if err := w.Charts().Add(chartCurrentAvg); err != nil {
		w.Warning(err)
	}
	if err := w.Charts().Add(chartCurrentStdDev); err != nil {
		w.Warning(err)
	}
	if err := w.Charts().Add(chartLifetime); err != nil {
		w.Warning(err)
	}
}

// processAverageStatisticWithContext is a convenience wrapper that handles context-based metadata
func (w *WebSpherePMI) processAverageStatisticWithContext(
	contextName string,
	instance string,
	labels []module.Label,
	metric ExtractedAverageMetric,
	mx map[string]int64,
	priorityOffset int,
	units string,
) {
	// Use smart family routing for session, security, webservice, transaction, and EJB metrics
	var family string
	if strings.HasPrefix(contextName, "security_") {
		family = getSecurityMetricFamily(contextName, metric.Name)
	} else if contextName == "pmi_webservice" {
		family = getWebServiceMetricFamily(contextName, metric.Name)
	} else if contextName == "transaction_manager" {
		family = getTransactionMetricFamily(contextName, metric.Name)
	} else if contextName == "generic_ejb" {
		family = getEJBMetricFamilyWithInstance(contextName, metric.Name, instance)
	} else if contextName == "portlet" {
		// For regular portlet contexts, use the standard family mapping
		family, _ = getContextMetadata(contextName)
	} else if contextName == "isc_product" || contextName == "wim" {
		// For ISC and WIM, always use their specific families
		family, _ = getContextMetadata(contextName)
	} else {
		family = getSessionMetricFamily(contextName, metric.Name)
	}
	_, basePriority := getContextMetadata(contextName) // Still get priority from context
	context := fmt.Sprintf("websphere_pmi.%s", contextName)

	w.processAverageStatistic(
		context,
		family,
		instance,
		labels,
		metric,
		mx,
		basePriority+priorityOffset,
		units,
	)
}
