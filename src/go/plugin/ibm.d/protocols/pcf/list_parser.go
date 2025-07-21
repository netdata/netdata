// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"strings"
	"unsafe"
)

// GenericListResult contains the results of parsing a generic list response.
type GenericListResult struct {
	Items          []interface{}      // Successfully retrieved items
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorItems     map[int32][]string // MQ error code -> item names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// itemProcessor is a function that processes a single item from a PCF response.
type itemProcessor func(attrs map[C.MQLONG]interface{}) (interface{}, error)

// parseGenericListResponse is a generic function to parse a multi-message PCF response.
func (c *Client) parseGenericListResponse(response []byte, nameAttr C.MQLONG, processor itemProcessor) *GenericListResult {
	result := &GenericListResult{
		Items:       make([]interface{}, 0),
		ErrorCounts: make(map[int32]int),
		ErrorItems:  make(map[int32][]string),
	}

	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}

		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))

		messageSize := int(C.sizeof_MQCFH)
		paramOffset := offset + int(C.sizeof_MQCFH)

		for i := 0; i < int(cfh.ParameterCount) && paramOffset < len(response); i++ {
			if paramOffset+8 > len(response) {
				break
			}
			paramLength := *(*C.MQLONG)(unsafe.Pointer(&response[paramOffset+4]))
			if paramLength <= 0 || paramOffset+int(paramLength) > len(response) {
				break
			}
			messageSize += int(paramLength)
			paramOffset += int(paramLength)
		}

		messageEnd := offset + messageSize

		if messageEnd > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalCorrupt]++
			break
		}

		attrs, err := c.ParsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			c.protocol.Warningf("Failed to parse response message at offset %d for queue manager '%s': %v",
				offset, c.config.QueueManager, err)
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}

		compCode := int32(0)
		mqError := int32(0)
		if cc, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if val, ok := cc.(int32); ok {
				compCode = val
			}
		}
		_ = compCode
		if rc, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if val, ok := rc.(int32); ok {
				mqError = val
			}
		}

		if mqError != 0 {
			result.ErrorCounts[mqError]++
		}

		itemName := ""
		if name, ok := attrs[nameAttr]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				itemName = strings.TrimSpace(nameStr)
			}
		}

		if mqError != 0 {
			if itemName != "" {
				result.ErrorItems[mqError] = append(result.ErrorItems[mqError], itemName)
			}
		} else if itemName != "" {
			item, err := processor(attrs)
			if err != nil {
				result.InternalErrors++
				result.ErrorCounts[ErrInternalParsing]++
			} else {
				result.Items = append(result.Items, item)
			}
		}

		offset = messageEnd
	}

	return result
}

// ChannelListResult contains the results of parsing channel list response
type ChannelListResult struct {
	Channels       []string           // Successfully retrieved channel names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorChannels  map[int32][]string // MQ error code -> channel names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// parseChannelListResponse parses a multi-message response containing channel information
func (c *Client) parseChannelListResponse(response []byte) *ChannelListResult {
	processor := func(attrs map[C.MQLONG]interface{}) (interface{}, error) {
		if name, ok := attrs[C.MQCACH_CHANNEL_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				return strings.TrimSpace(nameStr), nil
			}
		}
		return nil, fmt.Errorf("channel name not found")
	}

	genericResult := c.parseGenericListResponse(response, C.MQCACH_CHANNEL_NAME, processor)

	result := &ChannelListResult{
		ErrorCounts:    genericResult.ErrorCounts,
		ErrorChannels:  genericResult.ErrorItems,
		InternalErrors: genericResult.InternalErrors,
	}
	for _, item := range genericResult.Items {
		result.Channels = append(result.Channels, item.(string))
	}
	return result
}

// QueueListResult contains the results of parsing queue list response
type QueueListResult struct {
	Queues         []string           // Successfully retrieved queue names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorQueues    map[int32][]string // MQ error code -> queue names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// parseQueueListResponse parses a multi-message response containing queue information
func (c *Client) parseQueueListResponse(response []byte) *QueueListResult {
	processor := func(attrs map[C.MQLONG]interface{}) (interface{}, error) {
		if name, ok := attrs[C.MQCA_Q_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				return strings.TrimSpace(nameStr), nil
			}
		}
		return nil, fmt.Errorf("queue name not found")
	}

	genericResult := c.parseGenericListResponse(response, C.MQCA_Q_NAME, processor)

	result := &QueueListResult{
		ErrorCounts:    genericResult.ErrorCounts,
		ErrorQueues:    genericResult.ErrorItems,
		InternalErrors: genericResult.InternalErrors,
	}
	for _, item := range genericResult.Items {
		result.Queues = append(result.Queues, item.(string))
	}
	return result
}

// QueueListWithTypeResult contains the results of parsing queue list response with type information
type QueueListWithTypeResult struct {
	Queues         []QueueInfo           // Successfully retrieved queues with type info
	ErrorCounts    map[int32]int         // MQ error code -> count
	ErrorQueues    map[int32][]string    // MQ error code -> queue names that failed
	InternalErrors int                   // Count of parsing/internal errors
}

// parseQueueListResponseWithType parses a multi-message response containing queue information with type
func (c *Client) parseQueueListResponseWithType(response []byte) *QueueListWithTypeResult {
	processor := func(attrs map[C.MQLONG]interface{}) (interface{}, error) {
		queueInfo := QueueInfo{}
		if name, ok := attrs[C.MQCA_Q_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				queueInfo.Name = strings.TrimSpace(nameStr)
			}
		}
		if queueInfo.Name == "" {
			return nil, fmt.Errorf("queue name not found")
		}

		if qtype, ok := attrs[C.MQIA_Q_TYPE]; ok {
			if qtypeInt, ok := qtype.(int32); ok {
				queueInfo.Type = qtypeInt
			}
		}

		// Extract configuration attributes
		inhibitGet := NotCollected
		if attr, ok := attrs[C.MQIA_INHIBIT_GET]; ok {
			if val, ok := attr.(int32); ok {
				inhibitGet = AttributeValue(val)
			}
		}
		queueInfo.InhibitGet = inhibitGet

		inhibitPut := NotCollected
		if attr, ok := attrs[C.MQIA_INHIBIT_PUT]; ok {
			if val, ok := attr.(int32); ok {
				inhibitPut = AttributeValue(val)
			}
		}
		queueInfo.InhibitPut = inhibitPut

		backoutThreshold := NotCollected
		if attr, ok := attrs[C.MQIA_BACKOUT_THRESHOLD]; ok {
			if val, ok := attr.(int32); ok {
				backoutThreshold = AttributeValue(val)
			}
		}
		queueInfo.BackoutThreshold = backoutThreshold

		triggerDepth := NotCollected
		if attr, ok := attrs[C.MQIA_TRIGGER_DEPTH]; ok {
			if val, ok := attr.(int32); ok {
				triggerDepth = AttributeValue(val)
			}
		}
		queueInfo.TriggerDepth = triggerDepth

		triggerType := NotCollected
		if attr, ok := attrs[C.MQIA_TRIGGER_TYPE]; ok {
			if val, ok := attr.(int32); ok {
				triggerType = AttributeValue(val)
			}
		}
		queueInfo.TriggerType = triggerType

		maxMsgLength := NotCollected
		if attr, ok := attrs[C.MQIA_MAX_MSG_LENGTH]; ok {
			if val, ok := attr.(int32); ok {
				maxMsgLength = AttributeValue(val)
			}
		}
		queueInfo.MaxMsgLength = maxMsgLength

		defPriority := NotCollected
		if attr, ok := attrs[C.MQIA_DEF_PRIORITY]; ok {
			if val, ok := attr.(int32); ok {
				defPriority = AttributeValue(val)
			}
		}
		queueInfo.DefPriority = defPriority

		return queueInfo, nil
	}

	genericResult := c.parseGenericListResponse(response, C.MQCA_Q_NAME, processor)

	result := &QueueListWithTypeResult{
		ErrorCounts:    genericResult.ErrorCounts,
		ErrorQueues:    genericResult.ErrorItems,
		InternalErrors: genericResult.InternalErrors,
	}
	for _, item := range genericResult.Items {
		result.Queues = append(result.Queues, item.(QueueInfo))
	}
	return result
}

// TopicListResult contains the results of parsing topic list response
type TopicListResult struct {
	Topics         []string           // Successfully retrieved topic names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorTopics    map[int32][]string // MQ error code -> topic names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// parseTopicListResponse parses a multi-message response containing topic information
func (c *Client) parseTopicListResponse(response []byte) *TopicListResult {
	processor := func(attrs map[C.MQLONG]interface{}) (interface{}, error) {
		if name, ok := attrs[C.MQCA_TOPIC_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				return strings.TrimSpace(nameStr), nil
			}
		}
		return nil, fmt.Errorf("topic name not found")
	}

	genericResult := c.parseGenericListResponse(response, C.MQCA_TOPIC_NAME, processor)

	result := &TopicListResult{
		ErrorCounts:    genericResult.ErrorCounts,
		ErrorTopics:    genericResult.ErrorItems,
		InternalErrors: genericResult.InternalErrors,
	}
	for _, item := range genericResult.Items {
		result.Topics = append(result.Topics, item.(string))
	}
	return result
}
