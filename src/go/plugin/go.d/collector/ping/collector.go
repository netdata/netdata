// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ping", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			ProberConfig: ProberConfig{
				Network:    "ip",
				Privileged: true,
				Packets:    5,
				Interval:   confopt.Duration(time.Millisecond * 100),
			},
		},

		charts:    &module.Charts{},
		hosts:     make(map[string]bool),
		newProber: NewProber,
	}
}

type Config struct {
	Vnode        string   `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery  int      `yaml:"update_every,omitempty" json:"update_every"`
	Hosts        []string `yaml:"hosts" json:"hosts"`
	ProberConfig `yaml:",inline" json:",inline"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prober    Prober
	newProber func(ProberConfig, *logger.Logger) Prober

	hosts map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	pr, err := c.initProber()
	if err != nil {
		return fmt.Errorf("init ping prober: %v", err)
	}
	c.prober = pr

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
