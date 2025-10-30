//go:build cgo

package db2

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

type bufferpoolEntry struct {
	name    string
	meta    *bufferpoolMetrics
	metrics bufferpoolInstanceMetrics
}

type bufferpoolGroupAggregate struct {
	Hits, Misses             int64
	DataHits, DataMisses     int64
	IndexHits, IndexMisses   int64
	XDAHits, XDAMisses       int64
	ColumnHits, ColumnMisses int64

	LogicalReads, PhysicalReads           int64
	DataLogicalReads, DataPhysicalReads   int64
	IndexLogicalReads, IndexPhysicalReads int64

	UsedPages, TotalPages int64
	Writes                int64
}

func bufferpoolGroupKey(name string) string {
	if name == "" {
		return "__unknown__"
	}
	up := strings.ToUpper(name)
	if idx := strings.Index(up, "_"); idx > 0 {
		return up[:idx]
	}
	return up
}

func (a *bufferpoolGroupAggregate) add(m bufferpoolInstanceMetrics) {
	a.Hits += m.Hits
	a.Misses += m.Misses
	a.DataHits += m.DataHits
	a.DataMisses += m.DataMisses
	a.IndexHits += m.IndexHits
	a.IndexMisses += m.IndexMisses
	a.XDAHits += m.XDAHits
	a.XDAMisses += m.XDAMisses
	a.ColumnHits += m.ColumnHits
	a.ColumnMisses += m.ColumnMisses

	a.LogicalReads += m.LogicalReads
	a.PhysicalReads += m.PhysicalReads
	a.DataLogicalReads += m.DataLogicalReads
	a.DataPhysicalReads += m.DataPhysicalReads
	a.IndexLogicalReads += m.IndexLogicalReads
	a.IndexPhysicalReads += m.IndexPhysicalReads

	a.UsedPages += m.UsedPages
	a.TotalPages += m.TotalPages
	a.Writes += m.Writes
}

func (c *Collector) exportBufferpoolMetrics(mx metricsData) {
	entries := make([]bufferpoolEntry, 0, len(mx.bufferpools))
	for name, metrics := range mx.bufferpools {
		entries = append(entries, bufferpoolEntry{
			name:    name,
			meta:    c.bufferpools[name],
			metrics: metrics,
		})
	}

	if len(entries) == 0 {
		c.clearWarnOnce("db2_bufferpool_overflow")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	limit := c.MaxBufferpools
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	groupAgg := make(map[string]*bufferpoolGroupAggregate)
	overflowAgg := &bufferpoolGroupAggregate{}
	overflowCount := 0
	overflowGroups := make(map[string]int)
	overflowExample := make(map[string]string)

	for idx, entry := range entries {
		groupKey := bufferpoolGroupKey(entry.name)
		agg := groupAgg[groupKey]
		if agg == nil {
			agg = &bufferpoolGroupAggregate{}
			groupAgg[groupKey] = agg
		}
		agg.add(entry.metrics)

		if idx < limit {
			c.emitPerBufferpoolMetrics(entry)
			continue
		}

		overflowAgg.add(entry.metrics)
		overflowCount++
		overflowGroups[groupKey]++
		if _, ok := overflowExample[groupKey]; !ok {
			overflowExample[groupKey] = entry.name
		}
	}

	c.emitBufferpoolGroupMetrics(groupAgg, overflowAgg, overflowCount)

	if overflowCount > 0 {
		parts := make([]string, 0, len(overflowGroups))
		for group, count := range overflowGroups {
			parts = append(parts, fmt.Sprintf("%s:%d (e.g. %s)", group, count, overflowExample[group]))
		}
		sort.Strings(parts)
		c.warnOnce("db2_bufferpool_overflow", "too many buffer pools for per-instance charts (MaxBufferpools=%d). Aggregated %d additional pools: %s", c.MaxBufferpools, overflowCount, strings.Join(parts, ", "))
	} else {
		c.clearWarnOnce("db2_bufferpool_overflow")
	}
}

func (c *Collector) emitPerBufferpoolMetrics(entry bufferpoolEntry) {
	pageSizeLabel := "unknown"
	if entry.meta != nil && entry.meta.pageSize > 0 {
		pageSizeLabel = fmt.Sprintf("%d", entry.meta.pageSize)
	}

	labels := contexts.BufferpoolLabels{
		Bufferpool: entry.name,
		Page_size:  pageSizeLabel,
	}

	totalReads := entry.metrics.Hits + entry.metrics.Misses
	overallRatio := int64(0)
	if totalReads > 0 {
		overallRatio = entry.metrics.Hits * 100 * Precision / totalReads
	}

	dataReads := entry.metrics.DataHits + entry.metrics.DataMisses
	dataRatio := int64(0)
	if dataReads > 0 {
		dataRatio = entry.metrics.DataHits * 100 * Precision / dataReads
	}

	indexReads := entry.metrics.IndexHits + entry.metrics.IndexMisses
	indexRatio := int64(0)
	if indexReads > 0 {
		indexRatio = entry.metrics.IndexHits * 100 * Precision / indexReads
	}

	xdaReads := entry.metrics.XDAHits + entry.metrics.XDAMisses
	xdaRatio := int64(0)
	if xdaReads > 0 {
		xdaRatio = entry.metrics.XDAHits * 100 * Precision / xdaReads
	}

	columnReads := entry.metrics.ColumnHits + entry.metrics.ColumnMisses
	columnRatio := int64(0)
	if columnReads > 0 {
		columnRatio = entry.metrics.ColumnHits * 100 * Precision / columnReads
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
		Logical:  entry.metrics.LogicalReads,
		Physical: entry.metrics.PhysicalReads,
	})

	contexts.Bufferpool.DataReads.Set(c.State, labels, contexts.BufferpoolDataReadsValues{
		Logical:  entry.metrics.DataLogicalReads,
		Physical: entry.metrics.DataPhysicalReads,
	})

	contexts.Bufferpool.IndexReads.Set(c.State, labels, contexts.BufferpoolIndexReadsValues{
		Logical:  entry.metrics.IndexLogicalReads,
		Physical: entry.metrics.IndexPhysicalReads,
	})

	contexts.Bufferpool.Pages.Set(c.State, labels, contexts.BufferpoolPagesValues{
		Used:  entry.metrics.UsedPages,
		Total: entry.metrics.TotalPages,
	})

	contexts.Bufferpool.Writes.Set(c.State, labels, contexts.BufferpoolWritesValues{
		Writes: entry.metrics.Writes,
	})

	contexts.Bufferpool.DataReads.SetUpdateEvery(c.State, labels, c.Config.UpdateEvery)
	contexts.Bufferpool.IndexReads.SetUpdateEvery(c.State, labels, c.Config.UpdateEvery)
}

func (c *Collector) emitBufferpoolGroupMetrics(groups map[string]*bufferpoolGroupAggregate, overflow *bufferpoolGroupAggregate, overflowCount int) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		agg := groups[key]
		labels := contexts.BufferpoolGroupLabels{Group: key}

		totalReads := agg.Hits + agg.Misses
		overallRatio := int64(0)
		if totalReads > 0 {
			overallRatio = agg.Hits * 100 * Precision / totalReads
		}

		dataReads := agg.DataHits + agg.DataMisses
		dataRatio := int64(0)
		if dataReads > 0 {
			dataRatio = agg.DataHits * 100 * Precision / dataReads
		}

		indexReads := agg.IndexHits + agg.IndexMisses
		indexRatio := int64(0)
		if indexReads > 0 {
			indexRatio = agg.IndexHits * 100 * Precision / indexReads
		}

		xdaReads := agg.XDAHits + agg.XDAMisses
		xdaRatio := int64(0)
		if xdaReads > 0 {
			xdaRatio = agg.XDAHits * 100 * Precision / xdaReads
		}

		columnReads := agg.ColumnHits + agg.ColumnMisses
		columnRatio := int64(0)
		if columnReads > 0 {
			columnRatio = agg.ColumnHits * 100 * Precision / columnReads
		}

		contexts.BufferpoolGroup.HitRatio.Set(c.State, labels, contexts.BufferpoolGroupHitRatioValues{
			Overall: overallRatio,
		})

		contexts.BufferpoolGroup.DetailedHitRatio.Set(c.State, labels, contexts.BufferpoolGroupDetailedHitRatioValues{
			Data:   dataRatio,
			Index:  indexRatio,
			Xda:    xdaRatio,
			Column: columnRatio,
		})

		contexts.BufferpoolGroup.Reads.Set(c.State, labels, contexts.BufferpoolGroupReadsValues{
			Logical:  agg.LogicalReads,
			Physical: agg.PhysicalReads,
		})

		contexts.BufferpoolGroup.DataReads.Set(c.State, labels, contexts.BufferpoolGroupDataReadsValues{
			Logical:  agg.DataLogicalReads,
			Physical: agg.DataPhysicalReads,
		})

		contexts.BufferpoolGroup.IndexReads.Set(c.State, labels, contexts.BufferpoolGroupIndexReadsValues{
			Logical:  agg.IndexLogicalReads,
			Physical: agg.IndexPhysicalReads,
		})

		contexts.BufferpoolGroup.Pages.Set(c.State, labels, contexts.BufferpoolGroupPagesValues{
			Used:  agg.UsedPages,
			Total: agg.TotalPages,
		})

		contexts.BufferpoolGroup.Writes.Set(c.State, labels, contexts.BufferpoolGroupWritesValues{
			Writes: agg.Writes,
		})
	}

	if overflowCount > 0 && overflow != nil {
		labels := contexts.BufferpoolGroupLabels{Group: "__other__"}

		totalReads := overflow.Hits + overflow.Misses
		overallRatio := int64(0)
		if totalReads > 0 {
			overallRatio = overflow.Hits * 100 * Precision / totalReads
		}

		dataReads := overflow.DataHits + overflow.DataMisses
		dataRatio := int64(0)
		if dataReads > 0 {
			dataRatio = overflow.DataHits * 100 * Precision / dataReads
		}

		indexReads := overflow.IndexHits + overflow.IndexMisses
		indexRatio := int64(0)
		if indexReads > 0 {
			indexRatio = overflow.IndexHits * 100 * Precision / indexReads
		}

		xdaReads := overflow.XDAHits + overflow.XDAMisses
		xdaRatio := int64(0)
		if xdaReads > 0 {
			xdaRatio = overflow.XDAHits * 100 * Precision / xdaReads
		}

		columnReads := overflow.ColumnHits + overflow.ColumnMisses
		columnRatio := int64(0)
		if columnReads > 0 {
			columnRatio = overflow.ColumnHits * 100 * Precision / columnReads
		}

		contexts.BufferpoolGroup.HitRatio.Set(c.State, labels, contexts.BufferpoolGroupHitRatioValues{
			Overall: overallRatio,
		})
		contexts.BufferpoolGroup.DetailedHitRatio.Set(c.State, labels, contexts.BufferpoolGroupDetailedHitRatioValues{
			Data:   dataRatio,
			Index:  indexRatio,
			Xda:    xdaRatio,
			Column: columnRatio,
		})
		contexts.BufferpoolGroup.Reads.Set(c.State, labels, contexts.BufferpoolGroupReadsValues{
			Logical:  overflow.LogicalReads,
			Physical: overflow.PhysicalReads,
		})
		contexts.BufferpoolGroup.DataReads.Set(c.State, labels, contexts.BufferpoolGroupDataReadsValues{
			Logical:  overflow.DataLogicalReads,
			Physical: overflow.DataPhysicalReads,
		})
		contexts.BufferpoolGroup.IndexReads.Set(c.State, labels, contexts.BufferpoolGroupIndexReadsValues{
			Logical:  overflow.IndexLogicalReads,
			Physical: overflow.IndexPhysicalReads,
		})
		contexts.BufferpoolGroup.Pages.Set(c.State, labels, contexts.BufferpoolGroupPagesValues{
			Used:  overflow.UsedPages,
			Total: overflow.TotalPages,
		})
		contexts.BufferpoolGroup.Writes.Set(c.State, labels, contexts.BufferpoolGroupWritesValues{
			Writes: overflow.Writes,
		})
	}
}
