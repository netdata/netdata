//go:build !cgo

package pcf

import (
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"math"
	"time"
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

func (t ChannelType) String() string {
	return "unknown"
}

// ChannelStatus represents the status of MQ channel
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

func (s ChannelStatus) String() string {
	return "unknown"
}

// ListenerStatus represents the status of MQ listener
type ListenerStatus int32

const (
	ListenerStatusStopped  ListenerStatus = 0
	ListenerStatusStarting ListenerStatus = 1
	ListenerStatusRunning  ListenerStatus = 2
	ListenerStatusStopping ListenerStatus = 3
	ListenerStatusRetrying ListenerStatus = 4
)

// Stub implementations for when CGO is disabled

type Config struct {
	QueueManager string
	Channel      string
	Host         string
	Port         int
	User         string
	Password     string
}

type Client struct {
	state  *framework.CollectorState
	config Config
}

func NewClient(config Config, state *framework.CollectorState) *Client {
	return &Client{
		config: config,
		state:  state,
	}
}

func (c *Client) Connect() error {
	return errors.New("PCF protocol requires CGO support for IBM MQ library")
}

func (c *Client) IsConnected() bool {
	return false
}

func (c *Client) Disconnect() error {
	return nil
}

func (c *Client) GetQueueManagerInfo() (*QueueManagerInfo, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

type QueueManagerMetrics struct {
	Status          int64
	ConnectionCount AttributeValue
	StartDate       AttributeValue
	StartTime       AttributeValue
	Uptime          AttributeValue
}

func (c *Client) GetQueueManagerStatus() (*QueueManagerMetrics, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetConnectionInfo() (version, edition, endpoint string, err error) {
	return "", "", "", errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetQueues(collectConfig, collectMetrics, collectReset bool, maxQueues int, selector string, collectSystem bool) (*QueueCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetChannels(collectConfig, collectMetrics bool, maxChannels int, selector string, collectSystem bool) (*ChannelCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetListeners(collectConfig bool, maxListeners int, selector string, collectSystem bool) (*ListenerCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetTopics(collectMetrics bool, maxTopics int, selector string, collectSystem bool) (*TopicCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetSubscriptions(maxSubscriptions int, selector string) (*SubscriptionCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) ResetQueueStatistics(pattern string) error {
	return errors.New("PCF protocol requires CGO support")
}

func (c *Client) CollectStatistics() (*StatisticsCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetQueueStatistics(name string) (*QueueStatistics, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetStatisticsQueue() (*StatisticsCollectionResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetStatisticsInterval() int {
	return 0
}

// Resource Monitor Methods (IBM $SYS topics)
func (c *Client) IsResourceMonitoringSupported() bool {
	return false
}

func (c *Client) EnableResourceMonitoring() error {
	return errors.New("PCF protocol requires CGO support")
}

type ResourcePublicationsResult struct {
	Stats            CollectionStats
	UserCPUPercent   AttributeValue
	SystemCPUPercent AttributeValue
	AvailableMemory  AttributeValue
	UsedMemory       AttributeValue
	MemoryUsedMB     AttributeValue
	LogUsedBytes     AttributeValue
	LogMaxBytes      AttributeValue
}

func (c *Client) GetResourcePublications() (*ResourcePublicationsResult, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetResourceMonitorData() (map[string]interface{}, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

// ParseMQDateTime stub
func ParseMQDateTime(dateStr, timeStr string) (time.Time, error) {
	return time.Time{}, errors.New("PCF protocol requires CGO support")
}

// QueueTypeString returns a string representation of queue type
func QueueTypeString(t int32) string {
	return "unknown"
}

// Stub type definitions
type ConnectionConfig struct {
	QueueManager string
	Host         string
	Port         int
	Channel      string
	User         string
	Password     string
}

type QueueManagerInfo struct {
	Name         string
	Status       int32
	Connections  int32
	StartDate    string
	StartTime    string
	Platform     string
	CommandLevel int32
	IsBroker     bool
}

type QueueInfo struct {
	Name  string
	Type  string
	Usage string
}

type QueueMetrics struct {
	Name                string
	Type                AttributeValue
	CurrentDepth        int64
	MaxDepth            int64
	DepthPercentage     float64
	OpenInputCount      AttributeValue
	OpenOutputCount     AttributeValue
	EnqueueCount        int64
	DequeueCount        int64
	HighDepth           int64
	TimeSinceReset      int64
	OldestMsgAge        AttributeValue
	UncommittedMsgs     AttributeValue
	LastGetDate         AttributeValue
	LastGetTime         AttributeValue
	LastPutDate         AttributeValue
	LastPutTime         AttributeValue
	HasStatusMetrics    bool
	HasResetStats       bool
	CurrentFileSize     AttributeValue
	CurrentMaxFileSize  AttributeValue
	QTimeShort          AttributeValue
	QTimeLong           AttributeValue
	InhibitGet          AttributeValue
	InhibitPut          AttributeValue
	BackoutThreshold    AttributeValue
	TriggerDepth        AttributeValue
	TriggerType         AttributeValue
	MaxMsgLength        AttributeValue
	DefPriority         AttributeValue
	ServiceInterval     AttributeValue
	RetentionInterval   AttributeValue
	Scope               AttributeValue
	Usage               AttributeValue
	MsgDeliverySequence AttributeValue
	HardenGetBackout    AttributeValue
	DefPersistence      AttributeValue
}

type ChannelInfo struct {
	Name           string
	Type           string
	ConnectionName string
}

type ChannelMetrics struct {
	Name   string
	Type   ChannelType
	Status ChannelStatus

	// Message metrics (only for message channels)
	Messages *int64
	Bytes    *int64
	Batches  *int64

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

	// Extended status metrics
	BuffersSent         AttributeValue
	BuffersReceived     AttributeValue
	CurrentMessages     AttributeValue
	XmitQueueTime       AttributeValue
	MCAStatus           AttributeValue
	InDoubtStatus       AttributeValue
	SSLKeyResets        AttributeValue
	NPMSpeed            AttributeValue
	CurrentSharingConvs AttributeValue
	ConnectionName      string
}

type ListenerInfo struct {
	Name      string
	Port      int32
	IPAddress string
}

type ListenerMetrics struct {
	Name        string
	Status      ListenerStatus
	Port        int64
	Backlog     AttributeValue
	IPAddress   string
	Description string
	StartDate   string
	StartTime   string
	Uptime      int64
}

type TopicInfo struct {
	Name string
	Type string
}

type TopicMetrics struct {
	Name        string
	TopicString string

	Publishers  int64
	Subscribers int64

	PublishMsgCount int64

	LastPubDate AttributeValue
	LastPubTime AttributeValue
}

type QueueResetStats struct {
	GetCount int64
	PutCount int64
}

type QueueType int32

type QueueStatistics struct {
	Name string
	Type QueueType

	MinDepth AttributeValue
	MaxDepth AttributeValue

	AvgQTimeNonPersistent AttributeValue
	AvgQTimePersistent    AttributeValue

	// Put operations
	PutsCount             AttributeValue
	PutsNonPersistent     AttributeValue
	PutsPersistent        AttributeValue
	PutBytesNonPersistent AttributeValue
	PutBytesPersistent    AttributeValue
	Put1Count             AttributeValue
	PutsFailed            AttributeValue
	Put1sFailed           AttributeValue

	// Get operations
	GetsCount             AttributeValue
	GetsNonPersistent     AttributeValue
	GetsPersistent        AttributeValue
	GetBytesNonPersistent AttributeValue
	GetBytesPersistent    AttributeValue
	GetsFailed            AttributeValue

	// Browse operations
	BrowseCount   AttributeValue
	BrowseBytes   AttributeValue
	BrowsesFailed AttributeValue

	// Rollback and backout
	RollbackCount AttributeValue
	BackoutCount  AttributeValue
	MsgsExpired   AttributeValue
	MsgsPurged    AttributeValue
	MsgsNotQueued AttributeValue

	QTimeShort AttributeValue
	QTimeLong  AttributeValue
}

type ChannelStatistics struct {
	Name   string
	Type   ChannelType
	Status ChannelStatus

	Messages          AttributeValue
	Bytes             AttributeValue
	FullBatches       AttributeValue
	IncompleteBatches AttributeValue
	AvgBatchSize      AttributeValue

	BuffersSent     AttributeValue
	BuffersReceived AttributeValue
	PutRetries      AttributeValue

	NetworkTime     AttributeValue
	ExitTime        AttributeValue
	BatchTime       AttributeValue
	SpeedIndicators [4]AttributeValue
}

type MQIStatistics struct {
	Name string

	Opens       AttributeValue
	OpensFailed AttributeValue

	Closes       AttributeValue
	ClosesFailed AttributeValue

	Inqs       AttributeValue
	InqsFailed AttributeValue

	Sets       AttributeValue
	SetsFailed AttributeValue

	Puts       AttributeValue
	PutBytes   AttributeValue
	PutsFailed AttributeValue

	Gets       AttributeValue
	GetBytes   AttributeValue
	GetsFailed AttributeValue

	Cbs       AttributeValue
	CbsFailed AttributeValue

	Commits  AttributeValue
	Backs    AttributeValue
	SubsReqs AttributeValue
	SubsPubs AttributeValue
	Browses  AttributeValue
}

type StatisticsType int

const (
	StatisticsTypeQueue   StatisticsType = 1
	StatisticsTypeChannel StatisticsType = 2
	StatisticsTypeMQI     StatisticsType = 3
)

type StatisticsMessage struct {
	Type         StatisticsType
	Command      int32
	QueueStats   []QueueStatistics
	ChannelStats []ChannelStatistics
	MQIStats     []MQIStatistics
}

type StatisticsCollectionResult struct {
	Messages []StatisticsMessage
}

type SubscriptionMetrics struct {
	Name            string
	TopicString     string
	Type            AttributeValue
	MessageCount    AttributeValue
	LastMessageDate string
	LastMessageTime string
}

// Collection result types
type CollectionStats struct {
	Discovery struct {
		Success        bool
		AvailableItems int64
		InvisibleItems int64
		IncludedItems  int64
		ExcludedItems  int64
		UnparsedItems  int64
		ErrorCounts    map[int32]int
		Total          int
		Matched        int
	}
	Config  *EnrichmentStats
	Metrics *EnrichmentStats
	Reset   *EnrichmentStats
}

type EnrichmentStats struct {
	TotalItems  int64
	OkItems     int64
	FailedItems int64
	ErrorCounts map[int32]int
}

type CollectionPhaseStats struct {
	Total       int
	Success     int
	Errors      int
	Filtered    int
	OkItems     int64
	FailedItems int64
}

type QueueCollectionResult struct {
	Queues []QueueMetrics
	Stats  CollectionStats
}

type ChannelCollectionResult struct {
	Channels []ChannelMetrics
	Stats    CollectionStats
}

type TopicCollectionResult struct {
	Topics []TopicMetrics
	Stats  CollectionStats
}

type ListenerCollectionResult struct {
	Listeners []ListenerMetrics
	Stats     CollectionStats
}

type SubscriptionCollectionResult struct {
	Subscriptions []SubscriptionMetrics
	Stats         CollectionStats
}
