//go:build cgo

package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportTableMetrics() {
	for name, metrics := range c.mx.tables {
		labels := contexts.TableLabels{
			Table: name,
		}

		contexts.Table.Size.Set(c.State, labels, contexts.TableSizeValues{
			Data:     metrics.DataSize,
			Index:    metrics.IndexSize,
			Long_obj: metrics.LongObjSize,
		})

		contexts.Table.Activity.Set(c.State, labels, contexts.TableActivityValues{
			Read:    metrics.RowsRead,
			Written: metrics.RowsWritten,
		})
	}
}
