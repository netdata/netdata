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
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
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
			UpdateEvery:     defaultUpdateEvery,
			AllowPrivileged: true,
		},
		store:                 store,
		metrics:               newCollectorMetrics(store),
		findTegrastats:        lookupTegrastats,
		fixedTegrastatsExists: fixedTegrastatsExists,
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

	// Discovery checks candidates without executing them.
	findTegrastats        func() (string, error)
	fixedTegrastatsExists func() bool
	timing                streamexec.Timing

	// Init captures the owner and policy once; only Run selects a source mode.
	acquisition *ndexec.Acquisition
	mode        tegrastatsMode

	// source supervises tegrastats once Check has resolved it; Run drives it.
	source *streamexec.Source[sample]
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if c.acquisition == nil {
		c.acquisition = ndexec.NewAcquisition("jetson", c.Name)
		if !c.AllowPrivileged {
			c.mode = tegrastatsUnprivileged
		}
	}
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	c.source = nil
	path, lookupErr := c.findTegrastats()
	if lookupErr != nil && (c.mode == tegrastatsUnprivileged || !c.fixedTegrastatsExists()) {
		return lookupErr
	}
	source, err := c.newTegrastatsSource(path, lookupErr)
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

// Run requests shutdown; acquisition admission remains held while privileged cleanup is pending.
// Cleanup must not release that ownership or start another producer.
func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
