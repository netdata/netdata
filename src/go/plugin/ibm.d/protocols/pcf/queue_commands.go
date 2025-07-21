// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)


// QueueInfo contains queue information including type
type QueueInfo struct {
	Name string
	Type int32 // MQQT_LOCAL, MQQT_ALIAS, MQQT_REMOTE, MQQT_MODEL, MQQT_CLUSTER
	
	// Basic metrics (available from both discovery and individual queries)
	CurrentDepth int64
	MaxDepth     int64
	
	// Configuration attributes (extracted from individual queue queries)
	InhibitGet          AttributeValue
	InhibitPut          AttributeValue
	BackoutThreshold    AttributeValue
	TriggerDepth        AttributeValue
	TriggerType         AttributeValue
	MaxMsgLength        AttributeValue
	DefPriority         AttributeValue
	ServiceInterval     AttributeValue
	RetentionInterval   AttributeValue
	Scope               AttributeValue
	Usage               AttributeValue
	MsgDeliverySequence AttributeValue
	HardenGetBackout    AttributeValue
	DefPersistence      AttributeValue
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

// GetQueues collects comprehensive queue metrics with full transparency statistics
func (c *Client) GetQueues(collectConfig, collectMetrics, collectReset bool, maxQueues int, selector string, collectSystem bool) (*QueueCollectionResult, error) {
	c.protocol.Debugf("PCF: Collecting queue metrics with selector '%s', max=%d, config=%v, metrics=%v, reset=%v, system=%v", 
		selector, maxQueues, collectConfig, collectMetrics, collectReset, collectSystem)
	
	result := &QueueCollectionResult{
		Stats: CollectionStats{},
	}
	
	// Step 1: Discovery - get all queue names
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_Q, []pcfParameter{
		newStringParameter(C.MQCA_Q_NAME, "*"), // Always discover ALL queues
	})
	if err != nil {
		result.Stats.Discovery.Success = false
		c.protocol.Errorf("PCF: Queue discovery failed: %v", err)
		return result, fmt.Errorf("queue discovery failed: %w", err)
	}
	
	result.Stats.Discovery.Success = true
	
	// Parse discovery response
	parsed := c.parseQueueListResponse(response)
	
	// Track discovery statistics
	successfulItems := int64(len(parsed.Queues))
	
	// Count all errors as invisible items (ErrorCounts includes both MQ errors and internal errors)
	var invisibleItems int64
	for _, count := range parsed.ErrorCounts {
		invisibleItems += int64(count)
	}
	
	result.Stats.Discovery.AvailableItems = successfulItems + invisibleItems
	result.Stats.Discovery.InvisibleItems = invisibleItems
	result.Stats.Discovery.UnparsedItems = 0 // No unparsed items at discovery level
	result.Stats.Discovery.ErrorCounts = parsed.ErrorCounts
	
	// Log discovery errors
	for errCode, count := range parsed.ErrorCounts {
		if errCode < 0 {
			c.protocol.Warningf("PCF: Internal error %d occurred %d times during queue discovery", errCode, count)
		} else {
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times during queue discovery", 
				errCode, mqReasonString(errCode), count)
		}
	}
	
	if len(parsed.Queues) == 0 {
		c.protocol.Debugf("PCF: No queues discovered")
		return result, nil
	}
	
	// Step 2: Smart filtering decision
	visibleItems := result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems - result.Stats.Discovery.UnparsedItems
	enrichAll := maxQueues <= 0 || visibleItems <= int64(maxQueues)
	
	c.protocol.Debugf("PCF: Discovery found %d visible queues (total: %d, invisible: %d, unparsed: %d). EnrichAll=%v", 
		visibleItems, result.Stats.Discovery.AvailableItems, result.Stats.Discovery.InvisibleItems, 
		result.Stats.Discovery.UnparsedItems, enrichAll)
	
	// Step 3: Apply filtering
	var queuesToEnrich []string
	if enrichAll || selector == "*" {
		// Enrich everything we can see (with system filtering)
		for _, queueName := range parsed.Queues {
			// Filter out system queues if not wanted
			if !collectSystem && strings.HasPrefix(queueName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			queuesToEnrich = append(queuesToEnrich, queueName)
			result.Stats.Discovery.IncludedItems++
		}
		c.protocol.Debugf("PCF: Enriching %d queues (excluded %d system queues)", 
			len(queuesToEnrich), result.Stats.Discovery.ExcludedItems)
	} else {
		// Apply selector pattern and system filtering
		for _, queueName := range parsed.Queues {
			// Filter out system queues first if not wanted
			if !collectSystem && strings.HasPrefix(queueName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			
			matched, err := filepath.Match(selector, queueName)
			if err != nil {
				c.protocol.Warningf("PCF: Invalid selector pattern '%s': %v", selector, err)
				matched = false
			}
			
			if matched {
				queuesToEnrich = append(queuesToEnrich, queueName)
				result.Stats.Discovery.IncludedItems++
			} else {
				result.Stats.Discovery.ExcludedItems++
			}
		}
		c.protocol.Debugf("PCF: Selector '%s' matched %d queues, excluded %d (including system filtering)", 
			selector, result.Stats.Discovery.IncludedItems, result.Stats.Discovery.ExcludedItems)
	}
	
	// Step 4: Enrich selected queues
	for _, queueName := range queuesToEnrich {
		qm := QueueMetrics{Name: queueName}
		
		// 4a: Collect configuration if requested
		if collectConfig {
			if result.Stats.Config == nil {
				result.Stats.Config = &EnrichmentStats{
					TotalItems:  int64(len(queuesToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}
			
			configData, err := c.getQueueConfiguration(queueName)
			if err != nil {
				result.Stats.Config.FailedItems++
				// Extract error code if possible
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Config.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Config.ErrorCounts[-1]++ // Unknown error
				}
				c.protocol.Debugf("PCF: Failed to get config for queue '%s': %v", queueName, err)
				// Continue with next queue - don't add incomplete data
				continue
			}
			
			result.Stats.Config.OkItems++
			
			// Copy config data
			qm.Type = QueueType(configData.Type)
			qm.CurrentDepth = configData.CurrentDepth
			qm.MaxDepth = configData.MaxDepth
			qm.InhibitGet = configData.InhibitGet
			qm.InhibitPut = configData.InhibitPut
			qm.BackoutThreshold = configData.BackoutThreshold
			qm.TriggerDepth = configData.TriggerDepth
			qm.TriggerType = configData.TriggerType
			qm.MaxMsgLength = configData.MaxMsgLength
			qm.DefPriority = configData.DefPriority
		}
		
		// 4b: Collect metrics if requested
		if collectMetrics {
			if result.Stats.Metrics == nil {
				result.Stats.Metrics = &EnrichmentStats{
					TotalItems:  int64(len(queuesToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}
			
			err := c.enrichWithStatus(&qm)
			if err != nil {
				result.Stats.Metrics.FailedItems++
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Metrics.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Metrics.ErrorCounts[-1]++
				}
				c.protocol.Debugf("PCF: Failed to get metrics for queue '%s': %v", queueName, err)
			} else {
				result.Stats.Metrics.OkItems++
			}
		}
		
		// 4c: Collect reset stats if requested
		if collectReset {
			if result.Stats.Reset == nil {
				result.Stats.Reset = &EnrichmentStats{
					TotalItems:  int64(len(queuesToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}
			
			err := c.enrichWithResetStats(&qm)
			if err != nil {
				result.Stats.Reset.FailedItems++
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Reset.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Reset.ErrorCounts[-1]++
				}
				// Reset stats failures are not critical - many queues don't support them
				c.protocol.Debugf("PCF: Failed to get reset stats for queue '%s': %v", queueName, err)
			} else {
				result.Stats.Reset.OkItems++
			}
		}
		
		result.Queues = append(result.Queues, qm)
	}
	
	// Log summary
	c.protocol.Debugf("PCF: Queue collection complete - discovered:%d visible:%d included:%d enriched:%d", 
		result.Stats.Discovery.AvailableItems, 
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Queues))
	
	return result, nil
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
	queueInfo.ServiceInterval = NotCollected
	queueInfo.RetentionInterval = NotCollected
	queueInfo.Scope = NotCollected
	queueInfo.Usage = NotCollected
	queueInfo.MsgDeliverySequence = NotCollected
	queueInfo.HardenGetBackout = NotCollected
	queueInfo.DefPersistence = NotCollected
	
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
	
	// Extract new configuration attributes
	if attr, ok := attrs[C.MQIA_Q_SERVICE_INTERVAL]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.ServiceInterval = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_RETENTION_INTERVAL]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.RetentionInterval = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_SCOPE]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.Scope = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_USAGE]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.Usage = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_MSG_DELIVERY_SEQUENCE]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.MsgDeliverySequence = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_HARDEN_GET_BACKOUT]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.HardenGetBackout = AttributeValue(val)
		}
	}
	
	if attr, ok := attrs[C.MQIA_DEF_PERSISTENCE]; ok {
		if val, ok := attr.(int32); ok {
			queueInfo.DefPersistence = AttributeValue(val)
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
			InhibitGet:          queueInfo.InhibitGet,
			InhibitPut:          queueInfo.InhibitPut,
			BackoutThreshold:    queueInfo.BackoutThreshold,
			TriggerDepth:        queueInfo.TriggerDepth,
			TriggerType:         queueInfo.TriggerType,
			MaxMsgLength:        queueInfo.MaxMsgLength,
			DefPriority:         queueInfo.DefPriority,
			ServiceInterval:     queueInfo.ServiceInterval,
			RetentionInterval:   queueInfo.RetentionInterval,
			Scope:               queueInfo.Scope,
			Usage:               queueInfo.Usage,
			MsgDeliverySequence: queueInfo.MsgDeliverySequence,
			HardenGetBackout:    queueInfo.HardenGetBackout,
			DefPersistence:      queueInfo.DefPersistence,
			// Status metrics initialized to NotCollected
			OpenInputCount:  NotCollected,
			OpenOutputCount: NotCollected,
			OldestMsgAge:    NotCollected,
			UncommittedMsgs: NotCollected,
			LastGetDate:     NotCollected,
			LastGetTime:     NotCollected,
			LastPutDate:     NotCollected,
			LastPutTime:     NotCollected,
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
	
	// Initialize status metrics to NotCollected
	metrics.OpenInputCount = NotCollected
	metrics.OpenOutputCount = NotCollected
	metrics.OldestMsgAge = NotCollected
	metrics.UncommittedMsgs = NotCollected
	metrics.LastGetDate = NotCollected
	metrics.LastGetTime = NotCollected
	metrics.LastPutDate = NotCollected
	metrics.LastPutTime = NotCollected
	
	// Extract status metrics - only set if actually present
	if val, ok := attrs[C.MQIA_OPEN_INPUT_COUNT]; ok {
		metrics.OpenInputCount = AttributeValue(val.(int32))
	}
	if val, ok := attrs[C.MQIA_OPEN_OUTPUT_COUNT]; ok {
		metrics.OpenOutputCount = AttributeValue(val.(int32))
	}
	if val, ok := attrs[C.MQIACF_OLDEST_MSG_AGE]; ok {
		metrics.OldestMsgAge = AttributeValue(val.(int32))
	}
	if val, ok := attrs[C.MQIACF_UNCOMMITTED_MSGS]; ok {
		metrics.UncommittedMsgs = AttributeValue(val.(int32))
	}
	
	// Extract last get/put date and time - string attributes
	if val, ok := attrs[C.MQCACF_LAST_GET_DATE]; ok {
		if dateStr, ok := val.(string); ok && dateStr != "" {
			// Convert YYYYMMDD string to int64
			if dateInt, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
				metrics.LastGetDate = AttributeValue(dateInt)
			}
		}
	}
	if val, ok := attrs[C.MQCACF_LAST_GET_TIME]; ok {
		if timeStr, ok := val.(string); ok && timeStr != "" {
			// Convert HHMMSSSS string to int64
			if timeInt, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
				metrics.LastGetTime = AttributeValue(timeInt)
			}
		}
	}
	if val, ok := attrs[C.MQCACF_LAST_PUT_DATE]; ok {
		if dateStr, ok := val.(string); ok && dateStr != "" {
			// Convert YYYYMMDD string to int64
			if dateInt, err := strconv.ParseInt(dateStr, 10, 64); err == nil {
				metrics.LastPutDate = AttributeValue(dateInt)
			}
		}
	}
	if val, ok := attrs[C.MQCACF_LAST_PUT_TIME]; ok {
		if timeStr, ok := val.(string); ok && timeStr != "" {
			// Convert HHMMSSSS string to int64
			if timeInt, err := strconv.ParseInt(timeStr, 10, 64); err == nil {
				metrics.LastPutTime = AttributeValue(timeInt)
			}
		}
	}
	
	// Extract average queue time
	if val, ok := attrs[C.MQIACF_Q_TIME_INDICATOR]; ok {
		if timeVal, ok := val.(int32); ok {
			metrics.AvgQueueTime = AttributeValue(timeVal)
			c.protocol.Debugf("PCF: Queue '%s' average queue time: %d microseconds", metrics.Name, timeVal)
		}
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