// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux || freebsd || openbsd || netbsd || dragonfly || darwin

package runit

import (
	"context"
	_ "embed"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("runit", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery:        collectorapi.UpdateEvery,
			AutoDetectionRetry: 60,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	return &Collector{
		Config: Config{
			Dir: defaultDir(),
		},
		store:   store,
		metrics: newCollectorMetrics(store),
	}
}

type (
	Config struct {
		UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
		Dir         string `yaml:"dir" json:"dir"`
	}

	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		store   metrix.CollectorStore
		metrics *collectorMetrics

		exec svCli
	}

	svCli interface {
		StatusAll(dir string) ([]byte, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		c.Errorf("config validation: %v", err)
		return err
	}

	err = c.initSvCli()
	if err != nil {
		c.Errorf("sv exec initialization: %v", err)
		return err
	}

	return nil
}

func (c *Collector) Check(context.Context) error {
	_, err := c.servicesCli()
	return err
}

func (c *Collector) Collect(context.Context) error { return c.collect() }

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
