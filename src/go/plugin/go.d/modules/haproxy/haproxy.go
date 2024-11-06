// SPDX-License-Identifier: GPL-3.0-or-later

package haproxy

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
	module.Register("haproxy", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

func New() *Haproxy {
	return &Haproxy{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL: "http://127.0.0.1:8404/metrics",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second),
				},
			},
		},

		charts:          charts.Copy(),
		proxies:         make(map[string]bool),
		validateMetrics: true,
	}
}

type Config struct {
	web.HTTPConfig `yaml:",inline" json:""`
	UpdateEvery    int `yaml:"update_every" json:"update_every"`
}

type Haproxy struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *module.Charts

	prom prometheus.Prometheus

	validateMetrics bool
	proxies         map[string]bool
}

func (h *Haproxy) Configuration() any {
	return h.Config
}

func (h *Haproxy) Init() error {
	if err := h.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := h.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("prometheus client initialization: %v", err)
	}
	h.prom = prom

	return nil
}

func (h *Haproxy) Check() error {
	mx, err := h.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (h *Haproxy) Charts() *module.Charts {
	return h.charts
}

func (h *Haproxy) Collect() map[string]int64 {
	mx, err := h.collect()
	if err != nil {
		h.Error(err)
		return nil
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (h *Haproxy) Cleanup() {
	if h.prom != nil && h.prom.HTTPClient() != nil {
		h.prom.HTTPClient().CloseIdleConnections()
	}
}
