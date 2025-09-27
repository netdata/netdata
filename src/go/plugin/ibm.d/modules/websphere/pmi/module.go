//go:build cgo
// +build cgo

package pmi

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/framework"
)

//go:embed config_schema.json
var configSchema string

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
	cfg := c.Config
	return &cfg
}

func init() {
	module.Register("websphere_pmi", module.Creator{
		JobConfigSchema: configSchema,
		Create:          func() module.Module { return New() },
		Config: func() any {
			cfg := defaultConfig()
			return &cfg
		},
	})
}
