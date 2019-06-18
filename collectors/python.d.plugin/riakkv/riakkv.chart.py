# -*- coding: utf-8 -*-
# Description: riak netdata python.d module
#
# See also:
# https://docs.riak.com/riak/kv/latest/using/reference/statistics-monitoring/index.html

from json import loads

from bases.FrameworkServices.UrlService import UrlService

# Riak updates the metrics at the /stats endpoint every 1 second.
# If we use `update_every = 1` here, that means we might get weird jitter in the graph,
# so the default is set to 2 seconds to prevent it.
update_every = 2

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
    "core.pbc",  # Number of currently active protocol buffer connections.
    "core.repairs",  # Total read repair operations coordinated by this node.
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
    "core.pbc": {
        "options": [None, "Protocol buffer connections by status", "connections", "load", "riak.core.protobuf_connections", "line"],
        "lines": [
            ["pbc_active", "active", "absolute"],
            # ["pbc_connects", "established_pastmin", "absolute"]
        ]
    },
    "core.repairs": {
        "options": [None, "Number of repair operations this node has coordinated", "repairs", "load", "riak.core.repairs", "line"],
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

    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw = self._get_raw_data()
        if not raw:
            return None

        try:
            return loads(raw)
        except (TypeError, ValueError) as err:
            self.error(err)
            return None
