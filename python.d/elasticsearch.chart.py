# -*- coding: utf-8 -*-
# Description: elastic search node stats netdata python.d module
# Author: l2isbad

from base import UrlService
from requests import get
from socket import gethostbyname
try:
        from queue import Queue
except ImportError:
        from Queue import Queue
from threading import Thread

# default module values (can be overridden per job in `config`)
# update_every = 2
update_every = 5
priority = 60000
retries = 60

# charts order (can be overridden if you want less charts, or different order)
ORDER = ['search_perf_total', 'search_perf_time', 'search_latency', 'index_perf_total', 'index_perf_time',
         'index_latency', 'jvm_mem_heap', 'jvm_gc_count', 'jvm_gc_time', 'host_metrics_file_descriptors',
         'host_metrics_http', 'host_metrics_transport', 'thread_pool_qr', 'fdata_cache', 'fdata_ev_tr',
         'cluster_health_status', 'cluster_health_nodes', 'cluster_health_shards', 'cluster_stats_nodes',
         'cluster_stats_query_cache', 'cluster_stats_docs', 'cluster_stats_store', 'cluster_stats_indices_shards']

CHARTS = {
    'search_perf_total': {
        'options': [None, 'Number of queries, fetches', 'queries', 'Search performance', 'es.search_query', 'stacked'],
        'lines': [
            ['query_total', 'search_total', 'incremental'],
            ['fetch_total', 'fetch_total', 'incremental'],
            ['query_current', 'search_current', 'absolute'],
            ['fetch_current', 'fetch_current', 'absolute']
        ]},
    'search_perf_time': {
        'options': [None, 'Time spent on queries, fetches', 'seconds', 'Search performance', 'es.search_time', 'stacked'],
        'lines': [
            ['query_time_in_millis', 'query', 'incremental', 1, 1000],
            ['fetch_time_in_millis', 'fetch', 'incremental', 1, 1000]
        ]},
    'search_latency': {
        'options': [None, 'Query and fetch latency', 'ms', 'Search performance', 'es.search_latency', 'stacked'],
        'lines': [
            ['query_latency', 'query', 'absolute', 1, 1000],
            ['fetch_latency', 'fetch', 'absolute', 1, 1000]
        ]},
    'index_perf_total': {
        'options': [None, 'Number of documents indexed, index refreshes, flushes', 'documents/indexes',
                    'Indexing performance', 'es.index_doc', 'stacked'],
        'lines': [
            ['indexing_index_total', 'indexed', 'incremental'],
            ['refresh_total', 'refreshes', 'incremental'],
            ['flush_total', 'flushes', 'incremental'],
            ['indexing_index_current', 'indexed_current', 'absolute'],
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
            ['jvm_heap_percent', 'inuse', 'absolute'],
            ['jvm_heap_commit', 'commit', 'absolute', -1, 1048576]
        ]},
    'jvm_gc_count': {
        'options': [None, 'Count of garbage collections', 'counts', 'Memory usage and gc', 'es.gc_count', 'stacked'],
        'lines': [
            ['young_collection_count', 'young', 'incremental'],
            ['old_collection_count', 'old', 'incremental']
        ]},
    'jvm_gc_time': {
        'options': [None, 'Time spent on garbage collections', 'ms', 'Memory usage and gc', 'es.gc_time', 'stacked'],
        'lines': [
            ['young_collection_time_in_millis', 'young', 'incremental'],
            ['old_collection_time_in_millis', 'old', 'incremental']
        ]},
    'thread_pool_qr': {
        'options': [None, 'Number of queued/rejected threads in thread pool', 'threads', 'Queues and rejections',
                    'es.qr', 'stacked'],
        'lines': [
            ['bulk_queue', 'bulk_queue', 'absolute'],
            ['index_queue', 'index_queue', 'absolute'],
            ['search_queue', 'search_queue', 'absolute'],
            ['merge_queue', 'merge_queue', 'absolute'],
            ['bulk_rejected', 'bulk_rej', 'absolute'],
            ['index_rejected', 'index_rej', 'absolute'],
            ['search_rejected', 'search_rej', 'absolute'],
            ['merge_rejected', 'merge_rej', 'absolute']
        ]},
    'fdata_cache': {
        'options': [None, 'Fielddata cache size', 'MB', 'Fielddata cache', 'es.fdata_cache', 'line'],
        'lines': [
            ['index_fdata_mem', 'mem_size', 'absolute', 1, 1048576]
        ]},
    'fdata_ev_tr': {
        'options': [None, 'Fielddata evictions and circuit breaker tripped count', 'number of events',
                    'Fielddata cache', 'es.fdata_ev_tr', 'line'],
        'lines': [
            ['index_fdata_evic', 'evictions', 'incremental'],
            ['breakers_fdata_trip', 'tripped', 'incremental']
        ]},
    'cluster_health_nodes': {
        'options': [None, 'Nodes and tasks statistics', 'units', 'Cluster health API',
                    'es.cluster_health', 'stacked'],
        'lines': [
            ['health_number_of_nodes', 'nodes', 'absolute'],
            ['health_number_of_data_nodes', 'data_nodes', 'absolute'],
            ['health_number_of_pending_tasks', 'pending_tasks', 'absolute'],
            ['health_number_of_in_flight_fetch', 'inflight_fetch', 'absolute']
        ]},
    'cluster_health_status': {
        'options': [None, 'Cluster status', 'status', 'Cluster health API',
                    'es.cluster_health_status', 'area'],
        'lines': [
            ['status_green', 'green', 'absolute'],
            ['status_red', 'red', 'absolute'],
            ['status_foo1', None, 'absolute'],
            ['status_foo2', None, 'absolute'],
            ['status_foo3', None, 'absolute'],
            ['status_yellow', 'yellow', 'absolute']
        ]},
    'cluster_health_shards': {
        'options': [None, 'Shards statistics', 'shards', 'Cluster health API',
                    'es.cluster_health_sharts', 'stacked'],
        'lines': [
            ['health_active_shards', 'active_shards', 'absolute'],
            ['health_relocating_shards', 'relocating_shards', 'absolute'],
            ['health_unassigned_shards', 'unassigned', 'absolute'],
            ['health_delayed_unassigned_shards', 'delayed_unassigned', 'absolute'],
            ['health_initializing_shards', 'initializing', 'absolute'],
            ['health_active_shards_percent_as_number', 'active_percent', 'absolute']
        ]},
    'cluster_stats_nodes': {
        'options': [None, 'Nodes statistics', 'nodes', 'Cluster stats API',
                    'es.cluster_stats_nodes', 'stacked'],
        'lines': [
            ['count_data_only', 'data_only', 'absolute'],
            ['count_master_data', 'master_data', 'absolute'],
            ['count_total', 'total', 'absolute'],
            ['count_master_only', 'master_only', 'absolute'],
            ['count_client', 'client', 'absolute']
        ]},
    'cluster_stats_query_cache': {
        'options': [None, 'Query cache statistics', 'queries', 'Cluster stats API',
                    'es.cluster_stats_query_cache', 'stacked'],
        'lines': [
            ['query_cache_hit_count', 'hit', 'incremental'],
            ['query_cache_miss_count', 'miss', 'incremental']
        ]},
    'cluster_stats_docs': {
        'options': [None, 'Docs statistics', 'count', 'Cluster stats API',
                    'es.cluster_stats_docs', 'line'],
        'lines': [
            ['docs_count', 'docs', 'absolute']
        ]},
    'cluster_stats_store': {
        'options': [None, 'Store statistics', 'MB', 'Cluster stats API',
                    'es.cluster_stats_store', 'line'],
        'lines': [
            ['store_size_in_bytes', 'size', 'absolute', 1, 1048567]
        ]},
    'cluster_stats_indices_shards': {
        'options': [None, 'Indices and shards statistics', 'count', 'Cluster stats API',
                    'es.cluster_stats_ind_sha', 'stacked'],
        'lines': [
            ['indices_count', 'indices', 'absolute'],
            ['shards_total', 'shards', 'absolute']
        ]},
    'host_metrics_transport': {
        'options': [None, 'Cluster communication transport metrics', 'kbit/s', 'Host metrics',
                    'es.host_metrics_transport', 'area'],
        'lines': [
            ['transport_rx_size_in_bytes', 'in', 'incremental', 8, 1000],
            ['transport_tx_size_in_bytes', 'out', 'incremental', -8, 1000]
        ]},
    'host_metrics_file_descriptors': {
        'options': [None, 'Available file descriptors in percent', 'percent', 'Host metrics',
                    'es.host_metrics_descriptors', 'area'],
        'lines': [
            ['file_descriptors_used', 'used', 'absolute', 1, 10]
        ]},
    'host_metrics_http': {
        'options': [None, 'Opened HTTP connections', 'connections', 'Host metrics',
                    'es.host_metrics_http', 'line'],
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
        self.port = self.configuration.get('port')
        self.user = self.configuration.get('user')
        self.password = self.configuration.get('pass')
        self.latency = dict()

    def check(self):
        # We can't start if <host> AND <port> not specified
        if not all([self.host, self.port]):
            return False

        # It as a bad idea to use hostname.
        # Hostname -> ipaddress
        try:
            self.host = gethostbyname(self.host)
        except Exception as e:
            self.error(str(e))
            return False

        # HTTP Auth? NOT TESTED
        self.auth = self.user and self.password

        # Create URL for every Elasticsearch API
        url_node_stats = 'http://%s:%s/_nodes/_local/stats' % (self.host, self.port)
        url_cluster_health = 'http://%s:%s/_cluster/health' % (self.host, self.port)
        url_cluster_stats = 'http://%s:%s/_cluster/stats' % (self.host, self.port)

        # Create list of enabled API calls
        user_choice = [bool(self.configuration.get('node_stats', True)),
                       bool(self.configuration.get('cluster_health', True)),
                       bool(self.configuration.get('cluster_stats', True))]
        
        avail_methods = [(self._get_node_stats, url_node_stats), 
                        (self._get_cluster_health, url_cluster_health),
                        (self._get_cluster_stats, url_cluster_stats)]

        # Remove disabled API calls from 'avail methods'
        self.methods = [avail_methods[_] for _ in range(len(avail_methods)) if user_choice[_]]

        # Run _get_data for ALL active API calls. 
        api_result = {}
        for method in self.methods:
            api_result[method[1]] = (bool(self._get_raw_data(method[1])))

        # We can start ONLY if all active API calls returned NOT None
        if not all(api_result.values()):
            self.error('Plugin could not get data from all APIs')
            self.error('%s' % api_result)
            return False
        else:
            self.info('%s' % api_result)
            self.info('Plugin was started successfully')

            return True

    def _get_raw_data(self, url):
        try:
            if not self.auth:
                raw_data = get(url)
            else:
                raw_data = get(url, auth=(self.user, self.password))
        except Exception:
            return None

        return raw_data

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            th = Thread(target=method[0], args=(queue, method[1]))
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

        data = self._get_raw_data(url)

        if not data:
            queue.put({})
        else:
            data = data.json()

            to_netdata = dict()
            to_netdata.update(update_key('health', data))
            to_netdata.update({'status_green': 0, 'status_red': 0, 'status_yellow': 0,
                               'status_foo1': 0, 'status_foo2': 0, 'status_foo3': 0})
            to_netdata[''.join(['status_', to_netdata.get('health_status', '')])] = 1

            queue.put(to_netdata)

    def _get_cluster_stats(self, queue, url):
        """
        Format data received from http request
        :return: dict
        """

        data = self._get_raw_data(url)

        if not data:
            queue.put({})
        else:
            data = data.json()

            to_netdata = dict()
            to_netdata.update(update_key('count', data['nodes']['count']))
            to_netdata.update(update_key('query_cache', data['indices']['query_cache']))
            to_netdata.update(update_key('docs', data['indices']['docs']))
            to_netdata.update(update_key('store', data['indices']['store']))
            to_netdata['indices_count'] = data['indices']['count']
            to_netdata['shards_total'] = data['indices'].get('shards', {}).get('total')

            queue.put(to_netdata)

    def _get_node_stats(self, queue, url):
        """
        Format data received from http request
        :return: dict
        """

        data = self._get_raw_data(url)

        if not data:
            queue.put({})
        else:
            data = data.json()
            node = list(data['nodes'].keys())[0]
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

            # Host metrics
            to_netdata.update(update_key('http', data['nodes'][node]['http']))
            to_netdata.update(update_key('transport', data['nodes'][node]['transport']))
            to_netdata['file_descriptors_used'] = round(float(data['nodes'][node]['process']['open_file_descriptors'])
                                                        / data['nodes'][node]['process']['max_file_descriptors'] * 1000)
            
            queue.put(to_netdata)

    def find_avg(self, value1, value2, key):
        if key not in self.latency:
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
