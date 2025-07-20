
package mq

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

// defaultConfig returns a new Config with all default values set.
// This is used both for New() and for the module registration to ensure
// consistency and single source of truth for defaults.
func defaultConfig() Config {
	return Config{
		// Connection defaults
		QueueManager: "QM1",
		Host:         "localhost",
		Port:         1414,
		Channel:      "SYSTEM.DEF.SVRCONN",
		User:         "", // No authentication by default
		Password:     "", // No authentication by default
		
		// Collection defaults - collect everything by default
		CollectQueues:           true,
		CollectChannels:         true,
		CollectTopics:           true,
		CollectListeners:        true,
		CollectSystemQueues:     true,
		CollectSystemChannels:   true,
		CollectSystemTopics:     true,
		CollectSystemListeners:  true,
		CollectChannelConfig:    true,
		CollectQueueConfig:      true,
		
		// Only destructive operations are disabled by default
		CollectResetQueueStats: false,
		// Statistics queue collection is disabled by default (may not be available on all systems)
		CollectStatisticsQueue: false,
		// Statistics collection interval should match MQ STATINT setting (default 60 seconds)
		StatisticsInterval:     60,
		
		// Selector defaults - empty means collect nothing (user must explicitly configure)
		QueueSelector:   "",
		ChannelSelector: "",
		TopicSelector:   "",
		ListenerSelector: "",
		
		// Cardinality control defaults
		MaxQueues:    100,
		MaxChannels:  100,
		MaxTopics:    100,
		MaxListeners: 100,
	}
}

func (c *Collector) Init(ctx context.Context) error {
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

	// Debug log the complete configuration as JSON
	if configJSON, err := json.Marshal(c.Config); err == nil {
		c.Debugf("Running with configuration: %s", string(configJSON))
	}
	
	// Create client
	c.client = pcf.NewClient(pcf.Config{
		QueueManager: c.Config.QueueManager,
		Channel:      c.Config.Channel,
		Host:         c.Config.Host,
		Port:         c.Config.Port,
		User:         c.Config.User,
		Password:     c.Config.Password,
	}, c.State)

	// Log the selectors - these are applied locally after discovery
	if c.Config.QueueSelector == "" {
		c.Infof("Queue selector: empty (no queues will be collected)")
	} else if c.Config.QueueSelector == "*" {
		c.Infof("Queue selector: all queues will be collected")
	} else {
		c.Infof("Queue selector configured: %s (applied after discovery)", c.Config.QueueSelector)
	}
	
	if c.Config.ChannelSelector == "" {
		c.Infof("Channel selector: empty (no channels will be collected)")
	} else if c.Config.ChannelSelector == "*" {
		c.Infof("Channel selector: all channels will be collected")
	} else {
		c.Infof("Channel selector configured: %s (applied after discovery)", c.Config.ChannelSelector)
	}

	// Warn about destructive statistics collection
	if c.Config.CollectResetQueueStats {
		c.Warningf("DESTRUCTIVE statistics collection is ENABLED!")
		c.Warningf("Queue message counters will be RESET TO ZERO after each collection!")
		c.Warningf("This WILL BREAK other monitoring tools using the same statistics!")
		c.Warningf("Only use this if Netdata is the ONLY monitoring tool for MQ!")
	}

	// Log statistics queue collection status
	if c.Config.CollectStatisticsQueue {
		c.Infof("Statistics queue collection is ENABLED")
		c.Infof("Will collect advanced metrics from SYSTEM.ADMIN.STATISTICS.QUEUE")
		c.Infof("Note: Statistics must be enabled on the queue manager (STATQ, STATINT settings)")
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
