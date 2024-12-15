// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("hdfs", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Collector {
	config := Config{
		HTTPConfig: web.HTTPConfig{
			RequestConfig: web.RequestConfig{
				URL: "http://127.0.0.1:9870/jmx",
			},
			ClientConfig: web.ClientConfig{
				Timeout: confopt.Duration(time.Second),
			},
		},
	}

	return &Collector{
		Config: config,
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	web.HTTPConfig `yaml:",inline" json:""`
	UpdateEvery    int `yaml:"update_every" json:"update_every"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	httpClient *http.Client

	nodeType string
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if c.URL == "" {
		return errors.New("URL is required but not set")
	}

	httpClient, err := web.NewHTTPClient(c.ClientConfig)
	if err != nil {
		return fmt.Errorf("failed to create HTTP client: %v", err)
	}
	c.httpClient = httpClient

	return nil
}

func (c *Collector) Check(context.Context) error {
	typ, err := c.determineNodeType()
	if err != nil {
		return fmt.Errorf("error on node type determination : %v", err)
	}
	c.nodeType = typ

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
	switch c.nodeType {
	default:
		return nil
	case nameNodeType:
		return nameNodeCharts()
	case dataNodeType:
		return dataNodeCharts()
	}
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
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
