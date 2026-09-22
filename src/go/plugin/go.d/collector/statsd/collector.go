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

const (
	defaultChartExpiry = 5
	// maxRecordSize is the single framer-owned bound on one record payload,
	// excluding its LF/CRLF terminator. It exceeds the largest UDP payload.
	maxRecordSize = 64 << 10
)

// The package is deliberately unregistered until the operator delivery stage.
// Resource values are provisional, and no listener is assigned by default.
func New() *Collector {
	return &Collector{
		Config: Config{
			MaxSeries:         1000,
			MetricIdleTimeout: confopt.Duration(5 * time.Minute),
			MaxTCPConnections: 64,
		},
		store:       metrix.NewCollectorStore(),
		now:         time.Now,
		profileDirs: defaultProfileDirs(),
		maxRecord:   maxRecordSize,
	}
}

type Collector struct {
	collectorapi.Base
	Config      `yaml:",inline" json:""`
	store       metrix.CollectorStore
	receiver    *receiver
	diagnostics *diagnostics
	profiles    []*profile
	diagEntry   chartengine.TemplateEntry
	templates   *chartengine.TemplateSet
	built       int // active profile count published in templates
	now         func() time.Time
	scratch     []percentileBin // Collect-owned, shared across all interval queries.
	profileDirs []profilecatalog.DirSpec
	maxRecord   int
}

var _ collectorapi.CollectorV2 = (*Collector)(nil)
var _ collectorapi.CollectorV2Runner = (*Collector)(nil)
var _ collectorapi.ChartTemplateSetProvider = (*Collector)(nil)

func (c *Collector) Configuration() any { return c.Config }

// Init validates configuration and prepares profiles and templates. It reads
// profile files but never acquires sockets: configuration tests run it while
// an incumbent job owns the endpoints.
func (c *Collector) Init(context.Context) error {
	if c.MaxSeries <= 0 {
		return errors.New("max_series must be positive")
	}
	if c.MetricIdleTimeout < 0 {
		return errors.New("metric_idle_timeout must be nonnegative")
	}
	if c.MaxTCPConnections <= 0 {
		return errors.New("max_tcp_connections must be positive")
	}
	if err := validateListeners(c.Listeners); err != nil {
		return err
	}
	var err error
	if c.diagEntry, err = diagnosticsEntry(); err != nil {
		return err
	}
	if c.profiles, err = loadProfiles(c.profileDirs, c.Profiles, enginePolicy(), c.Logger); err != nil {
		return err
	}
	if c.templates, err = c.templateSet(nil); err != nil {
		return err
	}
	lifetime := uint64(defaultChartExpiry)
	for _, p := range c.profiles {
		lifetime = max(lifetime, p.lifetime)
	}
	c.diagnostics = newDiagnostics(c.store, c.Config)
	c.receiver = newReceiver(
		c.MaxSeries,
		time.Duration(c.MetricIdleTimeout),
		c.store.(metrix.DescriptorRetention),
		lifetime,
		c.profiles,
	)
	return nil
}

// enginePolicy is fixed for a running job; every snapshot uses the same value.
func enginePolicy() chartengine.EnginePolicy {
	return chartengine.EnginePolicy{
		Autogen: &chartengine.AutogenPolicy{
			Enabled:                  true,
			ExpireAfterSuccessCycles: defaultChartExpiry,
		},
	}
}

// templateSet composes the fixed diagnostics entry with active profiles.
func (c *Collector) templateSet(active []bool) (*chartengine.TemplateSet, error) {
	entries := append([]chartengine.TemplateEntry{c.diagEntry}, profileEntries(c.profiles, active)...)
	return chartengine.NewTemplateSet(chartengine.TemplateSetSpec{
		Entries:                  entries,
		Policy:                   enginePolicy(),
		FallbackContextNamespace: contextNamespace,
	})
}

func (c *Collector) Check(context.Context) error {
	if c.receiver == nil {
		return errors.New("collector is not initialized")
	}
	return nil
}

// Run binds every configured listener, then serves until cancellation or
// permanent listener loss.
func (c *Collector) Run(ctx context.Context, ready func()) error {
	if c.receiver == nil {
		return errors.New("collector is not initialized")
	}
	if ctx.Err() != nil {
		return nil
	}
	s, err := c.listen(ctx)
	if err != nil {
		return err
	}
	return c.serve(ctx, s, ready)
}

// serve owns an acquired server. Readiness follows successful acquisition, not
// traffic. On cancellation or listener loss, admission stops before sockets
// close, so a failed receiver cannot publish held or empty intervals.
func (c *Collector) serve(ctx context.Context, s *server, ready func()) error {
	c.receiver.start()
	s.start()
	ready()
	select {
	case <-ctx.Done():
	case <-s.failed:
	}
	c.receiver.stop()
	s.close()
	return s.err
}

func (c *Collector) Collect(ctx context.Context) error { return c.collect(ctx) }

func (c *Collector) Cleanup(context.Context) {
	if c.receiver != nil {
		c.receiver.stop()
	}
}

func (c *Collector) MetricStore() metrix.CollectorStore         { return c.store }
func (c *Collector) ChartTemplateSet() *chartengine.TemplateSet { return c.templates }
