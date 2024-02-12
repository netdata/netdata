// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/matcher"
	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/prometheus/selector"
	"github.com/netdata/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("prometheus", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Prometheus {
	return &Prometheus{
		Config: Config{
			HTTP: web.HTTP{
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second * 10},
				},
			},
			MaxTS:          2000,
			MaxTSPerMetric: 200,
		},
		charts: &module.Charts{},
		cache:  newCache(),
	}
}

type Config struct {
	web.HTTP        `yaml:",inline"`
	Name            string `yaml:"name"`
	Application     string `yaml:"app"`
	BearerTokenFile string `yaml:"bearer_token_file"`

	Selector selector.Expr `yaml:"selector"`

	ExpectedPrefix string `yaml:"expected_prefix"`
	MaxTS          int    `yaml:"max_time_series"`
	MaxTSPerMetric int    `yaml:"max_time_series_per_metric"`
	FallbackType   struct {
		Counter []string `yaml:"counter"`
		Gauge   []string `yaml:"gauge"`
	} `yaml:"fallback_type"`
}

type Prometheus struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	prom  prometheus.Prometheus
	cache *cache

	fallbackType struct {
		counter matcher.Matcher
		gauge   matcher.Matcher
	}
}

func (p *Prometheus) Init() bool {
	if err := p.validateConfig(); err != nil {
		p.Errorf("validating config: %v", err)
		return false
	}

	prom, err := p.initPrometheusClient()
	if err != nil {
		p.Errorf("init prometheus client: %v", err)
		return false
	}
	p.prom = prom

	m, err := p.initFallbackTypeMatcher(p.FallbackType.Counter)
	if err != nil {
		p.Errorf("init counter fallback type matcher: %v", err)
		return false
	}
	p.fallbackType.counter = m

	m, err = p.initFallbackTypeMatcher(p.FallbackType.Gauge)
	if err != nil {
		p.Errorf("init counter fallback type matcher: %v", err)
		return false
	}
	p.fallbackType.gauge = m

	return true
}

func (p *Prometheus) Check() bool {
	return len(p.Collect()) > 0
}

func (p *Prometheus) Charts() *module.Charts {
	return p.charts
}

func (p *Prometheus) Collect() map[string]int64 {
	mx, err := p.collect()
	if err != nil {
		p.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (p *Prometheus) Cleanup() {}
