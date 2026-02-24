// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package sensors

import (
	"context"
	_ "embed"
	"errors"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/sensors/lmsensors"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("sensors", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			UpdateEvery: 10,
			Disabled:    true,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config:      Config{},
		charts:      &collectorapi.Charts{},
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
		collectorapi.Base
		Config `yaml:",inline" json:""`

		charts *collectorapi.Charts

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

func (c *Collector) Charts() *collectorapi.Charts {
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
