// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

// DefaultMetricsSampleInterval hard coded to 10
// https://github.com/cockroachdb/cockroach/blob/d5ffbf76fb4c4ef802836529188e4628476879bd/pkg/server/config.go#L56-L58
const cockroachDBSamplingInterval = 10

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("cockroachdb", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: cockroachDBSamplingInterval,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *CockroachDB {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "http://127.0.0.1:8080/_status/vars",
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second},
			},
		},
	}

	return &CockroachDB{
		Config: config,
		charts: charts.Copy(),
	}
}

type (
	Config struct {
		web.HTTP    `yaml:",inline"`
		UpdateEvery int `yaml:"update_every"`
	}

	CockroachDB struct {
		module.Base
		Config `yaml:",inline"`

		prom   prometheus.Prometheus
		charts *Charts
	}
)

func (c *CockroachDB) validateConfig() error {
	if c.URL == "" {
		return errors.New("URL is not set")
	}
	return nil
}

func (c *CockroachDB) initClient() error {
	client, err := web.NewHTTPClient(c.Client)
	if err != nil {
		return err
	}

	c.prom = prometheus.New(client, c.Request)
	return nil
}

func (c *CockroachDB) Init() bool {
	if err := c.validateConfig(); err != nil {
		c.Errorf("error on validating config: %v", err)
		return false
	}
	if err := c.initClient(); err != nil {
		c.Errorf("error on initializing client: %v", err)
		return false
	}
	if c.UpdateEvery < cockroachDBSamplingInterval {
		c.Warningf("'update_every'(%d) is lower then CockroachDB default sampling interval (%d)",
			c.UpdateEvery, cockroachDBSamplingInterval)
	}
	return true
}

func (c *CockroachDB) Check() bool {
	return len(c.Collect()) > 0
}

func (c *CockroachDB) Charts() *Charts {
	return c.charts
}

func (c *CockroachDB) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (CockroachDB) Cleanup() {}
