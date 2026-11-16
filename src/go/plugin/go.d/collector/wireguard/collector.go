// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var chartTemplateYAML string

func init() {
	collectorapi.Register("wireguard", collectorapi.Creator{
		JobConfigSchema: configSchema,
		CreateV2:        func() collectorapi.CollectorV2 { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		newWGClient: func() (wgClient, error) { return wgctrl.New() },
		now:         time.Now,
		store:       metrix.NewCollectorStore(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}

type (
	Collector struct {
		collectorapi.Base
		Config `yaml:",inline" json:""`

		client      wgClient
		newWGClient func() (wgClient, error)
		now         func() time.Time

		store metrix.CollectorStore
	}
	wgClient interface {
		Devices() ([]*wgtypes.Device, error)
		Close() error
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	return nil
}

func (c *Collector) Check(context.Context) error {
	devices, err := c.devices()
	if err != nil {
		return err
	}
	if len(devices) == 0 {
		return errors.New("no WireGuard devices found")
	}
	return nil
}

func (c *Collector) Collect(context.Context) error {
	return c.collect()
}

func (c *Collector) Cleanup(context.Context) {
	if c.client == nil {
		return
	}
	if err := c.client.Close(); err != nil {
		c.Warningf("cleanup: error on closing connection: %v", err)
	}
	c.client = nil
}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return chartTemplateYAML }
