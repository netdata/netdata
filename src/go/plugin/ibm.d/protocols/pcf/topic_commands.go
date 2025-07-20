// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

import "strings"

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

	return metrics, nil
}