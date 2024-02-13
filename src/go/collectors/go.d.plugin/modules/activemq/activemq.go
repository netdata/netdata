// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("activemq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

const (
	keyQueues   = "queues"
	keyTopics   = "topics"
	keyAdvisory = "Advisory"
)

var nameReplacer = strings.NewReplacer(".", "_", " ", "")

const (
	defaultMaxQueues   = 50
	defaultMaxTopics   = 50
	defaultURL         = "http://127.0.0.1:8161"
	defaultHTTPTimeout = time.Second
)

// New creates Example with default values.
func New() *ActiveMQ {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},

		MaxQueues: defaultMaxQueues,
		MaxTopics: defaultMaxTopics,
	}

	return &ActiveMQ{
		Config:       config,
		charts:       &Charts{},
		activeQueues: make(map[string]bool),
		activeTopics: make(map[string]bool),
	}
}

// Config is the ActiveMQ module configuration.
type Config struct {
	web.HTTP     `yaml:",inline"`
	Webadmin     string `yaml:"webadmin"`
	MaxQueues    int    `yaml:"max_queues"`
	MaxTopics    int    `yaml:"max_topics"`
	QueuesFilter string `yaml:"queues_filter"`
	TopicsFilter string `yaml:"topics_filter"`
}

// ActiveMQ ActiveMQ module.
type ActiveMQ struct {
	module.Base
	Config `yaml:",inline"`

	apiClient    *apiClient
	activeQueues map[string]bool
	activeTopics map[string]bool
	queuesFilter matcher.Matcher
	topicsFilter matcher.Matcher
	charts       *Charts
}

// Cleanup makes cleanup.
func (ActiveMQ) Cleanup() {}

// Init makes initialization.
func (a *ActiveMQ) Init() bool {
	if a.URL == "" {
		a.Error("URL not set")
		return false
	}

	if a.Webadmin == "" {
		a.Error("webadmin root path is not set")
		return false
	}

	if a.QueuesFilter != "" {
		f, err := matcher.NewSimplePatternsMatcher(a.QueuesFilter)
		if err != nil {
			a.Errorf("error on creating queues filter : %v", err)
			return false
		}
		a.queuesFilter = matcher.WithCache(f)
	}

	if a.TopicsFilter != "" {
		f, err := matcher.NewSimplePatternsMatcher(a.TopicsFilter)
		if err != nil {
			a.Errorf("error on creating topics filter : %v", err)
			return false
		}
		a.topicsFilter = matcher.WithCache(f)
	}

	client, err := web.NewHTTPClient(a.Client)
	if err != nil {
		a.Error(err)
		return false
	}

	a.apiClient = newAPIClient(client, a.Request, a.Webadmin)

	return true
}

// Check makes check.
func (a *ActiveMQ) Check() bool {
	return len(a.Collect()) > 0
}

// Charts creates Charts.
func (a ActiveMQ) Charts() *Charts {
	return a.charts
}

// Collect collects metrics.
func (a *ActiveMQ) Collect() map[string]int64 {
	metrics := make(map[string]int64)

	var (
		queues *queues
		topics *topics
		err    error
	)

	if queues, err = a.apiClient.getQueues(); err != nil {
		a.Error(err)
		return nil
	}

	if topics, err = a.apiClient.getTopics(); err != nil {
		a.Error(err)
		return nil
	}

	a.processQueues(queues, metrics)
	a.processTopics(topics, metrics)

	return metrics
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

func (a ActiveMQ) filterQueues(line string) bool {
	if a.queuesFilter == nil {
		return true
	}
	return a.queuesFilter.MatchString(line)
}

func (a ActiveMQ) filterTopics(line string) bool {
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
