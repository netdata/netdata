# -*- coding: utf-8 -*-
# Description: rabbitmq netdata python.d module
# Author: l2isbad

from base import UrlService
from socket import gethostbyname, gaierror
try:
        from queue import Queue
except ImportError:
        from Queue import Queue
from threading import Thread
from collections import namedtuple
from json import loads

# default module values (can be overridden per job in `config`)
update_every = 1
priority = 60000
retries = 60

METHODS = namedtuple('METHODS', ['get_data_function', 'url', 'stats'])

NODE_STATS = [('fd_used', None),
              ('mem_used', None),
              ('sockets_used', None),
              ('proc_used', None),
              ('disk_free', None)
              ]
OVERVIEW_STATS = [('object_totals.channels', None),
                  ('object_totals.consumers', None),
                  ('object_totals.connections', None),
                  ('object_totals.queues', None),
                  ('object_totals.exchanges', None),
                  ('queue_totals.messages_ready', None),
                  ('queue_totals.messages_unacknowledged', None),
                  ('message_stats.ack', None),
                  ('message_stats.redeliver', None),
                  ('message_stats.deliver', None),
                  ('message_stats.publish', None)
                  ]
ORDER = ['queued_messages', 'message_rates', 'global_counts',
         'file_descriptors', 'socket_descriptors', 'erlang_processes', 'memory', 'disk_space']

CHARTS = {
    'file_descriptors': {
        'options': [None, 'File Descriptors', 'descriptors', 'overview',
                    'rabbitmq.file_descriptors', 'line'],
        'lines': [
            ['fd_used', 'used', 'absolute']
        ]},
    'memory': {
        'options': [None, 'Memory', 'MB', 'overview',
                    'rabbitmq.memory', 'line'],
        'lines': [
            ['mem_used', 'used', 'absolute', 1, 1024 << 10]
        ]},
    'disk_space': {
        'options': [None, 'Disk Space', 'GB', 'overview',
                    'rabbitmq.disk_space', 'line'],
        'lines': [
            ['disk_free', 'free', 'absolute', 1, 1024 ** 3]
        ]},
    'socket_descriptors': {
        'options': [None, 'Socket Descriptors', 'descriptors', 'overview',
                    'rabbitmq.sockets', 'line'],
        'lines': [
            ['sockets_used', 'used', 'absolute']
        ]},
    'erlang_processes': {
        'options': [None, 'Erlang Processes', 'processes', 'overview',
                    'rabbitmq.processes', 'line'],
        'lines': [
            ['proc_used', 'used', 'absolute']
        ]},
    'global_counts': {
        'options': [None, 'Global Counts', 'counts', 'overview',
                    'rabbitmq.global_counts', 'line'],
        'lines': [
            ['channels', None, 'absolute'],
            ['consumers', None, 'absolute'],
            ['connections', None, 'absolute'],
            ['queues', None, 'absolute'],
            ['exchanges', None, 'absolute']
        ]},
    'queued_messages': {
        'options': [None, 'Queued Messages', 'messages', 'overview',
                    'rabbitmq.queued_messages', 'stacked'],
        'lines': [
            ['messages_ready', 'ready', 'absolute'],
            ['messages_unacknowledged', 'unacknowledged', 'absolute']
        ]},
    'message_rates': {
        'options': [None, 'Message Rates', 'messages/s', 'overview',
                    'rabbitmq.message_rates', 'stacked'],
        'lines': [
            ['ack', None, 'incremental'],
            ['redeliver', None, 'incremental'],
            ['deliver', None, 'incremental'],
            ['publish', None, 'incremental']
        ]}
}


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.host = self.configuration.get('host', '127.0.0.1')
        self.port = self.configuration.get('port', 15672)
        self.scheme = self.configuration.get('scheme', 'http')

    def check(self):
        # We can't start if <host> AND <port> not specified
        if not (self.host and self.port):
            self.error('Host is not defined in the module configuration file')
            return False

        # Hostname -> ip address
        try:
            self.host = gethostbyname(self.host)
        except gaierror as error:
            self.error(str(error))
            return False

        # Add handlers (auth, self signed cert accept)
        url = '%s://%s:%s/api' % (self.scheme, self.host, self.port)
        self.opener = self._build_opener(url=url)
        if not self.opener:
            return False
        # Add methods
        api_node = url + '/nodes'
        api_overview = url + '/overview'
        self.methods = [METHODS(get_data_function=self._get_overview_stats, url=api_node, stats=NODE_STATS),
                        METHODS(get_data_function=self._get_overview_stats, url=api_overview, stats=OVERVIEW_STATS)]

        result = self._get_data()
        if not result:
            self.error('_get_data() returned no data')
            return False
        self._data_from_check = result
        return True

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            th = Thread(target=method.get_data_function, args=(queue, method.url, method.stats))
            th.start()
            threads.append(th)

        for thread in threads:
            thread.join()
            result.update(queue.get())

        return result or None

    def _get_overview_stats(self, queue, url, stats):
        """
        Format data received from http request
        :return: dict
        """

        raw_data = self._get_raw_data(url)

        if not raw_data:
            return queue.put(dict())
        data = loads(raw_data)
        data = data[0] if isinstance(data, list) else data

        to_netdata = fetch_data_(raw_data=data, metrics_list=stats)
        return queue.put(to_netdata)


def fetch_data_(raw_data, metrics_list):
    to_netdata = dict()
    for metric, new_name in metrics_list:
        value = raw_data
        for key in metric.split('.'):
            try:
                value = value[key]
            except (KeyError, TypeError):
                break
        if not isinstance(value, dict):
            to_netdata[new_name or key] = value
    return to_netdata
