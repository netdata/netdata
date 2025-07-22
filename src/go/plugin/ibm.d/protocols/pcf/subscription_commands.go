// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"
import (
	"fmt"
	"strings"
	"time"
)

// InquireSubscription queries subscription information from the queue manager
func (c *Client) InquireSubscription(subName string) ([]SubscriptionMetrics, error) {
	c.protocol.Debugf("Inquiring subscriptions, filter: '%s'", subName)
	
	// Build parameters
	params := []pcfParameter{}
	
	// Add subscription name filter if provided
	if subName != "" {
		params = append(params, newStringParameter(C.MQCACF_SUB_NAME, subName))
	} else {
		// Use wildcard to get all subscriptions
		params = append(params, newStringParameter(C.MQCACF_SUB_NAME, "*"))
	}
	
	// Send PCF command
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_SUBSCRIPTION, params)
	if err != nil {
		c.protocol.Errorf("Failed to inquire subscriptions: %v", err)
		return nil, err
	}
	
	// Parse the response
	result := c.parseSubscriptionListResponse(response)
	
	if result.InternalErrors > 0 {
		c.protocol.Warningf("Encountered %d internal errors while parsing subscription list",
			result.InternalErrors)
	}
	
	c.protocol.Debugf("Found %d subscriptions", len(result.Subscriptions))
	return result.Subscriptions, nil
}

// InquireSubscriptionStatus queries subscription status information
func (c *Client) InquireSubscriptionStatus(subName string) (*SubscriptionMetrics, error) {
	c.protocol.Debugf("Inquiring subscription status for: '%s'", subName)
	
	// Build parameters - subscription name is required
	params := []pcfParameter{
		newStringParameter(C.MQCACF_SUB_NAME, subName),
	}
	
	// Send PCF command
	response, err := c.SendPCFCommand(C.MQCMD_INQUIRE_SUB_STATUS, params)
	if err != nil {
		c.protocol.Errorf("Failed to inquire subscription status for '%s': %v", subName, err)
		return nil, err
	}
	
	// Parse the response
	result := c.parseSubscriptionStatusResponse(response)
	
	if result.InternalErrors > 0 {
		c.protocol.Warningf("Encountered %d internal errors while parsing subscription status",
			result.InternalErrors)
	}
	
	if len(result.Subscriptions) == 0 {
		return nil, fmt.Errorf("no status found for subscription: %s", subName)
	}
	
	// Return the first (and should be only) subscription
	sub := result.Subscriptions[0]
	sub.Name = subName // Ensure name is set
	
	return &sub, nil
}

// GetSubscriptionAge calculates the age of the last message in seconds
func (s *SubscriptionMetrics) GetSubscriptionAge() (int64, error) {
	if s.LastMessageDate == "" || s.LastMessageTime == "" {
		return 0, fmt.Errorf("no last message timestamp available")
	}
	
	// IBM MQ date format: YYYY-MM-DD
	// IBM MQ time format: HH.MM.SS
	dateTimeStr := s.LastMessageDate + " " + strings.Replace(s.LastMessageTime, ".", ":", -1)
	
	lastMsgTime, err := time.Parse("2006-01-02 15:04:05", dateTimeStr)
	if err != nil {
		return 0, fmt.Errorf("failed to parse timestamp: %w", err)
	}
	
	age := time.Since(lastMsgTime).Seconds()
	return int64(age), nil
}

// SubscriptionParseResult contains subscription parsing results
type SubscriptionParseResult struct {
	Subscriptions  []SubscriptionMetrics
	InternalErrors int
	ErrorCounts    map[int32]int
}

// parseSubscriptionListResponse parses a subscription list response
func (c *Client) parseSubscriptionListResponse(response []byte) *SubscriptionParseResult {
	processor := func(attrs map[C.MQLONG]interface{}) (interface{}, error) {
		sub := SubscriptionMetrics{
			Type:         NotCollected,
			MessageCount: NotCollected,
		}
		
		// Get subscription name
		if name, ok := attrs[C.MQCACF_SUB_NAME]; ok {
			if nameStr, ok := name.(string); ok {
				sub.Name = strings.TrimSpace(nameStr)
			}
		}
		
		// Get topic string
		if topic, ok := attrs[C.MQCA_TOPIC_STRING]; ok {
			if topicStr, ok := topic.(string); ok {
				sub.TopicString = strings.TrimSpace(topicStr)
			}
		}
		
		// Get subscription type
		if subType, ok := attrs[C.MQIACF_SUB_TYPE]; ok {
			if typeVal, ok := subType.(int32); ok {
				sub.Type = AttributeValue(typeVal)
			}
		}
		
		if sub.Name == "" {
			return nil, fmt.Errorf("subscription name not found")
		}
		
		return sub, nil
	}
	
	genericResult := c.parseGenericListResponse(response, C.MQCACF_SUB_NAME, processor)
	
	result := &SubscriptionParseResult{
		ErrorCounts:    genericResult.ErrorCounts,
		InternalErrors: genericResult.InternalErrors,
		Subscriptions:  make([]SubscriptionMetrics, 0, len(genericResult.Items)),
	}
	
	for _, item := range genericResult.Items {
		if sub, ok := item.(SubscriptionMetrics); ok {
			result.Subscriptions = append(result.Subscriptions, sub)
		}
	}
	
	return result
}

// parseSubscriptionStatusResponse parses a subscription status response
func (c *Client) parseSubscriptionStatusResponse(response []byte) *SubscriptionParseResult {
	processor := func(attrs map[C.MQLONG]interface{}) (interface{}, error) {
		sub := SubscriptionMetrics{
			Type:         NotCollected,
			MessageCount: NotCollected,
		}
		
		// Get subscription name
		if name, ok := attrs[C.MQCACF_SUB_NAME]; ok {
			if nameStr, ok := name.(string); ok {
				sub.Name = strings.TrimSpace(nameStr)
			}
		}
		
		// Get topic string
		if topic, ok := attrs[C.MQCA_TOPIC_STRING]; ok {
			if topicStr, ok := topic.(string); ok {
				sub.TopicString = strings.TrimSpace(topicStr)
			}
		}
		
		// Get message count
		if count, ok := attrs[C.MQIACF_MESSAGE_COUNT]; ok {
			if countVal, ok := count.(int32); ok {
				sub.MessageCount = AttributeValue(countVal)
			}
		}
		
		// Get last message date/time
		if dateStr, ok := attrs[C.MQCACF_LAST_MSG_DATE]; ok {
			if dateVal, ok := dateStr.(string); ok {
				sub.LastMessageDate = strings.TrimSpace(dateVal)
			}
		}
		if timeStr, ok := attrs[C.MQCACF_LAST_MSG_TIME]; ok {
			if timeVal, ok := timeStr.(string); ok {
				sub.LastMessageTime = strings.TrimSpace(timeVal)
			}
		}
		
		if sub.Name == "" {
			return nil, fmt.Errorf("subscription name not found")
		}
		
		return sub, nil
	}
	
	genericResult := c.parseGenericListResponse(response, C.MQCACF_SUB_NAME, processor)
	
	result := &SubscriptionParseResult{
		ErrorCounts:    genericResult.ErrorCounts,
		InternalErrors: genericResult.InternalErrors,
		Subscriptions:  make([]SubscriptionMetrics, 0, len(genericResult.Items)),
	}
	
	for _, item := range genericResult.Items {
		if sub, ok := item.(SubscriptionMetrics); ok {
			result.Subscriptions = append(result.Subscriptions, sub)
		}
	}
	
	return result
}