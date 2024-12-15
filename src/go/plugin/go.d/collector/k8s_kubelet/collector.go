// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("k8s_kubelet", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			// NETDATA_CHART_PRIO_CGROUPS_CONTAINERS        40000
			Priority: 50000,
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
					URL:     "http://127.0.0.1:10255/metrics",
					Headers: make(map[string]string),
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
			TokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		},

		charts:             charts.Copy(),
		collectedVMPlugins: make(map[string]bool),
	}
}

type Config struct {
	Vnode          string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery    int    `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	TokenPath      string `yaml:"token_path,omitempty" json:"token_path"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus

	collectedVMPlugins map[string]bool // volume_manager_total_volumes
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	c.prom = prom

	if tok := c.initAuthToken(); tok != "" {
		c.RequestConfig.Headers["Authorization"] = "Bearer " + tok
	}

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
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
