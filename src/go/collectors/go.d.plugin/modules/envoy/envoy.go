// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/agent/module"
	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/web"
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
					Timeout: web.Duration{Duration: time.Second * 2},
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
	web.HTTP `yaml:",inline"`
}

type Envoy struct {
	module.Base
	Config `yaml:",inline"`

	prom prometheus.Prometheus

	charts *module.Charts

	servers                 map[string]bool
	clusterMgrs             map[string]bool
	clusterUpstream         map[string]bool
	listenerMgrs            map[string]bool
	listenerAdminDownstream map[string]bool
	listenerDownstream      map[string]bool
}

func (e *Envoy) Init() bool {
	if err := e.validateConfig(); err != nil {
		e.Errorf("config validation: %v", err)
		return false
	}

	prom, err := e.initPrometheusClient()
	if err != nil {
		e.Errorf("init Prometheus client: %v", err)
		return false
	}
	e.prom = prom

	return true
}

func (e *Envoy) Check() bool {
	return len(e.Collect()) > 0
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
