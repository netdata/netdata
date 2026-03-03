// SPDX-License-Identifier: GPL-3.0-or-later

package prometheus

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus/selector"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("prometheus", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
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
		charts: &collectorapi.Charts{},
		cache:  newCache(),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	Name               string          `yaml:"name,omitempty" json:"name"`
	Application        string          `yaml:"app,omitempty" json:"app"`
	LabelPrefix        string          `yaml:"label_prefix,omitempty" json:"label_prefix"`
	Selector           selector.Expr   `yaml:"selector,omitempty" json:"selector"`
	SelectorGroups     []SelectorGroup `yaml:"selector_groups,omitempty" json:"selector_groups"`
	ExpectedPrefix     string          `yaml:"expected_prefix,omitempty" json:"expected_prefix"`
	MaxTS              int             `yaml:"max_time_series" json:"max_time_series"`
	MaxTSPerMetric     int             `yaml:"max_time_series_per_metric" json:"max_time_series_per_metric"`
	FallbackType       struct {
		Gauge   []string `yaml:"gauge,omitempty" json:"gauge"`
		Counter []string `yaml:"counter,omitempty" json:"counter"`
	} `yaml:"fallback_type,omitempty" json:"fallback_type"`
	LabelRelabel   []RelabelRule   `yaml:"label_relabel,omitempty" json:"label_relabel"`
	ContextRules   []ContextRule   `yaml:"context_rules,omitempty" json:"context_rules"`
	DimensionRules []DimensionRule `yaml:"dimension_rules,omitempty" json:"dimension_rules"`
}

type RelabelRule struct {
	SourceLabels []string `yaml:"source_labels,omitempty" json:"source_labels"`
	Regex        string   `yaml:"regex,omitempty" json:"regex"`
	TargetLabel  string   `yaml:"target_label,omitempty" json:"target_label"`
	Replacement  string   `yaml:"replacement,omitempty" json:"replacement"`
	Action       string   `yaml:"action,omitempty" json:"action"`
}

type ContextRule struct {
	Match   string `yaml:"match" json:"match"`
	Context string `yaml:"context,omitempty" json:"context"`
	Title   string `yaml:"title,omitempty" json:"title"`
	Units   string `yaml:"units,omitempty" json:"units"`
	Type    string `yaml:"type,omitempty" json:"type"`
}

type DimensionRule struct {
	Match     string `yaml:"match" json:"match"`
	Dimension string `yaml:"dimension" json:"dimension"`
}

type SelectorGroup struct {
	Name           string          `yaml:"name" json:"name"`
	Selector       selector.Expr   `yaml:"selector,omitempty" json:"selector"`
	LabelRelabel   []RelabelRule   `yaml:"label_relabel,omitempty" json:"label_relabel"`
	ContextRules   []ContextRule   `yaml:"context_rules,omitempty" json:"context_rules"`
	DimensionRules []DimensionRule `yaml:"dimension_rules,omitempty" json:"dimension_rules"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *collectorapi.Charts

	prom prometheus.Prometheus

	cache        *cache
	fallbackType struct {
		counter matcher.Matcher
		gauge   matcher.Matcher
	}
	labelRelabelRules []compiledRelabelRule
	contextRules      []compiledContextRule
	dimensionRules    []compiledDimensionRule
	selectorGroups    []compiledSelectorGroup
	chartIDPrefix     string
	expectedPrefixValidated bool
	maxTSValidated          bool
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

	if err := c.initRules(); err != nil {
		return fmt.Errorf("init customization rules: %v", err)
	}

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

func (c *Collector) Charts() *collectorapi.Charts {
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
