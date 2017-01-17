# -*- coding: utf-8 -*-
# Description: elastic search node stats netdata python.d module
# Author: l2isbad

from base import UrlService
from json import loads as to_json

# default module values (can be overridden per job in `config`)
# update_every = 2
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['search_perf_total', 'search_perf_time', 'query_latency', 'index_perf_total', 'index_perf_time',
         'index_latency', 'jvm_mem_heap', 'jvm_gc_count', 'jvm_gc_time', 'thread_pool_qr', 'fdata_cache', 'fdata_ev_tr']

CHARTS = {
    'search_perf_total': {
        'options': [None, 'Number of queries, fetches', 'queries', 'Search performance', 'es.search_query', 'stacked'],
        'lines': [
            ['search_query_total', 'search_total', 'incremental'],
            ["search_fetch_total", 'fetch_total', 'incremental'],
            ["search_query_current", 'search_current', 'absolute'],
            ["search_fetch_current", 'fetch_current', 'absolute']
        ]},
    'search_perf_time': {
        'options': [None, 'Time spent on queries, fetches', 'seconds', 'Search performance', 'es.search_time', 'stacked'],
        'lines': [
            ["search_query_time", 'query', 'incremental', 1, 1000],
            ["search_fetch_time", 'fetch', 'incremental', 1, 1000]
        ]},
    'query_latency': {
        'options': [None, 'Query latency (1 query avg time)', 'ms', 'Search performance', 'es.query_latency', 'area'],
        'lines': [
            ["query_latency", 'avg time', 'absolute', 1, 100]
        ]},
    'index_perf_total': {
        'options': [None, 'Number of documents indexed, index refreshes, flushes', 'documents/indexes',
                    'Indexing performance', 'es.index_doc', 'stacked'],
        'lines': [
            ['index_index_total', 'indexed', 'incremental'],
            ["refresh_total", 'refreshes', 'incremental'],
            ["flush_total", 'flushes', 'incremental'],
            ["index_index_current", 'indexed_current', 'absolute'],
        ]},
    'index_perf_time': {
        'options': [None, 'Time spent on indexing, refreshing, flushing', 'seconds', 'Indexing performance',
                    'es.search_time', 'stacked'],
        'lines': [
            ["index_index_time", 'indexing', 'incremental', 1, 1000],
            ["refresh_time", 'refreshing', 'incremental', 1, 1000],
            ["flush_time", 'flushing', 'incremental', 1, 1000]
        ]},
    'index_latency': {
        'options': [None, 'Index latency (1 document indexing avg time)', 'ms', 'Indexing performance',
                    'es.index_latency', 'area'],
        'lines': [
            ["index_latency", 'avg time', 'absolute', 1, 100]
        ]},
    'jvm_mem_heap': {
        'options': [None, 'JVM heap currently in use/committed', 'percent/MB', 'Memory usage and gc',
                    'es.jvm_heap', 'area'],
        'lines': [
            ["jvm_heap_percent", 'inuse', 'absolute'],
            ["jvm_heap_commit", 'commit', 'absolute', -1, 1048576]
        ]},
    'jvm_gc_count': {
        'options': [None, 'Count of garbage collections', 'counts', 'Memory usage and gc', 'es.gc_count', 'stacked'],
        'lines': [
            ["gc_young_count", 'young', 'incremental'],
            ["gc_old_count", 'old', 'incremental']
        ]},
    'jvm_gc_time': {
        'options': [None, 'Time spent on garbage collections', 'ms', 'Memory usage and gc', 'es.gc_time', 'stacked'],
        'lines': [
            ["gc_young_time", 'young', 'incremental'],
            ["gc_old_time", 'old', 'incremental']
        ]},
    'thread_pool_qr': {
        'options': [None, 'Number of queued/rejected threads in thread pool', 'threads', 'Queues and rejections',
                    'es.qr', 'stacked'],
        'lines': [
            ["thread_bulk_queue", 'bulk_queue', 'absolute'],
            ["thread_index_queue", 'index_queue', 'absolute'],
            ["thread_search_queue", 'search_queue', 'absolute'],
            ["thread_merge_queue", 'merge_queue', 'absolute'],
            ["thread_bulk_rejected", 'bulk_rej', 'absolute'],
            ["thread_index_rejected", 'index_rej', 'absolute'],
            ["thread_search_rejected", 'search_rej', 'absolute'],
            ["thread_merge_rejected", 'merge_rej', 'absolute']
        ]},
    'fdata_cache': {
        'options': [None, 'Fielddata cache size', 'MB', 'Fielddata cache', 'es.fdata_cache', 'line'],
        'lines': [
            ["index_fdata_mem", 'mem_size', 'absolute', 1, 1048576]
        ]},
    'fdata_ev_tr': {
        'options': [None, 'Fielddata evictions and circuit breaker tripped count', 'number of events',
                    'Fielddata cache', 'es.fdata_ev_tr', 'line'],
        'lines': [
            ["index_fdata_evic", 'evictions', 'incremental'],
            ["breakers_fdata_trip", 'tripped', 'incremental']
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = self.configuration.get('url', 'empty')
        self.query_latency, self.index_latency = list(), list()

    def check(self):
        if not self.url.endswith('/_nodes/_local/stats'):
            self.error('Bad URL. Should be <url>/_nodes/_local/stats')
            return False
        if not UrlService.check(self):
            self.error('Plugin could not get the data')
            return False
        else:
            self.info('Plugin was started successfully')
            return True
        
    def _get_data(self):
        """
        Format data received from http request
        :return: dict
        """
        raw_data = self._get_raw_data()

        if not raw_data:
            return None

        try:
            data = to_json(raw_data)
            node = list(data['nodes'].keys())[0]
        except Exception:
            return None


        to_netdata = dict()

        # Search performance metrics
        to_netdata['search_query_total'] = data['nodes'][node]['indices']['search']['query_total']
        to_netdata['search_query_time'] = data['nodes'][node]['indices']['search']['query_time_in_millis']
        to_netdata['search_query_current'] = data['nodes'][node]['indices']['search']['query_current']
        to_netdata['search_fetch_total'] = data['nodes'][node]['indices']['search']['fetch_total']
        to_netdata['search_fetch_time'] = data['nodes'][node]['indices']['search']['fetch_time_in_millis']
        to_netdata['search_fetch_current'] = data['nodes'][node]['indices']['search']['fetch_current']
        to_netdata['query_latency'] = find_avg(to_netdata['search_query_total'],
                                               to_netdata['search_query_time'], self.query_latency)

        # Indexing performance metrics
        to_netdata['index_index_total'] = data['nodes'][node]['indices']['indexing']['index_total']
        to_netdata['index_index_time'] = data['nodes'][node]['indices']['indexing']['index_time_in_millis']
        to_netdata['index_index_current'] = data['nodes'][node]['indices']['indexing']['index_current']
        to_netdata['refresh_total'] = data['nodes'][node]['indices']['refresh']['total']
        to_netdata['refresh_time'] = data['nodes'][node]['indices']['refresh']['total_time_in_millis']
        to_netdata['flush_total'] = data['nodes'][node]['indices']['flush']['total']
        to_netdata['flush_time'] = data['nodes'][node]['indices']['flush']['total_time_in_millis']
        to_netdata['index_latency'] = find_avg(to_netdata['index_index_total'],
                                               to_netdata['index_index_time'], self.index_latency)

        # Memory usage and garbage collection
        to_netdata['gc_young_count'] = data['nodes'][node]['jvm']['gc']['collectors']['young']['collection_count']
        to_netdata['gc_old_count'] = data['nodes'][node]['jvm']['gc']['collectors']['old']['collection_count']
        to_netdata['gc_young_time'] = data['nodes'][node]['jvm']['gc']['collectors']['young']['collection_time_in_millis']
        to_netdata['gc_old_time'] = data['nodes'][node]['jvm']['gc']['collectors']['old']['collection_time_in_millis']
        to_netdata['jvm_heap_percent'] = data['nodes'][node]['jvm']['mem']['heap_used_percent']
        to_netdata['jvm_heap_commit'] = data['nodes'][node]['jvm']['mem']['heap_committed_in_bytes']

        # Thread pool queues and rejections
        for key1 in ['bulk', 'index', 'search', 'merge']:
            for key2 in ['queue', 'rejected']:
                to_netdata['_'.join(['thread', key1, key2])] =\
                    data['nodes'][node]['thread_pool'].get(key1, {}).get(key2, 0)

        # Fielddata cache
        to_netdata['index_fdata_mem'] = data['nodes'][node]['indices']['fielddata']['memory_size_in_bytes']
        to_netdata['index_fdata_evic'] = data['nodes'][node]['indices']['fielddata']['evictions']
        to_netdata['breakers_fdata_trip'] = data['nodes'][node]['breakers']['fielddata']['tripped']

        # Here we store previous value
        self.query_latency = [to_netdata['search_query_total'], to_netdata['search_query_time']]
        self.index_latency = [to_netdata['index_index_total'], to_netdata['index_index_time']]

        return to_netdata


def find_avg(value1, value2, exist):
    if exist:
        if not value1 == exist[0]:
            return float(value2 - exist[1]) / float(value1 - exist[0]) * 100
        else:
            return 0
    else:
        return 0
