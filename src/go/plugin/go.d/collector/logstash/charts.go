// SPDX-License-Identifier: GPL-3.0-or-later

package logstash

import (
	"fmt"
	"strings"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioJVMThreads = module.Priority + iota
	prioJVMMemHeapUsed
	prioJVMMemHeap
	prioJVMMemPoolsEden
	prioJVMMemPoolsSurvivor
	prioJVMMemPoolsOld
	prioJVMGCCollectorCount
	prioJVMGCCollectorTime
	prioOpenFileDescriptors
	prioEvent
	prioEventDuration
	prioPipelineEvent
	prioPipelineEventDurations
	prioUptime
)

var charts = module.Charts{
	// thread
	{
		ID:       "jvm_threads",
		Title:    "JVM Threads",
		Units:    "count",
		Fam:      "threads",
		Ctx:      "logstash.jvm_threads",
		Priority: prioJVMThreads,
		Dims: module.Dims{
			{ID: "jvm_threads_count", Name: "threads"},
		},
	},
	// memory
	{
		ID:       "jvm_mem_heap_used",
		Title:    "JVM Heap Memory Percentage",
		Units:    "percentage",
		Fam:      "memory",
		Ctx:      "logstash.jvm_mem_heap_used",
		Priority: prioJVMMemHeapUsed,
		Dims: module.Dims{
			{ID: "jvm_mem_heap_used_percent", Name: "in use"},
		},
	},
	{
		ID:       "jvm_mem_heap",
		Title:    "JVM Heap Memory",
		Units:    "KiB",
		Fam:      "memory",
		Ctx:      "logstash.jvm_mem_heap",
		Type:     module.Area,
		Priority: prioJVMMemHeap,
		Dims: module.Dims{
			{ID: "jvm_mem_heap_committed_in_bytes", Name: "committed", Div: 1024},
			{ID: "jvm_mem_heap_used_in_bytes", Name: "used", Div: 1024},
		},
	},
	{
		ID:       "jvm_mem_pools_eden",
		Title:    "JVM Pool Eden Memory",
		Units:    "KiB",
		Fam:      "memory",
		Ctx:      "logstash.jvm_mem_pools_eden",
		Type:     module.Area,
		Priority: prioJVMMemPoolsEden,
		Dims: module.Dims{
			{ID: "jvm_mem_pools_eden_committed_in_bytes", Name: "committed", Div: 1024},
			{ID: "jvm_mem_pools_eden_used_in_bytes", Name: "used", Div: 1024},
		},
	},
	{
		ID:       "jvm_mem_pools_survivor",
		Title:    "JVM Pool Survivor Memory",
		Units:    "KiB",
		Fam:      "memory",
		Ctx:      "logstash.jvm_mem_pools_survivor",
		Type:     module.Area,
		Priority: prioJVMMemPoolsSurvivor,
		Dims: module.Dims{
			{ID: "jvm_mem_pools_survivor_committed_in_bytes", Name: "committed", Div: 1024},
			{ID: "jvm_mem_pools_survivor_used_in_bytes", Name: "used", Div: 1024},
		},
	},
	{
		ID:       "jvm_mem_pools_old",
		Title:    "JVM Pool Old Memory",
		Units:    "KiB",
		Fam:      "memory",
		Ctx:      "logstash.jvm_mem_pools_old",
		Type:     module.Area,
		Priority: prioJVMMemPoolsOld,
		Dims: module.Dims{
			{ID: "jvm_mem_pools_old_committed_in_bytes", Name: "committed", Div: 1024},
			{ID: "jvm_mem_pools_old_used_in_bytes", Name: "used", Div: 1024},
		},
	},
	// garbage collection
	{
		ID:       "jvm_gc_collector_count",
		Title:    "Garbage Collection Count",
		Units:    "counts/s",
		Fam:      "garbage collection",
		Ctx:      "logstash.jvm_gc_collector_count",
		Priority: prioJVMGCCollectorCount,
		Dims: module.Dims{
			{ID: "jvm_gc_collectors_eden_collection_count", Name: "eden", Algo: module.Incremental},
			{ID: "jvm_gc_collectors_old_collection_count", Name: "old", Algo: module.Incremental},
		},
	},
	{
		ID:       "jvm_gc_collector_time",
		Title:    "Time Spent On Garbage Collection",
		Units:    "ms",
		Fam:      "garbage collection",
		Ctx:      "logstash.jvm_gc_collector_time",
		Priority: prioJVMGCCollectorTime,
		Dims: module.Dims{
			{ID: "jvm_gc_collectors_eden_collection_time_in_millis", Name: "eden", Algo: module.Incremental},
			{ID: "jvm_gc_collectors_old_collection_time_in_millis", Name: "old", Algo: module.Incremental},
		},
	},
	// processes
	{
		ID:       "open_file_descriptors",
		Title:    "Open File Descriptors",
		Units:    "fd",
		Fam:      "processes",
		Ctx:      "logstash.open_file_descriptors",
		Priority: prioOpenFileDescriptors,
		Dims: module.Dims{
			{ID: "process_open_file_descriptors", Name: "open"},
		},
	},
	// events
	{
		ID:       "event",
		Title:    "Events Overview",
		Units:    "events/s",
		Fam:      "events",
		Ctx:      "logstash.event",
		Priority: prioEvent,
		Dims: module.Dims{
			{ID: "event_in", Name: "in", Algo: module.Incremental},
			{ID: "event_filtered", Name: "filtered", Algo: module.Incremental},
			{ID: "event_out", Name: "out", Algo: module.Incremental},
		},
	},
	{
		ID:       "event_duration",
		Title:    "Events Duration",
		Units:    "seconds",
		Fam:      "events",
		Ctx:      "logstash.event_duration",
		Priority: prioEventDuration,
		Dims: module.Dims{
			{ID: "event_duration_in_millis", Name: "event", Div: 1000, Algo: module.Incremental},
			{ID: "event_queue_push_duration_in_millis", Name: "queue", Div: 1000, Algo: module.Incremental},
		},
	},
	// uptime
	{
		ID:       "uptime",
		Title:    "Uptime",
		Units:    "seconds",
		Fam:      "uptime",
		Ctx:      "logstash.uptime",
		Priority: prioUptime,
		Dims: module.Dims{
			{ID: "jvm_uptime_in_millis", Name: "uptime", Div: 1000},
		},
	},
}

var pipelineChartsTmpl = module.Charts{
	{
		ID:       "pipeline_%s_event",
		Title:    "Pipeline Events",
		Units:    "events/s",
		Fam:      "pipeline events",
		Ctx:      "logstash.pipeline_event",
		Priority: prioPipelineEvent,
		Dims: module.Dims{
			{ID: "pipelines_%s_event_in", Name: "in", Algo: module.Incremental},
			{ID: "pipelines_%s_event_filtered", Name: "filtered", Algo: module.Incremental},
			{ID: "pipelines_%s_event_out", Name: "out", Algo: module.Incremental},
		},
	},
	{
		ID:       "pipeline_%s_event_duration",
		Title:    "Pipeline Events Duration",
		Units:    "seconds",
		Fam:      "pipeline events duration",
		Ctx:      "logstash.pipeline_event_duration",
		Priority: prioPipelineEventDurations,
		Dims: module.Dims{
			{ID: "pipelines_%s_event_duration_in_millis", Name: "event", Div: 1000, Algo: module.Incremental},
			{ID: "pipelines_%s_event_queue_push_duration_in_millis", Name: "queue", Div: 1000, Algo: module.Incremental},
		},
	},
}

func (c *Collector) addPipelineCharts(id string) {
	charts := pipelineChartsTmpl.Copy()

	for _, chart := range *charts {
		chart.ID = fmt.Sprintf(chart.ID, id)
		chart.Labels = []module.Label{
			{Key: "pipeline", Value: id},
		}
		for _, dim := range chart.Dims {
			dim.ID = fmt.Sprintf(dim.ID, id)
		}
	}

	if err := c.Charts().Add(*charts...); err != nil {
		c.Warning(err)
	}
}

func (c *Collector) removePipelineCharts(id string) {
	for _, chart := range *c.Charts() {
		if strings.HasPrefix(chart.ID, "pipeline_"+id) {
			chart.MarkRemove()
			chart.MarkNotCreated()
		}
	}
}
