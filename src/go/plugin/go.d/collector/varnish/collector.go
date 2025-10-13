// SPDX-License-Identifier: GPL-3.0-or-later

package varnish

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("varnish", module.Creator{
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

		seenBackends: make(map[string]bool),
		seenStorages: make(map[string]bool),
		charts:       varnishCharts.Copy(),
	}

}

type Config struct {
	UpdateEvery     int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout         confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	InstanceName    string           `yaml:"instance_name,omitempty" json:"instance_name,omitempty"`
	DockerContainer string           `yaml:"docker_container,omitempty" json:"docker_container,omitempty"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	exec varnishstatBinary

	seenBackends map[string]bool
	seenStorages map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	vs, err := c.initVarnishstatBinary()
	if err != nil {
		return fmt.Errorf("init varnishstat exec: %v", err)
	}
	c.exec = vs

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
