// SPDX-License-Identifier: GPL-3.0-or-later

package dmcache

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
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
			Timeout: web.Duration(time.Second * 2),
		},
		charts:  &module.Charts{},
		devices: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
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
		c.Errorf("dmsetup exec initialization: %v", err)
		return err
	}
	c.exec = dmsetup

	return nil
}

func (c *DmCache) Check() error {
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
