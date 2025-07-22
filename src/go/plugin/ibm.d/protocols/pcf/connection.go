// SPDX-License-Identifier: GPL-3.0-or-later

package pcf

import (
	"fmt"
	"os"

	"github.com/ibm-messaging/mq-golang/v5/ibmmq"
	"github.com/ibm-messaging/mq-golang/v5/mqmetric"
)

// Connect connects to the queue manager using IBM library.
func (c *Client) Connect() error {
	c.protocol.Debugf("protocol starting connection attempt to queue manager '%s' on %s:%d via channel '%s' (user: '%s')",
		c.config.QueueManager, c.config.Host, c.config.Port, c.config.Channel, c.config.User)

	backoff := c.protocol.GetBackoff()
	attemptCount := 0
	err := backoff.Retry(func() error {
		attemptCount++
		c.protocol.Debugf("connection attempt #%d to %s:%d (queue manager: '%s', channel: '%s')",
			attemptCount, c.config.Host, c.config.Port, c.config.QueueManager, c.config.Channel)
		return c.doConnect()
	})
	if err != nil {
		c.protocol.Errorf("connection failed to queue manager '%s' at %s:%d after %d attempts: %v",
			c.config.QueueManager, c.config.Host, c.config.Port, attemptCount, err)
		c.protocol.Debugf("Connect FAILED")
		return err
	}
	c.protocol.MarkConnected()
	c.protocol.Debugf("successfully connected to queue manager '%s' at %s:%d (channel: '%s', user: '%s') after %d attempts",
		c.config.QueueManager, c.config.Host, c.config.Port, c.config.Channel, c.config.User, attemptCount)
	return nil
}

func (c *Client) doConnect() error {
	if c.connected {
		c.protocol.Debugf("already connected to queue manager '%s' at %s:%d, skipping connection attempt",
			c.config.QueueManager, c.config.Host, c.config.Port)
		return nil
	}

	// CRITICAL: IBM MQ PCF fails with LC_ALL=C locale
	// The MQ client library has issues with string handling in the C locale,
	// which causes PCF responses to be garbled and unparseable
	if currentLocale := os.Getenv("LC_ALL"); currentLocale == "C" {
		c.protocol.Warningf("LC_ALL environment variable is set to 'C', changing to 'en_US.UTF-8' to avoid MQ client library issues")
		if err := os.Setenv("LC_ALL", "en_US.UTF-8"); err != nil {
			c.protocol.Warningf("failed to change LC_ALL environment variable: %v", err)
		}
	}

	// Connection #1: IBM PCF connection for administrative commands
	if err := c.connectPCF(); err != nil {
		return fmt.Errorf("PCF connection failed: %w", err)
	}

	// Connection #2: Initialize IBM resource monitoring (separate connection)
	if err := c.initResourceMonitoring(); err != nil {
		c.protocol.Warningf("Resource monitoring initialization failed: %v", err)
		// Continue - resource monitoring is optional, PCF commands still work
		c.metricsReady = false
	} else {
		c.metricsReady = true
	}

	// Refresh static data on successful connection
	if err := c.refreshStaticData(); err != nil {
		c.protocol.Warningf("failed to refresh static data: %v", err)
		// Continue - not critical for basic operation
	}

	c.connected = true
	return nil
}

func (c *Client) connectPCF() error {
	c.protocol.Debugf("initializing IBM MQ connection to %s:%d", c.config.Host, c.config.Port)

	// Create connection options
	cno := ibmmq.NewMQCNO()
	cno.Options = ibmmq.MQCNO_CLIENT_BINDING

	// Create channel definition
	cd := ibmmq.NewMQCD()
	cd.ChannelName = c.config.Channel
	cd.ConnectionName = fmt.Sprintf("%s(%d)", c.config.Host, c.config.Port)

	// Set up authentication if credentials are provided
	if c.config.User != "" && c.config.Password != "" {
		csp := ibmmq.NewMQCSP()
		csp.AuthenticationType = ibmmq.MQCSP_AUTH_USER_ID_AND_PWD
		csp.UserId = c.config.User
		csp.Password = c.config.Password
		cno.SecurityParms = csp
		c.protocol.Debugf("authentication configured for user '%s'", c.config.User)
	}

	cno.ClientConn = cd

	// Connect to queue manager
	qmgr, err := ibmmq.Connx(c.config.QueueManager, cno)
	if err != nil {
		return fmt.Errorf("failed to connect to queue manager '%s': %w", c.config.QueueManager, err)
	}
	c.qmgr = qmgr

	c.protocol.Debugf("successfully connected to queue manager '%s'", c.config.QueueManager)

	// Open command queue
	if err := c.openCommandQueue(); err != nil {
		c.qmgr.Disc()
		return fmt.Errorf("failed to open command queue: %w", err)
	}

	// Create reply queue
	if err := c.createReplyQueue(); err != nil {
		c.cmdQueue.Close(0)
		c.qmgr.Disc()
		return fmt.Errorf("failed to create reply queue: %w", err)
	}

	return nil
}

func (c *Client) openCommandQueue() error {
	mqod := ibmmq.NewMQOD()
	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = "SYSTEM.ADMIN.COMMAND.QUEUE"

	openOptions := ibmmq.MQOO_OUTPUT | ibmmq.MQOO_FAIL_IF_QUIESCING

	obj, err := c.qmgr.Open(mqod, openOptions)
	if err != nil {
		return fmt.Errorf("failed to open SYSTEM.ADMIN.COMMAND.QUEUE: %w", err)
	}
	c.cmdQueue = obj

	c.protocol.Debugf("successfully opened SYSTEM.ADMIN.COMMAND.QUEUE")
	return nil
}

func (c *Client) createReplyQueue() error {
	// Create dynamic reply queue
	mqod := ibmmq.NewMQOD()
	mqod.ObjectType = ibmmq.MQOT_Q
	mqod.ObjectName = "SYSTEM.DEFAULT.MODEL.QUEUE"
	mqod.DynamicQName = "NETDATA.REPLY.*"

	openOptions := ibmmq.MQOO_INPUT_EXCLUSIVE | ibmmq.MQOO_FAIL_IF_QUIESCING

	obj, err := c.qmgr.Open(mqod, openOptions)
	if err != nil {
		return fmt.Errorf("failed to create reply queue: %w", err)
	}
	c.replyQueue = obj
	c.replyQueueName = obj.Name

	c.protocol.Debugf("successfully created reply queue: '%s'", c.replyQueueName)
	return nil
}

func (c *Client) initResourceMonitoring() error {
	// DISABLE: Resource monitoring needs to be properly implemented
	// The mqmetric library expects pre-existing queues, not model queues
	// This requires a different initialization approach than what was migrated

	c.protocol.Debugf("resource monitoring disabled - requires proper mqmetric integration")
	return fmt.Errorf("resource monitoring not yet implemented in migrated PCF client")
}

func (c *Client) refreshStaticData() error {
	// Get queue manager information to populate cached static data
	info, err := c.getQueueManagerInfo()
	if err != nil {
		return fmt.Errorf("failed to get queue manager info: %w", err)
	}

	c.cachedVersion = info.Version
	c.cachedEdition = info.Edition
	c.cachedCommandLevel = info.CommandLevel
	c.cachedPlatform = info.Platform

	c.protocol.Debugf("QMGR refreshed static data: version=%s, edition=%s, command_level=%d, platform=%d",
		c.cachedVersion, c.cachedEdition, c.cachedCommandLevel, c.cachedPlatform)

	return nil
}

func (c *Client) getQueueManagerInfo() (*QueueManagerInfo, error) {
	// Use the migrated PCF commands to get queue manager information
	err := c.refreshStaticDataFromPCF()
	if err != nil {
		return nil, err
	}

	return &QueueManagerInfo{
		Version:      c.cachedVersion,
		Edition:      c.cachedEdition,
		CommandLevel: c.cachedCommandLevel,
		Platform:     c.cachedPlatform,
	}, nil
}

// Disconnect closes both connections and cleans up resources
func (c *Client) Disconnect() {
	if !c.connected {
		c.protocol.Debugf("not connected, skipping disconnect")
		return
	}

	c.protocol.Debugf("disconnecting from queue manager '%s'", c.config.QueueManager)

	// Close PCF connection (Connection #1)
	if c.replyQueue != (ibmmq.MQObject{}) {
		c.replyQueue.Close(0)
		c.protocol.Debugf("closed reply queue")
	}

	if c.cmdQueue != (ibmmq.MQObject{}) {
		c.cmdQueue.Close(0)
		c.protocol.Debugf("closed command queue")
	}

	if c.connected {
		c.qmgr.Disc()
		c.protocol.Debugf("disconnected from queue manager")
	}

	// Close mqmetric connection (Connection #2)
	mqmetric.EndConnection()
	c.protocol.Debugf("closed metrics connection")

	c.connected = false
	c.metricsReady = false

	c.protocol.Debugf("disconnect complete")
}
