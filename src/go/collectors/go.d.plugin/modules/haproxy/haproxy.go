// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

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
	module.Register("haproxy", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *Haproxy {
	return &Haproxy{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8404/metrics",
				},
				Client: web.Client{
					Timeout: web.Duration{Duration: time.Second},
				},
			},
		},

		charts:          charts.Copy(),
		proxies:         make(map[string]bool),
		validateMetrics: true,
	}
}

type Config struct {
	web.HTTP `yaml:",inline"`
}

type Haproxy struct {
	module.Base
	Config `yaml:",inline"`

	charts *module.Charts

	prom            prometheus.Prometheus
	validateMetrics bool
	proxies         map[string]bool
}

func (h *Haproxy) Init() bool {
	if err := h.validateConfig(); err != nil {
		h.Errorf("config validation: %v", err)
		return false
	}

	prom, err := h.initPrometheusClient()
	if err != nil {
		h.Errorf("prometheus client initialization: %v", err)
		return false
	}
	h.prom = prom

	return true
}

func (h *Haproxy) Check() bool {
	return len(h.Collect()) > 0
}

func (h *Haproxy) Charts() *module.Charts {
	return h.charts
}

func (h *Haproxy) Collect() map[string]int64 {
	ms, err := h.collect()
	if err != nil {
		h.Error(err)
		return nil
	}

	if len(ms) == 0 {
		return nil
	}
	return ms
}

func (Haproxy) Cleanup() {
	// TODO: close http idle connections
}
