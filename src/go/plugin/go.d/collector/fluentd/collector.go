// SPDX-License-Identifier: GPL-3.0-or-later

package fluentd

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/matcher"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("fluentd", module.Creator{
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
					URL: "http://127.0.0.1:24220",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			}},
		activePlugins: make(map[string]bool),
		charts:        charts.Copy(),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
	PermitPlugin       string `yaml:"permit_plugin_id,omitempty" json:"permit_plugin_id"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	apiClient *apiClient

	permitPlugin  matcher.Matcher
	activePlugins map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("invalid config: %v", err)
	}

	pm, err := c.initPermitPluginMatcher()
	if err != nil {
		return fmt.Errorf("init permit_plugin_id: %v", err)
	}
	c.permitPlugin = pm

	client, err := c.initApiClient()
	if err != nil {
		return fmt.Errorf("init api client: %v", err)
	}
	c.apiClient = client

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout.Duration())

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
	return c.charts
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
	if c.apiClient != nil && c.apiClient.httpClient != nil {
		c.apiClient.httpClient.CloseIdleConnections()
	}
}
