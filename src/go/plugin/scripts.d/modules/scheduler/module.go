// SPDX-License-Identifier: GPL-3.0-or-later

package scheduler

import (
	"context"
	_ "embed"
	"fmt"
	"strings"
	"time"

	"github.com/netdata/netdata/go/plugins/pkg/confopt"
	"github.com/netdata/netdata/go/plugins/pkg/tlscfg"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/charts"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/runtime"
	"github.com/netdata/netdata/go/plugins/plugin/scripts.d/pkg/schedulers"
)

//go:embed config_schema.json
var configSchema string

var collectSchedulerMetrics = schedulers.CollectMetrics

func init() {
	module.Register("scheduler", module.Creator{
		JobConfigSchema: configSchema,
		Defaults:        module.Defaults{AutoDetectionRetry: 60},
		Create:          func() module.Module { return New() },
		Config:          func() any { return &Config{} },
	})
}

// Config defines a scheduler job configuration.
type Config struct {
	Name      string            `yaml:"name" json:"name"`
	Workers   int               `yaml:"workers,omitempty" json:"workers,omitempty"`
	QueueSize int               `yaml:"queue_size,omitempty" json:"queue_size,omitempty"`
	Labels    map[string]string `yaml:"labels,omitempty" json:"labels,omitempty"`
	Logging   LoggingConfig     `yaml:"logging,omitempty" json:"logging,omitempty"`
}

// LoggingConfig mirrors the Nagios logging block so schedulers can emit OTLP logs.
type LoggingConfig struct {
	Enabled bool              `yaml:"enabled,omitempty" json:"enabled"`
	OTLP    OTLPLoggingConfig `yaml:"otlp,omitempty" json:"otlp"`
}

type OTLPLoggingConfig struct {
	Endpoint         string            `yaml:"endpoint,omitempty" json:"endpoint"`
	Timeout          confopt.Duration  `yaml:"timeout,omitempty" json:"timeout"`
	TLS              *bool             `yaml:"tls,omitempty" json:"tls"`
	Headers          map[string]string `yaml:"headers,omitempty" json:"headers"`
	TLSServerName    string            `yaml:"tls_server_name,omitempty" json:"tls_server_name,omitempty"`
	tlscfg.TLSConfig `yaml:",inline" json:",inline"`
}

func (l *LoggingConfig) setDefaults() {
	if l == nil {
		return
	}
	if l.OTLP.Endpoint == "" {
		l.OTLP.Endpoint = runtime.DefaultOTLPEndpoint
	}
	if l.OTLP.Timeout == 0 {
		l.OTLP.Timeout = confopt.Duration(runtime.DefaultOTLPTimeout)
	}
	if l.OTLP.Headers == nil {
		l.OTLP.Headers = make(map[string]string)
	}
	if l.OTLP.TLS == nil {
		v := true
		l.OTLP.TLS = &v
	}
	if !l.Enabled {
		l.Enabled = true
	}
}

func (l LoggingConfig) emitterConfig() runtime.OTLPEmitterConfig {
	return runtime.OTLPEmitterConfig{
		Endpoint:   l.OTLP.Endpoint,
		Timeout:    time.Duration(l.OTLP.Timeout),
		UseTLS:     l.OTLP.tlsEnabled(),
		Headers:    l.OTLP.Headers,
		TLSConfig:  l.OTLP.TLSConfig,
		ServerName: l.OTLP.TLSServerName,
	}
}

func (c OTLPLoggingConfig) tlsEnabled() bool {
	if c.TLS == nil {
		return true
	}
	return *c.TLS
}

// Collector implements the virtual scheduler job.
type Collector struct {
	module.Base
	Config `yaml:",inline" json:",inline"`

	applied bool
	charts  *module.Charts
}

// New returns a Collector with defaults applied.
func New() *Collector {
	cfg := Config{
		Workers:   50,
		QueueSize: 128,
		Labels:    make(map[string]string),
		Logging: LoggingConfig{
			Enabled: true,
			OTLP: OTLPLoggingConfig{
				Endpoint: runtime.DefaultOTLPEndpoint,
				Timeout:  confopt.Duration(runtime.DefaultOTLPTimeout),
				TLS:      boolPtr(false),
				Headers:  make(map[string]string),
			},
		},
	}
	return &Collector{Config: cfg, charts: &module.Charts{}}
}

func boolPtr(v bool) *bool {
	b := v
	return &b
}

// Configuration satisfies module.Module.
func (c *Collector) Configuration() any { return &c.Config }

func (c *Collector) Init(context.Context) error {
	if c.Name == "" {
		return fmt.Errorf("scheduler name is required")
	}
	if c.Workers <= 0 {
		c.Workers = 50
	}
	if c.QueueSize <= 0 {
		c.QueueSize = 128
	}
	if c.Labels == nil {
		c.Labels = make(map[string]string)
	}
	c.Logging.setDefaults()
	if c.charts == nil {
		c.charts = &module.Charts{}
	}
	charts := charts.BuildSchedulerCharts(c.Name, 100)
	if err := c.charts.Add(charts...); err != nil {
		return err
	}

	def := schedulers.Definition{
		Name:           c.Name,
		Workers:        c.Workers,
		QueueSize:      c.QueueSize,
		Labels:         c.Labels,
		LoggingEnabled: c.Logging.Enabled,
		Logging:        c.Logging.emitterConfig(),
	}
	if err := schedulers.ApplyDefinition(def, c.Logger); err != nil {
		return err
	}
	c.applied = true
	return nil
}

func (c *Collector) Check(context.Context) error { return nil }

func (c *Collector) Charts() *module.Charts { return c.charts }

func (c *Collector) Collect(context.Context) map[string]int64 {
	all := collectSchedulerMetrics(c.Name)
	if len(all) == 0 {
		return nil
	}
	prefix := fmt.Sprintf("%s.scheduler.", c.Name)
	filtered := make(map[string]int64)
	for k, v := range all {
		if strings.HasPrefix(k, prefix) {
			filtered[k] = v
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func (c *Collector) Cleanup(context.Context) {
	if c.applied {
		_ = schedulers.RemoveDefinition(c.Name)
	}
	if c.charts != nil {
		c.charts = &module.Charts{}
	}
}
