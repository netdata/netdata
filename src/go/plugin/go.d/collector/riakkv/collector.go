// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("riakkv", module.Creator{
		Create: func() module.Module { return New() },
		// Riak updates the metrics on the /stats endpoint every 1 second.
		// If we use 1 here, it means we might get weird jitter in the graph,
		// so the default is set to 2 seconds to prevent that.
		Defaults: module.Defaults{
			UpdateEvery: 2,
		},
		JobConfigSchema: configSchema,
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					// https://docs.riak.com/riak/kv/2.2.3/developing/api/http/status.1.html
					URL: "http://127.0.0.1:8098/stats",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		once:   &sync.Once{},
		charts: charts.Copy(),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	once   *sync.Once
	charts *module.Charts

	httpClient *http.Client
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.URL == "" {
		return errors.New("config: url not set")
	}

	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return fmt.Errorf("init HTTP client: %v", err)
	}
	c.httpClient = httpClient

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout)

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

func (c *Collector) Cleanup(context.Context) {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
