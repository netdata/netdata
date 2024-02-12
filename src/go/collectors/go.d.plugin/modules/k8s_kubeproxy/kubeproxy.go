// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_kubeproxy

import (
	_ "embed"
	"time"

	"github.com/netdata/go.d.plugin/pkg/prometheus"
	"github.com/netdata/go.d.plugin/pkg/web"

	"github.com/netdata/go.d.plugin/agent/module"
)

const (
	defaultURL         = "http://127.0.0.1:10249/metrics"
	defaultHTTPTimeout = time.Second * 2
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
	})
}

// New creates KubeProxy with default values.
func New() *KubeProxy {
	config := Config{
		HTTP: web.HTTP{
			Request: web.Request{
				URL: defaultURL,
			},
			Client: web.Client{
				Timeout: web.Duration{Duration: defaultHTTPTimeout},
			},
		},
	}
	return &KubeProxy{
		Config: config,
		charts: charts.Copy(),
	}
}

// Config is the KubeProxy module configuration.
type Config struct {
	web.HTTP `yaml:",inline"`
}

// KubeProxy is KubeProxy module.
type KubeProxy struct {
	module.Base
	Config `yaml:",inline"`

	prom   prometheus.Prometheus
	charts *Charts
}

// Cleanup makes cleanup.
func (KubeProxy) Cleanup() {}

// Init makes initialization.
func (kp *KubeProxy) Init() bool {
	if kp.URL == "" {
		kp.Error("URL not set")
		return false
	}

	client, err := web.NewHTTPClient(kp.Client)
	if err != nil {
		kp.Errorf("error on creating http client : %v", err)
		return false
	}

	kp.prom = prometheus.New(client, kp.Request)

	return true
}

// Check makes check.
func (kp *KubeProxy) Check() bool {
	return len(kp.Collect()) > 0
}

// Charts creates Charts.
func (kp KubeProxy) Charts() *Charts {
	return kp.charts
}

// Collect collects metrics.
func (kp *KubeProxy) Collect() map[string]int64 {
	mx, err := kp.collect()

	if err != nil {
		kp.Error(err)
		return nil
	}

	return mx
}
