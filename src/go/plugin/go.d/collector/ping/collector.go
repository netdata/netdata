// SPDX-License-Identifier: GPL-3.0-or-later

package ping

import (
	"context"
	_ "embed"
	"errors"
	"fmt"
	"time"

	"github.com/netdata/netdata/go/plugins/logger"
	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/metrix"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

//go:embed "charts.yaml"
var pingChartTemplateV2 string

func init() {
	module.Register("ping", module.Creator{
		JobConfigSchema: configSchema,
		Defaults: module.Defaults{
			UpdateEvery: 5,
		},
		CreateV2: func() module.ModuleV2 { return New() },
		Config:   func() any { return &Config{} },
	})
}

func New() *Collector {
	store := metrix.NewCollectorStore()
	meter := store.Write().SnapshotMeter("")

	return &Collector{
		Config: Config{
			ProberConfig: ProberConfig{
				Network:    "ip",
				Privileged: true,
				Packets:    5,
				Interval:   confopt.Duration(time.Millisecond * 100),
			},
			JitterEWMASamples: 16,
			JitterSMAWindow:   10,
		},

		newProber:  NewProber,
		jitterEWMA: make(map[string]float64),
		jitterSMA:  make(map[string][]float64),
		store:      store,
		metrics: v2Metrics{
			minRTT:      meter.GaugeVec("min_rtt", []string{"host"}),
			maxRTT:      meter.GaugeVec("max_rtt", []string{"host"}),
			avgRTT:      meter.GaugeVec("avg_rtt", []string{"host"}),
			stdDevRTT:   meter.GaugeVec("std_dev_rtt", []string{"host"}),
			rttVariance: meter.GaugeVec("rtt_variance", []string{"host"}),
			meanJitter:  meter.GaugeVec("mean_jitter", []string{"host"}),
			ewmaJitter:  meter.GaugeVec("ewma_jitter", []string{"host"}),
			smaJitter:   meter.GaugeVec("sma_jitter", []string{"host"}),
			packetLoss:  meter.GaugeVec("packet_loss", []string{"host"}),
			packetsRecv: meter.GaugeVec("packets_recv", []string{"host"}),
			packetsSent: meter.GaugeVec("packets_sent", []string{"host"}),
		},
	}
}

type Config struct {
	Vnode             string   `yaml:"vnode,omitempty" json:"vnode"`
	UpdateEvery       int      `yaml:"update_every,omitempty" json:"update_every"`
	Hosts             []string `yaml:"hosts" json:"hosts"`
	JitterEWMASamples int      `yaml:"jitter_ewma_samples,omitempty" json:"jitter_ewma_samples"`
	JitterSMAWindow   int      `yaml:"jitter_sma_window,omitempty" json:"jitter_sma_window"`
	ProberConfig      `yaml:",inline" json:",inline"`
}

type Collector struct {
	module.Base
	Config `yaml:",inline" json:""`

	prober    Prober
	newProber func(ProberConfig, *logger.Logger) Prober

	store      metrix.CollectorStore
	metrics    v2Metrics
	jitterEWMA map[string]float64   // EWMA jitter state per host
	jitterSMA  map[string][]float64 // SMA jitter window per host
}

func (c *Collector) Configuration() any {
	return c.Config
}

func (c *Collector) Init(context.Context) error {
	err := c.validateConfig()
	if err != nil {
		return fmt.Errorf("config validation: %v", err)
	}

	pr, err := c.initProber()
	if err != nil {
		return fmt.Errorf("init ping prober: %v", err)
	}
	c.prober = pr

	return nil
}

func (c *Collector) Check(context.Context) error {
	samples := c.collectSamples(false)
	if len(samples) == 0 {
		return errors.New("no metrics collected")
	}
	return nil
}

func (c *Collector) Collect(context.Context) error {
	samples := c.collectSamples(true)
	if len(samples) == 0 {
		return nil
	}

	for _, sample := range samples {
		c.metrics.packetsRecv.WithLabelValues(sample.host).Observe(float64(sample.packetsRecv))
		c.metrics.packetsSent.WithLabelValues(sample.host).Observe(float64(sample.packetsSent))
		c.metrics.packetLoss.WithLabelValues(sample.host).Observe(sample.packetLossPercent)

		if sample.hasRTT {
			c.metrics.minRTT.WithLabelValues(sample.host).Observe(sample.minRTTMS)
			c.metrics.maxRTT.WithLabelValues(sample.host).Observe(sample.maxRTTMS)
			c.metrics.avgRTT.WithLabelValues(sample.host).Observe(sample.avgRTTMS)
			c.metrics.stdDevRTT.WithLabelValues(sample.host).Observe(sample.stdDevRTTMS)
			c.metrics.rttVariance.WithLabelValues(sample.host).Observe(sample.rttVarianceMS2)
		}

		if sample.hasJitter {
			c.metrics.meanJitter.WithLabelValues(sample.host).Observe(sample.meanJitterMS)
			c.metrics.ewmaJitter.WithLabelValues(sample.host).Observe(sample.ewmaJitterMS)
			c.metrics.smaJitter.WithLabelValues(sample.host).Observe(sample.smaJitterMS)
		}
	}

	return nil
}

func (c *Collector) Cleanup(context.Context) {}

func (c *Collector) MetricStore() metrix.CollectorStore { return c.store }

func (c *Collector) ChartTemplateYAML() string { return pingChartTemplateV2 }
