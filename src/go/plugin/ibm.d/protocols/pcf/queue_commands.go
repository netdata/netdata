// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
)


// QueueInfo contains queue information including type
type QueueInfo struct {
	Name string
	Type int32 // MQQT_LOCAL, MQQT_ALIAS, MQQT_REMOTE, MQQT_MODEL, MQQT_CLUSTER
	
	// Basic metrics (available from both discovery and individual queries)
	CurrentDepth int64
	MaxDepth     int64
	
	// Configuration attributes (extracted from individual queue queries)
	InhibitGet       AttributeValue
	InhibitPut       AttributeValue
	BackoutThreshold AttributeValue
	TriggerDepth     AttributeValue
	TriggerType      AttributeValue
	MaxMsgLength     AttributeValue
	DefPriority      AttributeValue
}

// QueueTypeString converts MQ queue type to string for labels
func QueueTypeString(qtype int32) string {
	switch qtype {
	case C.MQQT_LOCAL:
		return "local"
	case C.MQQT_MODEL:
		return "model"
	case C.MQQT_ALIAS:
		return "alias"
	case C.MQQT_REMOTE:
		return "remote"
	case C.MQQT_CLUSTER:
		return "cluster"
	default:
		return "unknown"
	}
}

// GetQueueList returns a list of queues with their types.
func (c *Client) GetQueueList() ([]QueueInfo, error) {
	c.protocol.Debugf("PCF: Getting queue list from queue manager '%s'", c.config.QueueManager)
	
	// Request all queues - only specify queue name pattern
	// Don't specify MQIA_Q_TYPE as it's not a filter but an attribute selector
	const pattern = "*"
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, pattern),
	}
	
	// Debug: Log the actual parameter details
	c.protocol.Debugf("PCF: Queue name parameter - value='%s', length=%d", pattern, len(pattern))
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get queue list from queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	// Parse the multi-message response with type information
	result := c.parseQueueListResponseWithType(response)
	
	// Log any errors encountered during parsing
	if result.InternalErrors > 0 {
		c.protocol.Warningf("PCF: Encountered %d internal errors while parsing queue list from queue manager '%s'", 
			result.InternalErrors, c.config.QueueManager)
	}
	
	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			// Internal error
			c.protocol.Warningf("PCF: Internal error %d occurred %d times while parsing queue list from queue manager '%s'", 
				errCode, count, c.config.QueueManager)
		} else {
			// MQ error
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times while parsing queue list from queue manager '%s'", 
				errCode, mqReasonString(errCode), count, c.config.QueueManager)
		}
	}
	
	c.protocol.Debugf("PCF: Retrieved %d queues from queue manager '%s'", len(result.Queues), c.config.QueueManager)
	
	return result.Queues, nil
}

// GetQueueMetrics collects comprehensive queue metrics using 3N pattern for full configuration data
func (c *Client) GetQueueMetrics(config QueueCollectionConfig) ([]QueueMetrics, error) {
	c.protocol.Debugf("PCF: Collecting queue metrics with pattern '%s', reset_stats=%v", 
		config.QueuePattern, config.CollectResetStats)
	
	// Step 1: Discovery - get queue names only (1 request for queue list)
	queueNames, err := c.getQueueNamesList(config.QueuePattern)
	if err != nil {
		return nil, fmt.Errorf("failed to discover queues: %w", err)
	}
	
	if len(queueNames) == 0 {
		c.protocol.Debugf("PCF: No queues match pattern '%s'", config.QueuePattern)
		return nil, nil
	}
	
	c.protocol.Debugf("PCF: Found %d queues matching pattern '%s'", len(queueNames), config.QueuePattern)
	
	var results []QueueMetrics
	var errors []string
	
	// Step 2: Collect detailed metrics for each queue (3N pattern)
	for _, queueName := range queueNames {
		// Step 2a: Get full configuration data (1 request per queue)
		configData, err := c.getQueueConfiguration(queueName)
		if err != nil {
			c.protocol.Warningf("PCF: Failed to get configuration for queue '%s': %v", queueName, err)
			errors = append(errors, fmt.Sprintf("configuration collection failed for queue %s: %v", queueName, err))
			continue // Skip this queue entirely if we can't get basic config
		}
		
		metrics := QueueMetrics{
			Name:             queueName,
			Type:             QueueType(configData.Type),
			CurrentDepth:     configData.CurrentDepth,
			MaxDepth:         configData.MaxDepth,
			// Configuration data from individual queue query
			InhibitGet:       configData.InhibitGet,
			InhibitPut:       configData.InhibitPut,
			BackoutThreshold: configData.BackoutThreshold,
			TriggerDepth:     configData.TriggerDepth,
			TriggerType:      configData.TriggerType,
			MaxMsgLength:     configData.MaxMsgLength,
			DefPriority:      configData.DefPriority,
		}
		
		// Step 2b: Get runtime status (1 request per queue)
		if err := c.enrichWithStatus(&metrics); err != nil {
			c.protocol.Warningf("PCF: Failed to get status for queue '%s': %v", queueName, err)
			errors = append(errors, fmt.Sprintf("status collection failed for queue %s: %v", queueName, err))
			// Continue with partial data
		}
		
		// Step 2c: Get reset statistics if requested (1 request per queue, optional)
		if config.CollectResetStats {
			if err := c.enrichWithResetStats(&metrics); err != nil {
				c.protocol.Debugf("PCF: Failed to get reset stats for queue '%s': %v", queueName, err)
				// Don't add to errors - reset stats are optional and not all queues support them
			}
		}
		
		results = append(results, metrics)
	}
	
	// Log summary
	c.protocol.Infof("PCF: Collected metrics for %d queues (pattern: '%s', reset_stats: %v, 3N requests)", 
		len(results), config.QueuePattern, config.CollectResetStats)
	
	if len(errors) > 0 {
		c.protocol.Warningf("PCF: Encountered %d errors during collection: %v", len(errors), errors)
	}
	
	return results, nil
}

// getQueueNamesList gets queue names matching pattern (lightweight discovery)
func (c *Client) getQueueNamesList(pattern string) ([]string, error) {
	c.protocol.Debugf("PCF: Getting queue names list using pattern '%s'", pattern)
	
	// Use simple discovery call - only get queue names, not full data
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, pattern),
	}
	
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get queue names from queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	// Parse response to extract just queue names
	result := c.parseQueueListResponse(response)
	
	// Log any errors encountered during parsing
	if result.InternalErrors > 0 {
		c.protocol.Warningf("PCF: Encountered %d internal errors while parsing queue names from queue manager '%s'", 
			result.InternalErrors, c.config.QueueManager)
	}
	
	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			// Internal error
			c.protocol.Warningf("PCF: Internal error %d occurred %d times while parsing queue names from queue manager '%s'", 
				errCode, count, c.config.QueueManager)
		} else {
			// MQ error
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times while parsing queue names from queue manager '%s'", 
				errCode, mqReasonString(errCode), count, c.config.QueueManager)
		}
	}
	
	c.protocol.Debugf("PCF: Retrieved %d queue names from queue manager '%s'", len(result.Queues), c.config.QueueManager)
	
	return result.Queues, nil
}

// getQueueConfiguration gets full configuration data for a specific queue 
func (c *Client) getQueueConfiguration(queueName string) (*QueueInfo, error) {
	c.protocol.Debugf("PCF: Getting configuration for queue '%s'", queueName)
	
	// Use individual queue query to get full configuration data
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}
	
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		return nil, fmt.Errorf("failed to get configuration for queue '%s': %w", queueName, err)
	}

	// Parse single queue response
	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		return nil, fmt.Errorf("failed to parse configuration response for queue '%s': %w", queueName, err)
	}

	// Extract configuration data
	queueInfo := &QueueInfo{
		Name: queueName,
	}
	
	// Extract queue type
	if qtype, ok := attrs[C.MQIA_Q_TYPE]; ok {
		if qtypeInt, ok := qtype.(int32); ok {
			queueInfo.Type = qtypeInt
		}
	}
	
	// Extract basic depth metrics if available
	if depth, ok := attrs[C.MQIA_CURRENT_Q_DEPTH]; ok {
		if depthInt, ok := depth.(int32); ok {
			queueInfo.CurrentDepth = int64(depthInt)
		}
	}
	if maxDepth, ok := attrs[C.MQIA_MAX_Q_DEPTH]; ok {
		if maxDepthInt, ok := maxDepth.(int32); ok {
			queueInfo.MaxDepth = int64(maxDepthInt)
		}
	}
	
	// Extract configuration attributes - initialize all to NotCollected, only set when actually present
	queueInfo.InhibitGet = NotCollected
	queueInfo.InhibitPut = NotCollected
	queueInfo.BackoutThreshold = NotCollected
	queueInfo.TriggerDepth = NotCollected
	queueInfo.TriggerType = NotCollected
	queueInfo.MaxMsgLength = NotCollected
	queueInfo.DefPriority = NotCollected
	
	if attr, ok := attrs[C.MQIA_INHIBIT_GET]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.InhibitGet = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_INHIBIT_PUT]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.InhibitPut = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_BACKOUT_THRESHOLD]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.BackoutThreshold = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_TRIGGER_DEPTH]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.TriggerDepth = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_TRIGGER_TYPE]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.TriggerType = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_MAX_MSG_LENGTH]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.MaxMsgLength = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_DEF_PRIORITY]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.DefPriority = AttributeValue(val)
		}
	}
	
	c.protocol.Debugf("PCF: Retrieved configuration for queue '%s' (type: %d)", 
		queueName, queueInfo.Type)
	
	return queueInfo, nil
}

// getQueueListWithBasicMetrics gets queue list with basic depth metrics from discovery
func (c *Client) getQueueListWithBasicMetrics(pattern string) ([]QueueMetrics, error) {
	c.protocol.Debugf("PCF: Getting queue list with basic metrics using pattern '%s'", pattern)
	
	// Use exact same parameters as working GetQueueList() method
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, pattern),
	}
	
	c.protocol.Debugf("PCF: Sending MQCMD_INQUIRE_Q with pattern='%s'", pattern)
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get queue list from queue manager '%s': %v", c.config.QueueManager, err)
		return nil, err
	}

	// Parse the multi-message response with type information
	result := c.parseQueueListResponseWithType(response)
	
	// Log any errors encountered during parsing
	if result.InternalErrors > 0 {
		c.protocol.Warningf("PCF: Encountered %d internal errors while parsing queue list from queue manager '%s'", 
			result.InternalErrors, c.config.QueueManager)
	}
	
	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			// Internal error
			c.protocol.Warningf("PCF: Internal error %d occurred %d times while parsing queue list from queue manager '%s'", 
				errCode, count, c.config.QueueManager)
		} else {
			// MQ error
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times while parsing queue list from queue manager '%s'", 
				errCode, mqReasonString(errCode), count, c.config.QueueManager)
		}
	}
	
	// Convert QueueInfo to QueueMetrics with basic data and configuration
	var metrics []QueueMetrics
	for _, queueInfo := range result.Queues {
		metrics = append(metrics, QueueMetrics{
			Name:         queueInfo.Name,
			Type:         QueueType(queueInfo.Type),
			CurrentDepth: 0, // Will be filled from detailed query if available
			MaxDepth:     0, // Will be filled from detailed query if available
			// Configuration data from discovery call
			InhibitGet:       queueInfo.InhibitGet,
			InhibitPut:       queueInfo.InhibitPut,
			BackoutThreshold: queueInfo.BackoutThreshold,
			TriggerDepth:     queueInfo.TriggerDepth,
			TriggerType:      queueInfo.TriggerType,
			MaxMsgLength:     queueInfo.MaxMsgLength,
			DefPriority:      queueInfo.DefPriority,
			// Note: Individual config attributes now tracked per-attribute with AttributeValue type
		})
	}
	
	c.protocol.Debugf("PCF: Retrieved %d queues from queue manager '%s'", len(metrics), c.config.QueueManager)
	
	return metrics, nil
}

// enrichWithStatus enriches queue metrics with runtime status
func (c *Client) enrichWithStatus(metrics *QueueMetrics) error {
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, metrics.Name),
		newIntParameter(C.MQIA_Q_TYPE, C.MQQT_ALL),
		newIntParameter(C.MQIACF_Q_STATUS_ATTRS, C.MQIACF_ALL),
	}
	
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q_STATUS, params)
	if err != nil {
		return err
	}
	
	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		return err
	}
	
	// Extract basic depth metrics (if not from discovery)
	if depth, ok := attrs[C.MQIA_CURRENT_Q_DEPTH]; ok {
		metrics.CurrentDepth = int64(depth.(int32))
	}
	if maxDepth, ok := attrs[C.MQIA_MAX_Q_DEPTH]; ok {
		metrics.MaxDepth = int64(maxDepth.(int32))
	}
	
	// Extract status metrics
	if val, ok := attrs[C.MQIA_OPEN_INPUT_COUNT]; ok {
		metrics.OpenInputCount = int64(val.(int32))
	}
	if val, ok := attrs[C.MQIA_OPEN_OUTPUT_COUNT]; ok {
		metrics.OpenOutputCount = int64(val.(int32))
	}
	if val, ok := attrs[C.MQIACF_OLDEST_MSG_AGE]; ok {
		metrics.OldestMsgAge = int64(val.(int32))
	}
	if val, ok := attrs[C.MQIACF_UNCOMMITTED_MSGS]; ok {
		metrics.UncommittedMsgs = int64(val.(int32))
	}
	
	// Mark as successfully collected
	metrics.HasStatusMetrics = true
	return nil
}

// enrichWithResetStats enriches queue metrics with reset statistics
func (c *Client) enrichWithResetStats(metrics *QueueMetrics) error {
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, metrics.Name),
	}
	
	response, err := c.SendPCFCommand(C.MQCMD_RESET_Q_STATS, params)
	if err != nil {
		return err
	}
	
	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		return err
	}
	
	// Extract reset statistics
	if val, ok := attrs[C.MQIA_MSG_ENQ_COUNT]; ok {
		metrics.EnqueueCount = int64(val.(int32))
	}
	if val, ok := attrs[C.MQIA_MSG_DEQ_COUNT]; ok {
		metrics.DequeueCount = int64(val.(int32))
	}
	if val, ok := attrs[C.MQIA_HIGH_Q_DEPTH]; ok {
		metrics.HighDepth = int64(val.(int32))
	}
	if val, ok := attrs[C.MQIA_TIME_SINCE_RESET]; ok {
		metrics.TimeSinceReset = int64(val.(int32))
	}
	
	// Mark as successfully collected
	metrics.HasResetStats = true
	return nil
}

// Legacy method for backward compatibility - will be removed
func (c *Client) GetQueueMetrics_Legacy(queueName string) (*QueueMetrics, error) {
	config := QueueCollectionConfig{
		QueuePattern:      queueName,
		CollectResetStats: false,
	}
	
	results, err := c.GetQueueMetrics(config)
	if err != nil {
		return nil, err
	}
	
	if len(results) == 0 {
		return nil, fmt.Errorf("queue not found: %s", queueName)
	}
	
	return &results[0], nil
}

// GetQueueConfig returns configuration for a specific queue.
func (c *Client) GetQueueConfig(queueName string) (*QueueConfig, error) {
	params := []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, queueName),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q, params)
	if err != nil {
		return nil, err
	}

	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		return nil, err
	}

	config := &QueueConfig{
		Name: queueName,
	}
	
	// Get queue type
	if queueType, ok := attrs[C.MQIA_Q_TYPE]; ok {
		config.Type = QueueType(queueType.(int32))
	}
	
	// Inhibit settings
	if inhibitGet, ok := attrs[C.MQIA_INHIBIT_GET]; ok {
		config.InhibitGet = int64(inhibitGet.(int32))
	}
	if inhibitPut, ok := attrs[C.MQIA_INHIBIT_PUT]; ok {
		config.InhibitPut = int64(inhibitPut.(int32))
	}
	
	// Thresholds and triggers
	if backoutThreshold, ok := attrs[C.MQIA_BACKOUT_THRESHOLD]; ok {
		config.BackoutThreshold = int64(backoutThreshold.(int32))
	}
	if triggerDepth, ok := attrs[C.MQIA_TRIGGER_DEPTH]; ok {
		config.TriggerDepth = int64(triggerDepth.(int32))
	}
	if triggerType, ok := attrs[C.MQIA_TRIGGER_TYPE]; ok {
		config.TriggerType = int64(triggerType.(int32))
	}
	
	// Limits
	if maxMsgLength, ok := attrs[C.MQIA_MAX_MSG_LENGTH]; ok {
		config.MaxMsgLength = int64(maxMsgLength.(int32))
	}
	if defPriority, ok := attrs[C.MQIA_DEF_PRIORITY]; ok {
		config.DefPriority = int64(defPriority.(int32))
	}

	return config, nil
}