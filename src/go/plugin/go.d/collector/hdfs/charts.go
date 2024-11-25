// SPDX-License-Identifier: GPL-3.0-or-later

package hdfs

import "github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"

type (
	Charts = module.Charts
	Dims   = module.Dims
	Vars   = module.Vars
)

var jvmCharts = Charts{
	{
		ID:    "jvm_heap_memory",
		Title: "Heap Memory",
		Units: "MiB",
		Fam:   "jvm",
		Ctx:   "hdfs.heap_memory",
		Type:  module.Area,
		Dims: Dims{
			{ID: "jvm_mem_heap_committed", Name: "committed", Div: 1000},
			{ID: "jvm_mem_heap_used", Name: "used", Div: 1000},
		},
		Vars: Vars{
			{ID: "jvm_mem_heap_max"},
		},
	},
	{
		ID:    "jvm_gc_count_total",
		Title: "GC Events",
		Units: "events/s",
		Fam:   "jvm",
		Ctx:   "hdfs.gc_count_total",
		Dims: Dims{
			{ID: "jvm_gc_count", Name: "gc", Algo: module.Incremental},
		},
	},
	{
		ID:    "jvm_gc_time_total",
		Title: "GC Time",
		Units: "ms",
		Fam:   "jvm",
		Ctx:   "hdfs.gc_time_total",
		Dims: Dims{
			{ID: "jvm_gc_time_millis", Name: "time", Algo: module.Incremental},
		},
	},
	{
		ID:    "jvm_gc_threshold",
		Title: "Number of Times That the GC Threshold is Exceeded",
		Units: "events/s",
		Fam:   "jvm",
		Ctx:   "hdfs.gc_threshold",
		Dims: Dims{
			{ID: "jvm_gc_num_info_threshold_exceeded", Name: "info", Algo: module.Incremental},
			{ID: "jvm_gc_num_warn_threshold_exceeded", Name: "warn", Algo: module.Incremental},
		},
	},
	{
		ID:    "jvm_threads",
		Title: "Number of Threads",
		Units: "num",
		Fam:   "jvm",
		Ctx:   "hdfs.threads",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "jvm_threads_new", Name: "new"},
			{ID: "jvm_threads_runnable", Name: "runnable"},
			{ID: "jvm_threads_blocked", Name: "blocked"},
			{ID: "jvm_threads_waiting", Name: "waiting"},
			{ID: "jvm_threads_timed_waiting", Name: "timed_waiting"},
			{ID: "jvm_threads_terminated", Name: "terminated"},
		},
	},
	{
		ID:    "jvm_logs_total",
		Title: "Number of Logs",
		Units: "logs/s",
		Fam:   "jvm",
		Ctx:   "hdfs.logs_total",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "jvm_log_info", Name: "info", Algo: module.Incremental},
			{ID: "jvm_log_error", Name: "error", Algo: module.Incremental},
			{ID: "jvm_log_warn", Name: "warn", Algo: module.Incremental},
			{ID: "jvm_log_fatal", Name: "fatal", Algo: module.Incremental},
		},
	},
}

var rpcActivityCharts = Charts{
	{
		ID:    "rpc_bandwidth",
		Title: "RPC Bandwidth",
		Units: "kilobits/s",
		Fam:   "rpc",
		Ctx:   "hdfs.rpc_bandwidth",
		Type:  module.Area,
		Dims: Dims{
			{ID: "rpc_received_bytes", Name: "received", Div: 1000, Algo: module.Incremental},
			{ID: "rpc_sent_bytes", Name: "sent", Div: -1000, Algo: module.Incremental},
		},
	},
	{
		ID:    "rpc_calls",
		Title: "RPC Calls",
		Units: "calls/s",
		Fam:   "rpc",
		Ctx:   "hdfs.rpc_calls",
		Dims: Dims{
			{ID: "rpc_queue_time_num_ops", Name: "calls", Algo: module.Incremental},
		},
	},
	{
		ID:    "rpc_open_connections",
		Title: "RPC Open Connections",
		Units: "connections",
		Fam:   "rpc",
		Ctx:   "hdfs.open_connections",
		Dims: Dims{
			{ID: "rpc_num_open_connections", Name: "open"},
		},
	},
	{
		ID:    "rpc_call_queue_length",
		Title: "RPC Call Queue Length",
		Units: "num",
		Fam:   "rpc",
		Ctx:   "hdfs.call_queue_length",
		Dims: Dims{
			{ID: "rpc_call_queue_length", Name: "length"},
		},
	},
	{
		ID:    "rpc_avg_queue_time",
		Title: "RPC Avg Queue Time",
		Units: "ms",
		Fam:   "rpc",
		Ctx:   "hdfs.avg_queue_time",
		Dims: Dims{
			{ID: "rpc_queue_time_avg_time", Name: "time", Div: 1000},
		},
	},
	{
		ID:    "rpc_avg_processing_time",
		Title: "RPC Avg Processing Time",
		Units: "ms",
		Fam:   "rpc",
		Ctx:   "hdfs.avg_processing_time",
		Dims: Dims{
			{ID: "rpc_processing_time_avg_time", Name: "time", Div: 1000},
		},
	},
}

var fsNameSystemCharts = Charts{
	{
		ID:    "fs_name_system_capacity",
		Title: "Capacity Across All Datanodes",
		Units: "KiB",
		Fam:   "fs name system",
		Ctx:   "hdfs.capacity",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "fsns_capacity_remaining", Name: "remaining", Div: 1024},
			{ID: "fsns_capacity_used", Name: "used", Div: 1024},
		},
		Vars: Vars{
			{ID: "fsns_capacity_total"},
		},
	},
	{
		ID:    "fs_name_system_used_capacity",
		Title: "Used Capacity Across All Datanodes",
		Units: "KiB",
		Fam:   "fs name system",
		Ctx:   "hdfs.used_capacity",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "fsns_capacity_used_dfs", Name: "dfs", Div: 1024},
			{ID: "fsns_capacity_used_non_dfs", Name: "non_dfs", Div: 1024},
		},
	},
	{
		ID:    "fs_name_system_load",
		Title: "Number of Concurrent File Accesses (read/write) Across All DataNodes",
		Units: "load",
		Fam:   "fs name system",
		Ctx:   "hdfs.load",
		Dims: Dims{
			{ID: "fsns_total_load", Name: "load"},
		},
	},
	{
		ID:    "fs_name_system_volume_failures_total",
		Title: "Number of Volume Failures Across All Datanodes",
		Units: "events/s",
		Fam:   "fs name system",
		Ctx:   "hdfs.volume_failures_total",
		Dims: Dims{
			{ID: "fsns_volume_failures_total", Name: "failures", Algo: module.Incremental},
		},
	},
	{
		ID:    "fs_files_total",
		Title: "Number of Tracked Files",
		Units: "num",
		Fam:   "fs name system",
		Ctx:   "hdfs.files_total",
		Dims: Dims{
			{ID: "fsns_files_total", Name: "files"},
		},
	},
	{
		ID:    "fs_blocks_total",
		Title: "Number of Allocated Blocks in the System",
		Units: "num",
		Fam:   "fs name system",
		Ctx:   "hdfs.blocks_total",
		Dims: Dims{
			{ID: "fsns_blocks_total", Name: "blocks"},
		},
	},
	{
		ID:    "fs_problem_blocks",
		Title: "Number of Problem Blocks (can point to an unhealthy cluster)",
		Units: "num",
		Fam:   "fs name system",
		Ctx:   "hdfs.blocks",
		Dims: Dims{
			{ID: "fsns_corrupt_blocks", Name: "corrupt"},
			{ID: "fsns_missing_blocks", Name: "missing"},
			{ID: "fsns_under_replicated_blocks", Name: "under_replicated"},
		},
	},
	{
		ID:    "fs_name_system_data_nodes",
		Title: "Number of Data Nodes By Status",
		Units: "num",
		Fam:   "fs name system",
		Ctx:   "hdfs.data_nodes",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "fsns_num_live_data_nodes", Name: "live"},
			{ID: "fsns_num_dead_data_nodes", Name: "dead"},
			{ID: "fsns_stale_data_nodes", Name: "stale"},
		},
	},
}

var fsDatasetStateCharts = Charts{
	{
		ID:    "fs_dataset_state_capacity",
		Title: "Capacity",
		Units: "KiB",
		Fam:   "fs dataset",
		Ctx:   "hdfs.datanode_capacity",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "fsds_capacity_remaining", Name: "remaining", Div: 1024},
			{ID: "fsds_capacity_used", Name: "used", Div: 1024},
		},
		Vars: Vars{
			{ID: "fsds_capacity_total"},
		},
	},
	{
		ID:    "fs_dataset_state_used_capacity",
		Title: "Used Capacity",
		Units: "KiB",
		Fam:   "fs dataset",
		Ctx:   "hdfs.datanode_used_capacity",
		Type:  module.Stacked,
		Dims: Dims{
			{ID: "fsds_capacity_used_dfs", Name: "dfs", Div: 1024},
			{ID: "fsds_capacity_used_non_dfs", Name: "non_dfs", Div: 1024},
		},
	},
	{
		ID:    "fs_dataset_state_num_failed_volumes",
		Title: "Number of Failed Volumes",
		Units: "num",
		Fam:   "fs dataset",
		Ctx:   "hdfs.datanode_failed_volumes",
		Dims: Dims{
			{ID: "fsds_num_failed_volumes", Name: "failed volumes"},
		},
	},
}

var fsDataNodeActivityCharts = Charts{
	{
		ID:    "dna_bandwidth",
		Title: "Bandwidth",
		Units: "KiB/s",
		Fam:   "activity",
		Ctx:   "hdfs.datanode_bandwidth",
		Type:  module.Area,
		Dims: Dims{
			{ID: "dna_bytes_read", Name: "reads", Div: 1024, Algo: module.Incremental},
			{ID: "dna_bytes_written", Name: "writes", Div: -1024, Algo: module.Incremental},
		},
	},
}

func dataNodeCharts() *Charts {
	charts := Charts{}
	panicIfError(charts.Add(*jvmCharts.Copy()...))
	panicIfError(charts.Add(*rpcActivityCharts.Copy()...))
	panicIfError(charts.Add(*fsDatasetStateCharts.Copy()...))
	panicIfError(charts.Add(*fsDataNodeActivityCharts.Copy()...))
	return &charts
}

func nameNodeCharts() *Charts {
	charts := Charts{}
	panicIfError(charts.Add(*jvmCharts.Copy()...))
	panicIfError(charts.Add(*rpcActivityCharts.Copy()...))
	panicIfError(charts.Add(*fsNameSystemCharts.Copy()...))
	return &charts
}

func panicIfError(err error) {
	if err != nil {
		panic(err)
	}
}
