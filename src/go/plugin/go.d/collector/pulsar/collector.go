// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pulsar", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 60,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8080/metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 5),
				},
			},
			TopicFilter: matcher.SimpleExpr{
				Includes: nil,
				Excludes: []string{"*"},
			},
		},
		once:               &sync.Once{},
		charts:             summaryCharts.Copy(),
		nsCharts:           namespaceCharts.Copy(),
		topicChartsMapping: topicChartsMapping(),
		cache:              newCache(),
		curCache:           newCache(),
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	TopicFilter    matcher.SimpleExpr `yaml:"topic_filter,omitempty" json:"topic_filter"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts   *Charts
	nsCharts *Charts

	prom prometheus.Prometheus

	topicFilter        matcher.Matcher
	cache              *cache
	curCache           *cache
	once               *sync.Once
	topicChartsMapping map[string]string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	c.prom = prom

	m, err := c.initTopicFilerMatcher()
	if err != nil {
		return fmt.Errorf("init topic filer: %v", err)
	}
	c.topicFilter = m

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

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
