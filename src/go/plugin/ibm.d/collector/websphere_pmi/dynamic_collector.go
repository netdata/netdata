// SPDX-License-Identifier: GPL-3.0-or-later

package websphere_pmi

import (
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// DynamicCollector manages dynamic chart creation and instance tracking
type DynamicCollector struct {
	mu           sync.RWMutex
	seen         map[string]bool // Track what instances we've seen this cycle
	collected    map[string]bool // Track what instances we've collected before
	chartCreated map[string]bool // Track what chart types we've created
	charts       *module.Charts  // Reference to main charts collection
}

// NewDynamicCollector creates a new dynamic collector
func NewDynamicCollector() *DynamicCollector {
	return &DynamicCollector{
		seen:         make(map[string]bool),
		collected:    make(map[string]bool),
		chartCreated: make(map[string]bool),
	}
}

// SetCharts sets the reference to the main charts collection
func (dc *DynamicCollector) SetCharts(charts *module.Charts) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.charts = charts
}

// ResetSeen resets the tracking for the current collection cycle
func (dc *DynamicCollector) ResetSeen() {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.seen = make(map[string]bool)
}

// MarkSeen marks an instance as seen in this collection cycle
func (dc *DynamicCollector) MarkSeen(instanceKey string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.seen[instanceKey] = true
}

// IsChartCreated checks if a chart type has been created
func (dc *DynamicCollector) IsChartCreated(chartContext string) bool {
	dc.mu.RLock()
	defer dc.mu.RUnlock()
	return dc.chartCreated[chartContext]
}

// MarkChartCreated marks a chart type as created
func (dc *DynamicCollector) MarkChartCreated(chartContext string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.chartCreated[chartContext] = true
}

// GetAbsentInstances returns instances that were collected before but not seen this cycle
func (dc *DynamicCollector) GetAbsentInstances() []string {
	dc.mu.RLock()
	defer dc.mu.RUnlock()

	var absent []string
	for instanceKey := range dc.collected {
		if !dc.seen[instanceKey] {
			absent = append(absent, instanceKey)
		}
	}
	return absent
}

// MarkCollected marks an instance as collected
func (dc *DynamicCollector) MarkCollected(instanceKey string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	dc.collected[instanceKey] = true
}

// RemoveCollected removes an instance from collected tracking
func (dc *DynamicCollector) RemoveCollected(instanceKey string) {
	dc.mu.Lock()
	defer dc.mu.Unlock()
	delete(dc.collected, instanceKey)
}

// ensureChartExists creates a chart if it doesn't exist and adds instance tracking
func (w *WebSpherePMI) ensureChartExists(context, title, units, chartType, family string, priority int, dimensions []string, instanceName string, labels map[string]string) {
	if w.dynamicCollector == nil {
		w.dynamicCollector = NewDynamicCollector()
		w.dynamicCollector.SetCharts(w.charts)
	}

	// Create unique chart ID for this instance (without module prefix)
	// Context: "websphere_pmi.threading.pools" -> Chart ID: "threading.pools_instanceName"
	contextWithoutModule := strings.TrimPrefix(context, "websphere_pmi.")
	chartID := fmt.Sprintf("%s_%s", contextWithoutModule, w.sanitizeForChartID(instanceName))
	instanceKey := fmt.Sprintf("%s|%s", context, instanceName)

	// Mark instance as seen in this cycle
	w.dynamicCollector.MarkSeen(instanceKey)

	// Check if chart already exists
	if w.charts.Has(chartID) {
		return
	}

	// Create chart
	chart := &module.Chart{
		ID:       chartID,
		Title:    title, // Use consistent title for all instances in same context
		Units:    units,
		Fam:      family,
		Ctx:      context,
		Type:     module.ChartType(chartType),
		Priority: priority,
		Dims:     make(module.Dims, 0, len(dimensions)),
		Labels:   make([]module.Label, 0, len(labels)),
	}

	// Add dimensions - dimension IDs must match the keys used in mx map
	// For NIDL compliance, dimension names are shared, but IDs include the chart ID
	for _, dim := range dimensions {
		dimID := fmt.Sprintf("%s_%s", chartID, dim)
		chart.Dims = append(chart.Dims, &module.Dim{
			ID:   dimID, // Unique dimension ID that matches mx map key
			Name: dim,   // Shared dimension name for NIDL compliance
		})
	}

	// Add labels
	for key, value := range labels {
		chart.Labels = append(chart.Labels, module.Label{
			Key:   key,
			Value: value,
		})
	}

	// Add identity labels from main collector
	if w.ClusterName != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "cluster", Value: w.ClusterName})
	}
	if w.CellName != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "cell", Value: w.CellName})
	}
	if w.NodeName != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "node", Value: w.NodeName})
	}
	if w.ServerType != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "server_type", Value: w.ServerType})
	}

	// Add custom labels
	for k, v := range w.CustomLabels {
		chart.Labels = append(chart.Labels, module.Label{Key: k, Value: v})
	}

	// Add version labels if available
	if w.wasVersion != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "websphere_version", Value: w.wasVersion})
	}
	if w.wasEdition != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "websphere_edition", Value: w.wasEdition})
	}

	// Add chart to collection
	if err := w.charts.Add(chart); err != nil {
		w.Warningf("failed to add chart %s: %v", chartID, err)
		return
	}

	// Mark instance as collected
	w.dynamicCollector.MarkCollected(instanceKey)

	w.Debugf("created chart %s for instance %s", chartID, instanceName)
}

// cleanupAbsentInstances removes charts for instances that are no longer present
func (w *WebSpherePMI) cleanupAbsentInstances() {
	if w.dynamicCollector == nil {
		return
	}

	absentInstances := w.dynamicCollector.GetAbsentInstances()
	for _, instanceKey := range absentInstances {
		w.removeInstanceCharts(instanceKey)
		w.dynamicCollector.RemoveCollected(instanceKey)
	}
}

// removeInstanceCharts removes all charts for a specific instance
func (w *WebSpherePMI) removeInstanceCharts(instanceKey string) {
	// Extract context and instance name from key
	parts := splitInstanceKey(instanceKey)
	if len(parts) != 2 {
		return
	}

	context := parts[0]
	instanceName := parts[1]
	chartID := fmt.Sprintf("%s_%s", context, w.sanitizeForChartID(instanceName))

	// Remove chart if it exists
	if w.charts.Has(chartID) {
		w.charts.Remove(chartID)
		w.Debugf("removed chart %s for absent instance %s", chartID, instanceName)
	}
}

// splitInstanceKey splits an instance key into context and instance name
func splitInstanceKey(instanceKey string) []string {
	// Find the first | separator
	for i, char := range instanceKey {
		if char == '|' {
			return []string{instanceKey[:i], instanceKey[i+1:]}
		}
	}
	return nil
}

// getMetricPrefix returns the prefix used for metrics in extractStatValues
func (w *WebSpherePMI) getMetricPrefix(context, instanceName string) string {
	// Map chart contexts to the prefixes used in collect_dynamic.go
	switch context {
	case "websphere_pmi.server.extensions":
		return fmt.Sprintf("server_extensions_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.server.status":
		return fmt.Sprintf("server_status_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.jvm.memory", "websphere_pmi.jvm.runtime":
		return fmt.Sprintf("jvm_runtime_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.jvm.object_pools":
		return fmt.Sprintf("jvm_object_pool_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.security.authentication":
		return fmt.Sprintf("security_auth_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.security.authorization":
		return fmt.Sprintf("security_authz_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.system.transactions":
		return fmt.Sprintf("transactions_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.system.data":
		return fmt.Sprintf("system_data_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.caching.dynacache":
		return fmt.Sprintf("dynacache_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.messaging.sib":
		return fmt.Sprintf("sib_messaging_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.threading.pools":
		return fmt.Sprintf("thread_pool_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.web.sessions":
		return fmt.Sprintf("web_sessions_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.connections.jdbc":
		return fmt.Sprintf("jdbc_pool_%s", w.sanitizeForMetricName(instanceName))
	case "websphere_pmi.connections.jca":
		return fmt.Sprintf("jca_pool_%s", w.sanitizeForMetricName(instanceName))
	default:
		// Generic fallback
		return fmt.Sprintf("generic_%s", w.sanitizeForMetricName(instanceName))
	}
}

// sanitizeForChartID sanitizes a string for use in chart IDs
func (w *WebSpherePMI) sanitizeForChartID(input string) string {
	// Replace invalid characters for chart IDs
	sanitized := ""
	for _, char := range input {
		if (char >= 'a' && char <= 'z') || (char >= 'A' && char <= 'Z') ||
			(char >= '0' && char <= '9') || char == '_' || char == '.' {
			sanitized += string(char)
		} else {
			sanitized += "_"
		}
	}
	return sanitized
}
