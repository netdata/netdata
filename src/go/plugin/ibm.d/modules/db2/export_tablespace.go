//go:build cgo

package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportTablespaceMetrics() {
	for name, metrics := range c.mx.tablespaces {
		meta := c.tablespaces[name]
		tspType := "unknown"
		contentType := "unknown"
		stateLabel := "unknown"
		if meta != nil {
			if meta.tbspType != "" {
				tspType = meta.tbspType
			}
			if meta.contentType != "" {
				contentType = meta.contentType
			}
			if meta.state != "" {
				stateLabel = meta.state
			}
		}

		labels := contexts.TablespaceLabels{
			Tablespace:   name,
			Type:         tspType,
			Content_type: contentType,
			State:        stateLabel,
		}

		contexts.Tablespace.Usage.Set(c.State, labels, contexts.TablespaceUsageValues{
			Used: metrics.UsedPercent,
		})

		contexts.Tablespace.Size.Set(c.State, labels, contexts.TablespaceSizeValues{
			Used: metrics.UsedSize,
			Free: metrics.FreeSize,
		})

		contexts.Tablespace.UsableSize.Set(c.State, labels, contexts.TablespaceUsableSizeValues{
			Total:  metrics.TotalSize,
			Usable: metrics.UsableSize,
		})

		contexts.Tablespace.State.Set(c.State, labels, contexts.TablespaceStateValues{
			State: metrics.State,
		})
	}
}
