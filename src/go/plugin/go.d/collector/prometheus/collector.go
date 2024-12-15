// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
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

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 10),
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
	Vnode           string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery     int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig  `yaml:",inline" json:""`
	Name            string        `yaml:"name,omitempty" json:"name"`
	Application     string        `yaml:"app,omitempty" json:"app"`
	LabelPrefix     string        `yaml:"label_prefix,omitempty" json:"label_prefix"`
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

type Collector struct {
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

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("validating config: %v", err)
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	c.prom = prom

	m, err := c.initFallbackTypeMatcher(c.FallbackType.Counter)
	if err != nil {
		return fmt.Errorf("init counter fallback type matcher: %v", err)
	}
	c.fallbackType.counter = m

	m, err = c.initFallbackTypeMatcher(c.FallbackType.Gauge)
	if err != nil {
		return fmt.Errorf("init counter fallback type matcher: %v", err)
	}
	c.fallbackType.gauge = m

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

func (c *Collector) Charts() *module.Charts {
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
