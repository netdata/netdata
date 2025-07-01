// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strconv"
	"strings"
)

// MetricTuple represents a flattened metric with full context and labels
type MetricTuple struct {
	Path   string            // Hierarchical path (e.g., "server/Thread Pools/WebContainer/ActiveCount")
	Labels map[string]string // All inherited labels (node, server, category, instance, etc.)
	Value  int64             // Metric value
	Unit   string            // Metric unit (requests, bytes, milliseconds, etc.)
	Type   string            // Metric type (count, time, bounded_range, range, double)
	
	// New fields for fully unique identification
	UniqueContext  string    // Fully unique context including metric name
	UniqueInstance string    // Fully unique instance including metric name
}

// ArrayInfo represents detected array structures
type ArrayInfo struct {
	Path     string   // Path to the array container
	Elements []string // Names of array elements
	Labels   map[string]string // Inherited labels
}

// FlattenerResult contains all flattened data
type FlattenerResult struct {
	Metrics []MetricTuple
	Arrays  []ArrayInfo
}

// XMLFlattener converts hierarchical WebSphere PMI XML to flat metric tuples
type XMLFlattener struct {
	// Configuration
	arrayDetectionThreshold int // Minimum similar items to consider as array (default: 2)
}

// NewXMLFlattener creates a new flattener instance
func NewXMLFlattener() *XMLFlattener {
	return &XMLFlattener{
		arrayDetectionThreshold: 2,
	}
}

// FlattenPMIStats flattens the entire PMI stats structure
func (f *XMLFlattener) FlattenPMIStats(stats *pmiStatsResponse) *FlattenerResult {
	result := &FlattenerResult{
		Metrics: make([]MetricTuple, 0),
		Arrays:  make([]ArrayInfo, 0),
	}

	// Process each node
	for _, node := range stats.Nodes {
		nodeLabels := map[string]string{
			"node": node.Name,
		}
		
		// Process each server
		for _, server := range node.Servers {
			serverLabels := copyLabels(nodeLabels)
			serverLabels["server"] = server.Name
			
			// Process server stats
			for _, stat := range server.Stats {
				f.flattenStat(&stat, "", serverLabels, result)
			}
		}
	}

	// Process direct stats (if any)
	for _, stat := range stats.Stats {
		f.flattenStat(&stat, "", map[string]string{}, result)
	}

	return result
}

// flattenStat recursively flattens a stat and its substats
func (f *XMLFlattener) flattenStat(stat *pmiStat, parentPath string, parentLabels map[string]string, result *FlattenerResult) {
	// Build current path
	currentPath := stat.Name
	if parentPath != "" {
		currentPath = parentPath + "/" + stat.Name
	}

	// Inherit parent labels
	currentLabels := copyLabels(parentLabels)

	// Add context-specific labels based on path
	f.addContextLabels(currentPath, stat.Name, currentLabels)

	// Check if this is an array (multiple similar substats)
	if f.isArray(stat) {
		arrayInfo := ArrayInfo{
			Path:     currentPath,
			Elements: make([]string, len(stat.SubStats)),
			Labels:   copyLabels(currentLabels),
		}
		
		for i, subStat := range stat.SubStats {
			arrayInfo.Elements[i] = subStat.Name
		}
		result.Arrays = append(result.Arrays, arrayInfo)

		// Process array elements with instance labels
		for i, subStat := range stat.SubStats {
			elementLabels := copyLabels(currentLabels)
			f.addArrayElementLabels(currentPath, subStat.Name, i, elementLabels)
			f.flattenStat(&subStat, currentPath, elementLabels, result)
		}
	} else {
		// Process direct metrics
		f.extractMetrics(stat, currentPath, currentLabels, result)

		// Process substats
		for _, subStat := range stat.SubStats {
			f.flattenStat(&subStat, currentPath, currentLabels, result)
		}
	}
}

// isArray determines if a stat represents an array of similar items
func (f *XMLFlattener) isArray(stat *pmiStat) bool {
	if len(stat.SubStats) < f.arrayDetectionThreshold {
		return false
	}

	// Check if all substats have similar structure (same metric types)
	if len(stat.SubStats) == 0 {
		return false
	}

	// Use first substat as template
	template := &stat.SubStats[0]
	templateSig := f.getStatSignature(template)

	// Check if all other substats have similar signature
	for i := 1; i < len(stat.SubStats); i++ {
		if f.getStatSignature(&stat.SubStats[i]) != templateSig {
			return false
		}
	}

	return true
}

// getStatSignature returns a signature string representing the structure of a stat
func (f *XMLFlattener) getStatSignature(stat *pmiStat) string {
	parts := []string{
		fmt.Sprintf("count:%d", len(stat.CountStatistics)),
		fmt.Sprintf("time:%d", len(stat.TimeStatistics)),
		fmt.Sprintf("bounded:%d", len(stat.BoundedRangeStatistics)),
		fmt.Sprintf("range:%d", len(stat.RangeStatistics)),
		fmt.Sprintf("double:%d", len(stat.DoubleStatistics)),
		fmt.Sprintf("substats:%d", len(stat.SubStats)),
	}
	return strings.Join(parts, ",")
}

// addContextLabels adds labels based on the hierarchical context
func (f *XMLFlattener) addContextLabels(path, name string, labels map[string]string) {
	pathParts := strings.Split(path, "/")
	
	// Skip 'server' from the path to get natural hierarchy
	cleanParts := []string{}
	for _, part := range pathParts {
		if part != "server" && part != "" {
			cleanParts = append(cleanParts, part)
		}
	}
	
	// The first non-server part becomes the category
	if len(cleanParts) > 0 {
		// Use the first level as category (normalized)
		labels["category"] = f.normalizeForLabel(cleanParts[0])
	}
	
	// For specific patterns, add instance labels
	for i, part := range pathParts {
		switch part {
		case "Servlets":
			labels["subcategory"] = "servlets"
		case "Portlets":
			labels["subcategory"] = "portlets"
		case "server":
			// Skip the server level entirely
			continue
		default:
			// For provider names, app names, etc.
			if i > 0 {
				prevPart := pathParts[i-1]
				switch prevPart {
				case "JDBC Connection Pools":
					labels["provider"] = part
					labels["instance"] = part
				case "JCA Connection Pools":
					labels["adapter"] = part
					labels["instance"] = part
				case "Web Applications":
					labels["app"] = part
					labels["instance"] = part
				case "Thread Pools":
					labels["pool"] = part
					labels["instance"] = part
				case "Servlets":
					labels["servlet"] = part
				case "Portlets":
					labels["portlet"] = part
				case "Dynamic Caching":
					// For Dynamic Caching, the full object path is the instance
					// e.g., "Object: ws/com.ibm.workplace/ExtensionRegistryCache"
					if strings.HasPrefix(part, "Object:") && i+1 < len(pathParts) {
						// Collect the full object path
						fullObjectPath := part
						for j := i + 1; j < len(pathParts); j++ {
							nextPart := pathParts[j]
							// Stop at known subcomponents
							if nextPart == "Object Cache" || nextPart == "Servlet Cache" || 
							   nextPart == "Counters" || nextPart == "Dependency IDs" {
								break
							}
							fullObjectPath += "/" + nextPart
						}
						labels["cache"] = fullObjectPath
						labels["instance"] = fullObjectPath
					} else {
						labels["cache"] = part
						labels["instance"] = part
					}
				case "Servlet Session Manager":
					labels["session_app"] = part
					labels["instance"] = part
				case "Security":
					// For Security, the next part is the type (Authentication, Authorization)
					if i+1 < len(pathParts) {
						securityType := pathParts[i+1]
						labels["security_type"] = strings.ToLower(strings.ReplaceAll(securityType, " ", "_"))
						labels["instance"] = strings.ToLower(strings.ReplaceAll(securityType, " ", "_"))
					}
				default:
					// For nested resources like "SIB JMS Resource Adapter/jms/built-in-jms-connectionfactory"
					if i > 1 && (prevPart == "SIB JMS Resource Adapter" || 
					            strings.Contains(part, "jms/") || 
					            strings.Contains(part, "jdbc/")) {
						labels["resource"] = part
						labels["instance"] = part
					}
				}
			}
		}
	}
}

// addArrayElementLabels adds labels specific to array elements
func (f *XMLFlattener) addArrayElementLabels(arrayPath, elementName string, index int, labels map[string]string) {
	// Add instance name and index
	labels["instance"] = elementName
	labels["index"] = strconv.Itoa(index)

	// Add specific labels based on array type
	if strings.Contains(arrayPath, "Thread Pools") {
		labels["pool"] = elementName
	} else if strings.Contains(arrayPath, "JDBC Connection Pools") {
		labels["datasource"] = elementName
	} else if strings.Contains(arrayPath, "Web Applications") {
		labels["app"] = elementName
	} else if strings.Contains(arrayPath, "Servlets") {
		labels["servlet"] = elementName
	} else if strings.Contains(arrayPath, "Portlets") {
		labels["portlet"] = elementName
	}
}

// extractMetrics extracts all metrics from a stat
func (f *XMLFlattener) extractMetrics(stat *pmiStat, basePath string, labels map[string]string, result *FlattenerResult) {
	// Extract CountStatistics
	for _, cs := range stat.CountStatistics {
		if value, err := strconv.ParseInt(cs.Count, 10, 64); err == nil {
			metric := MetricTuple{
				Path:   basePath + "/" + cs.Name,
				Labels: copyLabels(labels),
				Value:  value,
				Unit:   "requests", // Default unit for counts
				Type:   "count",
			}
			f.refineMetricUnit(&metric)
			
			// Generate fully unique context and instance
			metric.UniqueContext, metric.UniqueInstance = f.generateUniqueContextAndInstance(&metric)
			
			result.Metrics = append(result.Metrics, metric)
		}
	}

	// Extract TimeStatistics
	for _, ts := range stat.TimeStatistics {
		if value, err := strconv.ParseInt(ts.Count, 10, 64); err == nil {
			metric := MetricTuple{
				Path:   basePath + "/" + ts.Name,
				Labels: copyLabels(labels),
				Value:  value,
				Unit:   "milliseconds",
				Type:   "time",
			}
			
			// Generate fully unique context and instance
			metric.UniqueContext, metric.UniqueInstance = f.generateUniqueContextAndInstance(&metric)
			
			result.Metrics = append(result.Metrics, metric)
		}
	}

	// Extract BoundedRangeStatistics
	for _, brs := range stat.BoundedRangeStatistics {
		if value, err := strconv.ParseInt(brs.Current, 10, 64); err == nil {
			metric := MetricTuple{
				Path:   basePath + "/" + brs.Name,
				Labels: copyLabels(labels),
				Value:  value,
				Unit:   "items", // Default unit for bounded ranges
				Type:   "bounded_range",
			}
			f.refineMetricUnit(&metric)
			
			// Generate fully unique context and instance
			metric.UniqueContext, metric.UniqueInstance = f.generateUniqueContextAndInstance(&metric)
			
			result.Metrics = append(result.Metrics, metric)
		}
	}

	// Extract RangeStatistics
	for _, rs := range stat.RangeStatistics {
		if value, err := strconv.ParseInt(rs.Current, 10, 64); err == nil {
			metric := MetricTuple{
				Path:   basePath + "/" + rs.Name,
				Labels: copyLabels(labels),
				Value:  value,
				Unit:   "items",
				Type:   "range",
			}
			f.refineMetricUnit(&metric)
			
			// Generate fully unique context and instance
			metric.UniqueContext, metric.UniqueInstance = f.generateUniqueContextAndInstance(&metric)
			
			result.Metrics = append(result.Metrics, metric)
		}
	}

	// Extract DoubleStatistics
	for _, ds := range stat.DoubleStatistics {
		if value, err := strconv.ParseFloat(ds.Double, 64); err == nil {
			metric := MetricTuple{
				Path:   basePath + "/" + ds.Name,
				Labels: copyLabels(labels),
				Value:  int64(value), // Convert to int64 for consistency
				Unit:   "percent", // Default unit for doubles
				Type:   "double",
			}
			f.refineMetricUnit(&metric)
			
			// Generate fully unique context and instance
			metric.UniqueContext, metric.UniqueInstance = f.generateUniqueContextAndInstance(&metric)
			
			result.Metrics = append(result.Metrics, metric)
		}
	}
}

// refineMetricUnit infers better units based on metric name and type
func (f *XMLFlattener) refineMetricUnit(metric *MetricTuple) {
	name := strings.ToLower(metric.Path)
	
	// Percentage metrics (check first as these override other patterns)
	if strings.Contains(name, "percent") || strings.Contains(name, "utilization") || 
	   (strings.Contains(name, "usage") && metric.Type == "double") {
		metric.Unit = "percent"
		return
	}

	// Size-related metrics (check before time as "Runtime" contains "time")
	if strings.Contains(name, "size") || strings.Contains(name, "memory") || 
	   strings.Contains(name, "heap") || strings.Contains(name, "bytes") {
		metric.Unit = "bytes"
		return
	}

	// Time-related metrics
	if metric.Type == "time" || 
	   (strings.Contains(name, "time") && !strings.Contains(name, "runtime")) || 
	   strings.Contains(name, "duration") || strings.Contains(name, "response") {
		metric.Unit = "milliseconds"
		return
	}

	// Count metrics
	if metric.Type == "count" || strings.Contains(name, "count") || 
	   strings.Contains(name, "number") || strings.Contains(name, "active") || 
	   strings.Contains(name, "pool") || strings.Contains(name, "connection") {
		metric.Unit = "requests"
		return
	}

	// Rate metrics
	if strings.Contains(name, "rate") || strings.Contains(name, "throughput") {
		metric.Unit = "requests/s"
		return
	}
}

// generateUniqueContextAndInstance creates fully unique context and instance paths
// that include the complete hierarchical structure including the metric name.
// This ensures zero collisions before the correlator groups metrics.
func (f *XMLFlattener) generateUniqueContextAndInstance(metric *MetricTuple) (context, instance string) {
	// Split path into components and sanitize each part separately
	// This maintains the hierarchical structure for correlation
	
	pathParts := strings.Split(metric.Path, "/")
	sanitizedParts := make([]string, 0, len(pathParts))
	
	for _, part := range pathParts {
		if part != "" { // Skip empty parts
			sanitizedParts = append(sanitizedParts, sanitizeDimensionID(part))
		}
	}
	
	// Build the structured path
	structuredPath := strings.Join(sanitizedParts, ".")
	
	// Clean type and unit
	sanitizedType := sanitizeDimensionID(metric.Type)
	if sanitizedType == "" {
		sanitizedType = "unknown_type"
	}
	
	sanitizedUnit := sanitizeDimensionID(metric.Unit)
	if sanitizedUnit == "" {
		sanitizedUnit = "unknown_unit"
	}
	
	// Create unique context and instance by including all identifying information
	uniqueContext := fmt.Sprintf("websphere_pmi.%s", structuredPath)
	uniqueInstance := fmt.Sprintf("websphere_pmi.%s.%s.%s", 
		structuredPath, sanitizedType, sanitizedUnit)
		
	return uniqueContext, uniqueInstance
}

// extractObjectName extracts the object name from "Object: ws/com.ibm.workplace/ExtensionRegistryCache"
func (f *XMLFlattener) extractObjectName(objectSpec string) string {
	if strings.HasPrefix(objectSpec, "Object:") {
		name := strings.TrimSpace(objectSpec[7:]) // Remove "Object:" prefix
		// Take the last component of the path
		parts := strings.Split(name, "/")
		if len(parts) > 0 {
			return parts[len(parts)-1]
		}
		return name
	}
	return objectSpec
}

// normalizeForLabel converts a string to a valid label value
func (f *XMLFlattener) normalizeForLabel(s string) string {
	// Only replace / with _ to prevent UI from making submenus
	return strings.ReplaceAll(s, "/", "_")
}

// copyLabels creates a deep copy of a label map
func copyLabels(src map[string]string) map[string]string {
	dst := make(map[string]string, len(src))
	for k, v := range src {
		dst[k] = v
	}
	return dst
}