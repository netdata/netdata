// SPDX-License-Identifier: GPL-3.0-or-later

package vcsa

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("vcsa", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5, // VCSA health checks freq is 5 second.
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 5),
				},
			},
		},
		charts: vcsaHealthCharts.Copy(),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
}

type (
	Collector struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		client healthClient
	}

	healthClient interface {
		Login() error
		Logout() error
		Ping() error
		ApplMgmt() (string, error)
		DatabaseStorage() (string, error)
		Load() (string, error)
		Mem() (string, error)
		SoftwarePackages() (string, error)
		Storage() (string, error)
		Swap() (string, error)
		System() (string, error)
	}
)

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	cli, err := c.initHealthClient()
	if err != nil {
		return fmt.Errorf("error on creating health client : %vc", err)
	}
	c.client = cli

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout)

	return nil
}

func (c *Collector) Check(context.Context) error {
	err := c.client.Login()
	if err != nil {
		return err
	}

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
	if c.client == nil {
		return
	}
	err := c.client.Logout()
	if err != nil {
		c.Errorf("error on logout : %v", err)
	}
}
