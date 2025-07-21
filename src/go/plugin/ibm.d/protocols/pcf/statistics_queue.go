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

const (
	// Maximum number of statistics messages to process per collection
	// Set to 1M to handle any realistic deployment - we want to process ALL messages
	maxStatisticsMessages = 1000000
)

// GetStatisticsQueue collects messages from SYSTEM.ADMIN.STATISTICS.QUEUE
// This queue contains pre-published statistics messages with extended metrics
// like min/max depth, average queue time, operation counts, etc.
func (c *Client) GetStatisticsQueue() (*StatisticsCollectionResult, error) {
	if !c.connected {
		c.protocol.Warningf("PCF: Cannot get statistics queue messages from queue manager '%s' - not connected", 
			c.config.QueueManager)
		return nil, fmt.Errorf("not connected")
	}

	c.protocol.Debugf("PCF: Opening SYSTEM.ADMIN.STATISTICS.QUEUE on queue manager '%s'", c.config.QueueManager)

	// Open the statistics queue for input
	var od C.MQOD
	C.memset(unsafe.Pointer(&od), 0, C.sizeof_MQOD)
	C.set_od_struc_id(&od)
	od.Version = C.MQOD_VERSION_1
	statisticsQueueName := C.CString("SYSTEM.ADMIN.STATISTICS.QUEUE")
	defer C.free(unsafe.Pointer(statisticsQueueName))
	C.set_object_name(&od, statisticsQueueName)
	od.ObjectType = C.MQOT_Q

	var hStatisticsObj C.MQHOBJ
	var compCode C.MQLONG
	var reason C.MQLONG
	var openOptions C.MQLONG = C.MQOO_INPUT_AS_Q_DEF | C.MQOO_FAIL_IF_QUIESCING

	C.MQOPEN(c.hConn, C.PMQVOID(unsafe.Pointer(&od)), openOptions, &hStatisticsObj, &compCode, &reason)

	if compCode != C.MQCC_OK {
		c.protocol.Errorf("PCF: Failed to open SYSTEM.ADMIN.STATISTICS.QUEUE on queue manager '%s' - completion code %d, reason %d (%s)",
			c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		return nil, fmt.Errorf("MQOPEN statistics queue failed: completion code %d, reason %d (%s)", 
			compCode, reason, mqReasonString(int32(reason)))
	}

	defer func() {
		// Close the statistics queue
		C.MQCLOSE(c.hConn, &hStatisticsObj, C.MQCO_NONE, &compCode, &reason)
		if compCode != C.MQCC_OK {
			c.protocol.Warningf("PCF: Failed to close SYSTEM.ADMIN.STATISTICS.QUEUE on queue manager '%s' - completion code %d, reason %d (%s)",
				c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
		}
	}()

	c.protocol.Debugf("PCF: Successfully opened SYSTEM.ADMIN.STATISTICS.QUEUE on queue manager '%s', handle: %d", 
		c.config.QueueManager, hStatisticsObj)

	var result StatisticsCollectionResult
	result.Stats.Discovery.Success = true
	messageCount := 0

	// Process available statistics messages
	for messageCount < maxStatisticsMessages {
		messageCount++
		
		// Set up message descriptor
		var md C.MQMD
		C.memset(unsafe.Pointer(&md), 0, C.sizeof_MQMD)
		C.set_md_struc_id(&md)
		md.Version = C.MQMD_VERSION_1

		// Set up get message options
		var gmo C.MQGMO
		C.memset(unsafe.Pointer(&gmo), 0, C.sizeof_MQGMO)
		C.set_gmo_struc_id(&gmo)
		gmo.Version = C.MQGMO_VERSION_1
		gmo.Options = C.MQGMO_NO_WAIT | C.MQGMO_FAIL_IF_QUIESCING | C.MQGMO_CONVERT
		// No wait interval - return immediately if no messages available

		// Get message length first
		var bufferLength C.MQLONG = 0
		c.protocol.Debugf("PCF: Attempting to get statistics message #%d from queue manager '%s' (no wait)", 
			messageCount, c.config.QueueManager)
		
		C.MQGET(c.hConn, hStatisticsObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), 
			bufferLength, nil, &bufferLength, &compCode, &reason)

		if reason == C.MQRC_NO_MSG_AVAILABLE {
			// No more messages available
			c.protocol.Debugf("PCF: No more statistics messages available from queue manager '%s' (processed %d messages)", 
				c.config.QueueManager, messageCount-1)
			break
		}

		if reason != C.MQRC_TRUNCATED_MSG_FAILED {
			if messageCount == 1 {
				// First message failed - this is a real error
				c.protocol.Errorf("PCF: Failed to get statistics message length from queue manager '%s' - completion code %d, reason %d (%s)",
					c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
				return nil, fmt.Errorf("MQGET statistics length failed: completion code %d, reason %d (%s)", 
					compCode, reason, mqReasonString(int32(reason)))
			} else {
				// Subsequent message failed - log and continue
				c.protocol.Warningf("PCF: Failed to get statistics message #%d from queue manager '%s' - completion code %d, reason %d (%s)",
					messageCount, c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
				break
			}
		}

		if bufferLength > maxMQGetBufferSize {
			c.protocol.Errorf("PCF: Statistics message from queue manager '%s' too large (%d bytes), maximum allowed is %d bytes", 
				c.config.QueueManager, bufferLength, maxMQGetBufferSize)
			return nil, fmt.Errorf("statistics message too large (%d bytes), maximum allowed is %d bytes", 
				bufferLength, maxMQGetBufferSize)
		}

		c.protocol.Debugf("PCF: Statistics message size is %d bytes, allocating buffer", bufferLength)

		// Allocate buffer and get actual message
		buffer := make([]byte, bufferLength)
		C.MQGET(c.hConn, hStatisticsObj, C.PMQVOID(unsafe.Pointer(&md)), C.PMQVOID(unsafe.Pointer(&gmo)), 
			bufferLength, C.PMQVOID(unsafe.Pointer(&buffer[0])), &bufferLength, &compCode, &reason)

		if compCode != C.MQCC_OK {
			if messageCount == 1 {
				// First message failed - this is a real error
				c.protocol.Errorf("PCF: Failed to get statistics message data from queue manager '%s' - completion code %d, reason %d (%s)",
					c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
				return nil, fmt.Errorf("MQGET statistics data failed: completion code %d, reason %d (%s)", 
					compCode, reason, mqReasonString(int32(reason)))
			} else {
				// Subsequent message failed - log and continue
				c.protocol.Warningf("PCF: Failed to get statistics message #%d data from queue manager '%s' - completion code %d, reason %d (%s)",
					messageCount, c.config.QueueManager, compCode, reason, mqReasonString(int32(reason)))
				break
			}
		}

		c.protocol.Debugf("PCF: Successfully received statistics message #%d (%d bytes) from queue manager '%s'", 
			messageCount, bufferLength, c.config.QueueManager)

		// Parse the statistics message
		statisticsMsg, err := c.parseStatisticsMessage(buffer[:bufferLength], &md)
		if err != nil {
			c.protocol.Warningf("PCF: Failed to parse statistics message #%d from queue manager '%s': %v", 
				messageCount, c.config.QueueManager, err)
			result.Stats.Discovery.UnparsedItems++
			continue
		}

		// Filter out old statistics messages (only process messages created after job started)
		if c.shouldSkipOldStatisticsMessage(statisticsMsg) {
			c.protocol.Debugf("PCF: Skipping old statistics message #%d (put time: %s %s, job started: %s)", 
				messageCount, statisticsMsg.PutDate, statisticsMsg.PutTime, c.jobCreationTime.Format("2006-01-02 15:04:05"))
			continue
		}

		result.Messages = append(result.Messages, *statisticsMsg)
		result.Stats.Discovery.AvailableItems++
		
		// Continue processing subsequent messages (no wait needed)
	}

	if messageCount >= maxStatisticsMessages {
		c.protocol.Errorf("PCF: CRITICAL - Reached statistics messages limit (%d) for queue manager '%s' - this indicates an extremely large deployment", 
			maxStatisticsMessages, c.config.QueueManager)
	}

	c.protocol.Debugf("PCF: Statistics collection completed for queue manager '%s' - processed %d messages", 
		c.config.QueueManager, len(result.Messages))

	return &result, nil
}

// parseStatisticsMessage parses a raw statistics message into structured data
func (c *Client) parseStatisticsMessage(buffer []byte, md *C.MQMD) (*StatisticsMessage, error) {
	if len(buffer) < int(C.sizeof_MQCFH) {
		return nil, fmt.Errorf("statistics message too short (%d bytes), minimum required is %d bytes", 
			len(buffer), C.sizeof_MQCFH)
	}

	// Parse PCF header
	cfh := (*C.MQCFH)(unsafe.Pointer(&buffer[0]))
	
	if cfh.Type != C.MQCFT_STATISTICS {
		return nil, fmt.Errorf("unexpected message type %d, expected MQCFT_STATISTICS (%d)", 
			cfh.Type, C.MQCFT_STATISTICS)
	}

	c.protocol.Debugf("PCF: Parsing statistics message - command: %s, parameters: %d, length: %d", 
		mqcmdToString(cfh.Command), cfh.ParameterCount, cfh.StrucLength)

	var msg StatisticsMessage
	msg.Command = int32(cfh.Command)
	msg.MessageLength = int32(cfh.StrucLength)
	
	// Extract message timestamp from MQMD
	mdDateBytes := make([]byte, 8)
	mdTimeBytes := make([]byte, 8)
	C.memcpy(unsafe.Pointer(&mdDateBytes[0]), unsafe.Pointer(&md.PutDate), 8)
	C.memcpy(unsafe.Pointer(&mdTimeBytes[0]), unsafe.Pointer(&md.PutTime), 8)
	msg.PutDate = string(mdDateBytes[:])
	msg.PutTime = string(mdTimeBytes[:])

	// Determine statistics type based on command
	switch cfh.Command {
	case C.MQCMD_STATISTICS_Q:
		msg.Type = StatisticsType(cfh.Command)
		queueStats, err := c.parseQueueStatistics(buffer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse queue statistics: %w", err)
		}
		msg.QueueStats = queueStats
		
	case C.MQCMD_STATISTICS_CHANNEL:
		msg.Type = StatisticsType(cfh.Command)
		channelStats, err := c.parseChannelStatistics(buffer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse channel statistics: %w", err)
		}
		msg.ChannelStats = channelStats
		
	case C.MQCMD_STATISTICS_MQI:
		msg.Type = StatisticsType(cfh.Command)
		mqiStats, err := c.parseMQIStatistics(buffer)
		if err != nil {
			return nil, fmt.Errorf("failed to parse MQI statistics: %w", err)
		}
		msg.MQIStats = mqiStats
		
	default:
		return nil, fmt.Errorf("unsupported statistics command: %s (%d)", 
			mqcmdToString(cfh.Command), cfh.Command)
	}

	return &msg, nil
}

// shouldSkipOldStatisticsMessage checks if a statistics message should be skipped
// because it was created before the job started (old accumulated message)
func (c *Client) shouldSkipOldStatisticsMessage(msg *StatisticsMessage) bool {
	// Parse the message timestamp
	msgTime, err := ParseMQDateTime(strings.TrimSpace(msg.PutDate), strings.TrimSpace(msg.PutTime))
	if err != nil {
		// If we can't parse the timestamp, assume it's old and skip it
		c.protocol.Warningf("PCF: Failed to parse statistics message timestamp (%s %s): %v - skipping message", 
			msg.PutDate, msg.PutTime, err)
		return true
	}
	
	// Skip messages that were created before the job started
	if msgTime.Before(c.jobCreationTime) {
		return true
	}
	
	return false
}

// parseQueueStatistics parses queue statistics from a PCF message
func (c *Client) parseQueueStatistics(buffer []byte) ([]QueueStatistics, error) {
	attrs, err := c.ParsePCFResponse(buffer, "STATISTICS_Q")
	if err != nil {
		return nil, fmt.Errorf("failed to parse PCF attributes: %w", err)
	}

	var stats []QueueStatistics

	// Extract queue name
	queueName, ok := attrs[C.MQCA_Q_NAME].(string)
	if !ok {
		return nil, fmt.Errorf("queue name not found in statistics message")
	}

	var stat QueueStatistics
	stat.Name = queueName
	
	// Extract queue type if available
	if qType, ok := attrs[C.MQIA_Q_TYPE]; ok {
		if typeVal, ok := qType.(int32); ok {
			stat.Type = QueueType(typeVal)
		}
	}

	// Extract min/max depth
	if val, ok := attrs[C.MQIAMO_Q_MIN_DEPTH]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MinDepth = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_Q_MAX_DEPTH]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MaxDepth = AttributeValue(intVal)
		}
	}

	// Extract average queue time (array with [0]=non-persistent, [1]=persistent)
	if val, ok := attrs[C.MQIAMO64_AVG_Q_TIME]; ok {
		if arrayVal, ok := val.([]int64); ok && len(arrayVal) >= 2 {
			stat.AvgQTimeNonPersistent = AttributeValue(arrayVal[0])
			stat.AvgQTimePersistent = AttributeValue(arrayVal[1])
		}
	}

	// Extract put/get operations (arrays with [0]=non-persistent, [1]=persistent)
	if val, ok := attrs[C.MQIAMO_PUTS]; ok {
		if arrayVal, ok := val.([]int32); ok && len(arrayVal) >= 2 {
			stat.PutsNonPersistent = AttributeValue(arrayVal[0])
			stat.PutsPersistent = AttributeValue(arrayVal[1])
		}
	}
	if val, ok := attrs[C.MQIAMO_GETS]; ok {
		if arrayVal, ok := val.([]int32); ok && len(arrayVal) >= 2 {
			stat.GetsNonPersistent = AttributeValue(arrayVal[0])
			stat.GetsPersistent = AttributeValue(arrayVal[1])
		}
	}

	// Extract byte counters (arrays with [0]=non-persistent, [1]=persistent)
	if val, ok := attrs[C.MQIAMO64_PUT_BYTES]; ok {
		if arrayVal, ok := val.([]int64); ok && len(arrayVal) >= 2 {
			stat.PutBytesNonPersistent = AttributeValue(arrayVal[0])
			stat.PutBytesPersistent = AttributeValue(arrayVal[1])
		}
	}
	if val, ok := attrs[C.MQIAMO64_GET_BYTES]; ok {
		if arrayVal, ok := val.([]int64); ok && len(arrayVal) >= 2 {
			stat.GetBytesNonPersistent = AttributeValue(arrayVal[0])
			stat.GetBytesPersistent = AttributeValue(arrayVal[1])
		}
	}

	// Extract failure counters
	if val, ok := attrs[C.MQIAMO_PUTS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.PutsFailed = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_PUT1S_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Put1sFailed = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_GETS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.GetsFailed = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_BROWSES_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.BrowsesFailed = AttributeValue(intVal)
		}
	}

	// Extract message lifecycle counters
	if val, ok := attrs[C.MQIAMO_MSGS_EXPIRED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MsgsExpired = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_MSGS_PURGED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MsgsPurged = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_MSGS_NOT_QUEUED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MsgsNotQueued = AttributeValue(intVal)
		}
	}

	// Extract additional counters
	if val, ok := attrs[C.MQIAMO_BROWSES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.BrowseCount = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO64_BROWSE_BYTES]; ok {
		if intVal, ok := val.(int64); ok {
			stat.BrowseBytes = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_PUT1S]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Put1Count = AttributeValue(intVal)
		}
	}

	// Extract timestamp information
	if val, ok := attrs[C.MQCAMO_START_DATE]; ok {
		if _, ok := val.(string); ok {
			// Convert string to int representation for consistency
			stat.StartDate = AttributeValue(0) // TODO: Convert date string to int
		}
	}
	if val, ok := attrs[C.MQCAMO_START_TIME]; ok {
		if _, ok := val.(string); ok {
			// Convert string to int representation for consistency
			stat.StartTime = AttributeValue(0) // TODO: Convert time string to int
		}
	}

	stats = append(stats, stat)
	
	c.protocol.Debugf("PCF: Parsed queue statistics for queue '%s' - min_depth: %v, max_depth: %v", 
		stat.Name, stat.MinDepth, stat.MaxDepth)

	return stats, nil
}

// parseChannelStatistics parses channel statistics from a PCF message
func (c *Client) parseChannelStatistics(buffer []byte) ([]ChannelStatistics, error) {
	attrs, err := c.ParsePCFResponse(buffer, "STATISTICS_CHANNEL")
	if err != nil {
		return nil, fmt.Errorf("failed to parse PCF attributes: %w", err)
	}

	var stats []ChannelStatistics

	// Extract channel name
	channelName, ok := attrs[C.MQCACH_CHANNEL_NAME].(string)
	if !ok {
		return nil, fmt.Errorf("channel name not found in statistics message")
	}

	var stat ChannelStatistics
	stat.Name = channelName

	// Extract channel type and status if available
	if cType, ok := attrs[C.MQIACH_CHANNEL_TYPE]; ok {
		if typeVal, ok := cType.(int32); ok {
			stat.Type = ChannelType(typeVal)
		}
	}
	if cStatus, ok := attrs[C.MQIACH_CHANNEL_STATUS]; ok {
		if statusVal, ok := cStatus.(int32); ok {
			stat.Status = ChannelStatus(statusVal)
		}
	}

	// Extract message metrics
	if val, ok := attrs[C.MQIAMO_MSGS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Messages = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO64_BYTES]; ok {
		if intVal, ok := val.(int64); ok {
			stat.Bytes = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_FULL_BATCHES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.FullBatches = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_INCOMPLETE_BATCHES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.IncompleteBatches = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_AVG_BATCH_SIZE]; ok {
		if intVal, ok := val.(int32); ok {
			stat.AvgBatchSize = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_PUT_RETRIES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.PutRetries = AttributeValue(intVal)
		}
	}

	stats = append(stats, stat)
	
	c.protocol.Debugf("PCF: Parsed channel statistics for channel '%s' - messages: %v, bytes: %v", 
		stat.Name, stat.Messages, stat.Bytes)

	return stats, nil
}

// parseMQIStatistics parses MQI statistics from the raw PCF message
func (c *Client) parseMQIStatistics(buffer []byte) ([]MQIStatistics, error) {
	attrs, err := c.ParsePCFResponse(buffer, "STATISTICS_MQI")
	if err != nil {
		return nil, fmt.Errorf("failed to parse MQI statistics: %w", err)
	}
	
	var mqiStats []MQIStatistics
	var stat MQIStatistics
	
	// Extract name (could be queue manager or queue name depending on STATMQI setting)
	if val, ok := attrs[C.MQCA_Q_MGR_NAME]; ok {
		if strVal, ok := val.(string); ok {
			stat.Name = strings.TrimSpace(strVal)
		}
	}
	// If not queue manager name, try queue name
	if stat.Name == "" {
		if val, ok := attrs[C.MQCA_Q_NAME]; ok {
			if strVal, ok := val.(string); ok {
				stat.Name = strings.TrimSpace(strVal)
			}
		}
	}
	
	// Extract MQOPEN operations
	if val, ok := attrs[C.MQIAMO_OPENS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Opens = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_OPENS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.OpensFailed = AttributeValue(intVal)
		}
	}
	
	// Extract MQCLOSE operations
	if val, ok := attrs[C.MQIAMO_CLOSES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Closes = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_CLOSES_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.ClosesFailed = AttributeValue(intVal)
		}
	}
	
	// Extract MQINQ operations
	if val, ok := attrs[C.MQIAMO_INQS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Inqs = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_INQS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.InqsFailed = AttributeValue(intVal)
		}
	}
	
	// Extract MQSET operations
	if val, ok := attrs[C.MQIAMO_SETS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Sets = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[C.MQIAMO_SETS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.SetsFailed = AttributeValue(intVal)
		}
	}
	
	// Extract timestamp information
	var startDate, startTime, endDate, endTime string
	if val, ok := attrs[C.MQCAMO_START_DATE]; ok {
		if strVal, ok := val.(string); ok {
			startDate = strings.TrimSpace(strVal)
		}
	}
	if val, ok := attrs[C.MQCAMO_START_TIME]; ok {
		if strVal, ok := val.(string); ok {
			startTime = strings.TrimSpace(strVal)
		}
	}
	if val, ok := attrs[C.MQCAMO_END_DATE]; ok {
		if strVal, ok := val.(string); ok {
			endDate = strings.TrimSpace(strVal)
		}
	}
	if val, ok := attrs[C.MQCAMO_END_TIME]; ok {
		if strVal, ok := val.(string); ok {
			endTime = strings.TrimSpace(strVal)
		}
	}
	
	// Convert timestamps to Unix epoch if we have both date and time
	if startDate != "" && startTime != "" {
		if startTimestamp, err := ParseMQDateTime(startDate, startTime); err == nil {
			stat.StartDate = AttributeValue(startTimestamp.Unix())
			stat.StartTime = AttributeValue(startTimestamp.Unix())
		}
	}
	if endDate != "" && endTime != "" {
		if endTimestamp, err := ParseMQDateTime(endDate, endTime); err == nil {
			stat.EndDate = AttributeValue(endTimestamp.Unix())
			stat.EndTime = AttributeValue(endTimestamp.Unix())
		}
	}
	
	// Add the statistics entry
	mqiStats = append(mqiStats, stat)
	
	c.protocol.Debugf("PCF: Parsed MQI statistics for '%s' - opens: %v (failed: %v), closes: %v (failed: %v), inqs: %v (failed: %v), sets: %v (failed: %v)", 
		stat.Name, stat.Opens, stat.OpensFailed, stat.Closes, stat.ClosesFailed, 
		stat.Inqs, stat.InqsFailed, stat.Sets, stat.SetsFailed)
	
	return mqiStats, nil
}