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

func (a *ActiveMQ) collect() (map[string]int64, error) {
	metrics := make(map[string]int64)

	var (
		queues *queues
		topics *topics
		err    error
	)

	if queues, err = a.apiClient.getQueues(); err != nil {
		return nil, err
	}

	if topics, err = a.apiClient.getTopics(); err != nil {
		return nil, err
	}

	a.processQueues(queues, metrics)
	a.processTopics(topics, metrics)

	return metrics, nil
}

func (a *ActiveMQ) processQueues(queues *queues, metrics map[string]int64) {
	var (
		count   = len(a.activeQueues)
		updated = make(map[string]bool)
		unp     int
	)

	for _, q := range queues.Items {
		if strings.Contains(q.Name, keyAdvisory) {
			continue
		}

		if !a.activeQueues[q.Name] {
			if a.MaxQueues != 0 && count > a.MaxQueues {
				unp++
				continue
			}

			if !a.filterQueues(q.Name) {
				continue
			}

			a.activeQueues[q.Name] = true
			a.addQueueTopicCharts(q.Name, keyQueues)
		}

		rname := nameReplacer.Replace(q.Name)

		metrics["queues_"+rname+"_consumers"] = q.Stats.ConsumerCount
		metrics["queues_"+rname+"_enqueued"] = q.Stats.EnqueueCount
		metrics["queues_"+rname+"_dequeued"] = q.Stats.DequeueCount
		metrics["queues_"+rname+"_unprocessed"] = q.Stats.EnqueueCount - q.Stats.DequeueCount

		updated[q.Name] = true
	}

	for name := range a.activeQueues {
		if !updated[name] {
			delete(a.activeQueues, name)
			a.removeQueueTopicCharts(name, keyQueues)
		}
	}

	if unp > 0 {
		a.Debugf("%d queues were unprocessed due to max_queues limit (%d)", unp, a.MaxQueues)
	}
}

func (a *ActiveMQ) processTopics(topics *topics, metrics map[string]int64) {
	var (
		count   = len(a.activeTopics)
		updated = make(map[string]bool)
		unp     int
	)

	for _, t := range topics.Items {
		if strings.Contains(t.Name, keyAdvisory) {
			continue
		}

		if !a.activeTopics[t.Name] {
			if a.MaxTopics != 0 && count > a.MaxTopics {
				unp++
				continue
			}

			if !a.filterTopics(t.Name) {
				continue
			}

			a.activeTopics[t.Name] = true
			a.addQueueTopicCharts(t.Name, keyTopics)
		}

		rname := nameReplacer.Replace(t.Name)

		metrics["topics_"+rname+"_consumers"] = t.Stats.ConsumerCount
		metrics["topics_"+rname+"_enqueued"] = t.Stats.EnqueueCount
		metrics["topics_"+rname+"_dequeued"] = t.Stats.DequeueCount
		metrics["topics_"+rname+"_unprocessed"] = t.Stats.EnqueueCount - t.Stats.DequeueCount

		updated[t.Name] = true
	}

	for name := range a.activeTopics {
		if !updated[name] {
			// TODO: delete after timeout?
			delete(a.activeTopics, name)
			a.removeQueueTopicCharts(name, keyTopics)
		}
	}

	if unp > 0 {
		a.Debugf("%d topics were unprocessed due to max_topics limit (%d)", unp, a.MaxTopics)
	}
}

func (a *ActiveMQ) filterQueues(line string) bool {
	if a.queuesFilter == nil {
		return true
	}
	return a.queuesFilter.MatchString(line)
}

func (a *ActiveMQ) filterTopics(line string) bool {
	if a.topicsFilter == nil {
		return true
	}
	return a.topicsFilter.MatchString(line)
}

func (a *ActiveMQ) addQueueTopicCharts(name, typ string) {
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

	_ = a.charts.Add(*charts...)

}

func (a *ActiveMQ) removeQueueTopicCharts(name, typ string) {
	rname := nameReplacer.Replace(name)

	chart := a.charts.Get(fmt.Sprintf("%s_%s_messages", typ, rname))
	chart.MarkRemove()
	chart.MarkNotCreated()

	chart = a.charts.Get(fmt.Sprintf("%s_%s_unprocessed_messages", typ, rname))
	chart.MarkRemove()
	chart.MarkNotCreated()

	chart = a.charts.Get(fmt.Sprintf("%s_%s_consumers", typ, rname))
	chart.MarkRemove()
	chart.MarkNotCreated()
}
