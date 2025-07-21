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

// SysTopicMessage represents a message from $SYS topics
type SysTopicMessage struct {
	Topic     string
	Timestamp int64
	Data      map[string]interface{}
}

// SysTopicCollection holds the results of $SYS topic collection
type SysTopicCollection struct {
	Messages []SysTopicMessage
	Stats    CollectionStats
}

// GetSysTopicMessages collects messages from $SYS topics for resource monitoring
// This uses MQGET with NO_WAIT similar to statistics queue collection
func (c *Client) GetSysTopicMessages(topics []string) (*SysTopicCollection, error) {
	if !c.connected {
		c.protocol.Warningf("Cannot get $SYS topic messages from queue manager '%s' - not connected", 
			c.config.QueueManager)
		return nil, fmt.Errorf("not connected")
	}

	var result SysTopicCollection
	result.Stats.Discovery.Success = true

	// For each topic, try to get messages
	for _, topic := range topics {
		if err := c.collectSysTopicMessages(topic, &result); err != nil {
			c.protocol.Warningf("Failed to collect messages from topic %s: %v", topic, err)
			result.Stats.Discovery.UnparsedItems++
			// Continue with other topics
		}
	}

	c.protocol.Debugf("$SYS topic collection completed for queue manager '%s' - collected %d messages from %d topics", 
		c.config.QueueManager, len(result.Messages), len(topics))

	return &result, nil
}

// collectSysTopicMessages collects messages from a specific $SYS topic
func (c *Client) collectSysTopicMessages(topicString string, result *SysTopicCollection) error {
	c.protocol.Debugf("Opening subscription for $SYS topic '%s' on queue manager '%s'", 
		topicString, c.config.QueueManager)

	// First, we need to create a managed queue for the subscription
	var od C.MQOD
	C.memset(unsafe.Pointer(&od), 0, C.sizeof_MQOD)
	C.set_od_struc_id(&od)
	od.Version = C.MQOD_VERSION_4
	od.ObjectType = C.MQOT_Q
	// Use model queue for dynamic queue creation
	modelQueueName := C.CString("SYSTEM.NDYNAMIC.MODEL.QUEUE")
	defer C.free(unsafe.Pointer(modelQueueName))
	C.set_object_name(&od, modelQueueName)
	// Dynamic queue name pattern
	dynamicQName := C.CString("NETDATA.SYS.*")
	defer C.free(unsafe.Pointer(dynamicQName))
	C.set_dynamic_q_name(&od, dynamicQName)

	var hQueue C.MQHOBJ
	var compCode C.MQLONG
	var reason C.MQLONG
	var openOptions C.MQLONG = C.MQOO_INPUT_EXCLUSIVE | C.MQOO_FAIL_IF_QUIESCING

	// Open dynamic queue
	C.MQOPEN(c.hConn, C.PMQVOID(unsafe.Pointer(&od)), openOptions, &hQueue, &compCode, &reason)
	
	if compCode != C.MQCC_OK {
		c.protocol.Errorf("Failed to open dynamic queue for $SYS topic on queue manager '%s' - completion code %d, reason %d (%s)",
			c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		return fmt.Errorf("MQOPEN dynamic queue failed: completion code %d, reason %d (%s)", 
			compCode, reason, mqReasonString(int32(reason)))
	}

	defer func() {
		// Close and delete the dynamic queue
		C.MQCLOSE(c.hConn, &hQueue, C.MQCO_DELETE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.protocol.Warningf("Failed to close/delete dynamic queue on queue manager '%s' - completion code %d, reason %d (%s)",
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		}
	}()

	// Now create a subscription to the $SYS topic
	var sd C.MQSD
	C.memset(unsafe.Pointer(&sd), 0, C.sizeof_MQSD)
	C.set_sd_struc_id(&sd)
	sd.Version = C.MQSD_VERSION_1
	sd.Options = C.MQSO_CREATE | C.MQSO_NON_DURABLE | C.MQSO_FAIL_IF_QUIESCING | C.MQSO_MANAGED

	// Set the topic string
	topicCStr := C.CString(topicString)
	defer C.free(unsafe.Pointer(topicCStr))
	sd.ObjectString.VSPtr = C.MQPTR(topicCStr)
	sd.ObjectString.VSLength = C.MQLONG(len(topicString))

	// Set the queue handle for managed subscription
	// Note: ObjectHandle may be named differently in different MQ versions
	// sd.ObjectHandle = hQueue  // Not available in our headers

	var hSub C.MQHOBJ
	C.MQSUB(c.hConn, C.PMQVOID(unsafe.Pointer(&sd)), &hQueue, &hSub, &compCode, &reason)

	if compCode != C.MQCC_OK {
		c.protocol.Errorf("Failed to create subscription for $SYS topic '%s' on queue manager '%s' - completion code %d, reason %d (%s)",
			topicString, c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		return fmt.Errorf("MQSUB failed: completion code %d, reason %d (%s)", 
			compCode, reason, mqReasonString(int32(reason)))
	}

	defer func() {
		// Close the subscription
		var closeOptions C.MQLONG = C.MQCO_REMOVE_SUB
		C.MQCLOSE(c.hConn, &hSub, closeOptions, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.protocol.Warningf("Failed to close subscription on queue manager '%s' - completion code %d, reason %d (%s)",
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		}
	}()

	// Now get messages from the queue (with NO_WAIT like statistics queue)
	messageCount := 0
	maxMessages := 100 // Limit messages per topic to avoid excessive processing

	for messageCount < maxMessages {
		messageCount++
		
		// Set up message descriptor
		var md C.MQMD
		C.memset(unsafe.Pointer(&md), 0, C.sizeof_MQMD)
		C.set_md_struc_id(&md)
		md.Version = C.MQMD_VERSION_1

		// Set up get message options with NO_WAIT
		var gmo C.MQGMO
		C.memset(unsafe.Pointer(&gmo), 0, C.sizeof_MQGMO)
		C.set_gmo_struc_id(&gmo)
		gmo.Version = C.MQGMO_VERSION_1
		gmo.Options = C.MQGMO_NO_WAIT | C.MQGMO_FAIL_IF_QUIESCING | C.MQGMO_CONVERT

		// Get message length first
		var bufferLength C.MQLONG = 0
		C.MQGET(c.hConn, hQueue, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), 
			bufferLength, nil, &bufferLength, &compCode, &reason)

		if reason == C.MQRC_NO_MSG_AVAILABLE {
			// No more messages available
			c.protocol.Debugf("No more messages available for $SYS topic '%s' (processed %d messages)", 
				topicString, messageCount-1)
			break
		}

		if reason != C.MQRC_TRUNCATED_MSG_FAILED {
			if messageCount == 1 {
				// First message failed - this is expected if no messages published yet
				c.protocol.Debugf("No messages available for $SYS topic '%s' - reason %d (%s)",
					topicString, reason, mqReasonString(int32(reason)))
				return nil // Not an error, just no data yet
			} else {
				// Subsequent message failed
				c.protocol.Warningf("Failed to get message #%d from $SYS topic '%s' - reason %d (%s)",
					messageCount, topicString, reason, mqReasonString(int32(reason)))
				break
			}
		}

		// Allocate buffer and get actual message
		buffer := make([]byte, bufferLength)
		C.MQGET(c.hConn, hQueue, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), 
			bufferLength, C.PMQVOID(unsafe.Pointer(&buffer[0])), &bufferLength, &compCode, &reason)

		if compCode != C.MQCC_OK {
			c.protocol.Warningf("Failed to get message data from $SYS topic '%s' - reason %d (%s)",
				topicString, reason, mqReasonString(int32(reason)))
			continue
		}

		// Parse the message
		msg, err := c.parseSysTopicMessage(topicString, buffer[:bufferLength], &md)
		if err != nil {
			c.protocol.Warningf("Failed to parse message from $SYS topic '%s': %v", topicString, err)
			result.Stats.Discovery.UnparsedItems++
			continue
		}

		result.Messages = append(result.Messages, *msg)
		result.Stats.Discovery.AvailableItems++
	}

	return nil
}

// parseSysTopicMessage parses a message from a $SYS topic
func (c *Client) parseSysTopicMessage(topic string, buffer []byte, md *C.MQMD) (*SysTopicMessage, error) {
	msg := &SysTopicMessage{
		Topic: topic,
		Data:  make(map[string]interface{}),
	}

	// Get timestamp from message descriptor
	mdDateBytes := make([]byte, 8)
	mdTimeBytes := make([]byte, 8)
	C.memcpy(unsafe.Pointer(&mdDateBytes[0]), unsafe.Pointer(&md.PutDate), 8)
	C.memcpy(unsafe.Pointer(&mdTimeBytes[0]), unsafe.Pointer(&md.PutTime), 8)
	
	putDate := strings.TrimSpace(string(mdDateBytes[:]))
	putTime := strings.TrimSpace(string(mdTimeBytes[:]))
	
	if timestamp, err := ParseMQDateTime(putDate, putTime); err == nil {
		msg.Timestamp = timestamp.Unix()
	}

	// Parse based on topic type
	switch {
	case strings.Contains(topic, "ResourceStatistics/QueueManager"):
		return c.parseQueueManagerResourceStats(msg, buffer)
	case strings.Contains(topic, "Log/"):
		return c.parseLogUtilization(msg, buffer)
	default:
		// Generic PCF parsing for other $SYS topics
		attrs, err := c.ParsePCFResponse(buffer, "SYS_TOPIC")
		if err != nil {
			return nil, fmt.Errorf("failed to parse PCF response: %w", err)
		}
		// Convert map[C.MQLONG]interface{} to map[string]interface{}
		for key, value := range attrs {
			msg.Data[fmt.Sprintf("attr_%d", key)] = value
		}
	}

	return msg, nil
}

// parseQueueManagerResourceStats parses Queue Manager resource statistics
func (c *Client) parseQueueManagerResourceStats(msg *SysTopicMessage, buffer []byte) (*SysTopicMessage, error) {
	attrs, err := c.ParsePCFResponse(buffer, "QM_RESOURCE_STATS")
	if err != nil {
		return nil, fmt.Errorf("failed to parse resource stats: %w", err)
	}

	// Store all attributes generically since we don't know exact constants
	// The actual fields depend on MQ version and platform
	// Convert map[C.MQLONG]interface{} to map[string]interface{}
	for key, value := range attrs {
		msg.Data[fmt.Sprintf("attr_%d", key)] = value
	}
	
	// Log what we found for debugging
	c.protocol.Debugf("$SYS topic resource stats contains %d attributes", len(attrs))
	for key, value := range attrs {
		c.protocol.Debugf("  Attribute %v = %v (type: %T)", key, value, value)
	}

	return msg, nil
}

// parseLogUtilization parses log utilization messages
func (c *Client) parseLogUtilization(msg *SysTopicMessage, buffer []byte) (*SysTopicMessage, error) {
	attrs, err := c.ParsePCFResponse(buffer, "LOG_UTILIZATION")
	if err != nil {
		return nil, fmt.Errorf("failed to parse log utilization: %w", err)
	}

	// Store all attributes generically
	// Convert map[C.MQLONG]interface{} to map[string]interface{}
	for key, value := range attrs {
		msg.Data[fmt.Sprintf("attr_%d", key)] = value
	}
	
	// Log what we found for debugging
	c.protocol.Debugf("$SYS topic log utilization contains %d attributes", len(attrs))
	for key, value := range attrs {
		c.protocol.Debugf("  Attribute %v = %v (type: %T)", key, value, value)
	}

	return msg, nil
}

// GetSysTopicSubscriptions returns the list of $SYS topics to subscribe to
func GetDefaultSysTopics() []string {
	return []string{
		"$SYS/MQ/INFO/QMGR/QM1/ResourceStatistics/QueueManager",
		"$SYS/MQ/INFO/QMGR/QM1/ResourceStatistics/Log",
		"$SYS/MQ/INFO/QMGR/QM1/ResourceStatistics/Storage",
	}
}