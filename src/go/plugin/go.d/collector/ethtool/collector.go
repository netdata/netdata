// SPDX-License-Identifier: GPL-3.0-or-later

package ethtool

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ethtool", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:             &module.Charts{},
		seenOpticIfaces:    make(map[string]bool),
		ignoredOpticIfaces: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery     int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout         confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	OpticInterfaces string           `yaml:"optical_interfaces,omitempty" json:"optical_interfaces"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec ethtoolCli

	seenOpticIfaces    map[string]bool
	ignoredOpticIfaces map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %s", err)
	}

	et, err := c.initEthtoolCli()
	if err != nil {
		return fmt.Errorf("ethtool exec initialization: %v", err)
	}
	c.exec = et

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

func (c *Collector) Cleanup(context.Context) {}
