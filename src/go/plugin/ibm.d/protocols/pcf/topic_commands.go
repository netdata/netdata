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

	result := c.parseTopicListResponse(response)

	if result.InternalErrors > 0 {
		c.protocol.Warningf("encountered %d internal errors while parsing topic list", result.InternalErrors)
	}

	for errCode, count := range result.ErrorCounts {
		if errCode < 0 {
			c.protocol.Debugf("internal error %d occurred %d times", errCode, count)
		} else {
			c.protocol.Debugf("MQ error %d (%s) occurred %d times", errCode, mqReasonString(errCode), count)
		}
	}

	return result.Topics, nil
}

// GetTopicMetrics returns metrics for a specific topic.
func (c *Client) GetTopicMetrics(topicString string) (*TopicMetrics, error) {
	c.protocol.Debugf("Getting metrics for topic '%s' from queue manager '%s'", topicString, c.config.QueueManager)

	params := []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_STRING, topicString),
		newIntParameter(C.MQIACF_TOPIC_STATUS_TYPE, C.MQIACF_TOPIC_PUB),
	}
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_TOPIC_STATUS, params)
	if err != nil {
		c.protocol.Errorf("Failed to get metrics for topic '%s' from queue manager '%s': %v",
			topicString, c.config.QueueManager, err)
		return nil, err
	}

	attrs, err := c.ParsePCFResponse(response, "")
	if err != nil {
		c.protocol.Errorf("Failed to parse metrics response for topic '%s' from queue manager '%s': %v",
			topicString, c.config.QueueManager, err)
		return nil, err
	}

	metrics := &TopicMetrics{
		TopicString: topicString,
	}

	if name, ok := attrs[C.MQCA_TOPIC_NAME]; ok {
		metrics.Name = strings.TrimSpace(name.(string))
	} else {
		metrics.Name = topicString
	}

	if publishers, ok := attrs[C.MQIA_PUB_COUNT]; ok {
		metrics.Publishers = int64(publishers.(int32))
	}
	if subscribers, ok := attrs[C.MQIA_SUB_COUNT]; ok {
		metrics.Subscribers = int64(subscribers.(int32))
	}

	if messages, ok := attrs[C.MQIAMO_PUBLISH_MSG_COUNT]; ok {
		metrics.PublishMsgCount = int64(messages.(int32))
	}

	var lastPubDate, lastPubTime string
	if date, ok := attrs[C.MQCACF_LAST_PUB_DATE]; ok {
		lastPubDate = strings.TrimSpace(date.(string))
	}
	if time, ok := attrs[C.MQCACF_LAST_PUB_TIME]; ok {
		lastPubTime = strings.TrimSpace(time.(string))
	}

	if lastPubDate != "" && lastPubTime != "" {
		if timestamp, err := ParseMQDateTime(lastPubDate, lastPubTime); err == nil {
			metrics.LastPubDate = AttributeValue(timestamp.Unix())
			metrics.LastPubTime = AttributeValue(timestamp.Unix())
		} else {
			c.protocol.Debugf("Failed to parse last publication timestamp for topic '%s': %v", topicString, err)
		}
	}

	return metrics, nil
}

// GetTopics collects comprehensive topic metrics with full transparency statistics
func (c *Client) GetTopics(collectMetrics bool, maxTopics int, selector string, collectSystem bool) (*TopicCollectionResult, error) {
	c.protocol.Debugf("Collecting topic metrics with selector '%s', max=%d, metrics=%v, system=%v",
		selector, maxTopics, collectMetrics, collectSystem)

	result := &TopicCollectionResult{
		Stats: CollectionStats{},
	}

	// Step 1: Discovery
	discoveredTopics, err := c.discoverTopics(result)
	if err != nil {
		return result, err
	}

	// Step 2: Filtering
	topicsToEnrich := c.filterTopics(discoveredTopics, selector, collectSystem, maxTopics, result)

	// Step 3: Enrichment
	c.enrichTopics(topicsToEnrich, collectMetrics, result)

	c.protocol.Debugf("Topic collection complete - discovered:%d visible:%d included:%d enriched:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems-result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Topics))

	return result, nil
}

func (c *Client) discoverTopics(result *TopicCollectionResult) ([]string, error) {
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_TOPIC, []pcfParameter{
		newStringParameter(C.MQCA_TOPIC_NAME, "*"),
	})
	if err != nil {
		result.Stats.Discovery.Success = false
		c.protocol.Errorf("Topic discovery failed: %v", err)
		return nil, fmt.Errorf("topic discovery failed: %w", err)
	}

	result.Stats.Discovery.Success = true
	parsed := c.parseTopicListResponse(response)

	successfulItems := int64(len(parsed.Topics))
	var invisibleItems int64
	for _, count := range parsed.ErrorCounts {
		invisibleItems += int64(count)
	}

	result.Stats.Discovery.AvailableItems = successfulItems + invisibleItems
	result.Stats.Discovery.InvisibleItems = invisibleItems
	result.Stats.Discovery.ErrorCounts = parsed.ErrorCounts

	for errCode, count := range parsed.ErrorCounts {
		if errCode < 0 {
			c.protocol.Warningf("Internal error %d occurred %d times during topic discovery", errCode, count)
		} else {
			c.protocol.Warningf("MQ error %d (%s) occurred %d times during topic discovery",
				errCode, mqReasonString(errCode), count)
		}
	}

	if len(parsed.Topics) == 0 {
		c.protocol.Debugf("No topics discovered")
	}

	return parsed.Topics, nil
}

func (c *Client) filterTopics(topics []string, selector string, collectSystem bool, maxTopics int, result *TopicCollectionResult) []string {
	visibleItems := result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems
	enrichAll := maxTopics <= 0 || visibleItems <= int64(maxTopics)

	c.protocol.Debugf("Discovery found %d visible topics (total: %d, invisible: %d). EnrichAll=%v",
		visibleItems, result.Stats.Discovery.AvailableItems, result.Stats.Discovery.InvisibleItems, enrichAll)

	var topicsToEnrich []string
	if enrichAll || selector == "*" {
		for _, topicName := range topics {
			if !collectSystem && strings.HasPrefix(topicName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}
			topicsToEnrich = append(topicsToEnrich, topicName)
			result.Stats.Discovery.IncludedItems++
		}
		c.protocol.Debugf("Enriching %d topics (excluded %d system topics)",
			len(topicsToEnrich), result.Stats.Discovery.ExcludedItems)
	} else {
		for _, topicName := range topics {
			if !collectSystem && strings.HasPrefix(topicName, "SYSTEM.") {
				result.Stats.Discovery.ExcludedItems++
				continue
			}

			matched, err := filepath.Match(selector, topicName)
			if err != nil {
				c.protocol.Warningf("Invalid selector pattern '%s': %v", selector, err)
				matched = false
			}

			if matched {
				topicsToEnrich = append(topicsToEnrich, topicName)
				result.Stats.Discovery.IncludedItems++
			} else {
				result.Stats.Discovery.ExcludedItems++
			}
		}
		c.protocol.Debugf("Selector '%s' matched %d topics, excluded %d (including system filtering)",
			selector, result.Stats.Discovery.IncludedItems, result.Stats.Discovery.ExcludedItems)
	}
	return topicsToEnrich
}

func (c *Client) enrichTopics(topicsToEnrich []string, collectMetrics bool, result *TopicCollectionResult) {
	for _, topicName := range topicsToEnrich {
		tm := TopicMetrics{TopicString: topicName, Name: topicName}

		if collectMetrics {
			c.enrichTopicWithMetrics(&tm, result)
		}

		result.Topics = append(result.Topics, tm)
	}
}

func (c *Client) enrichTopicWithMetrics(tm *TopicMetrics, result *TopicCollectionResult) {
	if result.Stats.Metrics == nil {
		result.Stats.Metrics = &EnrichmentStats{
			TotalItems:  int64(len(result.Topics)),
			ErrorCounts: make(map[int32]int),
		}
	}

	metricsData, err := c.GetTopicMetrics(tm.TopicString)
	if err != nil {
		result.Stats.Metrics.FailedItems++
		if pcfErr, ok := err.(*PCFError); ok {
			result.Stats.Metrics.ErrorCounts[pcfErr.Code]++
		} else {
			result.Stats.Metrics.ErrorCounts[-1]++
		}
		c.protocol.Debugf("Failed to get metrics for topic '%s': %v", tm.TopicString, err)
	} else {
		result.Stats.Metrics.OkItems++
		tm.Name = metricsData.Name
		tm.TopicString = metricsData.TopicString
		tm.Publishers = metricsData.Publishers
		tm.Subscribers = metricsData.Subscribers
		tm.PublishMsgCount = metricsData.PublishMsgCount
		tm.LastPubDate = metricsData.LastPubDate
		tm.LastPubTime = metricsData.LastPubTime
	}
}
