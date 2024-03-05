// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("cassandra", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Cassandra {
	return &Cassandra{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:7072/metrics",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second * 5),
				},
			},
		},
		charts:          baseCharts.Copy(),
		validateMetrics: true,
		mx:              newCassandraMetrics(),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type Cassandra struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prom prometheus.Prometheus

	validateMetrics bool

	mx *cassandraMetrics
}

func (c *Cassandra) Configuration() any {
	return c.Config
}

func (c *Cassandra) Init() error {
	if err := c.validateConfig(); err != nil {
		c.Errorf("error on validating config: %v", err)
		return err
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		c.Errorf("error on init prometheus client: %v", err)
		return err
	}
	c.prom = prom

	return nil
}

func (c *Cassandra) Check() error {
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

func (c *Cassandra) Charts() *module.Charts {
	return c.charts
}

func (c *Cassandra) Collect() map[string]int64 {
	mx, err := c.collect()
	if err != nil {
		c.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (c *Cassandra) Cleanup() {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
