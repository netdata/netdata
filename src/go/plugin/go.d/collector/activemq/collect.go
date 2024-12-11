// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	"fmt"
	"strings"
)

const (
	keyQueues   = "queues"
	keyTopics   = "topics"
	keyAdvisory = "Advisory"
)

var nameReplacer = strings.NewReplacer(".", "_", " ", "")

func (c *Collector) collect() (map[string]int64, error) {
	metrics := make(map[string]int64)

	var (
		queues *queues
		topics *topics
		err    error
	)

	if queues, err = c.apiClient.getQueues(); err != nil {
		return nil, err
	}

	if topics, err = c.apiClient.getTopics(); err != nil {
		return nil, err
	}

	c.processQueues(queues, metrics)
	c.processTopics(topics, metrics)

	return metrics, nil
}

func (c *Collector) processQueues(queues *queues, metrics map[string]int64) {
	var (
		count   = len(c.activeQueues)
		updated = make(map[string]bool)
		unp     int
	)

	for _, q := range queues.Items {
		if strings.Contains(q.Name, keyAdvisory) {
			continue
		}

		if !c.activeQueues[q.Name] {
			if c.MaxQueues != 0 && count > c.MaxQueues {
				unp++
				continue
			}

			if !c.filterQueues(q.Name) {
				continue
			}

			c.activeQueues[q.Name] = true
			c.addQueueTopicCharts(q.Name, keyQueues)
		}

		rname := nameReplacer.Replace(q.Name)

		metrics["queues_"+rname+"_consumers"] = q.Stats.ConsumerCount
		metrics["queues_"+rname+"_enqueued"] = q.Stats.EnqueueCount
		metrics["queues_"+rname+"_dequeued"] = q.Stats.DequeueCount
		metrics["queues_"+rname+"_unprocessed"] = q.Stats.EnqueueCount - q.Stats.DequeueCount

		updated[q.Name] = true
	}

	for name := range c.activeQueues {
		if !updated[name] {
			delete(c.activeQueues, name)
			c.removeQueueTopicCharts(name, keyQueues)
		}
	}

	if unp > 0 {
		c.Debugf("%d queues were unprocessed due to max_queues limit (%d)", unp, c.MaxQueues)
	}
}

func (c *Collector) processTopics(topics *topics, metrics map[string]int64) {
	var (
		count   = len(c.activeTopics)
		updated = make(map[string]bool)
		unp     int
	)

	for _, t := range topics.Items {
		if strings.Contains(t.Name, keyAdvisory) {
			continue
		}

		if !c.activeTopics[t.Name] {
			if c.MaxTopics != 0 && count > c.MaxTopics {
				unp++
				continue
			}

			if !c.filterTopics(t.Name) {
				continue
			}

			c.activeTopics[t.Name] = true
			c.addQueueTopicCharts(t.Name, keyTopics)
		}

		rname := nameReplacer.Replace(t.Name)

		metrics["topics_"+rname+"_consumers"] = t.Stats.ConsumerCount
		metrics["topics_"+rname+"_enqueued"] = t.Stats.EnqueueCount
		metrics["topics_"+rname+"_dequeued"] = t.Stats.DequeueCount
		metrics["topics_"+rname+"_unprocessed"] = t.Stats.EnqueueCount - t.Stats.DequeueCount

		updated[t.Name] = true
	}

	for name := range c.activeTopics {
		if !updated[name] {
			// TODO: delete after timeout?
			delete(c.activeTopics, name)
			c.removeQueueTopicCharts(name, keyTopics)
		}
	}

	if unp > 0 {
		c.Debugf("%d topics were unprocessed due to max_topics limit (%d)", unp, c.MaxTopics)
	}
}

func (c *Collector) filterQueues(line string) bool {
	if c.queuesFilter == nil {
		return true
	}
	return c.queuesFilter.MatchString(line)
}

func (c *Collector) filterTopics(line string) bool {
	if c.topicsFilter == nil {
		return true
	}
	return c.topicsFilter.MatchString(line)
}

func (c *Collector) addQueueTopicCharts(name, typ string) {
	rname := nameReplacer.Replace(name)

	charts := charts.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, typ, rname)
		chart.Title = fmt.Sprintf(chart.Title, name)
		chart.Fam = typ

		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, typ, rname)
		}
	}

	_ = c.charts.Add(*charts...)

}

func (c *Collector) removeQueueTopicCharts(name, typ string) {
	rname := nameReplacer.Replace(name)

	chart := c.charts.Get(fmt.Sprintf("%s_%s_messages", typ, rname))
	chart.MarkRemove()
	chart.MarkNotCreated()

	chart = c.charts.Get(fmt.Sprintf("%s_%s_unprocessed_messages", typ, rname))
	chart.MarkRemove()
	chart.MarkNotCreated()

	chart = c.charts.Get(fmt.Sprintf("%s_%s_consumers", typ, rname))
	chart.MarkRemove()
	chart.MarkNotCreated()
}
