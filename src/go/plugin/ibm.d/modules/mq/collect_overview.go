package mq

import (
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/mq/contexts"
)

// setQueueOverviewMetrics updates the queue overview chart with monitoring status
func (c *Collector) setQueueOverviewMetrics(monitored, excluded, invisible, failed int64) {
	contexts.QueueManager.QueuesOverview.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerQueuesOverviewValues{
		Monitored: monitored,
		Excluded:  excluded,
		Invisible: invisible,
		Failed:    failed,
	})
}

// setChannelOverviewMetrics updates the channel overview chart with monitoring status
func (c *Collector) setChannelOverviewMetrics(monitored, excluded, invisible, failed int64) {
	contexts.QueueManager.ChannelsOverview.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerChannelsOverviewValues{
		Monitored: monitored,
		Excluded:  excluded,
		Invisible: invisible,
		Failed:    failed,
	})
}

// setTopicOverviewMetrics updates the topic overview chart with monitoring status
func (c *Collector) setTopicOverviewMetrics(monitored, excluded, invisible, failed int64) {
	contexts.QueueManager.TopicsOverview.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerTopicsOverviewValues{
		Monitored: monitored,
		Excluded:  excluded,
		Invisible: invisible,
		Failed:    failed,
	})
}

// setListenerOverviewMetrics updates the listener overview chart with monitoring status
func (c *Collector) setListenerOverviewMetrics(monitored, excluded, invisible, failed int64) {
	contexts.QueueManager.ListenersOverview.Set(c.State, contexts.EmptyLabels{}, contexts.QueueManagerListenersOverviewValues{
		Monitored: monitored,
		Excluded:  excluded,
		Invisible: invisible,
		Failed:    failed,
	})
}
