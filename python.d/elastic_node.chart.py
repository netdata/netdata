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
ORDER = ['search_perf_total', 'search_perf_time', 'search_latency', 'index_perf_total', 'index_perf_time',
         'index_latency', 'jvm_mem_heap', 'jvm_gc_count', 'jvm_gc_time', 'thread_pool_qr', 'fdata_cache', 'fdata_ev_tr']

CHARTS = {
    'search_perf_total': {
        'options': [None, 'Number of queries, fetches', 'queries', 'Search performance', 'es.search_query', 'stacked'],
        'lines': [
            ['query_total', 'search_total', 'incremental'],
            ["fetch_total", 'fetch_total', 'incremental'],
            ["query_current", 'search_current', 'absolute'],
            ["fetch_current", 'fetch_current', 'absolute']
        ]},
    'search_perf_time': {
        'options': [None, 'Time spent on queries, fetches', 'seconds', 'Search performance', 'es.search_time', 'stacked'],
        'lines': [
            ["query_time_in_millis", 'query', 'incremental', 1, 1000],
            ["fetch_time_in_millis", 'fetch', 'incremental', 1, 1000]
        ]},
    'search_latency': {
        'options': [None, 'Query and fetch latency', 'ms', 'Search performance', 'es.search_latency', 'stacked'],
        'lines': [
            ["query_latency", 'query', 'absolute', 1, 1000],
            ["fetch_latency", 'fetch', 'absolute', 1, 1000]
        ]},
    'index_perf_total': {
        'options': [None, 'Number of documents indexed, index refreshes, flushes', 'documents/indexes',
                    'Indexing performance', 'es.index_doc', 'stacked'],
        'lines': [
            ['indexing_index_total', 'indexed', 'incremental'],
            ["refresh_total", 'refreshes', 'incremental'],
            ["flush_total", 'flushes', 'incremental'],
            ["indexing_index_current", 'indexed_current', 'absolute'],
        ]},
    'index_perf_time': {
        'options': [None, 'Time spent on indexing, refreshing, flushing', 'seconds', 'Indexing performance',
                    'es.search_time', 'stacked'],
        'lines': [
            ['indexing_index_time_in_millis', 'indexing', 'incremental', 1, 1000],
            ['refresh_total_time_in_millis', 'refreshing', 'incremental', 1, 1000],
            ['flush_total_time_in_millis', 'flushing', 'incremental', 1, 1000]
        ]},
    'index_latency': {
        'options': [None, 'Indexing and flushing latency', 'ms', 'Indexing performance',
                    'es.index_latency', 'stacked'],
        'lines': [
            ['indexing_latency', 'indexing', 'absolute', 1, 1000],
            ['flushing_latency', 'flushing', 'absolute', 1, 1000]
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
            ["young_collection_count", 'young', 'incremental'],
            ["old_collection_count", 'old', 'incremental']
        ]},
    'jvm_gc_time': {
        'options': [None, 'Time spent on garbage collections', 'ms', 'Memory usage and gc', 'es.gc_time', 'stacked'],
        'lines': [
            ["young_collection_time_in_millis", 'young', 'incremental'],
            ["old_collection_time_in_millis", 'old', 'incremental']
        ]},
    'thread_pool_qr': {
        'options': [None, 'Number of queued/rejected threads in thread pool', 'threads', 'Queues and rejections',
                    'es.qr', 'stacked'],
        'lines': [
            ["bulk_queue", 'bulk_queue', 'absolute'],
            ["index_queue", 'index_queue', 'absolute'],
            ["search_queue", 'search_queue', 'absolute'],
            ["merge_queue", 'merge_queue', 'absolute'],
            ["bulk_rejected", 'bulk_rej', 'absolute'],
            ["index_rejected", 'index_rej', 'absolute'],
            ["search_rejected", 'search_rej', 'absolute'],
            ["merge_rejected", 'merge_rej', 'absolute']
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
        self.latency = dict()

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
        to_netdata.update(data['nodes'][node]['indices']['search'])
        to_netdata['query_latency'] = self.find_avg(to_netdata['query_total'],
                                               to_netdata['query_time_in_millis'], 'query_latency')
        to_netdata['fetch_latency'] = self.find_avg(to_netdata['fetch_total'],
                                               to_netdata['fetch_time_in_millis'], 'fetch_latency')

        # Indexing performance metrics
        for key in ['indexing', 'refresh', 'flush']:
            to_netdata.update(update_key(key, data['nodes'][node]['indices'].get(key, {})))
        to_netdata['indexing_latency'] = self.find_avg(to_netdata['indexing_index_total'],
                                               to_netdata['indexing_index_time_in_millis'], 'index_latency')
        to_netdata['flushing_latency'] = self.find_avg(to_netdata['flush_total'],
                                               to_netdata['flush_total_time_in_millis'], 'flush_latency')
        # Memory usage and garbage collection
        to_netdata.update(update_key('young', data['nodes'][node]['jvm']['gc']['collectors']['young']))
        to_netdata.update(update_key('old', data['nodes'][node]['jvm']['gc']['collectors']['old']))
        to_netdata['jvm_heap_percent'] = data['nodes'][node]['jvm']['mem']['heap_used_percent']
        to_netdata['jvm_heap_commit'] = data['nodes'][node]['jvm']['mem']['heap_committed_in_bytes']

        # Thread pool queues and rejections
        for key in ['bulk', 'index', 'search', 'merge']:
            to_netdata.update(update_key(key, data['nodes'][node]['thread_pool'].get(key, {})))

        # Fielddata cache
        to_netdata['index_fdata_mem'] = data['nodes'][node]['indices']['fielddata']['memory_size_in_bytes']
        to_netdata['index_fdata_evic'] = data['nodes'][node]['indices']['fielddata']['evictions']
        to_netdata['breakers_fdata_trip'] = data['nodes'][node]['breakers']['fielddata']['tripped']

        return to_netdata

    def find_avg(self, value1, value2, key):
        if key not in self.latency.keys():
            self.latency.update({key: [value1, value2]})
            return 0
        else:
            if not self.latency[key][0] == value1:
                latency = round(float(value2 - self.latency[key][1]) / float(value1 - self.latency[key][0]) * 1000)
                self.latency.update({key: [value1, value2]})
                return latency
            else:
                self.latency.update({key: [value1, value2]})
                return 0

def update_key(string, dictionary):
    return {'_'.join([string, k]): v for k, v in dictionary.items()}
