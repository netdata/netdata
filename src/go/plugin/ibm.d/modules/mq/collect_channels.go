package mq

import (
	"fmt"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

func (c *Collector) collectChannelMetrics() error {
	c.Debugf("Collecting channels with selector '%s', config: %v, system: %v", 
		c.Config.ChannelSelector, c.Config.CollectChannelConfig, c.Config.CollectSystemChannels)
	
	// Use new GetChannels with transparency
	result, err := c.client.GetChannels(
		c.Config.CollectChannelConfig, // collectConfig
		true,                          // collectMetrics (always)
		c.Config.MaxChannels,          // maxChannels (0 = no limit)
		c.Config.ChannelSelector,      // selector pattern
		c.Config.CollectSystemChannels, // collectSystem
	)
	if err != nil {
		return fmt.Errorf("failed to collect channel metrics: %w", err)
	}
	
	// Check discovery success
	if !result.Stats.Discovery.Success {
		c.Errorf("Channel discovery failed completely")
		return fmt.Errorf("channel discovery failed")
	}
	
	// Map transparency counters to user-facing semantics
	monitored := int64(0)
	if result.Stats.Metrics != nil {
		monitored = result.Stats.Metrics.OkItems
	}
	
	failed := result.Stats.Discovery.UnparsedItems
	if result.Stats.Metrics != nil {
		failed += result.Stats.Metrics.FailedItems
	}
	// Note: Config failures are not counted as they're optional
	
	// Update overview metrics with correct semantics  
	c.setChannelOverviewMetrics(
		monitored,                             // monitored (successfully enriched)
		result.Stats.Discovery.ExcludedItems,  // excluded (filtered by user)
		result.Stats.Discovery.InvisibleItems, // invisible (discovery errors)
		failed,                                // failed (unparsed + enrichment failures)
	)
	
	// Log collection summary
	c.Debugf("Channel collection complete - discovered:%d visible:%d included:%d collected:%d failed:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Channels),
		failed)
	
	// Process collected channel metrics
	for _, channel := range result.Channels {
		labels := contexts.ChannelLabels{
			Channel: channel.Name,
		}

		// Set channel status - convert enum to individual metrics
		statusValues := contexts.ChannelStatusValues{}
		
		// Set the active status to 1
		switch channel.Status {
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
		if channel.Messages != nil {
			contexts.Channel.Messages.Set(c.State, labels, contexts.ChannelMessagesValues{
				Messages: *channel.Messages,
			})
		}
		
		// Set channel bytes (incremental) - only if available
		if channel.Bytes != nil {
			contexts.Channel.Bytes.Set(c.State, labels, contexts.ChannelBytesValues{
				Bytes: *channel.Bytes,
			})
		}
		
		// Set channel batches (incremental) - only if available
		if channel.Batches != nil {
			contexts.Channel.Batches.Set(c.State, labels, contexts.ChannelBatchesValues{
				Batches: *channel.Batches,
			})
		}
		
		// Configuration metrics - only set when configuration collection is enabled
		if c.Config.CollectChannelConfig {
			// Set batch size - only if available
			if channel.BatchSize.IsCollected() {
				contexts.Channel.BatchSize.Set(c.State, labels, contexts.ChannelBatchSizeValues{
					Batch_size: channel.BatchSize.Int64(),
				})
			}
			
			// Set batch interval - only if available
			if channel.BatchInterval.IsCollected() {
				contexts.Channel.BatchInterval.Set(c.State, labels, contexts.ChannelBatchIntervalValues{
					Batch_interval: channel.BatchInterval.Int64(),
				})
			}
			
			// Set intervals - only if at least one is available and not -1 (which means disabled/not applicable)
			intervalValues := contexts.ChannelIntervalsValues{}
			hasAnyInterval := false
			
			if channel.DiscInterval.IsCollected() && channel.DiscInterval.Int64() != -1 {
				intervalValues.Disc_interval = channel.DiscInterval.Int64()
				hasAnyInterval = true
			}
			if channel.HbInterval.IsCollected() && channel.HbInterval.Int64() != -1 {
				intervalValues.Hb_interval = channel.HbInterval.Int64()
				hasAnyInterval = true
			}
			if channel.KeepAliveInterval.IsCollected() && channel.KeepAliveInterval.Int64() != -1 {
				intervalValues.Keep_alive_interval = channel.KeepAliveInterval.Int64()
				hasAnyInterval = true
			}
			
			// Only send the metric if we have at least one valid (non -1) interval
			if hasAnyInterval {
				contexts.Channel.Intervals.Set(c.State, labels, intervalValues)
			}
			
			// Set short retry count - only if available
			if channel.ShortRetry.IsCollected() {
				contexts.Channel.ShortRetryCount.Set(c.State, labels, contexts.ChannelShortRetryCountValues{
					Short_retry: channel.ShortRetry.Int64(),
				})
			}
			
			// Set long retry interval - only if available
			if channel.LongRetry.IsCollected() {
				contexts.Channel.LongRetryInterval.Set(c.State, labels, contexts.ChannelLongRetryIntervalValues{
					Long_retry: channel.LongRetry.Int64(),
				})
			}
			
			// Set max message length - only if available
			if channel.MaxMsgLength.IsCollected() {
				contexts.Channel.MaxMessageLength.Set(c.State, labels, contexts.ChannelMaxMessageLengthValues{
					Max_msg_length: channel.MaxMsgLength.Int64(),
				})
			}
			
			// Set sharing conversations - only if available
			if channel.SharingConversations.IsCollected() {
				contexts.Channel.SharingConversations.Set(c.State, labels, contexts.ChannelSharingConversationsValues{
					Sharing_conversations: channel.SharingConversations.Int64(),
				})
			}
			
			// Set network priority - only if available
			if channel.NetworkPriority.IsCollected() {
				contexts.Channel.NetworkPriority.Set(c.State, labels, contexts.ChannelNetworkPriorityValues{
					Network_priority: channel.NetworkPriority.Int64(),
				})
			}
		}
	}
	
	return nil
}