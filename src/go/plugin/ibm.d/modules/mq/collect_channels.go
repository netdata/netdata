package mq

import (
	"strings"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

func (c *Collector) collectChannelMetrics() error {
	channels, err := c.client.GetChannelList()
	if err != nil {
		return err
	}

	// Track overview metrics
	var monitored, excluded, failed int64

	for _, channelName := range channels {
		// Apply system channel filtering
		if !c.Config.CollectSystemChannels && strings.HasPrefix(channelName, "SYSTEM.") {
			excluded++
			continue
		}
		
		// Additional filtering could be added here if needed
		// For now, we rely on the PCF glob pattern filtering

		metrics, err := c.client.GetChannelMetrics(channelName)
		if err != nil {
			c.Warningf("failed to get metrics for channel %s: %v", channelName, err)
			failed++
			continue
		}

		monitored++

		labels := contexts.ChannelLabels{
			Channel: channelName,
		}

		// Set channel status - convert enum to individual metrics
		statusValues := contexts.ChannelStatusValues{}
		
		// Set the active status to 1
		switch metrics.Status {
		case pcf.ChannelStatusInactive:
			statusValues.Inactive = 1
		case pcf.ChannelStatusBinding:
			statusValues.Binding = 1
		case pcf.ChannelStatusStarting:
			statusValues.Starting = 1
		case pcf.ChannelStatusRunning:
			statusValues.Running = 1
		case pcf.ChannelStatusStopping:
			statusValues.Stopping = 1
		case pcf.ChannelStatusRetrying:
			statusValues.Retrying = 1
		case pcf.ChannelStatusStopped:
			statusValues.Stopped = 1
		case pcf.ChannelStatusRequesting:
			statusValues.Requesting = 1
		case pcf.ChannelStatusPaused:
			statusValues.Paused = 1
		case pcf.ChannelStatusDisconnected:
			statusValues.Disconnected = 1
		case pcf.ChannelStatusInitializing:
			statusValues.Initializing = 1
		case pcf.ChannelStatusSwitching:
			statusValues.Switching = 1
		}
		
		contexts.Channel.Status.Set(c.State, labels, statusValues)
		
		// Set channel messages (incremental) - only if available
		if metrics.Messages != nil {
			contexts.Channel.Messages.Set(c.State, labels, contexts.ChannelMessagesValues{
				Messages: *metrics.Messages,
			})
		}
		
		// Set channel bytes (incremental) - only if available
		if metrics.Bytes != nil {
			contexts.Channel.Bytes.Set(c.State, labels, contexts.ChannelBytesValues{
				Bytes: *metrics.Bytes,
			})
		}
		
		// Set channel batches (incremental) - only if available
		if metrics.Batches != nil {
			contexts.Channel.Batches.Set(c.State, labels, contexts.ChannelBatchesValues{
				Batches: *metrics.Batches,
			})
		}
		
		// Collect channel configuration if enabled
		if c.Config.CollectChannelConfig {
			config, err := c.client.GetChannelConfig(channelName)
			if err != nil {
				c.Debugf("failed to get config for channel %s: %v", channelName, err)
			} else {
				// Set batch configuration - only if both values are collected
				if config.BatchSize.IsCollected() && config.BatchInterval.IsCollected() {
					contexts.Channel.BatchConfig.Set(c.State, labels, contexts.ChannelBatchConfigValues{
						Batch_size:     config.BatchSize.Int64(),
						Batch_interval: config.BatchInterval.Int64(),
					})
				}
				
				// Set intervals - only if all values are collected
				if config.DiscInterval.IsCollected() && config.HbInterval.IsCollected() && config.KeepAliveInterval.IsCollected() {
					contexts.Channel.Intervals.Set(c.State, labels, contexts.ChannelIntervalsValues{
						Disc_interval:       config.DiscInterval.Int64(),
						Hb_interval:         config.HbInterval.Int64(),
						Keep_alive_interval: config.KeepAliveInterval.Int64(),
					})
				}
				
				// Set retry configuration - only if both values are collected
				if config.ShortRetry.IsCollected() && config.LongRetry.IsCollected() {
					contexts.Channel.RetryConfig.Set(c.State, labels, contexts.ChannelRetryConfigValues{
						Short_retry: config.ShortRetry.Int64(),
						Long_retry:  config.LongRetry.Int64(),
					})
				}
				
				// Set max message length - only if collected
				if config.MaxMsgLength.IsCollected() {
					contexts.Channel.MaxMessageLength.Set(c.State, labels, contexts.ChannelMaxMessageLengthValues{
						Max_msg_length: config.MaxMsgLength.Int64(),
					})
				}
				
				// Set conversation configuration - only if both values are collected
				if config.SharingConversations.IsCollected() && config.NetworkPriority.IsCollected() {
					contexts.Channel.ConversationConfig.Set(c.State, labels, contexts.ChannelConversationConfigValues{
						Sharing_conversations: config.SharingConversations.Int64(),
						Network_priority:      config.NetworkPriority.Int64(),
					})
				}
			}
		}
	}
	
	// Set overview metrics
	c.setChannelOverviewMetrics(monitored, excluded, 0, failed)
	
	return nil
}