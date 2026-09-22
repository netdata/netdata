// SPDX-License-Identifier: GPL-3.0-or-later

package statsd

import (
	"context"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/chartengine"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/profilecatalog"
)

// New returns a collector with provisional resource defaults. The collector is
// not registered yet, and no listener is assigned by default.
func New() *Collector {
	return &Collector{
		Config: Config{
			MaxSeries:         defaultMaxSeries,
			MetricIdleTimeout: confopt.Duration(defaultMetricIdleTimeout),
			MaxTCPConnections: defaultMaxTCPConnections,
		},
		store:       metrix.NewCollectorStore(),
		now:         time.Now,
		profileDirs: defaultProfileDirs(),
		maxRecord:   maxRecordSize,
	}
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	store       metrix.CollectorStore
	receiver    *receiver
	diagnostics *diagnostics
	profiles    []*profile

	// templates is the published native set: diagnostics plus the profiles
	// activated when it was built. Collect replaces it only when activation grows.
	templates *chartengine.TemplateSet
	published int // activated profile count captured in templates

	scratch []percentileBin                      // Collect-owned, shared by all percentile queries
	values  [len(valueFields)]metrix.SampleValue // Collect-owned; staging copies each point

	// Test seams.
	now         func() time.Time
	profileDirs []profilecatalog.DirSpec
	maxRecord   int
}

var (
	_ collectorapi.CollectorV2              = (*Collector)(nil)
	_ collectorapi.CollectorV2Runner        = (*Collector)(nil)
	_ collectorapi.ChartTemplateSetProvider = (*Collector)(nil)
)

var errNotInitialized = errors.New("collector is not initialized")

func (c *Collector) Configuration() any { return c.Config }

// Init validates configuration and prepares profiles, templates and receiver
// state. It never acquires sockets: configuration tests run it while an
// incumbent job owns the endpoints.
func (c *Collector) Init(context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	if err := c.initTemplates(); err != nil {
		return err
	}
	c.initReceiver()
	return nil
}

func (c *Collector) Check(context.Context) error {
	if c.receiver == nil {
		return errNotInitialized
	}
	return nil
}

// Run binds every configured listener, then serves until cancellation or
// permanent listener loss.
func (c *Collector) Run(ctx context.Context, ready func()) error { return c.run(ctx, ready) }

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {
	if c.receiver != nil {
		c.receiver.stop()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateSet() *chartengine.TemplateSet { return c.templates }
