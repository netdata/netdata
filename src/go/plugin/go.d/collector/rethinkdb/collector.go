// SPDX-License-Identifier: GPL-3.0-or-later

package rethinkdb

import (
	"context"
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("rethinkdb", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address: "127.0.0.1:28015",
			Timeout: confopt.Duration(time.Second * 1),
		},

		charts:      clusterCharts.Copy(),
		newConn:     newRethinkdbConn,
		seenServers: make(map[string]bool),
	}
}

type Config struct {
	Vnode              string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int              `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int              `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string           `yaml:"address" json:"address"`
	Timeout            confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	Username           string           `yaml:"username,omitempty" json:"username"`
	Password           string           `yaml:"password,omitempty" json:"password"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	newConn func(cfg Config) (rdbConn, error)
	rdb     rdbConn

	seenServers map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.Address == "" {
		return errors.New("config: address is not set")
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
	ms, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (c *Collector) Cleanup(context.Context) {
	if c.rdb != nil {
		if err := c.rdb.close(); err != nil {
			c.Warningf("cleanup: error on closing client [%s]: %v", c.Address, err)
		}
		c.rdb = nil
	}
}
