# -*- coding: utf-8 -*-
# Description: elastic search node stats netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

import json
import threading

from collections import namedtuple
from socket import gethostbyname, gaierror

try:
    from queue import Queue
except ImportError:
    from Queue import Queue

from bases.FrameworkServices.UrlService import UrlService

# default module values (can be overridden per job in `config`)
update_every = 5

METHODS = namedtuple('METHODS', ['get_data', 'url', 'run'])

NODE_STATS = [
    'indices.search.fetch_current',
    'indices.search.fetch_total',
    'indices.search.query_current',
    'indices.search.query_total',
    'indices.search.query_time_in_millis',
    'indices.search.fetch_time_in_millis',
    'indices.indexing.index_total',
    'indices.indexing.index_current',
    'indices.indexing.index_time_in_millis',
    'indices.refresh.total',
    'indices.refresh.total_time_in_millis',
    'indices.flush.total',
    'indices.flush.total_time_in_millis',
    'indices.translog.operations',
    'indices.translog.size_in_bytes',
    'indices.translog.uncommitted_operations',
    'indices.translog.uncommitted_size_in_bytes',
    'indices.segments.count',
    'indices.segments.terms_memory_in_bytes',
    'indices.segments.stored_fields_memory_in_bytes',
    'indices.segments.term_vectors_memory_in_bytes',
    'indices.segments.norms_memory_in_bytes',
    'indices.segments.points_memory_in_bytes',
    'indices.segments.doc_values_memory_in_bytes',
    'indices.segments.index_writer_memory_in_bytes',
    'indices.segments.version_map_memory_in_bytes',
    'indices.segments.fixed_bit_set_memory_in_bytes',
    'jvm.gc.collectors.young.collection_count',
    'jvm.gc.collectors.old.collection_count',
    'jvm.gc.collectors.young.collection_time_in_millis',
    'jvm.gc.collectors.old.collection_time_in_millis',
    'jvm.mem.heap_used_percent',
    'jvm.mem.heap_used_in_bytes',
    'jvm.mem.heap_committed_in_bytes',
    'jvm.buffer_pools.direct.count',
    'jvm.buffer_pools.direct.used_in_bytes',
    'jvm.buffer_pools.direct.total_capacity_in_bytes',
    'jvm.buffer_pools.mapped.count',
    'jvm.buffer_pools.mapped.used_in_bytes',
    'jvm.buffer_pools.mapped.total_capacity_in_bytes',
    'thread_pool.bulk.queue',
    'thread_pool.bulk.rejected',
    'thread_pool.write.queue',
    'thread_pool.write.rejected',
    'thread_pool.index.queue',
    'thread_pool.index.rejected',
    'thread_pool.search.queue',
    'thread_pool.search.rejected',
    'thread_pool.merge.queue',
    'thread_pool.merge.rejected',
    'indices.fielddata.memory_size_in_bytes',
    'indices.fielddata.evictions',
    'breakers.fielddata.tripped',
    'http.current_open',
    'transport.rx_size_in_bytes',
    'transport.tx_size_in_bytes',
    'process.max_file_descriptors',
    'process.open_file_descriptors'
]

CLUSTER_STATS = [
    'nodes.count.data',
    'nodes.count.master',
    'nodes.count.total',
    'nodes.count.coordinating_only',
    'nodes.count.ingest',
    'indices.docs.count',
    'indices.query_cache.hit_count',
    'indices.query_cache.miss_count',
    'indices.store.size_in_bytes',
    'indices.count',
    'indices.shards.total'
]

HEALTH_STATS = [
    'number_of_nodes',
    'number_of_data_nodes',
    'number_of_pending_tasks',
    'number_of_in_flight_fetch',
    'active_shards',
    'relocating_shards',
    'unassigned_shards',
    'delayed_unassigned_shards',
    'initializing_shards',
    'active_shards_percent_as_number'
]

LATENCY = {
    'query_latency': {
        'total': 'indices_search_query_total',
        'spent_time': 'indices_search_query_time_in_millis'
    },
    'fetch_latency': {
        'total': 'indices_search_fetch_total',
        'spent_time': 'indices_search_fetch_time_in_millis'
    },
    'indexing_latency': {
        'total': 'indices_indexing_index_total',
        'spent_time': 'indices_indexing_index_time_in_millis'
    },
    'flushing_latency': {
        'total': 'indices_flush_total',
        'spent_time': 'indices_flush_total_time_in_millis'
    }
}

# charts order (can be overridden if you want less charts, or different order)
ORDER = [
    'search_performance_total',
    'search_performance_current',
    'search_performance_time',
    'search_latency',
    'index_performance_total',
    'index_performance_current',
    'index_performance_time',
    'index_latency',
    'index_translog_operations',
    'index_translog_size',
    'index_segments_count',
    'index_segments_memory_writer',
    'index_segments_memory',
    'jvm_mem_heap',
    'jvm_mem_heap_bytes',
    'jvm_buffer_pool_count',
    'jvm_direct_buffers_memory',
    'jvm_mapped_buffers_memory',
    'jvm_gc_count',
    'jvm_gc_time',
    'host_metrics_file_descriptors',
    'host_metrics_http',
    'host_metrics_transport',
    'thread_pool_queued',
    'thread_pool_rejected',
    'fielddata_cache',
    'fielddata_evictions_tripped',
    'cluster_health_status',
    'cluster_health_nodes',
    'cluster_health_pending_tasks',
    'cluster_health_flight_fetch',
    'cluster_health_shards',
    'cluster_stats_nodes',
    'cluster_stats_query_cache',
    'cluster_stats_docs',
    'cluster_stats_store',
    'cluster_stats_indices',
    'cluster_stats_shards_total',
    'index_docs_count',
    'index_store_size',
    'index_replica',
    'index_health',
]

CHARTS = {
    'search_performance_total': {
        'options': [None, 'Queries And Fetches', 'events/s', 'search performance',
                    'elastic.search_performance_total', 'stacked'],
        'lines': [
            ['indices_search_query_total', 'queries', 'incremental'],
            ['indices_search_fetch_total', 'fetches', 'incremental']
        ]
    },
    'search_performance_current': {
        'options': [None, 'Queries and  Fetches In Progress', 'events', 'search performance',
                    'elastic.search_performance_current', 'stacked'],
        'lines': [
            ['indices_search_query_current', 'queries', 'absolute'],
            ['indices_search_fetch_current', 'fetches', 'absolute']
        ]
    },
    'search_performance_time': {
        'options': [None, 'Time Spent On Queries And Fetches', 'seconds', 'search performance',
                    'elastic.search_performance_time', 'stacked'],
        'lines': [
            ['indices_search_query_time_in_millis', 'query', 'incremental', 1, 1000],
            ['indices_search_fetch_time_in_millis', 'fetch', 'incremental', 1, 1000]
        ]
    },
    'search_latency': {
        'options': [None, 'Query And Fetch Latency', 'milliseconds', 'search performance', 'elastic.search_latency',
                    'stacked'],
        'lines': [
            ['query_latency', 'query', 'absolute', 1, 1000],
            ['fetch_latency', 'fetch', 'absolute', 1, 1000]
        ]
    },
    'index_performance_total': {
        'options': [None, 'Indexed Documents, Index Refreshes, Index Flushes To Disk', 'events/s',
                    'indexing performance', 'elastic.index_performance_total', 'stacked'],
        'lines': [
            ['indices_indexing_index_total', 'indexed', 'incremental'],
            ['indices_refresh_total', 'refreshes', 'incremental'],
            ['indices_flush_total', 'flushes', 'incremental']
        ]
    },
    'index_performance_current': {
        'options': [None, 'Number Of Documents Currently Being Indexed', 'currently indexed',
                    'indexing performance', 'elastic.index_performance_current', 'stacked'],
        'lines': [
            ['indices_indexing_index_current', 'documents', 'absolute']
        ]
    },
    'index_performance_time': {
        'options': [None, 'Time Spent On Indexing, Refreshing, Flushing', 'seconds', 'indexing performance',
                    'elastic.index_performance_time', 'stacked'],
        'lines': [
            ['indices_indexing_index_time_in_millis', 'indexing', 'incremental', 1, 1000],
            ['indices_refresh_total_time_in_millis', 'refreshing', 'incremental', 1, 1000],
            ['indices_flush_total_time_in_millis', 'flushing', 'incremental', 1, 1000]
        ]
    },
    'index_latency': {
        'options': [None, 'Indexing And Flushing Latency', 'milliseconds', 'indexing performance',
                    'elastic.index_latency', 'stacked'],
        'lines': [
            ['indexing_latency', 'indexing', 'absolute', 1, 1000],
            ['flushing_latency', 'flushing', 'absolute', 1, 1000]
        ]
    },
    'index_translog_operations': {
        'options': [None, 'Translog Operations', 'operations', 'translog',
                    'elastic.index_translog_operations', 'area'],
        'lines': [
            ['indices_translog_operations', 'total', 'absolute'],
            ['indices_translog_uncommitted_operations', 'uncommitted', 'absolute']
        ]
    },
    'index_translog_size': {
        'options': [None, 'Translog Size', 'MiB', 'translog',
                    'elastic.index_translog_size', 'area'],
        'lines': [
            ['indices_translog_size_in_bytes', 'total', 'absolute', 1, 1048567],
            ['indices_translog_uncommitted_size_in_bytes', 'uncommitted', 'absolute', 1, 1048567]
        ]
    },
    'index_segments_count': {
        'options': [None, 'Total Number Of Indices Segments', 'segments', 'indices segments',
                    'elastic.index_segments_count', 'line'],
        'lines': [
            ['indices_segments_count', 'segments', 'absolute']
        ]
    },
    'index_segments_memory_writer': {
        'options': [None, 'Index Writer Memory Usage', 'MiB', 'indices segments',
                    'elastic.index_segments_memory_writer', 'area'],
        'lines': [
            ['indices_segments_index_writer_memory_in_bytes', 'total', 'absolute', 1, 1048567]
        ]
    },
    'index_segments_memory': {
        'options': [None, 'Indices Segments Memory Usage', 'MiB', 'indices segments',
                    'elastic.index_segments_memory', 'stacked'],
        'lines': [
            ['indices_segments_terms_memory_in_bytes', 'terms', 'absolute', 1, 1048567],
            ['indices_segments_stored_fields_memory_in_bytes', 'stored fields', 'absolute', 1, 1048567],
            ['indices_segments_term_vectors_memory_in_bytes', 'term vectors', 'absolute', 1, 1048567],
            ['indices_segments_norms_memory_in_bytes', 'norms', 'absolute', 1, 1048567],
            ['indices_segments_points_memory_in_bytes', 'points', 'absolute', 1, 1048567],
            ['indices_segments_doc_values_memory_in_bytes', 'doc values', 'absolute', 1, 1048567],
            ['indices_segments_version_map_memory_in_bytes', 'version map', 'absolute', 1, 1048567],
            ['indices_segments_fixed_bit_set_memory_in_bytes', 'fixed bit set', 'absolute', 1, 1048567]
        ]
    },
    'jvm_mem_heap': {
        'options': [None, 'JVM Heap Percentage Currently in Use', 'percentage', 'memory usage and gc',
                    'elastic.jvm_heap', 'area'],
        'lines': [
            ['jvm_mem_heap_used_percent', 'inuse', 'absolute']
        ]
    },
    'jvm_mem_heap_bytes': {
        'options': [None, 'JVM Heap Commit And Usage', 'MiB', 'memory usage and gc',
                    'elastic.jvm_heap_bytes', 'area'],
        'lines': [
            ['jvm_mem_heap_committed_in_bytes', 'committed', 'absolute', 1, 1048576],
            ['jvm_mem_heap_used_in_bytes', 'used', 'absolute', 1, 1048576]
        ]
    },
    'jvm_buffer_pool_count': {
        'options': [None, 'JVM Buffers', 'pools', 'memory usage and gc',
                    'elastic.jvm_buffer_pool_count', 'line'],
        'lines': [
            ['jvm_buffer_pools_direct_count', 'direct', 'absolute'],
            ['jvm_buffer_pools_mapped_count', 'mapped', 'absolute']
        ]
    },
    'jvm_direct_buffers_memory': {
        'options': [None, 'JVM Direct Buffers Memory', 'MiB', 'memory usage and gc',
                    'elastic.jvm_direct_buffers_memory', 'area'],
        'lines': [
            ['jvm_buffer_pools_direct_used_in_bytes', 'used', 'absolute', 1, 1048567],
            ['jvm_buffer_pools_direct_total_capacity_in_bytes', 'total capacity', 'absolute', 1, 1048567]
        ]
    },
    'jvm_mapped_buffers_memory': {
        'options': [None, 'JVM Mapped Buffers Memory', 'MiB', 'memory usage and gc',
                    'elastic.jvm_mapped_buffers_memory', 'area'],
        'lines': [
            ['jvm_buffer_pools_mapped_used_in_bytes', 'used', 'absolute', 1, 1048567],
            ['jvm_buffer_pools_mapped_total_capacity_in_bytes', 'total capacity', 'absolute', 1, 1048567]
        ]
    },
    'jvm_gc_count': {
        'options': [None, 'Garbage Collections', 'events/s', 'memory usage and gc', 'elastic.gc_count', 'stacked'],
        'lines': [
            ['jvm_gc_collectors_young_collection_count', 'young', 'incremental'],
            ['jvm_gc_collectors_old_collection_count', 'old', 'incremental']
        ]
    },
    'jvm_gc_time': {
        'options': [None, 'Time Spent On Garbage Collections', 'milliseconds', 'memory usage and gc',
                    'elastic.gc_time', 'stacked'],
        'lines': [
            ['jvm_gc_collectors_young_collection_time_in_millis', 'young', 'incremental'],
            ['jvm_gc_collectors_old_collection_time_in_millis', 'old', 'incremental']
        ]
    },
    'thread_pool_queued': {
        'options': [None, 'Number Of Queued Threads In Thread Pool', 'queued threads', 'queues and rejections',
                    'elastic.thread_pool_queued', 'stacked'],
        'lines': [
            ['thread_pool_bulk_queue', 'bulk', 'absolute'],
            ['thread_pool_write_queue', 'write', 'absolute'],
            ['thread_pool_index_queue', 'index', 'absolute'],
            ['thread_pool_search_queue', 'search', 'absolute'],
            ['thread_pool_merge_queue', 'merge', 'absolute']
        ]
    },
    'thread_pool_rejected': {
        'options': [None, 'Rejected Threads In Thread Pool', 'rejected threads', 'queues and rejections',
                    'elastic.thread_pool_rejected', 'stacked'],
        'lines': [
            ['thread_pool_bulk_rejected', 'bulk', 'absolute'],
            ['thread_pool_write_rejected', 'write', 'absolute'],
            ['thread_pool_index_rejected', 'index', 'absolute'],
            ['thread_pool_search_rejected', 'search', 'absolute'],
            ['thread_pool_merge_rejected', 'merge', 'absolute']
        ]
    },
    'fielddata_cache': {
        'options': [None, 'Fielddata Cache', 'MiB', 'fielddata cache', 'elastic.fielddata_cache', 'line'],
        'lines': [
            ['indices_fielddata_memory_size_in_bytes', 'cache', 'absolute', 1, 1048576]
        ]
    },
    'fielddata_evictions_tripped': {
        'options': [None, 'Fielddata Evictions And Circuit Breaker Tripped Count', 'events/s',
                    'fielddata cache', 'elastic.fielddata_evictions_tripped', 'line'],
        'lines': [
            ['indices_fielddata_evictions', 'evictions', 'incremental'],
            ['indices_fielddata_tripped', 'tripped', 'incremental']
        ]
    },
    'cluster_health_nodes': {
        'options': [None, 'Nodes Statistics', 'nodes', 'cluster health API',
                    'elastic.cluster_health_nodes', 'area'],
        'lines': [
            ['number_of_nodes', 'nodes', 'absolute'],
            ['number_of_data_nodes', 'data_nodes', 'absolute'],
        ]
    },
    'cluster_health_pending_tasks': {
        'options': [None, 'Tasks Statistics', 'tasks', 'cluster health API',
                    'elastic.cluster_health_pending_tasks', 'line'],
        'lines': [
            ['number_of_pending_tasks', 'pending_tasks', 'absolute'],
        ]
    },
    'cluster_health_flight_fetch': {
        'options': [None, 'In Flight Fetches Statistics', 'fetches', 'cluster health API',
                    'elastic.cluster_health_flight_fetch', 'line'],
        'lines': [
            ['number_of_in_flight_fetch', 'in_flight_fetch', 'absolute']
        ]
    },
    'cluster_health_status': {
        'options': [None, 'Cluster Status', 'status', 'cluster health API',
                    'elastic.cluster_health_status', 'area'],
        'lines': [
            ['status_green', 'green', 'absolute'],
            ['status_red', 'red', 'absolute'],
            ['status_yellow', 'yellow', 'absolute']
        ]
    },
    'cluster_health_shards': {
        'options': [None, 'Shards Statistics', 'shards', 'cluster health API',
                    'elastic.cluster_health_shards', 'stacked'],
        'lines': [
            ['active_shards', 'active_shards', 'absolute'],
            ['relocating_shards', 'relocating_shards', 'absolute'],
            ['unassigned_shards', 'unassigned', 'absolute'],
            ['delayed_unassigned_shards', 'delayed_unassigned', 'absolute'],
            ['initializing_shards', 'initializing', 'absolute'],
            ['active_shards_percent_as_number', 'active_percent', 'absolute']
        ]
    },
    'cluster_stats_nodes': {
        'options': [None, 'Nodes Statistics', 'nodes', 'cluster stats API',
                    'elastic.cluster_nodes', 'area'],
        'lines': [
            ['nodes_count_data', 'data', 'absolute'],
            ['nodes_count_master', 'master', 'absolute'],
            ['nodes_count_total', 'total', 'absolute'],
            ['nodes_count_ingest', 'ingest', 'absolute'],
            ['nodes_count_coordinating_only', 'coordinating_only', 'absolute']
        ]
    },
    'cluster_stats_query_cache': {
        'options': [None, 'Query Cache Statistics', 'queries', 'cluster stats API',
                    'elastic.cluster_query_cache', 'stacked'],
        'lines': [
            ['indices_query_cache_hit_count', 'hit', 'incremental'],
            ['indices_query_cache_miss_count', 'miss', 'incremental']
        ]
    },
    'cluster_stats_docs': {
        'options': [None, 'Docs Statistics', 'docs', 'cluster stats API',
                    'elastic.cluster_docs', 'line'],
        'lines': [
            ['indices_docs_count', 'docs', 'absolute']
        ]
    },
    'cluster_stats_store': {
        'options': [None, 'Store Statistics', 'MiB', 'cluster stats API',
                    'elastic.cluster_store', 'line'],
        'lines': [
            ['indices_store_size_in_bytes', 'size', 'absolute', 1, 1048567]
        ]
    },
    'cluster_stats_indices': {
        'options': [None, 'Indices Statistics', 'indices', 'cluster stats API',
                    'elastic.cluster_indices', 'line'],
        'lines': [
            ['indices_count', 'indices', 'absolute'],
        ]
    },
    'cluster_stats_shards_total': {
        'options': [None, 'Total Shards Statistics', 'shards', 'cluster stats API',
                    'elastic.cluster_shards_total', 'line'],
        'lines': [
            ['indices_shards_total', 'shards', 'absolute']
        ]
    },
    'host_metrics_transport': {
        'options': [None, 'Cluster Communication Transport Metrics', 'kilobit/s', 'host metrics',
                    'elastic.host_transport', 'area'],
        'lines': [
            ['transport_rx_size_in_bytes', 'in', 'incremental', 8, 1000],
            ['transport_tx_size_in_bytes', 'out', 'incremental', -8, 1000]
        ]
    },
    'host_metrics_file_descriptors': {
        'options': [None, 'Available File Descriptors In Percent', 'percentage', 'host metrics',
                    'elastic.host_descriptors', 'area'],
        'lines': [
            ['file_descriptors_used', 'used', 'absolute', 1, 10]
        ]
    },
    'host_metrics_http': {
        'options': [None, 'Opened HTTP Connections', 'connections', 'host metrics',
                    'elastic.host_http_connections', 'line'],
        'lines': [
            ['http_current_open', 'opened', 'absolute', 1, 1]
        ]
    },
    'index_docs_count': {
        'options': [None, 'Docs Count', 'count', 'indices', 'elastic.index_docs', 'line'],
        'lines': []
    },
    'index_store_size': {
        'options': [None, 'Store Size', 'bytes', 'indices', 'elastic.index_store_size', 'line'],
        'lines': []
    },
    'index_replica': {
        'options': [None, 'Replica', 'count', 'indices', 'elastic.index_replica', 'line'],
        'lines': []
    },
    'index_health': {
        'options': [None, 'Health', 'status', 'indices', 'elastic.index_health', 'line'],
        'lines': []
    },
}


def convert_index_store_size_to_bytes(size):
    # can be b, kb, mb, gb or None
    if size is None:
        return -1
    if size.endswith('kb'):
        return round(float(size[:-2]) * 1024)
    elif size.endswith('mb'):
        return round(float(size[:-2]) * 1024 * 1024)
    elif size.endswith('gb'):
        return round(float(size[:-2]) * 1024 * 1024 * 1024)
    elif size.endswith('tb'):
        return round(float(size[:-2]) * 1024 * 1024 * 1024 * 1024)
    elif size.endswith('b'):
        return round(float(size[:-1]))
    return -1


def convert_index_null_value(value):
    if value is None:
        return -1
    return value


def convert_index_health(health):
    if health == 'green':
        return 0
    elif health == 'yellow':
        return 1
    elif health == 'read':
        return 2
    return -1


def get_survive_any(method):
    def w(*args):
        try:
            method(*args)
        except Exception as error:
            self, queue, url = args[0], args[1], args[2]
            self.error("error during '{0}' : {1}".format(url, error))
            queue.put(dict())

    return w


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host')
        self.port = self.configuration.get('port', 9200)
        self.url = '{scheme}://{host}:{port}'.format(
            scheme=self.configuration.get('scheme', 'http'),
            host=self.host,
            port=self.port,
        )
        self.latency = dict()
        self.methods = list()
        self.collected_indices = set()

    def check(self):
        if not self.host:
            self.error('Host is not defined in the module configuration file')
            return False

        try:
            self.host = gethostbyname(self.host)
        except gaierror as error:
            self.error(repr(error))
            return False

        self.methods = [
            METHODS(
                get_data=self._get_node_stats,
                url=self.url + '/_nodes/_local/stats',
                run=self.configuration.get('node_stats', True),
            ),
            METHODS(
                get_data=self._get_cluster_health,
                url=self.url + '/_cluster/health',
                run=self.configuration.get('cluster_health', True)
            ),
            METHODS(
                get_data=self._get_cluster_stats,
                url=self.url + '/_cluster/stats',
                run=self.configuration.get('cluster_stats', True),
            ),
            METHODS(
                get_data=self._get_indices,
                url=self.url + '/_cat/indices?format=json',
                run=self.configuration.get('indices_stats', False),
            ),
        ]
        return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            if not method.run:
                continue
            th = threading.Thread(
                target=method.get_data,
                args=(queue, method.url),
            )
            th.daemon = True
            th.start()
            threads.append(th)

        for thread in threads:
            thread.join()
            result.update(queue.get())

        return result or None

    def add_index_to_charts(self, idx_name):
        for name in ('index_docs_count', 'index_store_size', 'index_replica', 'index_health'):
            chart = self.charts[name]
            dim = ['{0}_{1}'.format(idx_name, name), idx_name]
            chart.add_dimension(dim)

    @get_survive_any
    def _get_indices(self, queue, url):
        # [
        #     {
        #         "pri.store.size": "650b",
        #         "health": "yellow",
        #         "status": "open",
        #         "index": "twitter",
        #         "pri": "5",
        #         "rep": "1",
        #         "docs.count": "10",
        #         "docs.deleted": "3",
        #         "store.size": "650b"
        #     },
        #     {
        #         "status":"open",
        #         "index":".kibana_3",
        #         "health":"red",
        #         "uuid":"umAdNrq6QaOXrmZjAowTNw",
        #         "store.size":null,
        #         "pri.store.size":null,
        #         "docs.count":null,
        #         "rep":"0",
        #         "pri":"1",
        #         "docs.deleted":null
        #     },
        #     {
        #         "health" : "green",
        #         "status" : "close",
        #         "index" : "siem-events-2021.09.12",
        #         "uuid" : "mTQ-Yl5TS7S3lGoRORE-Pg",
        #         "pri" : "4",
        #         "rep" : "0",
        #         "docs.count" : null,
        #         "docs.deleted" : null,
        #         "store.size" : null,
        #         "pri.store.size" : null
        #     }
        # ]
        raw_data = self._get_raw_data(url)
        if not raw_data:
            return queue.put(dict())

        indices = self.json_parse(raw_data)
        if not indices:
            return queue.put(dict())

        charts_initialized = len(self.charts) != 0
        data = dict()
        for idx in indices:
            try:
                name = idx['index']
                is_system_index = name.startswith('.')
                if is_system_index:
                    continue

                v = {
                    '{0}_index_replica'.format(name): idx['rep'],
                    '{0}_index_health'.format(name): convert_index_health(idx['health']),
                }
                docs_count = convert_index_null_value(idx['docs.count'])
                if docs_count != -1:
                    v['{0}_index_docs_count'.format(name)] = idx['docs.count']
                size = convert_index_store_size_to_bytes(idx['store.size'])
                if size != -1:
                    v['{0}_index_store_size'.format(name)] = size
            except KeyError as error:
                self.debug("error on parsing index : {0}".format(repr(error)))
                continue

            data.update(v)
            if name not in self.collected_indices and charts_initialized:
                self.collected_indices.add(name)
                self.add_index_to_charts(name)

        return queue.put(data)

    @get_survive_any
    def _get_cluster_health(self, queue, url):
        raw = self._get_raw_data(url)
        if not raw:
            return queue.put(dict())

        parsed = self.json_parse(raw)
        if not parsed:
            return queue.put(dict())

        data = fetch_data(raw_data=parsed, metrics=HEALTH_STATS)
        dummy = {
            'status_green': 0,
            'status_red': 0,
            'status_yellow': 0,
        }
        data.update(dummy)
        current_status = 'status_' + parsed['status']
        data[current_status] = 1

        return queue.put(data)

    @get_survive_any
    def _get_cluster_stats(self, queue, url):
        raw = self._get_raw_data(url)
        if not raw:
            return queue.put(dict())

        parsed = self.json_parse(raw)
        if not parsed:
            return queue.put(dict())

        data = fetch_data(raw_data=parsed, metrics=CLUSTER_STATS)

        return queue.put(data)

    @get_survive_any
    def _get_node_stats(self, queue, url):
        raw = self._get_raw_data(url)
        if not raw:
            return queue.put(dict())

        parsed = self.json_parse(raw)
        if not parsed:
            return queue.put(dict())

        node = list(parsed['nodes'].keys())[0]
        data = fetch_data(raw_data=parsed['nodes'][node], metrics=NODE_STATS)

        # Search, index, flush, fetch performance latency
        for key in LATENCY:
            try:
                data[key] = self.find_avg(
                    total=data[LATENCY[key]['total']],
                    spent_time=data[LATENCY[key]['spent_time']],
                    key=key)
            except KeyError:
                continue
        if 'process_open_file_descriptors' in data and 'process_max_file_descriptors' in data:
            v = float(data['process_open_file_descriptors']) / data['process_max_file_descriptors'] * 1000
            data['file_descriptors_used'] = round(v)

        return queue.put(data)

    def json_parse(self, reply):
        try:
            return json.loads(reply)
        except ValueError as err:
            self.error(err)
            return None

    def find_avg(self, total, spent_time, key):
        if key not in self.latency:
            self.latency[key] = dict(total=total, spent_time=spent_time)
            return 0

        if self.latency[key]['total'] != total:
            spent_diff = spent_time - self.latency[key]['spent_time']
            total_diff = total - self.latency[key]['total']
            latency = float(spent_diff) / float(total_diff) * 1000
            self.latency[key]['total'] = total
            self.latency[key]['spent_time'] = spent_time
            return latency

        self.latency[key]['spent_time'] = spent_time
        return 0


def fetch_data(raw_data, metrics):
    data = dict()
    for metric in metrics:
        value = raw_data
        metrics_list = metric.split('.')
        try:
            for m in metrics_list:
                value = value[m]
        except (KeyError, TypeError):
            continue
        data['_'.join(metrics_list)] = value
    return data
