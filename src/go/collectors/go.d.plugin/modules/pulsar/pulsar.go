// SPDX-License-Identifier: GPL-3.0-or-later

package pulsar

import (
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
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
	})
}

func New() *Pulsar {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "http://127.0.0.1:8080/metrics",
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second},
			},
		},
		TopicFiler: matcher.SimpleExpr{
			Includes: nil,
			Excludes: []string{"*"},
		},
	}
	return &Pulsar{
		Config:             config,
		once:               &sync.Once{},
		charts:             summaryCharts.Copy(),
		nsCharts:           namespaceCharts.Copy(),
		topicChartsMapping: topicChartsMapping(),
		cache:              newCache(),
		curCache:           newCache(),
	}
}

type (
	Config struct {
		web.HTTP   `yaml:",inline"`
		TopicFiler matcher.SimpleExpr `yaml:"topic_filter"`
	}

	Pulsar struct {
		module.Base
		Config `yaml:",inline"`

		prom               prometheus.Prometheus
		topicFilter        matcher.Matcher
		cache              *cache
		curCache           *cache
		once               *sync.Once
		charts             *Charts
		nsCharts           *Charts
		topicChartsMapping map[string]string
	}

	namespace struct{ name string }
	topic     struct{ namespace, name string }
	cache     struct {
		namespaces map[namespace]bool
		topics     map[topic]bool
	}
)

func newCache() *cache {
	return &cache{
		namespaces: make(map[namespace]bool),
		topics:     make(map[topic]bool),
	}
}

func (p Pulsar) validateConfig() error {
	if p.URL == "" {
		return errors.New("URL is not set")
	}
	return nil
}

func (p *Pulsar) initClient() error {
	client, err := web.NewHTTPClient(p.Client)
	if err != nil {
		return err
	}

	p.prom = prometheus.New(client, p.Request)
	return nil
}

func (p *Pulsar) initTopicFiler() error {
	if p.TopicFiler.Empty() {
		p.topicFilter = matcher.TRUE()
		return nil
	}

	m, err := p.TopicFiler.Parse()
	if err != nil {
		return err
	}
	p.topicFilter = m
	return nil
}

func (p *Pulsar) Init() bool {
	if err := p.validateConfig(); err != nil {
		p.Errorf("config validation: %v", err)
		return false
	}
	if err := p.initClient(); err != nil {
		p.Errorf("client initializing: %v", err)
		return false
	}
	if err := p.initTopicFiler(); err != nil {
		p.Errorf("topic filer initialization: %v", err)
		return false
	}
	return true
}

func (p *Pulsar) Check() bool {
	return len(p.Collect()) > 0
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

func (Pulsar) Cleanup() {}
