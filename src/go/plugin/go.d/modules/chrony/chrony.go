// SPDX-License-Identifier: GPL-3.0-or-later

package chrony

import (
	_ "embed"
	"errors"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"

	"github.com/facebook/time/ntp/chrony"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("chrony", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Chrony {
	return &Chrony{
		Config: Config{
			Address: "127.0.0.1:323",
			Timeout: web.Duration(time.Second),
		},
		charts:             charts.Copy(),
		addStatsChartsOnce: &sync.Once{},
		newClient:          newChronyClient,
	}
}

type Config struct {
	UpdateEvery int          `yaml:"update_every,omitempty" json:"update_every"`
	Address     string       `yaml:"address" json:"address"`
	Timeout     web.Duration `yaml:"timeout,omitempty" json:"timeout"`
}

type (
	Chrony struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts             *module.Charts
		addStatsChartsOnce *sync.Once

		client    chronyClient
		newClient func(c Config) (chronyClient, error)
	}
	chronyClient interface {
		Tracking() (*chrony.ReplyTracking, error)
		Activity() (*chrony.ReplyActivity, error)
		ServerStats() (*serverStats, error)
		Close()
	}
)

func (c *Chrony) Configuration() any {
	return c.Config
}

func (c *Chrony) Init() error {
	if err := c.validateConfig(); err != nil {
		c.Errorf("config validation: %v", err)
		return err
	}

	return nil
}

func (c *Chrony) Check() error {
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

func (c *Chrony) Charts() *module.Charts {
	return c.charts
}

func (c *Chrony) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Chrony) Cleanup() {
	if c.client != nil {
		c.client.Close()
		c.client = nil
	}
}
