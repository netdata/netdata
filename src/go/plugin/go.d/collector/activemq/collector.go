// SPDX-License-Identifier: GPL-3.0-or-later

package activemq

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("activemq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8161",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
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
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	Webadmin           string `yaml:"webadmin,omitempty" json:"webadmin"`
	MaxQueues          int    `yaml:"max_queues" json:"max_queues"`
	MaxTopics          int    `yaml:"max_topics" json:"max_topics"`
	QueuesFilter       string `yaml:"queues_filter,omitempty" json:"queues_filter"`
	TopicsFilter       string `yaml:"topics_filter,omitempty" json:"topics_filter"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	apiClient *apiClient

	activeQueues map[string]bool
	activeTopics map[string]bool
	queuesFilter matcher.Matcher
	topicsFilter matcher.Matcher
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	qf, err := c.initQueuesFiler()
	if err != nil {
		return fmt.Errorf("init queues filer: %v", err)
	}
	c.queuesFilter = qf

	tf, err := c.initTopicsFilter()
	if err != nil {
		return fmt.Errorf("init topics filter: %v", err)
	}
	c.topicsFilter = tf

	client, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return fmt.Errorf("create http client: %v", err)
	}

	c.apiClient = newAPIClient(client, c.RequestConfig, c.Webadmin)

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (c *Collector) Charts() *Charts {
	return c.charts
}

func (c *Collector) Cleanup(context.Context) {
	if c.apiClient != nil && c.apiClient.httpClient != nil {
		c.apiClient.httpClient.CloseIdleConnections()
	}
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()

	if err != nil {
		c.Error(err)
		return nil
	}

	return mx
}
