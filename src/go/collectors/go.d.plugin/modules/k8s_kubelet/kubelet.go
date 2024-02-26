// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubelet

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
	module.Register("k8s_kubelet", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			// NETDATA_CHART_PRIO_CGROUPS_CONTAINERS        40000
			Priority: 50000,
		},
		Create: func() module.Module { return New() },
	})
}

func New() *Kubelet {
	return &Kubelet{
		Config: Config{
			HTTP: web.HTTP{
				Request: web.Request{
					URL:     "http://127.0.0.1:10255/metrics",
					Headers: make(map[string]string),
				},
				Client: web.Client{
					Timeout: web.Duration(time.Second),
				},
			},
			TokenPath: "/var/run/secrets/kubernetes.io/serviceaccount/token",
		},

		charts:             charts.Copy(),
		collectedVMPlugins: make(map[string]bool),
	}
}

type Config struct {
	web.HTTP    `yaml:",inline" json:""`
	UpdateEvery int    `yaml:"update_every" json:"update_every"`
	TokenPath   string `yaml:"token_path" json:"token_path"`
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
		k.Request.Headers["Authorization"] = "Bearer " + tok
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
