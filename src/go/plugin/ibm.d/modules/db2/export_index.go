//go:build cgo

package db2

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

type indexEntry struct {
	name    string
	metrics indexInstanceMetrics
}

type indexGroupAggregate struct {
	LeafNodes  int64
	IndexScans int64
	FullScans  int64
}

func indexGroupKey(name string) string {
	if idx := strings.Index(name, "."); idx > 0 {
		return name[:idx]
	}
	return name
}

func (a *indexGroupAggregate) add(m indexInstanceMetrics) {
	a.LeafNodes += m.LeafNodes
	a.IndexScans += m.IndexScans
	a.FullScans += m.FullScans
}

func (c *Collector) exportIndexMetrics() {
	entries := make([]indexEntry, 0, len(c.mx.indexes))
	for name, metrics := range c.mx.indexes {
		entries = append(entries, indexEntry{name: name, metrics: metrics})
	}

	if len(entries) == 0 {
		c.clearWarnOnce("db2_index_overflow")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	limit := c.MaxIndexes
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	groupAgg := make(map[string]*indexGroupAggregate)
	overflowAgg := &indexGroupAggregate{}
	overflowCount := 0
	overflowGroups := make(map[string]int)
	overflowExample := make(map[string]string)

	for idx, entry := range entries {
		key := indexGroupKey(entry.name)
		agg := groupAgg[key]
		if agg == nil {
			agg = &indexGroupAggregate{}
			groupAgg[key] = agg
		}
		agg.add(entry.metrics)

		if idx < limit {
			labels := contexts.IndexLabels{Index: entry.name}
			contexts.Index.Usage.Set(c.State, labels, contexts.IndexUsageValues{
				Index: entry.metrics.IndexScans,
				Full:  entry.metrics.FullScans,
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

	c.emitIndexGroupMetrics(groupAgg, overflowAgg, overflowCount)

	if overflowCount > 0 {
		parts := make([]string, 0, len(overflowGroups))
		for group, count := range overflowGroups {
			parts = append(parts, fmt.Sprintf("%s:%d (e.g. %s)", group, count, overflowExample[group]))
		}
		sort.Strings(parts)
		c.warnOnce("db2_index_overflow", "too many indexes for per-instance charts (MaxIndexes=%d). Aggregated %d additional indexes: %s", c.MaxIndexes, overflowCount, strings.Join(parts, ", "))
	} else {
		c.clearWarnOnce("db2_index_overflow")
	}
}

func (c *Collector) emitIndexGroupMetrics(groups map[string]*indexGroupAggregate, overflow *indexGroupAggregate, overflowCount int) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		agg := groups[key]
		labels := contexts.IndexGroupLabels{Group: key}

		contexts.IndexGroup.Usage.Set(c.State, labels, contexts.IndexGroupUsageValues{
			Index: agg.IndexScans,
			Full:  agg.FullScans,
		})
	}

	if overflowCount > 0 && overflow != nil {
		labels := contexts.IndexGroupLabels{Group: "__other__"}
		contexts.IndexGroup.Usage.Set(c.State, labels, contexts.IndexGroupUsageValues{
			Index: overflow.IndexScans,
			Full:  overflow.FullScans,
		})
	}
}
