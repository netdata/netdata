// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package sensors

import (
	"context"
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/sensors/lmsensors"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("sensors", module.Creator{
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
		Config:      Config{},
		charts:      &module.Charts{},
		seenSensors: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	Relabel     []struct {
		Chip    string `yaml:"chip" json:"chip"`
		Sensors []struct {
			Name  string `yaml:"name" json:"name"`
			Label string `yaml:"label" json:"label"`
		} `yaml:"sensors,omitempty" json:"sensors"`
	} `yaml:"relabel,omitempty" json:"relabel"`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		sc sysfsScanner

		seenSensors map[string]bool
	}
	sysfsScanner interface {
		Scan() ([]*lmsensors.Chip, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	sc := lmsensors.New()
	sc.Logger = c.Logger
	c.sc = sc

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
