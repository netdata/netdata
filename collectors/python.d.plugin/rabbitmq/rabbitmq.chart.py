# -*- coding: utf-8 -*-
# Description: rabbitmq netdata python.d module
# Author: ilyam8
# SPDX-License-Identifier: GPL-3.0-or-later

from json import loads

from bases.FrameworkServices.UrlService import UrlService

API_NODE = 'api/nodes'
API_OVERVIEW = 'api/overview'
API_QUEUES = 'api/queues'
API_VHOSTS = 'api/vhosts'

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
    'message_stats.publish',
    'churn_rates.connection_created_details.rate',
    'churn_rates.connection_closed_details.rate',
    'churn_rates.channel_created_details.rate',
    'churn_rates.channel_closed_details.rate',
    'churn_rates.queue_created_details.rate',
    'churn_rates.queue_declared_details.rate',
    'churn_rates.queue_deleted_details.rate'
]

QUEUE_STATS = [
    'messages',
    'messages_paged_out',
    'messages_persistent',
    'messages_ready',
    'messages_unacknowledged',
    'message_stats.ack',
    'message_stats.confirm',
    'message_stats.deliver',
    'message_stats.get',
    'message_stats.get_no_ack',
    'message_stats.publish',
    'message_stats.redeliver',
    'message_stats.return_unroutable',
]

VHOST_MESSAGE_STATS = [
    'message_stats.ack',
    'message_stats.confirm',
    'message_stats.deliver',
    'message_stats.get',
    'message_stats.get_no_ack',
    'message_stats.publish',
    'message_stats.redeliver',
    'message_stats.return_unroutable',
]

ORDER = [
    'queued_messages',
    'connection_churn_rates',
    'channel_churn_rates',
    'queue_churn_rates',
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
    'connection_churn_rates': {
        'options': [None, 'Connection Churn Rates', 'operations/s', 'overview', 'rabbitmq.connection_churn_rates', 'line'],
        'lines': [
            ['churn_rates_connection_created_details_rate', 'created', 'absolute'],
            ['churn_rates_connection_closed_details_rate', 'closed', 'absolute']
        ]
    },
    'channel_churn_rates': {
        'options': [None, 'Channel Churn Rates', 'operations/s', 'overview', 'rabbitmq.channel_churn_rates', 'line'],
        'lines': [
            ['churn_rates_channel_created_details_rate', 'created', 'absolute'],
            ['churn_rates_channel_closed_details_rate', 'closed', 'absolute']
        ]
    },
    'queue_churn_rates': {
        'options': [None, 'Queue Churn Rates', 'operations/s', 'overview', 'rabbitmq.queue_churn_rates', 'line'],
        'lines': [
            ['churn_rates_queue_created_details_rate', 'created', 'absolute'],
            ['churn_rates_queue_declared_details_rate', 'declared', 'absolute'],
            ['churn_rates_queue_deleted_details_rate', 'deleted', 'absolute']
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


def vhost_chart_template(name):
    order = [
        'vhost_{0}_message_stats'.format(name),
    ]
    family = 'vhost {0}'.format(name)

    charts = {
        order[0]: {
            'options': [
                None, 'Vhost "{0}" Messages'.format(name), 'messages/s', family, 'rabbitmq.vhost_messages', 'stacked'],
            'lines': [
                ['vhost_{0}_message_stats_ack'.format(name), 'ack', 'incremental'],
                ['vhost_{0}_message_stats_confirm'.format(name), 'confirm', 'incremental'],
                ['vhost_{0}_message_stats_deliver'.format(name), 'deliver', 'incremental'],
                ['vhost_{0}_message_stats_get'.format(name), 'get', 'incremental'],
                ['vhost_{0}_message_stats_get_no_ack'.format(name), 'get_no_ack', 'incremental'],
                ['vhost_{0}_message_stats_publish'.format(name), 'publish', 'incremental'],
                ['vhost_{0}_message_stats_redeliver'.format(name), 'redeliver', 'incremental'],
                ['vhost_{0}_message_stats_return_unroutable'.format(name), 'return_unroutable', 'incremental'],
            ]
        },
    }

    return order, charts

def queue_chart_template(queue_id):
    vhost, name = queue_id
    order = [
        'vhost_{0}_queue_{1}_queued_message'.format(vhost, name),
        'vhost_{0}_queue_{1}_messages_stats'.format(vhost, name),
    ]
    family = 'vhost {0}'.format(vhost)

    charts = {
        order[0]: {
            'options': [
                None, 'Queue "{0}" in "{1}" queued messages'.format(name, vhost), 'messages', family, 'rabbitmq.queue_messages', 'line'],
            'lines': [
                ['vhost_{0}_queue_{1}_messages'.format(vhost, name), 'messages', 'absolute'],
                ['vhost_{0}_queue_{1}_messages_paged_out'.format(vhost, name), 'paged_out', 'absolute'],
                ['vhost_{0}_queue_{1}_messages_persistent'.format(vhost, name), 'persistent', 'absolute'],
                ['vhost_{0}_queue_{1}_messages_ready'.format(vhost, name), 'ready', 'absolute'],
                ['vhost_{0}_queue_{1}_messages_unacknowledged'.format(vhost, name), 'unack', 'absolute'],
            ]
        },
        order[1]: {
            'options': [
                None, 'Queue "{0}" in "{1}" messages stats'.format(name, vhost), 'messages/s', family, 'rabbitmq.queue_messages_stats', 'line'],
            'lines': [
                ['vhost_{0}_queue_{1}_message_stats_ack'.format(vhost, name), 'ack', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_confirm'.format(vhost, name), 'confirm', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_deliver'.format(vhost, name), 'deliver', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_get'.format(vhost, name), 'get', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_get_no_ack'.format(vhost, name), 'get_no_ack', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_publish'.format(vhost, name), 'publish', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_redeliver'.format(vhost, name), 'redeliver', 'incremental'],
                ['vhost_{0}_queue_{1}_message_stats_return_unroutable'.format(vhost, name), 'return_unroutable', 'incremental'],
            ]
        },
    }

    return order, charts


class VhostStatsBuilder:
    def __init__(self):
        self.stats = None

    def set(self, raw_stats):
        self.stats = raw_stats

    def name(self):
        return self.stats['name']

    def has_msg_stats(self):
        return bool(self.stats.get('message_stats'))

    def msg_stats(self):
        name = self.name()
        stats = fetch_data(raw_data=self.stats, metrics=VHOST_MESSAGE_STATS)
        return dict(('vhost_{0}_{1}'.format(name, k), v) for k, v in stats.items())

class QueueStatsBuilder:
    def __init__(self):
        self.stats = None

    def set(self, raw_stats):
        self.stats = raw_stats

    def id(self):
        return self.stats['vhost'], self.stats['name']

    def queue_stats(self):
        vhost, name = self.id()
        stats = fetch_data(raw_data=self.stats, metrics=QUEUE_STATS)
        return dict(('vhost_{0}_queue_{1}_{2}'.format(vhost, name, k), v) for k, v in stats.items())


class Service(UrlService):
    def __init__(self, configuration=None, name=None):
        UrlService.__init__(self, configuration=configuration, name=name)
        self.order = ORDER
        self.definitions = CHARTS
        self.url = '{0}://{1}:{2}'.format(
            configuration.get('scheme', 'http'),
            configuration.get('host', '127.0.0.1'),
            configuration.get('port', 15672),
        )
        self.node_name = str()
        self.vhost = VhostStatsBuilder()
        self.collected_vhosts = set()
        self.collect_queues_metrics = configuration.get('collect_queues_metrics', False)
        self.debug("collect_queues_metrics is {0}".format("enabled" if self.collect_queues_metrics else "disabled"))
        if self.collect_queues_metrics:
            self.queue = QueueStatsBuilder()
            self.collected_queues = set()

    def _get_data(self):
        data = dict()

        stats = self.get_overview_stats()
        if not stats:
            return None

        data.update(stats)

        stats = self.get_nodes_stats()
        if not stats:
            return None

        data.update(stats)

        stats = self.get_vhosts_stats()
        if stats:
            data.update(stats)

        if self.collect_queues_metrics:
            stats = self.get_queues_stats()
            if stats:
                data.update(stats)

        return data or None

    def get_overview_stats(self):
        url = '{0}/{1}'.format(self.url, API_OVERVIEW)
        self.debug("doing http request to '{0}'".format(url))
        raw = self._get_raw_data(url)
        if not raw:
            return None

        data = loads(raw)
        self.node_name = data['node']
        self.debug("found node name: '{0}'".format(self.node_name))

        stats = fetch_data(raw_data=data, metrics=OVERVIEW_STATS)
        self.debug("number of metrics: {0}".format(len(stats)))
        return stats

    def get_nodes_stats(self):
        if self.node_name == "":
            self.error("trying to get node stats, but node name is not set")
            return None

        url = '{0}/{1}/{2}'.format(self.url, API_NODE, self.node_name)
        self.debug("doing http request to '{0}'".format(url))
        raw = self._get_raw_data(url)
        if not raw:
            return None

        data = loads(raw)
        stats = fetch_data(raw_data=data, metrics=NODE_STATS)
        handle_disabled_disk_monitoring(stats)
        self.debug("number of metrics: {0}".format(len(stats)))
        return stats

    def get_vhosts_stats(self):
        url = '{0}/{1}'.format(self.url, API_VHOSTS)
        self.debug("doing http request to '{0}'".format(url))
        raw = self._get_raw_data(url)
        if not raw:
            return None

        data = dict()
        vhosts = loads(raw)
        charts_initialized = len(self.charts) > 0

        for vhost in vhosts:
            self.vhost.set(vhost)
            if not self.vhost.has_msg_stats():
                continue

            if charts_initialized and self.vhost.name() not in self.collected_vhosts:
                self.collected_vhosts.add(self.vhost.name())
                self.add_vhost_charts(self.vhost.name())

            data.update(self.vhost.msg_stats())

        self.debug("number of vhosts: {0}, metrics: {1}".format(len(vhosts), len(data)))
        return data

    def get_queues_stats(self):
        url = '{0}/{1}'.format(self.url, API_QUEUES)
        self.debug("doing http request to '{0}'".format(url))
        raw = self._get_raw_data(url)
        if not raw:
            return None

        data = dict()
        queues = loads(raw)
        charts_initialized = len(self.charts) > 0

        for queue in queues:
            self.queue.set(queue)
            if self.queue.id()[0] not in self.collected_vhosts:
                continue

            if charts_initialized and self.queue.id() not in self.collected_queues:
                self.collected_queues.add(self.queue.id())
                self.add_queue_charts(self.queue.id())

            data.update(self.queue.queue_stats())

        self.debug("number of queues: {0}, metrics: {1}".format(len(queues), len(data)))
        return data

    def add_vhost_charts(self, vhost_name):
        order, charts = vhost_chart_template(vhost_name)

        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dimensions = charts[chart_name]['lines']

            new_chart = self.charts.add_chart(params)
            for dimension in dimensions:
                new_chart.add_dimension(dimension)

    def add_queue_charts(self, queue_id):
        order, charts = queue_chart_template(queue_id)

        for chart_name in order:
            params = [chart_name] + charts[chart_name]['options']
            dimensions = charts[chart_name]['lines']

            new_chart = self.charts.add_chart(params)
            for dimension in dimensions:
                new_chart.add_dimension(dimension)


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


def handle_disabled_disk_monitoring(node_stats):
    # https://github.com/netdata/netdata/issues/7218
    # can be "disk_free": "disk_free_monitoring_disabled"
    v = node_stats.get('disk_free')
    if v and not isinstance(v, int):
        del node_stats['disk_free']
