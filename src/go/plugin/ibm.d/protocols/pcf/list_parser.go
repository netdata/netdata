// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"strings"
	"unsafe"
)

// ChannelListResult contains the results of parsing channel list response
type ChannelListResult struct {
	Channels       []string           // Successfully retrieved channel names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorChannels  map[int32][]string // MQ error code -> channel names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// parseChannelListResponse parses a multi-message response containing channel information
func (c *Client) parseChannelListResponse(response []byte) *ChannelListResult {
	result := &ChannelListResult{
		Channels:       make([]string, 0),
		ErrorCounts:    make(map[int32]int),
		ErrorChannels:  make(map[int32][]string),
	}

	// Parse response in chunks (each channel gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}

		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		
		// Calculate the full message size by walking through all parameters
		messageSize := int(C.sizeof_MQCFH)
		paramOffset := offset + int(C.sizeof_MQCFH)
		
		for i := 0; i < int(cfh.ParameterCount) && paramOffset < len(response); i++ {
			if paramOffset+8 > len(response) { // Need at least type + length
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
		
		// Parse this message
		attrs, err := c.ParsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			c.protocol.Warningf("PCF: Failed to parse response message at offset %d for queue manager '%s': %v", 
				offset, c.config.QueueManager, err)
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}
		
		// Get error code for this specific array item
		compCode := int32(0)
		mqError := int32(0)
		if cc, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if val, ok := cc.(int32); ok {
				compCode = val
			}
		}
		_ = compCode // Keep variable for parsing structure documentation
		if rc, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if val, ok := rc.(int32); ok {
				mqError = val
			}
		}
		
		// Count errors
		if mqError != 0 {
			result.ErrorCounts[mqError]++
		}
		
		// Extract channel name
		channelName := ""
		if name, ok := attrs[C.MQCACH_CHANNEL_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				channelName = strings.TrimSpace(nameStr)
			}
		}
		
		// Store result based on error status
		if mqError != 0 {
			// Channel had an error
			if channelName != "" {
				result.ErrorChannels[mqError] = append(result.ErrorChannels[mqError], channelName)
			}
		} else if channelName != "" {
			// Successful channel
			result.Channels = append(result.Channels, channelName)
		}
		
		offset = messageEnd
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

// QueueListWithTypeResult contains the results of parsing queue list response with type information
type QueueListWithTypeResult struct {
	Queues         []QueueInfo           // Successfully retrieved queues with type info
	ErrorCounts    map[int32]int         // MQ error code -> count
	ErrorQueues    map[int32][]string    // MQ error code -> queue names that failed
	InternalErrors int                   // Count of parsing/internal errors
}

// parseQueueListResponse parses a multi-message response containing queue information
func (c *Client) parseQueueListResponse(response []byte) *QueueListResult {
	result := &QueueListResult{
		Queues:      make([]string, 0),
		ErrorCounts: make(map[int32]int),
		ErrorQueues: make(map[int32][]string),
	}

	// Parse response in chunks (each queue gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}

		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		
		// Calculate the full message size
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
		
		// Parse this message
		attrs, err := c.ParsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			c.protocol.Warningf("PCF: Failed to parse response message at offset %d for queue manager '%s': %v", 
				offset, c.config.QueueManager, err)
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}
		
		// Get error code
		compCode := int32(0)
		mqError := int32(0)
		if cc, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if val, ok := cc.(int32); ok {
				compCode = val
			}
		}
		_ = compCode // Keep variable for parsing structure documentation
		if rc, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if val, ok := rc.(int32); ok {
				mqError = val
			}
		}
		
		// Count errors
		if mqError != 0 {
			result.ErrorCounts[mqError]++
		}
		
		// Extract queue name
		queueName := ""
		if name, ok := attrs[C.MQCA_Q_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				queueName = strings.TrimSpace(nameStr)
			}
		}
		
		// Store result based on error status
		if mqError != 0 {
			// Queue had an error
			if queueName != "" {
				result.ErrorQueues[mqError] = append(result.ErrorQueues[mqError], queueName)
			}
		} else if queueName != "" {
			// Successful queue
			result.Queues = append(result.Queues, queueName)
		}
		
		offset = messageEnd
	}
	
	return result
}

// parseQueueListResponseWithType parses a multi-message response containing queue information with type
func (c *Client) parseQueueListResponseWithType(response []byte) *QueueListWithTypeResult {
	result := &QueueListWithTypeResult{
		Queues:      make([]QueueInfo, 0),
		ErrorCounts: make(map[int32]int),
		ErrorQueues: make(map[int32][]string),
	}

	// Parse response in chunks (each queue gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}

		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		
		// Calculate the full message size
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
		
		// Parse this message
		attrs, err := c.ParsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			c.protocol.Warningf("PCF: Failed to parse response message at offset %d for queue manager '%s': %v", 
				offset, c.config.QueueManager, err)
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}
		
		// Get error code
		compCode := int32(0)
		mqError := int32(0)
		if cc, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if val, ok := cc.(int32); ok {
				compCode = val
			}
		}
		_ = compCode // Keep variable for parsing structure documentation
		if rc, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if val, ok := rc.(int32); ok {
				mqError = val
			}
		}
		
		// Count errors
		if mqError != 0 {
			result.ErrorCounts[mqError]++
		}
		
		// Extract queue name and type
		queueName := ""
		queueType := int32(0)
		
		if name, ok := attrs[C.MQCA_Q_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				queueName = strings.TrimSpace(nameStr)
			}
		}
		
		if qtype, ok := attrs[C.MQIA_Q_TYPE]; ok {
			if qtypeInt, ok := qtype.(int32); ok {
				queueType = qtypeInt
			}
		}
		
		// Extract configuration attributes (available in same response)
		// Initialize all to NotCollected, only set when actually present
		inhibitGet := NotCollected
		inhibitPut := NotCollected
		backoutThreshold := NotCollected
		triggerDepth := NotCollected
		triggerType := NotCollected
		maxMsgLength := NotCollected
		defPriority := NotCollected
		
		if attr, ok := attrs[C.MQIA_INHIBIT_GET]; ok {
			if val, ok := attr.(int32); ok {
				inhibitGet = AttributeValue(val)
			}
		}
		
		if attr, ok := attrs[C.MQIA_INHIBIT_PUT]; ok {
			if val, ok := attr.(int32); ok {
				inhibitPut = AttributeValue(val)
			}
		}
		
		if attr, ok := attrs[C.MQIA_BACKOUT_THRESHOLD]; ok {
			if val, ok := attr.(int32); ok {
				backoutThreshold = AttributeValue(val)
			}
		}
		
		if attr, ok := attrs[C.MQIA_TRIGGER_DEPTH]; ok {
			if val, ok := attr.(int32); ok {
				triggerDepth = AttributeValue(val)
			}
		}
		
		if attr, ok := attrs[C.MQIA_TRIGGER_TYPE]; ok {
			if val, ok := attr.(int32); ok {
				triggerType = AttributeValue(val)
			}
		}
		
		if attr, ok := attrs[C.MQIA_MAX_MSG_LENGTH]; ok {
			if val, ok := attr.(int32); ok {
				maxMsgLength = AttributeValue(val)
			}
		}
		
		if attr, ok := attrs[C.MQIA_DEF_PRIORITY]; ok {
			if val, ok := attr.(int32); ok {
				defPriority = AttributeValue(val)
			}
		}
		
		// Store result based on error status
		if mqError != 0 {
			// Queue had an error
			if queueName != "" {
				result.ErrorQueues[mqError] = append(result.ErrorQueues[mqError], queueName)
			}
		} else if queueName != "" {
			// Successful queue
			queueInfo := QueueInfo{
				Name:             queueName,
				Type:             queueType,
				InhibitGet:       inhibitGet,
				InhibitPut:       inhibitPut,
				BackoutThreshold: backoutThreshold,
				TriggerDepth:     triggerDepth,
				TriggerType:      triggerType,
				MaxMsgLength:     maxMsgLength,
				DefPriority:      defPriority,
			}
			result.Queues = append(result.Queues, queueInfo)
		}
		
		offset = messageEnd
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
	result := &TopicListResult{
		Topics:      make([]string, 0),
		ErrorCounts: make(map[int32]int),
		ErrorTopics: make(map[int32][]string),
	}

	// Parse response in chunks (each topic gets its own response message)
	offset := 0
	for offset < len(response) {
		if offset+int(C.sizeof_MQCFH) > len(response) {
			result.InternalErrors++
			result.ErrorCounts[ErrInternalShort]++
			break
		}

		cfh := (*C.MQCFH)(unsafe.Pointer(&response[offset]))
		
		// Calculate the full message size
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
		
		// Parse this message
		attrs, err := c.ParsePCFResponse(response[offset:messageEnd], "")
		if err != nil {
			c.protocol.Warningf("PCF: Failed to parse response message at offset %d for queue manager '%s': %v", 
				offset, c.config.QueueManager, err)
			result.InternalErrors++
			result.ErrorCounts[ErrInternalParsing]++
			offset = messageEnd
			continue
		}
		
		// Get error code
		compCode := int32(0)
		mqError := int32(0)
		if cc, ok := attrs[C.MQIACF_COMP_CODE]; ok {
			if val, ok := cc.(int32); ok {
				compCode = val
			}
		}
		_ = compCode // Keep variable for parsing structure documentation
		if rc, ok := attrs[C.MQIACF_REASON_CODE]; ok {
			if val, ok := rc.(int32); ok {
				mqError = val
			}
		}
		
		// Count errors
		if mqError != 0 {
			result.ErrorCounts[mqError]++
		}
		
		// Extract topic name
		topicName := ""
		if name, ok := attrs[C.MQCA_TOPIC_NAME]; ok {
			if nameStr, ok := name.(string); ok && nameStr != "" {
				topicName = strings.TrimSpace(nameStr)
			}
		}
		
		// Store result based on error status
		if mqError != 0 {
			// Topic had an error
			if topicName != "" {
				result.ErrorTopics[mqError] = append(result.ErrorTopics[mqError], topicName)
			}
		} else if topicName != "" {
			// Successful topic
			result.Topics = append(result.Topics, topicName)
		}
		
		offset = messageEnd
	}
	
	return result
}