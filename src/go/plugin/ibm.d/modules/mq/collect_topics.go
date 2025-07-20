package mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
)

func (c *Collector) collectTopicMetrics() error {
	topics, err := c.client.GetTopicList()
	if err != nil {
		return err
	}

	// Track overview metrics
	var monitored, excluded, failed int64

	for _, topicName := range topics {
		metrics, err := c.client.GetTopicMetrics(topicName)
		if err != nil {
			c.Warningf("failed to get metrics for topic %s: %v", topicName, err)
			failed++
			continue
		}

		monitored++

		labels := contexts.TopicLabels{
			Topic: topicName,
		}

		// Use structured data from protocol
		contexts.Topic.Publishers.Set(c.State, labels, contexts.TopicPublishersValues{
			Publishers: metrics.Publishers,
		})
		contexts.Topic.Subscribers.Set(c.State, labels, contexts.TopicSubscribersValues{
			Subscribers: metrics.Subscribers,
		})
		contexts.Topic.Messages.Set(c.State, labels, contexts.TopicMessagesValues{
			Messages: metrics.PublishMsgCount,
		})
	}
	
	// Set overview metrics
	c.setTopicOverviewMetrics(monitored, excluded, 0, failed)
	
	return nil
}