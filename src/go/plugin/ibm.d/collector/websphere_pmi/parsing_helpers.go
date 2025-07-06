// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
	"strings"
	
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// MetricMapping defines the mapping between metric names and their collection keys
type MetricMapping struct {
	MetricName   string
	CollectionKey string
	StatType     string // "count", "time", "range", "bounded_range", "average", "double"
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
}

type ExtractedTimeMetric struct {
	Name  string
	Count int64
	Total int64
	Min   int64
	Max   int64
	Mean  int64 // calculated if possible
}

type ExtractedRangeMetric struct {
	Name          string
	Current       int64
	HighWaterMark int64
	LowWaterMark  int64
	Integral      int64
	Mean          int64
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
}

type ExtractedAverageMetric struct {
	Name         string
	Count        int64
	Total        int64
	Mean         int64
	Min          int64
	Max          int64
	SumOfSquares int64
}

type ExtractedDoubleMetric struct {
	Name  string
	Value int64 // with precision applied
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

// extractCountStatistics extracts ALL CountStatistics and returns structured data
func (w *WebSpherePMI) extractCountStatistics(stats []countStat) []ExtractedCountMetric {
	var results []ExtractedCountMetric
	
	for _, cs := range stats {
		results = append(results, ExtractedCountMetric{
			Name:  cs.Name,
			Value: parseIntSafe(cs.Count),
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
	mx[fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))] = metric.Value
}

// collectTimeMetric collects all time statistic dimensions into the mx map
func (w *WebSpherePMI) collectTimeMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedTimeMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_count"] = metric.Count
	mx[base+"_total"] = metric.Total
	mx[base+"_min"] = metric.Min
	mx[base+"_max"] = metric.Max
	mx[base+"_mean"] = metric.Mean  // Always send, even if 0
}

// collectRangeMetric collects all range statistic dimensions into the mx map
func (w *WebSpherePMI) collectRangeMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedRangeMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_current"] = metric.Current
	mx[base+"_high_watermark"] = metric.HighWaterMark
	mx[base+"_low_watermark"] = metric.LowWaterMark
	mx[base+"_integral"] = metric.Integral  // Always send, even if 0
	mx[base+"_mean"] = metric.Mean          // Always send, even if 0
}

// collectBoundedRangeMetric collects all bounded range statistic dimensions into the mx map
func (w *WebSpherePMI) collectBoundedRangeMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedBoundedRangeMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_value"] = metric.Value
	mx[base+"_upper_bound"] = metric.UpperBound
	mx[base+"_lower_bound"] = metric.LowerBound
	mx[base+"_high_watermark"] = metric.HighWaterMark
	mx[base+"_low_watermark"] = metric.LowWaterMark
	mx[base+"_mean"] = metric.Mean          // Always send, even if 0
	mx[base+"_integral"] = metric.Integral  // Always send, even if 0
}

// collectAverageMetric collects all average statistic dimensions into the mx map
func (w *WebSpherePMI) collectAverageMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedAverageMetric) {
	base := fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))
	mx[base+"_count"] = metric.Count
	mx[base+"_total"] = metric.Total
	mx[base+"_mean"] = metric.Mean
	mx[base+"_min"] = metric.Min
	mx[base+"_max"] = metric.Max
	mx[base+"_sum_of_squares"] = metric.SumOfSquares  // Always send, even if 0
}

// collectDoubleMetric collects a single double metric into the mx map
func (w *WebSpherePMI) collectDoubleMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedDoubleMetric) {
	mx[fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))] = metric.Value
}


// =============================================================================
// TIME STATISTIC PROCESSOR
// =============================================================================
// Smart processor for TimeStatistics that creates rate, current latency, and lifetime charts

// processTimeStatistic handles all aspects of a TimeStatistic metric
func (w *WebSpherePMI) processTimeStatistic(
	context string,     // Chart context (e.g., "websphere_pmi.servlet")
	family string,      // Chart family (e.g., "web/servlets")
	instance string,    // Clean instance name
	labels []module.Label, // Chart labels
	metric ExtractedTimeMetric,
	mx map[string]int64,
	priority int,       // Base priority for charts
) {
	// Create cache key for delta calculations
	cacheKey := fmt.Sprintf("%s_%s_%s", context, instance, metric.Name)
	
	// Extract component prefix from context (e.g., "websphere_pmi.servlet" -> "servlet")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")
	
	// 1. Always collect rate (operations/s)
	mx[fmt.Sprintf("%s_%s_%s_operations", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Count
	
	// 2. Always collect lifetime statistics (raw nanoseconds - Netdata will auto-scale)
	mx[fmt.Sprintf("%s_%s_%s_min", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Min
	mx[fmt.Sprintf("%s_%s_%s_max", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Max
	mx[fmt.Sprintf("%s_%s_%s_mean", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Mean
	
	// 3. Calculate current latency if we have previous data
	if prev, exists := w.timeStatCache[cacheKey]; exists {
		deltaCount := metric.Count - prev.Count
		deltaTotal := metric.Total - prev.Total
		
		// Only skip if counter reset detected, otherwise send the data (including zeros)
		if metric.Count >= prev.Count && metric.Total >= prev.Total {
			var currentLatency int64
			if deltaCount > 0 {
				currentLatency = deltaTotal / deltaCount
			} else {
				currentLatency = 0 // No new operations = 0 latency
			}
			mx[fmt.Sprintf("%s_%s_%s_current", componentPrefix, instance, w.cleanID(metric.Name))] = currentLatency
		}
	}
	
	// 4. Update cache for next iteration
	w.timeStatCache[cacheKey] = &timeStatCacheEntry{
		Count: metric.Count,
		Total: metric.Total,
	}
	
	// 5. Ensure charts exist
	w.ensureTimeStatCharts(context, family, instance, labels, metric.Name, priority)
}

// ensureTimeStatCharts creates the 3 charts for a TimeStatistic if they don't exist
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
	
	// Make contexts unique per metric to avoid conflicts
	uniqueContext := fmt.Sprintf("%s_%s", context, w.cleanID(metricName))
	
	// Extract component prefix from context (e.g., "websphere_pmi.servlet" -> "servlet")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")
	
	// Chart 1: Operation Rate
	chartRate := &module.Chart{
		ID:       baseChartID + "_rate",
		Title:    fmt.Sprintf("%s Rate", metricName),
		Units:    "operations/s",
		Fam:      family,
		Ctx:      uniqueContext + "_rate",
		Type:     module.Line,
		Priority: priority,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_operations", componentPrefix, instance, cleanMetric), Name: "operations", Algo: module.Incremental},
		},
		Labels: labels,
	}
	
	// Chart 2: Current Latency (this iteration)
	chartCurrent := &module.Chart{
		ID:       baseChartID + "_current_latency",
		Title:    fmt.Sprintf("%s Current Latency", metricName),
		Units:    "nanoseconds",
		Fam:      family,
		Ctx:      uniqueContext + "_current_latency",
		Type:     module.Line,
		Priority: priority + 1,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_current", componentPrefix, instance, cleanMetric), Name: "current"},
		},
		Labels: labels,
	}
	
	// Chart 3: Lifetime Latency Statistics
	chartLifetime := &module.Chart{
		ID:       baseChartID + "_lifetime_latency",
		Title:    fmt.Sprintf("%s Lifetime Latency", metricName),
		Units:    "nanoseconds",
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
	
	// Add all three charts
	if err := w.Charts().Add(chartRate); err != nil {
		w.Warning(err)
	}
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
	case "transaction_manager":
		return "transactions", prioTransactionManager
	case "jvm_runtime":
		return "jvm", prioSystemJVM
	case "thread_pool":
		return "threadpools", prioThreadPools
	case "jdbc":
		return "jdbc", prioJDBCPools
	case "jca_pool":
		return "jca", prioJCAPools
	case "webapp_container", "webapp":
		return "web/apps", prioWebApps
	case "servlet":
		return "web/servlets", prioServlets
	case "sessions":
		return "web/sessions", prioSessions
	case "cache":
		return "cache", prioCacheManager
	case "object_cache":
		return "object_cache", prioObjectCache
	case "orb":
		return "orb", prioORB
	case "enterprise_app":
		return "apps", prioEnterpriseApps
	case "ejb_container":
		return "ejb", prioEJBContainer
	case "portlet":
		return "web/portlets", prioPortlets
	case "web_service":
		return "webservices", prioWebServices
	case "sib_jms":
		return "jms", prioJMSAdapter
	case "security_auth", "security_authz":
		return "security", prioSecurity
	case "wlm":
		return "wlm", prioWLM
	case "system_data":
		return "system", prioSystemData
	case "connection_manager":
		return "connections", prioConnectionManager
	case "bean_manager":
		return "ejb", prioEJBContainer
	default:
		// For any unknown context, use a generic family and priority
		return "other", 9000
	}
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
	family, basePriority := getContextMetadata(contextName)
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
	context string,     // Chart context (e.g., "websphere_pmi.session")
	family string,      // Chart family (e.g., "web/sessions")
	instance string,    // Clean instance name
	labels []module.Label, // Chart labels
	metric ExtractedAverageMetric,
	mx map[string]int64,
	priority int,       // Base priority for charts
	units string,       // Units for the metric (e.g., "bytes", "milliseconds", "count")
) {
	// Create cache key for delta calculations
	cacheKey := fmt.Sprintf("%s_%s_%s", context, instance, metric.Name)
	
	// Extract component prefix from context (e.g., "websphere_pmi.session" -> "session")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")
	
	// 1. Always collect rate (operations/s)
	mx[fmt.Sprintf("%s_%s_%s_operations", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Count
	
	// 2. Always collect lifetime statistics
	mx[fmt.Sprintf("%s_%s_%s_lifetime_min", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Min
	mx[fmt.Sprintf("%s_%s_%s_lifetime_max", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Max
	mx[fmt.Sprintf("%s_%s_%s_lifetime_mean", componentPrefix, instance, w.cleanID(metric.Name))] = metric.Mean
	
	// 3. Calculate current average and stddev if we have previous data
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
	
	// 4. Update cache for next iteration
	w.avgStatCache[cacheKey] = &avgStatCacheEntry{
		Count:        metric.Count,
		Total:        metric.Total,
		SumOfSquares: metric.SumOfSquares,
	}
	
	// 5. Ensure charts exist
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

// ensureAvgStatCharts creates the 4 charts for an AverageStatistic if they don't exist
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
	
	// Make contexts unique per metric to avoid conflicts
	uniqueContext := fmt.Sprintf("%s_%s", context, w.cleanID(metricName))
	
	// Extract component prefix from context (e.g., "websphere_pmi.session" -> "session")
	componentPrefix := strings.TrimPrefix(context, "websphere_pmi.")
	
	// Chart 1: Operation Rate
	chartRate := &module.Chart{
		ID:       baseChartID + "_rate",
		Title:    fmt.Sprintf("%s Rate", metricName),
		Units:    "operations/s",
		Fam:      family,
		Ctx:      uniqueContext + "_rate",
		Type:     module.Line,
		Priority: priority,
		Dims: module.Dims{
			{ID: fmt.Sprintf("%s_%s_%s_operations", componentPrefix, instance, cleanMetric), Name: "operations", Algo: module.Incremental},
		},
		Labels: labels,
	}
	
	// Chart 2: Current Average (this iteration)
	chartCurrentAvg := &module.Chart{
		ID:       baseChartID + "_current_avg",
		Title:    fmt.Sprintf("%s Current Average", metricName),
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
	
	// Chart 3: Current Standard Deviation (this iteration)
	chartCurrentStdDev := &module.Chart{
		ID:       baseChartID + "_current_stddev",
		Title:    fmt.Sprintf("%s Current Standard Deviation", metricName),
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
	
	// Chart 4: Lifetime Statistics
	chartLifetime := &module.Chart{
		ID:       baseChartID + "_lifetime",
		Title:    fmt.Sprintf("%s Lifetime Statistics", metricName),
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
	
	// Add all four charts
	if err := w.Charts().Add(chartRate); err != nil {
		w.Warning(err)
	}
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
	family, basePriority := getContextMetadata(contextName)
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