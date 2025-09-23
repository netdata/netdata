package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportDatabaseMetrics() {
	for name, values := range c.mx.databases {
		meta := c.databases[name]
		statusLabel := "unknown"
		if meta != nil && meta.status != "" {
			statusLabel = meta.status
		}

		labels := contexts.DatabaseLabels{
			Database: name,
			Status:   statusLabel,
		}

		contexts.Database.Status.Set(c.State, labels, contexts.DatabaseStatusValues{
			Status: values.Status,
		})

		contexts.Database.Applications.Set(c.State, labels, contexts.DatabaseApplicationsValues{
			Applications: values.Applications,
		})
	}
}
