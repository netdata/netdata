// SPDX-License-Identifier: GPL-3.0-or-later

//go:build linux && cgo

package mq_pcf

import "C"

import (
	"context"
	_ "embed"
	"errors"
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
	collectorName = "mq_pcf"
)

// Config is the configuration for the collector.
type Config struct {
	// PCF Connection parameters
	QueueManager  string `yaml:"queue_manager"`
	Channel       string `yaml:"channel"`
	Host          string `yaml:"host"`
	Port          int    `yaml:"port"`
	User          string `yaml:"user"`
	Password      string `yaml:"password"`
	
	// Collection options - use pointers for tri-state (nil, true, false)
	CollectQueues   *bool `yaml:"collect_queues"`
	CollectChannels *bool `yaml:"collect_channels"`
	CollectTopics   *bool `yaml:"collect_topics"`
	
	// Filtering
	QueueSelector   string `yaml:"queue_selector"`
	ChannelSelector string `yaml:"channel_selector"`
	
	module.Module `yaml:",inline"`
}

// Collector is the collector type.
type Collector struct {
	conf Config
	module.Base
	sync.RWMutex
	
	charts *module.Charts
	
	// Instance tracking for dynamic chart creation
	collected map[string]bool
	
	// MQ connection
	mqConn *mqConnection
	
	// Version detection
	version string
	edition string
}

// New creates a new collector.
func New() *Collector {
	return &Collector{
		charts:   baseCharts.Copy(),
		collected: make(map[string]bool),
	}
}

// Init is called once when the collector is created.
func (c *Collector) Init(ctx context.Context) error {
	// Validate required configuration
	if c.conf.QueueManager == "" {
		return errors.New("queue_manager is required")
	}
	
	// Set default connection parameters if not specified
	if c.conf.Host == "" {
		c.conf.Host = "localhost"
	}
	if c.conf.Port == 0 {
		c.conf.Port = 1414 // Default MQ port
	}
	if c.conf.Channel == "" {
		c.conf.Channel = "SYSTEM.DEF.SVRCONN" // Default server connection channel
	}
	
	// CRITICAL: Admin configuration MUST always take precedence over auto-detection
	// Only set defaults for collection options if admin hasn't explicitly configured them
	
	if c.conf.CollectQueues == nil {
		// Auto-detection: Default to true for queues (basic feature, supported in all versions)
		defaultValue := true
		c.conf.CollectQueues = &defaultValue
	}
	
	if c.conf.CollectChannels == nil {
		// Auto-detection: Default to true for channels (basic feature, supported in all versions)  
		defaultValue := true
		c.conf.CollectChannels = &defaultValue
	}
	
	if c.conf.CollectTopics == nil {
		// Auto-detection: Default to false for topics (performance impact, optional feature)
		// Version detection could be added here later to enable for MQ 8.0+
		defaultValue := false
		c.conf.CollectTopics = &defaultValue
	}
	
	c.Infof("Collection settings: queues=%v, channels=%v, topics=%v", 
		*c.conf.CollectQueues, *c.conf.CollectChannels, *c.conf.CollectTopics)
	
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
func (c *Collector) Cleanup(ctx context.Context) {
	c.disconnect()
}

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
