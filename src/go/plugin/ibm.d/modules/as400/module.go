//go:build cgo
// +build cgo

package as400

import (
	_ "embed"

	"github.com/netdata/netdata/go/plugins/plugin/framework/collectorapi"
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
	collectorapi.Register("as400", collectorapi.Creator{
		JobConfigSchema: configSchema,
		Create: func() collectorapi.CollectorV1 {
			return New()
		},
		Config: func() any {
			config := defaultConfig()
			return &config
		},
	})
}
