// SPDX-License-Identifier: GPL-3.0-or-later

package k8s_apiserver

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/prometheus"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/pkg/web"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	module.Register("k8s_apiserver", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			Priority: 50100,
		},
		Create: func() module.Module { return New() },
		Config: func() any { return &Config{} },
	})
}

func New() *Collector {
	return &Collector{
		Config: Config{
			HTTPConfig: web.HTTPConfig{
				RequestConfig: web.RequestConfig{
					URL:             "https://kubernetes.default.svc:443/metrics",
					Headers:         make(map[string]string),
					BearerTokenFile: "/var/run/secrets/kubernetes.io/serviceaccount/token",
				},
				ClientConfig: web.ClientConfig{
					Timeout: confopt.Duration(time.Second * 2),
					TLSConfig: tlscfg.TLSConfig{
						TLSCA: "/var/run/secrets/kubernetes.io/serviceaccount/ca.crt",
					},
				},
			},
		},
		charts:                 baseCharts.Copy(),
		collectedResources:     make(map[string]bool),
		collectedVerbs:         make(map[string]bool),
		collectedCodes:         make(map[string]bool),
		collectedWorkqueues:    make(map[string]bool),
		collectedAdmissionCtrl: make(map[string]bool),
		collectedAdmissionWH:   make(map[string]bool),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus

	// Track collected dynamic dimensions/charts
	collectedResources     map[string]bool
	collectedVerbs         map[string]bool
	collectedCodes         map[string]bool
	collectedWorkqueues    map[string]bool
	collectedAdmissionCtrl map[string]bool
	collectedAdmissionWH   map[string]bool
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	if err := c.validateConfig(); err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	prom, err := c.initPrometheusClient()
	if err != nil {
		return fmt.Errorf("init prometheus client: %v", err)
	}
	c.prom = prom

	return nil
}

func (c *Collector) Check(context.Context) error {
	mx, err := c.collect()
	if err != nil {
		return err
	}
	if len(mx) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Charts() *Charts {
	return c.charts
}

func (c *Collector) Collect(context.Context) map[string]int64 {
	mx, err := c.collect()

	if err != nil {
		c.Error(err)
		return nil
	}

	return mx
}

func (c *Collector) Cleanup(context.Context) {
	if c.prom != nil && c.prom.HTTPClient() != nil {
		c.prom.HTTPClient().CloseIdleConnections()
	}
}
