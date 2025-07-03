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
		// Configuration collection succeeded - create relevant charts and add metrics
		c.addQueueConfigMetricsWithCharts(queueName, configMetrics, queueLabels, tempMx)
	}

	// Try to collect queue runtime metrics (MQCMD_INQUIRE_Q_STATUS)
	runtimeMetrics := c.collectQueueRuntimeData(ctx, queueName)
	if len(runtimeMetrics) > 0 {
		// Runtime collection succeeded - create relevant charts and add metrics
		c.addQueueRuntimeMetricsWithCharts(queueName, runtimeMetrics, queueLabels, tempMx)
	}

	// Try to collect queue reset statistics (if enabled)
	if c.CollectResetQueueStats != nil && *c.CollectResetQueueStats {
		resetMetrics := c.collectQueueResetData(ctx, queueName)
		if len(resetMetrics) > 0 {
			// Reset stats collection succeeded - create relevant charts and add metrics
			c.addQueueResetMetricsWithCharts(queueName, resetMetrics, queueLabels, tempMx)
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


// addQueueConfigMetricsWithCharts creates charts and adds metrics with properly synchronized dimension IDs
func (c *Collector) addQueueConfigMetricsWithCharts(queueName string, metrics map[string]int64, labels map[string]string, mx map[string]int64) {
	cleanName := c.cleanName(queueName)
	
	// Create charts based on what metrics are actually available and store metrics with matching dimension IDs
	
	// Queue depth chart (if we have depth-related metrics)
	depthDims := []string{}
	if _, hasDepth := metrics["depth"]; hasDepth {
		depthDims = append(depthDims, "depth")
	}
	if len(depthDims) > 0 {
		chartID := fmt.Sprintf("queue_depth_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations: chartID_dimensionName
		for _, dim := range depthDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
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
		chartID := fmt.Sprintf("queue_config_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range configDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
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
		chartID := fmt.Sprintf("queue_inhibit_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range inhibitDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
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
		chartID := fmt.Sprintf("queue_defaults_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range defaultDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
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
		chartID := fmt.Sprintf("queue_messages_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range counterDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
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
		chartID := fmt.Sprintf("queue_depth_events_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range eventDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}
}

// addQueueRuntimeMetricsWithCharts creates charts and adds metrics with properly synchronized dimension IDs
func (c *Collector) addQueueRuntimeMetricsWithCharts(queueName string, metrics map[string]int64, labels map[string]string, mx map[string]int64) {
	cleanName := c.cleanName(queueName)
	
	// Queue activity (open handles)
	activityDims := []string{}
	if _, hasInput := metrics["open_input_count"]; hasInput {
		activityDims = append(activityDims, "open_input_count")
	}
	if _, hasOutput := metrics["open_output_count"]; hasOutput {
		activityDims = append(activityDims, "open_output_count")
	}
	if len(activityDims) > 0 {
		chartID := fmt.Sprintf("queue_activity_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range activityDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}
}

// addQueueResetMetricsWithCharts creates charts and adds metrics with properly synchronized dimension IDs
func (c *Collector) addQueueResetMetricsWithCharts(queueName string, metrics map[string]int64, labels map[string]string, mx map[string]int64) {
	cleanName := c.cleanName(queueName)
	
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
		chartID := fmt.Sprintf("queue_reset_stats_%s", cleanName)
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
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range resetDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}
}

// collectChannelMetricsWithDynamicCharts implements the WebSphere PMI pattern for channels:
// 1. First attempt data collection
// 2. Only create charts if collection succeeds
// 3. Create charts specific to the metrics that were actually collected
func (c *Collector) collectChannelMetricsWithDynamicCharts(ctx context.Context, channelName, cleanName string, mx map[string]int64) {
	// Temporary map to collect metrics before chart creation
	tempMx := make(map[string]int64)
	
	// Labels for all channel charts
	channelLabels := map[string]string{
		"channel": channelName,
	}

	// Try to collect channel runtime metrics (MQCMD_INQUIRE_CHANNEL_STATUS)
	runtimeMetrics := c.collectChannelRuntimeData(ctx, channelName)
	if len(runtimeMetrics) > 0 {
		// Runtime collection succeeded - create relevant charts and add metrics
		c.addChannelRuntimeMetricsWithCharts(channelName, runtimeMetrics, channelLabels, tempMx)
	}

	// Try to collect channel configuration metrics (MQCMD_INQUIRE_CHANNEL)
	configMetrics := c.collectChannelConfigurationData(ctx, channelName)
	if len(configMetrics) > 0 {
		// Configuration collection succeeded - create relevant charts and add metrics
		c.addChannelConfigMetricsWithCharts(channelName, configMetrics, channelLabels, tempMx)
	}

	// Add all successfully collected metrics to main map
	for key, value := range tempMx {
		mx[key] = value
	}

	c.Debugf("Collected %d metrics for channel %s", len(tempMx), channelName)
}

// collectChannelRuntimeData attempts to collect runtime metrics and returns what was found
func (c *Collector) collectChannelRuntimeData(ctx context.Context, channelName string) map[string]int64 {
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, channelName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL_STATUS, params)
	if err != nil {
		c.Debugf("Failed to send MQCMD_INQUIRE_CHANNEL_STATUS for %s: %v", channelName, err)
		return nil
	}

	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_CHANNEL_STATUS")
	if err != nil {
		c.Debugf("Failed to parse channel status response for %s: %v", channelName, err)
		return nil
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			c.Debugf("MQCMD_INQUIRE_CHANNEL_STATUS failed for %s: reason %d (%s)", channelName, reason, mqReasonString(reason))
			return nil
		}
	}

	// Extract available runtime metrics
	metrics := make(map[string]int64)

	// Channel status
	if val, ok := attrs[C.MQIACH_CHANNEL_STATUS]; ok {
		if status, ok := val.(int32); ok {
			metrics["status"] = int64(status)
		}
	}

	// Message statistics
	if val, ok := attrs[C.MQIACH_MSGS]; ok {
		if msgs, ok := val.(int32); ok {
			metrics["messages"] = int64(msgs)
		}
	}

	// Byte statistics
	if val, ok := attrs[C.MQIACH_BYTES_SENT]; ok {
		if bytes, ok := val.(int32); ok {
			metrics["bytes"] = int64(bytes)
		}
	}

	// Batch statistics
	if val, ok := attrs[C.MQIACH_BATCHES]; ok {
		if batches, ok := val.(int32); ok {
			metrics["batches"] = int64(batches)
		}
	}

	return metrics
}

// collectChannelConfigurationData attempts to collect configuration metrics and returns what was found
func (c *Collector) collectChannelConfigurationData(ctx context.Context, channelName string) map[string]int64 {
	params := []pcfParameter{
		newStringParameter(C.MQCACH_CHANNEL_NAME, channelName),
	}

	response, err := c.sendPCFCommand(C.MQCMD_INQUIRE_CHANNEL, params)
	if err != nil {
		c.Debugf("Failed to send MQCMD_INQUIRE_CHANNEL for %s: %v", channelName, err)
		return nil
	}

	attrs, err := c.parsePCFResponse(response, "MQCMD_INQUIRE_CHANNEL")
	if err != nil {
		c.Debugf("Failed to parse channel config response for %s: %v", channelName, err)
		return nil
	}

	// Check for MQ errors in the response
	if reasonCode, ok := attrs[C.MQIACF_REASON_CODE]; ok {
		if reason, ok := reasonCode.(int32); ok && reason != 0 {
			c.Debugf("MQCMD_INQUIRE_CHANNEL failed for %s: reason %d (%s)", channelName, reason, mqReasonString(reason))
			return nil
		}
	}

	// Extract available configuration metrics
	metrics := make(map[string]int64)

	// Batch settings
	if val, ok := attrs[C.MQIACH_BATCH_SIZE]; ok {
		if batchSize, ok := val.(int32); ok {
			metrics["batch_size"] = int64(batchSize)
		}
	}
	if val, ok := attrs[C.MQIACH_BATCH_INTERVAL]; ok {
		if batchInterval, ok := val.(int32); ok {
			metrics["batch_interval"] = int64(batchInterval)
		}
	}

	// Timeout settings
	if val, ok := attrs[C.MQIACH_DISC_INTERVAL]; ok {
		if discInterval, ok := val.(int32); ok {
			metrics["disc_interval"] = int64(discInterval)
		}
	}
	if val, ok := attrs[C.MQIACH_HB_INTERVAL]; ok {
		if hbInterval, ok := val.(int32); ok {
			metrics["hb_interval"] = int64(hbInterval)
		}
	}
	if val, ok := attrs[C.MQIACH_KEEP_ALIVE_INTERVAL]; ok {
		if keepAlive, ok := val.(int32); ok {
			metrics["keep_alive_interval"] = int64(keepAlive)
		}
	}

	// Retry settings
	if val, ok := attrs[C.MQIACH_SHORT_RETRY]; ok {
		if shortRetry, ok := val.(int32); ok {
			metrics["short_retry"] = int64(shortRetry)
		}
	}
	if val, ok := attrs[C.MQIACH_SHORT_TIMER]; ok {
		if shortTimer, ok := val.(int32); ok {
			metrics["short_timer"] = int64(shortTimer)
		}
	}
	if val, ok := attrs[C.MQIACH_LONG_RETRY]; ok {
		if longRetry, ok := val.(int32); ok {
			metrics["long_retry"] = int64(longRetry)
		}
	}
	if val, ok := attrs[C.MQIACH_LONG_TIMER]; ok {
		if longTimer, ok := val.(int32); ok {
			metrics["long_timer"] = int64(longTimer)
		}
	}

	// Limits
	if val, ok := attrs[C.MQIACH_MAX_MSG_LENGTH]; ok {
		if maxMsgLength, ok := val.(int32); ok {
			metrics["max_msg_length"] = int64(maxMsgLength)
		}
	}
	if val, ok := attrs[C.MQIACH_SHARING_CONVERSATIONS]; ok {
		if sharingConversations, ok := val.(int32); ok {
			metrics["sharing_conversations"] = int64(sharingConversations)
		}
	}
	if val, ok := attrs[C.MQIACH_NETWORK_PRIORITY]; ok {
		if networkPriority, ok := val.(int32); ok {
			metrics["network_priority"] = int64(networkPriority)
		}
	}

	return metrics
}

// addChannelRuntimeMetricsWithCharts creates charts and adds metrics with properly synchronized dimension IDs
func (c *Collector) addChannelRuntimeMetricsWithCharts(channelName string, metrics map[string]int64, labels map[string]string, mx map[string]int64) {
	cleanName := c.cleanName(channelName)

	// Channel status
	statusDims := []string{}
	if _, hasStatus := metrics["status"]; hasStatus {
		statusDims = append(statusDims, "status")
	}
	if len(statusDims) > 0 {
		chartID := fmt.Sprintf("channel_status_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_status",
			"Channel Status",
			"status",
			"line",
			"channels",
			prioChannelStatus,
			statusDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range statusDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}

	// Message rate
	messageDims := []string{}
	if _, hasMessages := metrics["messages"]; hasMessages {
		messageDims = append(messageDims, "messages")
	}
	if len(messageDims) > 0 {
		chartID := fmt.Sprintf("channel_messages_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_messages",
			"Channel Message Rate",
			"messages/s",
			"line",
			"channels",
			prioChannelMessages,
			messageDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range messageDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}

	// Byte rate
	byteDims := []string{}
	if _, hasBytes := metrics["bytes"]; hasBytes {
		byteDims = append(byteDims, "bytes")
	}
	if len(byteDims) > 0 {
		chartID := fmt.Sprintf("channel_bytes_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_bytes",
			"Channel Data Transfer Rate",
			"bytes/s",
			"line",
			"channels",
			prioChannelBytes,
			byteDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range byteDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}

	// Batch rate
	batchDims := []string{}
	if _, hasBatches := metrics["batches"]; hasBatches {
		batchDims = append(batchDims, "batches")
	}
	if len(batchDims) > 0 {
		chartID := fmt.Sprintf("channel_batches_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_batches",
			"Channel Batch Rate",
			"batches/s",
			"line",
			"channels",
			prioChannelBatches,
			batchDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range batchDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}
}

// addChannelConfigMetricsWithCharts creates charts and adds metrics with properly synchronized dimension IDs
func (c *Collector) addChannelConfigMetricsWithCharts(channelName string, metrics map[string]int64, labels map[string]string, mx map[string]int64) {
	cleanName := c.cleanName(channelName)

	// Batch configuration
	batchDims := []string{}
	if _, hasBatchSize := metrics["batch_size"]; hasBatchSize {
		batchDims = append(batchDims, "batch_size")
	}
	if _, hasBatchInterval := metrics["batch_interval"]; hasBatchInterval {
		batchDims = append(batchDims, "batch_interval")
	}
	if len(batchDims) > 0 {
		chartID := fmt.Sprintf("channel_batch_config_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_batch_config",
			"Channel Batch Configuration",
			"value",
			"line",
			"channels",
			prioChannelStatus+1,
			batchDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range batchDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}

	// Timeout configuration
	timeoutDims := []string{}
	if _, hasDiscInterval := metrics["disc_interval"]; hasDiscInterval {
		timeoutDims = append(timeoutDims, "disc_interval")
	}
	if _, hasHbInterval := metrics["hb_interval"]; hasHbInterval {
		timeoutDims = append(timeoutDims, "hb_interval")
	}
	if _, hasKeepAlive := metrics["keep_alive_interval"]; hasKeepAlive {
		timeoutDims = append(timeoutDims, "keep_alive_interval")
	}
	if len(timeoutDims) > 0 {
		chartID := fmt.Sprintf("channel_timeouts_config_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_timeouts_config",
			"Channel Timeout Settings",
			"seconds",
			"line",
			"channels",
			prioChannelStatus+2,
			timeoutDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range timeoutDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}

	// Retry configuration
	retryDims := []string{}
	if _, hasShortRetry := metrics["short_retry"]; hasShortRetry {
		retryDims = append(retryDims, "short_retry")
	}
	if _, hasShortTimer := metrics["short_timer"]; hasShortTimer {
		retryDims = append(retryDims, "short_timer")
	}
	if _, hasLongRetry := metrics["long_retry"]; hasLongRetry {
		retryDims = append(retryDims, "long_retry")
	}
	if _, hasLongTimer := metrics["long_timer"]; hasLongTimer {
		retryDims = append(retryDims, "long_timer")
	}
	if len(retryDims) > 0 {
		chartID := fmt.Sprintf("channel_retry_config_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_retry_config",
			"Channel Retry Configuration",
			"value",
			"line",
			"channels",
			prioChannelStatus+3,
			retryDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range retryDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}

	// Limits configuration
	limitsDims := []string{}
	if _, hasMaxMsgLength := metrics["max_msg_length"]; hasMaxMsgLength {
		limitsDims = append(limitsDims, "max_msg_length")
	}
	if _, hasSharingConversations := metrics["sharing_conversations"]; hasSharingConversations {
		limitsDims = append(limitsDims, "sharing_conversations")
	}
	if _, hasNetworkPriority := metrics["network_priority"]; hasNetworkPriority {
		limitsDims = append(limitsDims, "network_priority")
	}
	if len(limitsDims) > 0 {
		chartID := fmt.Sprintf("channel_limits_config_%s", cleanName)
		c.ensureChartExists(
			"mq_pcf.channel_limits_config",
			"Channel Limits",
			"value",
			"line",
			"channels",
			prioChannelStatus+4,
			limitsDims,
			channelName,
			labels,
		)
		// Store metrics with dimension IDs that match chart expectations
		for _, dim := range limitsDims {
			if value, exists := metrics[dim]; exists {
				mx[fmt.Sprintf("%s_%s", chartID, dim)] = value
			}
		}
	}
}