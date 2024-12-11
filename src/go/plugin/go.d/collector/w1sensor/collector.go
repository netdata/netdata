// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package w1sensor

import (
	"context"
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("w1sensor", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 1,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			SensorsPath: "/sys/bus/w1/devices",
		},
		charts:      &module.Charts{},
		seenSensors: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	SensorsPath string `yaml:"sensors_path,omitempty" json:"sensors_path"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	seenSensors map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.SensorsPath == "" {
		return errors.New("config: no sensors path specified")
	}

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
