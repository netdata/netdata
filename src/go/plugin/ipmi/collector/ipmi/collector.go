// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && (amd64 || arm64)

package ipmi

import (
	"context"
	_ "embed"
	"strings"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/funcapi"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/internal/ipmiapi"
	"github.com/netdata/netdata/go/plugins/plugin/ipmi/collector/ipmi/ipmifunc"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplateYAML string

const defaultUpdateEvery = 5

func init() {
	collectorapi.Register("ipmi", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults:        collectorapi.Defaults{UpdateEvery: defaultUpdateEvery},
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
		SharedFunctions: func() []funcapi.FunctionConfig { return ipmifunc.Methods(defaultUpdateEvery) },
		MethodHandler:   functionHandler,
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	c := &Collector{Config: defaultConfig(), store: store, metrics: newCollectorMetrics(store),
		newReader: func(cfg ipmiapi.Config) (reader, error) { return ipmiapi.New(cfg) }}
	c.funcRouter = ipmifunc.NewRouter(functionDeps{snapshot: &c.snapshot})
	return c
}

type reader interface {
	Check(context.Context) error
	Collect(context.Context, bool) (*ipmiapi.Snapshot, error)
	Close(context.Context) error
}

type Collector struct {
	collectorapi.Base
	Config      `yaml:",inline" json:""`
	store       metrix.CollectorStore
	metrics     *collectorMetrics
	device      reader
	newReader   func(ipmiapi.Config) (reader, error)
	snapshot    atomic.Pointer[ipmiapi.Snapshot]
	funcRouter  funcapi.MethodHandler
	lastWarning string
}

func (c *Collector) Configuration() any         { return c.Config }
func (c *Collector) Init(context.Context) error { return c.Config.validate() }
func (c *Collector) Check(ctx context.Context) error {
	if err := c.ensureReader(ctx); err != nil {
		return err
	}
	return c.device.Check(ctx)
}
func (c *Collector) Collect(ctx context.Context) error {
	if err := c.ensureReader(ctx); err != nil {
		c.snapshot.Store(nil)
		return err
	}
	snapshot, err := c.device.Collect(ctx, c.CollectSEL)
	if ctx.Err() != nil {
		err = ctx.Err()
	}
	if err != nil {
		c.snapshot.Store(nil)
		c.closeReader()
		return err
	}
	c.metrics.observe(snapshot)
	c.snapshot.Store(snapshot)
	warning := strings.Join(snapshot.Warnings, "; ")
	if warning != "" && warning != c.lastWarning {
		c.Warning(warning)
	}
	c.lastWarning = warning
	return nil
}
func (c *Collector) Cleanup(ctx context.Context) {
	c.funcRouter.Cleanup(ctx)
	c.snapshot.Store(nil)
	c.closeReader()
}
func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return chartTemplateYAML }

func (c *Collector) ensureReader(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if c.device != nil {
		return nil
	}
	dev, err := c.newReader(ipmiapi.Config{Driver: c.Driver, Device: c.Device,
		Hostname: c.Hostname, Port: c.Port, Username: c.Username, Password: c.Password,
		Timeout: time.Duration(c.Timeout)})
	if err != nil {
		return err
	}
	c.device = dev
	return nil
}
func (c *Collector) closeReader() {
	if c.device != nil {
		ctx, cancel := context.WithTimeout(context.Background(), time.Duration(c.Timeout))
		defer cancel()
		if err := c.device.Close(ctx); err != nil {
			c.Debugf("close IPMI connection: %v", err)
		}
		c.device = nil
	}
}
