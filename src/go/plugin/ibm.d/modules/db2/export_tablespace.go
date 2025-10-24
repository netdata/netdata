//go:build cgo

package db2

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

type tablespaceEntry struct {
	name    string
	meta    *tablespaceMetrics
	metrics tablespaceInstanceMetrics
}

type tablespaceGroupAggregate struct {
	UsedSize   int64
	FreeSize   int64
	TotalSize  int64
	UsableSize int64
	State      int64
}

func tablespaceGroupKey(meta *tablespaceMetrics) string {
	if meta == nil {
		return "UNKNOWN"
	}
	parts := []string{}
	if meta.tbspType != "" {
		parts = append(parts, strings.ToUpper(meta.tbspType))
	}
	if meta.contentType != "" {
		parts = append(parts, strings.ToUpper(meta.contentType))
	}
	if len(parts) == 0 {
		return "UNKNOWN"
	}
	return strings.Join(parts, "/")
}

func (a *tablespaceGroupAggregate) add(m tablespaceInstanceMetrics) {
	a.UsedSize += m.UsedSize
	a.FreeSize += m.FreeSize
	a.TotalSize += m.TotalSize
	a.UsableSize += m.UsableSize
	a.State += m.State
}

func (c *Collector) exportTablespaceMetrics() {
	entries := make([]tablespaceEntry, 0, len(c.mx.tablespaces))
	for name, metrics := range c.mx.tablespaces {
		entries = append(entries, tablespaceEntry{
			name:    name,
			meta:    c.tablespaces[name],
			metrics: metrics,
		})
	}

	if len(entries) == 0 {
		c.clearWarnOnce("db2_tablespace_overflow")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].name < entries[j].name
	})

	limit := c.MaxTablespaces
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	groupAgg := make(map[string]*tablespaceGroupAggregate)
	overflowAgg := &tablespaceGroupAggregate{}
	overflowCount := 0
	overflowGroups := make(map[string]int)
	overflowExample := make(map[string]string)

	for idx, entry := range entries {
		key := tablespaceGroupKey(entry.meta)
		agg := groupAgg[key]
		if agg == nil {
			agg = &tablespaceGroupAggregate{}
			groupAgg[key] = agg
		}
		agg.add(entry.metrics)

		if idx < limit {
			c.emitPerTablespaceMetrics(entry)
			continue
		}

		overflowAgg.add(entry.metrics)
		overflowCount++
		overflowGroups[key]++
		if _, ok := overflowExample[key]; !ok {
			overflowExample[key] = entry.name
		}
	}

	c.emitTablespaceGroupMetrics(groupAgg, overflowAgg, overflowCount)

	if overflowCount > 0 {
		parts := make([]string, 0, len(overflowGroups))
		for group, count := range overflowGroups {
			parts = append(parts, fmt.Sprintf("%s:%d (e.g. %s)", group, count, overflowExample[group]))
		}
		sort.Strings(parts)
		c.warnOnce("db2_tablespace_overflow", "too many tablespaces for per-instance charts (MaxTablespaces=%d). Aggregated %d additional tablespaces: %s", c.MaxTablespaces, overflowCount, strings.Join(parts, ", "))
	} else {
		c.clearWarnOnce("db2_tablespace_overflow")
	}
}

func (c *Collector) emitPerTablespaceMetrics(entry tablespaceEntry) {
	tspType := "unknown"
	contentType := "unknown"
	stateLabel := "unknown"
	if entry.meta != nil {
		if entry.meta.tbspType != "" {
			tspType = entry.meta.tbspType
		}
		if entry.meta.contentType != "" {
			contentType = entry.meta.contentType
		}
		if entry.meta.state != "" {
			stateLabel = entry.meta.state
		}
	}

	labels := contexts.TablespaceLabels{
		Tablespace:   entry.name,
		Type:         tspType,
		Content_type: contentType,
		State:        stateLabel,
	}

	contexts.Tablespace.Usage.Set(c.State, labels, contexts.TablespaceUsageValues{
		Used: entry.metrics.UsedPercent,
	})

	contexts.Tablespace.Size.Set(c.State, labels, contexts.TablespaceSizeValues{
		Used: entry.metrics.UsedSize,
		Free: entry.metrics.FreeSize,
	})

	contexts.Tablespace.UsableSize.Set(c.State, labels, contexts.TablespaceUsableSizeValues{
		Total:  entry.metrics.TotalSize,
		Usable: entry.metrics.UsableSize,
	})

	contexts.Tablespace.State.Set(c.State, labels, contexts.TablespaceStateValues{
		State: entry.metrics.State,
	})
}

func (c *Collector) emitTablespaceGroupMetrics(groups map[string]*tablespaceGroupAggregate, overflow *tablespaceGroupAggregate, overflowCount int) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		agg := groups[key]
		labels := contexts.TablespaceGroupLabels{Group: key}

		usedPercent := int64(0)
		if agg.TotalSize > 0 {
			usedPercent = agg.UsedSize * 100 * Precision / agg.TotalSize
		}
		contexts.TablespaceGroup.Usage.Set(c.State, labels, contexts.TablespaceGroupUsageValues{
			Used: usedPercent,
		})

		contexts.TablespaceGroup.Size.Set(c.State, labels, contexts.TablespaceGroupSizeValues{
			Used: agg.UsedSize,
			Free: agg.FreeSize,
		})

		contexts.TablespaceGroup.UsableSize.Set(c.State, labels, contexts.TablespaceGroupUsableSizeValues{
			Total:  agg.TotalSize,
			Usable: agg.UsableSize,
		})

		contexts.TablespaceGroup.State.Set(c.State, labels, contexts.TablespaceGroupStateValues{
			State: agg.State,
		})
	}

	if overflowCount > 0 && overflow != nil {
		labels := contexts.TablespaceGroupLabels{Group: "__other__"}
		usedPercent := int64(0)
		if overflow.TotalSize > 0 {
			usedPercent = overflow.UsedSize * 100 * Precision / overflow.TotalSize
		}
		contexts.TablespaceGroup.Usage.Set(c.State, labels, contexts.TablespaceGroupUsageValues{Used: usedPercent})
		contexts.TablespaceGroup.Size.Set(c.State, labels, contexts.TablespaceGroupSizeValues{Used: overflow.UsedSize, Free: overflow.FreeSize})
		contexts.TablespaceGroup.UsableSize.Set(c.State, labels, contexts.TablespaceGroupUsableSizeValues{Total: overflow.TotalSize, Usable: overflow.UsableSize})
		contexts.TablespaceGroup.State.Set(c.State, labels, contexts.TablespaceGroupStateValues{State: overflow.State})
	}
}
