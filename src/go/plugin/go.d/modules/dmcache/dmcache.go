// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux

package dmcache

import (
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
	module.Register("dmcache", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 10,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *DmCache {
	return &DmCache{
		Config: Config{
			Timeout: confopt.Duration(time.Second * 2),
		},
		charts:  &module.Charts{},
		devices: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int              `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	DmCache struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		exec dmsetupCLI

		devices map[string]bool
	}
	dmsetupCLI interface {
		cacheStatus() ([]byte, error)
	}
)

func (c *DmCache) Configuration() any {
	return c.Config
}

func (c *DmCache) Init() error {
	dmsetup, err := c.initDmsetupCLI()
	if err != nil {
		return fmt.Errorf("dmsetup exec initialization: %v", err)
	}
	c.exec = dmsetup

	return nil
}

func (c *DmCache) Check() error {
	mx, err := c.collect()
	if err != nil {
		return err
	}

	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}

	return nil
}

func (c *DmCache) Charts() *module.Charts {
	return c.charts
}

func (c *DmCache) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *DmCache) Cleanup() {}
