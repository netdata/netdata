// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import (
	"fmt"
	"time"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
	"github.com/ibm-messaging/mq-golang/v5/mqmetric"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

const (
	maxMQGetBufferSize = 10 * 1024 * 1024 // 10MB maximum buffer size for MQGET responses
)

// ResourceStatus represents the state of resource monitoring discovery
type ResourceStatus int

const (
	ResourceStatusDisabled ResourceStatus = 0 // Default - not attempted yet
	ResourceStatusEnabled  ResourceStatus = 1 // Discovery succeeded, monitoring active
	ResourceStatusFailed   ResourceStatus = 2 // Discovery failed permanently, never try again
)

// Client is the PCF protocol client using IBM library.
type Client struct {
	config Config
	
	// IBM connection management (Connection #1: PCF commands)
	qmgr       ibmmq.MQQueueManager
	cmdQueue   ibmmq.MQObject
	replyQueue ibmmq.MQObject
	replyQueueName string // Store the actual reply queue name
	connected  bool
	protocol   *framework.ProtocolClient
	
	// Job creation timestamp for filtering statistics messages
	jobCreationTime time.Time
	
	// Cached static data (refreshed on reconnection)
	cachedVersion      string
	cachedEdition      string
	cachedCommandLevel int32
	cachedPlatform     int32
	
	// Cached queue manager configuration intervals (refreshed on reconnection)
	cachedStatisticsInterval       int32  // STATINT - Statistics interval in seconds (MQIA_STATISTICS_INTERVAL = 131)
	cachedMonitoringQueue          int32  // Queue monitoring interval in seconds (MQIA_MONITORING_Q = 123)
	cachedMonitoringChannel        int32  // Channel monitoring interval in seconds (MQIA_MONITORING_CHANNEL = 122)
	cachedMonitoringAutoClussdr    int32  // Auto-defined cluster sender channel monitoring interval (MQIA_MONITORING_AUTO_CLUSSDR = 124)
	cachedSysTopicInterval         int32  // $SYS topic publication interval in seconds (configurable, default 10s)
	
	// Resource monitoring (Connection #2: mqmetric)
	resourceConfig            *mqmetric.DiscoverConfig
	resourceMonitoringEnabled bool   // Track if resource monitoring should be enabled
	resourceStatus            ResourceStatus // Global status: disabled(0), enabled(1), failed(2)
	metricsReady              bool   // Track if metrics connection is ready
}

// Config is the configuration for the PCF client.
type Config struct {
	QueueManager string
	Channel      string
	Host         string
	Port         int
	User         string
	Password     string
}

// NewClient creates a new PCF client.
func NewClient(config Config, state *framework.CollectorState) *Client {
	return &Client{
		config:          config,
		protocol:        framework.NewProtocolClient("pcf", state),
		jobCreationTime: time.Now(), // Record job creation time for statistics filtering
	}
}

// IsConnected returns true if the client is connected to the queue manager.
func (c *Client) IsConnected() bool {
	return c.connected
}

// GetVersion returns the cached version and edition information
func (c *Client) GetVersion() (string, string, error) {
	if !c.IsConnected() {
		return "", "", fmt.Errorf("not connected")
	}
	return c.cachedVersion, c.cachedEdition, nil
}

// GetConnectionInfo returns the version, edition, and endpoint information
func (c *Client) GetConnectionInfo() (version, edition, endpoint string, err error) {
	if !c.IsConnected() {
		return "", "", "", fmt.Errorf("not connected")
	}
	endpoint = fmt.Sprintf("%s:%d", c.config.Host, c.config.Port)
	return c.cachedVersion, c.cachedEdition, endpoint, nil
}

// GetCommandLevel returns the cached command level
func (c *Client) GetCommandLevel() int32 {
	return c.cachedCommandLevel
}

// GetStatisticsInterval returns the cached STATINT value in seconds (0 if not available)
func (c *Client) GetStatisticsInterval() int32 {
	return c.cachedStatisticsInterval
}

// GetMonitoringQueueInterval returns the cached queue monitoring interval in seconds (0 if not available)
func (c *Client) GetMonitoringQueueInterval() int32 {
	return c.cachedMonitoringQueue
}

// GetMonitoringChannelInterval returns the cached channel monitoring interval in seconds (0 if not available)
func (c *Client) GetMonitoringChannelInterval() int32 {
	return c.cachedMonitoringChannel
}

// GetMonitoringAutoClussdrInterval returns the cached auto-defined cluster sender channel monitoring interval in seconds (0 if not available)
func (c *Client) GetMonitoringAutoClussdrInterval() int32 {
	return c.cachedMonitoringAutoClussdr
}

// GetSysTopicInterval returns the cached $SYS topic publication interval in seconds (0 if not available)
func (c *Client) GetSysTopicInterval() int32 {
	return c.cachedSysTopicInterval
}

// IsResourceMonitoringEnabled returns whether resource monitoring is enabled
func (c *Client) IsResourceMonitoringEnabled() bool {
	return c.resourceMonitoringEnabled
}

// GetResourceStatus returns the current resource monitoring status
func (c *Client) GetResourceStatus() ResourceStatus {
	return c.resourceStatus
}

// IsResourceMonitoringAvailable returns whether resource monitoring is available
func (c *Client) IsResourceMonitoringAvailable() bool {
	return c.metricsReady
}