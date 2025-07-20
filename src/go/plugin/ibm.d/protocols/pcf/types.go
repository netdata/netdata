// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

import "math"

// AttributeValue represents a configuration attribute value
type AttributeValue int64

// Special value indicating an attribute was not collected/available
const NotCollected AttributeValue = math.MinInt64

// IsCollected returns true if the attribute was successfully collected
func (a AttributeValue) IsCollected() bool {
	return a != NotCollected
}

// Int64 returns the int64 value, should only be called when IsCollected() is true
func (a AttributeValue) Int64() int64 {
	return int64(a)
}

// ChannelType represents the type of MQ channel
type ChannelType int32

const (
	ChannelTypeSender      ChannelType = 1
	ChannelTypeServer      ChannelType = 2
	ChannelTypeReceiver    ChannelType = 3
	ChannelTypeRequester   ChannelType = 4
	ChannelTypeClntconn    ChannelType = 6
	ChannelTypeSvrconn     ChannelType = 7
	ChannelTypeClussdr     ChannelType = 8
	ChannelTypeClusrcvr    ChannelType = 9
	ChannelTypeMqtt        ChannelType = 10
	ChannelTypeAMQP        ChannelType = 11
)

// ChannelStatus represents the current status of a channel
type ChannelStatus int32

const (
	ChannelStatusInactive     ChannelStatus = 0
	ChannelStatusBinding      ChannelStatus = 1
	ChannelStatusStarting     ChannelStatus = 2
	ChannelStatusRunning      ChannelStatus = 3
	ChannelStatusStopping     ChannelStatus = 4
	ChannelStatusRetrying     ChannelStatus = 5
	ChannelStatusStopped      ChannelStatus = 6
	ChannelStatusRequesting   ChannelStatus = 7
	ChannelStatusPaused       ChannelStatus = 8
	ChannelStatusDisconnected ChannelStatus = 9
	ChannelStatusInitializing ChannelStatus = 13
	ChannelStatusSwitching    ChannelStatus = 14
)

// ChannelMetrics contains runtime metrics for a channel
type ChannelMetrics struct {
	Name   string
	Type   ChannelType
	Status ChannelStatus
	
	// Message metrics (only for message channels)
	Messages *int64 // Total messages sent
	Bytes    *int64 // Total bytes sent
	Batches  *int64 // Total batches sent
	
	// Current connections (only for SVRCONN)
	Connections *int64
	
	// Buffer metrics (only for sender/receiver channels)
	BuffersUsed *int64
	BuffersMax  *int64
}

// ChannelConfig contains configuration for a channel
type ChannelConfig struct {
	Name string
	Type ChannelType
	
	// Batch configuration (not available for all channel types)
	BatchSize     AttributeValue
	BatchInterval AttributeValue
	
	// Intervals (availability varies by channel type)
	DiscInterval      AttributeValue
	HbInterval        AttributeValue
	KeepAliveInterval AttributeValue
	
	// Retry configuration (sender/receiver specific)
	ShortRetry AttributeValue
	LongRetry  AttributeValue
	
	// Limits
	MaxMsgLength         AttributeValue
	SharingConversations AttributeValue
	NetworkPriority      AttributeValue
}

// QueueType represents the type of MQ queue
type QueueType int32

// QueueCollectionConfig configures what queue data to collect
type QueueCollectionConfig struct {
	QueuePattern      string // Glob pattern for queue selection (e.g., "*", "SYSTEM.*", "APP.QUEUE.*")
	CollectResetStats bool   // Whether to collect destructive reset statistics
}

// QueueMetrics contains runtime metrics for a queue
type QueueMetrics struct {
	Name string
	Type QueueType
	
	// Basic metrics (always available from discovery)
	CurrentDepth int64
	MaxDepth     int64
	
	// Status metrics (from MQCMD_INQUIRE_Q_STATUS)
	OpenInputCount  AttributeValue
	OpenOutputCount AttributeValue
	OldestMsgAge    AttributeValue
	UncommittedMsgs AttributeValue
	LastGetDate     AttributeValue  // YYYYMMDD format (e.g., 20240120)
	LastGetTime     AttributeValue  // HHMMSSSS format (e.g., 14302500 = 14:30:25.00)
	LastPutDate     AttributeValue  // YYYYMMDD format (e.g., 20240120)
	LastPutTime     AttributeValue  // HHMMSSSS format (e.g., 14302500 = 14:30:25.00)
	HasStatusMetrics bool  // Indicates if status collection succeeded
	
	// Reset statistics (from MQCMD_RESET_Q_STATS)
	EnqueueCount    int64
	DequeueCount    int64
	HighDepth       int64
	TimeSinceReset  int64
	HasResetStats   bool   // Indicates if reset stats were collected
	
	// Configuration metrics (from discovery MQCMD_INQUIRE_Q)
	// Note: NotCollected means "attribute not collected/available"
	InhibitGet       AttributeValue  // Range: 0-1 (boolean), NotCollected = not available
	InhibitPut       AttributeValue  // Range: 0-1 (boolean), NotCollected = not available
	BackoutThreshold AttributeValue  // Range: 0-999999999, NotCollected = not available
	TriggerDepth     AttributeValue  // Range: 1-999999999, NotCollected = not available
	TriggerType      AttributeValue  // Range: 0-3 (NONE/FIRST/EVERY/DEPTH), NotCollected = not available
	MaxMsgLength     AttributeValue  // Range: 0-max_int32, NotCollected = not available
	DefPriority      AttributeValue  // Range: 0-9, NotCollected = not available
}

// QueueConfig contains configuration for a queue
type QueueConfig struct {
	Name string
	Type QueueType
	
	// Inhibit settings
	InhibitGet int64
	InhibitPut int64
	
	// Thresholds
	BackoutThreshold int64
	TriggerDepth     int64
	TriggerType      int64
	
	// Limits
	MaxMsgLength int64
	DefPriority  int64
}

// TopicMetrics contains runtime metrics for a topic
type TopicMetrics struct {
	Name        string
	TopicString string
	
	// Publisher/Subscriber counts
	Publishers  int64
	Subscribers int64
	
	// Message count
	PublishMsgCount int64
}

// QueueManagerMetrics contains runtime metrics for a queue manager
type QueueManagerMetrics struct {
	// Status - 1 if running, 0 if not responding
	Status int64
}

// ListenerStatus represents the status of a listener
type ListenerStatus int32

const (
	ListenerStatusStopped ListenerStatus = 0
	ListenerStatusRunning ListenerStatus = 1
)

// ListenerMetrics contains runtime metrics for a listener
type ListenerMetrics struct {
	Name   string
	Status ListenerStatus
	Port   int64
}