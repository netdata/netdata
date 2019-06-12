# -*- coding: utf-8 -*-
# Description: riak netdata python.d module
#
# See also:
# https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html

from json import loads

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
update_every = 2
priority = 60000
retries = 60

KEYS = [
    # KV Throughput Statistics
    "node_gets",
    "node_gets_total",
    "node_puts",
    "node_puts_total",
    # CRDT Throughput Statistics
    "node_gets_counter",
    "node_gets_counter_total",
    "node_gets_set",
    "node_gets_set_total",
    "node_gets_map",
    "node_gets_map_total",
    "node_puts_counter",
    "node_puts_counter_total",
    "node_puts_set",
    "node_puts_set_total",
    "node_puts_map",
    "node_puts_map_total",
    "object_merge",
    "object_merge_total",
    "object_counter_merge",
    "object_counter_merge_total",
    "object_set_merge",
    "object_set_merge_total",
    "object_map_merge",
    "object_map_merge_total",
    # Protocol Buffers Statistics
    "pbc_active",
    "pbc_connects",
    "pbc_connects_total",
    # Read Repair Statistics
    "read_repairs",
    "read_repairs_total",
    "skipped_read_repairs",
    "skipped_read_repairs_total",
    "read_repairs_counter",
    "read_repairs_counter_total",
    "read_repairs_set",
    "read_repairs_set_total",
    "read_repairs_map",
    "read_repairs_map_total",
    "read_repairs_primary_notfound_one",
    "read_repairs_primary_notfound_count",
    "read_repairs_primary_outofdate_one",
    "read_repairs_primary_outofdate_count",
    "read_repairs_fallback_notfound_one",
    "read_repairs_fallback_notfound_count",
    "read_repairs_fallback_outofdate_one",
    "read_repairs_fallback_outofdate_count",
    # Overload Protection Statistics
    "node_get_fsm_active",
    "node_get_fsm_active_60s",
    "node_get_fsm_in_rate",
    "node_get_fsm_out_rate",
    "node_get_fsm_rejected",
    "node_get_fsm_rejected_60s",
    "node_get_fsm_rejected_total",
    "node_get_fsm_errors",
    "node_get_fsm_errors_total",
    "node_put_fsm_active",
    "node_put_fsm_active_60s",
    "node_put_fsm_in_rate",
    "node_put_fsm_out_rate",
    "node_put_fsm_rejected",
    "node_put_fsm_rejected_60s",
    "node_put_fsm_rejected_total",
    # VNode Statistics
    "riak_kv_vnodes_running",
    "vnode_gets",
    "vnode_gets_total",
    "vnode_puts",
    "vnode_puts_total",
    "vnode_counter_update",
    "vnode_counter_update_total",
    "vnode_set_update",
    "vnode_set_update_total",
    "vnode_map_update",
    "vnode_map_update_total",
    "vnode_index_deletes",
    "vnode_index_deletes_postings",
    "vnode_index_deletes_postings_total",
    "vnode_index_deletes_total",
    "vnode_index_reads",
    "vnode_index_reads_total",
    "vnode_index_refreshes",
    "vnode_index_refreshes_total",
    "vnode_index_writes",
    "vnode_index_writes_postings",
    "vnode_index_writes_postings_total",
    "vnode_index_writes_total",
    "dropped_vnode_requests_total",
    # Search Statistics
    "search_index_fail_one",
    "search_index_fail_count",
    "search_index_throughput_one",
    "search_index_throughput_count",
    "search_query_fail_one",
    "search_query_fail_count",
    "search_query_throughput_one",
    "search_query_throughput_count",
    # Keylisting Statistics
    "list_fsm_active",
    "list_fsm_create",
    "list_fsm_create_total",
    "list_fsm_create_error",
    "list_fsm_create_error_total",
    # Secondary Indexing Statistics
    "index_fsm_active",
    "index_fsm_create",
    "index_fsm_create_error",
    # MapReduce Statistics
    "riak_pipe_vnodes_running",
    "executing_mappers",
    "pipeline_active",
    "pipeline_create_count",
    "pipeline_create_error_count",
    "pipeline_create_error_one",
    "pipeline_create_one",
    # Ring Statistics
    "rings_reconciled",
    "rings_reconciled_total",
    "converge_delay_last",
    "converge_delay_max",
    "converge_delay_mean",
    "converge_delay_min",
    "rebalance_delay_last",
    "rebalance_delay_max",
    "rebalance_delay_mean",
    "rebalance_delay_min",
    "rejected_handoffs",
    "handoff_timeouts",
    "coord_redirs_total",
    "gossip_received",
    "ignored_gossip_total",
    # System Statistics
    "mem_allocated",
    "mem_total",
    "memory_atom",
    "memory_atom_used",
    "memory_binary",
    "memory_code",
    "memory_ets",
    "memory_processes",
    "memory_processes_used",
    "memory_system",
    "memory_total",
    "sys_monitor_count",
    "sys_port_count",
    "sys_process_count",
    # Misc. Statistics
    "late_put_fsm_coordinator_ack",
    "postcommit_fail",
    "precommit_fail",
    "leveldb_read_block_error"
]

STAT_KEYS = [
    # KV Latency and Object Statistics
    "node_get_fsm_objsize",
    "node_get_fsm_siblings",
    "node_get_fsm_time",
    "node_put_fsm_time",
    # CRDT Latency and Object Statistics
    "node_get_fsm_counter_time",
    "node_get_fsm_set_time",
    "node_get_fsm_map_time",
    "node_put_fsm_counter_time",
    "node_put_fsm_set_time",
    "node_put_fsm_map_time",
    "node_get_fsm_counter_objsize",
    "node_get_fsm_counter_siblings",
    "node_get_fsm_set_objsize",
    "node_get_fsm_set_siblings",
    "node_get_fsm_map_objsize",
    "node_get_fsm_map_siblings",
    "object_merge_time",
    "object_counter_merge_time",
    "object_set_merge_time",
    "object_map_merge_time",
    "counter_actor_counts",
    "set_actor_counts",
    "map_actor_counts",
    # VNode Latency and Object Statistics
    "vnode_get_fsm_time",
    "vnode_put_fsm_time",
    "vnode_counter_update_time",
    "vnode_set_update_time",
    "vnode_map_update_time"
]

SEARCH_LATENCY_KEYS = [
    "search_query_latency",
    "search_index_latency"
]

VNODEQ_KEYS = [
    "riak_kv_vnodeq",
    "riak_pipe_vnodeq"
]

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    # Throughput metrics
    # https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#throughput-metrics
    # Collected in totals.
    "kv.node_operations",  # K/V node operations.
    "dt.vnode_updates",  # Data type vnode updates.
    "search.queries",  # Search queries on the node.
    "search.documents",  # Documents indexed by Search.
    "consistent.operations",  # Consistent node operations.

    # Latency metrics
    # https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#throughput-metrics
    # Collected for the past minute in milliseconds,
    # returned from riak in microseconds.
    "kv.latency.get",  # K/V GET FSM traversal latency.
    "kv.latency.put",  # K/V PUT FSM traversal latency.
    "dt.latency.counter",  # Update Counter Data type latency.
    "dt.latency.set",  # Update Set Data type latency.
    "dt.latency.map",  # Update Map Data type latency.
    "search.latency.query",  # Search query latency.
    "search.latency.index",  # Time it takes for search to index a new document.
    "consistent.latency.get",  # Strong consistent read latency.
    "consistent.latency.put",  # Strong consistent write latency.

    # Erlang resource usage metrics
    # https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#erlang-resource-usage-metrics
    # Processes collected as a gauge,
    # memory collected as Megabytes, returned as bytes from Riak.
    "vm.processes",  # Number of processes currently running in the Erlang VM.
    "vm.memory.processes",  # Total amount of memory allocated & used for Erlang processes.

    # General Riak Load / Health metrics
    # https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#general-riak-load-health-metrics
    # The following are collected by Riak over the past minute:
    "kv.siblings_encountered.get",  # Siblings encountered during GET operations by this node.
    "kv.objsize.get",  # Object size encountered by this node.
    "search.vnodeq_size",  # Number of unprocessed messages in the vnode message queues (Search).
    # The following are calculated in total, or as gauges:
    "search.index_errors",  # Errors of the search subsystem while indexing documents.
    "core.pbc_active",  # Number of currently active protocol buffer connections.
    "core.read_repairs",  # Total read repair operations coordinated by this node.
    "core.fsm_active",  # Active finite state machines by kind.
    "core.fsm_rejected",  # Rejected finite state machines by kind.

    # General Riak Search Load / Health metrics
    # https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html#general-riak-search-load-health-metrics
    # Reported as counters.
    "search.errors",  # Write and read errors of the Search subsystem.
]

CHARTS = {
    # Throughput metrics
    "kv.node_operations": {
        "options": [None, "Reads & writes coordinated by this node", "operations/s", "throughput", "riak.kv.throughput", "line"],
        "lines": [
            ["node_gets_total", "gets", "incremental"],
            ["node_puts_total", "puts", "incremental"]
        ]
    },
    "dt.vnode_updates": {
        "options": [None, "Update operations coordinated by local vnodes by data type", "operations/s", "throughput", "riak.dt.vnode_updates", "line"],
        "lines": [
            ["vnode_counter_update_total", "counters", "incremental"],
            ["vnode_set_update_total", "sets", "incremental"],
            ["vnode_map_update_total", "maps", "incremental"],
        ]
    },
    "search.queries": {
        "options": [None, "Search queries on the node", "queries/s", "throughput", "riak.search", "line"],
        "lines": [
            ["search_query_throughput_count", "queries", "incremental"]
        ]
    },
    "search.documents": {
        "options": [None, "Documents indexed by search", "documents/s", "throughput", "riak.search.documents", "line"],
        "lines": [
            ["search_index_throughput_count", "indexed", "incremental"]
        ]
    },
    "consistent.operations": {
        "options": [None, "Consistent node operations", "operations/s", "throughput", "riak.consistent.operations", "line"],
        "lines": [
            ["consistent_gets_total", "gets", "incremental"],
            ["consistent_puts_total", "puts", "incremental"],
        ]
    },

    # Latency metrics
    "kv.latency.get": {
        "options": [None, "Time between reception of a client GET request and subsequent response to client", "ms", "latency", "riak.kv.latency.get", "line"],
        "lines": [
            ["node_get_fsm_time_mean", "mean", "absolute", 1, 1000],
            ["node_get_fsm_time_median", "median", "absolute", 1, 1000],
            ["node_get_fsm_time_95", "95", "absolute", 1, 1000],
            ["node_get_fsm_time_99", "99", "absolute", 1, 1000],
            ["node_get_fsm_time_100", "100", "absolute", 1, 1000],
        ]
    },
    "kv.latency.put": {
        "options": [None, "Time between reception of a client PUT request and subsequent response to client", "ms", "latency", "riak.kv.latency.put", "line"],
        "lines": [
            ["node_put_fsm_time_mean", "mean", "absolute", 1, 1000],
            ["node_put_fsm_time_median", "median", "absolute", 1, 1000],
            ["node_put_fsm_time_95", "95", "absolute", 1, 1000],
            ["node_put_fsm_time_99", "99", "absolute", 1, 1000],
            ["node_put_fsm_time_100", "100", "absolute", 1, 1000],
        ]
    },
    "dt.latency.counter": {
        "options": [None, "Time it takes to perform an Update Counter operation", "ms", "latency", "riak.dt.latency.counter_merge", "line"],
        "lines": [
            ["object_counter_merge_time_mean", "mean", "absolute", 1, 1000],
            ["object_counter_merge_time_median", "median", "absolute", 1, 1000],
            ["object_counter_merge_time_95", "95", "absolute", 1, 1000],
            ["object_counter_merge_time_99", "99", "absolute", 1, 1000],
            ["object_counter_merge_time_100", "100", "absolute", 1, 1000],
        ]
    },
    "dt.latency.set": {
        "options": [None, "Time it takes to perform an Update Set operation", "ms", "latency", "riak.dt.latency.set_merge", "line"],
        "lines": [
            ["object_set_merge_time_mean", "mean", "absolute", 1, 1000],
            ["object_set_merge_time_median", "median", "absolute", 1, 1000],
            ["object_set_merge_time_95", "95", "absolute", 1, 1000],
            ["object_set_merge_time_99", "99", "absolute", 1, 1000],
            ["object_set_merge_time_100", "100", "absolute", 1, 1000],
        ]
    },
    "dt.latency.map": {
        "options": [None, "Time it takes to perform an Update Map operation", "ms", "latency", "riak.dt.latency.map_merge", "line"],
        "lines": [
            ["object_map_merge_time_mean", "mean", "absolute", 1, 1000],
            ["object_map_merge_time_median", "median", "absolute", 1, 1000],
            ["object_map_merge_time_95", "95", "absolute", 1, 1000],
            ["object_map_merge_time_99", "99", "absolute", 1, 1000],
            ["object_map_merge_time_100", "100", "absolute", 1, 1000],
        ]
    },
    "search.latency.query": {
        "options": [None, "Search query latency", "ms", "latency", "riak.search.latency.query", "line"],
        "lines": [
            ["search_query_latency_median", "median", "absolute", 1, 1000],
            ["search_query_latency_min", "min", "absolute", 1, 1000],
            ["search_query_latency_95", "95", "absolute", 1, 1000],
            ["search_query_latency_99", "99", "absolute", 1, 1000],
            ["search_query_latency_999", "999", "absolute", 1, 1000],
            ["search_query_latency_max", "max", "absolute", 1, 1000],
        ]
    },
    "search.latency.index": {
        "options": [None, "Time it takes Search to index a new document", "ms", "latency", "riak.search.latency.index", "line"],
        "lines": [
            ["search_index_latency_median", "median", "absolute", 1, 1000],
            ["search_index_latency_min", "min", "absolute", 1, 1000],
            ["search_index_latency_95", "95", "absolute", 1, 1000],
            ["search_index_latency_99", "99", "absolute", 1, 1000],
            ["search_index_latency_999", "999", "absolute", 1, 1000],
            ["search_index_latency_max", "max", "absolute", 1, 1000],
        ]
    },

    # Riak Strong Consistency metrics
    "consistent.latency.get": {
        "options": [None, "Strongly consistent read latency", "ms", "latency", "riak.consistent.latency.get", "line"],
        "lines": [
            ["consistent_get_time_mean", "mean", "absolute", 1, 1000],
            ["consistent_get_time_median", "median", "absolute", 1, 1000],
            ["consistent_get_time_95", "95", "absolute", 1, 1000],
            ["consistent_get_time_99", "99", "absolute", 1, 1000],
            ["consistent_get_time_100", "100", "absolute", 1, 1000],
        ]
    },
    "consistent.latency.put": {
        "options": [None, "Strongly consistent write latency", "ms", "latency", "riak.consistent.latency.put", "line"],
        "lines": [
            ["consistent_put_time_mean", "mean", "absolute", 1, 1000],
            ["consistent_put_time_median", "median", "absolute", 1, 1000],
            ["consistent_put_time_95", "95", "absolute", 1, 1000],
            ["consistent_put_time_99", "99", "absolute", 1, 1000],
            ["consistent_put_time_100", "100", "absolute", 1, 1000],
        ]
    },

    # BEAM metrics
    "vm.processes": {
        "options": [None, "Total processes running in the Erlang VM", "total", "vm", "riak.vm", "line"],
        "lines": [
            ["sys_process_count", "processes", "absolute"],
        ]
    },
    "vm.memory.processes": {
        "options": [None, "Memory allocated & used by Erlang processes", "MB", "vm", "riak.vm.memory.processes", "line"],
        "lines": [
            ["memory_processes", "allocated", "absolute", 1, 1024 * 1024],
            ["memory_processes_used", "used", "absolute", 1, 1024 * 1024]
        ]
    },

    # General Riak Load/Health metrics
    "kv.siblings_encountered.get": {
        "options": [None, "Number of siblings encountered during GET operations by this node during the past minute", "siblings", "load", "riak.kv.siblings_encountered.get", "line"],
        "lines": [
            ["node_get_fsm_siblings_mean", "mean", "absolute"],
            ["node_get_fsm_siblings_median", "median", "absolute"],
            ["node_get_fsm_siblings_95", "95", "absolute"],
            ["node_get_fsm_siblings_99", "99", "absolute"],
            ["node_get_fsm_siblings_100", "100", "absolute"],
        ]
    },
    "kv.objsize.get": {
        "options": [None, "Object size encountered by this node during the past minute", "KB", "load", "riak.kv.objsize.get", "line"],
        "lines": [
            ["node_get_fsm_objsize_mean", "mean", "absolute", 1, 1024],
            ["node_get_fsm_objsize_median", "median", "absolute", 1, 1024],
            ["node_get_fsm_objsize_95", "95", "absolute", 1, 1024],
            ["node_get_fsm_objsize_99", "99", "absolute", 1, 1024],
            ["node_get_fsm_objsize_100", "100", "absolute", 1, 1024],
        ]
    },
    "search.vnodeq_size": {
        "options": [None, "Number of unprocessed messages in the vnode message queues of Search on this node in the past minute", "messages", "load", "riak.search.vnodeq_size", "line"],
        "lines": [
            ["riak_search_vnodeq_mean", "mean", "absolute"],
            ["riak_search_vnodeq_median", "median", "absolute"],
            ["riak_search_vnodeq_95", "95", "absolute"],
            ["riak_search_vnodeq_99", "99", "absolute"],
            ["riak_search_vnodeq_100", "100", "absolute"],
        ]
    },
    "search.index_errors": {
        "options": [None, "Number of document index errors encountered by Search", "errors", "load", "riak.search.index", "line"],
        "lines": [
            ["search_index_fail_count", "errors", "absolute"]
        ]
    },
    "core.pbc_active": {
        "options": [None, "Number of currently active protocol buffer connections", "connections", "load", "riak.core.protobuf_connections", "line"],
        "lines": [
            ["pbc_active", "active", "absolute"],
            # ["pbc_connects", "established_pastmin", "absolute"]
        ]
    },
    "core.read_repairs": {
        "options": [None, "Number of read repair operations this node has coordinated", "repairs", "load", "riak.core.repairs", "line"],
        "lines": [
            ["read_repairs", "read", "absolute"]
        ]
    },
    "core.fsm_active": {
        "options": [None, "Active finite state machines by kind", "fsms", "load", "riak.core.fsm_active", "line"],
        "lines": [
            ["node_get_fsm_active", "get", "absolute"],
            ["node_put_fsm_active", "put", "absolute"],
            ["index_fsm_active", "secondary index", "absolute"],
            ["list_fsm_active", "list keys", "absolute"]
        ]
    },
    "core.fsm_rejected": {
        # Writing "Sidejob's" here seems to cause some weird issues: it results in this chart being rendered in
        # its own context and additionally, moves the entire Riak graph all the way up to the top of the Netdata
        # dashboard for some reason.
        "options": [None, "Finite state machines being rejected by Sidejobs overload protection", "fsms", "load", "riak.core.fsm_rejected", "line"],
        "lines": [
            ["node_get_fsm_rejected", "get", "absolute"],
            ["node_put_fsm_rejected", "put", "absolute"]
        ]
    },

    # General Riak Search Load / Health metrics
    "search.errors": {
        "options": [None, "Number of writes to Search failed due to bad data format by reason", "writes", "load", "riak.search.index", "line"],
        "lines": [
            ["search_index_bad_entry_count", "bad_entry", "absolute"],
            ["search_index_extract_fail_count", "extract_fail", "absolute"],
        ]
    }
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.keys = list(KEYS)
        for k in ["mean", "median", "95", "99", "100"]:
            for m in STAT_KEYS:
                self.keys.append(m + "_" + k)
        for k in ["min", "max", "mean", "median", "95", "99", "999"]:
            for m in SEARCH_LATENCY_KEYS:
                self.keys.append(m + "_" + k)
        for k in ["min", "max", "mean", "median", "total"]:
            for m in VNODEQ_KEYS:
                self.keys.append(m + "_" + k)

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw = self._get_raw_data()
        if not raw:
            return None

        data = dict()
        stats = loads(raw)
        for k in self.keys:
            if k in stats:
                data[k] = stats[k]

        return data or None
