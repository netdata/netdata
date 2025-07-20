// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

import (
	"fmt"
	"math"
)

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
	
	// Configuration metrics (populated when collectConfig is true)
	BatchSize            AttributeValue
	BatchInterval        AttributeValue
	DiscInterval         AttributeValue
	HbInterval           AttributeValue
	KeepAliveInterval    AttributeValue
	ShortRetry           AttributeValue
	LongRetry            AttributeValue
	MaxMsgLength         AttributeValue
	SharingConversations AttributeValue
	NetworkPriority      AttributeValue
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
	InhibitGet          AttributeValue  // Range: 0-1 (boolean), NotCollected = not available
	InhibitPut          AttributeValue  // Range: 0-1 (boolean), NotCollected = not available
	BackoutThreshold    AttributeValue  // Range: 0-999999999, NotCollected = not available
	TriggerDepth        AttributeValue  // Range: 1-999999999, NotCollected = not available
	TriggerType         AttributeValue  // Range: 0-3 (NONE/FIRST/EVERY/DEPTH), NotCollected = not available
	MaxMsgLength        AttributeValue  // Range: 0-max_int32, NotCollected = not available
	DefPriority         AttributeValue  // Range: 0-9, NotCollected = not available
	ServiceInterval     AttributeValue  // MQIA_Q_SERVICE_INTERVAL - Target service time in milliseconds
	RetentionInterval   AttributeValue  // MQIA_RETENTION_INTERVAL - Queue retention hours
	Scope               AttributeValue  // MQIA_SCOPE - Queue scope (0=MQSCO_Q_MGR, 1=MQSCO_CELL)
	Usage               AttributeValue  // MQIA_USAGE - Queue usage (0=MQUS_NORMAL, 1=MQUS_TRANSMISSION)
	MsgDeliverySequence AttributeValue  // MQIA_MSG_DELIVERY_SEQUENCE (0=MQMDS_PRIORITY, 1=MQMDS_FIFO)
	HardenGetBackout    AttributeValue  // MQIA_HARDEN_GET_BACKOUT - Backout persistence (0/1)
	DefPersistence      AttributeValue  // MQIA_DEF_PERSISTENCE (0=MQPER_NOT_PERSISTENT, 1=MQPER_PERSISTENT)
	
	// Additional status metrics (from MQCMD_INQUIRE_Q_STATUS)
	AvgQueueTime        AttributeValue  // MQIACF_Q_TIME_INDICATOR - Average time messages on queue (microseconds)
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
	
	// Connection count - number of active connections to the queue manager
	ConnectionCount AttributeValue  // Range: 0-max_connections, NotCollected = not available
	
	// Queue manager start date/time and calculated uptime
	StartDate AttributeValue  // Start date in YYYYMMDD format, NotCollected = not available
	StartTime AttributeValue  // Start time in HHMMSSSS format, NotCollected = not available  
	Uptime    AttributeValue  // Calculated uptime in seconds, NotCollected = not available
}

// ListenerStatus represents the status of a listener
type ListenerStatus int32

const (
	ListenerStatusStopped  ListenerStatus = 0  // MQSVC_STATUS_STOPPED
	ListenerStatusStarting ListenerStatus = 1  // MQSVC_STATUS_STARTING
	ListenerStatusRunning  ListenerStatus = 2  // MQSVC_STATUS_RUNNING
	ListenerStatusStopping ListenerStatus = 3  // MQSVC_STATUS_STOPPING
	ListenerStatusRetrying ListenerStatus = 4  // MQSVC_STATUS_RETRYING
)

// ListenerMetrics contains runtime metrics for a listener
type ListenerMetrics struct {
	Name        string
	Status      ListenerStatus
	Port        int64
	Backlog     AttributeValue  // Connection backlog limit
	IPAddress   string          // IP address the listener binds to
	Description string          // Listener description
	StartDate   string          // Start date in YYYYMMDD format
	StartTime   string          // Start time in HHMMSSSS format
	Uptime      int64           // Uptime in seconds (calculated)
}

// EnrichmentStats tracks the success/failure of enrichment operations
type EnrichmentStats struct {
	TotalItems  int64           // Items attempted to enrich
	OkItems     int64           // Successfully enriched
	FailedItems int64           // Failed to enrich
	ErrorCounts map[int32]int   // Count by MQ error code
}

// CollectionStats provides complete transparency for collection operations
type CollectionStats struct {
	Discovery struct {
		Success        bool            // Did discovery work at all?
		AvailableItems int64           // Total items discovered (all responses, success + errors)
		InvisibleItems int64           // Items that returned errors (any error code)
		IncludedItems  int64           // Visible items matched by selector
		ExcludedItems  int64           // Visible items excluded by selector
		UnparsedItems  int64           // Visible items we couldn't parse
		ErrorCounts    map[int32]int   // Count by MQ error code
	}
	
	Config  *EnrichmentStats       // Config enrichment stats (optional)
	Metrics *EnrichmentStats       // Metrics enrichment stats (optional)
	Reset   *EnrichmentStats       // Reset stats enrichment (optional)
}

// QueueCollectionResult contains queue metrics and collection statistics
type QueueCollectionResult struct {
	Queues []QueueMetrics
	Stats  CollectionStats
}

// ChannelCollectionResult contains channel metrics and collection statistics
type ChannelCollectionResult struct {
	Channels []ChannelMetrics
	Stats    CollectionStats
}

// TopicCollectionResult contains topic metrics and collection statistics
type TopicCollectionResult struct {
	Topics []TopicMetrics
	Stats  CollectionStats
}

// ListenerCollectionResult contains listener metrics and collection statistics
type ListenerCollectionResult struct {
	Listeners []ListenerMetrics
	Stats     CollectionStats
}

// PCFError represents an error with an MQ reason code
type PCFError struct {
	Message string
	Code    int32  // MQ reason code (MQRC_*)
}

func (e *PCFError) Error() string {
	return e.Message
}

// NewPCFError creates a new PCF error with reason code
func NewPCFError(code int32, format string, args ...interface{}) *PCFError {
	return &PCFError{
		Message: fmt.Sprintf(format, args...),
		Code:    code,
	}
}

// StatisticsType represents the type of statistics message
type StatisticsType int32

const (
	StatisticsTypeQueue   StatisticsType = 1 // MQCMD_STATISTICS_Q
	StatisticsTypeChannel StatisticsType = 2 // MQCMD_STATISTICS_CHANNEL
)

// QueueStatistics contains extended queue metrics from statistics queue
type QueueStatistics struct {
	Name string
	Type QueueType
	
	// Min/Max depth metrics
	MinDepth AttributeValue  // MQIAMO_Q_MIN_DEPTH - Minimum queue depth since statistics reset
	MaxDepth AttributeValue  // MQIAMO_Q_MAX_DEPTH - Maximum queue depth since statistics reset
	
	// Average queue time metrics (split by persistence)
	AvgQTimeNonPersistent AttributeValue  // MQIAMO64_AVG_Q_TIME[0] - Average time on queue for non-persistent messages (microseconds)
	AvgQTimePersistent    AttributeValue  // MQIAMO64_AVG_Q_TIME[1] - Average time on queue for persistent messages (microseconds)
	
	// Operation counters (split by persistence)
	PutsNonPersistent AttributeValue  // MQIAMO_PUTS[0] - Non-persistent put operations
	PutsPersistent    AttributeValue  // MQIAMO_PUTS[1] - Persistent put operations
	GetsNonPersistent AttributeValue  // MQIAMO_GETS[0] - Non-persistent get operations
	GetsPersistent    AttributeValue  // MQIAMO_GETS[1] - Persistent get operations
	
	// Byte counters (split by persistence)
	PutBytesNonPersistent AttributeValue  // MQIAMO64_PUT_BYTES[0] - Bytes put (non-persistent)
	PutBytesPersistent    AttributeValue  // MQIAMO64_PUT_BYTES[1] - Bytes put (persistent)
	GetBytesNonPersistent AttributeValue  // MQIAMO64_GET_BYTES[0] - Bytes retrieved (non-persistent)
	GetBytesPersistent    AttributeValue  // MQIAMO64_GET_BYTES[1] - Bytes retrieved (persistent)
	
	// Failure counters
	PutsFailed       AttributeValue  // MQIAMO_PUTS_FAILED - Failed put operations
	Put1sFailed      AttributeValue  // MQIAMO_PUT1S_FAILED - Failed put1 operations
	GetsFailed       AttributeValue  // MQIAMO_GETS_FAILED - Failed get operations
	BrowsesFailed    AttributeValue  // MQIAMO_BROWSES_FAILED - Failed browse operations
	
	// Message lifecycle counters
	MsgsExpired    AttributeValue  // MQIAMO_MSGS_EXPIRED - Messages that expired
	MsgsPurged     AttributeValue  // MQIAMO_MSGS_PURGED - Messages purged
	MsgsNotQueued  AttributeValue  // MQIAMO_MSGS_NOT_QUEUED - Messages not queued
	
	// Browse metrics
	BrowseCount AttributeValue  // MQIAMO_BROWSES - Browse operations
	BrowseBytes AttributeValue  // MQIAMO64_BROWSE_BYTES - Bytes browsed
	Put1Count   AttributeValue  // MQIAMO_PUT1S - Put1 operations
	
	// Timestamp when statistics were collected
	StartDate AttributeValue  // Statistics start date (YYYYMMDD format)
	StartTime AttributeValue  // Statistics start time (HHMMSSSS format)  
	EndDate   AttributeValue  // Statistics end date (YYYYMMDD format)
	EndTime   AttributeValue  // Statistics end time (HHMMSSSS format)
}

// ChannelStatistics contains extended channel metrics from statistics queue
type ChannelStatistics struct {
	Name   string
	Type   ChannelType
	Status ChannelStatus
	
	// Message metrics
	Messages      AttributeValue  // MQIAMO_MSGS - Messages sent/received
	Bytes         AttributeValue  // MQIAMO64_BYTES - Bytes sent/received
	FullBatches   AttributeValue  // MQIAMO_FULL_BATCHES - Full batches
	IncompleteBatches AttributeValue  // MQIAMO_INCOMPLETE_BATCHES - Incomplete batches
	AvgBatchSize  AttributeValue  // MQIAMO_AVG_BATCH_SIZE - Average batch size
	PutRetries    AttributeValue  // MQIAMO_PUT_RETRIES - Put retry count
	
	// Timestamp when statistics were collected
	StartDate AttributeValue  // Statistics start date (YYYYMMDD format)
	StartTime AttributeValue  // Statistics start time (HHMMSSSS format)
	EndDate   AttributeValue  // Statistics end date (YYYYMMDD format)
	EndTime   AttributeValue  // Statistics end time (HHMMSSSS format)
}

// StatisticsMessage represents a statistics message from SYSTEM.ADMIN.STATISTICS.QUEUE
type StatisticsMessage struct {
	Type     StatisticsType  // Type of statistics (queue or channel)
	Command  int32          // MQ command that generated the statistics
	
	// Parsed statistics data (only one will be populated based on Type)
	QueueStats   []QueueStatistics   // Queue statistics (if Type == StatisticsTypeQueue)
	ChannelStats []ChannelStatistics // Channel statistics (if Type == StatisticsTypeChannel)
	
	// Message metadata
	MessageLength int32      // Length of the statistics message
	PutDate       string     // Date message was put (YYYYMMDD)
	PutTime       string     // Time message was put (HHMMSSSS)
}

// StatisticsCollectionResult contains statistics metrics and collection information
type StatisticsCollectionResult struct {
	Messages []StatisticsMessage
	Stats    CollectionStats
}