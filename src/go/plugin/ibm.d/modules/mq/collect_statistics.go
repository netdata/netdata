package mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

// collectStatistics collects advanced metrics from SYSTEM.ADMIN.STATISTICS.QUEUE
// This provides additional metrics like min/max depth, average queue time, and operation counts
// that are not available through regular PCF commands.
func (c *Collector) collectStatistics() error {
	if !c.Config.CollectStatisticsQueue {
		// Statistics collection is disabled
		return nil
	}

	c.Debugf("Collecting statistics from SYSTEM.ADMIN.STATISTICS.QUEUE")

	// Get statistics messages from the statistics queue
	result, err := c.client.GetStatisticsQueue()
	if err != nil {
		c.Warningf("Failed to collect statistics from SYSTEM.ADMIN.STATISTICS.QUEUE: %v", err)
		return nil // Don't fail the entire collection for statistics queue issues
	}

	c.Debugf("Retrieved %d statistics messages", len(result.Messages))

	// Process statistics messages
	for _, msg := range result.Messages {
		switch msg.Type {
		case pcf.StatisticsTypeQueue:
			if err := c.collectQueueStatistics(msg.QueueStats); err != nil {
				c.Warningf("Failed to process queue statistics: %v", err)
			}
		case pcf.StatisticsTypeChannel:
			if err := c.collectChannelStatistics(msg.ChannelStats); err != nil {
				c.Warningf("Failed to process channel statistics: %v", err)
			}
		case pcf.StatisticsTypeMQI:
			if err := c.collectMQIStatistics(msg.MQIStats); err != nil {
				c.Warningf("Failed to process MQI statistics: %v", err)
			}
		default:
			c.Debugf("Unknown statistics message type: %d", msg.Type)
		}
	}

	return nil
}

// collectQueueStatistics processes queue statistics messages
func (c *Collector) collectQueueStatistics(queueStats []pcf.QueueStatistics) error {
	for _, stat := range queueStats {
		queueName := stat.Name
		queueType := "local" // TODO: Convert stat.Type to string
		
		labels := contexts.QueueStatisticsLabels{
			Queue: queueName,
			Type:  queueType,
		}

		// Min/Max depth metrics
		if stat.MinDepth.IsCollected() && stat.MaxDepth.IsCollected() {
			contexts.QueueStatistics.DepthMinMax.Set(c.State, labels, contexts.QueueStatisticsDepthMinMaxValues{
				Min_depth: stat.MinDepth.Int64(),
				Max_depth: stat.MaxDepth.Int64(),
			})
			contexts.QueueStatistics.DepthMinMax.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// Average queue time metrics (split by persistence)
		if stat.AvgQTimeNonPersistent.IsCollected() && stat.AvgQTimePersistent.IsCollected() {
			contexts.QueueStatistics.AvgQueueTime.Set(c.State, labels, contexts.QueueStatisticsAvgQueueTimeValues{
				Non_persistent: stat.AvgQTimeNonPersistent.Int64(),
				Persistent:     stat.AvgQTimePersistent.Int64(),
			})
			contexts.QueueStatistics.AvgQueueTime.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}
		
		// Queue time indicators (short/long period)
		if stat.QTimeShort.IsCollected() && stat.QTimeLong.IsCollected() {
			contexts.QueueStatistics.QueueTimeIndicators.Set(c.State, labels, contexts.QueueStatisticsQueueTimeIndicatorsValues{
				Short_period: stat.QTimeShort.Int64(),
				Long_period:  stat.QTimeLong.Int64(),
			})
			contexts.QueueStatistics.QueueTimeIndicators.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// Operation counters (incremental - rates per second)
		contexts.QueueStatistics.Operations.Set(c.State, labels, contexts.QueueStatisticsOperationsValues{
			Puts_non_persistent: getValue(stat.PutsNonPersistent),
			Puts_persistent:     getValue(stat.PutsPersistent),
			Gets_non_persistent: getValue(stat.GetsNonPersistent),
			Gets_persistent:     getValue(stat.GetsPersistent),
			Put1s:               getValue(stat.Put1Count),
			Browses:             getValue(stat.BrowseCount),
		})
		contexts.QueueStatistics.Operations.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())

		// Byte counters (incremental - rates per second)
		contexts.QueueStatistics.Bytes.Set(c.State, labels, contexts.QueueStatisticsBytesValues{
			Put_bytes_non_persistent: getValue(stat.PutBytesNonPersistent),
			Put_bytes_persistent:     getValue(stat.PutBytesPersistent),
			Get_bytes_non_persistent: getValue(stat.GetBytesNonPersistent),
			Get_bytes_persistent:     getValue(stat.GetBytesPersistent),
			Browse_bytes:             getValue(stat.BrowseBytes),
		})
		contexts.QueueStatistics.Bytes.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())

		// Failure counters (incremental - rates per second)
		contexts.QueueStatistics.Failures.Set(c.State, labels, contexts.QueueStatisticsFailuresValues{
			Puts_failed:    getValue(stat.PutsFailed),
			Put1s_failed:   getValue(stat.Put1sFailed),
			Gets_failed:    getValue(stat.GetsFailed),
			Browses_failed: getValue(stat.BrowsesFailed),
		})
		contexts.QueueStatistics.Failures.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())

		// Message lifecycle counters (incremental - rates per second)
		contexts.QueueStatistics.MessageLifecycle.Set(c.State, labels, contexts.QueueStatisticsMessageLifecycleValues{
			Expired:    getValue(stat.MsgsExpired),
			Purged:     getValue(stat.MsgsPurged),
			Not_queued: getValue(stat.MsgsNotQueued),
		})
		contexts.QueueStatistics.MessageLifecycle.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())

		c.Debugf("Collected statistics for queue '%s' (type: %s)", queueName, queueType)
	}

	return nil
}

// collectChannelStatistics processes channel statistics messages
func (c *Collector) collectChannelStatistics(channelStats []pcf.ChannelStatistics) error {
	for _, stat := range channelStats {
		channelName := stat.Name
		channelType := "svrconn" // TODO: Convert stat.Type to string
		
		labels := contexts.ChannelStatisticsLabels{
			Channel: channelName,
			Type:    channelType,
		}

		// Message metrics (incremental - rates per second)
		if stat.Messages.IsCollected() {
			contexts.ChannelStatistics.Messages.Set(c.State, labels, contexts.ChannelStatisticsMessagesValues{
				Messages: stat.Messages.Int64(),
			})
			contexts.ChannelStatistics.Messages.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// Byte metrics (incremental - rates per second)
		if stat.Bytes.IsCollected() {
			contexts.ChannelStatistics.Bytes.Set(c.State, labels, contexts.ChannelStatisticsBytesValues{
				Bytes: stat.Bytes.Int64(),
			})
			contexts.ChannelStatistics.Bytes.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// Batch metrics (incremental - rates per second)
		contexts.ChannelStatistics.Batches.Set(c.State, labels, contexts.ChannelStatisticsBatchesValues{
			Full_batches:       getValue(stat.FullBatches),
			Incomplete_batches: getValue(stat.IncompleteBatches),
		})
		contexts.ChannelStatistics.Batches.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())

		// Average batch size (absolute value)
		if stat.AvgBatchSize.IsCollected() {
			contexts.ChannelStatistics.BatchSize.Set(c.State, labels, contexts.ChannelStatisticsBatchSizeValues{
				Avg_batch_size: stat.AvgBatchSize.Int64(),
			})
			contexts.ChannelStatistics.BatchSize.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// Put retry metrics (incremental - rates per second)
		if stat.PutRetries.IsCollected() {
			contexts.ChannelStatistics.PutRetries.Set(c.State, labels, contexts.ChannelStatisticsPutRetriesValues{
				Put_retries: stat.PutRetries.Int64(),
			})
			contexts.ChannelStatistics.PutRetries.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		c.Debugf("Collected statistics for channel '%s' (type: %s)", channelName, channelType)
	}

	return nil
}

// collectMQIStatistics processes MQI statistics messages
func (c *Collector) collectMQIStatistics(mqiStats []pcf.MQIStatistics) error {
	for _, stat := range mqiStats {
		// MQI stats are at queue manager level
		labels := contexts.MQIStatisticsLabels{
			Queue_manager: stat.Name,
		}

		// MQOPEN operations (incremental - rates per second)
		if stat.Opens.IsCollected() || stat.OpensFailed.IsCollected() {
			contexts.MQIStatistics.Opens.Set(c.State, labels, contexts.MQIStatisticsOpensValues{
				Opens_total:  getValue(stat.Opens),
				Opens_failed: getValue(stat.OpensFailed),
			})
			contexts.MQIStatistics.Opens.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// MQCLOSE operations (incremental - rates per second)
		if stat.Closes.IsCollected() || stat.ClosesFailed.IsCollected() {
			contexts.MQIStatistics.Closes.Set(c.State, labels, contexts.MQIStatisticsClosesValues{
				Closes_total:  getValue(stat.Closes),
				Closes_failed: getValue(stat.ClosesFailed),
			})
			contexts.MQIStatistics.Closes.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// MQINQ operations (incremental - rates per second)
		if stat.Inqs.IsCollected() || stat.InqsFailed.IsCollected() {
			contexts.MQIStatistics.Inqs.Set(c.State, labels, contexts.MQIStatisticsInqsValues{
				Inqs_total:  getValue(stat.Inqs),
				Inqs_failed: getValue(stat.InqsFailed),
			})
			contexts.MQIStatistics.Inqs.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		// MQSET operations (incremental - rates per second)
		if stat.Sets.IsCollected() || stat.SetsFailed.IsCollected() {
			contexts.MQIStatistics.Sets.Set(c.State, labels, contexts.MQIStatisticsSetsValues{
				Sets_total:  getValue(stat.Sets),
				Sets_failed: getValue(stat.SetsFailed),
			})
			contexts.MQIStatistics.Sets.SetUpdateEvery(c.State, labels, c.GetEffectiveStatisticsInterval())
		}

		c.Debugf("Collected MQI statistics for queue manager '%s'", stat.Name)
	}

	return nil
}

// getValue returns the int64 value of an AttributeValue, or 0 if not collected
func getValue(attr pcf.AttributeValue) int64 {
	if attr.IsCollected() {
		return attr.Int64()
	}
	return 0
}