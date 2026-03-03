//go:build cgo
// +build cgo

package pmi

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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
	collectorapi.Register("websphere_pmi", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create:          func() collectorapi.CollectorV1 { return New() },
		Config: func() any {
			cfg := defaultConfig()
			return &cfg
		},
	})
}
