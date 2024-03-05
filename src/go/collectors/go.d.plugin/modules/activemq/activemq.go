// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/matcher"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("activemq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *ActiveMQ {
	return &ActiveMQ{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8161",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
			Webadmin:  "admin",
			MaxQueues: 50,
			MaxTopics: 50,
		},
		charts:       &Charts{},
		activeQueues: make(map[string]bool),
		activeTopics: make(map[string]bool),
	}
}

type Config struct {
	web.HTTP     `yaml:",inline" json:""`
	UpdateEvery  int    `yaml:"update_every" json:"update_every"`
	Webadmin     string `yaml:"webadmin" json:"webadmin"`
	MaxQueues    int    `yaml:"max_queues" json:"max_queues"`
	MaxTopics    int    `yaml:"max_topics" json:"max_topics"`
	QueuesFilter string `yaml:"queues_filter" json:"queues_filter"`
	TopicsFilter string `yaml:"topics_filter" json:"topics_filter"`
}

type ActiveMQ struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	apiClient *apiClient

	activeQueues map[string]bool
	activeTopics map[string]bool
	queuesFilter matcher.Matcher
	topicsFilter matcher.Matcher
}

func (a *ActiveMQ) Configuration() any {
	return a.Config
}

func (a *ActiveMQ) Init() error {
	if err := a.validateConfig(); err != nil {
		a.Errorf("config validation: %v", err)
		return err
	}

	qf, err := a.initQueuesFiler()
	if err != nil {
		a.Error(err)
		return err
	}
	a.queuesFilter = qf

	tf, err := a.initTopicsFilter()
	if err != nil {
		a.Error(err)
		return err
	}
	a.topicsFilter = tf

	client, err := web.NewHTTPClient(a.Client)
	if err != nil {
		a.Error(err)
		return err
	}

	a.apiClient = newAPIClient(client, a.Request, a.Webadmin)

	return nil
}

func (a *ActiveMQ) Check() error {
	mx, err := a.collect()
	if err != nil {
		a.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (a *ActiveMQ) Charts() *Charts {
	return a.charts
}

func (a *ActiveMQ) Cleanup() {
	if a.apiClient != nil && a.apiClient.httpClient != nil {
		a.apiClient.httpClient.CloseIdleConnections()
	}
}

func (a *ActiveMQ) Collect() map[string]int64 {
	mx, err := a.collect()

	if err != nil {
		a.Error(err)
		return nil
	}

	return mx
}
