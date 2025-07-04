// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
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