// SPDX-License-Identifier: GPL-3.0-or-later

package vernemq

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
	module.Register("vernemq", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
	})
}

func New() *VerneMQ {
	return &VerneMQ{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:8888/metrics",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts: charts.Copy(),
		cache:  make(map[string]bool),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int `yaml:"update_every" json:"update_every"`
}

type (
	VerneMQ struct {
		module.Base
		Config `yaml:",inline" json:""`

		charts *Charts

		prom prometheus.Prometheus

		cache map[string]bool
	}
)

func (v *VerneMQ) Configuration() any {
	return v.Config
}

func (v *VerneMQ) Init() error {
	if err := v.validateConfig(); err != nil {
		v.Errorf("error on validating config: %v", err)
		return err
	}

	prom, err := v.initPrometheusClient()
	if err != nil {
		v.Error(err)
		return err
	}
	v.prom = prom

	return nil
}

func (v *VerneMQ) Check() error {
	mx, err := v.collect()
	if err != nil {
		v.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (v *VerneMQ) Charts() *Charts {
	return v.charts
}

func (v *VerneMQ) Collect() map[string]int64 {
	mx, err := v.collect()
	if err != nil {
		v.Error(err)
	}

	if len(mx) == 0 {
		return nil
	}
	return mx
}

func (v *VerneMQ) Cleanup() {
	if v.prom != nil && v.prom.HTTPClient() != nil {
		v.prom.HTTPClient().CloseIdleConnections()
	}
}
