// SPDX-License-Identifier: GPL-3.0-or-later

package geth

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

func init() {
	module.Register("geth", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Geth {
	return &Geth{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:6060/debug/metrics/prometheus",
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
	web.HTTPConfig `yaml:",inline" json:""`
	UpdateEvery    int `yaml:"update_every" json:"update_every"`
}

type Geth struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus
}

func (g *Geth) Configuration() any {
	return g.Config
}

func (g *Geth) Init() error {
	if err := g.validateConfig(); err != nil {
		g.Errorf("error on validating config: %g", err)
		return err
	}

	prom, err := g.initPrometheusClient()
	if err != nil {
		g.Error(err)
		return err
	}
	g.prom = prom

	return nil
}

func (g *Geth) Check() error {
	mx, err := g.collect()
	if err != nil {
		g.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (g *Geth) Charts() *Charts {
	return g.charts
}

func (g *Geth) Collect() map[string]int64 {
	mx, err := g.collect()
	if err != nil {
		g.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (g *Geth) Cleanup() {
	if g.prom != nil && g.prom.HTTPClient() != nil {
		g.prom.HTTPClient().CloseIdleConnections()
	}
}
