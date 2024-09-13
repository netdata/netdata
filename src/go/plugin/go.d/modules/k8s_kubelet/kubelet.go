// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

import (
	_ "embed"
	"errors"
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

func New() *Kubelet {
	return &Kubelet{
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
	UpdateEvery    int `yaml:"update_every,omitempty" json:"update_every"`
	web.HTTPConfig `yaml:",inline" json:""`
	TokenPath      string `yaml:"token_path,omitempty" json:"token_path"`
}

type Kubelet struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus

	collectedVMPlugins map[string]bool // volume_manager_total_volumes
}

func (k *Kubelet) Configuration() any {
	return k.Config
}

func (k *Kubelet) Init() error {
	if err := k.validateConfig(); err != nil {
		k.Errorf("config validation: %v", err)
		return err
	}

	prom, err := k.initPrometheusClient()
	if err != nil {
		k.Error(err)
		return err
	}
	k.prom = prom

	if tok := k.initAuthToken(); tok != "" {
		k.RequestConfig.Headers["Authorization"] = "Bearer " + tok
	}

	return nil
}

func (k *Kubelet) Check() error {
	mx, err := k.collect()
	if err != nil {
		k.Error(err)
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (k *Kubelet) Charts() *Charts {
	return k.charts
}

func (k *Kubelet) Collect() map[string]int64 {
	mx, err := k.collect()

	if err != nil {
		k.Error(err)
		return nil
	}

	return mx
}

func (k *Kubelet) Cleanup() {
	if k.prom != nil && k.prom.HTTPClient() != nil {
		k.prom.HTTPClient().CloseIdleConnections()
	}
}
