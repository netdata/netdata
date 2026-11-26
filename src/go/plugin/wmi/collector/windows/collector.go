// SPDX-License-Identifier: GPL-3.0-or-later

// Package windows collects host metrics from a remote Windows host over WMI.
package windows

import (
	"context"
	"crypto/tls"
	_ "embed"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/framework/dyncfg"
	"github.com/netdata/netdata/go/plugins/plugin/framework/vnodes"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/pinger"
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

// querySession is one collection cycle's WMI session, over either transport.
type querySession interface {
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
	lastCPU       cpuSample

	winrmTLS   *tls.Config
	newSession func(context.Context) (querySession, error)

	// pingClient is nil once the poller turned out to lack the privilege to ping.
	pingClient pinger.Client
	newPinger  func(pinger.Config, *logger.Logger) (pinger.Client, error)
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	c := &Collector{
		Config:    defaultConfig(),
		store:     store,
		metrics:   newCollectorMetrics(store),
		newPinger: pinger.New,
	}
	c.newSession = c.openSession
	return c
}

func (c *Collector) openSession(ctx context.Context) (querySession, error) {
	if c.Transport == transportWinRM {
		return wmiquery.OpenWinRM(c.Host, c.Username, c.Password, c.winrmTLS), nil
	}
	session, err := wmiquery.OpenDCOM(ctx, c.Host, c.Username, c.Password)
	if err != nil {
		return nil, err
	}
	return session, nil
}

func (c *Collector) Configuration() any { return c.Config }

func (c *Collector) Init(ctx context.Context) error {
	if err := c.Config.validate(); err != nil {
		return err
	}
	if c.Transport == transportWinRM {
		cfg, err := tlscfg.NewTLSConfig(ctx, tlscfg.TLSConfig{
			TLSCA: c.TLSCA,
		})
		if err != nil {
			// The loader's message names the file, so only the wrapping text carries it.
			return fmt.Errorf("%w: %v", dyncfg.NewPublicError("could not load tls_ca", err), err)
		}
		c.winrmTLS = cfg
	}
	if err := c.initPinger(); err != nil {
		return err
	}
	c.vnode = newVirtualNode(c.Host)
	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	// Readiness depends only on the enabled metric groups.
	_, err := c.poll(ctx, false)
	return err
}

// Collect pings the host alongside the WMI poll. A failed poll fails the
// collection, so the job never publishes ping alone.
func (c *Collector) Collect(ctx context.Context) error {
	inventoryDue := c.Vnode == "" && !time.Now().Before(c.nextInventory)
	waitPing := c.startPing(ctx)
	snapshot, err := c.poll(ctx, inventoryDue)
	ping, pinged := waitPing(err != nil)
	if err != nil {
		return err
	}
	c.metrics.write(snapshot)
	if pinged {
		c.metrics.writePing(c.Host, ping)
	}
	return nil
}

// Cleanup has nothing to release: every poll closes its own session.
func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) VirtualNode() *vnodes.VirtualNode { return c.vnode }

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
