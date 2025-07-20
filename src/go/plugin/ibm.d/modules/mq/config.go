
package mq

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"

// Config is the configuration for the collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	// PCF Connection parameters
	QueueManager string `yaml:"queue_manager" json:"queue_manager"`
	Channel      string `yaml:"channel" json:"channel"`
	Host         string `yaml:"host" json:"host"`
	Port         int    `yaml:"port" json:"port"`
	User         string `yaml:"user" json:"user"`
	Password     string `yaml:"password" json:"password"`

	// Collection options
	CollectQueues   bool `yaml:"collect_queues" json:"collect_queues"`
	CollectChannels bool `yaml:"collect_channels" json:"collect_channels"`
	CollectTopics   bool `yaml:"collect_topics" json:"collect_topics"`

	// System objects collection
	CollectSystemQueues   bool `yaml:"collect_system_queues" json:"collect_system_queues"`
	CollectSystemChannels bool `yaml:"collect_system_channels" json:"collect_system_channels"`

	// Static configuration collection
	CollectChannelConfig bool `yaml:"collect_channel_config" json:"collect_channel_config"`
	CollectQueueConfig   bool `yaml:"collect_queue_config" json:"collect_queue_config"`

	// Filtering
	QueueSelector   string `yaml:"queue_selector" json:"queue_selector"`
	ChannelSelector string `yaml:"channel_selector" json:"channel_selector"`

	// Destructive statistics collection
	CollectResetQueueStats bool `yaml:"collect_reset_queue_stats" json:"collect_reset_queue_stats"`
}

// SetDefaults sets default values for the configuration
// Only sets defaults for unspecified values, preserves user configuration
func (c *Config) SetDefaults() {
	// Framework defaults
	if c.UpdateEvery == 0 {
		c.UpdateEvery = 1
	}
	if c.ObsoletionIterations == 0 {
		c.ObsoletionIterations = 60
	}
	
	// Connection defaults
	if c.Port == 0 {
		c.Port = 1414 // Default MQ port
	}
	if c.Channel == "" {
		c.Channel = "SYSTEM.DEF.SVRCONN"
	}
	
	// Collection defaults - preserve user settings by using a marker/flag approach
	// Note: Go's zero-value for bool is false, so we can't distinguish between
	// user-set false and uninitialized false. We'll only set true defaults.
	// For bool fields that default to false, we don't need to set them here.
	
	// These default to true only if not explicitly configured
	// TODO: This is a limitation of bool types - consider using *bool in the future
	// For now, we'll document that these default to true
	
	// Filtering defaults
	if c.QueueSelector == "" {
		c.QueueSelector = "*" // All queues by default
	}
	if c.ChannelSelector == "" {
		c.ChannelSelector = "*" // All channels by default
	}
	
	// Note: Bool fields with false defaults don't need to be set here since
	// Go's zero value for bool is false. User can explicitly set them to true.
}

