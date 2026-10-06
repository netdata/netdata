// SPDX-License-Identifier: GPL-3.0-or-later

package jetson

import (
	"context"
	_ "embed"
	"errors"
	"os/exec"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplateYAML string

func init() {
	collectorapi.Register("jetson", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 1,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			UpdateEvery: 1,
		},
		store:      store,
		metrics:    newCollectorMetrics(store),
		findBinary: findTegrastats,
		timing: sourceTiming{
			freshFor:   3 * time.Second,
			stallAfter: 10 * time.Second,
			retryMin:   time.Second,
			retryMax:   30 * time.Second,
		},
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store      metrix.CollectorStore
	metrics    *collectorMetrics
	findBinary func() (string, error)
	binary     string
	timing     sourceTiming
	latest     atomic.Pointer[observation]
	runMu      sync.Mutex
	cancel     context.CancelFunc
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	return c.Config.validate()
}

func (c *Collector) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	binary, err := c.findBinary()
	if err != nil {
		return err
	}
	c.binary = binary
	return nil
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Run(ctx context.Context, ready func()) error { return c.run(ctx, ready) }

func (c *Collector) Cleanup(context.Context) {
	c.runMu.Lock()
	defer c.runMu.Unlock()
	if c.cancel != nil {
		c.cancel()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }
func (c *Collector) ChartTemplateYAML() string          { return chartTemplateYAML }

func findTegrastats() (string, error) {
	if runtime.GOOS != "linux" {
		return "", errors.New("jetson requires Linux")
	}
	path, err := exec.LookPath("tegrastats")
	if err != nil {
		return "", errors.New("tegrastats executable not found in PATH")
	}
	return path, nil
}
