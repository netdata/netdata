//go:build cgo
// +build cgo

package jmx

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

//go:embed config_schema.json
var configSchema string

// New returns a configured WebSphere JMX collector instance.
func New() *Collector {
	cfg := defaultConfig()
	return &Collector{
		Collector: framework.Collector{
			Config: cfg.Config,
		},
		Config: cfg,
	}
}

func (c *Collector) Configuration() any {
	cfgCopy := c.Config
	return &cfgCopy
}

func init() {
	module.Register("websphere_jmx", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config: func() any {
			cfg := defaultConfig()
			return &cfg
		},
	})
}
