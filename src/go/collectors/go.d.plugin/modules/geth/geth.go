// SPDX-License-Identifier: GPL-3.0-or-later

package geth

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/go.d.plugin/pkg/prometheus"
	"github.com/netdata/netdata/go/go.d.plugin/pkg/web"

	"github.com/netdata/netdata/go/go.d.plugin/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("geth", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Geth {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: "http://127.0.0.1:6060/debug/metrics/prometheus",
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: time.Second},
			},
		},
	}

	return &Geth{
		Config: config,
		charts: charts.Copy(),
	}
}

type (
	Config struct {
		web.HTTP `yaml:",inline"`
	}

	Geth struct {
		module.Base
		Config `yaml:",inline"`

		prom   prometheus.Prometheus
		charts *Charts
	}
)

func (g Geth) validateConfig() error {
	if g.URL == "" {
		return errors.New("URL is not set")
	}
	return nil
}

func (g *Geth) initClient() error {
	client, err := web.NewHTTPClient(g.Client)
	if err != nil {
		return err
	}

	g.prom = prometheus.New(client, g.Request)
	return nil
}

func (g *Geth) Init() bool {
	if err := g.validateConfig(); err != nil {
		g.Errorf("error on validating config: %g", err)
		return false
	}
	if err := g.initClient(); err != nil {
		g.Errorf("error on initializing client: %g", err)
		return false
	}
	return true
}

func (g *Geth) Check() bool {
	return len(g.Collect()) > 0
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

func (Geth) Cleanup() {}
