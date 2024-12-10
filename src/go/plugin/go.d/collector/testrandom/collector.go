// SPDX-License-Identifier: GPL-3.0-or-later

package testrandom

import (
	"context"
	_ "embed"
	"fmt"
	"math/rand"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("testrandom", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: module.UpdateEvery,
			Priority:    module.Priority,
			Disabled:    true,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Charts: ConfigCharts{
				Num:  1,
				Dims: 4,
			},
			HiddenCharts: ConfigCharts{
				Num:  0,
				Dims: 4,
			},
		},

		randInt:       func() int64 { return rand.Int63n(100) },
		collectedDims: make(map[string]bool),
	}
}

type (
	Config struct {
		UpdateEvery  int          `yaml:"update_every,omitempty" json:"update_every"`
		Charts       ConfigCharts `yaml:"charts" json:"charts"`
		HiddenCharts ConfigCharts `yaml:"hidden_charts" json:"hidden_charts"`
	}
	ConfigCharts struct {
		Type     string `yaml:"type,omitempty" json:"type"`
		Num      int    `yaml:"num" json:"num"`
		Contexts int    `yaml:"contexts" json:"contexts"`
		Dims     int    `yaml:"dimensions" json:"dimensions"`
		Labels   int    `yaml:"labels" json:"labels"`
	}
)

type Collector struct {
	module.Base // should be embedded by every module
	Config      `yaml:",inline"`

	randInt       func() int64
	charts        *module.Charts
	collectedDims map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	charts, err := c.initCharts()
	if err != nil {
		return fmt.Errorf("charts init: %v", err)
	}
	c.charts = charts
	return nil
}

func (c *Collector) Check(context.Context) error {
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
