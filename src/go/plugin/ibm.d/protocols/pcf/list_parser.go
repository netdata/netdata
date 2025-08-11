// SPDX-License-Identifier: GPL-3.0-or-later


package pcf


import (
	"strings"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
)


// ChannelListResult contains the results of parsing channel list response
type ChannelListResult struct {
	Channels       []string           // Successfully retrieved channel names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorChannels  map[int32][]string // MQ error code -> channel names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// QueueListResult contains the results of parsing queue list response
type QueueListResult struct {
	Queues         []string           // Successfully retrieved queue names
	ErrorCounts    map[int32]int      // MQ error code -> count
	ErrorQueues    map[int32][]string // MQ error code -> queue names that failed
	InternalErrors int                // Count of parsing/internal errors
}

// parseQueueListResponseFromParams parses PCF parameters into QueueListResult
func (c *Client) parseQueueListResponseFromParams(params []*ibmmq.PCFParameter) *QueueListResult {
	result := &QueueListResult{
		Queues:         []string{},
		ErrorCounts:    make(map[int32]int),
		ErrorQueues:    make(map[int32][]string),
		InternalErrors: 0,
	}

	// Convert each parameter set (assuming it's a multi-object response)
	for _, param := range params {
		if param.Type == ibmmq.MQCFT_STRING && param.Parameter == ibmmq.MQCA_Q_NAME {
			if len(param.String) > 0 {
				queueName := strings.TrimSpace(param.String[0])
				if queueName != "" {
					result.Queues = append(result.Queues, queueName)
				}
			}
		}
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

// parseQueueListResponseWithTypeFromParams parses PCF parameters into QueueListWithTypeResult
func (c *Client) parseQueueListResponseWithTypeFromParams(params []*ibmmq.PCFParameter) *QueueListWithTypeResult {
	result := &QueueListWithTypeResult{
		Queues:         []QueueInfo{},
		ErrorCounts:    make(map[int32]int),
		ErrorQueues:    make(map[int32][]string),
		InternalErrors: 0,
	}

	// Convert IBM PCFParameter array to attrs format for existing smart function
	attrs := convertPCFParametersToAttrs(params)

	// Process queue attributes using existing logic
	for _, param := range params {
		if param.Type == ibmmq.MQCFT_STRING && param.Parameter == ibmmq.MQCA_Q_NAME {
			if len(param.String) > 0 {
				queueName := strings.TrimSpace(param.String[0])
				if queueName != "" {
					// Create QueueInfo from attributes
					queueInfo := QueueInfo{
						Name: queueName,
					}
					
					// Extract type and configuration attributes
					if qtype, ok := attrs[ibmmq.MQIA_Q_TYPE]; ok {
						if qtypeInt, ok := qtype.(int32); ok {
							queueInfo.Type = qtypeInt
						}
					}
					
					// Set configuration attributes with NotCollected as default
					queueInfo.InhibitGet = NotCollected
					queueInfo.InhibitPut = NotCollected
					queueInfo.BackoutThreshold = NotCollected
					queueInfo.TriggerDepth = NotCollected
					queueInfo.TriggerType = NotCollected
					queueInfo.MaxMsgLength = NotCollected
					queueInfo.DefPriority = NotCollected
					
					// Override with actual values if available
					if val, ok := attrs[ibmmq.MQIA_INHIBIT_GET].(int32); ok {
						queueInfo.InhibitGet = AttributeValue(val)
					}
					if val, ok := attrs[ibmmq.MQIA_INHIBIT_PUT].(int32); ok {
						queueInfo.InhibitPut = AttributeValue(val)
					}
					if val, ok := attrs[ibmmq.MQIA_BACKOUT_THRESHOLD].(int32); ok {
						queueInfo.BackoutThreshold = AttributeValue(val)
					}
					if val, ok := attrs[ibmmq.MQIA_TRIGGER_DEPTH].(int32); ok {
						queueInfo.TriggerDepth = AttributeValue(val)
					}
					if val, ok := attrs[ibmmq.MQIA_TRIGGER_TYPE].(int32); ok {
						queueInfo.TriggerType = AttributeValue(val)
					}
					if val, ok := attrs[ibmmq.MQIA_MAX_MSG_LENGTH].(int32); ok {
						queueInfo.MaxMsgLength = AttributeValue(val)
					}
					if val, ok := attrs[ibmmq.MQIA_DEF_PRIORITY].(int32); ok {
						queueInfo.DefPriority = AttributeValue(val)
					}
					
					result.Queues = append(result.Queues, queueInfo)
				}
			}
		}
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


// parseTopicListResponseFromParams parses PCF parameters into TopicListResult
func (c *Client) parseTopicListResponseFromParams(params []*ibmmq.PCFParameter) *TopicListResult {
	result := &TopicListResult{
		Topics:         []string{},
		ErrorCounts:    make(map[int32]int),
		ErrorTopics:    make(map[int32][]string),
		InternalErrors: 0,
	}

	// Process each parameter to extract topic names
	for _, param := range params {
		if param.Type == ibmmq.MQCFT_STRING && param.Parameter == ibmmq.MQCA_TOPIC_NAME {
			if len(param.String) > 0 {
				topicName := strings.TrimSpace(param.String[0])
				if topicName != "" {
					result.Topics = append(result.Topics, topicName)
				}
			}
		}
	}

	return result
}
