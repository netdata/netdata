package mq

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/protocols/pcf"
)

type queueGroupAggregate struct {
	DepthCurrent int64
	DepthMax     int64

	MessagesEnqueued int64
	MessagesDequeued int64
	HasMessages      bool

	ConnectionsInput  int64
	ConnectionsOutput int64
	HasConnections    bool

	Uncommitted    int64
	HasUncommitted bool

	FileSizeCurrent int64
	FileSizeMax     int64
	HasFileSize     bool

	OldestMessageAge int64
	HasOldest        bool
}

func (a *queueGroupAggregate) add(queue *pcf.QueueMetrics) {
	a.DepthCurrent += queue.CurrentDepth
	a.DepthMax += queue.MaxDepth

	if queue.HasResetStats {
		a.MessagesEnqueued += queue.EnqueueCount
		a.MessagesDequeued += queue.DequeueCount
		a.HasMessages = true
	}

	if queue.HasStatusMetrics {
		if queue.OpenInputCount.IsCollected() {
			a.ConnectionsInput += queue.OpenInputCount.Int64()
			a.HasConnections = true
		}
		if queue.OpenOutputCount.IsCollected() {
			a.ConnectionsOutput += queue.OpenOutputCount.Int64()
			a.HasConnections = true
		}
		if queue.UncommittedMsgs.IsCollected() {
			a.Uncommitted += queue.UncommittedMsgs.Int64()
			a.HasUncommitted = true
		}
		if queue.CurrentFileSize.IsCollected() {
			a.FileSizeCurrent += queue.CurrentFileSize.Int64()
			a.HasFileSize = true
		}
		if queue.CurrentMaxFileSize.IsCollected() {
			a.FileSizeMax += queue.CurrentMaxFileSize.Int64()
			a.HasFileSize = true
		}
		if queue.OldestMsgAge.IsCollected() {
			age := queue.OldestMsgAge.Int64()
			if age >= 0 {
				if !a.HasOldest || age > a.OldestMessageAge {
					a.OldestMessageAge = age
				}
				a.HasOldest = true
			}
		}
	}
}

func queueGroupKey(name string) string {
	if name == "" {
		return "__unknown__"
	}
	if strings.HasPrefix(name, "SYSTEM.") {
		return "SYSTEM"
	}
	parts := strings.Split(name, ".")
	switch len(parts) {
	case 0:
		return "__unknown__"
	case 1:
		return parts[0]
	default:
		return strings.Join(parts[:2], ".")
	}
}

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

func (c *Collector) shouldCollectQueue(name string) bool {
	included := true
	if len(c.Config.IncludeQueues) > 0 {
		if c.queueIncludeMatcher == nil {
			return false
		}
		included = c.queueIncludeMatcher.MatchString(name)
		if !included {
			return false
		}
	}

	if c.queueExcludeMatcher != nil && c.queueExcludeMatcher.MatchString(name) {
		if len(c.Config.IncludeQueues) > 0 {
			return included
		}
		return false
	}

	return true
}

func (c *Collector) collectQueueMetrics() error {
	c.Debugf("Collecting queues include=%v exclude=%v config=%v reset_stats=%v system=%v",
		c.Config.IncludeQueues, c.Config.ExcludeQueues, c.Config.CollectQueueConfig, c.Config.CollectResetQueueStats, c.Config.CollectSystemQueues)

	result, err := c.client.GetQueues(
		c.Config.CollectQueueConfig,
		true,
		c.Config.CollectResetQueueStats,
		0,   // fetch everything; we enforce limits locally
		"*", // selector - we perform filtering ourselves
		c.Config.CollectSystemQueues,
	)
	if err != nil {
		return fmt.Errorf("failed to collect queue metrics: %w", err)
	}

	if !result.Stats.Discovery.Success {
		c.Errorf("Queue discovery failed completely")
		return fmt.Errorf("queue discovery failed")
	}

	failed := result.Stats.Discovery.UnparsedItems
	if result.Stats.Metrics != nil {
		failed += result.Stats.Metrics.FailedItems
	}

	filtered := make([]*pcf.QueueMetrics, 0, len(result.Queues))
	for i := range result.Queues {
		queue := result.Queues[i]
		if c.shouldCollectQueue(queue.Name) {
			queueCopy := queue
			filtered = append(filtered, &queueCopy)
		}
	}

	excludedByFilter := int64(len(result.Queues) - len(filtered))
	monitored := int64(len(filtered))

	c.setQueueOverviewMetrics(
		monitored,
		result.Stats.Discovery.ExcludedItems+excludedByFilter,
		result.Stats.Discovery.InvisibleItems,
		failed,
	)

	if len(filtered) == 0 {
		c.Debugf("No queues matched the include/exclude patterns")
		c.clearWarnOnce("queue_overflow")
		return nil
	}

	sort.Slice(filtered, func(i, j int) bool {
		return filtered[i].Name < filtered[j].Name
	})

	limit := c.Config.MaxQueues
	if limit < 0 {
		limit = 0
	}

	aggregated := make(map[string]*queueGroupAggregate)
	overflowTotals := &queueGroupAggregate{}
	overflowCount := 0
	overflowGroups := make(map[string]int)
	overflowExamples := make(map[string]string)

	for idx, queue := range filtered {
		groupKey := queueGroupKey(queue.Name)
		agg := aggregated[groupKey]
		if agg == nil {
			agg = &queueGroupAggregate{}
			aggregated[groupKey] = agg
		}
		agg.add(queue)

		if limit == 0 || idx < limit {
			c.emitPerQueueMetrics(queue)
			continue
		}

		overflowTotals.add(queue)
		overflowCount++
		overflowGroups[groupKey]++
		if _, exists := overflowExamples[groupKey]; !exists {
			overflowExamples[groupKey] = queue.Name
		}
	}

	c.emitQueueGroupMetrics(aggregated)

	if overflowCount > 0 {
		c.emitPerQueueOverflowMetrics(overflowTotals)

		parts := make([]string, 0, len(overflowGroups))
		for group, count := range overflowGroups {
			sample := overflowExamples[group]
			parts = append(parts, fmt.Sprintf("%s:%d (e.g. %s)", group, count, sample))
		}
		sort.Strings(parts)
		c.warnOnce("queue_overflow", "too many queues for per-queue charts (MaxQueues=%d). Aggregated %d additional queues: %s", limit, overflowCount, strings.Join(parts, ", "))
	} else {
		c.clearWarnOnce("queue_overflow")
	}

	c.Debugf("queue collection complete - discovered:%d matched:%d overflow:%d groups:%d",
		len(result.Queues), len(filtered), overflowCount, len(aggregated))

	return nil
}

func (c *Collector) emitPerQueueMetrics(queue *pcf.QueueMetrics) {
	labels := contexts.QueueLabels{
		Queue: queue.Name,
		Type:  pcf.QueueTypeString(int32(queue.Type)),
	}

	contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
		Current: queue.CurrentDepth,
		Max:     queue.MaxDepth,
	})

	if queue.MaxDepth > 0 {
		percentage := float64(queue.CurrentDepth) / float64(queue.MaxDepth) * 100.0
		contexts.Queue.DepthPercentage.Set(c.State, labels, contexts.QueueDepthPercentageValues{
			Percentage: int64(percentage * 1000),
		})
	}

	if queue.HasStatusMetrics {
		if queue.OpenInputCount.IsCollected() && queue.OpenOutputCount.IsCollected() {
			contexts.Queue.Connections.Set(c.State, labels, contexts.QueueConnectionsValues{
				Input:  queue.OpenInputCount.Int64(),
				Output: queue.OpenOutputCount.Int64(),
			})
		}

		if queue.OldestMsgAge.IsCollected() && queue.OldestMsgAge.Int64() != -1 {
			contexts.Queue.OldestMessageAge.Set(c.State, labels, contexts.QueueOldestMessageAgeValues{
				Oldest_msg_age: queue.OldestMsgAge.Int64(),
			})
		}

		if queue.UncommittedMsgs.IsCollected() {
			contexts.Queue.UncommittedMessages.Set(c.State, labels, contexts.QueueUncommittedMessagesValues{
				Uncommitted: queue.UncommittedMsgs.Int64(),
			})
		}

		if queue.CurrentFileSize.IsCollected() || queue.CurrentMaxFileSize.IsCollected() {
			fileSizeValues := contexts.QueueFileSizeValues{}
			hasAny := false
			if queue.CurrentFileSize.IsCollected() {
				fileSizeValues.Current = queue.CurrentFileSize.Int64()
				hasAny = true
			}
			if queue.CurrentMaxFileSize.IsCollected() {
				fileSizeValues.Max = queue.CurrentMaxFileSize.Int64()
				hasAny = true
			}
			if hasAny {
				contexts.Queue.FileSize.Set(c.State, labels, fileSizeValues)
			}
		}

		if queue.QTimeShort.IsCollected() && queue.QTimeLong.IsCollected() &&
			queue.QTimeShort.Int64() != -1 && queue.QTimeLong.Int64() != -1 {
			contexts.Queue.QueueTimeIndicators.Set(c.State, labels, contexts.QueueQueueTimeIndicatorsValues{
				Short_period: queue.QTimeShort.Int64(),
				Long_period:  queue.QTimeLong.Int64(),
			})
		}

		sinceLastGet := convertMQDateTimeToSecondsSince(queue.LastGetDate, queue.LastGetTime)
		sinceLastPut := convertMQDateTimeToSecondsSince(queue.LastPutDate, queue.LastPutTime)
		if sinceLastGet >= 0 || sinceLastPut >= 0 {
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

	if queue.HasResetStats {
		contexts.Queue.Messages.Set(c.State, labels, contexts.QueueMessagesValues{
			Enqueued: queue.EnqueueCount,
			Dequeued: queue.DequeueCount,
		})
	}

	contexts.Queue.HighDepth.Set(c.State, labels, contexts.QueueHighDepthValues{
		High_depth: queue.HighDepth,
	})

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

	if !c.Config.CollectQueueConfig {
		return
	}

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

	if queue.ServiceInterval.IsCollected() {
		contexts.Queue.ServiceInterval.Set(c.State, labels, contexts.QueueServiceIntervalValues{
			Service_interval: queue.ServiceInterval.Int64(),
		})
	}

	if queue.RetentionInterval.IsCollected() {
		contexts.Queue.RetentionInterval.Set(c.State, labels, contexts.QueueRetentionIntervalValues{
			Retention_interval: queue.RetentionInterval.Int64(),
		})
	}

	if queue.DefPersistence.IsCollected() {
		persistent := int64(0)
		nonPersistent := int64(0)
		if queue.DefPersistence.Int64() == 1 {
			persistent = 1
		} else {
			nonPersistent = 1
		}
		contexts.Queue.MessagePersistence.Set(c.State, labels, contexts.QueueMessagePersistenceValues{
			Persistent:     persistent,
			Non_persistent: nonPersistent,
		})
	}

	if queue.Scope.IsCollected() {
		queueManager := int64(0)
		cell := int64(0)
		if queue.Scope.Int64() == 0 {
			queueManager = 1
		} else {
			cell = 1
		}
		contexts.Queue.QueueScope.Set(c.State, labels, contexts.QueueQueueScopeValues{
			Queue_manager: queueManager,
			Cell:          cell,
		})
	}

	if queue.Usage.IsCollected() {
		normal := int64(0)
		transmission := int64(0)
		if queue.Usage.Int64() == 0 {
			normal = 1
		} else {
			transmission = 1
		}
		contexts.Queue.QueueUsage.Set(c.State, labels, contexts.QueueQueueUsageValues{
			Normal:       normal,
			Transmission: transmission,
		})
	}

	if queue.MsgDeliverySequence.IsCollected() {
		priority := int64(0)
		fifo := int64(0)
		if queue.MsgDeliverySequence.Int64() == 0 {
			priority = 1
		} else {
			fifo = 1
		}
		contexts.Queue.MessageDeliverySequence.Set(c.State, labels, contexts.QueueMessageDeliverySequenceValues{
			Priority: priority,
			Fifo:     fifo,
		})
	}

	if queue.HardenGetBackout.IsCollected() {
		enabled := int64(0)
		disabled := int64(0)
		if queue.HardenGetBackout.Int64() == 1 {
			enabled = 1
		} else {
			disabled = 1
		}
		contexts.Queue.HardenGetBackout.Set(c.State, labels, contexts.QueueHardenGetBackoutValues{
			Enabled:  enabled,
			Disabled: disabled,
		})
	}
}

func (c *Collector) emitPerQueueOverflowMetrics(total *queueGroupAggregate) {
	if total == nil {
		return
	}

	labels := contexts.QueueLabels{
		Queue: "__other__",
		Type:  "aggregated",
	}

	contexts.Queue.Depth.Set(c.State, labels, contexts.QueueDepthValues{
		Current: total.DepthCurrent,
		Max:     total.DepthMax,
	})

	if total.DepthMax > 0 {
		percentage := float64(total.DepthCurrent) / float64(total.DepthMax) * 100.0
		contexts.Queue.DepthPercentage.Set(c.State, labels, contexts.QueueDepthPercentageValues{
			Percentage: int64(percentage * 1000),
		})
	}

	if total.HasMessages {
		contexts.Queue.Messages.Set(c.State, labels, contexts.QueueMessagesValues{
			Enqueued: total.MessagesEnqueued,
			Dequeued: total.MessagesDequeued,
		})
	}

	if total.HasConnections {
		contexts.Queue.Connections.Set(c.State, labels, contexts.QueueConnectionsValues{
			Input:  total.ConnectionsInput,
			Output: total.ConnectionsOutput,
		})
	}

	if total.HasUncommitted {
		contexts.Queue.UncommittedMessages.Set(c.State, labels, contexts.QueueUncommittedMessagesValues{
			Uncommitted: total.Uncommitted,
		})
	}

	if total.HasFileSize {
		contexts.Queue.FileSize.Set(c.State, labels, contexts.QueueFileSizeValues{
			Current: total.FileSizeCurrent,
			Max:     total.FileSizeMax,
		})
	}

	if total.HasOldest {
		contexts.Queue.OldestMessageAge.Set(c.State, labels, contexts.QueueOldestMessageAgeValues{
			Oldest_msg_age: total.OldestMessageAge,
		})
	}
}

func (c *Collector) emitQueueGroupMetrics(groups map[string]*queueGroupAggregate) {
	if len(groups) == 0 {
		return
	}

	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		agg := groups[key]
		labels := contexts.QueueGroupLabels{Group: key}

		contexts.QueueGroup.Depth.Set(c.State, labels, contexts.QueueGroupDepthValues{
			Current: agg.DepthCurrent,
			Max:     agg.DepthMax,
		})

		if agg.DepthMax > 0 {
			percentage := float64(agg.DepthCurrent) / float64(agg.DepthMax) * 100.0
			contexts.QueueGroup.DepthPercentage.Set(c.State, labels, contexts.QueueGroupDepthPercentageValues{
				Percentage: int64(percentage * 1000),
			})
		}

		if agg.HasMessages {
			contexts.QueueGroup.Messages.Set(c.State, labels, contexts.QueueGroupMessagesValues{
				Enqueued: agg.MessagesEnqueued,
				Dequeued: agg.MessagesDequeued,
			})
		}

		if agg.HasConnections {
			contexts.QueueGroup.Connections.Set(c.State, labels, contexts.QueueGroupConnectionsValues{
				Input:  agg.ConnectionsInput,
				Output: agg.ConnectionsOutput,
			})
		}

		if agg.HasUncommitted {
			contexts.QueueGroup.UncommittedMessages.Set(c.State, labels, contexts.QueueGroupUncommittedMessagesValues{
				Uncommitted: agg.Uncommitted,
			})
		}

		if agg.HasFileSize {
			contexts.QueueGroup.FileSize.Set(c.State, labels, contexts.QueueGroupFileSizeValues{
				Current: agg.FileSizeCurrent,
				Max:     agg.FileSizeMax,
			})
		}

		if agg.HasOldest {
			contexts.QueueGroup.OldestMessageAge.Set(c.State, labels, contexts.QueueGroupOldestMessageAgeValues{
				Oldest_msg_age: agg.OldestMessageAge,
			})
		}
	}
}
