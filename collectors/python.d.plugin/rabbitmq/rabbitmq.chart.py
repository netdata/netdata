# -*- coding: utf-8 -*-
# Description: rabbitmq netdata python.d module
# Author: l2isbad
# SPDX-License-Identifier: GPL-3.0-or-later

from collections import namedtuple
from json import loads
from socket import gethostbyname, gaierror
from threading import Thread
try:
        from queue import Queue
except ImportError:
        from Queue import Queue

from bases.FrameworkServices.UrlService import UrlService


METHODS = namedtuple('METHODS', ['get_data', 'url', 'stats'])

NODE_STATS = [
    'fd_used',
    'mem_used',
    'sockets_used',
    'proc_used',
    'disk_free',
    'run_queue'
]

OVERVIEW_STATS = [
    'object_totals.channels',
    'object_totals.consumers',
    'object_totals.connections',
    'object_totals.queues',
    'object_totals.exchanges',
    'queue_totals.messages_ready',
    'queue_totals.messages_unacknowledged',
    'message_stats.ack',
    'message_stats.redeliver',
    'message_stats.deliver',
    'message_stats.publish'
]

ORDER = [
    'queued_messages',
    'message_rates',
    'global_counts',
    'file_descriptors',
    'socket_descriptors',
    'erlang_processes',
    'erlang_run_queue',
    'memory',
    'disk_space'
]

CHARTS = {
    'file_descriptors': {
        'options': [None, 'File Descriptors', 'descriptors', 'overview', 'rabbitmq.file_descriptors', 'line'],
        'lines': [
            ['fd_used', 'used', 'absolute']
        ]
    },
    'memory': {
        'options': [None, 'Memory', 'MiB', 'overview', 'rabbitmq.memory', 'area'],
        'lines': [
            ['mem_used', 'used', 'absolute', 1, 1 << 20]
        ]
    },
    'disk_space': {
        'options': [None, 'Disk Space', 'GiB', 'overview', 'rabbitmq.disk_space', 'area'],
        'lines': [
            ['disk_free', 'free', 'absolute', 1, 1 << 30]
        ]
    },
    'socket_descriptors': {
        'options': [None, 'Socket Descriptors', 'descriptors', 'overview', 'rabbitmq.sockets', 'line'],
        'lines': [
            ['sockets_used', 'used', 'absolute']
        ]
    },
    'erlang_processes': {
        'options': [None, 'Erlang Processes', 'processes', 'overview', 'rabbitmq.processes', 'line'],
        'lines': [
            ['proc_used', 'used', 'absolute']
        ]
    },
    'erlang_run_queue': {
        'options': [None, 'Erlang Run Queue', 'processes', 'overview', 'rabbitmq.erlang_run_queue', 'line'],
        'lines': [
            ['run_queue', 'length', 'absolute']
        ]
    },
    'global_counts': {
        'options': [None, 'Global Counts', 'counts', 'overview', 'rabbitmq.global_counts', 'line'],
        'lines': [
            ['object_totals_channels', 'channels', 'absolute'],
            ['object_totals_consumers', 'consumers', 'absolute'],
            ['object_totals_connections', 'connections', 'absolute'],
            ['object_totals_queues', 'queues', 'absolute'],
            ['object_totals_exchanges', 'exchanges', 'absolute']
        ]
    },
    'queued_messages': {
        'options': [None, 'Queued Messages', 'messages', 'overview', 'rabbitmq.queued_messages', 'stacked'],
        'lines': [
            ['queue_totals_messages_ready', 'ready', 'absolute'],
            ['queue_totals_messages_unacknowledged', 'unacknowledged', 'absolute']
        ]
    },
    'message_rates': {
        'options': [None, 'Message Rates', 'messages/s', 'overview', 'rabbitmq.message_rates', 'line'],
        'lines': [
            ['message_stats_ack', 'ack', 'incremental'],
            ['message_stats_redeliver', 'redeliver', 'incremental'],
            ['message_stats_deliver', 'deliver', 'incremental'],
            ['message_stats_publish', 'publish', 'incremental']
        ]
    }
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
        self.url = '{scheme}://{host}:{port}/api'.format(scheme=self.scheme,
                                                         host=self.host,
                                                         port=self.port)
        # Add methods
        api_node = self.url + '/nodes'
        api_overview = self.url + '/overview'
        self.methods = [METHODS(get_data=self._get_overview_stats,
                                url=api_node,
                                stats=NODE_STATS),
                        METHODS(get_data=self._get_overview_stats,
                                url=api_overview,
                                stats=OVERVIEW_STATS)]
        return UrlService.check(self)

    def _get_data(self):
        threads = list()
        queue = Queue()
        result = dict()

        for method in self.methods:
            th = Thread(target=method.get_data,
                        args=(queue, method.url, method.stats))
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

        to_netdata = fetch_data(raw_data=data, metrics=stats)
        return queue.put(to_netdata)


def fetch_data(raw_data, metrics):
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
