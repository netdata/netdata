// SPDX-License-Identifier: GPL-3.0-or-later

package envoy

import (
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
	module.Register("envoy", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Envoy {
	return &Envoy{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:9091/stats/prometheus",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
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
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
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
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := e.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init Prometheus client: %v", err)
	}
	e.prom = prom

	return nil
}

func (e *Envoy) Check() error {
	mx, err := e.collect()
	if err != nil {
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
