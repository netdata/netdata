
package mq

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"

// Config is the configuration for the collector.
type Config struct {
	framework.Config `yaml:",inline" json:",inline"`

	// IBM MQ Queue Manager name to connect to
	QueueManager string `yaml:"queue_manager" json:"queue_manager"`
	// IBM MQ channel name for connection
	Channel      string `yaml:"channel" json:"channel"`
	// IBM MQ server hostname or IP address
	Host         string `yaml:"host" json:"host"`
	// IBM MQ server port number
	Port         int    `yaml:"port" json:"port"`
	// Username for IBM MQ authentication
	User         string `yaml:"user" json:"user"`
	// Password for IBM MQ authentication
	Password     string `yaml:"password" json:"password"`

	// Enable collection of queue metrics
	CollectQueues   bool `yaml:"collect_queues" json:"collect_queues"`
	// Enable collection of channel metrics
	CollectChannels bool `yaml:"collect_channels" json:"collect_channels"`
	// Enable collection of topic metrics
	CollectTopics   bool `yaml:"collect_topics" json:"collect_topics"`
	// Enable collection of listener metrics
	CollectListeners bool `yaml:"collect_listeners" json:"collect_listeners"`

	// Enable collection of system queue metrics
	CollectSystemQueues   bool `yaml:"collect_system_queues" json:"collect_system_queues"`
	// Enable collection of system channel metrics
	CollectSystemChannels bool `yaml:"collect_system_channels" json:"collect_system_channels"`

	// Enable collection of channel configuration metrics
	CollectChannelConfig bool `yaml:"collect_channel_config" json:"collect_channel_config"`
	// Enable collection of queue configuration metrics
	CollectQueueConfig   bool `yaml:"collect_queue_config" json:"collect_queue_config"`

	// Pattern to filter queues (wildcards supported)
	QueueSelector   string `yaml:"queue_selector" json:"queue_selector"`
	// Pattern to filter channels (wildcards supported)
	ChannelSelector string `yaml:"channel_selector" json:"channel_selector"`

	// Enable collection of queue statistics (destructive operation)
	CollectResetQueueStats bool `yaml:"collect_reset_queue_stats" json:"collect_reset_queue_stats"`
}


