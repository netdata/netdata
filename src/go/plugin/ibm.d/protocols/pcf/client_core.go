// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package pcf

// #include "pcf_helpers.h"
import "C"

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

const (
	maxMQGetBufferSize = 10 * 1024 * 1024 // 10MB maximum buffer size for MQGET responses
)

// Client is the PCF protocol client.
type Client struct {
	config          Config
	hConn           C.MQHCONN  // Connection handle
	hObj            C.MQHOBJ   // Object handle for admin queue
	hReplyObj       C.MQHOBJ   // Object handle for persistent reply queue
	replyQueueName  [48]C.char // Store the actual reply queue name
	connected       bool
	protocol        *framework.ProtocolClient
	
	// Job creation timestamp for filtering statistics messages
	jobCreationTime time.Time
	
	// Cached static data (refreshed on reconnection)
	cachedVersion      string
	cachedEdition      string
	cachedCommandLevel int32
	cachedPlatform     int32
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