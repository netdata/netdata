package mq

import (
	"fmt"
	"strings"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

func (c *Collector) collectQueueMetrics() error {
	
	// Build queue selector pattern for efficient collection
	queuePattern := c.Config.QueueSelector
	if !c.Config.CollectSystemQueues {
		// If system queues are disabled and pattern is "*", exclude SYSTEM queues
		if queuePattern == "*" {
			// We'll filter SYSTEM queues after collection since PCF patterns don't support negation
		}
	}
	
	c.Debugf("Using queue pattern: '%s', reset_stats: %v", queuePattern, c.Config.CollectResetQueueStats)
	
	// Configure comprehensive queue collection
	config := pcf.QueueCollectionConfig{
		QueuePattern:      queuePattern,
		CollectResetStats: c.Config.CollectResetQueueStats,
	}
	
	// Protocol handles all the complex orchestration
	queueMetrics, err := c.client.GetQueueMetrics(config)
	if err != nil {
		return fmt.Errorf("failed to collect queue metrics: %w", err)
	}
	
	// Track overview metrics
	var monitored, excluded, failed int64
	
	// Process all collected metrics
	for _, queue := range queueMetrics {
		// Apply system queue filtering (post-collection since PCF doesn't support negation)
		if !c.Config.CollectSystemQueues && strings.HasPrefix(queue.Name, "SYSTEM.") {
			excluded++
			continue
		}
		
		// Additional filtering could be added here if needed
		// For now, we rely on the PCF glob pattern filtering
		
		monitored++
		
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
			contexts.Queue.Connections.Set(c.State, labels, contexts.QueueConnectionsValues{
				Input:  queue.OpenInputCount,
				Output: queue.OpenOutputCount,
			})
			
			// Oldest message age - send whatever we collected
			contexts.Queue.OldestMessageAge.Set(c.State, labels, contexts.QueueOldestMessageAgeValues{
				Oldest_msg_age: queue.OldestMsgAge,
			})
			
			if queue.UncommittedMsgs > 0 {
				// Only set uncommitted messages if there are any
				// TODO: Add uncommitted messages context if needed  
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
	
	// Set overview metrics
	c.setQueueOverviewMetrics(monitored, excluded, 0, 0, 0, failed)
	
	return nil
}