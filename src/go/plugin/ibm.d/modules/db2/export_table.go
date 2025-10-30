//go:build cgo

package db2

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

type tableEntry struct {
	name    string
	metrics tableInstanceMetrics
}

type tableGroupAggregate struct {
	DataSize    int64
	IndexSize   int64
	LongObjSize int64
	RowsRead    int64
	RowsWritten int64
	Count       int64
}

func tableGroupKey(name string) string {
	if idx := strings.Index(name, "."); idx > 0 {
		return name[:idx]
	}
	return name
}

func (a *tableGroupAggregate) add(m tableInstanceMetrics) {
	a.Count++
	a.DataSize += m.DataSize
	a.IndexSize += m.IndexSize
	a.LongObjSize += m.LongObjSize
	a.RowsRead += m.RowsRead
	a.RowsWritten += m.RowsWritten
}

func (c *Collector) exportTableMetrics() {
	entries := make([]tableEntry, 0, len(c.mx.tables))
	for name, metrics := range c.mx.tables {
		entries = append(entries, tableEntry{name: name, metrics: metrics})
	}

	if len(entries) == 0 {
		c.clearWarnOnce("db2_table_overflow")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	limit := c.MaxTables
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	groupAgg := make(map[string]*tableGroupAggregate)
	overflowAgg := &tableGroupAggregate{}
	overflowCount := 0
	overflowGroups := make(map[string]int)
	overflowExample := make(map[string]string)

	for idx, entry := range entries {
		key := tableGroupKey(entry.name)
		agg := groupAgg[key]
		if agg == nil {
			agg = &tableGroupAggregate{}
			groupAgg[key] = agg
		}
		agg.add(entry.metrics)

		if idx < limit {
			labels := contexts.TableLabels{Table: entry.name}
			contexts.Table.Size.Set(c.State, labels, contexts.TableSizeValues{
				Data:     entry.metrics.DataSize,
				Index:    entry.metrics.IndexSize,
				Long_obj: entry.metrics.LongObjSize,
			})
			contexts.Table.Activity.Set(c.State, labels, contexts.TableActivityValues{
				Read:    entry.metrics.RowsRead,
				Written: entry.metrics.RowsWritten,
			})
			continue
		}

		overflowAgg.add(entry.metrics)
		overflowCount++
		overflowGroups[key]++
		if _, ok := overflowExample[key]; !ok {
			overflowExample[key] = entry.name
		}
	}

	c.emitTableGroupMetrics(groupAgg, overflowAgg, overflowCount)

	if overflowCount > 0 {
		parts := make([]string, 0, len(overflowGroups))
		for group, count := range overflowGroups {
			parts = append(parts, fmt.Sprintf("%s:%d (e.g. %s)", group, count, overflowExample[group]))
		}
		sort.Strings(parts)
		c.warnOnce("db2_table_overflow", "too many tables for per-instance charts (MaxTables=%d). Aggregated %d additional tables: %s", c.MaxTables, overflowCount, strings.Join(parts, ", "))
	} else {
		c.clearWarnOnce("db2_table_overflow")
	}
}

func (c *Collector) emitTableGroupMetrics(groups map[string]*tableGroupAggregate, overflow *tableGroupAggregate, overflowCount int) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		agg := groups[key]
		labels := contexts.TableGroupLabels{Group: key}

		contexts.TableGroup.Size.Set(c.State, labels, contexts.TableGroupSizeValues{
			Data:     agg.DataSize,
			Index:    agg.IndexSize,
			Long_obj: agg.LongObjSize,
		})

		contexts.TableGroup.Activity.Set(c.State, labels, contexts.TableGroupActivityValues{
			Read:    agg.RowsRead,
			Written: agg.RowsWritten,
		})
	}

	if overflowCount > 0 && overflow != nil {
		labels := contexts.TableGroupLabels{Group: "__other__"}
		contexts.TableGroup.Size.Set(c.State, labels, contexts.TableGroupSizeValues{
			Data:     overflow.DataSize,
			Index:    overflow.IndexSize,
			Long_obj: overflow.LongObjSize,
		})
		contexts.TableGroup.Activity.Set(c.State, labels, contexts.TableGroupActivityValues{
			Read:    overflow.RowsRead,
			Written: overflow.RowsWritten,
		})
	}
}
