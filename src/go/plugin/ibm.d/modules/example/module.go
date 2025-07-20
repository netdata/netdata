package example

import (
	_ "embed"
	
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

//go:embed "config_schema.json"
var configSchema string

// New creates a new example collector instance
func New() *Collector {
	return &Collector{
		Collector: framework.Collector{
			Config: framework.Config{
				UpdateEvery:          1,
				ObsoletionIterations: 60,
			},
		},
	}
}

// Configuration returns the configuration for the web UI
func (c *Collector) Configuration() any {
	return &c.config
}

// Register the module
func init() {
	module.Register("example", module.Creator{
		JobConfigSchema: configSchema,
		Create: func() module.Module {
			return New()
		},
		Config: func() any {
			return &Config{}
		},
	})
}