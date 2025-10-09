//go:build cgo

package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportIndexMetrics() {
	for name, metrics := range c.mx.indexes {
		labels := contexts.IndexLabels{
			Index: name,
		}

		contexts.Index.Usage.Set(c.State, labels, contexts.IndexUsageValues{
			Index: metrics.IndexScans,
			Full:  metrics.FullScans,
		})
	}
}
