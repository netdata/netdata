// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

import (
	_ "embed"
	"errors"
	"time"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/pkg/web"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("k8s_kubeproxy", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			// NETDATA_CHART_PRIO_CGROUPS_CONTAINERS        40000
			Priority: 50000,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *KubeProxy {
	return &KubeProxy{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL: "http://127.0.0.1:10249/metrics",
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
		},
		charts: charts.Copy(),
	}
}

type Config struct {
	UpdateEvery int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTP    `yaml:",inline" json:""`
}

type KubeProxy struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus
}

func (kp *KubeProxy) Configuration() any {
	return kp.Config
}

func (kp *KubeProxy) Init() error {
	if err := kp.validateConfig(); err != nil {
		kp.Errorf("config validation: %v", err)
		return err
	}

	prom, err := kp.initPrometheusClient()
	if err != nil {
		kp.Error(err)
		return err
	}
	kp.prom = prom

	return nil
}

func (kp *KubeProxy) Check() error {
	mx, err := kp.collect()
	if err != nil {
		kp.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (kp *KubeProxy) Charts() *Charts {
	return kp.charts
}

func (kp *KubeProxy) Collect() map[string]int64 {
	mx, err := kp.collect()

	if err != nil {
		kp.Error(err)
		return nil
	}

	return mx
}

func (kp *KubeProxy) Cleanup() {
	if kp.prom != nil && kp.prom.HTTPClient() != nil {
		kp.prom.HTTPClient().CloseIdleConnections()
	}
}
