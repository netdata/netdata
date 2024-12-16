// SPDX-License-Identifier: GPL-3.0-or-later

package pika

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/tlscfg"

	"github.com/blang/semver/v4"
	"github.com/redis/go-redis/v9"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("pika", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address: "redis://@localhost:9221",
			Timeout: confopt.Duration(time.Second),
		},

		collectedCommands: make(map[string]bool),
		collectedDbs:      make(map[string]bool),
	}
}

type Config struct {
	Vnode            string           `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery      int              `yaml:"update_every,omitempty" json:"update_every"`
	Address          string           `yaml:"address" json:"address"`
	Timeout          confopt.Duration `yaml:"timeout,omitempty" json:"timeout"`
	tlscfg.TLSConfig `yaml:",inline" json:""`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		pdb redisClient

		server            string
		version           *semver.Version
		collectedCommands map[string]bool
		collectedDbs      map[string]bool
	}
	redisClient interface {
		Info(ctx context.Context, section ...string) *redis.StringCmd
		Close() error
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	pdb, err := c.initRedisClient()
	if err != nil {
		return fmt.Errorf("init redis client: %v", err)
	}
	c.pdb = pdb

	charts, err := c.initCharts()
	if err != nil {
		return fmt.Errorf("init charts: %v", err)
	}
	c.charts = charts

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
	if c.pdb == nil {
		return
	}
	err := c.pdb.Close()
	if err != nil {
		c.Warningf("cleanup: error on closing redis client [%s]: %v", c.Address, err)
	}
	c.pdb = nil
}
