# -*- coding: utf-8 -*-
# Description: elastic search node stats netdata python.d module
# Author: l2isbad
# SPDX-License-Identifier: GPL-3.0+

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
    'nodes.count.data_only',
    'nodes.count.master_data',
    'nodes.count.total',
    'nodes.count.master_only',
    'nodes.count.client',
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
    'query_latency':
        {'total': 'indices_search_query_total',
         'spent_time': 'indices_search_query_time_in_millis'},
    'fetch_latency':
        {'total': 'indices_search_fetch_total',
         'spent_time': 'indices_search_fetch_time_in_millis'},
    'indexing_latency':
        {'total': 'indices_indexing_index_total',
         'spent_time': 'indices_indexing_index_time_in_millis'},
    'flushing_latency':
        {'total': 'indices_flush_total',
         'spent_time': 'indices_flush_total_time_in_millis'}
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
    'cluster_health_shards',
    'cluster_stats_nodes',
    'cluster_stats_query_cache',
    'cluster_stats_docs',
    'cluster_stats_store',
    'cluster_stats_indices_shards',
]

CHARTS = {
    'search_performance_total': {
        'options': [None, 'Queries And Fetches', 'number of', 'search performance',
                    'elastic.search_performance_total', 'stacked'],
        'lines': [
            ['indices_search_query_total', 'queries', 'incremental'],
            ['indices_search_fetch_total', 'fetches', 'incremental']
        ]},
    'search_performance_current': {
        'options': [None, 'Queries and  Fetches In Progress', 'number of', 'search performance',
                    'elastic.search_performance_current', 'stacked'],
        'lines': [
            ['indices_search_query_current', 'queries', 'absolute'],
            ['indices_search_fetch_current', 'fetches', 'absolute']
        ]},
    'search_performance_time': {
        'options': [None, 'Time Spent On Queries And Fetches', 'seconds', 'search performance',
                    'elastic.search_performance_time', 'stacked'],
        'lines': [
            ['indices_search_query_time_in_millis', 'query', 'incremental', 1, 1000],
            ['indices_search_fetch_time_in_millis', 'fetch', 'incremental', 1, 1000]
        ]},
    'search_latency': {
        'options': [None, 'Query And Fetch Latency', 'ms', 'search performance', 'elastic.search_latency', 'stacked'],
        'lines': [
            ['query_latency', 'query', 'absolute', 1, 1000],
            ['fetch_latency', 'fetch', 'absolute', 1, 1000]
        ]},
    'index_performance_total': {
        'options': [None, 'Indexed Documents, Index Refreshes, Index Flushes To Disk', 'number of',
                    'indexing performance', 'elastic.index_performance_total', 'stacked'],
        'lines': [
            ['indices_indexing_index_total', 'indexed', 'incremental'],
            ['indices_refresh_total', 'refreshes', 'incremental'],
            ['indices_flush_total', 'flushes', 'incremental']
        ]},
    'index_performance_current': {
        'options': [None, 'Number Of Documents Currently Being Indexed', 'currently indexed',
                    'indexing performance', 'elastic.index_performance_current', 'stacked'],
        'lines': [
            ['indices_indexing_index_current', 'documents', 'absolute']
        ]},
    'index_performance_time': {
        'options': [None, 'Time Spent On Indexing, Refreshing, Flushing', 'seconds', 'indexing performance',
                    'elastic.index_performance_time', 'stacked'],
        'lines': [
            ['indices_indexing_index_time_in_millis', 'indexing', 'incremental', 1, 1000],
            ['indices_refresh_total_time_in_millis', 'refreshing', 'incremental', 1, 1000],
            ['indices_flush_total_time_in_millis', 'flushing', 'incremental', 1, 1000]
        ]},
    'index_latency': {
        'options': [None, 'Indexing And Flushing Latency', 'ms', 'indexing performance',
                    'elastic.index_latency', 'stacked'],
        'lines': [
            ['indexing_latency', 'indexing', 'absolute', 1, 1000],
            ['flushing_latency', 'flushing', 'absolute', 1, 1000]
        ]},
    'index_translog_operations': {
        'options': [None, 'Translog Operations', 'count', 'translog',
                    'elastic.index_translog_operations', 'area'],
        'lines': [
            ['indices_translog_operations', 'total', 'absolute'],
            ['indices_translog_uncommitted_operations', 'uncommited', 'absolute']
        ]},
    'index_translog_size': {
        'options': [None, 'Translog Size', 'MB', 'translog',
                    'elastic.index_translog_size', 'area'],
        'lines': [
            ['indices_translog_size_in_bytes', 'total', 'absolute', 1, 1048567],
            ['indices_translog_uncommitted_size_in_bytes', 'uncommited', 'absolute', 1, 1048567]
        ]},
    'index_segments_count': {
        'options': [None, 'Total Number Of Indices Segments', 'count', 'indices segments',
                    'elastic.index_segments_count', 'line'],
        'lines': [
            ['indices_segments_count', 'segments', 'absolute']
        ]},
    'index_segments_memory_writer': {
        'options': [None, 'Index Writer Memory Usage', 'MB', 'indices segments',
                    'elastic.index_segments_memory_writer', 'area'],
        'lines': [
            ['indices_segments_index_writer_memory_in_bytes', 'total', 'absolute', 1, 1048567]
        ]},
    'index_segments_memory': {
        'options': [None, 'Indices Segments Memory Usage', 'MB', 'indices segments',
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
        ]},
    'jvm_mem_heap': {
        'options': [None, 'JVM Heap Percentage Currently in Use', 'percent', 'memory usage and gc',
                    'elastic.jvm_heap', 'area'],
        'lines': [
            ['jvm_mem_heap_used_percent', 'inuse', 'absolute']
        ]},
    'jvm_mem_heap_bytes': {
        'options': [None, 'JVM Heap Commit And Usage', 'MB', 'memory usage and gc',
                    'elastic.jvm_heap_bytes', 'area'],
        'lines': [
            ['jvm_mem_heap_committed_in_bytes', 'commited', 'absolute', 1, 1048576],
            ['jvm_mem_heap_used_in_bytes', 'used', 'absolute', 1, 1048576]
        ]},
    'jvm_buffer_pool_count': {
        'options': [None, 'JVM Buffers', 'count', 'memory usage and gc',
                    'elastic.jvm_buffer_pool_count', 'line'],
        'lines': [
            ['jvm_buffer_pools_direct_count', 'direct', 'absolute'],
            ['jvm_buffer_pools_mapped_count', 'mapped', 'absolute']
        ]},
    'jvm_direct_buffers_memory': {
        'options': [None, 'JVM Direct Buffers Memory', 'MB', 'memory usage and gc',
                    'elastic.jvm_direct_buffers_memory', 'area'],
        'lines': [
            ['jvm_buffer_pools_direct_used_in_bytes', 'used', 'absolute', 1, 1048567],
            ['jvm_buffer_pools_direct_total_capacity_in_bytes', 'total capacity', 'absolute', 1, 1048567]
        ]},
    'jvm_mapped_buffers_memory': {
        'options': [None, 'JVM Mapped Buffers Memory', 'MB', 'memory usage and gc',
                    'elastic.jvm_mapped_buffers_memory', 'area'],
        'lines': [
            ['jvm_buffer_pools_mapped_used_in_bytes', 'used', 'absolute', 1, 1048567],
            ['jvm_buffer_pools_mapped_total_capacity_in_bytes', 'total capacity', 'absolute', 1, 1048567]
        ]},
    'jvm_gc_count': {
        'options': [None, 'Garbage Collections', 'counts', 'memory usage and gc', 'elastic.gc_count', 'stacked'],
        'lines': [
            ['jvm_gc_collectors_young_collection_count', 'young', 'incremental'],
            ['jvm_gc_collectors_old_collection_count', 'old', 'incremental']
        ]},
    'jvm_gc_time': {
        'options': [None, 'Time Spent On Garbage Collections', 'ms', 'memory usage and gc',
                    'elastic.gc_time', 'stacked'],
        'lines': [
            ['jvm_gc_collectors_young_collection_time_in_millis', 'young', 'incremental'],
            ['jvm_gc_collectors_old_collection_time_in_millis', 'old', 'incremental']
        ]},
    'thread_pool_queued': {
        'options': [None, 'Number Of Queued Threads In Thread Pool', 'queued threads', 'queues and rejections',
                    'elastic.thread_pool_queued', 'stacked'],
        'lines': [
            ['thread_pool_bulk_queue', 'bulk', 'absolute'],
            ['thread_pool_write_queue', 'write', 'absolute'],
            ['thread_pool_index_queue', 'index', 'absolute'],
            ['thread_pool_search_queue', 'search', 'absolute'],
            ['thread_pool_merge_queue', 'merge', 'absolute']
        ]},
    'thread_pool_rejected': {
        'options': [None, 'Rejected Threads In Thread Pool', 'rejected threads', 'queues and rejections',
                    'elastic.thread_pool_rejected', 'stacked'],
        'lines': [
            ['thread_pool_bulk_rejected', 'bulk', 'absolute'],
            ['thread_pool_write_rejected', 'write', 'absolute'],
            ['thread_pool_index_rejected', 'index', 'absolute'],
            ['thread_pool_search_rejected', 'search', 'absolute'],
            ['thread_pool_merge_rejected', 'merge', 'absolute']
        ]},
    'fielddata_cache': {
        'options': [None, 'Fielddata Cache', 'MB', 'fielddata cache', 'elastic.fielddata_cache', 'line'],
        'lines': [
            ['indices_fielddata_memory_size_in_bytes', 'cache', 'absolute', 1, 1048576]
        ]},
    'fielddata_evictions_tripped': {
        'options': [None, 'Fielddata Evictions And Circuit Breaker Tripped Count', 'number of events',
                    'fielddata cache', 'elastic.fielddata_evictions_tripped', 'line'],
        'lines': [
            ['indices_fielddata_evictions', 'evictions', 'incremental'],
            ['indices_fielddata_tripped', 'tripped', 'incremental']
        ]},
    'cluster_health_nodes': {
        'options': [None, 'Nodes And Tasks Statistics', 'units', 'cluster health API',
                    'elastic.cluster_health_nodes', 'stacked'],
        'lines': [
            ['number_of_nodes', 'nodes', 'absolute'],
            ['number_of_data_nodes', 'data_nodes', 'absolute'],
            ['number_of_pending_tasks', 'pending_tasks', 'absolute'],
            ['number_of_in_flight_fetch', 'in_flight_fetch', 'absolute']
        ]},
    'cluster_health_status': {
        'options': [None, 'Cluster Status', 'status', 'cluster health API',
                    'elastic.cluster_health_status', 'area'],
        'lines': [
            ['status_green', 'green', 'absolute'],
            ['status_red', 'red', 'absolute'],
            ['status_foo1', None, 'absolute'],
            ['status_foo2', None, 'absolute'],
            ['status_foo3', None, 'absolute'],
            ['status_yellow', 'yellow', 'absolute']
        ]},
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
        ]},
    'cluster_stats_nodes': {
        'options': [None, 'Nodes Statistics', 'nodes', 'cluster stats API',
                    'elastic.cluster_nodes', 'stacked'],
        'lines': [
            ['nodes_count_data_only', 'data_only', 'absolute'],
            ['nodes_count_master_data', 'master_data', 'absolute'],
            ['nodes_count_total', 'total', 'absolute'],
            ['nodes_count_master_only', 'master_only', 'absolute'],
            ['nodes_count_client', 'client', 'absolute']
        ]},
    'cluster_stats_query_cache': {
        'options': [None, 'Query Cache Statistics', 'queries', 'cluster stats API',
                    'elastic.cluster_query_cache', 'stacked'],
        'lines': [
            ['indices_query_cache_hit_count', 'hit', 'incremental'],
            ['indices_query_cache_miss_count', 'miss', 'incremental']
        ]},
    'cluster_stats_docs': {
        'options': [None, 'Docs Statistics', 'count', 'cluster stats API',
                    'elastic.cluster_docs', 'line'],
        'lines': [
            ['indices_docs_count', 'docs', 'absolute']
        ]},
    'cluster_stats_store': {
        'options': [None, 'Store Statistics', 'MB', 'cluster stats API',
                    'elastic.cluster_store', 'line'],
        'lines': [
            ['indices_store_size_in_bytes', 'size', 'absolute', 1, 1048567]
        ]},
    'cluster_stats_indices_shards': {
        'options': [None, 'Indices And Shards Statistics', 'count', 'cluster stats API',
                    'elastic.cluster_indices_shards', 'stacked'],
        'lines': [
            ['indices_count', 'indices', 'absolute'],
            ['indices_shards_total', 'shards', 'absolute']
        ]},
    'host_metrics_transport': {
        'options': [None, 'Cluster Communication Transport Metrics', 'kilobit/s', 'host metrics',
                    'elastic.host_transport', 'area'],
        'lines': [
            ['transport_rx_size_in_bytes', 'in', 'incremental', 8, 1000],
            ['transport_tx_size_in_bytes', 'out', 'incremental', -8, 1000]
        ]},
    'host_metrics_file_descriptors': {
        'options': [None, 'Available File Descriptors In Percent', 'percent', 'host metrics',
                    'elastic.host_descriptors', 'area'],
        'lines': [
            ['file_descriptors_used', 'used', 'absolute', 1, 10]
        ]},
    'host_metrics_http': {
        'options': [None, 'Opened HTTP Connections', 'connections', 'host metrics',
                    'elastic.host_http_connections', 'line'],
        'lines': [
            ['http_current_open', 'opened', 'absolute', 1, 1]
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host')
        self.port = self.configuration.get('port', 9200)
        self.url = '{scheme}://{host}:{port}'.format(scheme=self.configuration.get('scheme', 'http'),
                                                     host=self.host,
                                                     port=self.port)
        self.latency = dict()
        self.methods = list()

    def check(self):
        if not all([self.host,
                    self.port,
                    isinstance(self.host, str),
                    isinstance(self.port, (str, int))]):
            self.error('Host is not defined in the module configuration file')
            return False

        # Hostname -> ip address
        try:
            self.host = gethostbyname(self.host)
        except gaierror as error:
            self.error(str(error))
            return False

        # Create URL for every Elasticsearch API
        self.methods = [METHODS(get_data=self._get_node_stats,
                                url=self.url + '/_nodes/_local/stats',
                                run=self.configuration.get('node_stats', True)),
                        METHODS(get_data=self._get_cluster_health,
                                url=self.url + '/_cluster/health',
                                run=self.configuration.get('cluster_health', True)),
                        METHODS(get_data=self._get_cluster_stats,
                                url=self.url + '/_cluster/stats',
                                run=self.configuration.get('cluster_stats', True))]

        # Remove disabled API calls from 'avail methods'
        return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            if not method.run:
                continue
            th = threading.Thread(target=method.get_data,
                                  args=(queue, method.url))
            th.start()
            threads.append(th)

        for thread in threads:
            thread.join()
            result.update(queue.get())

        return result or None

    def _get_cluster_health(self, queue, url):
        """
        Format data received from http request
        :return: dict
        """

        raw_data = self._get_raw_data(url)

        if not raw_data:
            return queue.put(dict())

        data = self.json_reply(raw_data)

        if not data:
            return queue.put(dict())

        to_netdata = fetch_data_(raw_data=data,
                                 metrics=HEALTH_STATS)

        to_netdata.update({'status_green': 0, 'status_red': 0, 'status_yellow': 0,
                           'status_foo1': 0, 'status_foo2': 0, 'status_foo3': 0})
        current_status = 'status_' + data['status']
        to_netdata[current_status] = 1

        return queue.put(to_netdata)

    def _get_cluster_stats(self, queue, url):
        """
        Format data received from http request
        :return: dict
        """

        raw_data = self._get_raw_data(url)

        if not raw_data:
            return queue.put(dict())

        data = self.json_reply(raw_data)

        if not data:
            return queue.put(dict())

        to_netdata = fetch_data_(raw_data=data,
                                 metrics=CLUSTER_STATS)

        return queue.put(to_netdata)

    def _get_node_stats(self, queue, url):
        """
        Format data received from http request
        :return: dict
        """

        raw_data = self._get_raw_data(url)

        if not raw_data:
            return queue.put(dict())

        data = self.json_reply(raw_data)

        if not data:
            return queue.put(dict())

        node = list(data['nodes'].keys())[0]
        to_netdata = fetch_data_(raw_data=data['nodes'][node],
                                 metrics=NODE_STATS)

        # Search, index, flush, fetch performance latency
        for key in LATENCY:
            try:
                to_netdata[key] = self.find_avg(total=to_netdata[LATENCY[key]['total']],
                                                spent_time=to_netdata[LATENCY[key]['spent_time']],
                                                key=key)
            except KeyError:
                continue
        if 'process_open_file_descriptors' in to_netdata and 'process_max_file_descriptors' in to_netdata:
            to_netdata['file_descriptors_used'] = round(float(to_netdata['process_open_file_descriptors'])
                                                        / to_netdata['process_max_file_descriptors'] * 1000)

        return queue.put(to_netdata)

    def json_reply(self, reply):
        try:
            return json.loads(reply)
        except ValueError as err:
            self.error(err)
            return None

    def find_avg(self, total, spent_time, key):
        if key not in self.latency:
            self.latency[key] = dict(total=total,
                                     spent_time=spent_time)
            return 0
        if self.latency[key]['total'] != total:
            latency = float(spent_time - self.latency[key]['spent_time'])\
                      / float(total - self.latency[key]['total']) * 1000
            self.latency[key]['total'] = total
            self.latency[key]['spent_time'] = spent_time
            return latency
        self.latency[key]['spent_time'] = spent_time
        return 0


def fetch_data_(raw_data, metrics):
    data = dict()
    for metric in metrics:
        value = raw_data
        metrics_list = metric.split('.')
        try:
            for m in metrics_list:
                value = value[m]
        except KeyError:
            continue
        data['_'.join(metrics_list)] = value
    return data
