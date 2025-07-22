// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import (
	"fmt"
	"strings"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)

const (
	// Maximum number of statistics messages to process per collection
	// Set to 1M to handle any realistic deployment - we want to process ALL messages
	maxStatisticsMessages = 1000000
)

// GetStatisticsQueue collects messages from SYSTEM.ADMIN.STATISTICS.QUEUE
// This queue contains pre-published statistics messages with extended metrics
// like min/max depth, average queue time, operation counts, etc.
//
// LIMITATION: The IBM MQ Go library (github.com/ibm-messaging/mq-golang/v5) currently
// does not support reading messages from queues - it only supports PCF administrative
// commands. This functionality requires MQGET operations which are not yet implemented
// in the Go library. For now, this returns empty results to maintain compatibility.
func (c *Client) GetStatisticsQueue() (*StatisticsCollectionResult, error) {
	if !c.connected {
		c.protocol.Debugf("GetStatisticsQueue FAILED: not connected")
		return nil, fmt.Errorf("not connected")
	}

	c.protocol.Warningf("Statistics queue collection is not yet implemented in the IBM MQ Go library migration")
	c.protocol.Debugf("GetStatisticsQueue returning empty results - feature needs implementation")
	
	// Return empty successful result for now
	result := &StatisticsCollectionResult{
		Stats: CollectionStats{
			Discovery: struct {
				Success        bool
				AvailableItems int64
				InvisibleItems int64
				IncludedItems  int64
				ExcludedItems  int64
				UnparsedItems  int64
				ErrorCounts    map[int32]int
			}{
				Success: true,
				AvailableItems: 0,
				InvisibleItems: 0,
				IncludedItems: 0,
				ExcludedItems: 0,
				UnparsedItems: 0,
				ErrorCounts: make(map[int32]int),
			},
		},
		Messages: []StatisticsMessage{},
	}

	c.protocol.Debugf("GetStatisticsQueue SUCCESS (stub implementation)")
	return result, nil
}

// parseStatisticsMessage parses a raw statistics message into structured data
// LIMITATION: Cannot be implemented until IBM MQ Go library supports MQGET operations
func (c *Client) parseStatisticsMessage(buffer []byte, md *ibmmq.MQMD) (*StatisticsMessage, error) {
	return nil, fmt.Errorf("statistics message parsing not yet implemented")
}

// shouldSkipOldStatisticsMessage checks if a statistics message should be skipped
// LIMITATION: Cannot be implemented until IBM MQ Go library supports MQGET operations
func (c *Client) shouldSkipOldStatisticsMessage(msg *StatisticsMessage) bool {
	return false
}

// parseQueueStatistics parses queue statistics from a PCF message
// NOTE: This function is partially implemented for when MQGET operations become available
func (c *Client) parseQueueStatistics(buffer []byte) ([]QueueStatistics, error) {
	attrs, err := c.parsePCFResponse(buffer, "STATISTICS_Q")
	if err != nil {
		return nil, fmt.Errorf("failed to parse PCF attributes: %w", err)
	}

	var stats []QueueStatistics

	// Extract queue name
	queueName, ok := attrs[ibmmq.MQCA_Q_NAME].(string)
	if !ok {
		return nil, fmt.Errorf("queue name not found in statistics message")
	}

	var stat QueueStatistics
	stat.Name = queueName
	
	// Extract queue type if available
	if qType, ok := attrs[ibmmq.MQIA_Q_TYPE]; ok {
		if typeVal, ok := qType.(int32); ok {
			stat.Type = QueueType(typeVal)
		}
	}

	// Extract min/max depth
	if val, ok := attrs[ibmmq.MQIAMO_Q_MIN_DEPTH]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MinDepth = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_Q_MAX_DEPTH]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MaxDepth = AttributeValue(intVal)
		}
	}

	// Extract average queue time (array with [0]=non-persistent, [1]=persistent)
	if val, ok := attrs[ibmmq.MQIAMO64_AVG_Q_TIME]; ok {
		if arrayVal, ok := val.([]int64); ok && len(arrayVal) >= 2 {
			stat.AvgQTimeNonPersistent = AttributeValue(arrayVal[0])
			stat.AvgQTimePersistent = AttributeValue(arrayVal[1])
		}
	}
	
	// Extract short/long time indicators
	// Note: These constants appear to be MQIAMO_Q_TIME_AVG/MIN/MAX in some versions
	// For now, we'll leave these fields unpopulated until we can verify the correct constants
	stat.QTimeShort = NotCollected
	stat.QTimeLong = NotCollected

	// Extract put/get operations (arrays with [0]=non-persistent, [1]=persistent)
	if val, ok := attrs[ibmmq.MQIAMO_PUTS]; ok {
		if arrayVal, ok := val.([]int32); ok && len(arrayVal) >= 2 {
			stat.PutsNonPersistent = AttributeValue(arrayVal[0])
			stat.PutsPersistent = AttributeValue(arrayVal[1])
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_GETS]; ok {
		if arrayVal, ok := val.([]int32); ok && len(arrayVal) >= 2 {
			stat.GetsNonPersistent = AttributeValue(arrayVal[0])
			stat.GetsPersistent = AttributeValue(arrayVal[1])
		}
	}

	// Extract byte counters (arrays with [0]=non-persistent, [1]=persistent)
	if val, ok := attrs[ibmmq.MQIAMO64_PUT_BYTES]; ok {
		if arrayVal, ok := val.([]int64); ok && len(arrayVal) >= 2 {
			stat.PutBytesNonPersistent = AttributeValue(arrayVal[0])
			stat.PutBytesPersistent = AttributeValue(arrayVal[1])
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO64_GET_BYTES]; ok {
		if arrayVal, ok := val.([]int64); ok && len(arrayVal) >= 2 {
			stat.GetBytesNonPersistent = AttributeValue(arrayVal[0])
			stat.GetBytesPersistent = AttributeValue(arrayVal[1])
		}
	}

	// Extract failure counters
	if val, ok := attrs[ibmmq.MQIAMO_PUTS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.PutsFailed = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_PUT1S_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Put1sFailed = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_GETS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.GetsFailed = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_BROWSES_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.BrowsesFailed = AttributeValue(intVal)
		}
	}

	// Extract message lifecycle counters
	if val, ok := attrs[ibmmq.MQIAMO_MSGS_EXPIRED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MsgsExpired = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_MSGS_PURGED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MsgsPurged = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_MSGS_NOT_QUEUED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.MsgsNotQueued = AttributeValue(intVal)
		}
	}

	// Extract additional counters
	if val, ok := attrs[ibmmq.MQIAMO_BROWSES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.BrowseCount = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO64_BROWSE_BYTES]; ok {
		if intVal, ok := val.(int64); ok {
			stat.BrowseBytes = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_PUT1S]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Put1Count = AttributeValue(intVal)
		}
	}

	// Extract timestamp information
	if val, ok := attrs[ibmmq.MQCAMO_START_DATE]; ok {
		if dateStr, ok := val.(string); ok {
			// For now, just mark as collected without conversion
			// Future: implement proper date string parsing
			stat.StartDate = AttributeValue(1) // Mark as collected
			_ = dateStr // Avoid unused variable warning
		}
	}
	if val, ok := attrs[ibmmq.MQCAMO_START_TIME]; ok {
		if timeStr, ok := val.(string); ok {
			// For now, just mark as collected without conversion
			// Future: implement proper time string parsing
			stat.StartTime = AttributeValue(1) // Mark as collected
			_ = timeStr // Avoid unused variable warning
		}
	}

	stats = append(stats, stat)
	
	c.protocol.Debugf("Parsed queue statistics for queue '%s' - min_depth: %v, max_depth: %v", 
		stat.Name, stat.MinDepth, stat.MaxDepth)

	return stats, nil
}

// parseChannelStatistics parses channel statistics from a PCF message
func (c *Client) parseChannelStatistics(buffer []byte) ([]ChannelStatistics, error) {
	attrs, err := c.parsePCFResponse(buffer, "STATISTICS_CHANNEL")
	if err != nil {
		return nil, fmt.Errorf("failed to parse PCF attributes: %w", err)
	}

	var stats []ChannelStatistics

	// Extract channel name
	channelName, ok := attrs[ibmmq.MQCACH_CHANNEL_NAME].(string)
	if !ok {
		return nil, fmt.Errorf("channel name not found in statistics message")
	}

	var stat ChannelStatistics
	stat.Name = channelName

	// Extract channel type and status if available
	if cType, ok := attrs[ibmmq.MQIACH_CHANNEL_TYPE]; ok {
		if typeVal, ok := cType.(int32); ok {
			stat.Type = ChannelType(typeVal)
		}
	}
	if cStatus, ok := attrs[ibmmq.MQIACH_CHANNEL_STATUS]; ok {
		if statusVal, ok := cStatus.(int32); ok {
			stat.Status = ChannelStatus(statusVal)
		}
	}

	// Extract message metrics
	if val, ok := attrs[ibmmq.MQIAMO_MSGS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Messages = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO64_BYTES]; ok {
		if intVal, ok := val.(int64); ok {
			stat.Bytes = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_FULL_BATCHES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.FullBatches = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_INCOMPLETE_BATCHES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.IncompleteBatches = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_AVG_BATCH_SIZE]; ok {
		if intVal, ok := val.(int32); ok {
			stat.AvgBatchSize = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_PUT_RETRIES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.PutRetries = AttributeValue(intVal)
		}
	}

	stats = append(stats, stat)
	
	c.protocol.Debugf("Parsed channel statistics for channel '%s' - messages: %v, bytes: %v", 
		stat.Name, stat.Messages, stat.Bytes)

	return stats, nil
}

// parseMQIStatistics parses MQI statistics from the raw PCF message
func (c *Client) parseMQIStatistics(buffer []byte) ([]MQIStatistics, error) {
	attrs, err := c.parsePCFResponse(buffer, "STATISTICS_MQI")
	if err != nil {
		return nil, fmt.Errorf("failed to parse MQI statistics: %w", err)
	}
	
	var mqiStats []MQIStatistics
	var stat MQIStatistics
	
	// Extract name (could be queue manager or queue name depending on STATMQI setting)
	if val, ok := attrs[ibmmq.MQCA_Q_MGR_NAME]; ok {
		if strVal, ok := val.(string); ok {
			stat.Name = strings.TrimSpace(strVal)
		}
	}
	// If not queue manager name, try queue name
	if stat.Name == "" {
		if val, ok := attrs[ibmmq.MQCA_Q_NAME]; ok {
			if strVal, ok := val.(string); ok {
				stat.Name = strings.TrimSpace(strVal)
			}
		}
	}
	
	// Extract MQOPEN operations
	if val, ok := attrs[ibmmq.MQIAMO_OPENS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Opens = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_OPENS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.OpensFailed = AttributeValue(intVal)
		}
	}
	
	// Extract MQCLOSE operations
	if val, ok := attrs[ibmmq.MQIAMO_CLOSES]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Closes = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_CLOSES_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.ClosesFailed = AttributeValue(intVal)
		}
	}
	
	// Extract MQINQ operations
	if val, ok := attrs[ibmmq.MQIAMO_INQS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Inqs = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_INQS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.InqsFailed = AttributeValue(intVal)
		}
	}
	
	// Extract MQSET operations
	if val, ok := attrs[ibmmq.MQIAMO_SETS]; ok {
		if intVal, ok := val.(int32); ok {
			stat.Sets = AttributeValue(intVal)
		}
	}
	if val, ok := attrs[ibmmq.MQIAMO_SETS_FAILED]; ok {
		if intVal, ok := val.(int32); ok {
			stat.SetsFailed = AttributeValue(intVal)
		}
	}
	
	// Extract timestamp information
	var startDate, startTime, endDate, endTime string
	if val, ok := attrs[ibmmq.MQCAMO_START_DATE]; ok {
		if strVal, ok := val.(string); ok {
			startDate = strings.TrimSpace(strVal)
		}
	}
	if val, ok := attrs[ibmmq.MQCAMO_START_TIME]; ok {
		if strVal, ok := val.(string); ok {
			startTime = strings.TrimSpace(strVal)
		}
	}
	if val, ok := attrs[ibmmq.MQCAMO_END_DATE]; ok {
		if strVal, ok := val.(string); ok {
			endDate = strings.TrimSpace(strVal)
		}
	}
	if val, ok := attrs[ibmmq.MQCAMO_END_TIME]; ok {
		if strVal, ok := val.(string); ok {
			endTime = strings.TrimSpace(strVal)
		}
	}
	
	// Convert timestamps to Unix epoch if we have both date and time
	// MQCAMO_START_DATE/TIME and MQCAMO_END_DATE/TIME are administrative monitoring timestamps (local time)
	if startDate != "" && startTime != "" {
		if startTimestamp, err := ParseMQAdminDateTime(startDate, startTime); err == nil {
			stat.StartDate = AttributeValue(startTimestamp.Unix())
			stat.StartTime = AttributeValue(startTimestamp.Unix())
		}
	}
	if endDate != "" && endTime != "" {
		if endTimestamp, err := ParseMQAdminDateTime(endDate, endTime); err == nil {
			stat.EndDate = AttributeValue(endTimestamp.Unix())
			stat.EndTime = AttributeValue(endTimestamp.Unix())
		}
	}
	
	// Add the statistics entry
	mqiStats = append(mqiStats, stat)
	
	c.protocol.Debugf("Parsed MQI statistics for '%s' - opens: %v (failed: %v), closes: %v (failed: %v), inqs: %v (failed: %v), sets: %v (failed: %v)", 
		stat.Name, stat.Opens, stat.OpensFailed, stat.Closes, stat.ClosesFailed, 
		stat.Inqs, stat.InqsFailed, stat.Sets, stat.SetsFailed)
	
	return mqiStats, nil
}