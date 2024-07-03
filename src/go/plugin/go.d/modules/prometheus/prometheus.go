// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
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
		Config: func() any { return &Config{} },
	})
}

func New() *Prometheus {
	return &Prometheus{
		Config: Config{
			HTTP: web.HTTP{
				Client: web.Client{
					Timeout: web.Duration(time.Second * 10),
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
	UpdateEvery     int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP        `yaml:",inline" json:""`
	Name            string        `yaml:"name,omitempty" json:"name"`
	Application     string        `yaml:"app,omitempty" json:"app"`
	BearerTokenFile string        `yaml:"bearer_token_file,omitempty" json:"bearer_token_file"`
	Selector        selector.Expr `yaml:"selector,omitempty" json:"selector"`
	ExpectedPrefix  string        `yaml:"expected_prefix,omitempty" json:"expected_prefix"`
	MaxTS           int           `yaml:"max_time_series" json:"max_time_series"`
	MaxTSPerMetric  int           `yaml:"max_time_series_per_metric" json:"max_time_series_per_metric"`
	FallbackType    struct {
		Gauge   []string `yaml:"gauge,omitempty" json:"gauge"`
		Counter []string `yaml:"counter,omitempty" json:"counter"`
	} `yaml:"fallback_type,omitempty" json:"fallback_type"`
}

type Prometheus struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prom prometheus.Prometheus

	cache        *cache
	fallbackType struct {
		counter matcher.Matcher
		gauge   matcher.Matcher
	}
}

func (p *Prometheus) Configuration() any {
	return p.Config
}

func (p *Prometheus) Init() error {
	if err := p.validateConfig(); err != nil {
		p.Errorf("validating config: %v", err)
		return err
	}

	prom, err := p.initPrometheusClient()
	if err != nil {
		p.Errorf("init prometheus client: %v", err)
		return err
	}
	p.prom = prom

	m, err := p.initFallbackTypeMatcher(p.FallbackType.Counter)
	if err != nil {
		p.Errorf("init counter fallback type matcher: %v", err)
		return err
	}
	p.fallbackType.counter = m

	m, err = p.initFallbackTypeMatcher(p.FallbackType.Gauge)
	if err != nil {
		p.Errorf("init counter fallback type matcher: %v", err)
		return err
	}
	p.fallbackType.gauge = m

	return nil
}

func (p *Prometheus) Check() error {
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

func (p *Prometheus) Cleanup() {
	if p.prom != nil && p.prom.HTTPClient() != nil {
		p.prom.HTTPClient().CloseIdleConnections()
	}
}
