package framework

import (
	"context"
	"fmt"
	"strings"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

// Collector is the base type for all framework-based collectors
type Collector struct {
	module.Base
	
	Config              Config
	State               *CollectorState
	registeredContexts  []interface{} // All contexts from generated code
	contextMap          map[string]interface{}
	charts              *module.Charts
	impl                CollectorImpl // The actual collector implementation
	globalLabels        []module.Label // Job-level labels applied to all charts
}

// Init initializes the collector (go.d framework requirement)
func (c *Collector) Init(ctx context.Context) error {
	// Initialize state
	c.State = NewCollectorState()
	c.State.collector = &c.Base // Set logger reference
	c.contextMap = make(map[string]interface{})
	c.charts = &module.Charts{}
	c.globalLabels = make([]module.Label, 0)
	
	// Set defaults
	if c.Config.ObsoletionIterations == 0 {
		c.Config.ObsoletionIterations = 60
	}
	if c.Config.UpdateEvery == 0 {
		c.Config.UpdateEvery = 1
	}
	
	// Validate configuration
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid configuration: %v", err)
	}
	
	return nil
}

// Check tests connectivity (go.d framework requirement)
func (c *Collector) Check(ctx context.Context) error {
	// This will be overridden by specific collectors
	// to test protocol connectivity
	return nil
}

// Charts returns the chart definitions (go.d framework requirement)
func (c *Collector) Charts() *module.Charts {
	return c.charts
}

// Collect gathers metrics (go.d framework requirement)
func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	// Increment global iteration counter
	c.State.iteration++
	
	// Clear previous iteration errors
	c.State.ClearErrors()
	
	// Run collection (implemented by specific collector)
	var err error
	if c.impl != nil {
		err = c.impl.CollectOnce()
	} else {
		err = fmt.Errorf("collector implementation not set")
	}
	
	// Handle errors (let the module decide how to handle them)
	if err != nil {
		c.Errorf("collection failed: %v", err)
		// Return whatever metrics we have (partial collection is OK)
	}
	
	// Convert collected metrics to go.d format
	metrics := c.convertMetrics()
	
	// Advance iteration and handle obsoletion
	c.State.NextIteration(c.Config.ObsoletionIterations)
	
	// Handle obsolete instances - mark their charts as obsolete
	for _, instanceKey := range c.State.GetObsoleteInstances() {
		c.markChartsObsolete(instanceKey)
	}
	
	return metrics
}

// Cleanup performs cleanup (go.d framework requirement)
func (c *Collector) Cleanup(ctx context.Context) {
	// This will be overridden by specific collectors
	// to close connections, etc.
}

// SetImpl sets the collector implementation
func (c *Collector) SetImpl(impl CollectorImpl) {
	c.impl = impl
}

// validateConfig validates the configuration
func (c *Collector) validateConfig() error {
	// Validate update intervals are multiples of base
	for groupName, interval := range c.Config.CollectionGroups {
		if interval % c.Config.UpdateEvery != 0 {
			// Adjust to nearest valid multiple
			adjusted := ((interval / c.Config.UpdateEvery) + 1) * c.Config.UpdateEvery
			c.Config.CollectionGroups[groupName] = adjusted
			c.Infof("adjusted %s interval from %d to %d (must be multiple of %d)", 
				groupName, interval, adjusted, c.Config.UpdateEvery)
		}
	}
	
	return nil
}


// convertMetrics converts framework metrics to go.d format
func (c *Collector) convertMetrics() map[string]int64 {
	mx := make(map[string]int64)
	
	// Track seen instances for dynamic chart creation
	seenInstances := make(map[string]bool)
	
	// Convert instance metrics
	for _, metric := range c.State.GetMetrics() {
		// Track this instance
		instanceKey := metric.Instance.key
		seenInstances[instanceKey] = true
		
		// Check if we need to create charts for this instance
		if !c.hasChartsForInstance(instanceKey) {
			// Find the context this instance belongs to
			for _, ctx := range c.registeredContexts {
				contextMeta := extractContextMetadata(ctx)
				if contextMeta != nil && contextMeta.Name == metric.Instance.contextName {
					// Create charts for this new instance
					chart := c.createChartFromContext(ctx, instanceKey)
					if chart != nil {
						c.Debugf("Creating dynamic chart for instance: %s", instanceKey)
						// Add labels from the instance
						for k, v := range metric.Instance.labels {
							chart.Labels = append(chart.Labels, module.Label{
								Key:   k,
								Value: v,
							})
						}
						c.charts.Add(chart)
					}
					break
				}
			}
		}
		
		// Generate go.d metric key
		key := c.generateMetricKey(metric)
		mx[key] = metric.Value
	}
	
	// Add protocol observability metrics (if any protocols are registered)
	// This will be added when protocol observability charts are created
	// for name, pm := range c.State.protocols {
	// 	prefix := fmt.Sprintf("protocol_%s_", name)
	// 	mx[prefix+"requests"] = pm.RequestCount
	// 	mx[prefix+"errors"] = pm.ErrorCount
	// 	if pm.RequestCount > 0 {
	// 		mx[prefix+"avg_latency"] = pm.TotalLatency / pm.RequestCount
	// 	}
	// 	mx[prefix+"max_latency"] = pm.MaxLatency
	// 	mx[prefix+"bytes_sent"] = pm.BytesSent
	// 	mx[prefix+"bytes_received"] = pm.BytesReceived
	// }
	
	return mx
}

// generateMetricKey creates the go.d metric key from instance and dimension
func (c *Collector) generateMetricKey(metric MetricValue) string {
	// Use our new scheme: {instance_id}.{dimension_name}
	return metric.Instance.key + "." + metric.Dimension
}

// cleanLabelValue cleans a label value for use in metric keys
func cleanLabelValue(value string) string {
	// Replace problematic characters
	r := strings.NewReplacer(
		" ", "_",
		".", "_",
		"-", "_",
		"/", "_",
		":", "_",
		"=", "_",
	)
	return strings.ToLower(r.Replace(value))
}

// createChartFromContext creates a go.d chart from a Context[T]
func (c *Collector) createChartFromContext(ctx interface{}, instanceID string) *module.Chart {
	// Use reflection to extract context metadata
	contextMeta := extractContextMetadata(ctx)
	if contextMeta == nil {
		return nil
	}
	
	// Create chart with unique ID for dynamic instances
	// For labeled contexts, include the instance ID in the chart ID
	chartID := cleanChartID(instanceID)
	if !contextMeta.HasLabels {
		// For unlabeled contexts, use just the context name
		chartID = cleanChartID(contextMeta.Name)
	}
	
	chart := &module.Chart{
		ID:       chartID,
		OverID:   instanceID,  // Set OverID to instance ID (with dots, not underscores)
		Title:    contextMeta.Title,
		Units:    contextMeta.Units,
		Fam:      contextMeta.Family,
		Ctx:      contextMeta.Name,
		Type:     contextMeta.Type,
		Priority: contextMeta.Priority,
		Opts:     module.Opts{},
	}
	
	// Add dimensions with full dimension IDs
	for _, dim := range contextMeta.Dimensions {
		// Use the full dimension ID: {instance_id}.{dimension_name}
		dimID := instanceID + "." + dim.Name
		chartDim := &module.Dim{
			ID:   dimID,
			Name: dim.Name,
			Algo: dim.Algorithm,
			Mul:  1,             // Always 1 - values are already in base units
			Div:  dim.Precision, // Only divide by precision to restore decimals
		}
		chart.AddDim(chartDim)
	}
	
	// Apply global labels to the new chart
	c.applyGlobalLabelsToChart(chart)
	
	return chart
}

// cleanChartID converts context name to valid chart ID
func cleanChartID(contextName string) string {
	// Convert dots to underscores for chart ID
	// example.test_absolute -> example_test_absolute
	return strings.ReplaceAll(contextName, ".", "_")
}

// hasChartsForInstance checks if charts exist for a given instance
func (c *Collector) hasChartsForInstance(instanceKey string) bool {
	// Check if any chart has dimensions with this instance key prefix
	for _, chart := range *c.charts {
		for _, dim := range chart.Dims {
			if strings.HasPrefix(dim.ID, instanceKey+".") {
				return true
			}
		}
	}
	return false
}

// markChartsObsolete marks all charts for a given instance as obsolete
func (c *Collector) markChartsObsolete(instanceKey string) {
	// Find charts that belong to this instance
	chartID := cleanChartID(instanceKey)
	
	for _, chart := range *c.charts {
		if chart.ID == chartID && !chart.Obsolete {
			c.Debugf("Marking chart %s as obsolete for instance %s", chart.ID, instanceKey)
			chart.Obsolete = true
			chart.MarkNotCreated() // Reset created flag to trigger CHART command with obsolete flag
		}
	}
}

// Helper method for collectors to register generated contexts
func (c *Collector) RegisterContexts(contexts ...interface{}) {
	c.registeredContexts = append(c.registeredContexts, contexts...)
	// Build context map for quick lookup
	if c.contextMap == nil {
		c.contextMap = make(map[string]interface{})
	}
	for _, ctx := range contexts {
		name := extractContextName(ctx)
		c.contextMap[name] = ctx
	}
}

// GetBase returns the base module
func (c *Collector) GetBase() *module.Base {
	return &c.Base
}

// Configuration returns the configuration interface
func (c *Collector) Configuration() any {
	// Default implementation - collectors should override this
	return c.Config
}

// SetGlobalLabel adds or updates a job-level label that will be applied to all charts
func (c *Collector) SetGlobalLabel(key, value string) {
	// Check if label already exists
	for i, label := range c.globalLabels {
		if label.Key == key {
			c.globalLabels[i].Value = value
			c.updateChartsWithGlobalLabels()
			return
		}
	}
	
	// Add new label
	c.globalLabels = append(c.globalLabels, module.Label{Key: key, Value: value})
	c.updateChartsWithGlobalLabels()
}

// SetGlobalLabels replaces all global labels
func (c *Collector) SetGlobalLabels(labels map[string]string) {
	c.globalLabels = make([]module.Label, 0, len(labels))
	for key, value := range labels {
		c.globalLabels = append(c.globalLabels, module.Label{Key: key, Value: value})
	}
	c.updateChartsWithGlobalLabels()
}

// updateChartsWithGlobalLabels applies global labels to all existing charts
func (c *Collector) updateChartsWithGlobalLabels() {
	if c.charts == nil {
		return
	}
	
	for _, chart := range *c.charts {
		// Update existing chart labels
		c.applyGlobalLabelsToChart(chart)
	}
}

// applyGlobalLabelsToChart adds global labels to a chart, avoiding duplicates
func (c *Collector) applyGlobalLabelsToChart(chart *module.Chart) {
	// Create a map of existing labels for quick lookup
	existingLabels := make(map[string]int)
	for i, label := range chart.Labels {
		existingLabels[label.Key] = i
	}
	
	// Apply global labels
	for _, globalLabel := range c.globalLabels {
		if idx, exists := existingLabels[globalLabel.Key]; exists {
			// Update existing label
			chart.Labels[idx] = globalLabel
		} else {
			// Add new label
			chart.Labels = append(chart.Labels, globalLabel)
		}
	}
}

// GetCurrentIteration returns the current global iteration counter
func (c *Collector) GetCurrentIteration() int64 {
	if c.State != nil {
		return c.State.GetIteration()
	}
	return 0
}