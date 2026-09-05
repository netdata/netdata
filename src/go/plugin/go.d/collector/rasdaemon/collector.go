// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package rasdaemon

import (
	"context"
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/ndexec"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplate string

func init() {
	collectorapi.Register("rasdaemon", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			// RAS events are rare and arrive asynchronously; polling faster buys nothing and
			// each cycle spawns a Perl process that scans the event database.
			UpdateEvery: 10,
		},
		CreateV2: func() collectorapi.CollectorV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 10),
		},
		store: metrix.NewCollectorStore(),
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store metrix.CollectorStore

	exec rasMcCtlCli
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(context.Context) error {
	if err := c.validate(); err != nil {
		return fmt.Errorf("config validation: %w", err)
	}
	return c.initRasMcCtl()
}

func (c *Collector) initRasMcCtl() error {
	binPath := c.BinaryPath
	if binPath == "" {
		found, err := ndexec.FindBinary(rasMcCtlBinaryNames, rasMcCtlFallbackPaths)
		if err != nil {
			return fmt.Errorf("locating ras-mc-ctl: %w", err)
		}
		binPath = found
	}
	c.exec = newRasMcCtlExec(binPath, c.Timeout.Duration(), c.Logger)
	return nil
}

// Check validates that ras-mc-ctl runs and produces parsable output.
//
// It MUST NOT write metrics: the framework calls Check during autodetection and DynCfg `test`,
// outside any active metrix cycle, where writing an instrument panics with
// "metrix: write outside active cycle".
func (c *Collector) Check(ctx context.Context) error {
	_, err := c.fetchSummary(ctx)
	return err
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplate }
