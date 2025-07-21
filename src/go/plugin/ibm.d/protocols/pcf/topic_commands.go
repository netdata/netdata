// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

import (
	"fmt" 
	"path/filepath"
	"strings"
)

// #include "pcf_helpers.h"
import "C"


// GetTopicList returns a list of topics.
func (c *Client) GetTopicList() ([]string, error) {
	const pattern = "*"
	params := []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_NAME, pattern),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_TOPIC, params)
	if err != nil {
		return nil, err
	}

	// Parse the multi-message response
	result := c.parseTopicListResponse(response)
	
	// Log any errors encountered during parsing
	if result.InternalErrors > 0 {
		c.protocol.Warningf("encountered %d internal errors while parsing topic list", result.InternalErrors)
	}
	
	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			// Internal error
			c.protocol.Debugf("internal error %d occurred %d times", errCode, count)
		} else {
			// MQ error
			c.protocol.Debugf("MQ error %d (%s) occurred %d times", errCode, mqReasonString(errCode), count)
		}
	}
	
	return result.Topics, nil
}

// GetTopicMetrics returns metrics for a specific topic.
func (c *Client) GetTopicMetrics(topicString string) (*TopicMetrics, error) {
	c.protocol.Debugf("PCF: Getting metrics for topic '%s' from queue manager '%s'", topicString, c.config.QueueManager)
	
	params := []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_STRING, topicString),
		newIntParameter(C.MQIACF_TOPIC_STATUS_TYPE, C.MQIACF_TOPIC_PUB),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_TOPIC_STATUS, params)
	if err != nil {
		c.protocol.Errorf("PCF: Failed to get metrics for topic '%s' from queue manager '%s': %v", 
			topicString, c.config.QueueManager, err)
		return nil, err
	}

	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		c.protocol.Errorf("PCF: Failed to parse metrics response for topic '%s' from queue manager '%s': %v", 
			topicString, c.config.QueueManager, err)
		return nil, err
	}

	metrics := &TopicMetrics{
		TopicString: topicString,
	}
	
	// Get topic name (may be different from topic string)
	if name, ok := attrs[C.MQCA_TOPIC_NAME]; ok {
		metrics.Name = strings.TrimSpace(name.(string))
	} else {
		metrics.Name = topicString
	}
	
	// Publisher/Subscriber counts
	if publishers, ok := attrs[C.MQIA_PUB_COUNT]; ok {
		metrics.Publishers = int64(publishers.(int32))
	}
	if subscribers, ok := attrs[C.MQIA_SUB_COUNT]; ok {
		metrics.Subscribers = int64(subscribers.(int32))
	}
	
	// Message count
	if messages, ok := attrs[C.MQIAMO_PUBLISH_MSG_COUNT]; ok {
		metrics.PublishMsgCount = int64(messages.(int32))
	}
	
	// Last publication timestamps
	var lastPubDate, lastPubTime string
	if date, ok := attrs[C.MQCACF_LAST_PUB_DATE]; ok {
		lastPubDate = strings.TrimSpace(date.(string))
	}
	if time, ok := attrs[C.MQCACF_LAST_PUB_TIME]; ok {
		lastPubTime = strings.TrimSpace(time.(string))
	}
	
	// Convert to Unix timestamp if both date and time are available
	if lastPubDate != "" && lastPubTime != "" {
		if timestamp, err := ParseMQDateTime(lastPubDate, lastPubTime); err == nil {
			metrics.LastPubDate = AttributeValue(timestamp.Unix())
			metrics.LastPubTime = AttributeValue(timestamp.Unix())
		} else {
			c.protocol.Debugf("PCF: Failed to parse last publication timestamp for topic '%s': %v", topicString, err)
		}
	}

	return metrics, nil
}

// GetTopics collects comprehensive topic metrics with full transparency statistics
func (c *Client) GetTopics(collectMetrics bool, maxTopics int, selector string, collectSystem bool) (*TopicCollectionResult, error) {
	c.protocol.Debugf("PCF: Collecting topic metrics with selector '%s', max=%d, metrics=%v, system=%v", 
		selector, maxTopics, collectMetrics, collectSystem)
	
	result := &TopicCollectionResult{
		Stats: CollectionStats{},
	}
	
	// Step 1: Discovery - get all topic names
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_TOPIC, []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_NAME, "*"), // Always discover ALL topics
	})
	if err != nil {
		result.Stats.Discovery.Success = false
		c.protocol.Errorf("PCF: Topic discovery failed: %v", err)
		return result, fmt.Errorf("topic discovery failed: %w", err)
	}
	
	result.Stats.Discovery.Success = true
	
	// Parse discovery response
	parsed := c.parseTopicListResponse(response)
	
	// Track discovery statistics
	successfulItems := int64(len(parsed.Topics))
	
	// Count all errors as invisible items
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
			c.protocol.Warningf("PCF: Internal error %d occurred %d times during topic discovery", errCode, count)
		} else {
			c.protocol.Warningf("PCF: MQ error %d (%s) occurred %d times during topic discovery", 
				errCode, mqReasonString(errCode), count)
		}
	}
	
	if len(parsed.Topics) == 0 {
		c.protocol.Debugf("PCF: No topics discovered")
		return result, nil
	}
	
	// Step 2: Smart filtering decision
	visibleItems := result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems - result.Stats.Discovery.UnparsedItems
	enrichAll := maxTopics <= 0 || visibleItems <= int64(maxTopics)
	
	c.protocol.Debugf("PCF: Discovery found %d visible topics (total: %d, invisible: %d, unparsed: %d). EnrichAll=%v", 
		visibleItems, result.Stats.Discovery.AvailableItems, result.Stats.Discovery.InvisibleItems, 
		result.Stats.Discovery.UnparsedItems, enrichAll)
	
	// Step 3: Apply filtering
	var topicsToEnrich []string
	if enrichAll || selector == "*" {
		// Enrich everything we can see (with system filtering)
		for _, topicName := range parsed.Topics {
			// Filter out system topics if not wanted
			if !collectSystem && strings.HasPrefix(topicName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			topicsToEnrich = append(topicsToEnrich, topicName)
			result.Stats.Discovery.IncludedItems++
		}
		c.protocol.Debugf("PCF: Enriching %d topics (excluded %d system topics)", 
			len(topicsToEnrich), result.Stats.Discovery.ExcludedItems)
	} else {
		// Apply selector pattern and system filtering
		for _, topicName := range parsed.Topics {
			// Filter out system topics first if not wanted
			if !collectSystem && strings.HasPrefix(topicName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			
			matched, err := filepath.Match(selector, topicName)
			if err != nil {
				c.protocol.Warningf("PCF: Invalid selector pattern '%s': %v", selector, err)
				matched = false
			}
			
			if matched {
				topicsToEnrich = append(topicsToEnrich, topicName)
				result.Stats.Discovery.IncludedItems++
			} else {
				result.Stats.Discovery.ExcludedItems++
			}
		}
		c.protocol.Debugf("PCF: Selector '%s' matched %d topics, excluded %d (including system filtering)", 
			selector, result.Stats.Discovery.IncludedItems, result.Stats.Discovery.ExcludedItems)
	}
	
	// Step 4: Enrich selected topics
	for _, topicName := range topicsToEnrich {
		tm := TopicMetrics{TopicString: topicName, Name: topicName}
		
		// Collect metrics if requested
		if collectMetrics {
			if result.Stats.Metrics == nil {
				result.Stats.Metrics = &EnrichmentStats{
					TotalItems:  int64(len(topicsToEnrich)),
					ErrorCounts: make(map[int32]int),
				}
			}
			
			metricsData, err := c.GetTopicMetrics(topicName)
			if err != nil {
				result.Stats.Metrics.FailedItems++
				if pcfErr, ok := err.(*PCFError); ok {
					result.Stats.Metrics.ErrorCounts[pcfErr.Code]++
				} else {
					result.Stats.Metrics.ErrorCounts[-1]++
				}
				c.protocol.Debugf("PCF: Failed to get metrics for topic '%s': %v", topicName, err)
			} else {
				result.Stats.Metrics.OkItems++
				
				// Copy metrics data
				tm.Name = metricsData.Name
				tm.TopicString = metricsData.TopicString
				tm.Publishers = metricsData.Publishers
				tm.Subscribers = metricsData.Subscribers
				tm.PublishMsgCount = metricsData.PublishMsgCount
				tm.LastPubDate = metricsData.LastPubDate
				tm.LastPubTime = metricsData.LastPubTime
			}
		}
		
		result.Topics = append(result.Topics, tm)
	}
	
	// Log summary
	c.protocol.Infof("PCF: Topic collection complete - discovered:%d visible:%d included:%d enriched:%d", 
		result.Stats.Discovery.AvailableItems, 
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Topics))
	
	return result, nil
}