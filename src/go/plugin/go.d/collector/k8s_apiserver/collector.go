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
	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
)

//go:embed "config_schema.json"
var configSchema string

func init() {
	collectorapi.Register("k8s_apiserver", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Defaults: collectorapi.Defaults{
			Priority: 50100,
		},
		Create: func() collectorapi.CollectorV1 { return New() },
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
		collectedResources:     make(map[string]int64),
		collectedVerbs:         make(map[string]int64),
		collectedCodes:         make(map[string]int64),
		collectedWorkqueues:    make(map[string]int64),
		collectedAdmissionCtrl: make(map[string]int64),
		collectedAdmissionWH:   make(map[string]int64),
		collectedRESTCodes:     make(map[string]int64),
		collectedRESTMethods:   make(map[string]int64),
	}
}

type Config struct {
	Vnode              string `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery        int    `yaml:"update_every,omitempty" json:"update_every"`
	AutoDetectionRetry int    `yaml:"autodetection_retry,omitempty" json:"autodetection_retry"`
	web.HTTPConfig     `yaml:",inline" json:""`
}

type Collector struct {
	collectorapi.Base
	Config `yaml:",inline" json:""`

	charts *Charts

	prom prometheus.Prometheus

	// Collection cycle counter for staleness tracking
	collectCycle int64

	// Track collected dynamic dimensions/charts with last-seen cycle
	// Maps dimension name to the cycle number when it was last seen
	collectedResources     map[string]int64
	collectedVerbs         map[string]int64
	collectedCodes         map[string]int64
	collectedWorkqueues    map[string]int64
	collectedAdmissionCtrl map[string]int64
	collectedAdmissionWH   map[string]int64
	collectedRESTCodes     map[string]int64
	collectedRESTMethods   map[string]int64
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
