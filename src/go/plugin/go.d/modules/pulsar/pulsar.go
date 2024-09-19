// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	_ "embed"
	"errors"
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

func New() *Pulsar {
	return &Pulsar{
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
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	TopicFilter    matcher.SimpleExpr `yaml:"topic_filter,omitempty" json:"topic_filter"`
}

type Pulsar struct {
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

func (p *Pulsar) Configuration() any {
	return p.Config
}

func (p *Pulsar) Init() error {
	if err := p.validateConfig(); err != nil {
		p.Errorf("config validation: %v", err)
		return err
	}

	prom, err := p.initPrometheusClient()
	if err != nil {
		p.Error(err)
		return err
	}
	p.prom = prom

	m, err := p.initTopicFilerMatcher()
	if err != nil {
		p.Error(err)
		return err
	}
	p.topicFilter = m

	return nil
}

func (p *Pulsar) Check() error {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (p *Pulsar) Charts() *Charts {
	return p.charts
}

func (p *Pulsar) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *Pulsar) Cleanup() {
	if p.prom != nil && p.prom.HTTPClient() != nil {
		p.prom.HTTPClient().CloseIdleConnections()
	}
}
