// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
	
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
	
	// Special handling for SessionObjectSize as AverageStatistic
	// This metric can appear as both TimeStatistic (container level) and AverageStatistic (app level)
	if prefix == "sessions" && metric.Name == "SessionObjectSize" {
		// Ensure we have a chart for this specific session's object size average metric
		w.ensureSessionObjectSizeAverageChart(cleanInst)
	}
}

// collectDoubleMetric collects a single double metric into the mx map
func (w *WebSpherePMI) collectDoubleMetric(mx map[string]int64, prefix, cleanInst string, metric ExtractedDoubleMetric) {
	mx[fmt.Sprintf("%s_%s_%s", prefix, cleanInst, w.cleanID(metric.Name))] = metric.Value
}

// ensureSessionObjectSizeAverageChart creates a chart for SessionObjectSize as AverageStatistic if it doesn't exist
func (w *WebSpherePMI) ensureSessionObjectSizeAverageChart(cleanInst string) {
	chartID := fmt.Sprintf("sessions_%s_SessionObjectSize_average", cleanInst)
	
	// Check if chart already exists
	if w.Charts().Has(chartID) {
		return
	}
	
	// Create TWO charts for SessionObjectSize average statistics - one for absolute values, one for incremental
	
	// Chart 1: Absolute values (mean, min, max)
	chartAbsolute := &module.Chart{
		ID:       chartID,
		Title:    "Session Object Size",
		Units:    "bytes",
		Fam:      "sessions",
		Ctx:      "websphere_pmi.session_object_size_average",
		Type:     module.Line,
		Priority: prioSessionsActive + 100, // Lower priority than main session charts
		Dims: module.Dims{
			{ID: fmt.Sprintf("sessions_%s_SessionObjectSize_mean", cleanInst), Name: "mean"},
			{ID: fmt.Sprintf("sessions_%s_SessionObjectSize_min", cleanInst), Name: "min"},
			{ID: fmt.Sprintf("sessions_%s_SessionObjectSize_max", cleanInst), Name: "max"},
		},
		Labels: []module.Label{
			{Key: "instance", Value: cleanInst},
		},
	}
	
	// Chart 2: Incremental counters
	chartCounters := &module.Chart{
		ID:       chartID + "_counters",
		Title:    "Session Object Size Counters",
		Units:    "operations/s",
		Fam:      "sessions",
		Ctx:      "websphere_pmi.session_object_size_counters",
		Type:     module.Line,
		Priority: prioSessionsActive + 101, // Lower priority than main session charts
		Dims: module.Dims{
			{ID: fmt.Sprintf("sessions_%s_SessionObjectSize_count", cleanInst), Name: "count", Algo: module.Incremental},
			{ID: fmt.Sprintf("sessions_%s_SessionObjectSize_total", cleanInst), Name: "total", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
			{ID: fmt.Sprintf("sessions_%s_SessionObjectSize_sum_of_squares", cleanInst), Name: "sum_of_squares", Algo: module.Incremental, DimOpts: module.DimOpts{Hidden: true}},
		},
		Labels: []module.Label{
			{Key: "instance", Value: cleanInst},
		},
	}
	
	if err := w.Charts().Add(chartAbsolute); err != nil {
		w.Warning(err)
	}
	
	if err := w.Charts().Add(chartCounters); err != nil {
		w.Warning(err)
	}
}