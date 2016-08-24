# -*- coding: utf-8 -*-
# Description: memcached netdata python.d module
# Author: Pawel Krupa (paulfantom)

from base import SocketService

# default module values (can be overridden per job in `config`)
#update_every = 2
priority = 60000
retries = 60

# default job configuration (overridden by python.d.plugin)
# config = {'local': {
#             'update_every': update_every,
#             'retries': retries,
#             'priority': priority,
#             'host': 'localhost',
#             'port': 11211,
#             'unix_socket': None
#          }}

ORDER = ['cache', 'net', 'connections', 'items', 'evicted_reclaimed',
         'get', 'get_rate', 'set_rate', 'delete', 'cas', 'increment', 'decrement', 'touch', 'touch_rate']

CHARTS = {
    'cache': {
        'options': [None, 'Cache Size', 'megabytes', 'Cache', 'memcached.cache', 'stacked'],
        'lines': [
            ['used', 'used', 'absolute', 1, 1048576],
            ['avail', 'available', 'absolute', 1, 1048576]
        ]},
    'net': {
        'options': [None, 'Network', 'kilobytes/s', 'Network', 'memcached.net', 'line'],
        'lines': [
            ['bytes_read', 'read', 'incremental', 1, 1024],
            ['bytes_written', 'written', 'incremental', 1, 1024]
        ]},
    'connections': {
        'options': [None, 'Connections', 'connections/s', 'Cluster', 'memcached.connections', 'line'],
        'lines': [
            ['curr_connections', 'current', 'incremental'],
            ['rejected_connections', 'rejected', 'incremental'],
            ['total_connections', 'total', 'incremental']
        ]},
    'items': {
        'options': [None, 'Items', 'items', 'Cluster', 'memcached.items', 'line'],
        'lines': [
            ['curr_items', 'current', 'absolute'],
            ['total_items', 'total', 'absolute']
        ]},
    'evicted_reclaimed': {
        'options': [None, 'Items', 'items', 'Evicted and Reclaimed', 'memcached.evicted_reclaimed', 'line'],
        'lines': [
            ['evictions', 'evicted', 'absolute'],
            ['reclaimed', 'reclaimed', 'absolute']
        ]},
    'get': {
        'options': [None, 'Requests', 'requests', 'GET', 'memcached.get', 'stacked'],
        'lines': [
            ['get_hits', 'hits', 'percent-of-absolute-row'],
            ['get_misses', 'misses', 'percent-of-absolute-row']
        ]},
    'get_rate': {
        'options': [None, 'Rate', 'requests/s', 'GET', 'memcached.get_rate', 'line'],
        'lines': [
            ['cmd_get', 'rate', 'incremental']
        ]},
    'set_rate': {
        'options': [None, 'Rate', 'requests/s', 'SET', 'memcached.set_rate', 'line'],
        'lines': [
            ['cmd_set', 'rate', 'incremental']
        ]},
    'delete': {
        'options': [None, 'Requests', 'requests', 'DELETE', 'memcached.delete', 'stacked'],
        'lines': [
            ['delete_hits', 'hits', 'percent-of-absolute-row'],
            ['delete_misses', 'misses', 'percent-of-absolute-row'],
        ]},
    'cas': {
        'options': [None, 'Requests', 'requests', 'CAS', 'memcached.cas', 'stacked'],
        'lines': [
            ['cas_hits', 'hits', 'percent-of-absolute-row'],
            ['cas_misses', 'misses', 'percent-of-absolute-row'],
            ['cas_badval', 'bad value', 'percent-of-absolute-row']
        ]},
    'increment': {
        'options': [None, 'Requests', 'requests', 'Increment', 'memcached.increment', 'stacked'],
        'lines': [
            ['incr_hits', 'hits', 'percent-of-absolute-row'],
            ['incr_misses', 'misses', 'percent-of-absolute-row']
        ]},
    'decrement': {
        'options': [None, 'Requests', 'requests', 'Decrement', 'memcached.decrement', 'stacked'],
        'lines': [
            ['decr_hits', 'hits', 'percent-of-absolute-row'],
            ['decr_misses', 'misses', 'percent-of-absolute-row']
        ]},
    'touch': {
        'options': [None, 'Requests', 'requests', 'Touch', 'memcached.touch', 'stacked'],
        'lines': [
            ['touch_hits', 'hits', 'percent-of-absolute-row'],
            ['touch_misses', 'misses', 'percent-of-absolute-row']
        ]},
    'touch_rate': {
        'options': [None, 'Rate', 'requests/s', 'Touch', 'memcached.touch_rate', 'line'],
        'lines': [
            ['cmd_touch', 'rate', 'incremental']
        ]}
}


class Service(SocketService):
    def __init__(self, configuration=None, name=None):
        SocketService.__init__(self, configuration=configuration, name=name)
        self.request = "stats\r\n"
        self.host = "localhost"
        self.port = 11211
        self._keep_alive = True
        self.unix_socket = None
        self.order = ORDER
        self.definitions = CHARTS

    def _get_data(self):
        """
        Get data from socket
        :return: dict
        """
        try:
            raw = self._get_raw_data().split("\n")
        except AttributeError:
            self.error("no data received")
            return None
        if raw[0].startswith('ERROR'):
            self.error("Memcached returned ERROR")
            return None
        data = {}
        for line in raw:
            if line.startswith('STAT'):
                try:
                    t = line[5:].split(' ')
                    data[t[0]] = int(t[1])
                except (IndexError, ValueError):
                    pass
        try:
            data['hit_rate'] = int((data['keyspace_hits'] / float(data['keyspace_hits'] + data['keyspace_misses'])) * 100)
        except:
            data['hit_rate'] = 0

        try:
            data['avail'] = int(data['limit_maxbytes']) - int(data['bytes'])
            data['used'] = data['bytes']
        except:
            pass

        if len(data) == 0:
            self.error("received data doesn't have needed records")
            return None
        else:
            return data

    def _check_raw_data(self, data):
        if data.endswith('END\r\n'):
            return True
        else:
            return False

    def check(self):
        """
        Parse configuration, check if memcached is available
        :return: boolean
        """
        self._parse_config()
        if self.name == "":
            self.name = "local"
        self.chart_name += "_" + self.name
        data = self._get_data()
        if data is None:
            return False

        return True
