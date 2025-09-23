//go:build cgo

package db2

import "github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"

func (c *Collector) exportMemoryPoolMetrics() {
	for poolType, metrics := range c.mx.memoryPools {
		meta := c.memoryPools[poolType]
		labelValue := poolType
		if meta != nil && meta.poolType != "" {
			labelValue = meta.poolType
		}

		labels := contexts.MemoryPoolLabels{
			Pool_type: labelValue,
		}

		contexts.MemoryPool.Usage.Set(c.State, labels, contexts.MemoryPoolUsageValues{
			Used: metrics.PoolUsed,
		})

		contexts.MemoryPool.HighWaterMark.Set(c.State, labels, contexts.MemoryPoolHighWaterMarkValues{
			Hwm: metrics.PoolUsedHWM,
		})
	}
}
