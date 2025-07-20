
package mq

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

//go:embed "config_schema.json"
var configSchema string

func New() *Collector {
	return &Collector{
		Collector: framework.Collector{
			Config: framework.Config{
				UpdateEvery:          1,
				ObsoletionIterations: 60,
			},
		},
		Config: defaultConfig(),
	}
}

func (c *Collector) Configuration() any {
	return &c.Config
}

func init() {
	module.Register("mq", module.Creator{
		JobConfigSchema: configSchema,
		Create: func() module.Module {
			return New()
		},
		Config: func() any {
			config := defaultConfig()
			return &config
		},
	})
}
