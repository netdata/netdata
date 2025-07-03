// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

import (
	"fmt"
	"strings"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// DynamicCollector manages dynamic chart creation and instance tracking for MQ PCF
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
// This is the key synchronization method that only creates charts when data is available
func (c *Collector) ensureChartExists(context, title, units, chartType, family string, priority int, dimensions []string, instanceName string, labels map[string]string) {
	if c.dynamicCollector == nil {
		c.dynamicCollector = NewDynamicCollector()
		c.dynamicCollector.SetCharts(c.charts)
	}

	// Create unique chart ID for this instance
	// Context: "mq_pcf.queue_depth" -> Chart ID: "queue_depth_instanceName"
	contextWithoutModule := strings.TrimPrefix(context, "mq_pcf.")
	chartID := fmt.Sprintf("%s_%s", contextWithoutModule, c.cleanName(instanceName))
	instanceKey := fmt.Sprintf("%s|%s", context, instanceName)

	// Mark instance as seen in this cycle
	c.dynamicCollector.MarkSeen(instanceKey)

	// Check if chart already exists
	if c.charts.Has(chartID) {
		return
	}

	// Create chart
	chart := &module.Chart{
		ID:       chartID,
		Title:    title,
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

	// Add version labels if available
	if c.version != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "mq_version", Value: c.version})
	}
	if c.edition != "" {
		chart.Labels = append(chart.Labels, module.Label{Key: "mq_edition", Value: c.edition})
	}

	// Add chart to collection
	if err := c.charts.Add(chart); err != nil {
		c.Warningf("failed to add chart %s: %v", chartID, err)
		return
	}

	// Mark instance as collected
	c.dynamicCollector.MarkCollected(instanceKey)

	c.Debugf("created chart %s for instance %s", chartID, instanceName)
}

// cleanupAbsentInstances removes charts for instances that are no longer present
func (c *Collector) cleanupAbsentInstances() {
	if c.dynamicCollector == nil {
		return
	}

	absentInstances := c.dynamicCollector.GetAbsentInstances()
	for _, instanceKey := range absentInstances {
		c.removeInstanceCharts(instanceKey)
		c.dynamicCollector.RemoveCollected(instanceKey)
	}
}

// removeInstanceCharts removes all charts for a specific instance
func (c *Collector) removeInstanceCharts(instanceKey string) {
	// Extract context and instance name from key
	parts := splitInstanceKey(instanceKey)
	if len(parts) != 2 {
		return
	}

	context := parts[0]
	instanceName := parts[1]
	contextWithoutModule := strings.TrimPrefix(context, "mq_pcf.")
	chartID := fmt.Sprintf("%s_%s", contextWithoutModule, c.cleanName(instanceName))

	// Mark chart as obsolete instead of removing it
	if chart := c.charts.Get(chartID); chart != nil {
		if !chart.Obsolete {
			chart.Obsolete = true
			chart.MarkNotCreated() // Reset created flag to trigger CHART command
			c.Debugf("marked chart %s as obsolete for absent instance %s", chartID, instanceName)
		}
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

// resetSeenTracking resets the tracking for what we've seen this cycle
func (c *Collector) resetSeenTracking() {
	if c.dynamicCollector == nil {
		c.dynamicCollector = NewDynamicCollector()
		c.dynamicCollector.SetCharts(c.charts)
	}
	c.dynamicCollector.ResetSeen()
}