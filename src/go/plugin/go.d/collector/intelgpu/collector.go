// SPDX-License-Identifier: GPL-3.0-or-later

package intelgpu

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("intelgpu", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		ndsudoName: "ndsudo",
		charts:     charts.Copy(),
		engines:    make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int    `yaml:"update_every,omitempty" json:"update_every"`
	Device      string `yaml:"device,omitempty" json:"device"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec       intelGpuTop
	ndsudoName string

	engines map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	topExec, err := c.initIntelGPUTopExec()

	if err != nil {
		return fmt.Errorf("init intelgpu top exec: %v", err)
	}

	c.exec = topExec

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
	if c.exec != nil {
		if err := c.exec.stop(); err != nil {
			c.Error(err)
		}
		c.exec = nil
	}
}
