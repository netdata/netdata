// SPDX-License-Identifier: GPL-3.0-or-later

package phpfpm

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
	module.Register("phpfpm", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1/status?full&json",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			FcgiPath: "/status",
		},
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	Socket         string `yaml:"socket,omitempty" json:"socket"`
	Address        string `yaml:"address,omitempty" json:"address"`
	FcgiPath       string `yaml:"fcgi_path,omitempty" json:"fcgi_path"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	client client
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	cli, err := c.initClient()
	if err != nil {
		return fmt.Errorf("init client: %v", err)
	}
	c.client = cli

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

func (c *Collector) Charts() *Charts {
	return charts.Copy()
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
