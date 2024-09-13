// SPDX-License-Identifier: GPL-3.0-or-later

package cockroachdb

import (
	_ "embed"
	"errors"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

// DefaultMetricsSampleInterval hard coded to 10
// https://github.com/cockroachdb/cockroach/blob/d5ffbf76fb4c4ef802836529188e4628476879bd/pkg/server/config.go#L56-L58
const dbSamplingInterval = 10

func init() {
	module.Register("cockroachdb", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: dbSamplingInterval,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *CockroachDB {
	return &CockroachDB{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8080/_status/vars",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
}

type CockroachDB struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus
}

func (c *CockroachDB) Configuration() any {
	return c.Config
}

func (c *CockroachDB) Init() error {
	if err := c.validateConfig(); err != nil {
		c.Errorf("error on validating config: %v", err)
		return err
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		c.Error(err)
		return err
	}
	c.prom = prom

	if c.UpdateEvery < dbSamplingInterval {
		c.Warningf("'update_every'(%d) is lower then CockroachDB default sampling interval (%d)",
			c.UpdateEvery, dbSamplingInterval)
	}

	return nil
}

func (c *CockroachDB) Check() error {
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

func (c *CockroachDB) Cleanup() {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
