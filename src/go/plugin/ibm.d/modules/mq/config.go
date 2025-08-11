
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
	// Enable collection of subscription metrics
	CollectSubscriptions bool `yaml:"collect_subscriptions" json:"collect_subscriptions"`

	// Enable collection of system queue metrics (SYSTEM.* queues provide critical infrastructure visibility)
	CollectSystemQueues   bool `yaml:"collect_system_queues" json:"collect_system_queues"`
	// Enable collection of system channel metrics (SYSTEM.* channels show clustering and administrative health)
	CollectSystemChannels bool `yaml:"collect_system_channels" json:"collect_system_channels"`
	// Enable collection of system topic metrics (SYSTEM.* topics show internal messaging patterns)
	CollectSystemTopics   bool `yaml:"collect_system_topics" json:"collect_system_topics"`
	// Enable collection of system listener metrics (SYSTEM.* listeners show internal connectivity)
	CollectSystemListeners bool `yaml:"collect_system_listeners" json:"collect_system_listeners"`

	// Enable collection of channel configuration metrics
	CollectChannelConfig bool `yaml:"collect_channel_config" json:"collect_channel_config"`
	// Enable collection of queue configuration metrics
	CollectQueueConfig   bool `yaml:"collect_queue_config" json:"collect_queue_config"`

	// Pattern to filter queues (wildcards supported)
	QueueSelector   string `yaml:"queue_selector" json:"queue_selector"`
	// Pattern to filter channels (wildcards supported)
	ChannelSelector string `yaml:"channel_selector" json:"channel_selector"`
	// Pattern to filter topics (wildcards supported)
	TopicSelector   string `yaml:"topic_selector" json:"topic_selector"`
	// Pattern to filter listeners (wildcards supported)
	ListenerSelector string `yaml:"listener_selector" json:"listener_selector"`
	// Pattern to filter subscriptions (wildcards supported)
	SubscriptionSelector string `yaml:"subscription_selector" json:"subscription_selector"`
	// Maximum number of queues to collect (0 = no limit)
	MaxQueues       int    `yaml:"max_queues" json:"max_queues"`
	// Maximum number of channels to collect (0 = no limit)
	MaxChannels     int    `yaml:"max_channels" json:"max_channels"`
	// Maximum number of topics to collect (0 = no limit)
	MaxTopics       int    `yaml:"max_topics" json:"max_topics"`
	// Maximum number of listeners to collect (0 = no limit)
	MaxListeners    int    `yaml:"max_listeners" json:"max_listeners"`

	// Enable collection of queue statistics (destructive operation)
	CollectResetQueueStats bool `yaml:"collect_reset_queue_stats" json:"collect_reset_queue_stats"`
	// Enable collection of statistics queue metrics (SYSTEM.ADMIN.STATISTICS.QUEUE provides advanced metrics like min/max depth)
	CollectStatisticsQueue bool `yaml:"collect_statistics_queue" json:"collect_statistics_queue"`
	
	// Enable collection of $SYS topic metrics (provides Queue Manager CPU, memory, and log utilization)
	CollectSysTopics bool `yaml:"collect_sys_topics" json:"collect_sys_topics"`
	
	// Statistics collection interval in seconds (auto-detected STATINT overwrites this value)
	StatisticsInterval int `yaml:"statistics_interval,omitempty" json:"statistics_interval,omitempty"`
	// $SYS topic collection interval in seconds (user override for customized MQ configurations)  
	SysTopicInterval   int `yaml:"sys_topic_interval,omitempty" json:"sys_topic_interval,omitempty"`
}


