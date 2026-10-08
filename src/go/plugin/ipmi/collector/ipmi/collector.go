// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"context"
	_ "embed"
	"sync/atomic"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/bmc"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/ipmifunc"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("ipmi", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: ipmiMethods,
		MethodHandler:   ipmiFunctionHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()

	c := &Collector{
		Config:    defaultConfig(),
		store:     store,
		metrics:   newCollectorMetrics(store),
		newReader: func(cfg bmc.Config) sensorReader { return bmc.New(cfg) },
	}
	c.funcRouter = ipmifunc.NewRouter(functionDeps{
		snapshot: &c.snapshot,
	})
	return c
}

// sensorReader is the BMC access the collector needs. Check leaves the device
// closed; Collect opens it on demand and Close releases it.
type sensorReader interface {
	Check(ctx context.Context) error
	Collect(ctx context.Context, collectSEL bool) (*bmc.Snapshot, error)
	Close(ctx context.Context) error
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *collectorMetrics

	newReader func(bmc.Config) sensorReader
	reader    sensorReader

	funcRouter funcapi.MethodHandler
	// snapshot is the latest successful collection; nil before one and after a failure.
	snapshot atomic.Pointer[bmc.Snapshot]
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	c.reader = c.newReader(bmc.Config{
		Device:  c.Device,
		Timeout: c.Timeout.Duration(),
	})
	return nil
}

func (c *Collector) Check(ctx context.Context) error { return c.reader.Check(ctx) }

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(ctx context.Context) {
	c.funcRouter.Cleanup(ctx)
	c.snapshot.Store(nil)
	if c.reader == nil {
		return
	}
	if err := c.reader.Close(ctx); err != nil {
		c.Debugf("close IPMI device: %v", err)
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
