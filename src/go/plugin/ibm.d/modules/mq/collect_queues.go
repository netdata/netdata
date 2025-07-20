package mq

import (
	"fmt"
	"time"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

// convertMQDateTimeToSecondsSince converts MQ date (YYYYMMDD) and time (HHMMSSSS) to seconds since that time
// Returns -1 if the date/time is invalid or not collected
func convertMQDateTimeToSecondsSince(date, timeVal pcf.AttributeValue) int64 {
	if !date.IsCollected() || !timeVal.IsCollected() {
		return -1
	}
	
	dateInt := date.Int64()
	timeInt := timeVal.Int64()
	
	// Extract date components from YYYYMMDD
	year := dateInt / 10000
	month := (dateInt % 10000) / 100
	day := dateInt % 100
	
	// Extract time components from HHMMSSSS
	hour := timeInt / 1000000
	minute := (timeInt % 1000000) / 10000
	second := (timeInt % 10000) / 100
	centisecond := timeInt % 100
	
	// Create time.Time object
	t := time.Date(int(year), time.Month(month), int(day), 
		int(hour), int(minute), int(second), int(centisecond)*10000000, 
		time.UTC)
	
	// Calculate seconds since that time
	secondsSince := int64(time.Since(t).Seconds())
	
	// Return -1 if the time is in the future or too far in the past (invalid)
	if secondsSince < 0 || secondsSince > 365*24*60*60 { // More than a year
		return -1
	}
	
	return secondsSince
}

func (c *Collector) collectQueueMetrics() error {
	c.Debugf("Collecting queues with selector '%s', config: %v, reset_stats: %v", 
		c.Config.QueueSelector, c.Config.CollectQueueConfig, c.Config.CollectResetQueueStats)
	
	// Use new GetQueues with transparency
	result, err := c.client.GetQueues(
		c.Config.CollectQueueConfig,     // collectConfig
		true,                            // collectMetrics (always)
		c.Config.CollectResetQueueStats, // collectReset
		c.Config.MaxQueues,              // maxQueues (0 = no limit)
		c.Config.QueueSelector,          // selector pattern
	)
	if err != nil {
		return fmt.Errorf("failed to collect queue metrics: %w", err)
	}
	
	// Check discovery success
	if !result.Stats.Discovery.Success {
		c.Errorf("Queue discovery failed completely")
		return fmt.Errorf("queue discovery failed")
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
	// Note: Config and Reset failures are not counted as they're optional
	
	// Update overview metrics with correct semantics  
	c.setQueueOverviewMetrics(
		monitored,                             // monitored (successfully enriched)
		result.Stats.Discovery.ExcludedItems,  // excluded (filtered by user)
		result.Stats.Discovery.InvisibleItems, // invisible (discovery errors)
		failed,                                // failed (unparsed + enrichment failures)
	)
	
	// Log collection summary
	c.Debugf("Queue collection complete - discovered:%d visible:%d included:%d collected:%d failed:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems - result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Queues),
		failed)
	
	// Process collected queue metrics
	for _, queue := range result.Queues {
		// Note: Protocol already applied selector and system queue filtering
		// We just need to set the metrics
		
		labels := contexts.QueueLabels{
			Queue: queue.Name,
			Type:  pcf.QueueTypeString(int32(queue.Type)),
		}
		
		// Basic metrics (always available)
		contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
			Current: queue.CurrentDepth,
			Max:     queue.MaxDepth,
		})
		
		// Status metrics (if status collection succeeded)
		if queue.HasStatusMetrics {
			// Only send connections if both values are collected
			if queue.OpenInputCount.IsCollected() && queue.OpenOutputCount.IsCollected() {
				contexts.Queue.Connections.Set(c.State, labels, contexts.QueueConnectionsValues{
					Input:  queue.OpenInputCount.Int64(),
					Output: queue.OpenOutputCount.Int64(),
				})
			}
			
			// Oldest message age - only send if collected
			if queue.OldestMsgAge.IsCollected() {
				contexts.Queue.OldestMessageAge.Set(c.State, labels, contexts.QueueOldestMessageAgeValues{
					Oldest_msg_age: queue.OldestMsgAge.Int64(),
				})
			}
			
			// Uncommitted messages - only send if collected
			if queue.UncommittedMsgs.IsCollected() {
				contexts.Queue.UncommittedMessages.Set(c.State, labels, contexts.QueueUncommittedMessagesValues{
					Uncommitted: queue.UncommittedMsgs.Int64(),
				})
			}
			
			// Last activity times - calculate seconds since last get/put
			sinceLastGet := convertMQDateTimeToSecondsSince(queue.LastGetDate, queue.LastGetTime)
			sinceLastPut := convertMQDateTimeToSecondsSince(queue.LastPutDate, queue.LastPutTime)
			
			// Only send if we have at least one valid time
			if sinceLastGet >= 0 || sinceLastPut >= 0 {
				// If one is invalid, use -1 to indicate no activity
				if sinceLastGet < 0 {
					sinceLastGet = -1
				}
				if sinceLastPut < 0 {
					sinceLastPut = -1
				}
				
				contexts.Queue.LastActivity.Set(c.State, labels, contexts.QueueLastActivityValues{
					Since_last_get: sinceLastGet,
					Since_last_put: sinceLastPut,
				})
			}
		}
		
		// Message count metrics (if reset stats were collected)
		if queue.HasResetStats {
			contexts.Queue.Messages.Set(c.State, labels, contexts.QueueMessagesValues{
				Enqueued: queue.EnqueueCount,
				Dequeued: queue.DequeueCount,
			})
			
			contexts.Queue.HighDepth.Set(c.State, labels, contexts.QueueHighDepthValues{
				High_depth: queue.HighDepth,
			})
			
			// TODO: Add time since reset context if needed
			// TimeSinceReset: queue.TimeSinceReset
		}
		
		// Configuration metrics - only send when actually collected
		// Basic configuration (inhibit status, max message length)
		if queue.InhibitGet.IsCollected() && queue.InhibitPut.IsCollected() {
			contexts.Queue.InhibitStatus.Set(c.State, labels, contexts.QueueInhibitStatusValues{
				Inhibit_get: queue.InhibitGet.Int64(),
				Inhibit_put: queue.InhibitPut.Int64(),
			})
		}
		
		if queue.MaxMsgLength.IsCollected() {
			contexts.Queue.MaxMessageLength.Set(c.State, labels, contexts.QueueMaxMessageLengthValues{
				Max_msg_length: queue.MaxMsgLength.Int64(),
			})
		}
		
		// Detailed configuration only if explicitly enabled
		if c.Config.CollectQueueConfig {
			if queue.DefPriority.IsCollected() {
				contexts.Queue.Priority.Set(c.State, labels, contexts.QueuePriorityValues{
					Def_priority: queue.DefPriority.Int64(),
				})
			}
			
			if queue.TriggerDepth.IsCollected() && queue.TriggerType.IsCollected() {
				contexts.Queue.Triggers.Set(c.State, labels, contexts.QueueTriggersValues{
					Trigger_depth: queue.TriggerDepth.Int64(),
					Trigger_type:  queue.TriggerType.Int64(),
				})
			}
			
			if queue.BackoutThreshold.IsCollected() {
				contexts.Queue.BackoutThreshold.Set(c.State, labels, contexts.QueueBackoutThresholdValues{
					Backout_threshold: queue.BackoutThreshold.Int64(),
				})
			}
		}
	}
	
	return nil
}