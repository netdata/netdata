// SPDX-License-Identifier: GPL-3.0-or-later

package supervisord

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("supervisord", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			URL: "http://127.0.0.1:9001/RPC2",
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(time.Second),
			},
		},

		charts: summaryCharts.Copy(),
		cache:  make(map[string]map[string]bool),
	}
}

type Config struct {
	Vnode            string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery      int    `yaml:"update_every,omitempty" json:"update_every"`
	URL              string `yaml:"url" json:"url"`
	web.ClientConfig `yaml:",inline" json:""`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	client supervisorClient

	cache map[string]map[string]bool // map[group][procName]collected
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.verifyConfig()
	if err != nil {
		return fmt.Errorf("verify config: %v", err)
	}

	client, err := c.initSupervisorClient()
	if err != nil {
		return fmt.Errorf("init supervisord client: %v", err)
	}
	c.client = client

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
	if c.client != nil {
		c.client.closeIdleConnections()
	}
}
