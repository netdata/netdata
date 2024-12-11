// SPDX-License-Identifier: GPL-3.0-or-later

package wireguard

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

	"golang.zx2c4.com/wireguard/wgctrl"
	"golang.zx2c4.com/wireguard/wgctrl/wgtypes"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("wireguard", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		newWGClient:  func() (wgClient, error) { return wgctrl.New() },
		charts:       &module.Charts{},
		devices:      make(map[string]bool),
		peers:        make(map[string]bool),
		cleanupEvery: time.Minute,
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client      wgClient
		newWGClient func() (wgClient, error)

		cleanupLastTime time.Time
		cleanupEvery    time.Duration
		devices         map[string]bool
		peers           map[string]bool
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
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Charts() *module.Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
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
