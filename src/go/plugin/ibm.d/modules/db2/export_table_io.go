package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportTableIOMetrics() {
	for table, metrics := range c.mx.tableIOs {
		labels := contexts.TableIOLabels{
			Table: table,
		}

		contexts.TableIO.Scans.Set(c.State, labels, contexts.TableIOScansValues{
			Scans: metrics.TableScans,
		})

		contexts.TableIO.Rows.Set(c.State, labels, contexts.TableIORowsValues{
			Read: metrics.RowsRead,
		})

		contexts.TableIO.Activity.Set(c.State, labels, contexts.TableIOActivityValues{
			Inserts: metrics.RowsInserted,
			Updates: metrics.RowsUpdated,
			Deletes: metrics.RowsDeleted,
		})

		contexts.TableIO.Overflow.Set(c.State, labels, contexts.TableIOOverflowValues{
			Overflow: metrics.OverflowAccesses,
		})
	}
}
