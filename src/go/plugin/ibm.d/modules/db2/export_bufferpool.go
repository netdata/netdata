package db2

import (
	"fmt"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

func (c *Collector) exportBufferpoolMetrics(mx metricsData) {
	for name, metrics := range mx.bufferpools {
		meta := c.bufferpools[name]
		pageSizeLabel := "unknown"
		if meta != nil && meta.pageSize > 0 {
			pageSizeLabel = fmt.Sprintf("%d", meta.pageSize)
		}

		labels := contexts.BufferpoolLabels{
			Bufferpool: name,
			Page_size:  pageSizeLabel,
		}

		totalReads := metrics.Hits + metrics.Misses
		overallRatio := int64(0)
		if totalReads > 0 {
			overallRatio = metrics.Hits * 100 * Precision / totalReads
		}

		dataReads := metrics.DataHits + metrics.DataMisses
		dataRatio := int64(0)
		if dataReads > 0 {
			dataRatio = metrics.DataHits * 100 * Precision / dataReads
		}

		indexReads := metrics.IndexHits + metrics.IndexMisses
		indexRatio := int64(0)
		if indexReads > 0 {
			indexRatio = metrics.IndexHits * 100 * Precision / indexReads
		}

		xdaReads := metrics.XDAHits + metrics.XDAMisses
		xdaRatio := int64(0)
		if xdaReads > 0 {
			xdaRatio = metrics.XDAHits * 100 * Precision / xdaReads
		}

		columnReads := metrics.ColumnHits + metrics.ColumnMisses
		columnRatio := int64(0)
		if columnReads > 0 {
			columnRatio = metrics.ColumnHits * 100 * Precision / columnReads
		}

		contexts.Bufferpool.HitRatio.Set(c.State, labels, contexts.BufferpoolHitRatioValues{
			Overall: overallRatio,
		})

		contexts.Bufferpool.DetailedHitRatio.Set(c.State, labels, contexts.BufferpoolDetailedHitRatioValues{
			Data:   dataRatio,
			Index:  indexRatio,
			Xda:    xdaRatio,
			Column: columnRatio,
		})

		contexts.Bufferpool.Reads.Set(c.State, labels, contexts.BufferpoolReadsValues{
			Logical:  metrics.LogicalReads,
			Physical: metrics.PhysicalReads,
		})

		contexts.Bufferpool.DataReads.Set(c.State, labels, contexts.BufferpoolDataReadsValues{
			Logical:  metrics.DataLogicalReads,
			Physical: metrics.DataPhysicalReads,
		})

		contexts.Bufferpool.IndexReads.Set(c.State, labels, contexts.BufferpoolIndexReadsValues{
			Logical:  metrics.IndexLogicalReads,
			Physical: metrics.IndexPhysicalReads,
		})

		contexts.Bufferpool.Pages.Set(c.State, labels, contexts.BufferpoolPagesValues{
			Used:  metrics.UsedPages,
			Total: metrics.TotalPages,
		})

		contexts.Bufferpool.Writes.Set(c.State, labels, contexts.BufferpoolWritesValues{
			Writes: metrics.Writes,
		})

		contexts.Bufferpool.DataReads.SetUpdateEvery(c.State, labels, c.Config.UpdateEvery)
		contexts.Bufferpool.IndexReads.SetUpdateEvery(c.State, labels, c.Config.UpdateEvery)
	}
}
