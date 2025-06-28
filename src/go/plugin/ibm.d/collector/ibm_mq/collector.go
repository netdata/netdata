// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package ibm_mq

import "C"

import (
	"context"
	_ "embed"
	"sync"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

const (
	// Set to the minimum required version of the monitored application.
	// If the version is lower than this, the collector will not run.
	minVersion = "9.1"
	// Set to the name of the collector.
	collectorName = "ibm_mq"
)

// Config is the configuration for the collector.
type Config struct {
	Channel       string `yaml:"channel"`
	QueueManager  string `yaml:"queue_manager"`
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	module.Module `yaml:",inline"`
}

// Collector is the collector type.
type Collector struct {
	conf Config
	module.Base
	sync.RWMutex
	
	charts *module.Charts
}

// New creates a new collector.
func New() *Collector {
	return &Collector{
		charts: baseCharts.Copy(),
	}
}

// Init is called once when the collector is created.
func (c *Collector) Init(ctx context.Context) error {
	return nil
}

// Check is called once when the collector is created.
func (c *Collector) Check(ctx context.Context) error {
	return nil
}

// Charts returns the charts.
func (c *Collector) Charts() *module.Charts {
	return c.charts
}

// Collect is called to collect metrics.
func (c *Collector) Collect(ctx context.Context) map[string]int64 {
	mx, err := c.collect(ctx)
	if err != nil {
		c.Error(err)
	}
	return mx
}

// Cleanup is called once when the collector is stopped.
func (c *Collector) Cleanup(ctx context.Context) {}

// Configuration returns the collector configuration.
func (c *Collector) Configuration() any {
	return c.conf
}

func init() {
	module.Register(collectorName, module.Creator{
		JobConfigSchema: configSchema,
		Create: func() module.Module {
			return New()
		},
		Config: func() any { return &Config{} },
	})
}
