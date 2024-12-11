// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn_status_log

import (
	"context"
	_ "embed"
	"errors"
	"fmt"

	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openvpn_status_log", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			LogPath: "/var/log/openvpn/status.log",
		},
		charts:         charts.Copy(),
		collectedUsers: make(map[string]bool),
	}
}

type Config struct {
	UpdateEvery  int                `yaml:"update_every,omitempty" json:"update_every"`
	LogPath      string             `yaml:"log_path" json:"log_path"`
	PerUserStats matcher.SimpleExpr `yaml:"per_user_stats,omitempty" json:"per_user_stats"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	perUserMatcher matcher.Matcher
	collectedUsers map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("error on validating config: %v", err)
	}

	m, err := c.initPerUserStatsMatcher()
	if err != nil {
		return fmt.Errorf("error on creating 'per_user_stats' matcher: %v", err)
	}
	if m != nil {
		c.perUserMatcher = m
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
