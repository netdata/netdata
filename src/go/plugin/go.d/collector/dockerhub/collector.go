// SPDX-License-Identifier: GPL-3.0-or-later

package dockerhub

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
	module.Register("dockerhub", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "https://hub.docker.com/v2/repositories",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
				},
			},
		},
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	Repositories   []string `yaml:"repositories" json:"repositories"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	client *apiClient
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	client, err := c.initApiClient()
	if err != nil {
		return fmt.Errorf("init api client: %v", err)
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

func (c *Collector) Charts() *Charts {
	cs := charts.Copy()
	addReposToCharts(c.Repositories, cs)
	return cs
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()

	if err != nil {
		c.Error(err)
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.client != nil && c.client.httpClient != nil {
		c.client.httpClient.CloseIdleConnections()
	}
}
