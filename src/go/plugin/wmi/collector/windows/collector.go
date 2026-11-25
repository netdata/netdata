// SPDX-License-Identifier: GPL-3.0-or-later

// Package windows collects host metrics over remote Windows Management Instrumentation.
package windows

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/wmi/collector/windows/internal/wmiquery"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplateYAML string

func init() {
	collectorapi.Register("windows", collectorapi.Creator{
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		JobConfigSchema: configSchema,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
	})
}

type queryClient interface {
	Query(ctx context.Context, wql string) ([]map[string]any, error)
	Close()
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store         metrix.CollectorStore
	metrics       collectorMetrics
	vnode         *vnodes.VirtualNode
	nextInventory time.Time

	open func(ctx context.Context, host, username, password, auth string) (queryClient, error)
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config:  defaultConfig(),
		store:   store,
		metrics: newCollectorMetrics(store),
		open: func(ctx context.Context, host, username, password, auth string) (queryClient, error) {
			return wmiquery.Open(ctx, host, username, password, auth)
		},
	}
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	c.vnode = newVirtualNode(c.Host)
	c.nextInventory = time.Time{}
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	_, err := c.poll(ctx)
	return err
}

func (c *Collector) Collect(ctx context.Context) error {
	snapshot, err := c.poll(ctx)
	if err != nil {
		return err
	}
	c.metrics.write(snapshot)
	return ctx.Err()
}

// Each poll closes its own client.
func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) VirtualNode() *vnodes.VirtualNode { return c.vnode }

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
