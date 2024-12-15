// SPDX-License-Identifier: GPL-3.0-or-later

package freeradius

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/freeradius/api"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("freeradius", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address: "127.0.0.1",
			Port:    18121,
			Secret:  "adminsecret",
			Timeout: confopt.Duration(time.Second),
		},
	}
}

type Config struct {
	Vnode       string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery int              `yaml:"update_every" json:"update_every"`
	Address     string           `yaml:"address" json:"address"`
	Port        int              `yaml:"port" json:"port"`
	Secret      string           `yaml:"secret" json:"secret"`
	Timeout     confopt.Duration `yaml:"timeout" json:"timeout"`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		client
	}
	client interface {
		Status() (*api.Status, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	c.client = api.New(api.Config{
		Address: c.Address,
		Port:    c.Port,
		Secret:  c.Secret,
		Timeout: c.Timeout.Duration(),
	})

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

func (c *Collector) Charts() *Charts {
	return charts.Copy()
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

func (c *Collector) Cleanup(context.Context) {}
