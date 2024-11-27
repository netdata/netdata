// SPDX-License-Identifier: GPL-3.0-or-later

package riakkv

import (
	"slices"

	"github.com/netdata/netdata/go/plugins/plugin/go.d/agent/module"
)

const (
	prioKvNodeOperations = module.Priority + iota
	prioDtVnodeUpdates
	prioSearchQueries
	prioSearchDocuments
	prioConsistentOperations

	prioKvLatencyGet
	prioKvLatencyPut
	prioDtLatencyCounter
	prioDtLatencySet
	prioDtLatencyMap
	prioSearchLatencyQuery
	prioSearchLatencyIndex
	prioConsistentLatencyGet
	prioConsistentLatencyPut

	prioVmProcessesCount
	prioVmProcessesMemory

	prioKvSiblingsEncounteredGet
	prioKvObjSizeGet
	prioSearchVnodeqSize
	prioSearchIndexErrors
	prioCorePbc
	prioCoreRepairs
	prioCoreFsmActive
	prioCoreFsmREjected
)

var charts = module.Charts{
	kvNodeOperationsChart.Copy(),
	dtVnodeUpdatesChart.Copy(),
	searchQueriesChart.Copy(),
	searchDocumentsChart.Copy(),
	consistentOperationsChart.Copy(),

	kvLatencyGetChart.Copy(),
	kvLatencyPutChart.Copy(),
	dtLatencyCounterChart.Copy(),
	dtLatencySetChart.Copy(),
	dtLatencyMapChart.Copy(),
	searchLatencyQueryChart.Copy(),
	searchLatencyIndexChart.Copy(),
	consistentLatencyGetChart.Copy(),
	consistentLatencyPutChart.Copy(),

	vmProcessesCountChart.Copy(),
	vmProcessesMemoryChart.Copy(),

	kvSiblingsEncounteredGetChart.Copy(),
	kvObjectSizeGetChart.Copy(),
	searchVnodeqSizeChart.Copy(),
	searchIndexErrorsChart.Copy(),
	corePbsChart.Copy(),
	coreRepairsChart.Copy(),
	coreFsmActiveChart.Copy(),
	coreFsmRejectedChart.Copy(),
}

/*
Throughput metrics
https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#throughput-metrics

Collected in totals
*/
var (
	kvNodeOperationsChart = module.Chart{
		ID:       "kv_node_operations",
		Title:    "Reads & writes coordinated by this node",
		Units:    "operations/s",
		Fam:      "throughput",
		Ctx:      "riak.kv.throughput",
		Priority: prioKvNodeOperations,
		Dims: module.Dims{
			{ID: "node_gets_total", Name: "gets", Algo: module.Incremental},
			{ID: "node_puts_total", Name: "puts", Algo: module.Incremental},
		},
	}
	dtVnodeUpdatesChart = module.Chart{
		ID:       "dt_vnode_updates",
		Title:    "Update operations coordinated by local vnodes by data type",
		Units:    "operations/s",
		Fam:      "throughput",
		Ctx:      "riak.dt.vnode_updates",
		Priority: prioDtVnodeUpdates,
		Dims: module.Dims{
			{ID: "vnode_counter_update_total", Name: "counters", Algo: module.Incremental},
			{ID: "vnode_set_update_total", Name: "sets", Algo: module.Incremental},
			{ID: "vnode_map_update_total", Name: "maps", Algo: module.Incremental},
		},
	}
	searchQueriesChart = module.Chart{
		ID:       "dt_vnode_updates",
		Title:    "Search queries on the node",
		Units:    "queries/s",
		Fam:      "throughput",
		Ctx:      "riak.search",
		Priority: prioSearchQueries,
		Dims: module.Dims{
			{ID: "search_query_throughput_count", Name: "queries", Algo: module.Incremental},
		},
	}
	searchDocumentsChart = module.Chart{
		ID:       "search_documents",
		Title:    "Documents indexed by search",
		Units:    "documents/s",
		Fam:      "throughput",
		Ctx:      "riak.search.documents",
		Priority: prioSearchDocuments,
		Dims: module.Dims{
			{ID: "search_index_throughput_count", Name: "indexed", Algo: module.Incremental},
		},
	}
	consistentOperationsChart = module.Chart{
		ID:       "consistent_operations",
		Title:    "Consistent node operations",
		Units:    "operations/s",
		Fam:      "throughput",
		Ctx:      "riak.consistent.operations",
		Priority: prioConsistentOperations,
		Dims: module.Dims{
			{ID: "consistent_gets_total", Name: "gets", Algo: module.Incremental},
			{ID: "consistent_puts_total", Name: "puts", Algo: module.Incremental},
		},
	}
)

/*
Latency metrics
https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#throughput-metrics

Collected for the past minute in milliseconds and
returned from Riak in microseconds.
*/
var (
	kvLatencyGetChart = module.Chart{
		ID:       "kv_latency_get",
		Title:    "Time between reception of a client GET request and subsequent response to client",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.kv.latency.get",
		Priority: prioKvLatencyGet,
		Dims: module.Dims{
			{ID: "node_get_fsm_time_mean", Name: "mean", Div: 1000},
			{ID: "node_get_fsm_time_median", Name: "median", Div: 1000},
			{ID: "node_get_fsm_time_95", Name: "95", Div: 1000},
			{ID: "node_get_fsm_time_99", Name: "99", Div: 1000},
			{ID: "node_get_fsm_time_100", Name: "100", Div: 1000},
		},
	}
	kvLatencyPutChart = module.Chart{
		ID:       "kv_latency_put",
		Title:    "Time between reception of a client PUT request and subsequent response to client",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.kv.latency.put",
		Priority: prioKvLatencyPut,
		Dims: module.Dims{
			{ID: "node_put_fsm_time_mean", Name: "mean", Div: 1000},
			{ID: "node_put_fsm_time_median", Name: "median", Div: 1000},
			{ID: "node_put_fsm_time_95", Name: "95", Div: 1000},
			{ID: "node_put_fsm_time_99", Name: "99", Div: 1000},
			{ID: "node_put_fsm_time_100", Name: "100", Div: 1000},
		},
	}
	dtLatencyCounterChart = module.Chart{
		ID:       "dt_latency_counter",
		Title:    "Time it takes to perform an Update Counter operation",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.dt.latency.counter_merge",
		Priority: prioDtLatencyCounter,
		Dims: module.Dims{
			{ID: "object_counter_merge_time_mean", Name: "mean", Div: 1000},
			{ID: "object_counter_merge_time_median", Name: "median", Div: 1000},
			{ID: "object_counter_merge_time_95", Name: "95", Div: 1000},
			{ID: "object_counter_merge_time_99", Name: "99", Div: 1000},
			{ID: "object_counter_merge_time_100", Name: "100", Div: 1000},
		},
	}
	dtLatencySetChart = module.Chart{
		ID:       "dt_latency_counter",
		Title:    "Time it takes to perform an Update Set operation",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.dt.latency.set_merge",
		Priority: prioDtLatencySet,
		Dims: module.Dims{
			{ID: "object_set_merge_time_mean", Name: "mean", Div: 1000},
			{ID: "object_set_merge_time_median", Name: "median", Div: 1000},
			{ID: "object_set_merge_time_95", Name: "95", Div: 1000},
			{ID: "object_set_merge_time_99", Name: "99", Div: 1000},
			{ID: "object_set_merge_time_100", Name: "100", Div: 1000},
		},
	}
	dtLatencyMapChart = module.Chart{
		ID:       "dt_latency_map",
		Title:    "Time it takes to perform an Update Map operation",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.dt.latency.map_merge",
		Priority: prioDtLatencyMap,
		Dims: module.Dims{
			{ID: "object_map_merge_time_mean", Name: "mean", Div: 1000},
			{ID: "object_map_merge_time_median", Name: "median", Div: 1000},
			{ID: "object_map_merge_time_95", Name: "95", Div: 1000},
			{ID: "object_map_merge_time_99", Name: "99", Div: 1000},
			{ID: "object_map_merge_time_100", Name: "100", Div: 1000},
		},
	}
	searchLatencyQueryChart = module.Chart{
		ID:       "search_latency_query",
		Title:    "Search query latency",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.search.latency.query",
		Priority: prioSearchLatencyQuery,
		Dims: module.Dims{
			{ID: "search_query_latency_median", Name: "median", Div: 1000},
			{ID: "search_query_latency_min", Name: "min", Div: 1000},
			{ID: "search_query_latency_95", Name: "95", Div: 1000},
			{ID: "search_query_latency_99", Name: "99", Div: 1000},
			{ID: "search_query_latency_999", Name: "999", Div: 1000},
			{ID: "search_query_latency_max", Name: "max", Div: 1000},
		},
	}
	searchLatencyIndexChart = module.Chart{
		ID:       "search_latency_index",
		Title:    "Time it takes Search to index a new document",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.search.latency.index",
		Priority: prioSearchLatencyIndex,
		Dims: module.Dims{
			{ID: "search_index_latency_median", Name: "median", Div: 1000},
			{ID: "search_index_latency_min", Name: "min", Div: 1000},
			{ID: "search_index_latency_95", Name: "95", Div: 1000},
			{ID: "search_index_latency_99", Name: "99", Div: 1000},
			{ID: "search_index_latency_999", Name: "999", Div: 1000},
			{ID: "search_index_latency_max", Name: "max", Div: 1000},
		},
	}
	consistentLatencyGetChart = module.Chart{
		ID:       "consistent_latency_get",
		Title:    "Strongly consistent read latency",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.consistent.latency.get",
		Priority: prioConsistentLatencyGet,
		Dims: module.Dims{
			{ID: "consistent_get_time_mean", Name: "mean", Div: 1000},
			{ID: "consistent_get_time_median", Name: "median", Div: 1000},
			{ID: "consistent_get_time_95", Name: "95", Div: 1000},
			{ID: "consistent_get_time_99", Name: "99", Div: 1000},
			{ID: "consistent_get_time_100", Name: "100", Div: 1000},
		},
	}
	consistentLatencyPutChart = module.Chart{
		ID:       "consistent_latency_put",
		Title:    "Strongly consistent write latency",
		Units:    "ms",
		Fam:      "latency",
		Ctx:      "riak.consistent.latency.put",
		Priority: prioConsistentLatencyPut,
		Dims: module.Dims{
			{ID: "consistent_put_time_mean", Name: "mean", Div: 1000},
			{ID: "consistent_put_time_median", Name: "median", Div: 1000},
			{ID: "consistent_put_time_95", Name: "95", Div: 1000},
			{ID: "consistent_put_time_99", Name: "99", Div: 1000},
			{ID: "consistent_put_time_100", Name: "100", Div: 1000},
		},
	}
)

/*
Erlang's resource usage metrics
https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#erlang-resource-usage-metrics

Processes collected as a gauge.
Memory collected as Megabytes, returned as bytes from Riak.
*/
var (
	vmProcessesCountChart = module.Chart{
		ID:       "vm_processes",
		Title:    "Total processes running in the Erlang VM",
		Units:    "processes",
		Fam:      "vm",
		Ctx:      "riak.vm.processes.count",
		Priority: prioVmProcessesCount,
		Dims: module.Dims{
			{ID: "sys_processes", Name: "processes"},
		},
	}
	vmProcessesMemoryChart = module.Chart{
		ID:       "vm_processes",
		Title:    "Memory allocated & used by Erlang processes",
		Units:    "bytes",
		Fam:      "vm",
		Ctx:      "riak.vm.processes.memory",
		Priority: prioVmProcessesMemory,
		Dims: module.Dims{
			{ID: "memory_processes", Name: "allocated"},
			{ID: "memory_processes_used", Name: "used"},
		},
	}
)

/*
General Riak Load / Health metrics
https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#general-riak-load-health-metrics
*/
var (
	//  General Riak Load / Health metrics
	// https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#general-riak-load-health-metrics
	// Collected by Riak over the past minute

	kvSiblingsEncounteredGetChart = module.Chart{
		ID:       "kv_siblings_encountered_get",
		Title:    "Siblings encountered during GET operations by this node during the past minute",
		Units:    "siblings",
		Fam:      "load",
		Ctx:      "riak.kv.siblings_encountered.get",
		Priority: prioKvSiblingsEncounteredGet,
		Dims: module.Dims{
			{ID: "node_get_fsm_siblings_mean", Name: "mean"},
			{ID: "node_get_fsm_siblings_median", Name: "median"},
			{ID: "node_get_fsm_siblings_95", Name: "95"},
			{ID: "node_get_fsm_siblings_99", Name: "99"},
			{ID: "node_get_fsm_siblings_100", Name: "100"},
		},
	}
	kvObjectSizeGetChart = module.Chart{
		ID:       "kv_siblings_encountered_get",
		Title:    "Object size encountered by this node during the past minute",
		Units:    "bytes",
		Fam:      "load",
		Ctx:      "riak.kv.objsize.get",
		Priority: prioKvObjSizeGet,
		Dims: module.Dims{
			{ID: "node_get_fsm_objsize_mean", Name: "mean"},
			{ID: "node_get_fsm_objsize_median", Name: "median"},
			{ID: "node_get_fsm_objsize_95", Name: "95"},
			{ID: "node_get_fsm_objsize_99", Name: "99"},
			{ID: "node_get_fsm_objsize_100", Name: "100"},
		},
	}
	searchVnodeqSizeChart = module.Chart{
		ID:       "kv_siblings_encountered_get",
		Title:    "Unprocessed messages in the vnode message queues of Search in the past minute",
		Units:    "messages",
		Fam:      "load",
		Ctx:      "riak.search.vnodeq_size",
		Priority: prioSearchVnodeqSize,
		Dims: module.Dims{
			{ID: "riak_search_vnodeq_mean", Name: "mean"},
			{ID: "riak_search_vnodeq_median", Name: "median"},
			{ID: "riak_search_vnodeq_95", Name: "95"},
			{ID: "riak_search_vnodeq_99", Name: "99"},
			{ID: "riak_search_vnodeq_100", Name: "100"},
		},
	}

	// General Riak Search Load / Health metrics
	// https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#general-riak-search-load-health-metrics
	// https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#general-riak-search-load-health-metrics
	// Reported as counters.

	searchIndexErrorsChart = module.Chart{
		ID:       "search_index_errors",
		Title:    "Errors encountered by Search",
		Units:    "errors",
		Fam:      "load",
		Ctx:      "riak.search.index.errors",
		Priority: prioSearchIndexErrors,
		Dims: module.Dims{
			{ID: "search_index_fail_count", Name: "index_fail"},
			{ID: "search_index_bad_entry_count", Name: "bad_entry"},
			{ID: "search_index_extract_fail_count", Name: "extract_fail"},
		},
	}
	corePbsChart = module.Chart{
		ID:       "core_pbc",
		Title:    "Protocol buffer connections by status",
		Units:    "connections",
		Fam:      "load",
		Ctx:      "riak.core.protobuf_connections",
		Priority: prioCorePbc,
		Dims: module.Dims{
			{ID: "pbc_active", Name: "active"},
		},
	}
	coreRepairsChart = module.Chart{
		ID:       "core_repairs",
		Title:    "Number of repair operations this node has coordinated",
		Units:    "repairs",
		Fam:      "load",
		Ctx:      "riak.core.protobuf_connections",
		Priority: prioCoreRepairs,
		Dims: module.Dims{
			{ID: "read_repairs", Name: "read"},
		},
	}
	coreFsmActiveChart = module.Chart{
		ID:       "core_fsm_active",
		Title:    "Active finite state machines by kind",
		Units:    "fsms",
		Fam:      "load",
		Ctx:      "riak.core.fsm_active",
		Priority: prioCoreFsmActive,
		Dims: module.Dims{
			{ID: "node_get_fsm_active", Name: "get"},
			{ID: "node_put_fsm_active", Name: "put"},
			{ID: "index_fsm_active", Name: "secondary_index"},
			{ID: "list_fsm_active", Name: "list_keys"},
		},
	}
	coreFsmRejectedChart = module.Chart{
		ID:       "core_fsm_rejected",
		Title:    "Finite state machines being rejected by Sidejobs overload protection",
		Units:    "fsms",
		Fam:      "load",
		Ctx:      "riak.core.fsm_rejected",
		Priority: prioCoreFsmREjected,
		Dims: module.Dims{
			{ID: "node_get_fsm_rejected", Name: "get"},
			{ID: "node_put_fsm_rejected", Name: "put"},
		},
	}
)

func (c *Collector) adjustCharts(mx map[string]int64) {
	var i int
	for _, chart := range *c.Charts() {
		chart.Dims = slices.DeleteFunc(chart.Dims, func(dim *module.Dim) bool {
			_, ok := mx[dim.ID]
			if !ok {
				c.Debugf("removing dimension '%s' from chart '%s': metric not found", dim.ID, chart.ID)
			}
			return !ok
		})

		if len(chart.Dims) == 0 {
			c.Debugf("removing chart '%s': no metrics found", chart.ID)
			continue
		}

		(*c.Charts())[i] = chart
		i++
	}
}
