// SPDX-License-Identifier: GPL-3.0-or-later

package smbios_memory

import (
	"context"
	_ "embed"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/terminal"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/internal/inventory"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/smbios_memory/smbiosfunc"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("smbios_memory", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		InstancePolicy:  collectorapi.InstancePolicySingle,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: smbiosMethods,
		MethodHandler:   smbiosFunctionHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	c := &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
		},
		store:      store,
		metrics:    newCollectorMetrics(store),
		now:        time.Now,
		isTerminal: terminal.IsTerminal,
	}
	c.funcRouter = smbiosfunc.NewRouter(functionDeps{snapshot: &c.snapshot})
	return c
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *collectorMetrics

	funcRouter funcapi.MethodHandler
	snapshot   atomic.Pointer[inventory.Snapshot]

	// readTable decodes the host's firmware table; tests inject a reader.
	readTable func() (*inventory.Table, error)
	now       func() time.Time
	// isTerminal reports a debug run from a terminal, which must not touch the Agent's baseline file.
	isTerminal func() bool

	baseline baselineStore
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	c.initTableReader()
	c.initBaselineStore()
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	// Preserve monitoring of a previously enrolled host during source failures,
	// including an invalid state file that needs operator attention.
	if c.baseline.exists() {
		return nil
	}
	_, err := c.readTable()
	return err
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) { c.snapshot.Store(nil) }

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
