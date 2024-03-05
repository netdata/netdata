// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

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
	module.Register("envoy", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Envoy {
	return &Envoy{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:9091/stats/prometheus",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},

		charts: &module.Charts{},

		servers:                 make(map[string]bool),
		clusterMgrs:             make(map[string]bool),
		clusterUpstream:         make(map[string]bool),
		listenerMgrs:            make(map[string]bool),
		listenerAdminDownstream: make(map[string]bool),
		listenerDownstream:      make(map[string]bool),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type Envoy struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prom prometheus.Prometheus

	servers                 map[string]bool
	clusterMgrs             map[string]bool
	clusterUpstream         map[string]bool
	listenerMgrs            map[string]bool
	listenerAdminDownstream map[string]bool
	listenerDownstream      map[string]bool
}

func (e *Envoy) Configuration() any {
	return e.Config
}

func (e *Envoy) Init() error {
	if err := e.validateConfig(); err != nil {
		e.Errorf("config validation: %v", err)
		return err
	}

	prom, err := e.initPrometheusClient()
	if err != nil {
		e.Errorf("init Prometheus client: %v", err)
		return err
	}
	e.prom = prom

	return nil
}

func (e *Envoy) Check() error {
	mx, err := e.collect()
	if err != nil {
		e.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")

	}
	return nil
}

func (e *Envoy) Charts() *module.Charts {
	return e.charts
}

func (e *Envoy) Collect() map[string]int64 {
	mx, err := e.collect()
	if err != nil {
		e.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (e *Envoy) Cleanup() {
	if e.prom == nil || e.prom.HTTPClient() == nil {
		return
	}

	e.prom.HTTPClient().CloseIdleConnections()
}
