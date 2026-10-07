// SPDX-License-Identifier: GPL-3.0-or-later

package cachestat

import (
	"context"
	_ "embed"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplate string

func init() {
	collectorapi.Register("cachestat", collectorapi.Creator{
		Defaults:        collectorapi.Defaults{UpdateEvery: 1},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		JobConfigSchema: configSchema,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
	})
}

type Config struct {
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store         metrix.CollectorStore
	metrics       instruments
	probe         func(context.Context) (string, error)
	open          func(context.Context, string) (nativeRuntime, error)
	accountTarget string
	runtime       nativeRuntime
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config:  Config{UpdateEvery: 1},
		store:   store,
		metrics: newInstruments(store),
		probe:   probeNative,
		open:    openNative,
	}
}

func (c *Collector) Configuration() any                 { return c.Config }
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return chartTemplate }

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.UpdateEvery < 1 {
		return fmt.Errorf("update_every must be at least 1")
	}
	return nil
}

// Check is also used by DynCfg test and overlapping replacement candidates.
// It resolves prerequisites without creating maps or attaching probes.
func (c *Collector) Check(ctx context.Context) error {
	target, err := c.probe(ctx)
	if err != nil {
		return fmt.Errorf("cachestat preflight: %w", err)
	}
	c.accountTarget = target
	return nil
}

func (c *Collector) Collect(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.accountTarget == "" {
		return fmt.Errorf("cachestat preflight has not succeeded")
	}
	if c.runtime == nil {
		rt, err := c.open(ctx, c.accountTarget)
		if err != nil {
			return fmt.Errorf("attach cachestat: %w", err)
		}
		c.runtime = rt
	}
	s, err := c.runtime.Snapshot()
	if err != nil {
		return fmt.Errorf("snapshot cachestat: %w", err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	c.metrics.observe(s)
	return nil
}

// Job runtime joins collection before cleanup, so the C handle has one owner.
func (c *Collector) Cleanup(context.Context) {
	if c.runtime != nil {
		c.runtime.Close()
		c.runtime = nil
	}
}
