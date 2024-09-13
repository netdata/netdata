// SPDX-License-Identifier: GPL-3.0-or-later

package clickhouse

import (
	_ "embed"
	"errors"
	"net/http"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("clickhouse", module.Creator{
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
		JobConfigSchema: configSchema,
	})
}

func New() *ClickHouse {
	return &ClickHouse{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8123",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		charts:       chCharts.Copy(),
		seenDisks:    make(map[string]*seenDisk),
		seenDbTables: make(map[string]*seenTable),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type (
	ClickHouse struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *module.Charts

		httpClient *http.Client

		seenDisks    map[string]*seenDisk
		seenDbTables map[string]*seenTable
	}
	seenDisk  struct{ disk string }
	seenTable struct{ db, table string }
)

func (c *ClickHouse) Configuration() any {
	return c.Config
}

func (c *ClickHouse) Init() error {
	if err := c.validateConfig(); err != nil {
		c.Errorf("config validation: %v", err)
		return err
	}

	httpClient, err := c.initHTTPClient()
	if err != nil {
		c.Errorf("init HTTPConfig client: %v", err)
		return err
	}
	c.httpClient = httpClient

	c.Debugf("using URL %s", c.URL)
	c.Debugf("using timeout: %s", c.Timeout)

	return nil
}

func (c *ClickHouse) Check() error {
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

func (c *ClickHouse) Charts() *module.Charts {
	return c.charts
}

func (c *ClickHouse) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}

	return mx
}

func (c *ClickHouse) Cleanup() {
	if c.httpClient != nil {
		c.httpClient.CloseIdleConnections()
	}
}
