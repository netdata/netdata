//go:build cgo
// +build cgo

package jmx

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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
	collectorapi.Register("websphere_jmx", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config: func() any {
			cfg := defaultConfig()
			return &cfg
		},
	})
}
