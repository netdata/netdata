// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

// #include <cmqc.h>
// #include <cmqxc.h>
// #include <cmqcfc.h>
import "C"

import (
	"context"
	"fmt"
	"strings"
)

// collectQueueMetricsWithDynamicCharts implements the WebSphere PMI pattern:
// 1. First attempt data collection
// 2. Only create charts if collection succeeds
// 3. Create charts specific to the metrics that were actually collected
func (c *Collector) collectQueueMetricsWithDynamicCharts(ctx context.Context, queueName, cleanName string, mx map[string]int64) {
	// Temporary map to collect metrics before chart creation
	tempMx := make(map[string]int64)
	
	// Labels for all queue charts
	queueLabels := map[string]string{
		"queue": queueName,
	}

	// Try to collect queue configuration metrics (MQCMD_INQUIRE_Q)
	configMetrics := c.collectQueueConfigurationData(ctx, queueName)
	if len(configMetrics) > 0 {
		// Configuration collection succeeded - create relevant charts
		c.createQueueConfigCharts(queueName, configMetrics, queueLabels)
		
		// Add metrics to main collection with proper dimension IDs
		for metric, value := range configMetrics {
			dimID := fmt.Sprintf("queue_config_%s_%s", cleanName, metric)
			tempMx[dimID] = value
		}
	}

	// Try to collect queue runtime metrics (MQCMD_INQUIRE_Q_STATUS)
	runtimeMetrics := c.collectQueueRuntimeData(ctx, queueName)
	if len(runtimeMetrics) > 0 {
		// Runtime collection succeeded - create relevant charts
		c.createQueueRuntimeCharts(queueName, runtimeMetrics, queueLabels)
		
		// Add metrics to main collection with proper dimension IDs
		for metric, value := range runtimeMetrics {
			dimID := fmt.Sprintf("queue_runtime_%s_%s", cleanName, metric)
			tempMx[dimID] = value
		}
	}

	// Try to collect queue reset statistics (if enabled)
	if c.CollectResetQueueStats != nil && *c.CollectResetQueueStats {
		resetMetrics := c.collectQueueResetData(ctx, queueName)
		if len(resetMetrics) > 0 {
			// Reset stats collection succeeded - create relevant charts
			c.createQueueResetCharts(queueName, resetMetrics, queueLabels)
			
			// Add metrics to main collection with proper dimension IDs
			for metric, value := range resetMetrics {
				dimID := fmt.Sprintf("queue_reset_%s_%s", cleanName, metric)
				tempMx[dimID] = value
			}
		}
	}

	// Add all successfully collected metrics to main map
	for key, value := range tempMx {
		mx[key] = value
	}

	c.Debugf("Collected %d metrics for queue %s", len(tempMx), queueName)
}

// collectQueueConfigurationData attempts to collect configuration metrics and returns what was found
func (c *Collector) collectQueueConfigurationData(ctx context.Context, queueName string) map[string]int64 {
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		c.Debugf("Failed to send MQCMD_INQUIRE_Q for %s: %v", queueName, err)
		return nil
	}

	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_Q")
	if err != nil {
		c.Debugf("Failed to parse queue config response for %s: %v", queueName, err)
		return nil
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			c.Debugf("MQCMD_INQUIRE_Q failed for %s: reason %d (%s)", queueName, reason, mqReasonString(reason))
			return nil
		}
	}

	// Extract available configuration metrics
	metrics := make(map[string]int64)
	
	// Queue depth and limits
	if val, ok := attrs[C.MQIA_CURRENT_Q_DEPTH]; ok {
		if depth, ok := val.(int32); ok {
			metrics["depth"] = int64(depth)
		}
	}
	if val, ok := attrs[C.MQIA_MAX_Q_DEPTH]; ok {
		if maxDepth, ok := val.(int32); ok {
			metrics["max_depth"] = int64(maxDepth)
		}
	}
	if val, ok := attrs[C.MQIA_Q_DEPTH_HIGH_LIMIT]; ok {
		if highLimit, ok := val.(int32); ok {
			metrics["depth_high_limit"] = int64(highLimit)
		}
	}
	if val, ok := attrs[C.MQIA_Q_DEPTH_LOW_LIMIT]; ok {
		if lowLimit, ok := val.(int32); ok {
			metrics["depth_low_limit"] = int64(lowLimit)
		}
	}

	// Inhibit settings
	if val, ok := attrs[C.MQIA_INHIBIT_GET]; ok {
		if inhibit, ok := val.(int32); ok {
			metrics["inhibit_get"] = int64(inhibit)
		}
	}
	if val, ok := attrs[C.MQIA_INHIBIT_PUT]; ok {
		if inhibit, ok := val.(int32); ok {
			metrics["inhibit_put"] = int64(inhibit)
		}
	}

	// Default settings
	if val, ok := attrs[C.MQIA_DEF_PRIORITY]; ok {
		if priority, ok := val.(int32); ok {
			metrics["def_priority"] = int64(priority)
		}
	}
	if val, ok := attrs[C.MQIA_DEF_PERSISTENCE]; ok {
		if persistence, ok := val.(int32); ok {
			metrics["def_persistence"] = int64(persistence)
		}
	}

	// Backout threshold
	if val, ok := attrs[C.MQIA_BACKOUT_THRESHOLD]; ok {
		if threshold, ok := val.(int32); ok {
			metrics["backout_threshold"] = int64(threshold)
		}
	}

	// Trigger depth
	if val, ok := attrs[C.MQIA_TRIGGER_DEPTH]; ok {
		if depth, ok := val.(int32); ok {
			metrics["trigger_depth"] = int64(depth)
		}
	}

	// Message counts (if available in config response)
	if val, ok := attrs[C.MQIA_MSG_ENQ_COUNT]; ok {
		if count, ok := val.(int32); ok {
			metrics["enqueued"] = int64(count)
		}
	}
	if val, ok := attrs[C.MQIA_MSG_DEQ_COUNT]; ok {
		if count, ok := val.(int32); ok {
			metrics["dequeued"] = int64(count)
		}
	}

	return metrics
}

// collectQueueRuntimeData attempts to collect runtime metrics and returns what was found
func (c *Collector) collectQueueRuntimeData(ctx context.Context, queueName string) map[string]int64 {
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_Q_STATUS, params)
	if err != nil {
		c.Debugf("Failed to send MQCMD_INQUIRE_Q_STATUS for %s: %v", queueName, err)
		return nil
	}

	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_Q_STATUS")
	if err != nil {
		c.Debugf("Failed to parse queue status response for %s: %v", queueName, err)
		return nil
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			c.Debugf("MQCMD_INQUIRE_Q_STATUS failed for %s: reason %d (%s)", queueName, reason, mqReasonString(reason))
			return nil
		}
	}

	// Extract available runtime metrics
	metrics := make(map[string]int64)

	// Open handles
	if val, ok := attrs[C.MQIA_OPEN_INPUT_COUNT]; ok {
		if count, ok := val.(int32); ok {
			metrics["open_input_count"] = int64(count)
		}
	}
	if val, ok := attrs[C.MQIA_OPEN_OUTPUT_COUNT]; ok {
		if count, ok := val.(int32); ok {
			metrics["open_output_count"] = int64(count)
		}
	}

	return metrics
}

// collectQueueResetData attempts to collect reset statistics and returns what was found
func (c *Collector) collectQueueResetData(ctx context.Context, queueName string) map[string]int64 {
	// WARNING: This is destructive and resets counters!
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_RESET_Q_STATS, params)
	if err != nil {
		c.Debugf("Failed to send MQCMD_RESET_Q_STATS for %s: %v", queueName, err)
		return nil
	}

	attrs, err := c.parsePCFResponse(response, "MQCMD_RESET_Q_STATS")
	if err != nil {
		c.Debugf("Failed to parse queue reset stats response for %s: %v", queueName, err)
		return nil
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			c.Debugf("MQCMD_RESET_Q_STATS failed for %s: reason %d (%s)", queueName, reason, mqReasonString(reason))
			return nil
		}
	}

	// Extract available reset statistics
	metrics := make(map[string]int64)

	// These metrics are only available through reset statistics
	// Peak queue depth
	if val, ok := attrs[C.MQIA_HIGH_Q_DEPTH]; ok {
		if peak, ok := val.(int32); ok {
			metrics["high_q_depth"] = int64(peak)
		}
	}

	// Oldest message age (if available)
	// Note: This attribute may not be available in all versions
	for attr, val := range attrs {
		// Log unknown attributes for debugging
		if attrInt32 := int32(attr); attrInt32 > 1000 && attrInt32 < 10000 {
			if intVal, ok := val.(int32); ok {
				c.Debugf("Queue %s reset stats - unknown attribute %d = %d", queueName, attrInt32, intVal)
				// Create a generic metric for unknown but potentially useful attributes
				metricName := fmt.Sprintf("attr_%d", attrInt32)
				metrics[metricName] = int64(intVal)
			}
		}
	}

	return metrics
}

// createQueueConfigCharts creates charts for configuration metrics that were successfully collected
func (c *Collector) createQueueConfigCharts(queueName string, metrics map[string]int64, labels map[string]string) {
	
	// Create charts based on what metrics are actually available
	
	// Queue depth chart (if we have depth-related metrics)
	depthDims := []string{}
	if _, hasDepth := metrics["depth"]; hasDepth {
		depthDims = append(depthDims, "depth")
	}
	if len(depthDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_depth",
			"Queue Current Depth",
			"messages",
			"line",
			"queues",
			prioQueueDepth,
			depthDims,
			queueName,
			labels,
		)
	}

	// Queue configuration chart (limits and thresholds)
	configDims := []string{}
	if _, hasMaxDepth := metrics["max_depth"]; hasMaxDepth {
		configDims = append(configDims, "max_depth")
	}
	if _, hasBackout := metrics["backout_threshold"]; hasBackout {
		configDims = append(configDims, "backout_threshold")
	}
	if _, hasTrigger := metrics["trigger_depth"]; hasTrigger {
		configDims = append(configDims, "trigger_depth")
	}
	if len(configDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_config",
			"Queue Configuration Limits",
			"messages",
			"line",
			"queues",
			prioQueueDepth+1,
			configDims,
			queueName,
			labels,
		)
	}

	// Queue inhibit status
	inhibitDims := []string{}
	if _, hasInhibitGet := metrics["inhibit_get"]; hasInhibitGet {
		inhibitDims = append(inhibitDims, "inhibit_get")
	}
	if _, hasInhibitPut := metrics["inhibit_put"]; hasInhibitPut {
		inhibitDims = append(inhibitDims, "inhibit_put")
	}
	if len(inhibitDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_inhibit",
			"Queue Inhibit Status",
			"status",
			"line",
			"queues",
			prioQueueDepth+2,
			inhibitDims,
			queueName,
			labels,
		)
	}

	// Queue defaults
	defaultDims := []string{}
	if _, hasPriority := metrics["def_priority"]; hasPriority {
		defaultDims = append(defaultDims, "def_priority")
	}
	if _, hasPersistence := metrics["def_persistence"]; hasPersistence {
		defaultDims = append(defaultDims, "def_persistence")
	}
	if len(defaultDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_defaults",
			"Queue Default Settings",
			"value",
			"line",
			"queues",
			prioQueueDepth+3,
			defaultDims,
			queueName,
			labels,
		)
	}

	// Message counters (if available in config)
	counterDims := []string{}
	if _, hasEnq := metrics["enqueued"]; hasEnq {
		counterDims = append(counterDims, "enqueued")
	}
	if _, hasDeq := metrics["dequeued"]; hasDeq {
		counterDims = append(counterDims, "dequeued")
	}
	if len(counterDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_messages",
			"Queue Message Counters",
			"messages/s",
			"line",
			"queues",
			prioQueueMessages,
			counterDims,
			queueName,
			labels,
		)
	}

	// Depth events/limits
	eventDims := []string{}
	if _, hasHighLimit := metrics["depth_high_limit"]; hasHighLimit {
		eventDims = append(eventDims, "depth_high_limit")
	}
	if _, hasLowLimit := metrics["depth_low_limit"]; hasLowLimit {
		eventDims = append(eventDims, "depth_low_limit")
	}
	if len(eventDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_depth_events",
			"Queue Depth Event Thresholds",
			"messages",
			"line",
			"queues",
			prioQueueDepth+5,
			eventDims,
			queueName,
			labels,
		)
	}
}

// createQueueRuntimeCharts creates charts for runtime metrics that were successfully collected
func (c *Collector) createQueueRuntimeCharts(queueName string, metrics map[string]int64, labels map[string]string) {
	// Queue activity (open handles)
	activityDims := []string{}
	if _, hasInput := metrics["open_input_count"]; hasInput {
		activityDims = append(activityDims, "open_input_count")
	}
	if _, hasOutput := metrics["open_output_count"]; hasOutput {
		activityDims = append(activityDims, "open_output_count")
	}
	if len(activityDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_activity",
			"Queue Activity Metrics",
			"connections",
			"line",
			"queues",
			prioQueueDepth+4,
			activityDims,
			queueName,
			labels,
		)
	}
}

// createQueueResetCharts creates charts for reset statistics that were successfully collected
func (c *Collector) createQueueResetCharts(queueName string, metrics map[string]int64, labels map[string]string) {
	// Peak depth and other reset statistics
	resetDims := []string{}
	if _, hasPeak := metrics["high_q_depth"]; hasPeak {
		resetDims = append(resetDims, "high_q_depth")
	}
	
	// Add any unknown attributes as generic metrics
	for metric := range metrics {
		if strings.HasPrefix(metric, "attr_") {
			resetDims = append(resetDims, metric)
		}
	}

	if len(resetDims) > 0 {
		c.ensureChartExists(
			"mq_pcf.queue_reset_stats",
			"Queue Reset Statistics",
			"value",
			"line",
			"queues",
			prioQueueDepth+6,
			resetDims,
			queueName,
			labels,
		)
	}
}