//go:build cgo

package db2

import (
	"fmt"
	"sort"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/ibm.d/modules/db2/contexts"
)

type connectionEntry struct {
	id      string
	meta    *connectionMetrics
	metrics connectionInstanceMetrics
}

type connectionGroupAggregate struct {
	Count int64

	State             int64
	Executing         int64
	RowsRead          int64
	RowsWritten       int64
	TotalCPUTime      int64
	LockWaitTime      int64
	LogDiskWaitTime   int64
	LogBufferWaitTime int64
	PoolReadTime      int64
	PoolWriteTime     int64
	DirectReadTime    int64
	DirectWriteTime   int64
	FCMRecvWaitTime   int64
	FCMSendWaitTime   int64
	RoutineTime       int64
	CompileTime       int64
	SectionTime       int64
	CommitTime        int64
	RollbackTime      int64
}

func connectionGroupKey(meta *connectionMetrics) string {
	if meta == nil {
		return "__unknown__"
	}
	name := strings.TrimSpace(meta.applicationName)
	if name == "" || name == "-" {
		name = meta.applicationID
	}
	if name == "" {
		name = "UNKNOWN"
	}
	fields := strings.Fields(name)
	key := strings.ToUpper(fields[0])
	if key == "" {
		key = "UNKNOWN"
	}
	return key
}

func (a *connectionGroupAggregate) add(m connectionInstanceMetrics) {
	a.Count++
	a.State += m.State
	a.Executing += m.ExecutingQueries
	a.RowsRead += m.RowsRead
	a.RowsWritten += m.RowsWritten
	a.TotalCPUTime += m.TotalCPUTime
	a.LockWaitTime += m.LockWaitTime
	a.LogDiskWaitTime += m.LogDiskWaitTime
	a.LogBufferWaitTime += m.LogBufferWaitTime
	a.PoolReadTime += m.PoolReadTime
	a.PoolWriteTime += m.PoolWriteTime
	a.DirectReadTime += m.DirectReadTime
	a.DirectWriteTime += m.DirectWriteTime
	a.FCMRecvWaitTime += m.FCMRecvWaitTime
	a.FCMSendWaitTime += m.FCMSendWaitTime
	a.RoutineTime += m.TotalRoutineTime
	a.CompileTime += m.TotalCompileTime
	a.SectionTime += m.TotalSectionTime
	a.CommitTime += m.TotalCommitTime
	a.RollbackTime += m.TotalRollbackTime
}

func (c *Collector) exportConnectionMetrics() {
	entries := make([]connectionEntry, 0, len(c.mx.connections))
	for id, metrics := range c.mx.connections {
		meta := c.connections[id]
		entries = append(entries, connectionEntry{
			id:      id,
			meta:    meta,
			metrics: metrics,
		})
	}

	if len(entries) == 0 {
		c.clearWarnOnce("db2_connection_overflow")
		return
	}

	sort.Slice(entries, func(i, j int) bool {
		return entries[i].id < entries[j].id
	})

	limit := c.MaxConnections
	if limit <= 0 || limit > len(entries) {
		limit = len(entries)
	}

	groupAgg := make(map[string]*connectionGroupAggregate)
	overflowAgg := &connectionGroupAggregate{}
	overflowCount := 0
	overflowGroups := make(map[string]int)
	overflowExample := make(map[string]string)

	for idx, entry := range entries {
		key := connectionGroupKey(entry.meta)
		agg := groupAgg[key]
		if agg == nil {
			agg = &connectionGroupAggregate{}
			groupAgg[key] = agg
		}
		agg.add(entry.metrics)

		if idx < limit {
			c.emitPerConnectionMetrics(entry)
			continue
		}

		overflowAgg.add(entry.metrics)
		overflowCount++
		overflowGroups[key]++
		if _, ok := overflowExample[key]; !ok {
			overflowExample[key] = entry.id
		}
	}

	c.emitConnectionGroupMetrics(groupAgg, overflowAgg, overflowCount)

	if overflowCount > 0 {
		parts := make([]string, 0, len(overflowGroups))
		for group, count := range overflowGroups {
			parts = append(parts, fmt.Sprintf("%s:%d (e.g. %s)", group, count, overflowExample[group]))
		}
		sort.Strings(parts)
		c.warnOnce("db2_connection_overflow", "too many connections for per-connection charts (MaxConnections=%d). Aggregated %d additional connections: %s", c.MaxConnections, overflowCount, strings.Join(parts, ", "))
	} else {
		c.clearWarnOnce("db2_connection_overflow")
	}
}

func (c *Collector) emitPerConnectionMetrics(entry connectionEntry) {
	labels := contexts.ConnectionLabels{
		Application_id: entry.id,
	}

	if entry.meta != nil {
		if entry.meta.applicationName != "" && entry.meta.applicationName != "-" {
			labels.Application_name = entry.meta.applicationName
		}
		if entry.meta.clientHostname != "" && entry.meta.clientHostname != "-" {
			labels.Client_hostname = entry.meta.clientHostname
		}
		if entry.meta.clientIP != "" && entry.meta.clientIP != "-" {
			labels.Client_ip = entry.meta.clientIP
		}
		if entry.meta.clientUser != "" && entry.meta.clientUser != "-" {
			labels.Client_user = entry.meta.clientUser
		}
		if entry.meta.connectionState != "" {
			labels.State = entry.meta.connectionState
		}
	}

	contexts.Connection.State.Set(c.State, labels, contexts.ConnectionStateValues{
		State: entry.metrics.State,
	})

	contexts.Connection.Activity.Set(c.State, labels, contexts.ConnectionActivityValues{
		Read:    entry.metrics.RowsRead,
		Written: entry.metrics.RowsWritten,
	})

	contexts.Connection.WaitTime.Set(c.State, labels, contexts.ConnectionWaitTimeValues{
		Lock:         entry.metrics.LockWaitTime,
		Log_disk:     entry.metrics.LogDiskWaitTime,
		Log_buffer:   entry.metrics.LogBufferWaitTime,
		Pool_read:    entry.metrics.PoolReadTime,
		Pool_write:   entry.metrics.PoolWriteTime,
		Direct_read:  entry.metrics.DirectReadTime,
		Direct_write: entry.metrics.DirectWriteTime,
		Fcm_recv:     entry.metrics.FCMRecvWaitTime,
		Fcm_send:     entry.metrics.FCMSendWaitTime,
	})

	contexts.Connection.ProcessingTime.Set(c.State, labels, contexts.ConnectionProcessingTimeValues{
		Routine:  entry.metrics.TotalRoutineTime,
		Compile:  entry.metrics.TotalCompileTime,
		Section:  entry.metrics.TotalSectionTime,
		Commit:   entry.metrics.TotalCommitTime,
		Rollback: entry.metrics.TotalRollbackTime,
	})
}

func (c *Collector) emitConnectionGroupMetrics(groups map[string]*connectionGroupAggregate, overflow *connectionGroupAggregate, overflowCount int) {
	keys := make([]string, 0, len(groups))
	for k := range groups {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	for _, key := range keys {
		agg := groups[key]
		labels := contexts.ConnectionGroupLabels{Group: key}

		contexts.ConnectionGroup.Count.Set(c.State, labels, contexts.ConnectionGroupCountValues{
			Count: agg.Count,
		})

		contexts.ConnectionGroup.State.Set(c.State, labels, contexts.ConnectionGroupStateValues{
			State: agg.State,
		})

		contexts.ConnectionGroup.Activity.Set(c.State, labels, contexts.ConnectionGroupActivityValues{
			Read:    agg.RowsRead,
			Written: agg.RowsWritten,
		})

		contexts.ConnectionGroup.WaitTime.Set(c.State, labels, contexts.ConnectionGroupWaitTimeValues{
			Lock:         agg.LockWaitTime,
			Log_disk:     agg.LogDiskWaitTime,
			Log_buffer:   agg.LogBufferWaitTime,
			Pool_read:    agg.PoolReadTime,
			Pool_write:   agg.PoolWriteTime,
			Direct_read:  agg.DirectReadTime,
			Direct_write: agg.DirectWriteTime,
			Fcm_recv:     agg.FCMRecvWaitTime,
			Fcm_send:     agg.FCMSendWaitTime,
		})

		contexts.ConnectionGroup.ProcessingTime.Set(c.State, labels, contexts.ConnectionGroupProcessingTimeValues{
			Routine:  agg.RoutineTime,
			Compile:  agg.CompileTime,
			Section:  agg.SectionTime,
			Commit:   agg.CommitTime,
			Rollback: agg.RollbackTime,
		})
	}

	if overflowCount > 0 && overflow != nil && overflow.Count > 0 {
		labels := contexts.ConnectionGroupLabels{Group: "__other__"}

		contexts.ConnectionGroup.Count.Set(c.State, labels, contexts.ConnectionGroupCountValues{Count: overflow.Count})
		contexts.ConnectionGroup.State.Set(c.State, labels, contexts.ConnectionGroupStateValues{State: overflow.State})
		contexts.ConnectionGroup.Activity.Set(c.State, labels, contexts.ConnectionGroupActivityValues{
			Read:    overflow.RowsRead,
			Written: overflow.RowsWritten,
		})
		contexts.ConnectionGroup.WaitTime.Set(c.State, labels, contexts.ConnectionGroupWaitTimeValues{
			Lock:         overflow.LockWaitTime,
			Log_disk:     overflow.LogDiskWaitTime,
			Log_buffer:   overflow.LogBufferWaitTime,
			Pool_read:    overflow.PoolReadTime,
			Pool_write:   overflow.PoolWriteTime,
			Direct_read:  overflow.DirectReadTime,
			Direct_write: overflow.DirectWriteTime,
			Fcm_recv:     overflow.FCMRecvWaitTime,
			Fcm_send:     overflow.FCMSendWaitTime,
		})
		contexts.ConnectionGroup.ProcessingTime.Set(c.State, labels, contexts.ConnectionGroupProcessingTimeValues{
			Routine:  overflow.RoutineTime,
			Compile:  overflow.CompileTime,
			Section:  overflow.SectionTime,
			Commit:   overflow.CommitTime,
			Rollback: overflow.RollbackTime,
		})
	}
}
