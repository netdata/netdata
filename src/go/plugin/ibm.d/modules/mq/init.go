
package mq

import (
	"context"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

func (c *Collector) Init(ctx context.Context) error {
	// Set configuration defaults first
	c.Config.SetDefaults()
	
	// Set this collector as the implementation
	c.SetImpl(c)
	
	// Copy framework configuration from module config to framework
	// Only if user provided values (non-zero)
	if c.Config.ObsoletionIterations != 0 {
		c.Collector.Config.ObsoletionIterations = c.Config.ObsoletionIterations
	}
	if c.Config.UpdateEvery != 0 {
		c.Collector.Config.UpdateEvery = c.Config.UpdateEvery
	}
	if c.Config.CollectionGroups != nil {
		c.Collector.Config.CollectionGroups = c.Config.CollectionGroups
	}

	// Register all contexts from generated code BEFORE base init
	c.RegisterContexts(contexts.GetAllContexts()...)

	// Initialize framework
	if err := c.Collector.Init(ctx); err != nil {
		return err
	}

	// Debug log the configuration values
	c.Debugf("Creating PCF client with config: QueueManager='%s', Host='%s', Port=%d, Channel='%s', User='%s'",
		c.Config.QueueManager, c.Config.Host, c.Config.Port, c.Config.Channel, c.Config.User)
	c.Debugf("Collection config: QueueSelector='%s', CollectResetQueueStats=%v, CollectSystemQueues=%v",
		c.Config.QueueSelector, c.Config.CollectResetQueueStats, c.Config.CollectSystemQueues)
	
	// Create client
	c.client = pcf.NewClient(pcf.Config{
		QueueManager: c.Config.QueueManager,
		Channel:      c.Config.Channel,
		Host:         c.Config.Host,
		Port:         c.Config.Port,
		User:         c.Config.User,
		Password:     c.Config.Password,
	}, c.State)

	// QueueSelector is used as a glob pattern for PCF commands, not regex
	// So we don't compile it as regex here - it's used directly in PCF calls
	c.Infof("Queue selector configured: %s (glob pattern)", c.Config.QueueSelector)
	
	// ChannelSelector is also used as glob pattern
	c.Infof("Channel selector configured: %s (glob pattern)", c.Config.ChannelSelector)

	// Warn about destructive statistics collection
	if c.Config.CollectResetQueueStats {
		c.Warningf("DESTRUCTIVE statistics collection is ENABLED!")
		c.Warningf("Queue message counters will be RESET TO ZERO after each collection!")
		c.Warningf("This WILL BREAK other monitoring tools using the same statistics!")
		c.Warningf("Only use this if Netdata is the ONLY monitoring tool for MQ!")
	}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	// Try to connect
	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("connection check failed: %w", err)
	}

	return nil
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.client != nil {
		c.client.Disconnect()
	}
}
