// SPDX-License-Identifier: GPL-3.0-or-later

package openvpn

import (
	"context"
	_ "embed"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/collector/openvpn/client"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/socket"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("openvpn", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			Address: "127.0.0.1:7505",
			Timeout: confopt.Duration(time.Second),
		},

		charts:         charts.Copy(),
		collectedUsers: make(map[string]bool),
	}
}

type Config struct {
	Vnode              string             `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int                `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int                `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	Address            string             `yaml:"address" json:"address"`
	Timeout            confopt.Duration   `yaml:"timeout,omitempty" json:"timeout"`
	PerUserStats       matcher.SimpleExpr `yaml:"per_user_stats,omitempty" json:"per_user_stats"`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *Charts

		client openVPNClient

		collectedUsers map[string]bool
		perUserMatcher matcher.Matcher
	}
	openVPNClient interface {
		socket.Client
		Version() (*client.Version, error)
		LoadStats() (*client.LoadStats, error)
		Users() (client.Users, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return err
	}

	m, err := c.initPerUserMatcher()
	if err != nil {
		return err
	}
	c.perUserMatcher = m

	c.client = c.initClient()

	c.Infof("using address: %s, timeout: %s", c.Address, c.Timeout)

	return nil
}

func (c *Collector) Check(ctx context.Context) error {
	if err := c.client.Connect(); err != nil {
		return err
	}
	defer func() { _ = c.client.Disconnect() }()

	ver, err := c.client.Version()
	if err != nil {
		c.Cleanup(ctx)
		return err
	}

	c.Infof("connected to OpenVPN v%d.%d.%d, Management v%d", ver.Major, ver.Minor, ver.Patch, ver.Management)

	return nil
}

func (c *Collector) Charts() *Charts { return c.charts }

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
	if c.client == nil {
		return
	}
	_ = c.client.Disconnect()
}
