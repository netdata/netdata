package mq

import (
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
)

func (c *Collector) collectTopicMetrics() error {
	c.Debugf("Collecting topics with selector '%s', system: %v", c.Config.TopicSelector, c.Config.CollectSystemTopics)

	// Use new GetTopics with transparency
	result, err := c.client.GetTopics(
		true,                         // collectMetrics (always)
		c.Config.MaxTopics,           // maxTopics (0 = no limit)
		c.Config.TopicSelector,       // selector pattern
		c.Config.CollectSystemTopics, // collectSystem
	)
	if err != nil {
		return fmt.Errorf("failed to collect topic metrics: %w", err)
	}

	// Check discovery success
	if !result.Stats.Discovery.Success {
		c.Errorf("Topic discovery failed completely")
		return fmt.Errorf("topic discovery failed")
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

	// Update overview metrics with correct semantics
	c.setTopicOverviewMetrics(
		monitored,                             // monitored (successfully enriched)
		result.Stats.Discovery.ExcludedItems,  // excluded (filtered by user)
		result.Stats.Discovery.InvisibleItems, // invisible (discovery errors)
		failed,                                // failed (unparsed + enrichment failures)
	)

	// Log collection summary
	c.Debugf("Topic collection complete - discovered:%d visible:%d included:%d collected:%d failed:%d",
		result.Stats.Discovery.AvailableItems,
		result.Stats.Discovery.AvailableItems-result.Stats.Discovery.InvisibleItems,
		result.Stats.Discovery.IncludedItems,
		len(result.Topics),
		failed)

	// Process collected topic metrics
	for _, topic := range result.Topics {
		labels := contexts.TopicLabels{
			Topic: topic.TopicString,
		}

		// Use structured data from protocol
		contexts.Topic.Publishers.Set(c.State, labels, contexts.TopicPublishersValues{
			Publishers: topic.Publishers,
		})
		contexts.Topic.Subscribers.Set(c.State, labels, contexts.TopicSubscribersValues{
			Subscribers: topic.Subscribers,
		})
		contexts.Topic.Messages.Set(c.State, labels, contexts.TopicMessagesValues{
			Messages: topic.PublishMsgCount,
		})

		// Calculate time since last message if timestamp is available
		if topic.LastPubTime > 0 {
			currentTime := time.Now().Unix()
			timeSinceLastMsg := currentTime - int64(topic.LastPubTime)

			contexts.Topic.TimeSinceLastMessage.Set(c.State, labels, contexts.TopicTimeSinceLastMessageValues{
				Time_since_last_msg: timeSinceLastMsg,
			})
		}
	}

	return nil
}
