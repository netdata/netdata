// SPDX-License-Identifier: GPL-3.0-or-later

package cassandra

import (
	_ "embed"
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
					Timeout: web.Duration{Duration: time.Second * 5},
				},
			},
		},
		charts:          baseCharts.Copy(),
		validateMetrics: true,
		mx:              newCassandraMetrics(),
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Cassandra struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	prom prometheus.Prometheus

	validateMetrics bool
	mx              *cassandraMetrics
}

func (c *Cassandra) Init() bool {
	if err := c.validateConfig(); err != nil {
		c.Errorf("error on validating config: %v", err)
		return false
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		c.Errorf("error on init prometheus client: %v", err)
		return false
	}
	c.prom = prom

	return true
}

func (c *Cassandra) Check() bool {
	return len(c.Collect()) > 0
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

func (c *Cassandra) Cleanup() {}
