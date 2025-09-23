package db2

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

func (c *Collector) exportMemorySetMetrics() {
	for _, ms := range c.memorySets {
		if ms == nil {
			continue
		}

		labels := contexts.MemorySetLabels{
			Host:     ms.hostName,
			Database: ms.dbName,
			Set_type: ms.setType,
			Member:   fmt.Sprintf("%d", ms.member),
		}

		contexts.MemorySet.Usage.Set(c.State, labels, contexts.MemorySetUsageValues{
			Used: ms.Used,
		})

		contexts.MemorySet.Committed.Set(c.State, labels, contexts.MemorySetCommittedValues{
			Committed: ms.Committed,
		})

		contexts.MemorySet.HighWaterMark.Set(c.State, labels, contexts.MemorySetHighWaterMarkValues{
			Hwm: ms.HighWaterMark,
		})

		contexts.MemorySet.AdditionalCommitted.Set(c.State, labels, contexts.MemorySetAdditionalCommittedValues{
			Additional: ms.AdditionalCommitted,
		})

		contexts.MemorySet.PercentUsedHWM.Set(c.State, labels, contexts.MemorySetPercentUsedHWMValues{
			Used_hwm: ms.PercentUsedHWM,
		})
	}
}
