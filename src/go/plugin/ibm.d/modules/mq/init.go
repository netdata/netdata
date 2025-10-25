package mq

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

func compileMatcher(patterns []string) (matcher.Matcher, error) {
	if len(patterns) == 0 {
		return nil, nil
	}

	expr := strings.Join(patterns, " ")
	expr = strings.TrimSpace(expr)
	if expr == "" {
		return nil, nil
	}

	return matcher.NewSimplePatternsMatcher(expr)
}

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
		CollectQueues:          true,
		CollectChannels:        true,
		CollectTopics:          true,
		CollectListeners:       true,
		CollectSubscriptions:   true,
		CollectSystemQueues:    true,
		CollectSystemChannels:  true,
		CollectSystemTopics:    true,
		CollectSystemListeners: true,
		CollectChannelConfig:   true,
		CollectQueueConfig:     true,

		// Only destructive operations are disabled by default
		CollectResetQueueStats: false,
		// Statistics queue collection is disabled by default (may not be available on all systems)
		CollectStatisticsQueue: false,
		// $SYS topic collection is disabled by default (requires MQ 9.0+)
		CollectSysTopics: false,

		// Interval defaults
		StatisticsInterval: 60, // Default 60s, auto-detected STATINT overwrites if available
		SysTopicInterval:   10, // Default 10s per IBM docs, user can override if customized

		// Queue selection defaults: monitor core system queues, ignore churny patterns
		IncludeQueues: []string{
			"SYSTEM.DEAD.LETTER.QUEUE",
			"SYSTEM.ADMIN.COMMAND.QUEUE",
			"SYSTEM.ADMIN.STATISTICS.QUEUE",
		},
		ExcludeQueues: []string{
			"SYSTEM.*",
			"AMQ.*",
		},

		ChannelSelector:      "",
		TopicSelector:        "",
		ListenerSelector:     "",
		SubscriptionSelector: "",

		// Cardinality control defaults
		MaxQueues:    50,
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

	// Statistics contexts will be configured with appropriate update interval
	// when they are first collected during statistics collection
	if c.Config.CollectStatisticsQueue {
		c.Infof("Statistics collection enabled - interval will be determined during Check()")
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

	includeMatcher, err := compileMatcher(c.Config.IncludeQueues)
	if err != nil {
		return fmt.Errorf("invalid include_queues patterns: %w", err)
	}
	excludeMatcher, err := compileMatcher(c.Config.ExcludeQueues)
	if err != nil {
		return fmt.Errorf("invalid exclude_queues patterns: %w", err)
	}
	c.queueIncludeMatcher = includeMatcher
	c.queueExcludeMatcher = excludeMatcher

	if len(c.Config.IncludeQueues) == 0 {
		c.Infof("Queue include patterns: none (all queues eligible)")
	} else {
		c.Infof("Queue include patterns: %v", c.Config.IncludeQueues)
	}
	if len(c.Config.ExcludeQueues) == 0 {
		c.Infof("Queue exclude patterns: none")
	} else {
		c.Infof("Queue exclude patterns: %v", c.Config.ExcludeQueues)
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

	// Log $SYS topic collection status
	if c.Config.CollectSysTopics {
		c.Infof("$SYS topic collection is ENABLED")
		c.Infof("Will collect resource metrics (CPU, memory, log) from $SYS topics")
		c.Infof("Note: Requires MQ 9.0+ with MONINT configured")
	}

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	// Try to connect
	if err := c.client.Connect(); err != nil {
		return fmt.Errorf("connection check failed: %w", err)
	}

	// Auto-detect intervals from queue manager configuration
	c.resolveIntervals()

	return nil
}

func (c *Collector) Cleanup(ctx context.Context) {
	if c.client != nil {
		c.client.Disconnect()
	}
}
