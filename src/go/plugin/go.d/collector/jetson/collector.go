// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package jetson

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/streamexec"
)

//go:embed config_schema.json
var configSchema string

//go:embed charts.yaml
var chartTemplateYAML string

func init() {
	collectorapi.Register("jetson", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: defaultUpdateEvery,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	return &Collector{
		Config: Config{
			UpdateEvery: defaultUpdateEvery,
		},
		store:          store,
		metrics:        newCollectorMetrics(store),
		findTegrastats: lookupTegrastats,
		timing: streamexec.Timing{
			MaxSampleAge:    3 * tegrastatsInterval,
			StallTimeout:    10 * tegrastatsInterval,
			RestartDelayMin: time.Second,
			RestartDelayMax: 30 * time.Second,
		},
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store   metrix.CollectorStore
	metrics *collectorMetrics

	// findTegrastats resolves the executable in Check; tests inject a fake.
	findTegrastats func() (string, error)
	timing         streamexec.Timing

	// source supervises tegrastats once Check has resolved it; Run drives it.
	source *streamexec.Source[sample]
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error { return nil }

func (c *Collector) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.source = nil
	path, err := c.findTegrastats()
	if err != nil {
		return err
	}
	source, err := c.newTegrastatsSource(path)
	if err != nil {
		return err
	}
	c.source = source
	return nil
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

// Run supervises tegrastats: a startup failure is returned to the managed runtime and its retry policy; after
// readiness, source failures are recovered by restarting tegrastats.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if c.source == nil {
		return errors.New("tegrastats executable has not been resolved")
	}
	return c.source.Run(ctx, ready)
}

// Cleanup has nothing to release: Run owns tegrastats and returns before Cleanup is called.
func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
