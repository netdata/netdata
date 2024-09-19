// SPDX-License-Identifier: GPL-3.0-or-later

package ceph

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("ceph", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Ceph {
	return &Ceph{
		Config: Config{
			BinaryPath: "/usr/bin/ceph",
			Timeout:    confopt.Duration(time.Second * 2),
		},
		seenPools: make(map[string]bool),
		seenOsds:  make(map[string]bool),
		charts:    generalCharts.Copy(),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	BinaryPath  string           `yaml:"binary_path,omitempty" json:"binary_path"`
}

type (
	Ceph struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec cephBinary

		seenPools map[string]bool
		seenOsds  map[string]bool
	}
)

func (c *Ceph) Configuration() any {
	return c.Config
}

func (c *Ceph) Init() error {
	cp, err := c.initCephBinary()
	if err != nil {
		c.Errorf("ceph exec initialization: %v", err)
		return err
	}
	c.exec = cp

	return nil
}

func (c *Ceph) Check() error {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *Ceph) Charts() *module.Charts {
	return c.charts
}

func (c *Ceph) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *Ceph) Cleanup() {}
