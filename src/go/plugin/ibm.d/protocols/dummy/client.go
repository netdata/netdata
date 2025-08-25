// Package dummy provides a mock protocol implementation for demonstration
// DO NOT USE IN PRODUCTION - This is only for testing the framework
package dummy

import (
	"fmt"
	"math/rand"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

// Config holds dummy client configuration
type Config struct {
	ConnectionString string
	Username         string
	Password         string
}

// Client is the dummy protocol client for demonstration
type Client struct {
	config  Config
	metrics *framework.ProtocolClient

	// Mock state
	connected bool
	random    *rand.Rand
}

// Data structures matching what real PCF would return
type QueueManagerInfo struct {
	Name    string
	Version string
}

type QueueData struct {
	Name             string
	Type             string
	CurrentDepth     int64
	MaxDepth         int64
	EnqueueCount     int64
	DequeueCount     int64
	OpenInputCount   int64
	OpenOutputCount  int64
	TriggerDepth     int64
	BackoutThreshold int64
	HasResetStats    bool
}

type ChannelData struct {
	Name          string
	Type          string
	Status        int32
	StatusName    string
	MessageCount  int64
	BytesSent     int64
	BytesReceived int64
	BatchCount    int64
}

type TopicData struct {
	Name            string
	PublisherCount  int64
	SubscriberCount int64
	MessageCount    int64
}

// NewClient creates a new dummy client
func NewClient(config Config, state *framework.CollectorState) *Client {
	return &Client{
		config:  config,
		metrics: framework.NewProtocolClient("dummy", state),
		random:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// Connect simulates establishing connection
func (c *Client) Connect() error {
	return c.metrics.Track("Connect", func() error {
		// Simulate connection delay
		time.Sleep(100 * time.Millisecond)

		if c.config.ConnectionString == "" {
			return fmt.Errorf("connection string required")
		}

		c.connected = true
		return nil
	})
}

// Disconnect simulates closing connection
func (c *Client) Disconnect() error {
	c.connected = false
	return nil
}

// GetQueueManagerInfo returns mock queue manager information
func (c *Client) GetQueueManagerInfo() (*QueueManagerInfo, error) {
	var info *QueueManagerInfo

	err := c.metrics.Track("GetQueueManagerInfo", func() error {
		if !c.connected {
			return fmt.Errorf("not connected")
		}

		// Mock data
		info = &QueueManagerInfo{
			Name:    "DUMMY_QM",
			Version: "9.3.0.0",
		}

		return nil
	})

	return info, err
}

// ListQueues returns mock queue names
func (c *Client) ListQueues(pattern string) ([]string, error) {
	var queues []string

	err := c.metrics.TrackWithSize("ListQueues", 128, func() (int64, error) {
		if !c.connected {
			return 0, fmt.Errorf("not connected")
		}

		// Mock data - simulate various queue types
		queues = []string{
			"DEV.QUEUE.1",
			"DEV.QUEUE.2",
			"APP.REQUEST",
			"APP.REPLY",
			"SYSTEM.ADMIN.COMMAND.QUEUE",
			"SYSTEM.DEFAULT.LOCAL.QUEUE",
		}

		// Simulate response size
		return int64(len(queues) * 64), nil
	})

	return queues, err
}

// GetQueueDetailsBatch returns mock queue details
func (c *Client) GetQueueDetailsBatch(names []string) ([]QueueData, error) {
	var data []QueueData

	err := c.metrics.TrackWithSize("GetQueueDetailsBatch", int64(len(names)*32), func() (int64, error) {
		if !c.connected {
			return 0, fmt.Errorf("not connected")
		}

		// Mock data for each queue
		for _, name := range names {
			qtype := "local"
			if name == "APP.REPLY" {
				qtype = "remote"
			} else if len(name) > 6 && name[:6] == "SYSTEM" {
				qtype = "local"
			}

			// Generate random but consistent data
			base := int64(c.random.Intn(1000))

			data = append(data, QueueData{
				Name:             name,
				Type:             qtype,
				CurrentDepth:     base + int64(c.random.Intn(100)),
				MaxDepth:         5000,
				EnqueueCount:     base * 100,
				DequeueCount:     base * 95,
				OpenInputCount:   int64(c.random.Intn(5)),
				OpenOutputCount:  int64(c.random.Intn(3)),
				TriggerDepth:     1000,
				BackoutThreshold: 3,
				HasResetStats:    true,
			})
		}

		// Simulate response size
		return int64(len(data) * 256), nil
	})

	return data, err
}

// ListChannels returns mock channel names
func (c *Client) ListChannels(pattern string) ([]string, error) {
	var channels []string

	err := c.metrics.Track("ListChannels", func() error {
		if !c.connected {
			return fmt.Errorf("not connected")
		}

		// Mock data
		channels = []string{
			"SYSTEM.DEF.SVRCONN",
			"SYSTEM.ADMIN.SVRCONN",
			"APP.CHANNEL.1",
			"APP.CHANNEL.2",
		}

		return nil
	})

	return channels, err
}

// GetChannelData returns mock channel details
func (c *Client) GetChannelData(name string) (*ChannelData, error) {
	var data *ChannelData

	err := c.metrics.Track("GetChannelData", func() error {
		if !c.connected {
			return fmt.Errorf("not connected")
		}

		// Mock data
		statuses := []string{"inactive", "running", "stopped"}
		statusName := statuses[c.random.Intn(len(statuses))]

		data = &ChannelData{
			Name:          name,
			Type:          "SVRCONN",
			Status:        int32(c.random.Intn(7)),
			StatusName:    statusName,
			MessageCount:  int64(c.random.Intn(10000)),
			BytesSent:     int64(c.random.Intn(1000000)),
			BytesReceived: int64(c.random.Intn(1000000)),
			BatchCount:    int64(c.random.Intn(1000)),
		}

		return nil
	})

	return data, err
}

// ListTopics returns mock topic names
func (c *Client) ListTopics(pattern string) ([]string, error) {
	var topics []string

	err := c.metrics.Track("ListTopics", func() error {
		if !c.connected {
			return fmt.Errorf("not connected")
		}

		// Mock data
		topics = []string{
			"Price/Stock/IBM",
			"Price/Stock/MSFT",
			"Sports/Football/Results",
		}

		return nil
	})

	return topics, err
}

// GetTopicData returns mock topic details
func (c *Client) GetTopicData(name string) (*TopicData, error) {
	var data *TopicData

	err := c.metrics.Track("GetTopicData", func() error {
		if !c.connected {
			return fmt.Errorf("not connected")
		}

		// Mock data
		data = &TopicData{
			Name:            name,
			PublisherCount:  int64(c.random.Intn(10)),
			SubscriberCount: int64(c.random.Intn(50)),
			MessageCount:    int64(c.random.Intn(10000)),
		}

		return nil
	})

	return data, err
}
