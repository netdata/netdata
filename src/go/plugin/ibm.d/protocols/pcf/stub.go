//go:build !cgo
// +build !cgo

package pcf

import (
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"time"
)

// Stub implementations for when CGO is disabled

type Client struct {
	state *framework.CollectorState
}

func NewClient(state *framework.CollectorState) *Client {
	return &Client{state: state}
}

func (c *Client) Connect(config ConnectionConfig) error {
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

func (c *Client) GetQueues(pattern string) ([]QueueInfo, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetChannels(pattern string) ([]ChannelInfo, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetListeners() ([]ListenerInfo, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetTopics(pattern string) ([]TopicInfo, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetSubscriptions(pattern string) ([]SubscriptionInfo, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetResourceUsageMonitor() (*ResourceUsage, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) ResetQueueStatistics(queueName string) (*QueueResetStats, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

func (c *Client) GetQueueStatistics(queueName string) (*QueueStatistics, error) {
	return nil, errors.New("PCF protocol requires CGO support")
}

// Type stubs

type ConnectionConfig struct {
	QueueManager string
	Host         string
	Port         int
	Channel      string
	User         string
	Password     string
}

type QueueManagerInfo struct {
	Name    string
	Status  string
	Version string
}

type QueueInfo struct {
	Name                string
	Type                string
	CurrentDepth        int64
	MaxDepth            int64
	OpenInputCount      int64
	OpenOutputCount     int64
	InhibitGet          bool
	InhibitPut          bool
	OldestMessageAge    int64
	LastGetDate         time.Time
	LastPutDate         time.Time
	MonitoringLevel     int32
	UncommittedMessages int64
	QueueTime           *QueueTime
}

type QueueTime struct {
	ShortSamples int64
	LongSamples  int64
}

type ChannelInfo struct {
	Name            string
	Type            string
	Status          int32
	ConnectionName  string
	Messages        int64
	BytesSent       int64
	BytesReceived   int64
	BuffersSent     int64
	BuffersReceived int64
}

type ListenerInfo struct {
	Name   string
	Port   int32
	Status int32
}

type TopicInfo struct {
	Name            string
	PublishCount    int64
	SubscriberCount int64
}

type SubscriptionInfo struct {
	Name         string
	Topic        string
	MessageCount int64
}

type ResourceUsage struct {
	CPUUsed                    int64
	MemoryUsed                 int64
	QueueManagerFileSystemUsed int64
	QueueManagerFileSystemFree int64
}

type QueueResetStats struct {
	GetCount int64
	PutCount int64
}

type QueueStatistics struct {
	GetCount int64
	PutCount int64
}
